// Package faq парсит FAQ-страницы Сколково и заводит их в хранилище и RAG
// как категорию «FAQ».
package faq

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

// FAQConfig — конфигурация источника FAQ.
type FAQConfig struct {
	FAQURL   string // URL страницы FAQ (например dochub.sk.ru/faq)
	Category string // категория FAQ (по умолчанию «FAQ»)
}

// Monitor загружает FAQ и синхронизирует их в хранилище и RAG.
type Monitor struct {
	Cfg   FAQConfig
	Store store.FAQStore
	Rag   *rag.Service // может быть nil — тогда без индексации
	HTTP  *http.Client
}

// New создаёт монитор FAQ.
func New(cfg FAQConfig, st store.FAQStore, ragSvc *rag.Service) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "FAQ"
	}
	return &Monitor{
		Cfg:   FAQConfig{FAQURL: cfg.FAQURL, Category: category},
		Store: st,
		Rag:   ragSvc,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации FAQ.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// ParseFAQ — основная функция: загружает HTML-страницу FAQ и извлекает
// элементы вопрос/ответ. Возвращает []*model.FAQItem.
func ParseFAQ(ctx context.Context, cfg FAQConfig, hc *http.Client) ([]*model.FAQItem, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	if cfg.FAQURL == "" {
		return nil, fmt.Errorf("не указан FAQURL в конфигурации FAQ")
	}

	return parseFAQFromHTML(ctx, cfg.FAQURL, cfg.Category, hc)
}

// parseFAQFromHTML загружает страницу FAQ и извлекает элементы вопрос/ответ.
func parseFAQFromHTML(ctx context.Context, sourceURL, category string, hc *http.Client) ([]*model.FAQItem, error) {
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
		return nil, fmt.Errorf("HTML FAQ: статус %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	// Ищем FAQ-элементы по характерным паттернам.
	items := findFAQItems(doc, sourceURL)

	now := time.Now()
	for _, it := range items {
		it.Category = category
		it.CreatedAt = now
	}

	return items, nil
}

// faqEntry — промежуточная структура для распарсенного элемента FAQ.
type faqEntry struct {
	Question string
	Answer   string
	Category string
	URL      string
}

// findFAQItems ищет элементы FAQ в DOM-дереве.
func findFAQItems(doc *html.Node, sourceURL string) []*model.FAQItem {
	var entries []faqEntry

	// Стратегия 1: ищем элементы с классами «faq», «question», «answer», «accordion».
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			class := strings.ToLower(attrVal(n, "class"))
			if strings.Contains(class, "faq-item") ||
				strings.Contains(class, "faq-item") ||
				strings.Contains(class, "accordion-item") ||
				strings.Contains(class, "qa-item") {
				if entry := extractFAQEntry(n); entry.Question != "" && entry.Answer != "" {
					entry.URL = sourceURL
					entries = append(entries, entry)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Стратегия 2: ищем <details> элементы (нативный аккордеон).
	if len(entries) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "details" {
				entry := extractFAQFromDetails(n)
				if entry.Question != "" && entry.Answer != "" {
					entry.URL = sourceURL
					entries = append(entries, entry)
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}

	// Стратегия 3: ищем пары заголовок+текст внутри секций FAQ.
	if len(entries) == 0 {
		entries = findFAQSections(doc, sourceURL)
	}

	items := make([]*model.FAQItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, &model.FAQItem{
			ID:        faqID(e.Question, e.URL),
			Question:  strings.TrimSpace(e.Question),
			Answer:    strings.TrimSpace(e.Answer),
			Category:  e.Category,
			SourceURL: e.URL,
		})
	}

	return items
}

// extractFAQEntry извлекает вопрос/ответ из DOM-узла с классом faq-item/accordion.
func extractFAQEntry(n *html.Node) faqEntry {
	var entry faqEntry

	// Ищем вопрос — обычно это заголовок или элемент с классом «question», «title».
	entry.Question = findFAQQuestion(n)

	// Ищем ответ — элемент с классом «answer», «content», «body» или оставшийся текст.
	entry.Answer = findFAQAnswer(n, entry.Question)

	return entry
}

// extractFAQFromDetails извлекает вопрос/ответ из <details>/<summary>.
func extractFAQFromDetails(n *html.Node) faqEntry {
	var entry faqEntry

	// Ищем <summary> — это вопрос.
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if entry.Question == "" && node.Type == html.ElementNode && node.Data == "summary" {
			entry.Question = nodeText(node)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	// Ответ — весь текст внутри <details> кроме <summary>.
	if entry.Question == "" {
		entry.Question = nodeText(n)
	}
	entry.Answer = detailsBodyText(n, entry.Question)

	return entry
}

// findFAQQuestion ищет текст вопроса в узле.
func findFAQQuestion(n *html.Node) string {
	var q string

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if q != "" {
			return
		}
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "question") ||
				strings.Contains(class, "title") ||
				strings.Contains(class, "header") {
				q = nodeText(node)
				return
			}
			// Заголовки h1-h6.
			switch node.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				q = nodeText(node)
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	return q
}

// findFAQAnswer ищет текст ответа в узле, исключая вопрос.
func findFAQAnswer(n *html.Node, question string) string {
	var answer string

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			class := strings.ToLower(attrVal(node, "class"))
			if strings.Contains(class, "answer") ||
				strings.Contains(class, "content") ||
				strings.Contains(class, "body") ||
				strings.Contains(class, "text") {
				if a := nodeText(node); a != "" && a != question {
					answer = a
					return
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if answer == "" {
		// Fallback: весь текст узла минус вопрос.
		text := nodeText(n)
		text = strings.TrimSpace(text)
		if question != "" && strings.HasPrefix(text, question) {
			text = strings.TrimPrefix(text, question)
			text = strings.TrimSpace(text)
		}
		answer = text
	}

	return answer
}

// detailsBodyText извлекает текст из <details> исключая <summary>.
func detailsBodyText(n *html.Node, summary string) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		// Пропускаем сам <summary>.
		if node.Type == html.ElementNode && node.Data == "summary" {
			return
		}
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	text := strings.TrimSpace(b.String())
	if summary != "" && strings.HasPrefix(text, summary) {
		text = strings.TrimPrefix(text, summary)
		text = strings.TrimSpace(text)
	}
	return text
}

// findFAQSections ищет секции FAQ по заголовкам и следующему за ними контенту.
func findFAQSections(doc *html.Node, sourceURL string) []faqEntry {
	var entries []faqEntry

	// Ищем элементы с id/class содержащими "faq".
	var faqRoot *html.Node
	var walkFind func(*html.Node)
	walkFind = func(n *html.Node) {
		if faqRoot != nil {
			return
		}
		if n.Type == html.ElementNode {
			id := strings.ToLower(attrVal(n, "id"))
			class := strings.ToLower(attrVal(n, "class"))
			if strings.Contains(id, "faq") || strings.Contains(class, "faq") {
				faqRoot = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkFind(c)
		}
	}
	walkFind(doc)

	if faqRoot == nil {
		return entries
	}

	// Внутри FAQ-секции ищем пары заголовок + контент.
	var walkSections func(*html.Node, *faqEntry)
	walkSections = func(n *html.Node, current *faqEntry) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				// Если уже есть текущий элемент с вопросом и ответом — сохраняем.
				if current != nil && current.Question != "" && current.Answer != "" {
					entries = append(entries, *current)
				}
				// Новый вопрос.
				*current = faqEntry{Question: nodeText(n), URL: sourceURL}
			case "p", "div":
				if current != nil && current.Question != "" && current.Answer == "" {
					txt := nodeText(n)
					if txt != "" && txt != current.Question {
						current.Answer = txt
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkSections(c, current)
		}
	}

	var cur faqEntry
	walkSections(faqRoot, &cur)
	if cur.Question != "" && cur.Answer != "" {
		entries = append(entries, cur)
	}

	return entries
}

// IngestFAQ записывает элементы FAQ в хранилище и индексирует в RAG.
func IngestFAQ(ctx context.Context, items []*model.FAQItem, st store.FAQStore, ragSvc *rag.Service, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(items)}

	for _, it := range items {
		if it.Question == "" || it.Answer == "" || it.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой вопрос, ответ или URL")
			continue
		}

		isNew := false
		if existing, err := st.GetFAQItem(ctx, it.ID); err == nil {
			// Обновляем существующий элемент.
			it.CreatedAt = existing.CreatedAt
			if err := st.UpdateFAQItem(ctx, it); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", it.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём новый элемент.
			it.CreatedAt = time.Now()
			if err := st.CreateFAQItem(ctx, it); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", it.ID, err))
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
			EntityType: changes.EntityFAQ,
			EntityID:   it.ID,
			Title:      it.Question,
			Category:   it.Category,
			Kind:       kind,
			SourceURL:  it.SourceURL,
			DetectedAt: time.Now(),
		})

		// Индексация в RAG (если сервис доступен).
		if ragSvc != nil {
			if err := indexFAQToRAG(ctx, it, ragSvc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("RAG %s: %v", it.ID, err))
			}
		}
	}

	return res, nil
}

// indexFAQToRAG индексирует элемент FAQ в RAG как документ категории «FAQ».
func indexFAQToRAG(ctx context.Context, it *model.FAQItem, ragSvc *rag.Service) error {
	// Формируем текстовое представление FAQ для индексации.
	var b strings.Builder
	b.WriteString("Вопрос: " + it.Question + "\n\n")
	b.WriteString("Ответ: " + it.Answer + "\n\n")
	b.WriteString("Источник: " + it.SourceURL)

	category := it.Category
	if category == "" {
		category = "FAQ"
	}
	_, err := ragSvc.IndexEntity(ctx, rag.EntityDoc{
		ID:         it.ID,
		EntityType: "faq",
		Title:      it.Question,
		SourceURL:  it.SourceURL,
		Category:   category,
		Status:     "действует", // у FAQ нет статуса — всегда актуально
		Text:       b.String(),
	})
	return err
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func faqID(question, sourceURL string) string {
	raw := question + "|" + sourceURL
	sum := sha1.Sum([]byte(raw))
	return "faq-" + hex.EncodeToString(sum[:])[:16]
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

// ---------------------------------------------------------------------------
// Вспомогательные функции для HTML-парсинга (дублируем из events для
// автономности пакета).
// ---------------------------------------------------------------------------

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
