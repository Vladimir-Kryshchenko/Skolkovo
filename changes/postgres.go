package changes

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schema — DDL ленты изменений (идемпотентно). Дублирует миграцию 004,
// чтобы пакет работал и без отдельного прогона миграций.
const schema = `
CREATE TABLE IF NOT EXISTS change_events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  TEXT        NOT NULL,
    entity_id    TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    category     TEXT,
    kind         TEXT        NOT NULL,
    source_url   TEXT,
    summary      TEXT,
    detected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    notified     BOOLEAN     NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_change_events_detected ON change_events (detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_events_unnotified ON change_events (detected_at) WHERE notified = FALSE;
CREATE INDEX IF NOT EXISTS idx_change_events_type ON change_events (entity_type);
`

// PostgresStore — реализация Store поверх PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore применяет схему и возвращает хранилище ленты изменений.
func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool) (*PostgresStore, error) {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

// Record фиксирует изменение. ID генерируется БД, поэтому в ev можно не задавать.
func (s *PostgresStore) Record(ctx context.Context, ev Event) error {
	if ev.DetectedAt.IsZero() {
		ev.DetectedAt = time.Now()
	}
	if ev.Kind == "" {
		ev.Kind = KindUpdated
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO change_events (entity_type, entity_id, title, category, kind, source_url, summary, detected_at, notified)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,FALSE)`,
		ev.EntityType, ev.EntityID, ev.Title, nullStr(ev.Category), string(ev.Kind),
		nullStr(ev.SourceURL), nullStr(ev.Summary), ev.DetectedAt)
	return err
}

const selectCols = `SELECT id, entity_type, entity_id, title,
       COALESCE(category,''), kind, COALESCE(source_url,''), COALESCE(summary,''),
       detected_at, notified
FROM change_events`

// Recent возвращает последние изменения по фильтру.
func (s *PostgresStore) Recent(ctx context.Context, f Filter) ([]Event, error) {
	q := selectCols + ` WHERE 1=1`
	args := []any{}
	n := 0
	if !f.Since.IsZero() {
		n++
		q += " AND detected_at >= $" + itoa(n)
		args = append(args, f.Since)
	}
	if f.EntityType != "" {
		n++
		q += " AND entity_type = $" + itoa(n)
		args = append(args, f.EntityType)
	}
	if f.Category != "" {
		n++
		q += " AND category = $" + itoa(n)
		args = append(args, f.Category)
	}
	q += " ORDER BY detected_at DESC"
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	n++
	q += " LIMIT $" + itoa(n)
	args = append(args, limit)
	return s.query(ctx, q, args...)
}

// Unnotified возвращает неотправленные изменения (по возрастанию времени — сначала старые).
func (s *PostgresStore) Unnotified(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.query(ctx, selectCols+` WHERE notified = FALSE ORDER BY detected_at ASC LIMIT $1`, limit)
}

// MarkNotified помечает изменения как отправленные.
func (s *PostgresStore) MarkNotified(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE change_events SET notified = TRUE WHERE id = ANY($1)`, ids)
	return err
}

func (s *PostgresStore) query(ctx context.Context, q string, args ...any) ([]Event, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var kind string
		if err := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.Title, &e.Category,
			&kind, &e.SourceURL, &e.Summary, &e.DetectedAt, &e.Notified); err != nil {
			return nil, err
		}
		e.Kind = Kind(kind)
		out = append(out, e)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// itoa — минимальный int→string без strconv-импорта в hot path построения запроса.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
