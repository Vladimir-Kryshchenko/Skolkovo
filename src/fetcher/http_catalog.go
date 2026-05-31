package fetcher

// HTTP-каталогизация — основной (быстрый) способ собрать список документов.
//
// Важное наблюдение (проверено эмпирически на dochub.sk.ru): страницы-листинги
// категорий `/foundation/documents/p/{slug}.aspx` отдаются ОБЫЧНЫМ HTTP-GET —
// WAF их не блокирует, и список документов уже встроен в HTML (ссылки вида
// `/foundation/documents/m/docs/{id}/download.aspx`). Headless-браузер для
// каталогизации не нужен. Тела файлов (`/m/docs/{id}/download.aspx`) при этом
// за WAF (403) — их по-прежнему берёт headless-фетчер.
//
// Этот метод — основной; headless EnumerateCategories/EnumerateSite остаются
// как fallback (см. EnumerateCategoriesAuto).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// maxPagesPerCategory ограничивает проход по пагинации одной категории.
const maxPagesPerCategory = 30

// EnumerateCategoriesHTTP собирает документы по категориям прямым HTTP-GET
// (без браузера). Возвращает все найденные ссылки на документы с заголовками.
func (f *Fetcher) EnumerateCategoriesHTTP(ctx context.Context, baseURL string, cats []CategorySpec) ([]CatalogItem, error) {
	base := strings.TrimSuffix(baseURL, "/")
	var out []CatalogItem
	seen := map[string]bool{}

	for i, c := range cats {
		pageURL := base + "/p/" + c.Slug + ".aspx"
		n := 0
		for visited := 0; pageURL != "" && visited < maxPagesPerCategory; visited++ {
			docs, next, err := f.fetchCategoryPage(ctx, pageURL)
			if err != nil {
				fmt.Printf("  [http] категория %s: %v\n", c.Slug, err)
				break
			}
			for _, d := range docs {
				if d.Link == "" || seen[d.Link] {
					continue
				}
				seen[d.Link] = true
				out = append(out, CatalogItem{Title: d.Title, Link: d.Link, Category: c.Name})
				n++
			}
			pageURL = next
			if pageURL != "" {
				time.Sleep(f.humanDelay(400, 1200))
			}
		}
		fmt.Printf("  [http] %d/%d %s: %d документов (всего: %d)\n", i+1, len(cats), c.Name, n, len(out))
		if i < len(cats)-1 {
			time.Sleep(f.humanDelay(500, 1500))
		}
	}
	return out, nil
}

// EnumerateCategoriesAuto — основной вход для каталогизации: пробует быстрый
// HTTP-парсинг, и только если он ничего не дал (0 документов или ошибка) —
// откатывается на headless-браузер (обход WAF через прокси). Так система
// продолжит работать, даже если прямой HTTP-метод однажды перестанет отдавать
// списки (изменится разметка/появится JS-рендер/WAF начнёт резать листинги).
func (f *Fetcher) EnumerateCategoriesAuto(ctx context.Context, baseURL string, cats []CategorySpec) ([]CatalogItem, error) {
	items, err := f.EnumerateCategoriesHTTP(ctx, baseURL, cats)
	if err == nil && len(items) > 0 {
		fmt.Printf("  [catalog] HTTP-метод: %d документов\n", len(items))
		return items, nil
	}
	if err != nil {
		fmt.Printf("  [catalog] HTTP-метод не сработал (%v) — откат на headless-браузер\n", err)
	} else {
		fmt.Printf("  [catalog] HTTP-метод дал 0 документов — откат на headless-браузер\n")
	}
	return f.EnumerateCategories(ctx, baseURL, cats)
}

// EnumerateSiteAuto — полный обход каталога: основной путь — быстрый HTTP-парсинг
// 8 категорий (он покрывает весь раздел документов). Если HTTP ничего не дал —
// откат на headless BFS-обход всего сайта (EnumerateSite). maxPages передаётся
// в fallback.
func (f *Fetcher) EnumerateSiteAuto(ctx context.Context, baseURL string, cats []CategorySpec, maxPages int) ([]CatalogItem, error) {
	items, err := f.EnumerateCategoriesHTTP(ctx, baseURL, cats)
	if err == nil && len(items) > 0 {
		fmt.Printf("  [crawl] HTTP-метод: %d документов\n", len(items))
		return items, nil
	}
	if err != nil {
		fmt.Printf("  [crawl] HTTP-метод не сработал (%v) — откат на headless-обход\n", err)
	} else {
		fmt.Printf("  [crawl] HTTP-метод дал 0 документов — откат на headless-обход\n")
	}
	return f.EnumerateSite(ctx, baseURL, cats, maxPages)
}

// docLink — ссылка на документ с заголовком, найденная в HTML.
type docLink struct {
	Title string
	Link  string
}

// fetchCategoryPage скачивает одну страницу категории и извлекает ссылки на
// документы и (если есть) ссылку на следующую страницу пагинации.
func (f *Fetcher) fetchCategoryPage(ctx context.Context, pageURL string) (docs []docLink, next string, err error) {
	pu, err := url.Parse(pageURL)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, "", fmt.Errorf("%w: HTTP %d", errWAFBlock, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	return parseCategoryHTML(body, pu)
}

// parseCategoryHTML извлекает ссылки на документы (/m/docs/.../download) и
// ссылку «следующая страница» из HTML страницы категории.
func parseCategoryHTML(htmlBytes []byte, base *url.URL) (docs []docLink, next string, err error) {
	root, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return nil, "", err
	}
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href, class, style := "", "", ""
			for _, a := range n.Attr {
				switch a.Key {
				case "href":
					href = a.Val
				case "class":
					class = a.Val
				case "style":
					style = a.Val
				}
			}
			if href != "" {
				abs := resolveRef(base, href)
				// Ссылка на документ.
				if isDocDownloadLink(abs) && !seen[abs] {
					seen[abs] = true
					docs = append(docs, docLink{Title: collapseSpaces(nodeText(n)), Link: abs})
				}
				// Ссылка пагинации «next» (видимая).
				if next == "" && strings.Contains(class, "next") &&
					!strings.Contains(strings.ReplaceAll(style, " ", ""), "display:none") &&
					strings.Contains(abs, "?pi") {
					next = abs
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return docs, next, nil
}

// isDocDownloadLink проверяет, что ссылка ведёт на тело документа dochub.
func isDocDownloadLink(u string) bool {
	low := strings.ToLower(u)
	return strings.Contains(low, "/m/docs/") &&
		(strings.Contains(low, "/download") || strings.HasSuffix(low, ".aspx"))
}

// resolveRef приводит href к абсолютному URL относительно base.
func resolveRef(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "javascript:") {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// nodeText собирает весь текст внутри узла.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

// collapseSpaces схлопывает пробельные последовательности и тримит строку.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
