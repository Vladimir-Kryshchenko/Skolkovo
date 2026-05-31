package navindex

import (
	"context"
	"fmt"

	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/qdrant"
)

const embedBatch = 16

// Indexer индексирует навигационную карту в отдельную Qdrant-коллекцию.
type Indexer struct {
	Qdr *qdrant.Client // отдельная коллекция навигации (не общая с документами)
	Emb embed.Embedder
	Dim int
}

// NewIndexer создаёт индексатор навигации.
func NewIndexer(qdr *qdrant.Client, emb embed.Embedder, dim int) *Indexer {
	return &Indexer{Qdr: qdr, Emb: emb, Dim: dim}
}

// Reindex полностью пересобирает навигационный индекс из Tree(): гарантирует
// коллекцию, считает эмбеддинги всех узлов и перезаписывает точки (ID
// детерминированы, поэтому устаревшие узлы перезаписываются, а не дублируются).
// Возвращает число проиндексированных узлов.
func (ix *Indexer) Reindex(ctx context.Context) (int, error) {
	if err := ix.Qdr.EnsureCollection(ctx, ix.Dim); err != nil {
		return 0, fmt.Errorf("создание коллекции навигации: %w", err)
	}
	nodes := Flatten(Tree())
	if len(nodes) == 0 {
		return 0, nil
	}
	var points []qdrant.Point
	for start := 0; start < len(nodes); start += embedBatch {
		end := min(start+embedBatch, len(nodes))
		batch := nodes[start:end]
		inputs := make([]string, len(batch))
		for i, n := range batch {
			inputs[i] = embed.PrefixPassage + n.Text
		}
		vecs, err := ix.Emb.Embed(ctx, inputs)
		if err != nil {
			return 0, fmt.Errorf("эмбеддинги навигации: %w", err)
		}
		for i, v := range vecs {
			n := batch[i]
			points = append(points, qdrant.Point{
				ID:     n.ID,
				Vector: v,
				Payload: map[string]any{
					"entity_type": "navigation",
					"interface":   n.Interface,
					"port":        n.Port,
					"audience":    n.Audience,
					"route":       n.Route,
					"page_title":  n.PageTitle,
					"block":       n.Block,
					"kind":        n.Kind,
					"howto":       n.HowTo,
					"text":        n.Text,
				},
			})
		}
	}
	if err := ix.Qdr.Upsert(ctx, points); err != nil {
		return 0, fmt.Errorf("upsert навигации в Qdrant: %w", err)
	}
	return len(points), nil
}

// Searcher выполняет семантический поиск по навигационной коллекции.
type Searcher struct {
	Qdr *qdrant.Client
	Emb embed.Embedder
}

// NewSearcher создаёт поисковик по навигации.
func NewSearcher(qdr *qdrant.Client, emb embed.Embedder) *Searcher {
	return &Searcher{Qdr: qdr, Emb: emb}
}

// Hit — результат навигационного поиска: «где это на сайте и как туда попасть».
type Hit struct {
	Interface string  `json:"interface"`
	Port      string  `json:"port"`
	Audience  string  `json:"audience"`
	Route     string  `json:"route"`
	PageTitle string  `json:"page_title"`
	Block     string  `json:"block,omitempty"`
	Kind      string  `json:"kind"`
	HowTo     string  `json:"howto"`
	Score     float32 `json:"score"`
}

// Search ищет наиболее релевантные элементы навигации под запрос пользователя.
func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 5
	}
	vecs, err := s.Emb.Embed(ctx, []string{embed.PrefixQuery + query})
	if err != nil {
		return nil, fmt.Errorf("эмбеддинг запроса навигации: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("пустой эмбеддинг запроса")
	}
	hits, err := s.Qdr.Search(ctx, vecs[0], limit, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{
			Interface: str(h.Payload["interface"]),
			Port:      str(h.Payload["port"]),
			Audience:  str(h.Payload["audience"]),
			Route:     str(h.Payload["route"]),
			PageTitle: str(h.Payload["page_title"]),
			Block:     str(h.Payload["block"]),
			Kind:      str(h.Payload["kind"]),
			HowTo:     str(h.Payload["howto"]),
			Score:     h.Score,
		})
	}
	return out, nil
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
