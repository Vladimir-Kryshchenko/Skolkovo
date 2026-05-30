// Package migrate — система миграций БД для «База Сколково».
//
// ApplyMigrations сканирует указанную директорию, находит файлы вида
// NNN_description.sql, проверяет таблицу schema_migrations и применяет
// только те миграции, которые ещё не были выполнены.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schemaMigrationsDDL создаёт таблицу отслеживания миграций (идемпотентно).
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version     TEXT PRIMARY KEY,
	applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	filename    TEXT NOT NULL,
	checksum    TEXT NOT NULL
);
`

// Migration описывает одну миграцию.
type Migration struct {
	Version  string // "001", "002", ...
	Filename string // "001_create_events.sql"
	SQL      string
	Checksum string // sha256 от содержимого файла
}

// AppliedMigration — запись из schema_migrations.
type AppliedMigration struct {
	Version   string
	AppliedAt time.Time
	Filename  string
	Checksum  string
}

// ApplyMigrations применяет все неприменённые SQL-файлы из migrationsDir.
// Перед выполнением проверяет checksum: если миграция уже применена и
// контрольная сумма совпадает — файл пропускается. Если checksum отличается
// (файл был изменён после применения), миграция считается ошибочной.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	// 1. Создаём таблицу отслеживания, если её нет.
	if _, err := pool.Exec(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	log.Printf("[migrate] таблица schema_migrations готова")

	// 2. Сканируем директорию с миграциями.
	migrations, err := scanMigrations(migrationsDir)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		log.Printf("[migrate] миграции не найдены в %s", migrationsDir)
		return nil
	}
	log.Printf("[migrate] найдено %d миграций в %s", len(migrations), migrationsDir)

	// 3. Загружаем уже применённые миграции.
	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return fmt.Errorf("load applied migrations: %w", err)
	}
	appliedSet := make(map[string]AppliedMigration, len(applied))
	for _, a := range applied {
		appliedSet[a.Version] = a
	}

	// 4. Применяем по порядку.
	var appliedCount, skippedCount int
	for _, m := range migrations {
		existing, exists := appliedSet[m.Version]
		if exists {
			if existing.Checksum == m.Checksum {
				log.Printf("[migrate] SKIP %s — уже применена, checksum совпадает", m.Filename)
				skippedCount++
				continue
			}
			// Checksum отличается — файл изменён после применения.
			return fmt.Errorf("migration %s already applied but checksum changed (was %s, now %s)",
				m.Filename, existing.Checksum, m.Checksum)
		}

		log.Printf("[migrate] APPLY %s ...", m.Filename)
		if _, err := pool.Exec(ctx, m.SQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.Filename, err)
		}

		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version, filename, checksum) VALUES ($1, $2, $3)`,
			m.Version, m.Filename, m.Checksum,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", m.Filename, err)
		}

		log.Printf("[migrate] OK %s применена", m.Filename)
		appliedCount++
	}

	log.Printf("[migrate] завершено: применено %d, пропущено %d", appliedCount, skippedCount)
	return nil
}

// scanMigrations читает все .sql файлы в директории, сортирует по версии.
func scanMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory %s does not exist", dir)
		}
		return nil, err
	}

	var migrations []Migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := e.Name()
		// Ожидаем формат NNN_description.sql
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("migration file %s does not match NNN_description.sql pattern", name)
		}
		version := parts[0]

		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}

		sum := sha256.Sum256(content)
		checksum := hex.EncodeToString(sum[:])

		migrations = append(migrations, Migration{
			Version:  version,
			Filename: name,
			SQL:      string(content),
			Checksum: checksum,
		})
	}

	// Сортируем по версии (лексикографически: 001 < 002 < ... < 010).
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// loadApplied загружает список применённых миграций из БД.
func loadApplied(ctx context.Context, pool *pgxpool.Pool) ([]AppliedMigration, error) {
	rows, err := pool.Query(ctx,
		`SELECT version, applied_at, filename, checksum FROM schema_migrations ORDER BY version`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AppliedMigration
	for rows.Next() {
		var a AppliedMigration
		if err := rows.Scan(&a.Version, &a.AppliedAt, &a.Filename, &a.Checksum); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
