package sitepages

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/changes"
)

// userAgent — браузерный UA: часть страниц Telligent/WAF отдаётся только «браузерам».
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// maxTextLen — верхний предел хранимого текста страницы (символов).
const maxTextLen = 200000

// fileExts — расширения файлов-документов. Их обходит конвейер документов
// (src/scraper), а не слой страниц: здесь они пропускаются.
var fileExts = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".rtf": true, ".ppt": true, ".pptx": true, ".zip": true, ".txt": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".svg": true,
	".css": true, ".js": true, ".ico": true, ".webp": true, ".mp4": true,
}

// Store — то, что краулеру нужно от хранилища (реализуется PostgresStore).
// Интерфейс позволяет подменять хранилище в тестах без Postgres.
type Store interface {
	Upsert(ctx context.Context, p *Page) (string, error)
}

// Report — итог одного обхода.
type Report struct {
	StartedAt time.Time
	Visited   int
	New       int
	Changed   int
	Unchanged int
	Errors    []string
}

// Crawler обходит публичные страницы сайта в пределах хостов стартовых URL.
type Crawler struct {
	Seeds    []string
	Store    Store
	HTTP     *http.Client
	Delay    time.Duration
	MaxPages int
	// Changes — необязательная лента изменений: при появлении/изменении страницы
	// фиксируется событие с EntityType = changes.EntitySitePage.
	Changes changes.Recorder
	// GetProxyURL — необязательный резолвер активного прокси (на каждый запрос).
	GetProxyURL func() string
}

// New создаёт краулер с разумными значениями по умолчанию.
func New(seeds []string, st Store) *Crawler {
	return &Crawler{
		Seeds:    seeds,
		Store:    st,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
		Delay:    3 * time.Second,
		MaxPages: 300,
	}
}

// UseProxy направляет запросы через статический прокси (пустой URL — без изменений).
func (c *Crawler) UseProxy(proxyURL string) {
	if strings.TrimSpace(proxyURL) == "" {
		return
	}
	pu, err := url.Parse(proxyURL)
	if err != nil {
		return
	}
	c.HTTP = &http.Client{Timeout: c.timeout(), Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
}

// UseDynamicProxy направляет запросы через прокси, выбираемый fn на каждый запрос
// (управление из админки на лету). Пустая строка — запрос идёт напрямую.
func (c *Crawler) UseDynamicProxy(fn func() string) {
	if fn == nil {
		return
	}
	c.GetProxyURL = fn
	tr := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			raw := strings.TrimSpace(fn())
			if raw == "" {
				return nil, nil
			}
			return url.Parse(raw)
		},
	}
	c.HTTP = &http.Client{Timeout: c.timeout(), Transport: tr}
}

func (c *Crawler) timeout() time.Duration {
	if c.HTTP != nil && c.HTTP.Timeout > 0 {
		return c.HTTP.Timeout
	}
	return 60 * time.Second
}

// Run обходит сайт начиная со стартовых URL в пределах их хостов и возвращает отчёт.
func (c *Crawler) Run(ctx context.Context) (*Report, error) {
	rep := &Report{StartedAt: time.Now()}
	if c.MaxPages <= 0 {
		c.MaxPages = 300
	}

	// Допустимые хосты — хосты стартовых URL.
	allowedHosts := map[string]bool{}
	var queue []string
	for _, seed := range c.Seeds {
		seed = strings.TrimSpace(seed)
		if seed == "" {
			continue
		}
		if u, err := url.Parse(seed); err == nil && u.Host != "" {
			allowedHosts[strings.ToLower(u.Host)] = true
			queue = append(queue, seed)
		}
	}
	if len(queue) == 0 {
		return rep, fmt.Errorf("нет валидных стартовых URL")
	}

	visited := map[string]bool{}
	for len(queue) > 0 && rep.Visited < c.MaxPages {
		select {
		case <-ctx.Done():
			return rep, ctx.Err()
		default:
		}

		pageURL := queue[0]
		queue = queue[1:]
		norm := normalizeURL(pageURL)
		if visited[norm] {
			continue
		}
		visited[norm] = true

		data, err := c.fetch(ctx, pageURL)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", pageURL, err))
			continue
		}
		rep.Visited++

		page, links := c.parse(data, pageURL)
		if err := c.save(ctx, page, rep); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("сохранение %s: %v", pageURL, err))
		}

		// Ставим в очередь ссылки в пределах разрешённых хостов (без файлов).
		base, _ := url.Parse(pageURL)
		for _, href := range links {
			abs := resolve(base, href)
			if abs == "" {
				continue
			}
			u, err := url.Parse(abs)
			if err != nil || !allowedHosts[strings.ToLower(u.Host)] {
				continue
			}
			if fileExts[strings.ToLower(path.Ext(u.Path))] {
				continue
			}
			if !visited[normalizeURL(abs)] {
				queue = append(queue, abs)
			}
		}
	}
	return rep, nil
}

