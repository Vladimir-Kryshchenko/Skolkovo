// Package preferences парсит льготы (преференции) Сколково через HTML-скрапинг
// и заводит их в хранилище как категорию «Льготы».
package preferences

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// userAgent — браузерный UA для страниц, отдающих контент только браузерам.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// PreferencesConfig — конфигурация источника льгот.
type PreferencesConfig struct {
	SourceURL string // URL страницы льгот (dochub.sk.ru/preferences и т.п.)
	Category  string // категория (по умолчанию «Льготы»)
}

// Monitor загружает льготы и синхронизирует их в хранилище.
type Monitor struct {
	Cfg   PreferencesConfig
	Store store.Store
	HTTP  *http.Client
}

// New создаёт монитор льгот.
func New(cfg PreferencesConfig, st store.Store) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "Льготы"
	}
	return &Monitor{
		Cfg:   PreferencesConfig{SourceURL: cfg.SourceURL, Category: category},
		Store: st,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации льгот.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// Run загружает льготы и сохраняет их в хранилище.
func (m *Monitor) Run(ctx context.Context, recs ...changes.Recorder) (*Result, error) {
	docs, err := ParsePreferences(ctx, m.Cfg, m.HTTP)
	if err != nil {
		return nil, fmt.Errorf("парсинг льгот: %w", err)
	}
	return IngestPreferences(ctx, docs, m.Store, recs...)
}

// ParsePreferences — основная функция: парсит страницу льгот через HTML-скрапинг.
// Возвращает []*model.Preference.
func ParsePreferences(ctx context.Context, cfg PreferencesConfig, hc *http.Client) ([]*model.Preference, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	if cfg.SourceURL == "" {
		return nil, fmt.Errorf("не указан SourceURL в конфигурации льгот")
	}

	return parsePreferencesFromHTML(ctx, cfg.SourceURL, hc)
}

// parsePreferencesFromHTML загружает страницу льгот и извлекает карточки льгот.
func parsePreferencesFromHTML(ctx context.Context, sourceURL string, hc *http.Client) ([]*model.Preference, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTML льготы: статус %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var preferences []*model.Preference

	// Ищем карточки льгот по распространённым паттернам.
	cards := findPreferenceCards(doc)
	if len(cards) == 0 {
		// Fallback: ищем любые ссылки, похожие на льготы.
		cards = findPreferenceLinks(doc, sourceURL)
	}

	for _, card := range cards {
		if card.Title == "" || card.URL == "" {
			continue
		}

		preferences = append(preferences, &model.Preference{
			ID:          preferenceID(card.URL),
			Title:       strings.TrimSpace(card.Title),
			PrefType:    card.PrefType,
			BenefitDesc: strings.TrimSpace(card.Description),
			SourceURL:   card.URL,
			Content:     strings.TrimSpace(card.Content),
			Status:      model.PrefStatusActive,
			FetchedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
	}

	return preferences, nil
}

// preferenceCard — промежуточная структура для распарсенной карточки льготы.
type preferenceCard struct {
	Title       string
	Description string
	PrefType    model.PreferenceType
	Content     string
	URL         string
}

// findPreferenceCards ищет карточки льгот по характерным CSS-классам/структурам.
func findPreferenceCards(doc *html.Node) []preferenceCard {
	var cards []preferenceCard

	// Стратегия 1: ищем элементы с классами, содержащими "benefit", "preference", "lgota", "privilege".
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := attrVal(n, "class")
			lower := strings.ToLower(class)
			if strings.Contains(lower, "benefit") ||
				strings.Contains(lower, "preference") ||
				strings.Contains(lower, "lgota") ||
				strings.Contains(lower, "privilege") ||
				strings.Contains(lower, "card") {
				if card := extractPreferenceCard(n); card.Title != "" && card.URL != "" {
					cards = append(cards, card)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Стратегия 2: если ничего не нашли, ищем <article> элементы.
	if len(cards) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "article" {
				if card := extractPreferenceCard(n); card.Title != "" && card.URL != "" {
					cards = append(cards, card)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	// Стратегия 3: ищем <li> элементы внутри списков льгот.
	if len(cards) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol") {
				class := attrVal(n, "class")
				lower := strings.ToLower(class)
				if strings.Contains(lower, "benefit") ||
					strings.Contains(lower, "preference") ||
					strings.Contains(lower, "lgota") ||
					strings.Contains(lower, "list") {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "li" {
							if card := extractPreferenceCard(c); card.Title != "" && card.URL != "" {
								cards = append(cards, card)
							}
						}
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	return cards
}

// findPreferenceLinks — fallback: ищет ссылки, похожие на льготы.
func findPreferenceLinks(doc *html.Node, baseURL string) []preferenceCard {
	var cards []preferenceCard
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attrVal(n, "href")
			if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") {
				goto recurse
			}

			abs := resolveURL(base, baseURL, href)
			// Проверяем, что ссылка ведёт на страницу льготы.
			if !isPreferenceURL(abs) {
				goto recurse
			}

			title := nodeText(n)
			if title == "" {
				goto recurse
			}

			cards = append(cards, preferenceCard{
				Title: title,
				URL:   abs,
			})
		}
	recurse:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return cards
}

// isPreferenceURL проверяет, что URL похож на страницу льготы.
func isPreferenceURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lower := strings.ToLower(u.Path)
	return strings.Contains(lower, "benefit") ||
		strings.Contains(lower, "preference") ||
		strings.Contains(lower, "lgota") ||
		strings.Contains(lower, "privilege") ||
		strings.Contains(lower, "lgoty") ||
		strings.Contains(lower, "preferencii")
}

// extractPreferenceCard извлекает данные льготы из DOM-узла.
func extractPreferenceCard(n *html.Node) preferenceCard {
	var card preferenceCard

	// Ищем заголовок (h1-h4, или .title, или первую ссылку).
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if card.Title != "" {
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "h1", "h2", "h3", "h4":
				card.Title = nodeText(node)
			case "a":
				class := attrVal(node, "class")
				lower := strings.ToLower(class)
				if strings.Contains(lower, "title") ||
					strings.Contains(lower, "name") ||
					strings.Contains(lower, "benefit") ||
					strings.Contains(lower, "preference") {
					card.Title = nodeText(node)
					card.URL = attrVal(node, "href")
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	// Если заголовок не найден через walk, берём весь текст узла.
	if card.Title == "" {
		card.Title = nodeText(n)
	}

	// Ищем ссылку, если ещё не нашли.
	if card.URL == "" {
		card.URL = findLink(n)
	}

	// Определяем тип льготы.
	card.PrefType = detectPreferenceType(n)

	// Ищем описание льготы.
	card.Description = extractPreferenceDescription(n, card.Title)

	// Извлекаем дополнительный контент.
	card.Content = extractField(n, "content")

	return card
}

// detectPreferenceType определяет тип льготы по ключевым словам в тексте.
func detectPreferenceType(n *html.Node) model.PreferenceType {
	text := strings.ToLower(nodeText(n))

	if strings.Contains(text, "налог на прибыль") || strings.Contains(text, "profit tax") {
		return model.PrefTaxProfit
	}
	if strings.Contains(text, "страх") || strings.Contains(text, "insurance") ||
		strings.Contains(text, "взносы") {
		return model.PrefInsurance
	}
	if strings.Contains(text, "ндс") || strings.Contains(text, "vat") {
		return model.PrefVAT
	}
	if strings.Contains(text, "тамож") || strings.Contains(text, "custom") ||
		strings.Contains(text, "пошлин") {
		return model.PrefCustoms
	}

	return model.PrefOther
}

// extractField ищет текст в элементе с классом, содержащим keyword.
func extractField(n *html.Node, keyword string) string {
	var result string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if result != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, keyword) {
				result = nodeText(node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return result
}

// extractPreferenceDescription извлекает описание, исключая заголовок.
func extractPreferenceDescription(n *html.Node, title string) string {
	text := nodeText(n)
	text = strings.TrimSpace(text)
	// Убираем заголовок из описания.
	if title != "" && strings.HasPrefix(text, title) {
		text = strings.TrimPrefix(text, title)
		text = strings.TrimSpace(text)
	}
	// Ограничиваем длину описания.
	if len(text) > 2000 {
		text = text[:2000] + "..."
	}
	return text
}

// findLink ищет первую ссылку в узле.
func findLink(n *html.Node) string {
	var link string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if link != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			href := attrVal(node, "href")
			if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "mailto:") {
				link = href
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return link
}

// resolveURL разрешает относительный URL относительно базового.
func resolveURL(base *url.URL, pageURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
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

// IngestPreferences записывает льготы в хранилище.
func IngestPreferences(ctx context.Context, prefList []*model.Preference, st store.Store, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(prefList)}

	for _, p := range prefList {
		if p.Title == "" || p.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}

		// Преобразуем Preference → Document для хранения в generic Store.
		doc := preferenceToDocument(p)

		isNew := false
		if _, err := st.Get(ctx, doc.ID); err == nil {
			// Обновляем существующую льготу.
			if err := st.Upsert(ctx, doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", doc.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём новую.
			if err := st.Upsert(ctx, doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", doc.ID, err))
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
			EntityType: changes.EntityPreference,
			EntityID:   doc.ID,
			Title:      doc.Title,
			Category:   doc.Category,
			Kind:       kind,
			SourceURL:  doc.SourceURL,
			DetectedAt: time.Now(),
		})
	}

	return res, nil
}

// preferenceToDocument преобразуем Preference → Document для хранения.
func preferenceToDocument(p *model.Preference) model.Document {
	status := model.StatusActive
	if p.Status == model.PrefStatusOutdated {
		status = model.StatusOutdated
	}
	return model.Document{
		ID:        p.ID,
		Title:     p.Title,
		SourceURL: p.SourceURL,
		FetchedAt: p.FetchedAt,
		Status:    status,
		Category:  "Льготы",
		FileHash:  contentHash(p.BenefitDesc + p.Content),
	}
}

// contentHash вычисляет хэш содержимого.
func contentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func preferenceID(preferenceURL string) string {
	sum := sha1.Sum([]byte(preferenceURL))
	return "pref-" + hex.EncodeToString(sum[:])[:16]
}

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
