// Package store хранит реестр документов (метаданные, статусы, версии).
//
// Доступны две реализации одного интерфейса Store:
//   - JSONStore     — файловый реестр (Документы_Сколково/Метаданные), без инфраструктуры;
//   - PostgresStore — продакшен-хранилище на PostgreSQL.
package store

import (
	"context"
	"errors"

	"baza-skolkovo/src/common/model"
)

// ErrNotFound возвращается, когда документ с указанным id отсутствует.
var ErrNotFound = errors.New("документ не найден")

// Filter ограничивает выборку документов.
type Filter struct {
	Status   model.Status // пустое значение — без фильтра по статусу
	Category string       // пустое значение — без фильтра по категории
}

// Store — интерфейс реестра документов.
type Store interface {
	Upsert(ctx context.Context, doc model.Document) error
	Get(ctx context.Context, id string) (model.Document, error)
	List(ctx context.Context, f Filter) ([]model.Document, error)
	SetStatus(ctx context.Context, id string, s model.Status) error
	SetIndexed(ctx context.Context, id string, indexed bool) error
	Delete(ctx context.Context, id string) error
	Close() error
}

func match(doc model.Document, f Filter) bool {
	if f.Status != "" && doc.Status != f.Status {
		return false
	}
	if f.Category != "" && doc.Category != f.Category {
		return false
	}
	return true
}
