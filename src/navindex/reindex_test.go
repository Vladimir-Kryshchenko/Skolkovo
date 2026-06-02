package navindex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"baza-skolkovo/src/common/qdrant"
)

// fakeEmbedder возвращает детерминированный вектор фиксированной размерности —
// эмбеддинги TEI в тесте не нужны, проверяем только маршрутизацию узлов в Qdrant.
type fakeEmbedder struct{ dim int }

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, f.dim)
		for j := range v {
			v[j] = 0.1
		}
		out[i] = v
	}
	return out, nil
}

// fakeQdrant — минимальная in-memory реализация REST API Qdrant, достаточная для
// EnsureCollection / Upsert / Search. Позволяет протестировать Reindex и Search
// без реального Qdrant и проверить маппинг payload (route/block/howto).
type fakeQdrant struct {
	mu      sync.Mutex
	created bool
	size    int
	points  []qdrant.Point
}

func (f *fakeQdrant) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Поиск ближайших точек.
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/points/search"):
			var req struct {
				Limit int `json:"limit"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			limit := req.Limit
			if limit <= 0 || limit > len(f.points) {
				limit = len(f.points)
			}
			hits := make([]map[string]any, 0, limit)
			for i := 0; i < limit; i++ {
				hits = append(hits, map[string]any{
					"id":      f.points[i].ID,
					"score":   1.0,
					"payload": f.points[i].Payload,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": hits})

		// Upsert точек.
		case r.Method == http.MethodPut && strings.Contains(path, "/points"):
			var req struct {
				Points []qdrant.Point `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f.points = append(f.points, req.Points...)
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": "completed"}})

		// Создание коллекции.
		case r.Method == http.MethodPut:
			var req struct {
				Vectors struct {
					Size int `json:"size"`
				} `json:"vectors"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.created = true
			f.size = req.Vectors.Size
			_ = json.NewEncoder(w).Encode(map[string]any{"result": true, "status": "ok"})

		// Существование/информация о коллекции.
		case r.Method == http.MethodGet:
			if !f.created {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"config": map[string]any{
						"params": map[string]any{
							"vectors": map[string]any{"size": f.size},
						},
					},
				},
			})

		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}
}

// TestReindexAndSearch проверяет сквозной путь навигации: Reindex кладёт в Qdrant
// по узлу на страницу и блок с осмысленным payload (route/howto/block/text), а
// Search читает их обратно в Hit. Ловит регрессы в формировании точек и маппинге.
func TestReindexAndSearch(t *testing.T) {
	const dim = 8
	fake := &fakeQdrant{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	qdr := qdrant.New(srv.URL, "skolkovo_navigation_test")
	ix := NewIndexer(qdr, fakeEmbedder{dim: dim}, dim)

	ctx := context.Background()
	wantNodes := len(Flatten(Tree()))

	n, err := ix.Reindex(ctx)
	if err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if n != wantNodes {
		t.Errorf("Reindex проиндексировал %d узлов, ожидалось %d", n, wantNodes)
	}
	if !fake.created {
		t.Error("коллекция навигации не создана")
	}
	if fake.size != dim {
		t.Errorf("размерность коллекции %d, ожидалось %d", fake.size, dim)
	}
	if len(fake.points) != wantNodes {
		t.Fatalf("в Qdrant %d точек, ожидалось %d", len(fake.points), wantNodes)
	}

	// Каждая точка несёт навигационный payload и непустой текст эмбеддинга.
	seenRoute, seenHowTo := false, false
	for _, p := range fake.points {
		if p.Payload["entity_type"] != "navigation" {
			t.Errorf("точка %s без entity_type=navigation: %v", p.ID, p.Payload["entity_type"])
		}
		if s, _ := p.Payload["text"].(string); strings.TrimSpace(s) == "" {
			t.Errorf("точка %s без текста эмбеддинга", p.ID)
		}
		if s, _ := p.Payload["route"].(string); s != "" {
			seenRoute = true
		}
		if s, _ := p.Payload["howto"].(string); s != "" {
			seenHowTo = true
		}
	}
	if !seenRoute || !seenHowTo {
		t.Error("в payload точек нет route/howto — навигация не сможет ответить «где это и как попасть»")
	}

	// Search читает точки обратно и маппит payload в Hit.
	searcher := NewSearcher(qdr, fakeEmbedder{dim: dim})
	hits, err := searcher.Search(ctx, "где переключить тему", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search не вернул ни одного результата")
	}
	h := hits[0]
	if h.Interface == "" || h.Port == "" || h.Route == "" {
		t.Errorf("Hit с пустыми полями маппинга: %+v", h)
	}
	if h.HowTo == "" {
		t.Errorf("Hit без howto («как попасть»): %+v", h)
	}
}

// TestEnsureCollectionDimMismatch проверяет, что EnsureCollection через Reindex
// не молчит при несовпадении размерности уже существующей коллекции.
func TestEnsureCollectionDimMismatch(t *testing.T) {
	fake := &fakeQdrant{created: true, size: 1024} // коллекция уже есть, dim=1024
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	qdr := qdrant.New(srv.URL, "skolkovo_navigation_test")
	ix := NewIndexer(qdr, fakeEmbedder{dim: 768}, 768) // конфиг ожидает 768

	if _, err := ix.Reindex(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка рассинхрона размерности (1024 != 768), получено nil")
	}
}
