package changes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool открывает пул к тестовой БД из SKOLKOVO_TEST_DSN или пропускает тест.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKOLKOVO_TEST_DSN")
	if dsn == "" {
		t.Skip("SKOLKOVO_TEST_DSN не задан — пропуск интеграционного теста с PostgreSQL")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("подключение к тестовой БД: %v", err)
	}
	return pool
}

func TestPostgresStore_RecordRecentNotify(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	st, err := NewPostgresStore(ctx, pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	// Уникальный entity_id, чтобы изолировать тестовые данные.
	eid := "test-doc-" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM change_events WHERE entity_id = $1", eid)
	})

	if err := st.Record(ctx, Event{
		EntityType: EntityDocument,
		EntityID:   eid,
		Title:      "Тестовый регламент",
		Category:   "Тест",
		Kind:       KindNew,
		SourceURL:  "https://example.com/doc",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Recent по фильтру категории должен вернуть нашу запись.
	recent, err := st.Recent(ctx, Filter{Category: "Тест", Limit: 10})
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	found := false
	for _, e := range recent {
		if e.EntityID == eid {
			found = true
			if e.Kind != KindNew || e.Title != "Тестовый регламент" {
				t.Errorf("неверные поля записи: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("запись не найдена в Recent")
	}

	// Unnotified → MarkNotified → больше не в Unnotified.
	un, err := st.Unnotified(ctx, 100)
	if err != nil {
		t.Fatalf("Unnotified: %v", err)
	}
	var ids []string
	for _, e := range un {
		if e.EntityID == eid {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		t.Fatal("запись должна быть среди неотправленных")
	}
	if err := st.MarkNotified(ctx, ids); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	un2, _ := st.Unnotified(ctx, 100)
	for _, e := range un2 {
		if e.EntityID == eid {
			t.Error("запись осталась неотправленной после MarkNotified")
		}
	}
}
