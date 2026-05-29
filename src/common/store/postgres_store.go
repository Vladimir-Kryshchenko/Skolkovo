package store

import (
	"context"
	"errors"

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
WHERE ($1='' OR status=$1) AND ($2='' OR category=$2)
ORDER BY fetched_at DESC`, string(f.Status), f.Category)
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
       status, category, version_label, valid_from, valid_to, supersedes, file_hash, indexed
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
		&status, &category, &versionLabel, &d.ValidFrom, &d.ValidTo, &supersedes, &d.FileHash, &d.Indexed)
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
