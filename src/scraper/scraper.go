// Package scraper обходит сайт dochub.sk.ru, скачивает документы
// и фиксирует их метаданные в реестре. Соблюдает Crawl-delay из robots.txt.
package scraper

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/extract"
	"baza-skolkovo/src/common/feed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// VersionArchiver сохраняет снимок версии документа (извлечённый текст редакции)
// для последующего семантического диффа. Реализуется store.PostgresVersionStore.
// Опционален: если nil — история версий не ведётся.
type VersionArchiver interface {
	SaveVersion(ctx context.Context, v *model.DocVersion) (int, error)
	CountVersions(ctx context.Context, documentID string) (int, error)
}

// userAgent — браузерный UA: часть страниц Telligent отдаётся только «браузерам».
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// fileExts — расширения, считающиеся документами для скачивания.
var fileExts = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".rtf": true, ".ppt": true, ".pptx": true, ".zip": true, ".txt": true,
}

// CategoryNames — человекочитаемые названия категорий по слагу страницы.
var CategoryNames = map[string]string{
	"legislative_acts":      "Законодательные акты",
	"design_rules":          "Правила проектирования",
	"other":                 "Иные нормативные документы",
	"development":           "Развитие территории",
	"tenders":               "Закупки и тендеры",
	"unactual_documents":    "Утратившие силу",
	"anti_corruption":       "Антикоррупция",
	"cybersec_and_persdata": "Кибербезопасность и перс. данные",
}

// Scraper обходит документы Сколково.
type Scraper struct {
	BaseURL  string
	RSSURL   string // лента-каталог документов (основной источник списка)
	OutDir   string // корень Документы_Сколково
	Store    store.Store
	HTTP     *http.Client
	Delay    time.Duration
	MaxPages int
	// Changes — необязательная лента изменений. Если задана, скрейпер фиксирует
	// каждое новое/обновлённое/устаревшее изменение документа.
	Changes changes.Recorder
	// Versions — необязательное хранилище версий документов. Если задано,
	// скрейпер сохраняет снимок текста каждой редакции (для диффа «что изменилось»).
	Versions VersionArchiver
	// GetProxyURL — необязательный резолвер активного прокси (ProxyManager).
	// Устанавливается через UseDynamicProxy.
	GetProxyURL func() string
}

// recordChange фиксирует изменение документа в ленте (если она подключена).
func (s *Scraper) recordChange(ctx context.Context, doc model.Document, isNew bool, summary string) {
	if s.Changes == nil {
		return
	}
	kind := changes.KindUpdated
	if isNew {
		kind = changes.KindNew
	}
	if doc.Status == model.StatusOutdated {
		kind = changes.KindOutdated
	}
	_ = s.Changes.Record(ctx, changes.Event{
		EntityType: changes.EntityDocument,
		EntityID:   doc.ID,
		Title:      doc.Title,
		Category:   doc.Category,
		Kind:       kind,
		SourceURL:  doc.SourceURL,
		Summary:    summary,
		DetectedAt: time.Now(),
	})
}

// New создаёт скрейпер с разумными значениями по умолчанию.
func New(baseURL, outDir string, st store.Store) *Scraper {
	return &Scraper{
		BaseURL:  baseURL,
		RSSURL:   deriveRSS(baseURL),
		OutDir:   outDir,
		Store:    st,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
		Delay:    3 * time.Second, // Crawl-delay из robots.txt dochub.sk.ru
		MaxPages: 200,
	}
}

// UseProxy направляет каталожные HTTP-запросы (RSS + страницы документов) через
// статический прокси. Нужно, когда сервер не имеет прямого доступа к dochub.sk.ru
// (например, зарубежный дата-центр за гео/WAF-блокировкой). Пустой proxyURL — без изменений.
func (s *Scraper) UseProxy(proxyURL string) {
	if strings.TrimSpace(proxyURL) == "" {
		return
	}
	pu, err := url.Parse(proxyURL)
	if err != nil {
		log.Printf("[scraper] некорректный PROXY_URL %q: %v — прокси не применён", proxyURL, err)
		return
	}
	timeout := s.clientTimeout()
	s.HTTP = &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
}

