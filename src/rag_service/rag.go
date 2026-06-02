// Package rag реализует индексацию документов в векторную БД и поиск по ним.
package rag

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"

	"baza-skolkovo/src/classifier"
	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/extract"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/qdrant"
	"baza-skolkovo/src/common/store"
)

const (
	chunkSize    = 1200
	chunkOverlap = 150
	// embedBatch — число чанков в одном запросе к TEI. Малый батч (4), т.к. TEI
	// (CPU multilingual-e5) отдаёт 413, если суммарный размер батча велик — это
	// било индексацию текста крупных PDF. См. embedChunksResilient.
	embedBatch = 4
)

// Service связывает реестр документов, эмбеддинги и Qdrant.
type Service struct {
	Store      store.Store
	Qdr        *qdrant.Client
	Emb        embed.Embedder
	Dim        int
	Classifier *classifier.DocumentClassifier // опционально, для авто-классификации
}

// New создаёт RAG-сервис.
func New(st store.Store, qdr *qdrant.Client, emb embed.Embedder, dim int) *Service {
	return &Service{Store: st, Qdr: qdr, Emb: emb, Dim: dim}
}

// WithClassifier устанавливает классификатор для автоматической категрации документов.
func (s *Service) WithClassifier(cls *classifier.DocumentClassifier) *Service {
	s.Classifier = cls
	return s
}

// Init гарантирует существование коллекции Qdrant.
func (s *Service) Init(ctx context.Context) error {
	return s.Qdr.EnsureCollection(ctx, s.Dim)
}

// IndexDocument извлекает текст документа, считает эмбеддинги и загружает в Qdrant.
// Старые точки документа удаляются перед загрузкой (переиндексация).
func (s *Service) IndexDocument(ctx context.Context, docID string) (int, error) {
	doc, err := s.Store.Get(ctx, docID)
	if err != nil {
		return 0, err
	}
	// Если локальный файл есть — индексируем по содержимому.
	// Если файла нет — формируем текст из метаданных документа, чтобы он
	// участвовал в семантическом поиске хотя бы по названию и категории.
	var chunks []string
	if doc.LocalPath != "" && extract.IsSupported(doc.LocalPath) {
		text, err := extract.Text(doc.LocalPath)
		if err != nil {
			log.Printf("[rag] %s: ошибка чтения файла, индексируем по метаданным: %v", docID, err)
		} else {
			chunks = chunkText(text, chunkSize, chunkOverlap)
		}
	}
	if len(chunks) == 0 {
		// Fallback: индексируем по метаданным (название + категория + URL).
		meta := strings.TrimSpace(doc.Title)
		if doc.Category != "" {
			meta += " | " + doc.Category
		}
		if doc.SourceURL != "" {
			meta += " | " + doc.SourceURL
		}
		if meta == "" {
			return 0, fmt.Errorf("документ %s без файла и без метаданных, пропускаем", docID)
		}
		chunks = chunkText(meta, chunkSize, chunkOverlap)
	}

	// Классификация документа после chunking (если классификатор включён).
	// Передаём первый чанк как текст для классификации.
	classifyText := ""
	if len(chunks) > 0 {
		classifyText = chunks[0]
	}
	if s.Classifier != nil && doc.Category == "" {
		result, err := s.Classifier.Classify(ctx, classifyText, doc.Title)
		if err != nil {
			log.Printf("[rag:classifier] ошибка классификации документа %s: %v", docID, err)
		} else if result.Category != "" {
			// Сохраняем категорию в реестр.
			doc.Category = result.Category
			if err := s.Store.Upsert(ctx, doc); err != nil {
				log.Printf("[rag:classifier] не удалось сохранить категорию для %s: %v", docID, err)
			} else {
				log.Printf("[rag:classifier] документ %s классифицирован как %s (confidence=%.2f)", docID, result.Category, result.Confidence)
			}
		}
	}

	if err := s.Qdr.DeleteByDocument(ctx, docID); err != nil {
		return 0, fmt.Errorf("очистка старых точек: %w", err)
	}

	okChunks, vecs := s.embedChunksResilient(ctx, chunks)
	if len(okChunks) == 0 {
		return 0, fmt.Errorf("эмбеддинги: ни один чанк не удалось закодировать (%d чанков)", len(chunks))
	}
	var points []qdrant.Point
	for i, v := range vecs {
		points = append(points, qdrant.Point{
			ID:     uuid.NewString(),
			Vector: v,
			Payload: map[string]any{
				"document_id": doc.ID,
				"title":       doc.Title,
				"source_url":  doc.SourceURL,
				"category":    doc.Category,
				"status":      string(doc.Status),
				"entity_type": "document",
				"chunk_index": i,
				"text":        okChunks[i],
			},
		})
	}

	if err := s.Qdr.Upsert(ctx, points); err != nil {
		return 0, fmt.Errorf("upsert в Qdrant: %w", err)
	}
	if err := s.Store.SetIndexed(ctx, docID, true); err != nil {
		return 0, err
	}
	return len(points), nil
}

