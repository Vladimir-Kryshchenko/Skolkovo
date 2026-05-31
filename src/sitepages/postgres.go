package sitepages

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound — страница с указанным ID отсутствует.
var ErrNotFound = errors.New("sitepages: страница не найдена")

// schema — DDL слоя страниц (идемпотентно). Дублирует миграцию 009, чтобы пакет
// работал и без отдельного прогона миграций.
const schema = `
CREATE TABLE IF NOT EXISTS site_pages (
    id            TEXT        PRIMARY KEY,
    url           TEXT        NOT NULL UNIQUE,
    title         TEXT        NOT NULL DEFAULT '',
    summary       TEXT        NOT NULL DEFAULT '',
    text          TEXT        NOT NULL DEFAULT '',
    section       TEXT        NOT NULL DEFAULT '',
    content_hash  TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'active',
    first_seen    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_changed  TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS text TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_site_pages_section      ON site_pages (section);
CREATE INDEX IF NOT EXISTS idx_site_pages_last_changed ON site_pages (last_changed DESC);
CREATE INDEX IF NOT EXISTS idx_site_pages_status       ON site_pages (status);
`

// PostgresStore — хранилище страниц поверх PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore применяет схему и возвращает хранилище страниц.
func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool) (*PostgresStore, error) {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

const selectCols = `SELECT id, url, title, summary, section, content_hash, status,
       first_seen, last_seen, last_changed
FROM site_pages`

// Get возвращает страницу по ID или ErrNotFound.
func (s *PostgresStore) Get(ctx context.Context, id string) (*Page, error) {
	row := s.pool.QueryRow(ctx, selectCols+` WHERE id = $1`, id)
	p, err := scanPage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// Upsert добавляет страницу или обновляет существующую. Возвращает один из
// UpsertNew | UpsertChanged | UpsertUnchanged: краулер по нему решает, фиксировать
// ли изменение в ленте. first_seen сохраняется при обновлениях.
func (s *PostgresStore) Upsert(ctx context.Context, p *Page) (string, error) {
	if p.Status == "" {
		p.Status = StatusActive
	}
	existing, err := s.Get(ctx, p.ID)
	if errors.Is(err, ErrNotFound) {
		now := time.Now()
		_, err = s.pool.Exec(ctx, `
INSERT INTO site_pages (id, url, title, summary, text, section, content_hash, status, first_seen, last_seen, last_changed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$9)`,
			p.ID, p.URL, p.Title, p.Summary, p.Text, p.Section, p.ContentHash, p.Status, now)
		if err != nil {
			return "", err
		}
		p.FirstSeen, p.LastSeen, p.LastChanged = now, now, now
		return UpsertNew, nil
	}
	if err != nil {
		return "", err
	}

	// Контент не менялся — отмечаем, что страница ещё жива.
	if existing.ContentHash == p.ContentHash {
		_, err = s.pool.Exec(ctx, `UPDATE site_pages SET last_seen = now(), status = $2 WHERE id = $1`, p.ID, p.Status)
		return UpsertUnchanged, err
	}

	// Контент изменился — обновляем поля и двигаем last_changed.
	_, err = s.pool.Exec(ctx, `
UPDATE site_pages
   SET title = $2, summary = $3, text = $4, section = $5, content_hash = $6, status = $7,
       last_seen = now(), last_changed = now()
 WHERE id = $1`,
		p.ID, p.Title, p.Summary, p.Text, p.Section, p.ContentHash, p.Status)
	return UpsertChanged, err
}

// GetWithText возвращает страницу вместе с полным текстом (для просмотрщика).
func (s *PostgresStore) GetWithText(ctx context.Context, id string) (*Page, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, url, title, summary, section, content_hash, status,
       first_seen, last_seen, last_changed, text
FROM site_pages WHERE id = $1`, id)
	var p Page
	err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Summary, &p.Section,
		&p.ContentHash, &p.Status, &p.FirstSeen, &p.LastSeen, &p.LastChanged, &p.Text)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListRecent возвращает страницы, отсортированные по дате последнего изменения.
func (s *PostgresStore) ListRecent(ctx context.Context, limit int) ([]*Page, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, selectCols+` ORDER BY last_changed DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPages(rows)
}

// ListAll возвращает все страницы (для полной переиндексации).
func (s *PostgresStore) ListAll(ctx context.Context) ([]*Page, error) {
	rows, err := s.pool.Query(ctx, selectCols+` ORDER BY url`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPages(rows)
}

// Count возвращает количество страниц.
func (s *PostgresStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM site_pages`).Scan(&n)
	return n, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanPage(row scannable) (*Page, error) {
	var p Page
	if err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Summary, &p.Section,
		&p.ContentHash, &p.Status, &p.FirstSeen, &p.LastSeen, &p.LastChanged); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanPages(rows pgx.Rows) ([]*Page, error) {
	var out []*Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