// UseDynamicProxy направляет каталожные запросы через прокси, выбираемый функцией
// fn на КАЖДЫЙ запрос. Позволяет управлять прокси из админки (ProxyManager) на
// лету: следующий цикл планировщика подхватит активный прокси без перезапуска.
// fn возвращает пустую строку — запрос идёт напрямую.
func (s *Scraper) UseDynamicProxy(fn func() string) {
	if fn == nil {
		return
	}
	s.GetProxyURL = fn
	tr := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			raw := strings.TrimSpace(fn())
			if raw == "" {
				return nil, nil // без прокси
			}
			return url.Parse(raw)
		},
	}
	s.HTTP = &http.Client{Timeout: s.clientTimeout(), Transport: tr}
}

func (s *Scraper) clientTimeout() time.Duration {
	if s.HTTP != nil && s.HTTP.Timeout > 0 {
		return s.HTTP.Timeout
	}
	return 60 * time.Second
}

// DeriveRSS возвращает URL ленты-каталога документов для базового URL раздела.
func DeriveRSS(baseURL string) string { return deriveRSS(baseURL) }

// deriveRSS возвращает URL ленты-каталога документов для базового URL раздела.
func deriveRSS(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/rss.aspx"
	u.RawQuery = ""
	return u.String()
}

// Report — итог одного запуска парсинга.
type Report struct {
	StartedAt  time.Time
	Visited    int
	Catalogued int // новых записей каталога из RSS
	Downloaded int // файлов фактически скачано (HTML-обход)
	Updated    int
	Unchanged  int
	Errors     []string
}

// ingestRSS заводит документы из RSS-ленты-каталога в реестр (статус «на_проверке»).
//
// Тела файлов на dochub.sk.ru закрыты WAF (страницы /m/docs/ отдают 403),
// поэтому здесь сохраняются метаданные без локального файла; контент
// добавляется вручную через админку или headless-загрузчиком (отдельная задача).
func (s *Scraper) ingestRSS(ctx context.Context, rep *Report) error {
	time.Sleep(s.Delay)
	data, err := s.get(ctx, s.RSSURL)
	if err != nil {
		return err
	}
	for _, it := range feed.Parse(data) {
		if it.Link == "" {
			continue
		}
		title := cleanTitle(it.Title)
		if title == "" {
			continue // папки и пустые заголовки пропускаем
		}
		id := docID(it.Link)

		marker := title + "|" + it.Link
		if it.Published != nil {
			marker += "|" + it.Published.Format(time.RFC3339)
		}
		sum := sha256.Sum256([]byte(marker))
		hash := hex.EncodeToString(sum[:])

		isNew := false
		if existing, err := s.Store.Get(ctx, id); err == nil {
			if existing.FileHash == hash {
				rep.Unchanged++
				continue
			}
			rep.Updated++
		} else {
			rep.Catalogued++
			isNew = true
		}

		status := model.StatusPending
		if u := strings.ToUpper(title); strings.Contains(u, "УТРАТИЛ") || strings.Contains(u, "УТРАТИВШ") {
			status = model.StatusOutdated // в заголовке прямо указано «УТРАТИЛИ СИЛУ»
		}

		doc := model.Document{
			ID:          id,
			Title:       title,
			SourceURL:   it.Link,
			PublishedAt: it.Published,
			FetchedAt:   time.Now(),
			Status:      status,
			FileHash:    hash,
		}
		if err := s.Store.Upsert(ctx, doc); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("каталог %s: %v", it.Link, err))
			continue
		}
		summary := "Новый документ в каталоге"
		if !isNew {
			summary = "Обновлены метаданные в RSS-каталоге"
		}
		s.recordChange(ctx, doc, isNew, summary)
	}
	return nil
}

// cleanTitle убирает префикс Telligent «File: » и возвращает пустую строку
// для записей-папок и разделов («Folder: », «Page: ») — их не каталогизируем.
func cleanTitle(title string) string {
	t := strings.TrimSpace(title)
	if strings.HasPrefix(t, "Folder:") || strings.HasPrefix(t, "Page:") {
		return ""
	}
	t = strings.TrimPrefix(t, "File:")
	return strings.TrimSpace(t)
}

