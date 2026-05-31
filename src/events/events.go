// Package events парсит мероприятия Сколково через RSS и HTML-скрапинг
// и заводит их в хранилище и RAG как категорию «Мероприятия».
package events

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
	"baza-skolkovo/src/common/feed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// userAgent — браузерный UA для страниц, отдающих контент только браузерам.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// EventsConfig — конфигурация источника мероприятий.
type EventsConfig struct {
	RSSURL    string // URL RSS-ленты мероприятий (если доступен)
	SourceURL string // URL HTML-страницы мероприятий (fallback: sk.ru/events)
	Category  string // категория мероприятий (по умолчанию «Мероприятия»)
}

// Monitor загружает мероприятия и синхронизирует их в хранилище и RAG.
type Monitor struct {
	Cfg   EventsConfig
	Store store.EventStore
	Rag   *rag.Service // может быть nil — тогда без индексации
	HTTP  *http.Client
}

// New создаёт монитор мероприятий.
func New(cfg EventsConfig, st store.EventStore, ragSvc *rag.Service) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "Мероприятия"
	}
	return &Monitor{
		Cfg:   EventsConfig{RSSURL: cfg.RSSURL, SourceURL: cfg.SourceURL, Category: category},
		Store: st,
		Rag:   ragSvc,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации мероприятий.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// ParseEvents — основная функция: пробует RSS, fallback — HTML-парсинг.
// Возвращает []*model.Event.
func ParseEvents(ctx context.Context, cfg EventsConfig, hc *http.Client) ([]*model.Event, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	// Пробуем RSS, если URL задан.
	if cfg.RSSURL != "" {
		if items, err := parseEventsFromRSS(ctx, cfg.RSSURL, hc); err == nil && len(items) > 0 {
			return items, nil
		}
	}

	// Fallback: HTML-парсинг.
	if cfg.SourceURL != "" {
		return parseEventsFromHTML(ctx, cfg.SourceURL, hc)
	}

	return nil, fmt.Errorf("не указан ни RSSURL, ни SourceURL в конфигурации мероприятий")
}

// parseEventsFromRSS парсит RSS-ленту мероприятий и маппит FeedItem → Event.
func parseEventsFromRSS(ctx context.Context, rssURL string, hc *http.Client) ([]*model.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
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
		return nil, fmt.Errorf("RSS мероприятий: статус %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	items := feed.Parse(data)
	events := make([]*model.Event, 0, len(items))
	now := time.Now()

	for _, it := range items {
		if it.Link == "" || it.Title == "" {
			continue
		}
		eventDate := time.Time{}
		if it.Published != nil {
			eventDate = *it.Published
		}

		status := model.EventActive
		if !eventDate.IsZero() && eventDate.Before(now) {
			status = model.EventPast
		}

		events = append(events, &model.Event{
			ID:          eventID(it.Link),
			Title:       strings.TrimSpace(it.Title),
			Description: feed.StripTags(it.Summary),
			EventDate:   eventDate,
			SourceURL:   it.Link,
			Status:      status,
			Category:    "Мероприятия",
			CreatedAt:   time.Now(),
		})
	}
	return events, nil
}

// parseEventsFromHTML загружает страницу мероприятий и извлекает карточки/элементы.
func parseEventsFromHTML(ctx context.Context, sourceURL string, hc *http.Client) ([]*model.Event, error) {
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
		return nil, fmt.Errorf("HTML мероприятий: статус %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var events []*model.Event
	now := time.Now()

	// Ищем карточки мероприятий по распространённым паттернам.
	// Сначала пробуем найти элементы с классами, характерными для карточек событий.
	cards := findEventCards(doc)
	if len(cards) == 0 {
		// Fallback: ищем любые ссылки с текстом, похожим на мероприятия.
		cards = findEventLinks(doc, sourceURL)
	}

	for _, card := range cards {
		if card.Title == "" || card.URL == "" {
			continue
		}

		status := model.EventActive
		if !card.Date.IsZero() && card.Date.Before(now) {
			status = model.EventPast
		}

		events = append(events, &model.Event{
			ID:           eventID(card.URL),
			Title:        strings.TrimSpace(card.Title),
			Description:  strings.TrimSpace(card.Description),
			EventDate:    card.Date,
			EventEndDate: card.EndDate,
			Location:     strings.TrimSpace(card.Location),
			SourceURL:    card.URL,
			Status:       status,
			Category:     "Мероприятия",
			CreatedAt:    time.Now(),
		})
	}

	return events, nil
}

// eventCard — промежуточная структура для распарсенной карточки мероприятия.
type eventCard struct {
	Title       string
	Description string
	Date        time.Time
	EndDate     time.Time
	Location    string
	URL         string
}

// findEventCards ищет карточки мероприятий по характерным CSS-классам/структурам.
func findEventCards(doc *html.Node) []eventCard {
	var cards []eventCard

	// Стратегия 1: ищем элементы с классами, содержащими "event", "card", "item".
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := attrVal(n, "class")
			if strings.Contains(strings.ToLower(class), "event") ||
				strings.Contains(strings.ToLower(class), "card") {
				if card := extractCard(n); card.Title != "" && card.URL != "" {
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
				if card := extractCard(n); card.Title != "" && card.URL != "" {
					cards = append(cards, card)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	// Стратегия 3: ищем <li> элементы внутри списков мероприятий.
	if len(cards) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol") {
				class := attrVal(n, "class")
				if strings.Contains(strings.ToLower(class), "event") ||
					strings.Contains(strings.ToLower(class), "list") {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.ElementNode && c.Data == "li" {
							if card := extractCard(c); card.Title != "" && card.URL != "" {
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

// findEventLinks — fallback: ищет ссылки, похожие на мероприятия.
func findEventLinks(doc *html.Node, baseURL string) []eventCard {
	var cards []eventCard
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
			// Проверяем, что ссылка ведёт на страницу мероприятия.
			if !isEventURL(abs) {
				goto recurse
			}

			title := nodeText(n)
			if title == "" {
				goto recurse
			}

			// Пытаемся извлечь дату из соседних элементов.
			date := extractDateFromParent(n)

			cards = append(cards, eventCard{
				Title: title,
				URL:   abs,
				Date:  date,
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

// isEventURL проверяет, что URL похож на страницу мероприятия.
func isEventURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	lower := strings.ToLower(u.Path)
	return strings.Contains(lower, "event") || strings.Contains(lower, "meropriyatie") ||
		strings.Contains(lower, "/events/")
}

// extractCard извлекает данные мероприятия из DOM-узла.
func extractCard(n *html.Node) eventCard {
	var card eventCard

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
				if strings.Contains(strings.ToLower(class), "title") ||
					strings.Contains(strings.ToLower(class), "name") ||
					strings.Contains(strings.ToLower(class), "event") {
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

	// Ищем дату.
	if card.Date.IsZero() {
		card.Date = findDate(n)
	}

	// Ищем локацию.
	card.Location = findLocation(n)

	// Ищем описание.
	card.Description = extractDescription(n, card.Title)

	// Ищем ссылку, если ещё не нашли.
	if card.URL == "" {
		card.URL = findLink(n)
	}

	return card
}

// findDate ищет дату мероприятия в DOM-узле.
func findDate(n *html.Node) time.Time {
	var walk func(*html.Node)
	var dateStr string
	walk = func(node *html.Node) {
		if dateStr != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "date") || strings.Contains(class, "time") ||
				strings.Contains(class, "datetime") {
				dateStr = nodeText(node)
				return
			}
			// Проверяем атрибут datetime (для <time> элементов).
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
		// Fallback: ищем текст, похожий на дату, во всём узле.
		dateStr = findDateInText(n)
	}

	return parseDate(dateStr)
}

// findDateInText ищет строку, похожую на дату, в текстовом содержимом.
func findDateInText(n *html.Node) string {
	text := nodeText(n)
	// Ищем паттерны дат: DD.MM.YYYY, DD/MM/YYYY, YYYY-MM-DD и т.д.
	for i := 0; i < len(text)-7; i++ {
		// Проверяем DD.MM.YYYY
		if i+9 <= len(text) && isDigit(text[i]) && isDigit(text[i+1]) &&
			text[i+2] == '.' && isDigit(text[i+3]) && isDigit(text[i+4]) &&
			text[i+5] == '.' && isDigit(text[i+6]) && isDigit(text[i+7]) &&
			isDigit(text[i+8]) && isDigit(text[i+9]) {
			return text[i : i+10]
		}
	}
	return ""
}

func isDigit(r byte) bool {
	return r >= '0' && r <= '9'
}

// findLocation ищет место проведения мероприятия.
func findLocation(n *html.Node) string {
	var walk func(*html.Node)
	var loc string
	walk = func(node *html.Node) {
		if loc != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "location") || strings.Contains(class, "place") ||
				strings.Contains(class, "venue") || strings.Contains(class, "address") ||
				strings.Contains(class, "place") {
				loc = nodeText(node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return loc
}

// extractDescription извлекает описание, исключая заголовок.
func extractDescription(n *html.Node, title string) string {
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

// extractDateFromParent пытается извлечь дату из родительских элементов.
func extractDateFromParent(n *html.Node) time.Time {
	for p := n.Parent; p != nil; p = p.Parent {
		if d := findDate(p); !d.IsZero() {
			return d
		}
	}
	return time.Time{}
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

// IngestEvents записывает мероприятия в хранилище и индексирует в RAG.
func IngestEvents(ctx context.Context, eventList []*model.Event, st store.EventStore, ragSvc *rag.Service, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(eventList)}

	for _, ev := range eventList {
		if ev.Title == "" || ev.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}

		isNew := false
		if existing, err := st.GetEvent(ctx, ev.ID); err == nil {
			// Обновляем существующее мероприятие.
			ev.CreatedAt = existing.CreatedAt
			if err := st.UpdateEvent(ctx, ev); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", ev.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём новое.
			ev.CreatedAt = time.Now()
			if err := st.CreateEvent(ctx, ev); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", ev.ID, err))
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
			EntityType: changes.EntityEvent,
			EntityID:   ev.ID,
			Title:      ev.Title,
			Category:   ev.Category,
			Kind:       kind,
			SourceURL:  ev.SourceURL,
			DetectedAt: time.Now(),
		})

		// Индексация в RAG (если сервис доступен).
		if ragSvc != nil {
			if err := indexEventToRAG(ctx, ev, ragSvc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("RAG %s: %v", ev.ID, err))
			}
		}
	}

	return res, nil
}

// indexEventToRAG индексирует мероприятие в RAG как документ категории «Мероприятия».
func indexEventToRAG(ctx context.Context, ev *model.Event, ragSvc *rag.Service) error {
	// Формируем текстовое представление мероприятия для индексации.
	var b strings.Builder
	b.WriteString(ev.Title)
	b.WriteString("\n\n")
	if !ev.EventDate.IsZero() {
		b.WriteString("Дата: " + ev.EventDate.Format("02.01.2006"))
		if !ev.EventEndDate.IsZero() {
			b.WriteString(" — " + ev.EventEndDate.Format("02.01.2006"))
		}
		b.WriteString("\n\n")
	}
	if ev.Location != "" {
		b.WriteString("Место: " + ev.Location + "\n\n")
	}
	if ev.Description != "" {
		b.WriteString(ev.Description)
		b.WriteString("\n\n")
	}
	b.WriteString("Источник: " + ev.SourceURL)

	category := ev.Category
	if category == "" {
		category = "Мероприятия"
	}
	// В поиск попадают только активные мероприятия; прошедшие/отменённые
	// помечаем «устарел», чтобы они отфильтровывались (FilterActive).
	status := "действует"
	if string(ev.Status) != "active" {
		status = "устарел"
	}
	_, err := ragSvc.IndexEntity(ctx, rag.EntityDoc{
		ID:         ev.ID,
		EntityType: "event",
		Title:      ev.Title,
		SourceURL:  ev.SourceURL,
		Category:   category,
		Status:     status,
		Text:       b.String(),
	})
	return err
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func eventID(eventURL string) string {
	sum := sha1.Sum([]byte(eventURL))
	return "event-" + hex.EncodeToString(sum[:])[:16]
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
