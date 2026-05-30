package health

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound возвращается, когда источник с указанным именем отсутствует.
var ErrNotFound = errors.New("источник не найден")

// schema — DDL мониторинга свежести (идемпотентно). Дублирует миграцию 004.
const schema = `
CREATE TABLE IF NOT EXISTS source_health (
    name                 TEXT        PRIMARY KEY,
    last_run_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at      TIMESTAMPTZ,
    items_last_run       INTEGER     NOT NULL DEFAULT 0,
    consecutive_failures INTEGER     NOT NULL DEFAULT 0,
    last_error           TEXT        NOT NULL DEFAULT '',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// PostgresStore — реализация Store поверх PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore применяет схему и возвращает хранилище.
func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool) (*PostgresStore, error) {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

// Record фиксирует результат прогона источника.
func (s *PostgresStore) Record(ctx context.Context, name string, items int, runErr error) error {
	if runErr != nil {
		_, err := s.pool.Exec(ctx, `
INSERT INTO source_health (name, last_run_at, consecutive_failures, last_error, updated_at)
VALUES ($1, now(), 1, $2, now())
ON CONFLICT (name) DO UPDATE SET
    last_run_at          = now(),
    consecutive_failures = source_health.consecutive_failures + 1,
    last_error           = EXCLUDED.last_error,
    updated_at           = now()`, name, runErr.Error())
		return err
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO source_health (name, last_run_at, last_success_at, items_last_run, consecutive_failures, last_error, updated_at)
VALUES ($1, now(), now(), $2, 0, '', now())
ON CONFLICT (name) DO UPDATE SET
    last_run_at          = now(),
    last_success_at      = now(),
    items_last_run       = EXCLUDED.items_last_run,
    consecutive_failures = 0,
    last_error           = '',
    updated_at           = now()`, name, items)
	return err
}

const selectCols = `SELECT name, last_run_at, last_success_at, items_last_run,
       consecutive_failures, last_error, updated_at
FROM source_health`

// List возвращает состояние всех источников (по имени).
func (s *PostgresStore) List(ctx context.Context) ([]Source, error) {
	rows, err := s.pool.Query(ctx, selectCols+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		src, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// Get возвращает состояние одного источника.
func (s *PostgresStore) Get(ctx context.Context, name string) (Source, error) {
	row := s.pool.QueryRow(ctx, selectCols+` WHERE name = $1`, name)
	src, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	return src, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scan(r scanner) (Source, error) {
	var s Source
	err := r.Scan(&s.Name, &s.LastRunAt, &s.LastSuccessAt, &s.ItemsLastRun,
		&s.ConsecutiveFailures, &s.LastError, &s.UpdatedAt)
	return s, err
}