// save фиксирует страницу в хранилище и (при новизне/изменении) — в ленте.
func (c *Crawler) save(ctx context.Context, page *Page, rep *Report) error {
	status, err := c.Store.Upsert(ctx, page)
	if err != nil {
		return err
	}
	switch status {
	case UpsertNew:
		rep.New++
		c.recordChange(ctx, page, changes.KindNew, "Новая страница сайта")
	case UpsertChanged:
		rep.Changed++
		c.recordChange(ctx, page, changes.KindUpdated, "Изменилось содержимое страницы сайта")
	default:
		rep.Unchanged++
	}
	return nil
}

func (c *Crawler) recordChange(ctx context.Context, page *Page, kind changes.Kind, summary string) {
	if c.Changes == nil {
		return
	}
	_ = c.Changes.Record(ctx, changes.Event{
		EntityType: changes.EntitySitePage,
		EntityID:   page.ID,
		Title:      page.Title,
		Category:   page.Section,
		Kind:       kind,
		SourceURL:  page.URL,
		Summary:    summary,
		DetectedAt: time.Now(),
	})
}

// fetch выполняет GET с браузерным UA и возвращает тело при статусе 200.
func (c *Crawler) fetch(ctx context.Context, u string) ([]byte, error) {
	time.Sleep(c.Delay)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("статус %d", resp.StatusCode)
	}
	// Ограничиваем размер тела (страницы, не файлы) — 4 МБ достаточно.
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// parse извлекает из HTML страницу (title, summary, section, hash) и список ссылок.
func (c *Crawler) parse(data []byte, pageURL string) (*Page, []string) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	page := &Page{
		ID:      pageID(pageURL),
		URL:     pageURL,
		Section: sectionFromURL(pageURL),
		Status:  StatusActive,
	}
	if err != nil {
		// Не удалось разобрать HTML — сохраняем хотя бы хэш сырого тела.
		sum := sha256.Sum256(data)
		page.ContentHash = hex.EncodeToString(sum[:])
		return page, nil
	}

	var title, metaDesc string
	var links []string
	var textBuf strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.ElementNode && n.Data == "title" && title == "":
			title = strings.TrimSpace(nodeText(n))
		case n.Type == html.ElementNode && n.Data == "meta":
			var name, content string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "name", "property":
					name = strings.ToLower(a.Val)
				case "content":
					content = a.Val
				}
			}
			if metaDesc == "" && (name == "description" || name == "og:description") {
				metaDesc = strings.TrimSpace(content)
			}
		case n.Type == html.ElementNode && n.Data == "a":
			for _, a := range n.Attr {
				if a.Key == "href" && a.Val != "" {
					links = append(links, a.Val)
				}
			}
		case n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style"):
			return // не считаем как текст
		case n.Type == html.TextNode:
			if t := strings.TrimSpace(n.Data); t != "" {
				textBuf.WriteString(t)
				textBuf.WriteString(" ")
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)

	bodyText := strings.Join(strings.Fields(textBuf.String()), " ")
	page.Title = title
	if page.Title == "" {
		page.Title = pageURL
	}
	page.Summary = metaDesc
	if page.Summary == "" {
		page.Summary = truncate(bodyText, 300)
	}
	// Полный видимый текст — для чтения в просмотрщике админки (с разумным лимитом).
	page.Text = truncate(bodyText, maxTextLen)
	sum := sha256.Sum256([]byte(bodyText))
	page.ContentHash = hex.EncodeToString(sum[:])
	return page, links
}

// nodeText собирает текст всех потомков узла в одну строку.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

// resolve приводит href к абсолютному URL относительно страницы, отбрасывая
// якоря, mailto:, tel: и javascript:.
func resolve(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "javascript:") {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil || base == nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// pageID детерминированно генерирует ID страницы из нормализованного URL.
func pageID(rawURL string) string {
	sum := sha1.Sum([]byte(normalizeURL(rawURL)))
	return hex.EncodeToString(sum[:])
}

// normalizeURL приводит URL к каноничному виду: схема/хост в нижний регистр,
// без query, фрагмента и хвостового «/».
func normalizeURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return rawURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.RawQuery = ""
	u.Fragment = ""
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String()
}

// sectionFromURL выводит человекочитаемый раздел из пути URL: сегменты пути
// (без файла-страницы) через « / ». Пустой путь → «Главная».
func sectionFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return "Главная"
	}
	segs := strings.Split(p, "/")
	// Если последний сегмент похож на файл-страницу (содержит «.»), отбрасываем его.
	if last := segs[len(segs)-1]; strings.Contains(last, ".") {
		segs = segs[:len(segs)-1]
	}
	if len(segs) == 0 {
		return "Главная"
	}
	return strings.Join(segs, " / ")
}
