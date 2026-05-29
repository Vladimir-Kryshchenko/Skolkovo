// Package store хранит интерфейсы хранилищ для расширенных источников данных:
// мероприятий, конкурсов, FAQ, постов Telegram, резидентов и связей документов.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
)

// Ошибки валидации для расширенных источников.
var (
	ErrEmptyTitle       = errors.New("заголовок не может быть пустым")
	ErrEmptyQuestion    = errors.New("вопрос не может быть пустым")
	ErrEmptyAnswer      = errors.New("ответ не может быть пустым")
	ErrEmptyChannel     = errors.New("канал не может быть пустым")
	ErrEmptyText        = errors.New("текст не может быть пустым")
	ErrEmptyName        = errors.New("имя не может быть пустым")
	ErrEmptySourceID    = errors.New("source_id не может быть пустым")
	ErrEmptyTargetID    = errors.New("target_id не может быть пустым")
	ErrInvalidEventDate = errors.New("дата окончания мероприятия не может быть раньше даты начала")
	ErrInvalidContestDates = errors.New("дата окончания конкурса не может быть раньше даты начала")
	ErrInvalidResidentINN = errors.New("некорректный ИНН резидента: должен быть 10 или 12 цифр")
	ErrInvalidLinkType  = errors.New("недопустимый тип связи")
	ErrNegativeLimit    = errors.New("лимит не может быть отрицательным")
)

