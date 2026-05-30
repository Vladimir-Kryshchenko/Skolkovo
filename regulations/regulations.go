// Package regulations парсит нормативно-правовые акты (НПА), регулирующие
// деятельность Сколково: 244-ФЗ и поправки, приказы Минэкономразвития,
// постановления Правительства. Источники: regulation.gov.ru и consultant.plus RSS.
package regulations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
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
	rag "baza-skolkovo/src/rag_service"
)

// RegulationsConfig — конфигурация источника НПА.
type RegulationsConfig struct {
	SearchURL   string   // URL поиска на regulation.gov.ru
	ExtraURLs   []string // дополнительные прямые ссылки на страницы НПА
	SearchQuery string   // ключевые слова для поиска (по умолчанию «Сколково»)
	Category    string   // категория документов (по умолчанию «НПА»)
	MaxResults  int      // максимальное число документов за запрос (0 = 50)
}

// Monitor синхронизирует НПА в хранилище.
type Monitor struct {
	Cfg     RegulationsConfig
	St      store.Store
	Rag     *rag.Service
	Changes changes.Recorder // лента изменений; может быть nil
	HTTP    *http.Client
}

// Result — итог синхронизации НПА.
type Result struct {
	Fetched int
	New     int
	Updated int
	Errors  []string
}

// NPADocument — нормативно-правовой акт.
type NPADocument struct {
	ExternalID  string    // идентификатор в системе regulation.gov.ru
	Title       string    // название НПА
	Number      string    // номер (например «244-ФЗ»)
	Type        string    // вид НПА: закон, постановление, приказ
	IssuedBy    string    // орган-издатель
	IssuedAt    time.Time // дата принятия
	EffectiveAt time.Time // дата вступления в силу
	SourceURL   string    // ссылка на страницу НПА
	Summary     string    // краткое содержание
	Status      string    // "active" | "amended" | "revoked"
}

// NewMonitor создаёт монитор НПА.
func NewMonitor(cfg RegulationsConfig, st store.Store, ragSvc *rag.Service) *Monitor {
	if cfg.Category == "" {
		cfg.Category = "НПА"
	}
	if cfg.SearchQuery == "" {
		cfg.SearchQuery = "Сколково"
	}
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 50
	}
	return &Monitor{
		Cfg:  cfg,
		St:   st,
		Rag:  ragSvc,
		HTTP: &http.Client{Timeout: 30 * time.Second},
	}
}

// Run запускает синхронизацию НПА.
func (m *Monitor) Run(ctx context.Context) (*Result, error) {
	docs, err := ParseRegulations(ctx, m.Cfg, m.HTTP)
	if err != nil {
		return nil, fmt.Errorf("парсинг НПА: %w", err)
	}
	return IngestRegulations(ctx, docs, m.St, m.Rag, m.Changes)
}

