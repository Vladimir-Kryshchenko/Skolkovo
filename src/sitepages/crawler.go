package sitepages

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
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

// goneMarker — необязательная способность хранилища помечать страницу как
// недоступную (404/410). Проверяется через type-assertion, чтобы не ломать
// тестовые заглушки, реализующие только Upsert.
type goneMarker interface {
	MarkGone(ctx context.Context, id string) (changed bool, err error)
}

// Renderer — необязательный JS-рендерер (реализуется через chromedp в main).
// Если задан, HTML страницы берётся через него: для разделов, где ссылки и
// контент подставляются JavaScript-ом и не видны при простом HTTP-запросе.
type Renderer interface {
	Render(ctx context.Context, url string) ([]byte, error)
}

// statusError — HTTP-ответ с не-200 статусом (нужен для детекции 404/410).
type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("статус %d", e.code) }

// Report — итог одного обхода.
type Report struct {
	StartedAt time.Time
	Visited   int
	New       int
	Changed   int
	Unchanged int
	Gone      int // страниц помечено недоступными (404/410)
	Errors    []string
}

// Crawler обходит публичные страницы сайта в пределах хостов стартовых URL.
type Crawler struct {
	Seeds    []string
	Store    Store
	HTTP     *http.Client
	Delay    time.Duration
	MaxPages int
	// MaxPerPath ограничивает число query-вариантов одного пути (host+path) в
	// очереди — защита от «бесконечных» пространств (календари, фасетные фильтры),
	// когда query-параметры значимы для пагинации. 0 — без ограничения.
	MaxPerPath int
	// Concurrency — число параллельных загрузчиков (волновой BFS). <=1 —
	// последовательный обход. Скачивание идёт параллельно, разбор/сохранение —
	// под общей блокировкой. Delay применяется к каждому запросу (вежливость).
	Concurrency int
	// Changes — необязательная лента изменений: при появлении/изменении страницы
	// фиксируется событие с EntityType = changes.EntitySitePage.
	Changes changes.Recorder
	// GetProxyURL — необязательный резолвер активного прокси (на каждый запрос).
	GetProxyURL func() string
	// Renderer — необязательный JS-рендерер (chromedp). Если задан, HTML берётся
	// через него (статус всё равно проверяется обычным HTTP — для детекции 404).
	Renderer Renderer
}

