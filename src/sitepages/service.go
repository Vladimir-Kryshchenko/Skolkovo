package sitepages

import (
	"context"
	"strings"
)

// AdminService — операции над одной страницей для админки: ручной перезапуск
// ИИ-аннотирования и сохранение правок куратора. Бандлит хранилище, обогатитель
// и индексатор, чтобы изменения сразу попадали и в БД, и в RAG-коллекцию.
type AdminService struct {
	Pages    *PostgresStore
	Enricher *Enricher
	Indexer  *Indexer
}

// NewAdminService собирает сервис админских операций над страницами.
func NewAdminService(pages *PostgresStore, enr *Enricher, idx *Indexer) *AdminService {
	return &AdminService{Pages: pages, Enricher: enr, Indexer: idx}
}

// ReannotateOne заново аннотирует одну страницу через ИИ и переиндексирует её.
// Возвращает статус: "done" — обогащено; "skipped" — агент не настроен;
// "failed" — ошибка LLM/сохранения.
func (s *AdminService) ReannotateOne(ctx context.Context, id string) (string, error) {
	p, err := s.Pages.GetWithText(ctx, id)
	if err != nil {
		return "", err
	}
	done, skipped, _ := s.Enricher.EnrichBatch(ctx, []*Page{p})
	switch {
	case done > 0:
		s.reindexOne(ctx, p)
		return "done", nil
	case skipped > 0:
		return "skipped", nil
	default:
		return "failed", nil
	}
}

// SaveAnnotation сохраняет ручную правку аннотации (теги/описание/цели/тезисы/
// выводы) и переиндексирует страницу. Помечает страницу аннотированной для
// текущего контента (переаннотируется автоматически лишь при изменении контента).
func (s *AdminService) SaveAnnotation(ctx context.Context, id string, a Annotation) error {
	p, err := s.Pages.GetWithText(ctx, id)
	if err != nil {
		return err
	}
	a.Tags = normalizeTags(a.Tags, nil, maxTags)
	a.Theses = cleanList(a.Theses, maxTheses)
	a.Summary = strings.TrimSpace(a.Summary)
	a.Goals = strings.TrimSpace(a.Goals)
	a.Conclusions = strings.TrimSpace(a.Conclusions)
	if err := s.Pages.UpdateEnrichment(ctx, id, a, p.ContentHash); err != nil {
		return err
	}
	_ = s.Pages.BumpTags(ctx, a.Tags)
	applyAnnotation(p, a)
	s.reindexOne(ctx, p)
	return nil
}

func (s *AdminService) reindexOne(ctx context.Context, p *Page) {
	if s.Indexer != nil {
		_, _ = s.Indexer.Reindex(ctx, []*Page{p})
	}
}