// validateEvent проверяет мероприятие.
func validateEvent(e *model.Event) error {
	if strings.TrimSpace(e.Title) == "" {
		return fmt.Errorf("Title: %w", ErrEmptyTitle)
	}
	if strings.TrimSpace(e.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	if !e.EventEndDate.IsZero() && !e.EventDate.IsZero() && e.EventEndDate.Before(e.EventDate) {
		return ErrInvalidEventDate
	}
	return nil
}

// validateContest проверяет конкурс/грант.
func validateContest(c *model.Contest) error {
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("Title: %w", ErrEmptyTitle)
	}
	if strings.TrimSpace(c.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	if !c.EndDate.IsZero() && !c.StartDate.IsZero() && c.EndDate.Before(c.StartDate) {
		return ErrInvalidContestDates
	}
	return nil
}

// validateFAQItem проверяет элемент FAQ.
func validateFAQItem(f *model.FAQItem) error {
	if strings.TrimSpace(f.Question) == "" {
		return fmt.Errorf("Question: %w", ErrEmptyQuestion)
	}
	if strings.TrimSpace(f.Answer) == "" {
		return fmt.Errorf("Answer: %w", ErrEmptyAnswer)
	}
	if strings.TrimSpace(f.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	return nil
}

// validateTelegramPost проверяет пост Telegram.
func validateTelegramPost(p *model.TelegramPost) error {
	if strings.TrimSpace(p.Channel) == "" {
		return fmt.Errorf("Channel: %w", ErrEmptyChannel)
	}
	if strings.TrimSpace(p.Text) == "" {
		return fmt.Errorf("Text: %w", ErrEmptyText)
	}
	if strings.TrimSpace(p.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	return nil
}

// validateResident проверяет запись резидента.
func validateResident(r *model.Resident) error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("Name: %w", ErrEmptyName)
	}
	if strings.TrimSpace(r.SourceURL) == "" {
		return fmt.Errorf("SourceURL: %w", ErrEmptyField)
	}
	if r.INN != "" && (len(r.INN) != 10 && len(r.INN) != 12) {
		return ErrInvalidResidentINN
	}
	if r.INN != "" {
		for _, ch := range r.INN {
			if ch < '0' || ch > '9' {
				return ErrInvalidResidentINN
			}
		}
	}
	return nil
}

// validateDocumentLink проверяет связь документов.
func validateDocumentLink(l *model.DocumentLink) error {
	if strings.TrimSpace(l.SourceID) == "" {
		return fmt.Errorf("SourceID: %w", ErrEmptySourceID)
	}
	if strings.TrimSpace(l.TargetID) == "" {
		return fmt.Errorf("TargetID: %w", ErrEmptyTargetID)
	}
	switch l.LinkType {
	case model.LinkReferences, model.LinkSupersedes, model.LinkRelated:
		// valid
	default:
		return ErrInvalidLinkType
	}
	return nil
}

// ---------------------------------------------------------------------------
// EventStore — интерфейс хранилища мероприятий.
// ---------------------------------------------------------------------------

// EventStore определяет операции CRUD для мероприятий.
type EventStore interface {
	// CreateEvent создаёт мероприятие после валидации.
	CreateEvent(ctx context.Context, event *model.Event) error

	// GetEvent возвращает мероприятие по идентификатору.
	GetEvent(ctx context.Context, id string) (*model.Event, error)

	// ListEvents возвращает список мероприятий с фильтрацией по категории, статусу и диапазону дат.
	// Пустые параметры означают отсутствие фильтра.
	ListEvents(ctx context.Context, category string, status model.EventStatus, dateFrom, dateTo *time.Time) ([]*model.Event, error)

	// UpdateEvent обновляет данные мероприятия после валидации.
	UpdateEvent(ctx context.Context, event *model.Event) error

	// DeleteEvent удаляет мероприятие по идентификатору.
	DeleteEvent(ctx context.Context, id string) error

	// CountEvents возвращает общее количество мероприятий.
	CountEvents(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------------------
// ContestStore — интерфейс хранилища конкурсов и грантов.
// ---------------------------------------------------------------------------

// ContestStore определяет операции CRUD для конкурсов/грантов.
type ContestStore interface {
	// CreateContest создаёт конкурс/грант после валидации.
	CreateContest(ctx context.Context, contest *model.Contest) error

	// GetContest возвращает конкурс/грант по идентификатору.
	GetContest(ctx context.Context, id string) (*model.Contest, error)

	// ListContests возвращает список конкурсов/грантов с фильтрацией по категории и статусу.
	// Пустые параметры означают отсутствие фильтра.
	ListContests(ctx context.Context, category string, status model.ContestStatus) ([]*model.Contest, error)

	// UpdateContest обновляет данные конкурса/гранта после валидации.
	UpdateContest(ctx context.Context, contest *model.Contest) error

	// DeleteContest удаляет конкурс/грант по идентификатору.
	DeleteContest(ctx context.Context, id string) error

	// CountActiveContests возвращает количество активных конкурсов.
	CountActiveContests(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------------------
// FAQStore — интерфейс хранилища часто задаваемых вопросов.
// ---------------------------------------------------------------------------

// FAQStore определяет операции CRUD для элементов FAQ.
type FAQStore interface {
	// CreateFAQItem создаёт элемент FAQ после валидации.
	CreateFAQItem(ctx context.Context, item *model.FAQItem) error

	// GetFAQItem возвращает элемент FAQ по идентификатору.
	GetFAQItem(ctx context.Context, id string) (*model.FAQItem, error)

	// ListFAQItems возвращает элементы FAQ указанной категории.
	// Пустой category означает возврат всех элементов.
	ListFAQItems(ctx context.Context, category string) ([]*model.FAQItem, error)

	// UpdateFAQItem обновляет элемент FAQ после валидации.
	UpdateFAQItem(ctx context.Context, item *model.FAQItem) error

	// DeleteFAQItem удаляет элемент FAQ по идентификатору.
	DeleteFAQItem(ctx context.Context, id string) error

	// CountFAQItems возвращает общее количество элементов FAQ.
	CountFAQItems(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------------------
// TelegramStore — интерфейс хранилища постов из Telegram.
// ---------------------------------------------------------------------------

// TelegramStore определяет операции для постов из Telegram-каналов.
type TelegramStore interface {
	// CreateTelegramPost создаёт запись поста из Telegram после валидации.
	CreateTelegramPost(ctx context.Context, post *model.TelegramPost) error

	// ListTelegramPosts возвращает посты указанного канала, не более limit штук.
	// Пустой channel означает возврат постов со всех каналов.
	ListTelegramPosts(ctx context.Context, channel string, limit int) ([]*model.TelegramPost, error)

	// GetLatestPostDate возвращает дату самого свежего поста в канале.
	// Если посты отсутствуют, возвращает nil без ошибки.
	GetLatestPostDate(ctx context.Context, channel string) (*time.Time, error)

	// CountPosts возвращает количество постов в канале.
	// Пустой channel означает подсчёт по всем каналам.
	CountPosts(ctx context.Context, channel string) (int, error)
}

// ---------------------------------------------------------------------------
// ResidentStore — интерфейс хранилища резидентов.
// ---------------------------------------------------------------------------

// ResidentStore определяет операции CRUD для записей реестра резидентов.
type ResidentStore interface {
	// CreateResident создаёт запись резидента после валидации.
	CreateResident(ctx context.Context, resident *model.Resident) error

	// GetResident возвращает запись резидента по идентификатору.
	GetResident(ctx context.Context, id string) (*model.Resident, error)

	// ListResidents возвращает список резидентов с фильтрацией по отрасли, статусу и текстовому запросу.
	// Пустые параметры означают отсутствие фильтра. Query ищет по имени и ИНН.
	ListResidents(ctx context.Context, industry string, status model.ResidentStatus, query string) ([]*model.Resident, error)

	// UpdateResident обновляет данные резидента после валидации.
	UpdateResident(ctx context.Context, resident *model.Resident) error

	// DeleteResident удаляет запись резидента по идентификатору.
	DeleteResident(ctx context.Context, id string) error

	// CountResidents возвращает общее количество резидентов.
	CountResidents(ctx context.Context) (int, error)
}

// ---------------------------------------------------------------------------
// DocumentLinkStore — интерфейс хранилища связей между документами.
// ---------------------------------------------------------------------------

// DocumentLinkStore определяет операции CRUD для связей документов.
type DocumentLinkStore interface {
	// CreateDocumentLink создаёт связь между документами после валидации.
	CreateDocumentLink(ctx context.Context, link *model.DocumentLink) error

	// GetDocumentLinks возвращает связи указанного документа опционально фильтруя по типу.
	// Пустой linkType означает возврат всех типов связей.
	GetDocumentLinks(ctx context.Context, documentID string, linkType model.DocumentLinkType) ([]*model.DocumentLink, error)

	// DeleteDocumentLink удаляет связь по идентификатору.
	DeleteDocumentLink(ctx context.Context, id string) error

	// ListAllLinks возвращает все существующие связи между документами.
	ListAllLinks(ctx context.Context) ([]*model.DocumentLink, error)
}