// New создаёт краулер с разумными значениями по умолчанию.
// MaxPages = 0 означает «без лимита» (обход ограничен только числом уникальных
// страниц в пределах разрешённых хостов).
func New(seeds []string, st Store) *Crawler {
	return &Crawler{
		Seeds:       seeds,
		Store:       st,
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		Delay:       3 * time.Second,
		MaxPages:    0,
		MaxPerPath:  1000, // запас для пагинации, но защита от бесконечных календарей
		Concurrency: 1,    // по умолчанию последовательно (детерминированно для тестов)
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
	unlimited := c.MaxPages <= 0 // 0 = без лимита страниц

	// Допустимые хосты и origin-ы (scheme://host) стартовых URL.
	allowedHosts := map[string]bool{}
	originSet := map[string]bool{}
	var origins []string
	for _, seed := range c.Seeds {
		if u, err := url.Parse(strings.TrimSpace(seed)); err == nil && u.Host != "" {
			allowedHosts[strings.ToLower(u.Host)] = true
			origin := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
			if !originSet[origin] {
				originSet[origin] = true
				origins = append(origins, origin)
			}
		}
	}
	if len(allowedHosts) == 0 {
		return rep, fmt.Errorf("нет валидных стартовых URL")
	}

	var (
		queue    []string
		visited  = map[string]bool{}
		enqueued = map[string]bool{}
		perPath  = map[string]int{}
		capped   = map[string]bool{}
	)
	// enqueue ставит URL в очередь, если он в разрешённом хосте, не файл, ещё не
	// посещён/не в очереди и не превышен лимит query-вариантов пути.
	enqueue := func(raw string) {
		u, err := url.Parse(raw)
		if err != nil || !allowedHosts[strings.ToLower(u.Host)] {
			return
		}
		if fileExts[strings.ToLower(path.Ext(u.Path))] {
			return
		}
		if isExcludedURL(u) {
			return // навигационная «ловушка»/служебная страница — не обходим
		}
		norm := normalizeURL(raw)
		if visited[norm] || enqueued[norm] {
			return
		}
		if c.MaxPerPath > 0 {
			pk := pathKey(raw)
			if perPath[pk] >= c.MaxPerPath {
				if !capped[pk] {
					capped[pk] = true
					rep.Errors = append(rep.Errors, fmt.Sprintf(
						"лимит query-вариантов пути достигнут (%d), часть страниц %s пропущена", c.MaxPerPath, pk))
				}
				return
			}
			perPath[pk]++
		}
		enqueued[norm] = true
		queue = append(queue, raw)
	}

	for _, seed := range c.Seeds {
		enqueue(strings.TrimSpace(seed))
	}
	// Засеваем очередь из sitemap.xml (и robots.txt → Sitemap) каждого origin —
	// так находятся страницы-сироты, на которые нет внутренних ссылок.
	for _, loc := range c.discoverFromSitemaps(ctx, origins) {
		enqueue(loc)
	}

	// Волновой BFS: на каждой волне страницы скачиваются параллельно (до
	// Concurrency загрузчиков), а разбор/сохранение/постановка ссылок в очередь —
	// под общей блокировкой. Следующая волна — собранные ссылки текущей.
	conc := c.Concurrency
	if conc < 1 {
		conc = 1
	}
	var mu sync.Mutex
	sem := make(chan struct{}, conc)

	frontier := queue
	queue = nil // дальше queue служит буфером для enqueue под блокировкой
	for len(frontier) > 0 {
		select {
		case <-ctx.Done():
			return rep, ctx.Err()
		default:
		}
		if !unlimited && rep.Visited >= c.MaxPages {
			break
		}
		// Ограничиваем волну остатком бюджета MaxPages.
		batch := frontier
		if !unlimited {
			if rem := c.MaxPages - rep.Visited; rem < len(batch) {
				batch = batch[:rem]
			}
		}

		var next []string
		var wg sync.WaitGroup
		for _, pageURL := range batch {
			norm := normalizeURL(pageURL)
			mu.Lock()
			if visited[norm] {
				mu.Unlock()
				continue
			}
			visited[norm] = true
			mu.Unlock()

			wg.Add(1)
			sem <- struct{}{}
			go func(pageURL string) {
				defer wg.Done()
				defer func() { <-sem }()

				data, err := c.fetch(ctx, pageURL) // сеть — вне блокировки
				if err != nil {
					var se *statusError
					gone := errors.As(err, &se) && (se.code == http.StatusNotFound || se.code == http.StatusGone)
					mu.Lock()
					if gone { // окончательный 404/410 — помечаем недоступной
						c.markGone(ctx, pageURL, rep)
					}
					rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", pageURL, err))
					mu.Unlock()
					return
				}
				// JS-рендеринг (если подключён) — тоже вне блокировки.
				if c.Renderer != nil {
					if rendered, rerr := c.Renderer.Render(ctx, pageURL); rerr == nil && len(rendered) > 0 {
						data = rendered
					} else if rerr != nil {
						mu.Lock()
						rep.Errors = append(rep.Errors, fmt.Sprintf("render %s: %v", pageURL, rerr))
						mu.Unlock()
					}
				}
				page, links := c.parse(data, pageURL) // разбор HTML — вне блокировки
				base, _ := url.Parse(pageURL)

				mu.Lock()
				rep.Visited++
				if err := c.save(ctx, page, rep); err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("сохранение %s: %v", pageURL, err))
				}
				before := len(queue)
				for _, href := range links {
					if abs := resolve(base, href); abs != "" {
						enqueue(abs) // добавляет уникальные ссылки в queue-буфер
					}
				}
				next = append(next, queue[before:]...) // переносим новинки в след. волну
				queue = queue[:before]
				mu.Unlock()
			}(pageURL)
		}
		wg.Wait()
		frontier = next
	}
	return rep, nil
}

// FetchPublished загружает одну страницу (с JS-рендерингом, если он включён) и
// возвращает извлечённую дату публикации на сайте, либо nil, если её не нашлось.
// Используется командой бэкфилла для уже сохранённых страниц без даты — без
// записи текста/хэша, только дата. Сетевые ошибки/404 возвращаются как есть.
func (c *Crawler) FetchPublished(ctx context.Context, pageURL string) (*time.Time, error) {
	data, err := c.fetch(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	if c.Renderer != nil {
		if rendered, rerr := c.Renderer.Render(ctx, pageURL); rerr == nil && len(rendered) > 0 {
			data = rendered
		}
	}
	page, _ := c.parse(data, pageURL)
	return page.PublishedAt, nil
}

// markGone помечает ранее сохранённую страницу недоступной (404/410) и фиксирует
// это в ленте изменений. Если хранилище не умеет MarkGone — тихо пропускает.
func (c *Crawler) markGone(ctx context.Context, pageURL string, rep *Report) {
	gm, ok := c.Store.(goneMarker)
	if !ok {
		return
	}
	changed, err := gm.MarkGone(ctx, pageID(pageURL))
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("пометка gone %s: %v", pageURL, err))
		return
	}
	if changed {
		rep.Gone++
		c.recordChange(ctx,
			&Page{ID: pageID(pageURL), URL: pageURL, Section: sectionFromURL(pageURL)},
			changes.KindRemoved, "Страница сайта стала недоступна (404/410)")
	}
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
// Делает до 3 попыток при сетевых ошибках и ответах 5xx/429 (sk.ru за WAF
// периодически отдаёт 502/503) — иначе единичный сбой обрывал бы весь обход.
func (c *Crawler) fetch(ctx context.Context, u string) ([]byte, error) {
	const attempts = 3
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Second * time.Duration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		body, retryable, err := c.fetchOnce(ctx, u)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

// fetchOnce выполняет один GET. retryable=true для временных сбоев (сеть, 5xx, 429).
func (c *Crawler) fetchOnce(ctx context.Context, u string) (body []byte, retryable bool, err error) {
	time.Sleep(c.Delay)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err // сетевая ошибка/таймаут — повторяемо
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, &statusError{code: resp.StatusCode}
	}
	// Ограничиваем размер тела (страницы, не файлы) — 4 МБ достаточно.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, true, err
	}
	return data, false, nil
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
	var sig publishedSignals // кандидаты на дату публикации (см. published.go)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		collectPublishedSignal(n, &sig)
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
	// Дата публикации на сайте Сколково (из разметки/видимой надписи; см. published.go).
	page.PublishedAt = sig.resolve()
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

