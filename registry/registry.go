// Package registry парсит публичный реестр резидентов Сколково через HTML-скрапинг
// и заводит их в хранилище как категорию «Реестр».
package registry

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// userAgent — браузерный UA для страниц, отдающих контент только браузерам.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// RegistryConfig — конфигурация источника реестра резидентов.
type RegistryConfig struct {
	SourceURL string // URL страницы реестра (по умолчанию "https://sk.ru/residents/")
	Category  string // категория (по умолчанию «Реестр»)
}

// Monitor загружает резидентов из реестра и синхронизирует их в хранилище.
type Monitor struct {
	Cfg   RegistryConfig
	Store store.ResidentStore
	HTTP  *http.Client
}

// New создаёт монитор реестра резидентов.
func New(cfg RegistryConfig, st store.ResidentStore) *Monitor {
	category := cfg.Category
	if category == "" {
		category = "Реестр"
	}
	srcURL := cfg.SourceURL
	if srcURL == "" {
		srcURL = "https://sk.ru/residents/"
	}
	return &Monitor{
		Cfg:   RegistryConfig{SourceURL: srcURL, Category: category},
		Store: st,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации резидентов.
type Result struct {
	New     int
	Updated int
	Errors  []string
}

// ParseResidents — основная функция: загружает страницу реестра и извлекает резидентов.
// Возвращает []*model.Resident.
func ParseResidents(ctx context.Context, cfg RegistryConfig, hc *http.Client) ([]*model.Resident, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	srcURL := cfg.SourceURL
	if srcURL == "" {
		srcURL = "https://sk.ru/residents/"
	}

	return parseResidentsTable(ctx, srcURL, hc)
}

// parseResidentsTable загружает страницу реестра и парсит таблицу резидентов.
func parseResidentsTable(ctx context.Context, sourceURL string, hc *http.Client) ([]*model.Resident, error) {
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
		return nil, fmt.Errorf("HTML реестр резидентов: статус %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var residents []*model.Resident

	// Стратегия 1: ищем <table> с классами, содержащими "resident", "registry", "registry-table".
	tables := findResidentTables(doc)
	if len(tables) > 0 {
		for _, table := range tables {
			residents = append(residents, parseTableRows(table, sourceURL)...)
		}
		return residents, nil
	}

	// Стратегия 2: ищем любые <table> элементы.
	tables = findAllTables(doc)
	if len(tables) > 0 {
		for _, table := range tables {
			residents = append(residents, parseTableRows(table, sourceURL)...)
		}
		if len(residents) > 0 {
			return residents, nil
		}
	}

	// Стратегия 3: ищем списки/карточки резидентов (div-ы с классами resident).
	cards := findResidentCards(doc, sourceURL)
	if len(cards) > 0 {
		return cards, nil
	}

	// Ничего не нашли — это не ошибка, просто пустой реестр.
	return nil, nil
}

// findResidentTables ищет таблицы с классами, характерными для реестра резидентов.
func findResidentTables(doc *html.Node) []*html.Node {
	var tables []*html.Node

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			class := strings.ToLower(attrVal(n, "class"))
			if strings.Contains(class, "resident") ||
				strings.Contains(class, "registry") ||
				strings.Contains(class, "reestr") {
				tables = append(tables, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return tables
}

// findAllTables ищет все <table> элементы.
func findAllTables(doc *html.Node) []*html.Node {
	var tables []*html.Node

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			tables = append(tables, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return tables
}

// parseTableRows парсит строки таблицы, пропуская заголовок, и извлекает резидентов.
func parseTableRows(table *html.Node, sourceURL string) []*model.Resident {
	var residents []*model.Resident

	// Ищем <tr> внутри <tbody> или напрямую в <table>.
	var rows []*html.Node

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tbody" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "tr" {
					rows = append(rows, c)
				}
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)

	// Если tbody не найден, берём все <tr> из таблицы.
	if len(rows) == 0 {
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "tr" {
				rows = append(rows, n)
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(table)
	}

	// Пропускаем первую строку (заголовок), если она содержит типичные заголовочные теги.
	startIdx := 0
	if len(rows) > 0 && isHeaderRow(rows[0]) {
		startIdx = 1
	}

	for i := startIdx; i < len(rows); i++ {
		r := parseResidentRow(rows[i], sourceURL)
		if r != nil && r.Name != "" {
			residents = append(residents, r)
		}
	}

	return residents
}

// isHeaderRow проверяет, является ли строка заголовком таблицы.
func isHeaderRow(tr *html.Node) bool {
	var walk func(*html.Node)
	var hasTh bool
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "th" {
			hasTh = true
			return
		}
		// Проверяем класс строки.
		if n.Type == html.ElementNode && n.Data == "tr" {
			class := strings.ToLower(attrVal(n, "class"))
			if strings.Contains(class, "header") || strings.Contains(class, "head") {
				hasTh = true
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(tr)
	return hasTh
}

// parseResidentRow парсит одну строку таблицы резидентов.
// Ожидает колонки: название, ИНН, отрасль, дата вступления, статус.
// Если колонка не найдена — поле остаётся пустым.
func parseResidentRow(tr *html.Node, sourceURL string) *model.Resident {
	// Собираем тексты всех <td> ячеек.
	var cells []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "td" {
			cells = append(cells, nodeText(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(tr)

	if len(cells) == 0 {
		return nil
	}

	r := &model.Resident{
		SourceURL: sourceURL,
		Status:    model.ResidentActive,
	}

	// Ищем ссылку с названием компании внутри строки.
	r.Name = extractResidentName(tr)
	r.ID = residentID(r.Name, sourceURL)

	if r.Name == "" && len(cells) > 0 {
		r.Name = strings.TrimSpace(cells[0])
		r.ID = residentID(r.Name, sourceURL)
	}

	// Распределяем ячейки по полям.
	// Стратегия: пробуем распознать каждую ячейку по содержимому.
	innFound := false
	dateFound := false
	statusFound := false

	for _, cell := range cells {
		text := strings.TrimSpace(cell)
		if text == "" {
			continue
		}

		// ИНН: 10 или 12 цифр.
		if !innFound && isINN(text) {
			r.INN = extractINN(text)
			if r.INN != "" {
				innFound = true
			}
			continue
		}

		// Дата вступления.
		if !dateFound {
			if d := parseDate(text); !d.IsZero() {
				r.JoinDate = d
				dateFound = true
				continue
			}
		}

		// Статус: ключевые слова.
		if !statusFound {
			lower := strings.ToLower(text)
			if isStatusText(lower) {
				r.Status = classifyStatus(lower)
				statusFound = true
				continue
			}
		}
	}

	// Если отрасль не определена отдельно — берём оставшиеся ячейки.
	if len(cells) >= 2 {
		// Пытаемся определить отрасль из ячеек, которые не распознаны.
		for _, cell := range cells {
			text := strings.TrimSpace(cell)
			if text == "" || text == r.Name {
				continue
			}
			if isINN(text) {
				continue
			}
			if parseDate(text).IsZero() && !isStatusText(strings.ToLower(text)) {
				r.Industry = text
				break
			}
		}
	}

	return r
}

// extractResidentName извлекает название компании из ссылки внутри строки.
func extractResidentName(tr *html.Node) string {
	var name string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if name != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			title := strings.TrimSpace(nodeText(n))
			if title != "" {
				name = title
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(tr)
	return name
}

// findResidentCards ищет карточки резидентов (fallback).
func findResidentCards(doc *html.Node, sourceURL string) []*model.Resident {
	var residents []*model.Resident

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := strings.ToLower(attrVal(n, "class"))
			if strings.Contains(class, "resident") || strings.Contains(class, "company") {
				if r := extractResidentCard(n, sourceURL); r != nil && r.Name != "" {
					residents = append(residents, r)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return residents
}

// extractResidentCard извлекает данные резидента из карточки.
func extractResidentCard(n *html.Node, sourceURL string) *model.Resident {
	text := nodeText(n)
	if text == "" {
		return nil
	}

	r := &model.Resident{SourceURL: sourceURL, Status: model.ResidentActive}

	// Ищем название из ссылки.
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			title := strings.TrimSpace(nodeText(node))
			if title != "" && r.Name == "" {
				r.Name = title
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if r.Name == "" {
		// Берём первую строку текста.
		lines := strings.SplitN(strings.TrimSpace(text), "\n", 2)
		if len(lines) > 0 {
			r.Name = strings.TrimSpace(lines[0])
		}
	}

	r.ID = residentID(r.Name, sourceURL)

	// Ищем ИНН.
	if m := innRegex.FindString(text); m != "" {
		r.INN = m
	}

	// Ищем дату.
	if d := parseDate(text); !d.IsZero() {
		r.JoinDate = d
	}

	// Определяем статус.
	lower := strings.ToLower(text)
	r.Status = classifyStatus(lower)

	return r
}

// innRegex — регулярное выражение для ИНН (10 или 12 цифр).
var innRegex = regexp.MustCompile(`\b(\d{10}|\d{12})\b`)

// isINN проверяет, является ли текст ИНН.
func isINN(text string) bool {
	cleaned := strings.ReplaceAll(text, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	return innRegex.MatchString(cleaned)
}

// extractINN извлекает ИНН из текста.
func extractINN(text string) string {
	cleaned := strings.ReplaceAll(text, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	if m := innRegex.FindString(cleaned); m != "" {
		return m
	}
	return ""
}

// isStatusText проверяет, является ли текст указанием статуса.
func isStatusText(lower string) bool {
	statusKeywords := []string{
		"действующ", "активн", "в реестре",
		"недействующ", "исключён", "исключен", "ликвидирован",
		"прекращен", "прекращён", "закрыт", "архив",
	}
	for _, kw := range statusKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// classifyStatus определяет статус резидента по тексту.
func classifyStatus(lower string) model.ResidentStatus {
	inactiveKeywords := []string{
		"недействующ", "исключён", "исключен", "ликвидирован",
		"прекращен", "прекращён", "закрыт", "архив",
	}
	for _, kw := range inactiveKeywords {
		if strings.Contains(lower, kw) {
			return model.ResidentInactive
		}
	}
	return model.ResidentActive
}

// parseDate разбирает строку в дату.
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
	}

	for _, layout := range formats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}

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

// IngestResidents записывает резидентов в хранилище.
func IngestResidents(ctx context.Context, residents []*model.Resident, st store.ResidentStore) (*Result, error) {
	res := &Result{}

	for _, r := range residents {
		if r.Name == "" {
			res.Errors = append(res.Errors, "пропущено: пустое имя")
			continue
		}

		// Ищем существующего резидента по INN или по Name.
		existing, err := findExistingResident(ctx, r, st)
		if err == nil && existing != nil {
			// Обновляем.
			r.ID = existing.ID
			r.CreatedAt = existing.CreatedAt
			if err := st.UpdateResident(ctx, r); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("обновление %s: %v", r.ID, err))
				continue
			}
			res.Updated++
		} else {
			// Создаём нового.
			r.CreatedAt = time.Now()
			if err := st.CreateResident(ctx, r); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("создание %s: %v", r.ID, err))
				continue
			}
			res.New++
		}
	}

	return res, nil
}

// findExistingResident ищет существующего резидента по INN или Name.
func findExistingResident(ctx context.Context, r *model.Resident, st store.ResidentStore) (*model.Resident, error) {
	// Ищем по INN.
	if r.INN != "" {
		results, err := st.ListResidents(ctx, "", "", r.INN)
		if err == nil && len(results) > 0 {
			return results[0], nil
		}
	}

	// Ищем по Name.
	if r.Name != "" {
		results, err := st.ListResidents(ctx, "", "", r.Name)
		if err == nil && len(results) > 0 {
			return results[0], nil
		}
	}

	// Если есть ID — пробуем прямой поиск.
	if r.ID != "" {
		return st.GetResident(ctx, r.ID)
	}

	return nil, fmt.Errorf("не найден")
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func residentID(name, sourceURL string) string {
	input := name + "|" + sourceURL
	sum := sha1.Sum([]byte(input))
	return "resident-" + hex.EncodeToString(sum[:])[:16]
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
