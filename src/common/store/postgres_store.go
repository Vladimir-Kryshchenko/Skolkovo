package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/common/model"
)

// schema — DDL реестра документов (идемпотентно).
const schema = `
CREATE TABLE IF NOT EXISTS documents (
    id            TEXT PRIMARY KEY,
    title         TEXT NOT NULL,
    source_url    TEXT NOT NULL,
    local_path    TEXT,
    published_at  DATE,
    fetched_at    TIMESTAMPTZ NOT NULL,
    status        TEXT NOT NULL,
    category      TEXT,
    version_label TEXT,
    valid_from    DATE,
    valid_to      DATE,
    supersedes    TEXT,
    file_hash     TEXT NOT NULL,
    indexed       BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_documents_status   ON documents (status);
CREATE INDEX IF NOT EXISTS idx_documents_category ON documents (category);
`

// PostgresStore — реализация Store поверх PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore подключается к Postgres по DSN и применяет схему.
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Upsert(ctx context.Context, d model.Document) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO documents (id, title, source_url, local_path, published_at, fetched_at,
                       status, category, version_label, valid_from, valid_to, supersedes, file_hash, indexed)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (id) DO UPDATE SET
    title=EXCLUDED.title, source_url=EXCLUDED.source_url, local_path=EXCLUDED.local_path,
    published_at=EXCLUDED.published_at, fetched_at=EXCLUDED.fetched_at, status=EXCLUDED.status,
    category=EXCLUDED.category, version_label=EXCLUDED.version_label, valid_from=EXCLUDED.valid_from,
    valid_to=EXCLUDED.valid_to, supersedes=EXCLUDED.supersedes, file_hash=EXCLUDED.file_hash,
    indexed=EXCLUDED.indexed`,
		d.ID, d.Title, d.SourceURL, nullStr(d.LocalPath), d.PublishedAt, d.FetchedAt,
		string(d.Status), nullStr(d.Category), nullStr(d.VersionLabel), d.ValidFrom, d.ValidTo,
		nullStr(d.Supersedes), d.FileHash, d.Indexed)
	return err
}

func (s *PostgresStore) Get(ctx context.Context, id string) (model.Document, error) {
	row := s.pool.QueryRow(ctx, selectCols+` WHERE id=$1`, id)
	d, err := scanDoc(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Document{}, ErrNotFound
	}
	return d, err
}

func (s *PostgresStore) List(ctx context.Context, f Filter) ([]model.Document, error) {
	rows, err := s.pool.Query(ctx, selectCols+`
WHERE ($1='' OR status=$1)
  AND ($2='' OR category=$2)
  AND ($3='' OR subcategory=$3)
  AND ($4::text[] IS NULL OR tags @> $4::text[])
ORDER BY fetched_at DESC`, string(f.Status), f.Category, f.Subcategory, tagsArg(f.Tags))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SetStatus(ctx context.Context, id string, st model.Status) error {
	return s.exec1(ctx, `UPDATE documents SET status=$2 WHERE id=$1`, id, string(st))
}

func (s *PostgresStore) SetIndexed(ctx context.Context, id string, indexed bool) error {
	return s.exec1(ctx, `UPDATE documents SET indexed=$2 WHERE id=$1`, id, indexed)
}

func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM documents WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

// Pool возвращает underlying pgxpool.Pool для создания расширенных хранилищ.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// UpdateClassification сохраняет ИИ-разметку документа: категорию, подкатегорию,
// теги и enrich_hash (= file_hash на момент разметки). НЕ трогает прочие поля —
// безопасно относительно параллельного переобхода (тот не пишет эти колонки).
func (s *PostgresStore) UpdateClassification(ctx context.Context, id, category, subcategory string, tags []string, enrichHash string) error {
	if tags == nil {
		tags = []string{}
	}
	return s.exec1Args(ctx, `
UPDATE documents
   SET category    = COALESCE(NULLIF($2,''), category),
       subcategory = $3,
       tags        = $4,
       enriched_at = now(),
       enrich_hash = $5
 WHERE id = $1`, id, category, subcategory, tags, enrichHash)
}

// ListNeedingEnrichment возвращает действующие документы, которым нужна ИИ-разметка:
// ещё не размечены или файл изменился (enrich_hash <> file_hash). limit<=0 — все.
func (s *PostgresStore) ListNeedingEnrichment(ctx context.Context, limit int) ([]model.Document, error) {
	q := selectCols + `
WHERE status = 'действует'
  AND (enriched_at IS NULL OR enrich_hash <> file_hash)
ORDER BY fetched_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListTags возвращает словарь авто-тегов документов, частые — первыми.
func (s *PostgresStore) ListTags(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT tag FROM document_tags ORDER BY usage_count DESC, tag`)
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

// BumpTags добавляет теги в словарь или увеличивает их usage_count.
func (s *PostgresStore) BumpTags(ctx context.Context, tags []string) error {
	for _, t := range tags {
		if t == "" {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO document_tags (tag, usage_count) VALUES ($1, 1)
			 ON CONFLICT (tag) DO UPDATE SET usage_count = document_tags.usage_count + 1`, t); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) exec1Args(ctx context.Context, sql string, args ...any) error {
	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// tagsArg возвращает аргумент для фильтра по тегам: nil (без фильтра) либо срез.
func tagsArg(tags []string) any {
	if len(tags) == 0 {
		return nil
	}
	return tags
}

func (s *PostgresStore) exec1(ctx context.Context, sql, id string, arg any) error {
	tag, err := s.pool.Exec(ctx, sql, id, arg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const selectCols = `SELECT id, title, source_url, local_path, published_at, fetched_at,
       status, category, version_label, valid_from, valid_to, supersedes, file_hash, indexed,
       subcategory, tags, enriched_at, enrich_hash
FROM documents`

// row — общий интерфейс для pgx.Row и pgx.Rows.
type row interface {
	Scan(dest ...any) error
}

func scanDoc(r row) (model.Document, error) {
	var d model.Document
	var status string
	var localPath, category, versionLabel, supersedes *string
	err := r.Scan(&d.ID, &d.Title, &d.SourceURL, &localPath, &d.PublishedAt, &d.FetchedAt,
		&status, &category, &versionLabel, &d.ValidFrom, &d.ValidTo, &supersedes, &d.FileHash, &d.Indexed,
		&d.Subcategory, &d.Tags, &d.EnrichedAt, &d.EnrichHash)
	if err != nil {
		return d, err
	}
	d.Status = model.Status(status)
	d.LocalPath = deref(localPath)
	d.Category = deref(category)
	d.VersionLabel = deref(versionLabel)
	d.Supersedes = deref(supersedes)
	return d, nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