// ParseRegulations загружает НПА из всех источников.
func ParseRegulations(ctx context.Context, cfg RegulationsConfig, hc *http.Client) ([]*model.Document, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	category := cfg.Category
	if category == "" {
		category = "НПА"
	}

	var allDocs []*model.Document

	// 1. Поиск на regulation.gov.ru
	if docs, err := searchRegulationGov(ctx, cfg, hc, category); err == nil {
		allDocs = append(allDocs, docs...)
	}

	// 2. RSS-лента Консультант+ по Сколково
	if docs, err := fetchConsultantPlusRSS(ctx, cfg.SearchQuery, hc, category); err == nil {
		allDocs = append(allDocs, docs...)
	}

	// 3. Прямые ссылки на конкретные НПА
	for _, u := range cfg.ExtraURLs {
		if doc, err := fetchNPAPage(ctx, u, category, hc); err == nil && doc != nil {
			allDocs = append(allDocs, doc)
		}
	}

	// 4. Базовые НПА всегда в базе (244-ФЗ и ключевые постановления)
	allDocs = append(allDocs, coreNPA(category)...)

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

// searchRegulationGov ищет НПА на regulation.gov.ru.
func searchRegulationGov(ctx context.Context, cfg RegulationsConfig, hc *http.Client, category string) ([]*model.Document, error) {
	searchURL := cfg.SearchURL
	if searchURL == "" {
		searchURL = "https://regulation.gov.ru/Regulation/Npa/Search"
	}

	q := url.Values{}
	q.Set("npaName", cfg.SearchQuery)
	q.Set("type", "")
	q.Set("StatusID", "5") // 5 = действующие
	reqURL := searchURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", regulationsUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("regulation.gov.ru: статус %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseRegulationSearchResults(body, reqURL, category)
}

// parseRegulationSearchResults извлекает НПА из страницы поиска regulation.gov.ru.
func parseRegulationSearchResults(body []byte, pageURL, category string) ([]*model.Document, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	var docs []*model.Document
	now := time.Now()

	// Ищем карточки НПА в таблице результатов.
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Строки таблицы результатов или карточки.
			if n.Data == "tr" || (n.Data == "div" && hasClass(n, "npa-item")) {
				title, link, npaType := extractNPAFromRow(n)
				if title != "" && isSkolkovoRelated(title) {
					id := npaID(link)
					if link == "" {
						id = npaID(title)
					}
					docs = append(docs, &model.Document{
						ID:        id,
						Title:     title,
						SourceURL: resolveURL(pageURL, link),
						FileHash:  contentHashReg(title + npaType),
						FetchedAt: now,
						Status:    model.StatusActive,
						Category:  category,
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return docs, nil
}

// rssItem — элемент RSS-ленты.
type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"`
	Desc    string `xml:"description"`
}

// rssFeed — RSS-лента.
type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

// fetchConsultantPlusRSS получает НПА из RSS Консультант+.
func fetchConsultantPlusRSS(ctx context.Context, query string, hc *http.Client, category string) ([]*model.Document, error) {
	// Консультант+ не имеет открытого RSS для поиска, поэтому используем
	// Гарант-парсер или публичные RSS по теме.
	rssURL := "https://www.garant.ru/rss/hotnews.xml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", regulationsUserAgent)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSS НПА: статус %d", resp.StatusCode)
	}

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}

	var docs []*model.Document
	now := time.Now()

	for _, item := range feed.Items {
		if !isSkolkovoRelated(item.Title) && !isSkolkovoRelated(item.Desc) {
			continue
		}
		docs = append(docs, &model.Document{
			ID:        npaID(item.Link),
			Title:     item.Title,
			SourceURL: item.Link,
			FileHash:  contentHashReg(item.Title + item.Desc),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		})
	}

	return docs, nil
}

// fetchNPAPage загружает страницу конкретного НПА.
func fetchNPAPage(ctx context.Context, rawURL, category string, hc *http.Client) (*model.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", regulationsUserAgent)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("НПА страница %s: статус %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	title := extractHTMLTitle(doc)
	if title == "" {
		title = rawURL
	}
	mainText := extractMainContentText(doc)

	return &model.Document{
		ID:        npaID(rawURL),
		Title:     title,
		SourceURL: rawURL,
		FileHash:  contentHashReg(mainText),
		FetchedAt: time.Now(),
		Status:    model.StatusActive,
		Category:  category,
	}, nil
}

// coreNPA возвращает ключевые НПА по Сколково как статичные документы.
// Используется как постоянная база — эти акты не исчезнут из базы при недоступности сайтов.
func coreNPA(category string) []*model.Document {
	now := time.Now()
	type npaEntry struct {
		id      string
		title   string
		url     string
		summary string
	}

	entries := []npaEntry{
		{
			id:    "244-fz-2010",
			title: "Федеральный закон № 244-ФЗ «Об инновационном центре «Сколково»",
			url:   "http://www.consultant.ru/document/cons_doc_LAW_105168/",
			summary: "Основополагающий закон об инновационном центре Сколково. Устанавливает " +
				"правовое положение ИЦ «Сколково», порядок получения и утраты статуса резидента, " +
				"льготы и преференции для резидентов: 0% налог на прибыль, пониженные страховые взносы, " +
				"освобождение от уплаты НДС на НИОКР. Принят 28.09.2010.",
		},
		{
			id:    "rusp-1574-2010",
			title: "Постановление Правительства РФ № 1574 о порядке ведения реестра резидентов Сколково",
			url:   "https://base.garant.ru/12183030/",
			summary: "Устанавливает порядок включения в реестр участников проекта «Сколково», " +
				"форму реестра и порядок исключения из него. Принято Правительством РФ.",
		},
		{
			id:    "fz-244-art246-1-nkrf",
			title: "Статья 246.1 НК РФ — Освобождение от обязанностей плательщика налога на прибыль",
			url:   "http://www.consultant.ru/document/cons_doc_LAW_28165/",
			summary: "Участники проекта «Сколково» освобождаются от обязанностей налогоплательщика " +
				"налога на прибыль организаций в течение 10 лет с момента получения статуса участника. " +
				"Условие: совокупный объём прибыли не превышает 300 млн рублей.",
		},
		{
			id:    "nkrf-art427-insurance-skolkovo",
			title: "Статья 427 НК РФ — Пониженные тарифы страховых взносов для участников Сколково",
			url:   "http://www.consultant.ru/document/cons_doc_LAW_28165/",
			summary: "Участники проекта «Сколково» применяют пониженный тариф страховых взносов: " +
				"ОПС — 14%, ОСС — 0%, ОМС — 0%. Применяется в течение 10 лет с момента получения статуса.",
		},
		{
			id:    "nkrf-art149-p3-16-vat",
			title: "Статья 149 НК РФ п.3 пп.16 — Освобождение от НДС НИОКР по Сколково",
			url:   "http://www.consultant.ru/document/cons_doc_LAW_28165/",
			summary: "Выполнение НИОКР организациями-участниками проекта «Сколково» не подлежит " +
				"налогообложению НДС. Льгота распространяется на услуги в сфере НИОКР.",
		},
		{
			id:    "nkrf-art381-property-tax",
			title: "Статья 381 НК РФ — Льготы по налогу на имущество для участников Сколково",
			url:   "http://www.consultant.ru/document/cons_doc_LAW_28165/",
			summary: "Организации — участники проекта «Сколково» освобождены от налога на имущество " +
				"организаций в отношении имущества, используемого для осуществления деятельности, " +
				"предусмотренной 244-ФЗ.",
		},
		{
			id:    "nkrf-art395-land-tax",
			title: "Статья 395 НК РФ — Освобождение от земельного налога для Сколково",
			url:   "http://www.consultant.ru/document/cons_doc_LAW_28165/",
			summary: "Управляющие компании «Сколково», фонды и резиденты освобождены от уплаты " +
				"земельного налога в отношении земельных участков ИЦ «Сколково».",
		},
		{
			id:    "eec-decision-130-customs",
			title: "Решение ЕЭК № 130 — Таможенные льготы для Сколково",
			url:   "https://www.alta.ru/tamdoc/12bn0130/",
			summary: "Устанавливает льготы по таможенным пошлинам при ввозе товаров, " +
				"необходимых для осуществления деятельности в ИЦ «Сколково».",
		},
		{
			id:    "244-fz-amendment-2022",
			title: "Изменения в 244-ФЗ 2022 года — расширение льгот для резидентов Сколково",
			url:   "https://sk.ru/foundation/documents/",
			summary: "Поправки к 244-ФЗ, расширяющие программу льгот и уточняющие порядок " +
				"применения налоговых преференций для резидентов Сколково.",
		},
	}

	docs := make([]*model.Document, 0, len(entries))
	for _, e := range entries {
		docs = append(docs, &model.Document{
			ID:        npaID(e.id),
			Title:     e.title,
			SourceURL: e.url,
			FileHash:  contentHashReg(e.summary),
			FetchedAt: now,
			Status:    model.StatusActive,
			Category:  category,
		})
	}
	return docs
}

// IngestRegulations записывает НПА в Store и индексирует в RAG.
func IngestRegulations(ctx context.Context, docs []*model.Document, st store.Store, ragSvc *rag.Service, recs ...changes.Recorder) (*Result, error) {
	res := &Result{Fetched: len(docs)}
	for _, doc := range docs {
		if doc.Title == "" || doc.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой заголовок или URL")
			continue
		}
		isNew := false
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

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

const regulationsUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

func npaID(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "npa-" + hex.EncodeToString(sum[:])[:16]
}

func contentHashReg(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func isSkolkovoRelated(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "сколково") ||
		strings.Contains(lower, "skolkovo") ||
		strings.Contains(lower, "244-фз") ||
		strings.Contains(lower, "244-fz") ||
		strings.Contains(lower, "инновационн") && strings.Contains(lower, "центр")
}

func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" && strings.Contains(a.Val, class) {
			return true
		}
	}
	return false
}

