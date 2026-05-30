// Package residents парсит реестр резидентов Сколково (каталог участников)
// через HTML-скрапинг и заводит их в хранилище. Реестр питает MCP-инструмент
// search_residents и аналитику отрасли.
//
// Структура каталога на sk.ru динамическая; парсер использует устойчивые
// эвристики (карточки/ссылки на профили резидентов). При недоступности страницы
// возвращается пустой результат — без фабрикации данных.
package residents

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// Config — конфигурация источника реестра резидентов.
type Config struct {
	SourceURL string // URL каталога резидентов
	Industry  string // отрасль по умолчанию (опционально)
}

// Result — итог синхронизации реестра резидентов.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// residentID формирует стабильный идентификатор по ссылке на профиль.
func residentID(link string) string {
	sum := sha1.Sum([]byte(link))
	return "resident-" + hex.EncodeToString(sum[:])[:16]
}

// ParseResidents загружает каталог резидентов и извлекает записи.
func ParseResidents(ctx context.Context, cfg Config, hc *http.Client) ([]*model.Resident, error) {
	if cfg.SourceURL == "" {
		return nil, fmt.Errorf("не указан SourceURL каталога резидентов")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.SourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("каталог резидентов: статус %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseResidentsHTML(cfg.SourceURL, cfg.Industry, body)
}

// parseResidentsHTML извлекает резидентов из HTML каталога: ссылки на профили
// участников с непустым названием организации.
func parseResidentsHTML(pageURL, industry string, body []byte) ([]*model.Resident, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(pageURL)

	now := time.Now()
	seen := map[string]bool{}
	var residents []*model.Resident

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attrVal(n, "href")
			name := nodeText(n)
			if href != "" && name != "" {
				abs := resolveURL(base, href)
				if isResidentURL(abs) && !seen[abs] {
					seen[abs] = true
					residents = append(residents, &model.Resident{
						ID:        residentID(abs),
						Name:      cleanName(name),
						Industry:  industry,
						Status:    model.ResidentActive,
						SourceURL: abs,
						CreatedAt: now,
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return residents, nil
}

// isResidentURL проверяет, что ссылка ведёт на профиль резидента/участника.
func isResidentURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lower := strings.ToLower(u.Path)
	for _, marker := range []string{"/resident", "/residents/", "/participant", "/uchastnik", "/members/", "/company/"} {
		if strings.Contains(lower, marker) {
			// Исключаем сам каталог (короткий путь без идентификатора).
			return len(strings.Trim(lower, "/")) > len(strings.Trim(marker, "/"))+1
		}
	}
	return false
}

// IngestResidents записывает резидентов в хранилище, фиксируя изменения в ленте.
func IngestResidents(ctx context.Context, list []*model.Resident, st store.ResidentStore, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(list)}
	for _, r := range list {
		if strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.SourceURL) == "" {
			res.Errors = append(res.Errors, "пропущено: пустое имя или URL")
			continue
		}

		isNew := false
		if existing, err := st.GetResident(ctx, r.ID); err == nil && existing != nil {
			if err := st.UpdateResident(ctx, r); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", r.ID, err))
				continue
			}
			res.Updated++
		} else {
			if err := st.CreateResident(ctx, r); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", r.ID, err))
				continue
			}
			res.New++
			isNew = true
		}

		kind := changes.KindUpdated
		if isNew {
			kind = changes.KindNew
		}
		changes.Notify(ctx, recs, changes.Event{
			EntityType: "resident",
			EntityID:   r.ID,
			Title:      r.Name,
			Category:   "Резиденты",
			Kind:       kind,
			SourceURL:  r.SourceURL,
			DetectedAt: time.Now(),
		})
	}
	return res, nil
}

// ---- HTML-утилиты ----

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func resolveURL(base *url.URL, href string) string {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	if base == nil {
		return ref.String()
	}
	return base.ResolveReference(ref).String()
}

// cleanName нормализует название организации (обрезает длину).
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	const max = 200
	if len(s) > max {
		s = s[:max]
	}
	return s
}
