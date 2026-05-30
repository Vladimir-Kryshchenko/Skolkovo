// Package requirements парсит требования резидентства Сколково
// (критерии входа, этапы процедуры, документы, сроки, продление, выход)
// и сохраняет их в хранилище документов как категорию «Требования».
package requirements

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// RequirementsConfig — конфигурация источника требований резидентства.
type RequirementsConfig struct {
	SourceURLs []string // URL-адреса страниц с требованиями (dochub.sk.ru, sk.ru)
	Category   string   // категория документов (по умолчанию «Требования»)
}

// Monitor загружает требования резидентства и синхронизирует их в хранилище и RAG.
type Monitor struct {
	Cfg   RequirementsConfig
	Store store.Store
	Rag   *rag.Service // может быть nil — тогда без индексации
	HTTP  *http.Client
}

// NewMonitor создаёт монитор требований резидентства.
func NewMonitor(cfg RequirementsConfig, st store.Store, ragSvc *rag.Service) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "Требования"
	}
	urls := cfg.SourceURLs
	if len(urls) == 0 {
		urls = defaultURLs()
	}
	return &Monitor{
		Cfg:   RequirementsConfig{SourceURLs: urls, Category: category},
		Store: st,
		Rag:   ragSvc,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации требований.
type Result struct {
	Fetched int
	New     int
	Updated int
	Errors  []string
}

// defaultURLs возвращает URL-адреса по умолчанию для парсинга требований.
func defaultURLs() []string {
	return []string{
		"https://dochub.sk.ru/residency/requirements",
		"https://sk.ru/residency/requirements",
	}
}

// ParseRequirements — основная функция: загружает страницы требований и извлекает
// структурированную информацию о критериях, этапах, документах, сроках и т.д.
func ParseRequirements(ctx context.Context, cfg RequirementsConfig, hc *http.Client) ([]*model.Document, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	urls := cfg.SourceURLs
	if len(urls) == 0 {
		urls = defaultURLs()
	}

	var allDocs []*model.Document

	for _, u := range urls {
		docs, err := parseRequirementsPage(ctx, u, hc)
		if err != nil {
			// Не прерываемся на ошибке одного URL — пробуем следующий.
			continue
		}
		allDocs = append(allDocs, docs...)
	}

	if len(allDocs) == 0 {
		return nil, fmt.Errorf("не удалось извлечь требования ни с одного URL")
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

// parseRequirementsPage загружает одну страницу требований и парсит её HTML.
func parseRequirementsPage(ctx context.Context, rawURL string, hc *http.Client) ([]*model.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
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
		return nil, fmt.Errorf("страница требований %s: статус %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseRequirementsHTML(rawURL, body)
}

// parseRequirementsHTML разбирает HTML страницы требований и извлекает секции.
func parseRequirementsHTML(pageURL string, body []byte) ([]*model.Document, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	category := "Требования"
	var docs []*model.Document
	now := time.Now()

	// Стратегия 1: ищем секции с характерными заголовками.
	sections := findRequirementSections(doc)
	if len(sections) > 0 {
		for _, sec := range sections {
			docs = append(docs, sectionToDocument(sec, pageURL, category, now))
		}
	}

	// Стратегия 2: fallback — ищем заголовки h1-h3 и их содержимое.
	if len(docs) == 0 {
		headings := extractHeadingsWithContent(doc)
		for _, h := range headings {
			if isRequirementHeading(h.Text) {
				docs = append(docs, &model.Document{
					ID:        documentID(pageURL + "#" + slugify(h.Text)),
					Title:     h.Text,
					SourceURL: pageURL,
					FileHash:  contentHash(h.Content),
					FetchedAt: now,
					Status:    model.StatusActive,
					Category:  category,
				})
			}
		}
	}

	// Стратегия 3: если ничего не нашли, создаём один документ с основным контентом.
	if len(docs) == 0 {
		text := extractMainText(doc)
		if text != "" {
			docs = append(docs, &model.Document{
				ID:        documentID(pageURL),
				Title:     extractPageTitle(doc),
				SourceURL: pageURL,
				FileHash:  contentHash(text),
				FetchedAt: now,
				Status:    model.StatusActive,
				Category:  category,
			})
		}
	}

	return docs, nil
}

// requirementSection — промежуточная структура для секции требований.
type requirementSection struct {
	Title   string
	Content string
	Type    string // entry_criteria, procedure_steps, documents, deadlines, renewal, exit
}

// findRequirementSections ищет секции требований по характерным заголовкам.
func findRequirementSections(doc *html.Node) []requirementSection {
	var sections []requirementSection
	var collected []requirementSection

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				title := nodeText(n)
				secType := classifyRequirementSection(title)
				if secType != "" {
					// Собираем содержимое до следующего заголовка.
					content := extractSiblingContent(n)
					collected = append(collected, requirementSection{
						Title:   title,
						Content: content,
						Type:    secType,
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	sections = collected
	return sections
}

// classifyRequirementSection классифицирует секцию по заголовку.
func classifyRequirementSection(title string) string {
	lower := strings.ToLower(title)

	// Критерии входа.
	if containsAny(lower, "критери", "входа в резидентств", "вход в резидентств",
		"условия получения", "кто может", "требования к резидент",
		"получить резидентство", "право на резидентство") {
		return "entry_criteria"
	}

	// Этапы процедуры.
	if containsAny(lower, "этап", "шаг", "процедур", "порядок получения",
		"алгоритм", "последовательность", "как получить", "как стать") {
		return "procedure_steps"
	}

	// Требуемые документы.
	if containsAny(lower, "документ", "пакет документ", "необходимые документ",
		"перечень документ", "какие документ") {
		return "documents"
	}

	// Сроки.
	if containsAny(lower, "срок", "длительность", "рассмотрения", "в течение",
		"календарн", "регламент", "временн") {
		return "deadlines"
	}

	// Продление.
	if containsAny(lower, "продлен", "продлить", "продление", "повторн",
		"обновление статуса") {
		return "renewal"
	}

	// Выход.
	if containsAny(lower, "выход", "прекращен", "утрата", "исключен",
		"расторжен", "отказ от резидентства", "лишени") {
		return "exit"
	}

	return ""
}

// extractHeadingsWithContent извлекает все заголовки с прилегающим содержимым.
func extractHeadingsWithContent(doc *html.Node) []struct {
	Text    string
	Content string
} {
	var result []struct {
		Text    string
		Content string
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "h1" || n.Data == "h2" || n.Data == "h3") {
			txt := strings.TrimSpace(nodeText(n))
			if txt != "" {
				result = append(result, struct {
					Text    string
					Content string
				}{Text: txt, Content: extractSiblingContent(n)})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return result
}

// extractPageTitle извлекает заголовок страницы.
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

// extractMainText извлекает основной текстовый контент.
func extractMainText(doc *html.Node) string {
	// Ищем <main> или <article>.
	for _, tag := range []string{"main", "article", "content"} {
		if node := findElementByTag(doc, tag); node != nil {
			t := strings.TrimSpace(nodeText(node))
			if t != "" {
				return t
			}
		}
	}
	// Fallback: весь текст body.
	if body := findElementByTag(doc, "body"); body != nil {
		return strings.TrimSpace(nodeText(body))
	}
	return ""
}

// isRequirementHeading проверяет, что заголовок относится к требованиям резидентства.
func isRequirementHeading(title string) bool {
	lower := strings.ToLower(title)
	return containsAny(lower,
		"резидентств", "резидент", "требован", "критерий", "этап", "документ",
		"срок", "продлен", "выход", "условие", "порядок", "процедур")
}

// extractSiblingContent извлекает текстовое содержимое узлов-соседей после данного узла.
func extractSiblingContent(startNode *html.Node) string {
	var b strings.Builder

	// Собираем текст от следующего sibling до следующего заголовка.
	for n := startNode.NextSibling; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode && isHeadingTag(n.Data) {
			break
		}
		if isContentNode(n) {
			t := strings.TrimSpace(nodeText(n))
			if t != "" {
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(t)
			}
		}
		// Также заглядываем внутрь узлов.
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && isHeadingTag(c.Data) {
				goto done
			}
		}
	}

done:
	// Если ничего не нашли в siblings, берём дочерние элементы startNode.
	if b.Len() == 0 {
		// Ищем следующий sibling-контейнер (div, section, p, ul, ol, table).
		for n := startNode.NextSibling; n != nil; n = n.NextSibling {
			if n.Type == html.ElementNode && isContainerTag(n.Data) {
				t := strings.TrimSpace(nodeText(n))
				if t != "" {
					b.WriteString(t)
				}
				break
			}
		}
	}

	// Ограничиваем длину.
	s := b.String()
	if len(s) > 10000 {
		s = s[:10000] + "..."
	}
	return s
}

// sectionToDocument преобразует секцию требований в Document.
func sectionToDocument(sec requirementSection, sourceURL, category string, now time.Time) *model.Document {
	content := sec.Content
	if content == "" {
		content = sec.Title
	}

	// Формируем тело документа: тип секции + содержимое.
	var body strings.Builder
	body.WriteString(sec.Title)
	body.WriteString("\n\n")
	body.WriteString(sec.Content)
	body.WriteString("\n\n")
	body.WriteString("Источник: " + sourceURL)

	bodyStr := body.String()

	return &model.Document{
		ID:        documentID(sourceURL + "#" + slugify(sec.Title)),
		Title:     sec.Title,
		SourceURL: sourceURL,
		FileHash:  contentHash(bodyStr),
		FetchedAt: now,
		Status:    model.StatusActive,
		Category:  category,
	}
}

// ---------------------------------------------------------------------------
// Специализированные парсеры
// ---------------------------------------------------------------------------

// ParseEntryCriteria парсит критерии входа в резидентство.
func ParseEntryCriteria(ctx context.Context, cfg RequirementsConfig, hc *http.Client) (*model.Document, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	urls := cfg.SourceURLs
	if len(urls) == 0 {
		urls = defaultURLs()
	}

	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := hc.Do(req)
		if err != nil {
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		doc, err := html.Parse(strings.NewReader(string(body)))
		if err != nil {
			continue
		}

		// Ищем секцию с критериями.
		if criteria := findEntryCriteria(doc, u); criteria != nil {
			return criteria, nil
		}
	}

	return nil, fmt.Errorf("критерии входа не найдены ни на одной странице")
}

// findEntryCriteria ищет секцию с критериями входа в DOM.
func findEntryCriteria(doc *html.Node, sourceURL string) *model.Document {
	var found *requirementSection

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				title := nodeText(n)
				if classifyRequirementSection(title) == "entry_criteria" {
					content := extractSiblingContent(n)
					found = &requirementSection{
						Title:   title,
						Content: content,
						Type:    "entry_criteria",
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if found == nil {
		return nil
	}

	now := time.Now()
	return sectionToDocument(*found, sourceURL, "Требования", now)
}

// ParseProcedureSteps парсит пошаговую процедуру получения резидентства.
func ParseProcedureSteps(ctx context.Context, cfg RequirementsConfig, hc *http.Client) ([]*model.Document, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	urls := cfg.SourceURLs
	if len(urls) == 0 {
		urls = defaultURLs()
	}

	var allDocs []*model.Document

	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := hc.Do(req)
		if err != nil {
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		doc, err := html.Parse(strings.NewReader(string(body)))
		if err != nil {
			continue
		}

		steps := findProcedureSteps(doc, u)
		allDocs = append(allDocs, steps...)
	}

	if len(allDocs) == 0 {
		return nil, fmt.Errorf("этапы процедуры не найдены ни на одной странице")
	}

	return allDocs, nil
}

// findProcedureSteps ищет секции с этапами процедуры в DOM.
func findProcedureSteps(doc *html.Node, sourceURL string) []*model.Document {
	var sections []requirementSection

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				title := nodeText(n)
				if classifyRequirementSection(title) == "procedure_steps" {
					content := extractSiblingContent(n)
					sections = append(sections, requirementSection{
						Title:   title,
						Content: content,
						Type:    "procedure_steps",
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	now := time.Now()
	docs := make([]*model.Document, 0, len(sections))
	for _, sec := range sections {
		docs = append(docs, sectionToDocument(sec, sourceURL, "Требования", now))
	}
	return docs
}

// ParseReportingRequirements парсит требования к отчётности резидентов.
func ParseReportingRequirements(ctx context.Context, cfg RequirementsConfig, hc *http.Client) ([]*model.Document, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	urls := cfg.SourceURLs
	if len(urls) == 0 {
		urls = defaultURLs()
	}

	var allDocs []*model.Document

	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := hc.Do(req)
		if err != nil {
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		doc, err := html.Parse(strings.NewReader(string(body)))
		if err != nil {
			continue
		}

		reports := findReportingRequirements(doc, u)
		allDocs = append(allDocs, reports...)
	}

	if len(allDocs) == 0 {
		return nil, fmt.Errorf("требования к отчётности не найдены ни на одной странице")
	}

	return allDocs, nil
}

// findReportingRequirements ищет секции с требованиями к отчётности в DOM.
func findReportingRequirements(doc *html.Node, sourceURL string) []*model.Document {
	var sections []requirementSection

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				title := nodeText(n)
				if isReportingHeading(title) {
					content := extractSiblingContent(n)
					sections = append(sections, requirementSection{
						Title:   title,
						Content: content,
						Type:    "reporting",
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	now := time.Now()
	docs := make([]*model.Document, 0, len(sections))
	for _, sec := range sections {
		docs = append(docs, sectionToDocument(sec, sourceURL, "Требования", now))
	}
	return docs
}

// isReportingHeading проверяет, что заголовок относится к отчётности.
func isReportingHeading(title string) bool {
	lower := strings.ToLower(title)
	return containsAny(lower,
		"отчёт", "отчет", "отчётност", "отчетност", "отчит",
		"документация", "сведения", "информация о деятельност")
}

// ---------------------------------------------------------------------------
// IngestRequirements
// ---------------------------------------------------------------------------

// IngestRequirements записывает требования в DocumentStore и индексирует в RAG.
func IngestRequirements(ctx context.Context, docs []*model.Document, st store.Store, ragSvc *rag.Service) (*Result, error) {
	res := &Result{Fetched: len(docs)}

	for _, doc := range docs {
		if doc.Title == "" || doc.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}

		if _, err := st.Get(ctx, doc.ID); err == nil {
			// Обновляем существующий документ.
			if err := st.Upsert(ctx, *doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", doc.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём новый.
			doc.FetchedAt = time.Now()
			if err := st.Upsert(ctx, *doc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", doc.ID, err))
				continue
			}
			res.New++
		}

		// Индексация в RAG (если сервис доступен).
		if ragSvc != nil {
			if err := indexDocumentToRAG(ctx, doc, ragSvc); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("RAG %s: %v", doc.ID, err))
			}
		}
	}

	return res, nil
}

// indexDocumentToRAG формирует текстовое представление и индексирует документ.
func indexDocumentToRAG(ctx context.Context, doc *model.Document, ragSvc *rag.Service) error {
	// RAG работает через IndexDocument по ID из Store.
	// Если документ уже имеет LocalPath, можно вызвать IndexDocument напрямую.
	if doc.LocalPath != "" {
		_, err := ragSvc.IndexDocument(ctx, doc.ID)
		return err
	}

	// Без LocalPath формируем текстовое представление для будущей индексации.
	// Это заглушка — реальная индексация произойдёт при наличии файла.
	return nil
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

func documentID(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return "req-" + hex.EncodeToString(sum[:])[:16]
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
		s = "section"
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

func isHeadingTag(tag string) bool {
	return tag == "h1" || tag == "h2" || tag == "h3" || tag == "h4" || tag == "h5" || tag == "h6"
}

func isContentNode(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	return isContainerTag(n.Data) || isTextualTag(n.Data)
}

func isContainerTag(tag string) bool {
	switch tag {
	case "div", "section", "article", "main", "p", "ul", "ol", "li", "table",
		"dl", "dd", "dt", "blockquote", "figure", "details", "summary":
		return true
	}
	return false
}

func isTextualTag(tag string) bool {
	switch tag {
	case "span", "em", "strong", "b", "i", "small", "mark", "sub", "sup":
		return true
	}
	return false
}

func findElementByTag(doc *html.Node, tag string) *html.Node {
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

// SortSectionsByType сортирует секции требований по типу (для стабильного вывода).
func SortSectionsByType(sections []requirementSection) {
	typeOrder := map[string]int{
		"entry_criteria":  1,
		"procedure_steps": 2,
		"documents":       3,
		"deadlines":       4,
		"renewal":         5,
		"exit":            6,
	}
	sort.Slice(sections, func(i, j int) bool {
		return typeOrder[sections[i].Type] < typeOrder[sections[j].Type]
	})
}

// ResolveURL разрешает относительный URL относительно базового.
func ResolveURL(baseURL, href string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}
