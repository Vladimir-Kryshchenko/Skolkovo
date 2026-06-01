package changes

import (
	"context"
	"testing"
	"time"
)

// TestPostgresStore_LatestByEntityAndAnalyzed проверяет точечные запросы для
// панели автоматического сравнения версий: «последнее событие документа» и
// «последний вердикт на документ» (только после обогащения).
func TestPostgresStore_LatestByEntityAndAnalyzed(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	st, err := NewPostgresStore(ctx, pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	eid := "test-latest-" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM change_events WHERE entity_id = $1", eid)
	})

	// Старое + новое события одного документа.
	if err := st.Record(ctx, Event{EntityType: EntityDocument, EntityID: eid, Title: "old", Kind: KindUpdated, DetectedAt: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatalf("Record old: %v", err)
	}
	if err := st.Record(ctx, Event{EntityType: EntityDocument, EntityID: eid, Title: "new", Kind: KindUpdated, DetectedAt: time.Now()}); err != nil {
		t.Fatalf("Record new: %v", err)
	}

	// LatestByEntity → самое свежее ("new").
	latest, found, err := st.LatestByEntity(ctx, EntityDocument, eid)
	if err != nil || !found {
		t.Fatalf("LatestByEntity: found=%v err=%v", found, err)
	}
	if latest.Title != "new" {
		t.Errorf("LatestByEntity: ожидалось 'new', получено %q", latest.Title)
	}

	// До обогащения вердиктов нет → документа быть не должно.
	before, err := st.LatestAnalyzedByType(ctx, EntityDocument)
	if err != nil {
		t.Fatalf("LatestAnalyzedByType before: %v", err)
	}
	if _, ok := before[eid]; ok {
		t.Error("без вердикта документ не должен попадать в LatestAnalyzedByType")
	}

	// Обогащаем самое свежее событие вердиктом.
	if err := st.Enrich(ctx, latest.ID, Enrichment{Severity: SeverityCritical, AnalysisSummary: "важное изменение", DiffAdded: 5, DiffRemoved: 2}); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	after, err := st.LatestAnalyzedByType(ctx, EntityDocument)
	if err != nil {
		t.Fatalf("LatestAnalyzedByType after: %v", err)
	}
	v, ok := after[eid]
	if !ok {
		t.Fatal("после Enrich документ должен попасть в LatestAnalyzedByType")
	}
	if v.Severity != SeverityCritical || v.AnalysisSummary != "важное изменение" {
		t.Errorf("неверный вердикт: %+v", v)
	}
}
