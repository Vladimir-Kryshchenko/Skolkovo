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
-- ИИ-обогащение (см. миграцию 010): теги, краткое описание, цели, тезисы, выводы.
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS tags         TEXT[]      NOT NULL DEFAULT '{}';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS ai_summary   TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS goals        TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS theses       TEXT[]      NOT NULL DEFAULT '{}';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS conclusions  TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enriched_at  TIMESTAMPTZ;
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enrich_hash  TEXT        NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_site_pages_section      ON site_pages (section);
CREATE INDEX IF NOT EXISTS idx_site_pages_last_changed ON site_pages (last_changed DESC);
CREATE INDEX IF NOT EXISTS idx_site_pages_status       ON site_pages (status);
CREATE INDEX IF NOT EXISTS idx_site_pages_tags         ON site_pages USING GIN (tags);
CREATE TABLE IF NOT EXISTS site_page_tags (
    tag         TEXT        PRIMARY KEY,
    usage_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
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
       first_seen, last_seen, last_changed,
       tags, ai_summary, goals, theses, conclusions, enriched_at
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

// GetWithText возвращает страницу вместе с полным текстом и ИИ-полями (для просмотрщика).
func (s *PostgresStore) GetWithText(ctx context.Context, id string) (*Page, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, url, title, summary, section, content_hash, status,
       first_seen, last_seen, last_changed, text,
       tags, ai_summary, goals, theses, conclusions, enriched_at
FROM site_pages WHERE id = $1`, id)
	var p Page
	err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Summary, &p.Section,
		&p.ContentHash, &p.Status, &p.FirstSeen, &p.LastSeen, &p.LastChanged, &p.Text,
		&p.Tags, &p.AISummary, &p.Goals, &p.Theses, &p.Conclusions, &p.EnrichedAt)
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

// ─── ИИ-обогащение ──────────────────────────────────────────────────────────

// UpdateEnrichment сохраняет результат аннотирования страницы ИИ и фиксирует
// enrich_hash = текущий content_hash (чтобы переаннотировать только при
// изменении контента). Базовые поля (title/text/...) не трогаются.
func (s *PostgresStore) UpdateEnrichment(ctx context.Context, id string, a Annotation, contentHash string) error {
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	theses := a.Theses
	if theses == nil {
		theses = []string{}
	}
	_, err := s.pool.Exec(ctx, `
UPDATE site_pages
   SET tags=$2, ai_summary=$3, goals=$4, theses=$5, conclusions=$6,
       enriched_at=now(), enrich_hash=$7
 WHERE id=$1`,
		id, tags, a.Summary, a.Goals, theses, a.Conclusions, contentHash)
	return err
}

const enrichSelect = `SELECT id, url, title, summary, section, content_hash, status,
       first_seen, last_seen, last_changed, text
FROM site_pages`

// ListNeedingEnrichment возвращает действующие страницы, ещё не аннотированные
// или аннотированные для устаревшего контента (enrich_hash <> content_hash),
// вместе с полным текстом. limit<=0 — без ограничения (для бэкфилла всей базы).
func (s *PostgresStore) ListNeedingEnrichment(ctx context.Context, limit int) ([]*Page, error) {
	q := enrichSelect + `
WHERE status = 'active' AND (enriched_at IS NULL OR enrich_hash <> content_hash)
ORDER BY last_changed DESC`
	return s.queryEnrich(ctx, q, limit)
}

// ListAllForEnrichment возвращает все действующие страницы с текстом (для
// принудительной переаннотации `enrich --all`). limit<=0 — без ограничения.
func (s *PostgresStore) ListAllForEnrichment(ctx context.Context, limit int) ([]*Page, error) {
	q := enrichSelect + `
WHERE status = 'active'
ORDER BY last_changed DESC`
	return s.queryEnrich(ctx, q, limit)
}

func (s *PostgresStore) queryEnrich(ctx context.Context, q string, limit int) ([]*Page, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if limit > 0 {
		rows, err = s.pool.Query(ctx, q+` LIMIT $1`, limit)
	} else {
		rows, err = s.pool.Query(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Page
	for rows.Next() {
		p, err := scanPageWithText(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ─── Словарь тегов (гибрид: ИИ переиспользует существующие, новые пополняют) ──

// ListTags возвращает теги словаря, самые частые — первыми.
func (s *PostgresStore) ListTags(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT tag FROM site_page_tags ORDER BY usage_count DESC, tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// BumpTags добавляет теги в словарь (или увеличивает их usage_count).
func (s *PostgresStore) BumpTags(ctx context.Context, tags []string) error {
	for _, t := range tags {
		if t == "" {
			continue
		}
		if _, err := s.pool.Exec(ctx, `
INSERT INTO site_page_tags (tag, usage_count) VALUES ($1, 1)
ON CONFLICT (tag) DO UPDATE SET usage_count = site_page_tags.usage_count + 1`, t); err != nil {
			return err
		}
	}
	return nil
}

// RelatedPage — лёгкая ссылка на связанную страницу (для блока «связанные»).
type RelatedPage struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Section string `json:"section"`
	Shared  int    `json:"shared,omitempty"` // число общих тегов (для связи по тегам)
}

// RelatedByTags возвращает действующие страницы с наибольшим числом общих тегов
// (кроме самой страницы id). Использует GIN-индекс по tags (оператор &&).
func (s *PostgresStore) RelatedByTags(ctx context.Context, id string, tags []string, limit int) ([]RelatedPage, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 6
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, url, title, section,
       cardinality(ARRAY(SELECT unnest(tags) INTERSECT SELECT unnest($2::text[]))) AS shared
FROM site_pages
WHERE id <> $1 AND status = 'active' AND tags && $2::text[]
ORDER BY shared DESC, last_changed DESC
LIMIT $3`, id, tags, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RelatedPage
	for rows.Next() {
		var r RelatedPage
		if err := rows.Scan(&r.ID, &r.URL, &r.Title, &r.Section, &r.Shared); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanPage(row scannable) (*Page, error) {
	var p Page
	if err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Summary, &p.Section,
		&p.ContentHash, &p.Status, &p.FirstSeen, &p.LastSeen, &p.LastChanged,
		&p.Tags, &p.AISummary, &p.Goals, &p.Theses, &p.Conclusions, &p.EnrichedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// scanPageWithText сканирует базовые поля + полный текст (для аннотирования и
// просмотрщика); ИИ-поля здесь не нужны.
func scanPageWithText(row scannable) (*Page, error) {
	var p Page
	if err := row.Scan(&p.ID, &p.URL, &p.Title, &p.Summary, &p.Section,
		&p.ContentHash, &p.Status, &p.FirstSeen, &p.LastSeen, &p.LastChanged, &p.Text); err != nil {
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