// IDForURL — публичная обёртка над pageID: ID страницы по её URL (для ссылок на
// просмотрщик из результатов семантического поиска, где известен только URL).
func IDForURL(rawURL string) string {
	return pageID(rawURL)
}

// trackingParams — рекламные/сессионные query-метки: их отбрасываем при
// нормализации, чтобы utm-хвосты не плодили дубликаты одной страницы.
var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "gclid": true, "fbclid": true,
	"yclid": true, "_openstat": true, "_ga": true, "mc_cid": true, "mc_eid": true,
}

// normalizeURL приводит URL к каноничному виду: схема/хост в нижний регистр, без
// фрагмента и хвостового «/». Значимые query-параметры (пагинация ?page=N,
// фильтры) СОХРАНЯЮТСЯ и канонизируются по порядку — иначе вторые и последующие
// страницы листингов недостижимы. Рекламные метки (utm_* и т.п.) отбрасываются.
func normalizeURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return rawURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if trackingParams[strings.ToLower(k)] {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode() // Encode сортирует ключи — канонический порядок
	}
	return u.String()
}

// excludedPathSegments — сегменты пути, которые краулер НЕ обходит: фасетные
// тег-фильтры и пользовательские/служебные разделы Telligent (dochub.sk.ru).
// Именно они порождают комбинаторный взрыв страниц (паучья ловушка): архив
// новостей предлагает «показать с ещё одним тегом», и пути вида
// …/tags/{t1}/{t1+t2}/{t1+t2+t3}/… множатся практически бесконечно. Кодированные
// кириллические теги делают каждую комбинацию уникальным URL. Совпадение по
// ЛЮБОМУ сегменту пути исключает страницу из обхода.
var excludedPathSegments = map[string]bool{
	"tags": true, "tag": true, // тег-фильтры — главный источник взрыва
	"user": true, "users": true, // профили/служебные страницы пользователей
	"members": true, "membership": true,
	"search": true,                 // страницы поиска
	"login":  true, "logout": true, // авторизация
	"signin": true, "signout": true,
	"register": true, "signup": true,
}

// AddExcludedSegments расширяет встроенный денилист сегментов пути значениями из
// конфигурации (SITEPAGES_EXCLUDE_SEGMENTS) — чтобы добавлять новые «ловушки» без
// правки кода и редеплоя. Вызывается один раз на старте, до запуска обхода/разметки
// (дальше карта только читается). Влияет и на обход (enqueue), и на разметку
// (EnrichBatch), и на prune — критерий ловушки единый.
func AddExcludedSegments(segs []string) {
	for _, s := range segs {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			excludedPathSegments[s] = true
		}
	}
}

// maxPathSegments — предохранитель от неизвестных «ловушек»: пути глубже этого
// почти всегда фасетная навигация, а не контент. Реальные статьи архива
// укладываются в ~8 сегментов (например /news/b/news/archive/ГГГГ/ММ/ДД/слаг.aspx).
const maxPathSegments = 12

// isExcludedURL сообщает, что URL — навигационная «ловушка» или служебная
// страница и обходить её не нужно (см. excludedPathSegments). Без этого фильтра
// краулер уходит в комбинаторный взрыв тег-фильтров dochub.sk.ru.
func isExcludedURL(u *url.URL) bool {
	n := 0
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "" {
			continue
		}
		n++
		seg = strings.ToLower(seg)
		if excludedPathSegments[seg] {
			return true
		}
		// Сегмент-файл (login.aspx, register.aspx) — сравниваем имя без расширения.
		if ext := path.Ext(seg); ext != "" && excludedPathSegments[strings.TrimSuffix(seg, ext)] {
			return true
		}
	}
	return n > maxPathSegments
}

// dropExcluded отбирает из pages только НЕ-ловушечные страницы (см. isExcludedURL)
// и возвращает отфильтрованный срез и число отброшенных. Общий фильтр для разметки
// (EnrichBatch) — гарантия, что токены не тратятся на denylist-URL, попавшие в БД.
func dropExcluded(pages []*Page) (kept []*Page, skipped int) {
	kept = pages[:0:0]
	for _, p := range pages {
		if u, err := url.Parse(p.URL); err == nil && isExcludedURL(u) {
			skipped++
			continue
		}
		kept = append(kept, p)
	}
	return kept, skipped
}

// pathKey — host+path без query (для лимита query-вариантов одного пути).
func pathKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
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
