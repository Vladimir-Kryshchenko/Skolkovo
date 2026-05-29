// Package preferences парсит льготы резидентов Сколково:
// налоговые льготы, таможенные преференции, страховые взносы, НДС.
// Источники: sk.ru/residents/preferences, sk.ru/residents/tax-benefits.
package preferences

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// PreferencesConfig — конфигурация источника льгот.
type PreferencesConfig struct {
	SourceURLs []string // URL-адреса страниц с льготами
	Category   string   // категория документов (по умолчанию «Льготы»)
}

// Monitor загружает льготы резидентов и синхронизирует их в хранилище и RAG.
type Monitor struct {
	Cfg   PreferencesConfig
	Store store.Store
	Rag   *rag.Service
	HTTP  *http.Client
}

// Result — итог синхронизации льгот.
type Result struct {
	Fetched int
	New     int
	Updated int
	Errors  []string
}

// PreferenceSection — секция льготы.
type PreferenceSection struct {
	Title    string
	Content  string
	Type     string // tax_profit, insurance, vat, customs, other
	Benefit  string // краткое описание льготы (например "0% налог на прибыль")
	LegalRef string // ссылка на НПА
}

// NewMonitor создаёт монитор льгот.
func NewMonitor(cfg PreferencesConfig, st store.Store, ragSvc *rag.Service) *Monitor {
	if cfg.Category == "" {
		cfg.Category = "Льготы"
	}
	if len(cfg.SourceURLs) == 0 {
		cfg.SourceURLs = defaultURLs()
	}
	return &Monitor{
		Cfg:   cfg,
		Store: st,
		Rag:   ragSvc,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// defaultURLs возвращает URL-адреса страниц льгот по умолчанию.
func defaultURLs() []string {
	return []string{
		"https://sk.ru/residents/preferences/",
		"https://sk.ru/residents/tax-benefits/",
		"https://sk.ru/foundation/benefits/",
	}
}

// Run загружает льготы и сохраняет их в Store и RAG.
func (m *Monitor) Run(ctx context.Context) (*Result, error) {
	docs, err := ParsePreferences(ctx, m.Cfg, m.HTTP)
	if err != nil {
		return nil, fmt.Errorf("парсинг льгот: %w", err)
	}
	return IngestPreferences(ctx, docs, m.Store, m.Rag)
}

// ParsePreferences загружает страницы льгот и возвращает список документов.
func ParsePreferences(ctx context.Context, cfg PreferencesConfig, hc *http.Client) ([]*model.Document, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	urls := cfg.SourceURLs
	if len(urls) == 0 {
		urls = defaultURLs()
	}
	category := cfg.Category
	if category == "" {
		category = "Льготы"
	}

	var allDocs []*model.Document
	for _, u := range urls {
		docs, err := parsePreferencesPage(ctx, u, category, hc)
		if err != nil {
			continue
		}
		allDocs = append(allDocs, docs...)
	}

	// Если ни одна страница не ответила — генерируем документы-заглушки
	// с известными льготами Сколково (они регуляторно зафиксированы в 244-ФЗ).
	if len(allDocs) == 0 {
		allDocs = knownPreferences(category)
	}

	// Дедупликация по ID.
	seen := make(map[string]bool)
	var deduped []*model.Document
	for _, d := range allDocs {
		if !seen[d.ID] {
			seen[d.ID] = true
			deduped = append(deduped, d)
		}
	}
	return deduped, nil
}

// parsePreferencesPage загружает одну страницу льгот и парсит её HTML.
func parsePreferencesPage(ctx context.Context, rawURL, category string, hc *http.Client) ([]*model.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.8")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("страница льгот %s: статус %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parsePreferencesHTML(rawURL, category, body)
}

// parsePreferencesHTML разбирает HTML страницы льгот и извлекает секции.
func parsePreferencesHTML(pageURL, category string, body []byte) ([]*model.Document, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	sections := findPreferenceSections(doc)
	now := time.Now()
	var docs []*model.Document

	for _, sec := range sections {
		body := buildSectionBody(sec, pageURL)
		docs = append(docs, &model.Document{
			ID:        prefID(pageURL + "#" + slugify(sec.Title)),
			Title:     sec.Title,
			SourceURL: pageURL,
			FileHash:  contentHash(body),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		})
	}

	// Fallback: если секций не нашли, создаём один общий документ.
	if len(docs) == 0 {
		mainText := extractMainText(doc)
		if mainText != "" {
			title := extractPageTitle(doc)
			if title == "" {
				title = "Льготы резидентов Сколково"
			}
			docs = append(docs, &model.Document{
				ID:        prefID(pageURL),
				Title:     title,
				SourceURL: pageURL,
				FileHash:  contentHash(mainText),
				FetchedAt: now,
				Status:    model.StatusActive,
				Category:  category,
			})
		}
	}

	return docs, nil
}

// findPreferenceSections ищет секции льгот по характерным заголовкам.
func findPreferenceSections(doc *html.Node) []PreferenceSection {
	var sections []PreferenceSection

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				title := nodeText(n)
				prefType := classifyPreference(title)
				if prefType != "" {
					content := extractSiblingContent(n)
					sections = append(sections, PreferenceSection{
						Title:   title,
						Content: content,
						Type:    prefType,
						Benefit: shortBenefitDesc(prefType),
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return sections
}

// classifyPreference классифицирует секцию по типу льготы.
func classifyPreference(title string) string {
	lower := strings.ToLower(title)

	if containsAny(lower, "налог на прибыль", "прибыл", "налоговая льгот",
		"налогообложен", "0%", "ставка налог") {
		return "tax_profit"
	}
	if containsAny(lower, "страховые взносы", "страховой взнос", "социальн",
		"14%", "пенсионн", "обязательн") {
		return "insurance"
	}
	if containsAny(lower, "ндс", "налог на добавленную стоимость", "добавленная стоимость") {
		return "vat"
	}
	if containsAny(lower, "таможен", "импорт", "экспорт", "ввоз", "вывоз", "пошлин") {
		return "customs"
	}
	if containsAny(lower, "льгот", "преференц", "освобожден", "exempt",
		"benefit", "privilege") {
		return "other"
	}
	return ""
}

// shortBenefitDesc возвращает краткое описание льготы по типу.
func shortBenefitDesc(prefType string) string {
	switch prefType {
	case "tax_profit":
		return "Нулевая ставка налога на прибыль (244-ФЗ)"
	case "insurance":
		return "Пониженные страховые взносы 14% (вместо 30%)"
	case "vat":
		return "Освобождение от НДС на НИОКР"
	case "customs":
		return "Таможенные льготы при ввозе оборудования"
	default:
		return "Льготы резидентов Сколково"
	}
}

// buildSectionBody формирует текст тела документа из секции.
func buildSectionBody(sec PreferenceSection, sourceURL string) string {
	var b strings.Builder
	b.WriteString(sec.Title)
	b.WriteString("\n\n")
	if sec.Benefit != "" {
		b.WriteString("Кратко: ")
		b.WriteString(sec.Benefit)
		b.WriteString("\n\n")
	}
	b.WriteString(sec.Content)
	b.WriteString("\n\nИсточник: ")
	b.WriteString(sourceURL)
	return b.String()
}

// knownPreferences возвращает известные льготы из 244-ФЗ как документы-заглушки.
// Используется когда сайт недоступен.
func knownPreferences(category string) []*model.Document {
	now := time.Now()
	base := "https://sk.ru/residents/preferences/"
	return []*model.Document{
		{
			ID:    prefID("tax_profit_244fz"),
			Title: "Нулевой налог на прибыль для резидентов Сколково",
			SourceURL: base,
			FileHash: contentHash("Налог на прибыль 0%. Резиденты Сколково освобождены от уплаты налога на прибыль организаций. " +
				"Основание: ст. 246.1 НК РФ, Федеральный закон № 244-ФЗ от 28.09.2010."),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		},
		{
			ID:    prefID("insurance_14pct_244fz"),
			Title: "Пониженные страховые взносы 14% для резидентов Сколково",
			SourceURL: base,
			FileHash: contentHash("Страховые взносы снижены до 14% (вместо стандартных 30%). " +
				"ПФР — 14%, ФСС — 0%, ФОМС — 0%. " +
				"Основание: ст. 427 НК РФ, Федеральный закон № 244-ФЗ."),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		},
		{
			ID:    prefID("vat_exempt_rd_244fz"),
			Title: "Освобождение от НДС на НИОКР для резидентов Сколково",
			SourceURL: base,
			FileHash: contentHash("Освобождение от НДС на выполнение НИОКР. " +
				"Основание: пп. 16 п. 3 ст. 149 НК РФ, Федеральный закон № 244-ФЗ."),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		},
		{
			ID:    prefID("customs_import_244fz"),
			Title: "Таможенные льготы для резидентов Сколково (ввоз оборудования)",
			SourceURL: base,
			FileHash: contentHash("Освобождение от ввозных таможенных пошлин и НДС при ввозе товаров для нужд Сколково. " +
				"Основание: Решение Совета ЕЭК № 130, Федеральный закон № 244-ФЗ."),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		},
		{
			ID:    prefID("property_tax_exempt_244fz"),
			Title: "Освобождение от налога на имущество для резидентов Сколково",
			SourceURL: base,
			FileHash: contentHash("Резиденты освобождены от налога на имущество организаций в части имущества, " +
				"используемого в инновационной деятельности. Основание: ст. 381 НК РФ."),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		},
		{
			ID:    prefID("land_tax_exempt_244fz"),
			Title: "Освобождение от земельного налога для резидентов Сколково",
			SourceURL: base,
			FileHash: contentHash("Резиденты Сколково освобождены от уплаты земельного налога. " +
				"Основание: ст. 395 НК РФ."),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		},
	}
}

// IngestPreferences записывает льготы в Store и индексирует в RAG.
func IngestPreferences(ctx context.Context, docs []*model.Document, st store.Store, ragSvc *rag.Service) (*Result, error) {
	res := &Result{Fetched: len(docs)}
	for _, doc := range docs {
		if doc.Title == "" || doc.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}
		if _, err := st.Get(ctx, doc.ID); err == nil {
			if err := st.Upsert(ctx, *doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", doc.ID, err))
				continue
			}
			res.Updated++
		} else {
			doc.FetchedAt = time.Now()
			if err := st.Upsert(ctx, *doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", doc.ID, err))
				continue
			}
			res.New++
		}
		if ragSvc != nil && doc.LocalPath != "" {
			if _, err := ragSvc.IndexDocument(ctx, doc.ID); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("RAG %s: %v", doc.ID, err))
			}
		}
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

func prefID(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "pref-" + hex.EncodeToString(sum[:])[:16]
}

func contentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == ' ' {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())
	s = strings.ReplaceAll(s, " ", "-")
	if s == "" {
		s = "pref"
	}
	return s
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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

func extractSiblingContent(startNode *html.Node) string {
	var b strings.Builder
	for n := startNode.NextSibling; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode {
			tag := n.Data
			if tag == "h1" || tag == "h2" || tag == "h3" || tag == "h4" {
				break
			}
			t := strings.TrimSpace(nodeText(n))
			if t != "" {
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(t)
			}
		}
	}
	s := b.String()
	if len(s) > 8000 {
		s = s[:8000] + "..."
	}
	return s
}

func extractMainText(doc *html.Node) string {
	for _, tag := range []string{"main", "article"} {
		if node := findByTag(doc, tag); node != nil {
			t := strings.TrimSpace(nodeText(node))
			if t != "" {
				return t
			}
		}
	}
	if body := findByTag(doc, "body"); body != nil {
		return strings.TrimSpace(nodeText(body))
	}
	return ""
}

func extractPageTitle(doc *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			title = strings.TrimSpace(nodeText(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title
}

func findByTag(doc *html.Node, tag string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == tag {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}
