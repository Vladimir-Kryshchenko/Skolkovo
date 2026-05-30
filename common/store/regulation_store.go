// Package store хранит интерфейсы хранилищ для льгот и нормативно-правовых актов.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"baza-skolkovo/src/common/model"
)

// Ошибки валидации для льгот и НПА.
var (
	ErrEmptyPreferenceTitle = errors.New("название льготы не может быть пустым")
	ErrEmptyNPATitle        = errors.New("название НПА не может быть пустым")
)

// validatePreference проверяет льготу.
func validatePreference(p *model.Preference) error {
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("Title: %w", ErrEmptyPreferenceTitle)
	}
	if strings.TrimSpace(p.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	return nil
}

// validateNPA проверяет нормативно-правовой акт.
func validateNPA(n *model.NPADocument) error {
	if strings.TrimSpace(n.Title) == "" {
		return fmt.Errorf("Title: %w", ErrEmptyNPATitle)
	}
	if strings.TrimSpace(n.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	return nil
}

// ---------------------------------------------------------------------------
// PreferenceStore — интерфейс хранилища льгот.
// ---------------------------------------------------------------------------

// PreferenceStore определяет операции CRUD для льгот.
type PreferenceStore interface {
	// CreatePreference создаёт льготу после валидации.
	CreatePreference(ctx context.Context, pref *model.Preference) error

	// GetPreference возвращает льготу по идентификатору.
	GetPreference(ctx context.Context, id string) (*model.Preference, error)

	// ListPreferences возвращает список льгот с фильтрацией по типу и статусу.
	// Пустые параметры означают отсутствие фильтра.
	ListPreferences(ctx context.Context, prefType string, status string) ([]*model.Preference, error)

	// UpdatePreference обновляет данные льготы после валидации.
	UpdatePreference(ctx context.Context, pref *model.Preference) error

	// DeletePreference удаляет льготу по идентификатору.
	DeletePreference(ctx context.Context, id string) error

	// CountPreferences возвращает общее количество льгот.
	CountPreferences(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------------------
// NPAStore — интерфейс хранилища нормативно-правовых актов.
// ---------------------------------------------------------------------------

// NPAStore определяет операции CRUD для нормативно-правовых актов.
type NPAStore interface {
	// CreateNPA создаёт нормативно-правовой акт после валидации.
	CreateNPA(ctx context.Context, npa *model.NPADocument) error

	// GetNPA возвращает нормативно-правовой акт по идентификатору.
	GetNPA(ctx context.Context, id string) (*model.NPADocument, error)

	// ListNPA возвращает список НПА с фильтрацией по типу и статусу.
	// Пустые параметры означают отсутствие фильтра.
	ListNPA(ctx context.Context, npaType string, status string) ([]*model.NPADocument, error)

	// UpdateNPA обновляет данные нормативно-правового акта после валидации.
	UpdateNPA(ctx context.Context, npa *model.NPADocument) error

	// DeleteNPA удаляет нормативно-правовой акт по идентификатору.
	DeleteNPA(ctx context.Context, id string) error

	// CountNPA возвращает общее количество нормативно-правовых актов.
	CountNPA(ctx context.Context) (int, error)
}
