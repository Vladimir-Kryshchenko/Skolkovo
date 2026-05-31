package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/common/model"
)

// PostgresVersionStore хранит снимки версий документов (doc_versions) —
// извлечённый текст каждой редакции для семантического диффа.
type PostgresVersionStore struct {
	db *pgxpool.Pool
}

// NewPostgresVersionStore создаёт хранилище версий.
func NewPostgresVersionStore(db *pgxpool.Pool) *PostgresVersionStore {
	return &PostgresVersionStore{db: db}
}

// SaveVersion добавляет новый снимок версии документа. Номер версии
// назначается автоматически (max+1 для данного документа). Возвращает
// присвоенный version_no.
func (s *PostgresVersionStore) SaveVersion(ctx context.Context, v *model.DocVersion) (int, error) {
	var versionNo int
	err := s.db.QueryRow(ctx, `
INSERT INTO doc_versions (document_id, version_no, file_hash, extracted_text, archived_path)
VALUES ($1, COALESCE((SELECT MAX(version_no) FROM doc_versions WHERE document_id=$1),0)+1, $2, $3, $4)
RETURNING version_no`,
		v.DocumentID, v.FileHash, v.ExtractedText, v.ArchivedPath).Scan(&versionNo)
	return versionNo, err
}

// LatestVersions возвращает последние n версий документа (по убыванию version_no).
func (s *PostgresVersionStore) LatestVersions(ctx context.Context, documentID string, n int) ([]*model.DocVersion, error) {
	if n <= 0 {
		n = 2
	}
	rows, err := s.db.Query(ctx, `
SELECT id, document_id, version_no, file_hash, extracted_text, COALESCE(archived_path,''), created_at
FROM doc_versions WHERE document_id = $1 ORDER BY version_no DESC LIMIT $2`, documentID, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.DocVersion
	for rows.Next() {
		var v model.DocVersion
		if err := rows.Scan(&v.ID, &v.DocumentID, &v.VersionNo, &v.FileHash,
			&v.ExtractedText, &v.ArchivedPath, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// CountVersions возвращает число сохранённых версий документа.
func (s *PostgresVersionStore) CountVersions(ctx context.Context, documentID string) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM doc_versions WHERE document_id = $1`, documentID).Scan(&n)
	return n, err
}
