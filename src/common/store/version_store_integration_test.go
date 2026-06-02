package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/common/model"
)

// versionTestPool открывает пул к тестовой БД из SKOLKOVO_TEST_DSN или пропускает тест.
func versionTestPool(t *testing.T) *pgxpool.Pool {
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

func TestPostgresVersionStore_DocumentsWithVersions(t *testing.T) {
	ctx := context.Background()
	pool := versionTestPool(t)
	defer pool.Close()

	// Таблица версий (идемпотентно — на случай, если миграции не прогнаны).
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS doc_versions (
		id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		document_id    TEXT        NOT NULL,
		version_no     INT         NOT NULL,
		file_hash      TEXT        NOT NULL DEFAULT '',
		extracted_text TEXT        NOT NULL DEFAULT '',
		archived_path  TEXT        NOT NULL DEFAULT '',
		created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (document_id, version_no))`); err != nil {
		t.Fatalf("create doc_versions: %v", err)
	}

	vs := NewPostgresVersionStore(pool)
	stamp := time.Now().Format("20060102150405.000000")
	docMulti := "test-vdoc-multi-" + stamp
	docSingle := "test-vdoc-single-" + stamp
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM doc_versions WHERE document_id IN ($1,$2)", docMulti, docSingle)
	})

	// docMulti: две версии; docSingle: одна.
	if _, err := vs.SaveVersion(ctx, &model.DocVersion{DocumentID: docMulti, ExtractedText: "v1"}); err != nil {
		t.Fatalf("SaveVersion 1: %v", err)
	}
	if _, err := vs.SaveVersion(ctx, &model.DocVersion{DocumentID: docMulti, ExtractedText: "v2"}); err != nil {
		t.Fatalf("SaveVersion 2: %v", err)
	}
	if _, err := vs.SaveVersion(ctx, &model.DocVersion{DocumentID: docSingle, ExtractedText: "only"}); err != nil {
		t.Fatalf("SaveVersion single: %v", err)
	}

	got, err := vs.DocumentsWithVersions(ctx, 2)
	if err != nil {
		t.Fatalf("DocumentsWithVersions: %v", err)
	}

	var multi *DocVersionSummary
	for i := range got {
		if got[i].DocumentID == docMulti {
			multi = &got[i]
		}
		if got[i].DocumentID == docSingle {
			t.Error("документ с одной версией не должен попадать в выборку (min=2)")
		}
	}
	if multi == nil {
		t.Fatal("документ с двумя версиями должен быть в выборке")
	}
	if multi.VersionCount != 2 || multi.LatestNo != 2 {
		t.Errorf("ожидалось VersionCount=2, LatestNo=2, получено %+v", *multi)
	}
}