// embedChunksResilient кодирует чанки в эмбеддинги батчами по embedBatch. Если
// батч не прошёл (например, TEI вернул 413 на крупном тексте), батч дробится по
// одному чанку; одиночный непроходимый чанк пропускается, чтобы документ всё
// равно проиндексировался. Возвращает успешно закодированные чанки и их векторы
// (выровненные по индексам). Так одна проблема не теряет весь документ.
func (s *Service) embedChunksResilient(ctx context.Context, chunks []string) (okChunks []string, vecs [][]float32) {
	for start := 0; start < len(chunks); start += embedBatch {
		end := min(start+embedBatch, len(chunks))
		batch := chunks[start:end]
		inputs := make([]string, len(batch))
		for i, c := range batch {
			inputs[i] = embed.PrefixPassage + c
		}
		if bv, err := s.Emb.Embed(ctx, inputs); err == nil && len(bv) == len(batch) {
			okChunks = append(okChunks, batch...)
			vecs = append(vecs, bv...)
			continue
		}
		// Батч не прошёл — пробуем каждый чанк по отдельности, пропуская сбойные.
		for _, c := range batch {
			if one, err := s.Emb.Embed(ctx, []string{embed.PrefixPassage + c}); err == nil && len(one) == 1 {
				okChunks = append(okChunks, c)
				vecs = append(vecs, one[0])
			}
		}
	}
	return okChunks, vecs
}

// EntityDoc — произвольная сущность (мероприятие, конкурс, FAQ и т.п.) для
// индексации в общий векторный индекс наряду с документами. Status должен быть
// «действует», чтобы сущность попадала в поиск (FilterActive).
type EntityDoc struct {
	ID         string // уникальный идентификатор группы точек (например, event.ID)
	EntityType string // document | event | contest | faq | news | preference | npa
	Title      string
	SourceURL  string
	Category   string
	Status     string // «действует» — попадает в поиск; иное — отфильтровывается
	Text       string // полный текст для чанкинга
}

// IndexEntity индексирует произвольную сущность в общий векторный индекс: чанкинг,
// эмбеддинги, перезапись точек (по ID). Позволяет находить мероприятия, конкурсы,
// FAQ и т.п. семантическим поиском наравне с документами. Возвращает число чанков.
func (s *Service) IndexEntity(ctx context.Context, e EntityDoc) (int, error) {
	chunks := chunkText(e.Text, chunkSize, chunkOverlap)
	if len(chunks) == 0 {
		return 0, nil
	}
	if err := s.Qdr.DeleteByDocument(ctx, e.ID); err != nil {
		return 0, fmt.Errorf("очистка старых точек: %w", err)
	}
	var points []qdrant.Point
	for start := 0; start < len(chunks); start += embedBatch {
		end := min(start+embedBatch, len(chunks))
		batch := chunks[start:end]
		inputs := make([]string, len(batch))
		for i, c := range batch {
			inputs[i] = embed.PrefixPassage + c
		}
		vecs, err := s.Emb.Embed(ctx, inputs)
		if err != nil {
			return 0, fmt.Errorf("эмбеддинги: %w", err)
		}
		for i, v := range vecs {
			points = append(points, qdrant.Point{
				ID:     uuid.NewString(),
				Vector: v,
				Payload: map[string]any{
					"document_id": e.ID,
					"title":       e.Title,
					"source_url":  e.SourceURL,
					"category":    e.Category,
					"status":      e.Status,
					"entity_type": e.EntityType,
					"chunk_index": start + i,
					"text":        batch[i],
				},
			})
		}
	}
	if err := s.Qdr.Upsert(ctx, points); err != nil {
		return 0, fmt.Errorf("upsert в Qdrant: %w", err)
	}
	return len(points), nil
}

// IndexResult — итог пакетной индексации.
type IndexResult struct {
	Documents int
	Chunks    int
	Errors    []string
}

// IndexActive индексирует все действующие документы, ещё не проиндексированные.
// Если force=true — переиндексирует и уже проиндексированные.
func (s *Service) IndexActive(ctx context.Context, force bool) (*IndexResult, error) {
	docs, err := s.Store.List(ctx, store.Filter{Status: model.StatusActive})
	if err != nil {
		return nil, err
	}
	res := &IndexResult{}
	for _, d := range docs {
		if d.Indexed && !force {
			continue
		}
		n, err := s.IndexDocument(ctx, d.ID)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", d.ID, err))
			continue
		}
		res.Documents++
		res.Chunks += n
	}
	return res, nil
}

// RemoveDocument удаляет документ из векторной БД (например, при переходе в «устарел»).
func (s *Service) RemoveDocument(ctx context.Context, docID string) error {
	if err := s.Qdr.DeleteByDocument(ctx, docID); err != nil {
		return err
	}
	return s.Store.SetIndexed(ctx, docID, false)
}

// Result — найденный фрагмент с метаданными документа.
type Result struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	SourceURL  string  `json:"source_url"`
	Category   string  `json:"category"`
	EntityType string  `json:"entity_type,omitempty"`
	ChunkIndex int     `json:"chunk_index"`
	Text       string  `json:"text"`
	Score      float32 `json:"score"`
}

// Search ищет релевантные фрагменты среди действующих документов.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 5
	}
	vecs, err := s.Emb.Embed(ctx, []string{embed.PrefixQuery + query})
	if err != nil {
		return nil, fmt.Errorf("эмбеддинг запроса: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("пустой эмбеддинг запроса")
	}
	hits, err := s.Qdr.Search(ctx, vecs[0], limit, qdrant.FilterActive())
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(hits))
	for _, h := range hits {
		out = append(out, Result{
			DocumentID: asString(h.Payload["document_id"]),
			Title:      asString(h.Payload["title"]),
			SourceURL:  asString(h.Payload["source_url"]),
			Category:   asString(h.Payload["category"]),
			EntityType: asString(h.Payload["entity_type"]),
			ChunkIndex: asInt(h.Payload["chunk_index"]),
			Text:       asString(h.Payload["text"]),
			Score:      h.Score,
		})
	}
	return out, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
