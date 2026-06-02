package sitepages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/qdrant"
)

const embedBatch = 16

// pageNamespace — стабильный namespace для детерминированных UUID точек Qdrant.
// Qdrant требует UUID/uint64 в качестве ID точки, поэтому sha1-ID страницы
// в БД и UUID-ID точки в Qdrant выводятся из одного URL, но по-разному.
var pageNamespace = uuid.NewSHA1(uuid.Nil, []byte("baza-skolkovo/sitepages"))

// pointID — детерминированный UUID точки Qdrant по URL страницы.
func pointID(url string) string {
	return uuid.NewSHA1(pageNamespace, []byte(url)).String()
}

// Indexer индексирует страницы сайта в отдельную Qdrant-коллекцию.
type Indexer struct {
	Qdr *qdrant.Client // отдельная коллекция страниц (не общая с документами)
	Emb embed.Embedder
	Dim int
}

// NewIndexer создаёт индексатор страниц сайта.
func NewIndexer(qdr *qdrant.Client, emb embed.Embedder, dim int) *Indexer {
	return &Indexer{Qdr: qdr, Emb: emb, Dim: dim}
}

// Reindex гарантирует коллекцию и (пере)индексирует переданные страницы: один
// вектор на страницу (title + summary). ID точек детерминированы, поэтому
// повторная индексация перезаписывает, а не дублирует. Для полной пересборки
// передают все страницы, для инкрементальной — только изменённые.
func (ix *Indexer) Reindex(ctx context.Context, pages []*Page) (int, error) {
	if err := ix.Qdr.EnsureCollection(ctx, ix.Dim); err != nil {
		return 0, fmt.Errorf("создание коллекции страниц: %w", err)
	}
	if len(pages) == 0 {
		return 0, nil
	}
	// Недоступные (gone) страницы не индексируем и удаляем их точки из коллекции,
	// чтобы поиск не отдавал мёртвые страницы.
	var active []*Page
	var deleteIDs []string
	for _, p := range pages {
		if p.Status == StatusGone {
			deleteIDs = append(deleteIDs, pointID(p.URL))
			continue
		}
		active = append(active, p)
	}
	if err := ix.Qdr.Delete(ctx, deleteIDs); err != nil {
		return 0, fmt.Errorf("удаление недоступных страниц из Qdrant: %w", err)
	}
	pages = active
	if len(pages) == 0 {
		return 0, nil
	}
	var points []qdrant.Point
	for start := 0; start < len(pages); start += embedBatch {
		end := min(start+embedBatch, len(pages))
		batch := pages[start:end]
		inputs := make([]string, len(batch))
		for i, p := range batch {
			inputs[i] = embed.PrefixPassage + indexText(p)
		}
		vecs, err := ix.Emb.Embed(ctx, inputs)
		if err != nil {
			return 0, fmt.Errorf("эмбеддинги страниц: %w", err)
		}
		for i, v := range vecs {
			p := batch[i]
			summary := p.AISummary
			if summary == "" {
				summary = p.Summary
			}
			payload := map[string]any{
				"entity_type":  "sitepage",
				"url":          p.URL,
				"title":        p.Title,
				"summary":      summary,
				"section":      p.Section,
				"status":       p.Status,
				"category":     p.Category,
				"subcategory":  p.Subcategory,
				"tags":         p.Tags,
				"goals":        p.Goals,
				"theses":       p.Theses,
				"conclusions":  p.Conclusions,
				"last_changed": p.LastChanged.Format(time.RFC3339),
			}
			if p.PublishedAt != nil {
				payload["published_at"] = p.PublishedAt.Format("2006-01-02")
			}
			points = append(points, qdrant.Point{ID: pointID(p.URL), Vector: v, Payload: payload})
		}
	}
	if err := ix.Qdr.Upsert(ctx, points); err != nil {
		return 0, fmt.Errorf("upsert страниц в Qdrant: %w", err)
	}
	return len(points), nil
}

// Searcher выполняет семантический поиск по коллекции страниц.
type Searcher struct {
	Qdr *qdrant.Client
	Emb embed.Embedder
}

// NewSearcher создаёт поисковик по страницам сайта.
func NewSearcher(qdr *qdrant.Client, emb embed.Embedder) *Searcher {
	return &Searcher{Qdr: qdr, Emb: emb}
}

