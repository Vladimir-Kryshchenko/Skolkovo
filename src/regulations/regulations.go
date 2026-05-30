// Package regulations парсит нормативно-правовые акты (НПА) через HTML-скрапинг
// и заводит их в хранилище как категорию «НПА».
package regulations

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

// RegulationsConfig — конфигурация источника НПА.
type RegulationsConfig struct {
	SourceURL string // URL страницы НПА
	Category  string // категория (по умолчанию «НПА»)
}

// Monitor загружает НПА и синхронизирует их в хранилище.
type Monitor struct {
	Cfg   RegulationsConfig
	Store store.Store
	HTTP  *http.Client
}

// New создаёт монитор НПА.
func New(cfg RegulationsConfig, st store.Store) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "НПА"
	}
	return &Monitor{
		Cfg:   RegulationsConfig{SourceURL: cfg.SourceURL, Category: category},
		Store: st,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации НПА.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// Run запускает синхронизацию НПА.
func (m *Monitor) Run(ctx context.Context, recs ...changes.Recorder) (*Result, error) {
	docs, err := ParseRegulations(ctx, m.Cfg, m.HTTP)
	if err != nil {
		return nil, fmt.Errorf("парсинг НПА: %w", err)
	}
	return IngestRegulations(ctx, docs, m.Store, recs...)
}

// ParseRegulations — основная функция: парсит страницу НПА через HTML-скрапинг.
// Возвращает []*model.NPADocument.
func ParseRegulations(ctx context.Context, cfg RegulationsConfig, hc *http.Client) ([]*model.NPADocument, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	if cfg.SourceURL == "" {
		return nil, fmt.Errorf("не указан SourceURL в конфигурации НПА")
	}

	return parseRegulationsFromHTML(ctx, cfg.SourceURL, hc)
}

// parseRegulationsFromHTML загружает страницу НПА и извлекает карточки документов.
func parseRegulationsFromHTML(ctx context.Context, sourceURL string, hc *http.Client) ([]*model.NPADocument, error) {
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
		return nil, fmt.Errorf("HTML НПА: статус %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var regulations []*model.NPADocument

	// Ищем карточки НПА по распространённым паттернам.
	cards := findRegulationCards(doc)
	if len(cards) == 0 {
		// Fallback: ищем любые ссылки, похожие на НПА.
		cards = findRegulationLinks(doc, sourceURL)
	}

	for _, card := range cards {
		if card.Title == "" || card.URL == "" {
			continue
		}

		regulations = append(regulations, &model.NPADocument{
			ID:          regulationID(card.URL),
			Title:       strings.TrimSpace(card.Title),
			NPANumber:   strings.TrimSpace(card.Number),
			NPAType:     card.NPAType,
			IssuedBy:    strings.TrimSpace(card.IssuedBy),
			IssuedAt:    card.IssuedAt,
			EffectiveAt: card.EffectiveAt,
			SourceURL:   card.URL,
			Summary:     strings.TrimSpace(card.Summary),
			Status:      card.Status,
			FetchedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
	}

	return regulations, nil
}

// regulationCard — промежуточная структура для распарсенной карточки НПА.
type regulationCard struct {
	Title       string
	Number      string
	NPAType     model.NPAType
	IssuedBy    string
	IssuedAt    time.Time
	EffectiveAt time.Time
	Summary     string
	Status      model.NPAStatus
	URL         string
}

// findRegulationCards ищет карточки НПА по характерным CSS-классам/структурам.
func findRegulationCards(doc *html.Node) []regulationCard {
	var cards []regulationCard

	// Стратегия 1: ищем элементы с классами, содержащими "regulation", "npa", "law", "normative", "акт", "закон".
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := attrVal(n, "class")
			lower := strings.ToLower(class)
			if strings.Contains(lower, "regulation") ||
				strings.Contains(lower, "npa") ||
				strings.Contains(lower, "law") ||
				strings.Contains(lower, "normative") ||
				strings.Contains(lower, "card") {
				if card := extractRegulationCard(n); card.Title != "" && card.URL != "" {
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
				if card := extractRegulationCard(n); card.Title != "" && card.URL != "" {
					cards = append(cards, card)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	// Стратегия 3: ищем <tr> элементы внутри таблиц НПА.
	if len(cards) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "table" {
				class := attrVal(n, "class")
				lower := strings.ToLower(class)
				if strings.Contains(lower, "regulation") ||
					strings.Contains(lower, "npa") ||
					strings.Contains(lower, "law") ||
					strings.Contains(lower, "table") {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "tbody" {
							for r := c.FirstChild; r != nil; r = r.NextSibling {
								if r.Type == html.ElementNode && r.Data == "tr" {
									if card := extractRegulationRow(r); card.Title != "" && card.URL != "" {
										cards = append(cards, card)
									}
								}
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

// findRegulationLinks — fallback: ищет ссылки, похожие на НПА.
func findRegulationLinks(doc *html.Node, baseURL string) []regulationCard {
	var cards []regulationCard
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
			// Проверяем, что ссылка ведёт на страницу НПА.
			if !isRegulationURL(abs) {
				goto recurse
			}

			title := nodeText(n)
			if title == "" {
				goto recurse
			}

			card := regulationCard{
				Title: title,
				URL:   abs,
			}
			card.NPAType = detectNPAType(title)
			card.Number = extractNPANumber(title)

			cards = append(cards, card)
		}
	recurse:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return cards
}

// isRegulationURL проверяет, что URL похож на страницу НПА.
func isRegulationURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lower := strings.ToLower(u.Path)
	return strings.Contains(lower, "regulation") ||
		strings.Contains(lower, "law") ||
		strings.Contains(lower, "npa") ||
		strings.Contains(lower, "normative") ||
		strings.Contains(lower, "document") ||
		strings.Contains(lower, "act") ||
		strings.Contains(lower, "zakon") ||
		strings.Contains(lower, "postanovlenie") ||
		strings.Contains(lower, "prikaz")
}

// extractRegulationCard извлекает данные НПА из DOM-узла.
func extractRegulationCard(n *html.Node) regulationCard {
	var card regulationCard

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
					strings.Contains(lower, "law") ||
					strings.Contains(lower, "regulation") {
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

	// Определяем тип НПА.
	card.NPAType = detectNPAType(card.Title)

	// Извлекаем номер НПА.
	card.Number = extractNPANumber(nodeText(n))

	// Ищем издающий орган.
	card.IssuedBy = extractField(n, "issuer")

	// Ищем даты.
	if card.IssuedAt.IsZero() {
		card.IssuedAt = findIssuedDate(n)
	}
	if card.EffectiveAt.IsZero() {
		card.EffectiveAt = findEffectiveDate(n)
	}

	// Определяем статус.
	card.Status = detectNPAStatus(nodeText(n))

	// Ищем краткое содержание.
	card.Summary = extractRegulationSummary(n, card.Title)

	return card
}

// extractRegulationRow извлекает данные НПА из строки таблицы.
func extractRegulationRow(n *html.Node) regulationCard {
	var card regulationCard

	// Ищем ссылку в ячейке.
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			if card.Title == "" {
				card.Title = nodeText(node)
			}
			if card.URL == "" {
				href := attrVal(node, "href")
				if href != "" && !strings.HasPrefix(href, "#") {
					card.URL = href
				}
			}
		}
		if node.Type == html.ElementNode && node.Data == "td" {
			text := nodeText(node)
			if card.NPAType == "" {
				card.NPAType = detectNPAType(text)
			}
			if card.Number == "" {
				if num := extractNPANumber(text); num != "" {
					card.Number = num
				}
			}
			if card.IssuedBy == "" && (strings.Contains(text, "мин") ||
				strings.Contains(text, "правительств") ||
				strings.Contains(text, "президент") ||
				strings.Contains(text, "государствен")) {
				card.IssuedBy = text
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return card
}

// detectNPAType определяет тип НПА по ключевым словам.
func detectNPAType(s string) model.NPAType {
	lower := strings.ToLower(s)

	if strings.Contains(lower, "федеральный закон") || strings.Contains(lower, "фз") {
		return model.NPATypeLaw
	}
	if strings.Contains(lower, "постановление") || strings.Contains(lower, "decree") {
		return model.NPATypeDecree
	}
	if strings.Contains(lower, "приказ") || strings.Contains(lower, "order") {
		return model.NPATypeOrder
	}
	if strings.Contains(lower, "решение") || strings.Contains(lower, "decision") {
		return model.NPATypeDecision
	}

	return model.NPATypeLaw // по умолчанию
}

// extractNPANumber извлекает номер НПА из текста (например "244-ФЗ", "123-П").
func extractNPANumber(s string) string {
	// Ищем паттерн: цифры-буквы (например "244-ФЗ", "1574").
	runes := []rune(s)
	n := len(runes)
	for i := 0; i < n; i++ {
		r := runes[i]
		if r >= '0' && r <= '9' {
			// Нашли начало числа — собираем номер.
			start := i
			for i < n {
				cr := runes[i]
				if (cr >= '0' && cr <= '9') || cr == '-' || cr == ' ' || cr == '\u2116' {
					i++
					continue
				}
				// Буквы (кириллица или латиница).
				if (cr >= 'A' && cr <= 'Z') || (cr >= 'a' && cr <= 'z') ||
					(cr >= '\u0410' && cr <= '\u044F') {
					i++
					continue
				}
				break
			}
			num := strings.TrimSpace(string(runes[start:i]))
			if num != "" && len(num) >= 2 {
				return num
			}
		}
	}
	return ""
}

// detectNPAStatus определяет статус НПА по ключевым словам.
func detectNPAStatus(s string) model.NPAStatus {
	lower := strings.ToLower(s)

	if strings.Contains(lower, "утратил силу") || strings.Contains(lower, "отмен") ||
		strings.Contains(lower, "revoked") {
		return model.NPAStatusRevoked
	}
	if strings.Contains(lower, "измен") || strings.Contains(lower, "ред") ||
		strings.Contains(lower, "amend") {
		return model.NPAStatusAmended
	}

	return model.NPAStatusActive
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

// findIssuedDate ищет дату принятия НПА.
func findIssuedDate(n *html.Node) time.Time {
	var walk func(*html.Node)
	var dateStr string
	walk = func(node *html.Node) {
		if dateStr != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "issued") || strings.Contains(class, "date") ||
				strings.Contains(class, "принят") || strings.Contains(class, "от ") {
				dateStr = nodeText(node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if dateStr == "" {
		dateStr = findDateInText(nodeText(n))
	}

	return parseDate(dateStr)
}

// findEffectiveDate ищет дату вступления в силу.
func findEffectiveDate(n *html.Node) time.Time {
	var walk func(*html.Node)
	var dateStr string
	walk = func(node *html.Node) {
		if dateStr != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "effective") || strings.Contains(class, "вступ") ||
				strings.Contains(class, "действ") {
				dateStr = nodeText(node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if dateStr == "" {
		// Ищем вторую дату в тексте.
		dateStr = findSecondDateInText(nodeText(n))
	}

	return parseDate(dateStr)
}

// extractRegulationSummary извлекает краткое содержание, исключая заголовок.
func extractRegulationSummary(n *html.Node, title string) string {
	text := nodeText(n)
	text = strings.TrimSpace(text)
	// Убираем заголовок из текста.
	if title != "" && strings.HasPrefix(text, title) {
		text = strings.TrimPrefix(text, title)
		text = strings.TrimSpace(text)
	}
	// Ограничиваем длину.
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

// isDigit проверяет, что байт — цифра.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// findDateInText ищет первую строку, похожую на дату.
func findDateInText(s string) string {
	for i := 0; i <= len(s)-10; i++ {
		if isDigit(s[i]) && isDigit(s[i+1]) &&
			(s[i+2] == '.' || s[i+2] == '/') &&
			isDigit(s[i+3]) && isDigit(s[i+4]) &&
			(s[i+5] == '.' || s[i+5] == '/') &&
			isDigit(s[i+6]) && isDigit(s[i+7]) &&
			isDigit(s[i+8]) && isDigit(s[i+9]) {
			return s[i : i+10]
		}
	}
	return ""
}

// findSecondDateInText ищет вторую строку, похожую на дату.
func findSecondDateInText(s string) string {
	found := 0
	for i := 0; i <= len(s)-10; i++ {
		if isDigit(s[i]) && isDigit(s[i+1]) &&
			(s[i+2] == '.' || s[i+2] == '/') &&
			isDigit(s[i+3]) && isDigit(s[i+4]) &&
			(s[i+5] == '.' || s[i+5] == '/') &&
			isDigit(s[i+6]) && isDigit(s[i+7]) &&
			isDigit(s[i+8]) && isDigit(s[i+9]) {
			found++
			if found == 2 {
				return s[i : i+10]
			}
		}
	}
	return ""
}

// parseDate разбирает строку в дату в различных форматах.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}

	formats := []string{
		"02.01.2006",
		"02/01/2006",
		"2006-01-02",
		"02 января 2006",
		"2 января 2006",
		"02.01.2006 15:04",
		"02/01/2006 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		time.RFC1123,
		time.RFC1123Z,
		time.RFC3339,
	}

	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}

	return time.Time{}
}

// IngestRegulations записывает НПА в хранилище.
func IngestRegulations(ctx context.Context, npaList []*model.NPADocument, st store.Store, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(npaList)}

	for _, n := range npaList {
		if n.Title == "" || n.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}

		// Преобразуем NPADocument → Document для хранения в generic Store.
		doc := npaToDocument(n)

		isNew := false
		if _, err := st.Get(ctx, doc.ID); err == nil {
			// Обновляем существующий НПА.
			if err := st.Upsert(ctx, doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", doc.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём новый.
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
			EntityType: changes.EntityNPA,
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

// npaToDocument преобразуем NPADocument → Document для хранения.
func npaToDocument(n *model.NPADocument) model.Document {
	status := model.StatusActive
	if n.Status == model.NPAStatusRevoked {
		status = model.StatusOutdated
	} else if n.Status == model.NPAStatusAmended {
		status = model.StatusActive // amended всё ещё действует
	}
	return model.Document{
		ID:        n.ID,
		Title:     n.Title,
		SourceURL: n.SourceURL,
		FetchedAt: n.FetchedAt,
		Status:    status,
		Category:  "НПА",
		FileHash:  contentHashReg(n.Summary),
	}
}

// contentHashReg вычисляет хэш содержимого.
func contentHashReg(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func regulationID(regulationURL string) string {
	sum := sha1.Sum([]byte(regulationURL))
	return "npa-" + hex.EncodeToString(sum[:])[:16]
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