// get выполняет GET с браузерным User-Agent и возвращает тело при статусе 200.
func (s *Scraper) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("статус %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Run актуализирует каталог: сначала ингест из RSS-ленты (основной источник
// списка документов), затем обход HTML на случай прямых файловых ссылок.
func (s *Scraper) Run(ctx context.Context) (*Report, error) {
	rep := &Report{StartedAt: time.Now()}

	if s.RSSURL != "" {
		if err := s.ingestRSS(ctx, rep); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("RSS-каталог %s: %v", s.RSSURL, err))
		}
	}

	base, err := url.Parse(s.BaseURL)
	if err != nil {
		return nil, err
	}

	queue := []string{s.BaseURL}
	visited := map[string]bool{}
	seenFiles := map[string]bool{}

	for len(queue) > 0 && rep.Visited < s.MaxPages {
		pageURL := queue[0]
		queue = queue[1:]
		if visited[pageURL] {
			continue
		}
		visited[pageURL] = true
		rep.Visited++

		links, err := s.fetchLinks(ctx, pageURL)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("страница %s: %v", pageURL, err))
			continue
		}

		category := categoryFromURL(pageURL)

		for _, l := range links {
			abs := resolve(base, pageURL, l.Href)
			if abs == "" {
				continue
			}
			u, err := url.Parse(abs)
			if err != nil || u.Host != base.Host {
				continue
			}
			ext := strings.ToLower(path.Ext(u.Path))
			switch {
			case fileExts[ext]:
				if seenFiles[abs] {
					continue
				}
				seenFiles[abs] = true
				if err := s.download(ctx, abs, l.Text, category, rep); err != nil {
					rep.Errors = append(rep.Errors, fmt.Sprintf("файл %s: %v", abs, err))
				}
			case isDocPage(u):
				if !visited[abs] {
					queue = append(queue, abs)
				}
			}
		}
	}
	return rep, nil
}

// download скачивает файл, считает хэш и обновляет реестр.
// Документ из раздела «утратившие силу» получает статус «устарел».
func (s *Scraper) download(ctx context.Context, fileURL, title, category string, rep *Report) error {
	time.Sleep(s.Delay)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("статус %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	id := docID(fileURL)

	// Проверка на изменения: если хэш совпал — документ не менялся.
	isNew := false
	if existing, err := s.Store.Get(ctx, id); err == nil {
		if existing.FileHash == hash {
			rep.Unchanged++
			return nil
		}
		rep.Updated++
		// Документ обновился: сохраняем снимок ПРЕДЫДУЩЕЙ редакции до перезаписи
		// файла, иначе старый текст будет потерян и дифф станет невозможен.
		s.archiveOldVersion(ctx, existing)
	} else {
		rep.Downloaded++
		isNew = true
	}

	status := model.StatusPending
	if strings.Contains(strings.ToLower(fileURL), "unactual") || category == CategoryNames["unactual_documents"] {
		status = model.StatusOutdated
	}

	statusDir := "На_проверке"
	if status == model.StatusOutdated {
		statusDir = "Архив"
	}
	name := safeFileName(fileURL)
	relDir := filepath.Join(s.OutDir, statusDir, slug(category))
	if err := os.MkdirAll(relDir, 0o755); err != nil {
		return err
	}
	localPath := filepath.Join(relDir, name)
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return err
	}

	if strings.TrimSpace(title) == "" {
		title = name
	}
	doc := model.Document{
		ID:        id,
		Title:     strings.TrimSpace(title),
		SourceURL: fileURL,
		LocalPath: localPath,
		FetchedAt: time.Now(),
		Status:    status,
		Category:  category,
		FileHash:  hash,
		Indexed:   false,
	}
	if err := s.Store.Upsert(ctx, doc); err != nil {
		return err
	}
	summary := "Скачан новый файл документа"
	if !isNew {
		summary = "Тело файла изменилось (новый хэш)"
	}
	s.recordChange(ctx, doc, isNew, summary)
	s.saveNewVersion(ctx, doc)
	return nil
}