// Hit — результат поиска по страницам сайта.
type Hit struct {
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Section   string   `json:"section"`
	Tags      []string `json:"tags,omitempty"`
	Published string   `json:"published,omitempty"` // дата публикации на сайте (ГГГГ-ММ-ДД)
	Score     float32  `json:"score"`
}

// Search ищет наиболее релевантные страницы под запрос пользователя.
func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	return s.SearchWithTags(ctx, query, limit, nil)
}

// SearchWithTags ищет страницы под запрос с необязательным фильтром по тегам
// (страница должна содержать ВСЕ указанные теги).
func (s *Searcher) SearchWithTags(ctx context.Context, query string, limit int, tags []string) ([]Hit, error) {
	if limit <= 0 {
		limit = 5
	}
	vecs, err := s.Emb.Embed(ctx, []string{embed.PrefixQuery + query})
	if err != nil {
		return nil, fmt.Errorf("эмбеддинг запроса страниц: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("пустой эмбеддинг запроса")
	}
	hits, err := s.Qdr.Search(ctx, vecs[0], limit, searchFilter(tags))
	if err != nil {
		return nil, err
	}
	return toHits(hits), nil
}

// Related находит семантически близкие страницы к данной (kNN по той же
// коллекции), исключая саму страницу. Питает блок «связанные страницы».
func (s *Searcher) Related(ctx context.Context, p *Page, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 6
	}
	vecs, err := s.Emb.Embed(ctx, []string{embed.PrefixQuery + indexText(p)})
	if err != nil {
		return nil, fmt.Errorf("эмбеддинг страницы: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("пустой эмбеддинг страницы")
	}
	// Берём на один больше и исключаем саму страницу (по URL); только доступные.
	filter := map[string]any{
		"must": []any{
			map[string]any{"key": "status", "match": map[string]any{"value": StatusActive}},
		},
		"must_not": []any{
			map[string]any{"key": "url", "match": map[string]any{"value": p.URL}},
		},
	}
	hits, err := s.Qdr.Search(ctx, vecs[0], limit+1, filter)
	if err != nil {
		return nil, err
	}
	out := toHits(hits)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// searchFilter строит фильтр Qdrant: только доступные страницы (status=active)
// и, если заданы теги, страница содержит ВСЕ указанные теги (AND).
func searchFilter(tags []string) map[string]any {
	must := []any{
		map[string]any{"key": "status", "match": map[string]any{"value": StatusActive}},
	}
	for _, t := range tags {
		if t == "" {
			continue
		}
		must = append(must, map[string]any{"key": "tags", "match": map[string]any{"value": t}})
	}
	return map[string]any{"must": must}
}

func toHits(hits []qdrant.SearchHit) []Hit {
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{
			URL:       str(h.Payload["url"]),
			Title:     str(h.Payload["title"]),
			Summary:   str(h.Payload["summary"]),
			Section:   str(h.Payload["section"]),
			Tags:      strSlice(h.Payload["tags"]),
			Published: str(h.Payload["published_at"]),
			Score:     h.Score,
		})
	}
	return out
}

// indexText собирает обогащённый текст страницы для эмбеддинга: заголовок +
// краткое описание (ИИ, иначе meta) + цели + тезисы + выводы. Если страница ещё
// не аннотирована — деградирует до title + summary.
func indexText(p *Page) string {
	parts := make([]string, 0, 6)
	if p.Title != "" {
		parts = append(parts, p.Title)
	}
	if p.Category != "" {
		cat := p.Category
		if p.Subcategory != "" {
			cat += " / " + p.Subcategory
		}
		parts = append(parts, cat)
	}
	if p.AISummary != "" {
		parts = append(parts, p.AISummary)
	} else if p.Summary != "" {
		parts = append(parts, p.Summary)
	}
	if p.Goals != "" {
		parts = append(parts, p.Goals)
	}
	if len(p.Theses) > 0 {
		parts = append(parts, strings.Join(p.Theses, ". "))
	}
	if p.Conclusions != "" {
		parts = append(parts, p.Conclusions)
	}
	if len(parts) == 0 {
		return p.URL
	}
	return strings.Join(parts, "\n")
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// strSlice конвертирует payload-значение (которое из JSON приходит как []any)
// в []string, отбрасывая не-строки.
func strSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
