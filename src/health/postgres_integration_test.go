package health

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

func TestPostgresStore_RecordAndState(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	st, err := NewPostgresStore(ctx, pool)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	name := "test-source-" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM source_health WHERE name = $1", name)
	})

	// Успешный прогон → ok, items записаны, ошибок нет.
	if err := st.Record(ctx, name, 42, nil); err != nil {
		t.Fatalf("Record(success): %v", err)
	}
	src, err := st.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if src.ItemsLastRun != 42 || src.ConsecutiveFailures != 0 {
		t.Errorf("после успеха: items=%d failures=%d", src.ItemsLastRun, src.ConsecutiveFailures)
	}
	if got := src.State(24*time.Hour, time.Now()); got != StatusOK {
		t.Errorf("State после успеха = %q, ожидалось ok", got)
	}

	// Три ошибки подряд → failing.
	for i := 0; i < 3; i++ {
		if err := st.Record(ctx, name, 0, errors.New("сбой парсера")); err != nil {
			t.Fatalf("Record(fail): %v", err)
		}
	}
	src, _ = st.Get(ctx, name)
	if src.ConsecutiveFailures < 3 {
		t.Errorf("ожидалось >=3 подряд ошибок, получено %d", src.ConsecutiveFailures)
	}
	if got := src.State(24*time.Hour, time.Now()); got != StatusFailing {
		t.Errorf("State после ошибок = %q, ожидалось failing", got)
	}
}