// archiveOldVersion сохраняет снимок предыдущей редакции документа до перезаписи
// файла: копирует старый файл в Архив/versions/{id}/ и, если история версий для
// документа ещё пуста, заводит запись о старой редакции. Best-effort (ошибки логируются).
func (s *Scraper) archiveOldVersion(ctx context.Context, existing model.Document) {
	if s.Versions == nil || existing.LocalPath == "" {
		return
	}
	oldText, err := extract.Text(existing.LocalPath)
	if err != nil {
		// Текст извлечь не удалось — снимок всё равно полезен для пометки факта.
		oldText = ""
	}
	archived := s.archiveFile(existing.ID, existing.FileHash, existing.LocalPath)

	// Если истории ещё нет (документ заведён до появления версионирования),
	// фиксируем старую редакцию первой версией, чтобы диффу было с чем сравнивать.
	if n, err := s.Versions.CountVersions(ctx, existing.ID); err == nil && n == 0 {
		if _, err := s.Versions.SaveVersion(ctx, &model.DocVersion{
			DocumentID:    existing.ID,
			FileHash:      existing.FileHash,
			ExtractedText: oldText,
			ArchivedPath:  archived,
		}); err != nil {
			log.Printf("[scraper] версия (старая) %s: %v", existing.ID, err)
		}
	}
}

// saveNewVersion фиксирует снимок только что скачанной редакции документа.
func (s *Scraper) saveNewVersion(ctx context.Context, doc model.Document) {
	if s.Versions == nil || doc.LocalPath == "" {
		return
	}
	text, err := extract.Text(doc.LocalPath)
	if err != nil {
		text = ""
	}
	if _, err := s.Versions.SaveVersion(ctx, &model.DocVersion{
		DocumentID:    doc.ID,
		FileHash:      doc.FileHash,
		ExtractedText: text,
		ArchivedPath:  doc.LocalPath,
	}); err != nil {
		log.Printf("[scraper] версия (новая) %s: %v", doc.ID, err)
	}
}

// archiveFile копирует файл редакции в Архив/versions/{docID}/{hash}{ext}.
// Возвращает путь к копии или "" при неудаче.
func (s *Scraper) archiveFile(docID, hash, srcPath string) string {
	if hash == "" {
		return ""
	}
	dir := filepath.Join(s.OutDir, "Архив", "versions", safeSegment(docID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	dst := filepath.Join(dir, hash+filepath.Ext(srcPath))
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return ""
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return ""
	}
	return dst
}

// safeSegment делает строку безопасной для использования как имя папки.
func safeSegment(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_")
	return r.Replace(s)
}

type link struct {
	Href string
	Text string
}

// fetchLinks загружает страницу и возвращает все ссылки (href + текст).
func (s *Scraper) fetchLinks(ctx context.Context, pageURL string) ([]link, error) {
	time.Sleep(s.Delay)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("статус %d", resp.StatusCode)
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}
	var links []link
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			var href, text string
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = a.Val
				}
			}
			text = nodeText(n)
			if href != "" {
				links = append(links, link{Href: href, Text: text})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return links, nil
}

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

// isDocPage сообщает, относится ли страница к разделу документов (для обхода).
func isDocPage(u *url.URL) bool {
	return strings.Contains(u.Path, "/foundation/documents/")
}

func categoryFromURL(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	base := strings.TrimSuffix(path.Base(u.Path), ".aspx")
	if name, ok := CategoryNames[base]; ok {
		return name
	}
	return ""
}

func resolve(base *url.URL, pageURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "javascript:") {
		return ""
	}
	pu, err := url.Parse(pageURL)
	if err != nil {
		pu = base
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return pu.ResolveReference(ref).String()
}

func docID(fileURL string) string {
	return DocID(fileURL)
}

// DocID детерминированно генерирует ID документа из URL, предварительно
// нормализуя его. Нормализация снижает дубли одного документа, пришедшего
// из разных источников (RSS, каталог по категориям, полный обход сайта)
// под слегка различающимися URL.
func DocID(rawURL string) string {
	sum := sha1.Sum([]byte(normalizeURL(rawURL)))
	return hex.EncodeToString(sum[:])
}

// normalizeURL приводит URL к каноничному виду для дедупликации:
// схема/хост в нижний регистр, отбрасываются query, фрагмент и хвостовой «/».
// При ошибке разбора возвращает исходную строку без изменений.
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

func safeFileName(fileURL string) string {
	u, err := url.Parse(fileURL)
	if err != nil {
		return "document"
	}
	name, _ := url.QueryUnescape(path.Base(u.Path))
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." {
		return "document"
	}
	return name
}

func slug(category string) string {
	if category == "" {
		return "Без_категории"
	}
	return strings.ReplaceAll(category, " ", "_")
}
