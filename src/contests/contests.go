// Package contests парсит конкурсы и гранты Сколково через HTML-скрапинг
// и заводит их в хранилище и RAG как категорию «Конкурсы».
package contests

import (
	"context"
	"crypto/sha1"
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
	rag "baza-skolkovo/src/rag_service"
)

// userAgent — браузерный UA для страниц, отдающих контент только браузерам.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// ContestsConfig — конфигурация источника конкурсов и грантов.
type ContestsConfig struct {
	ContestsURL string // URL страницы конкурсов (sk.ru/contests)
	GrantsURL   string // URL страницы грантов (sk.ru/grants)
	Category    string // категория (по умолчанию «Конкурсы»)
}

// Monitor загружает конкурсы/гранты и синхронизирует их в хранилище и RAG.
type Monitor struct {
	Cfg   ContestsConfig
	Store store.ContestStore
	Rag   *rag.Service // может быть nil — тогда без индексации
	HTTP  *http.Client
}

// New создаёт монитор конкурсов/грантов.
func New(cfg ContestsConfig, st store.ContestStore, ragSvc *rag.Service) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "Конкурсы"
	}
	return &Monitor{
		Cfg:   ContestsConfig{ContestsURL: cfg.ContestsURL, GrantsURL: cfg.GrantsURL, Category: category},
		Store: st,
		Rag:   ragSvc,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации конкурсов/грантов.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// ParseContests — основная функция: парсит страницы конкурсов и грантов.
// Возвращает []*model.Contest.
func ParseContests(ctx context.Context, cfg ContestsConfig, hc *http.Client) ([]*model.Contest, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	var allContests []*model.Contest

	// Парсим страницу конкурсов.
	if cfg.ContestsURL != "" {
		contests, err := parseContestsFromPage(ctx, cfg.ContestsURL, hc)
		if err != nil {
			// Не критичная ошибка — пробуем гранты.
			contests = nil
		}
		allContests = append(allContests, contests...)
	}

	// Парсим страницу грантов.
	if cfg.GrantsURL != "" {
		grants, err := parseContestsFromPage(ctx, cfg.GrantsURL, hc)
		if err != nil {
			grants = nil
		}
		allContests = append(allContests, grants...)
	}

	if len(allContests) == 0 && cfg.ContestsURL == "" && cfg.GrantsURL == "" {
		return nil, fmt.Errorf("не указан ни ContestsURL, ни GrantsURL в конфигурации конкурсов")
	}

	return allContests, nil
}

// parseContestsFromPage загружает страницу и извлекает карточки конкурсов/грантов.
func parseContestsFromPage(ctx context.Context, pageURL string, hc *http.Client) ([]*model.Contest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
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
		return nil, fmt.Errorf("HTML конкурсы/гранты: статус %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var contests []*model.Contest
	now := time.Now()

	// Ищем карточки конкурсов по распространённым паттернам.
	cards := findContestCards(doc)
	if len(cards) == 0 {
		// Fallback: ищем любые ссылки, похожие на конкурсы/гранты.
		cards = findContestLinks(doc, pageURL)
	}

	for _, card := range cards {
		if card.Title == "" || card.URL == "" {
			continue
		}

		status := model.ContestActive
		if !card.EndDate.IsZero() && card.EndDate.Before(now) {
			status = model.ContestClosed
		}

		contests = append(contests, &model.Contest{
			ID:           contestID(card.URL),
			Title:        strings.TrimSpace(card.Title),
			Description:  strings.TrimSpace(card.Description),
			StartDate:    card.StartDate,
			EndDate:      card.EndDate,
			Requirements: strings.TrimSpace(card.Requirements),
			Prize:        strings.TrimSpace(card.Prize),
			SourceURL:    card.URL,
			Status:       status,
			Category:     "Конкурсы",
			CreatedAt:    time.Now(),
		})
	}

	return contests, nil
}

// contestCard — промежуточная структура для распарсенной карточки конкурса.
type contestCard struct {
	Title        string
	Description  string
	StartDate    time.Time
	EndDate      time.Time
	Requirements string
	Prize        string
	URL          string
}

// findContestCards ищет карточки конкурсов по характерным CSS-классам/структурам.
func findContestCards(doc *html.Node) []contestCard {
	var cards []contestCard

	// Стратегия 1: ищем элементы с классами, содержащими "contest", "grant", "card", "item".
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := attrVal(n, "class")
			lower := strings.ToLower(class)
			if strings.Contains(lower, "contest") ||
				strings.Contains(lower, "grant") ||
				strings.Contains(lower, "card") ||
				strings.Contains(lower, "item") {
				if card := extractContestCard(n); card.Title != "" && card.URL != "" {
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
				if card := extractContestCard(n); card.Title != "" && card.URL != "" {
					cards = append(cards, card)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	// Стратегия 3: ищем <li> элементы внутри списков конкурсов.
	if len(cards) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol") {
				class := attrVal(n, "class")
				lower := strings.ToLower(class)
				if strings.Contains(lower, "contest") ||
					strings.Contains(lower, "grant") ||
					strings.Contains(lower, "list") {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "li" {
							if card := extractContestCard(c); card.Title != "" && card.URL != "" {
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

// findContestLinks — fallback: ищет ссылки, похожие на конкурсы/гранты.
func findContestLinks(doc *html.Node, baseURL string) []contestCard {
	var cards []contestCard
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
			// Проверяем, что ссылка ведёт на страницу конкурса/гранта.
			if !isContestURL(abs) {
				goto recurse
			}

			title := nodeText(n)
			if title == "" {
				goto recurse
			}

			card := contestCard{
				Title: title,
				URL:   abs,
			}
			card.StartDate = extractDateFromParent(n)
			card.EndDate = extractEndDateFromParent(n)

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

// isContestURL проверяет, что URL похож на страницу конкурса/гранта.
func isContestURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lower := strings.ToLower(u.Path)
	return strings.Contains(lower, "contest") ||
		strings.Contains(lower, "grant") ||
		strings.Contains(lower, "konkurs") ||
		strings.Contains(lower, "granty")
}

// extractContestCard извлекает данные конкурса из DOM-узла.
func extractContestCard(n *html.Node) contestCard {
	var card contestCard

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
					strings.Contains(lower, "contest") {
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

	// Ищем ссылки и даты.
	if card.URL == "" {
		card.URL = findLink(n)
	}
	if card.StartDate.IsZero() {
		card.StartDate = findStartDate(n)
	}
	if card.EndDate.IsZero() {
		card.EndDate = findEndDate(n)
	}

	// Ищем требования, призы и описание.
	card.Requirements = extractField(n, "requirement")
	card.Prize = extractField(n, "prize")
	card.Description = extractContestDescription(n, card.Title)

	return card
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

// findStartDate ищет дату начала конкурса в DOM-узле.
func findStartDate(n *html.Node) time.Time {
	var walk func(*html.Node)
	var dateStr string
	walk = func(node *html.Node) {
		if dateStr != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "start") || strings.Contains(class, "begin") ||
				strings.Contains(class, "date-from") || strings.Contains(class, "date_start") {
				dateStr = nodeText(node)
				return
			}
			if node.Data == "time" {
				if dt := attrVal(node, "datetime"); dt != "" {
					// Проверяем, что это дата начала (первый из двух).
					if !strings.Contains(strings.ToLower(attrVal(node, "class")), "end") {
						dateStr = dt
					}
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if dateStr == "" {
		// Fallback: ищем текст, похожий на дату начала, во всём узле.
		dateStr = findDateInText(n)
	}

	return parseDate(dateStr)
}

// findEndDate ищет дату окончания конкурса в DOM-узле.
func findEndDate(n *html.Node) time.Time {
	var walk func(*html.Node)
	var dateStr string
	walk = func(node *html.Node) {
		if dateStr != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "end") || strings.Contains(class, "finish") ||
				strings.Contains(class, "deadline") || strings.Contains(class, "date-to") ||
				strings.Contains(class, "date_end") {
				dateStr = nodeText(node)
				return
			}
			if node.Data == "time" {
				if dt := attrVal(node, "datetime"); dt != "" {
					dateStr = dt
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if dateStr == "" {
		dateStr = findSecondDateInText(n)
	}

	return parseDate(dateStr)
}

// extractDateFromParent пытается извлечь дату начала из родительских элементов.
func extractDateFromParent(n *html.Node) time.Time {
	for p := n.Parent; p != nil; p = p.Parent {
		if d := findStartDate(p); !d.IsZero() {
			return d
		}
	}
	return time.Time{}
}

// extractEndDateFromParent пытается извлечь дату окончания из родительских элементов.
func extractEndDateFromParent(n *html.Node) time.Time {
	for p := n.Parent; p != nil; p = p.Parent {
		if d := findEndDate(p); !d.IsZero() {
			return d
		}
	}
	return time.Time{}
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

// extractContestDescription извлекает описание, исключая заголовок.
func extractContestDescription(n *html.Node, title string) string {
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

	// Попробуем извлечь дату из более длинной строки.
	// Ищем подстроку DD.MM.YYYY.
	for i := 0; i <= len(s)-10; i++ {
		if isDigit(s[i]) && isDigit(s[i+1]) &&
			(s[i+2] == '.' || s[i+2] == '/') &&
			isDigit(s[i+3]) && isDigit(s[i+4]) &&
			(s[i+5] == '.' || s[i+5] == '/') &&
			isDigit(s[i+6]) && isDigit(s[i+7]) &&
			isDigit(s[i+8]) && isDigit(s[i+9]) {
			dateStr := s[i : i+10]
			for _, layout := range []string{"02.01.2006", "02/01/2006"} {
				if t, err := time.Parse(layout, dateStr); err == nil {
					return t
				}
			}
		}
	}

	return time.Time{}
}

func isDigit(r byte) bool {
	return r >= '0' && r <= '9'
}

// findDateInText ищет первую строку, похожую на дату, в текстовом содержимом.
func findDateInText(n *html.Node) string {
	text := nodeText(n)
	for i := 0; i <= len(text)-10; i++ {
		if isDigit(text[i]) && isDigit(text[i+1]) &&
			(text[i+2] == '.' || text[i+2] == '/') &&
			isDigit(text[i+3]) && isDigit(text[i+4]) &&
			(text[i+5] == '.' || text[i+5] == '/') &&
			isDigit(text[i+6]) && isDigit(text[i+7]) &&
			isDigit(text[i+8]) && isDigit(text[i+9]) {
			return text[i : i+10]
		}
	}
	return ""
}

// findSecondDateInText ищет вторую строку, похожую на дату (для даты окончания).
func findSecondDateInText(n *html.Node) string {
	text := nodeText(n)
	found := 0
	for i := 0; i <= len(text)-10; i++ {
		if isDigit(text[i]) && isDigit(text[i+1]) &&
			(text[i+2] == '.' || text[i+2] == '/') &&
			isDigit(text[i+3]) && isDigit(text[i+4]) &&
			(text[i+5] == '.' || text[i+5] == '/') &&
			isDigit(text[i+6]) && isDigit(text[i+7]) &&
			isDigit(text[i+8]) && isDigit(text[i+9]) {
			found++
			if found == 2 {
				return text[i : i+10]
			}
		}
	}
	return ""
}

// IngestContests записывает конкурсы в хранилище и индексирует в RAG.
func IngestContests(ctx context.Context, contestList []*model.Contest, st store.ContestStore, ragSvc *rag.Service, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(contestList)}

	for _, c := range contestList {
		if c.Title == "" || c.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}

		isNew := false
		if existing, err := st.GetContest(ctx, c.ID); err == nil {
			// Обновляем существующий конкурс.
			c.CreatedAt = existing.CreatedAt
			if err := st.UpdateContest(ctx, c); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", c.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём новый.
			c.CreatedAt = time.Now()
			if err := st.CreateContest(ctx, c); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", c.ID, err))
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
			EntityType: changes.EntityContest,
			EntityID:   c.ID,
			Title:      c.Title,
			Category:   c.Category,
			Kind:       kind,
			SourceURL:  c.SourceURL,
			DetectedAt: time.Now(),
		})

		// Индексация в RAG (если сервис доступен).
		if ragSvc != nil {
			if err := indexContestToRAG(ctx, c, ragSvc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("RAG %s: %v", c.ID, err))
			}
		}
	}

	return res, nil
}

// indexContestToRAG индексирует конкурс в RAG как документ категории «Конкурсы».
func indexContestToRAG(ctx context.Context, c *model.Contest, ragSvc *rag.Service) error {
	// Формируем текстовое представление конкурса для индексации.
	var b strings.Builder
	b.WriteString(c.Title)
	b.WriteString("\n\n")
	if !c.StartDate.IsZero() {
		b.WriteString("Дата начала: " + c.StartDate.Format("02.01.2006"))
		if !c.EndDate.IsZero() {
			b.WriteString(" — Дата окончания: " + c.EndDate.Format("02.01.2006"))
		}
		b.WriteString("\n\n")
	}
	if c.Requirements != "" {
		b.WriteString("Требования: " + c.Requirements + "\n\n")
	}
	if c.Prize != "" {
		b.WriteString("Призы: " + c.Prize + "\n\n")
	}
	if c.Description != "" {
		b.WriteString(c.Description)
		b.WriteString("\n\n")
	}
	b.WriteString("Источник: " + c.SourceURL)

	category := c.Category
	if category == "" {
		category = "Конкурсы"
	}
	// Только действующие конкурсы попадают в поиск; закрытые отфильтровываются.
	status := "действует"
	if string(c.Status) != "active" {
		status = "устарел"
	}
	_, err := ragSvc.IndexEntity(ctx, rag.EntityDoc{
		ID:         c.ID,
		EntityType: "contest",
		Title:      c.Title,
		SourceURL:  c.SourceURL,
		Category:   category,
		Status:     status,
		Text:       b.String(),
	})
	return err
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func contestID(contestURL string) string {
	sum := sha1.Sum([]byte(contestURL))
	return "contest-" + hex.EncodeToString(sum[:])[:16]
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