func extractNPAFromRow(n *html.Node) (title, link, npaType string) {
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			for _, a := range node.Attr {
				if a.Key == "href" {
					link = a.Val
				}
			}
			t := nodeTextReg(node)
			if title == "" && t != "" {
				title = t
			}
		}
		if node.Type == html.ElementNode && (node.Data == "td" || node.Data == "span") {
			t := nodeTextReg(node)
			if isNPAType(t) && npaType == "" {
				npaType = t
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return
}

func isNPAType(s string) bool {
	lower := strings.ToLower(s)
	return containsAnyReg(lower, "федеральный закон", "постановление", "приказ",
		"распоряжение", "указ", "решение")
}

func containsAnyReg(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func nodeTextReg(n *html.Node) string {
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

func extractHTMLTitle(doc *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			title = strings.TrimSpace(nodeTextReg(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title
}

func extractMainContentText(doc *html.Node) string {
	for _, tag := range []string{"main", "article", "body"} {
		if node := findNodeByTag(doc, tag); node != nil {
			t := strings.TrimSpace(nodeTextReg(node))
			if t != "" {
				if len(t) > 5000 {
					t = t[:5000]
				}
				return t
			}
		}
	}
	return ""
}

func findNodeByTag(doc *html.Node, tag string) *html.Node {
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

func resolveURL(base, href string) string {
	if href == "" {
		return base
	}
	if strings.HasPrefix(href, "http") {
		return href
	}
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	r, err := url.Parse(href)
	if err != nil {
		return href
	}
	return b.ResolveReference(r).String()
}
