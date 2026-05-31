// Package store хранит интерфейсы хранилищ для управления клиентами, чек-листами,
// дедлайнами, шаблонами документов и мульти-тенантами.
package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
)

// Ошибки валидации.
var (
	ErrInvalidINN        = errors.New("некорректный ИНН: должен быть 10 или 12 цифр")
	ErrInvalidEmail      = errors.New("некорректный email")
	ErrInvalidPhone      = errors.New("некорректный телефон: должен содержать только цифры, +, -, пробелы и скобки")
	ErrEmptyField        = errors.New("обязательное поле не заполнено")
	ErrInvalidTransition = errors.New("недопустимый переход между стадиями")
	ErrInvalidDeadline   = errors.New("дата дедлайна не может быть в прошлом")
)

// emailRe — упрощённая проверка формата email.
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// phoneRe — допустимые символы в номере телефона.
var phoneRe = regexp.MustCompile(`^[\d\s\-\+\(\)]+$`)

// validateClient проверяет обязательные поля клиента.
func validateClient(c *model.Client) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("Name: %w", ErrEmptyField)
	}
	if c.INN == "" || (len(c.INN) != 10 && len(c.INN) != 12) {
		return ErrInvalidINN
	}
	for _, r := range c.INN {
		if r < '0' || r > '9' {
			return ErrInvalidINN
		}
	}
	if c.ContactEmail != "" && !emailRe.MatchString(c.ContactEmail) {
		return ErrInvalidEmail
	}
	if c.ContactPhone != "" && !phoneRe.MatchString(c.ContactPhone) {
		return ErrInvalidPhone
	}
	// TenantID is optional — auto-assigned to default tenant if empty.
	// if strings.TrimSpace(c.TenantID) == "" {
	// 	return fmt.Errorf("TenantID: %w", ErrEmptyField)
	// }
	return nil
}

// validateStageTransition проверяет допустимость перехода.
func validateStageTransition(t *model.StageTransition) error {
	if strings.TrimSpace(t.ClientID) == "" {
		return fmt.Errorf("ClientID: %w", ErrEmptyField)
	}
	if !t.FromStage.CanTransition(t.ToStage) {
		return fmt.Errorf("переход %s → %s: %w", t.FromStage, t.ToStage, ErrInvalidTransition)
	}
	return nil
}

// validateChecklist проверяет шаблон чек-листа.
func validateChecklist(c *model.Checklist) error {
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("Title: %w", ErrEmptyField)
	}
	if c.ProcedureType == "" {
		return fmt.Errorf("ProcedureType: %w", ErrEmptyField)
	}
	return nil
}

// validateClientChecklist проверяет привязку чек-листа к клиенту.
func validateClientChecklist(cc *model.ClientChecklist) error {
	if strings.TrimSpace(cc.ClientID) == "" {
		return fmt.Errorf("ClientID: %w", ErrEmptyField)
	}
	if strings.TrimSpace(cc.ChecklistID) == "" {
		return fmt.Errorf("ChecklistID: %w", ErrEmptyField)
	}
	return nil
}

// validateDeadline проверяет дедлайн.
func validateDeadline(d *model.Deadline, now time.Time) error {
	if strings.TrimSpace(d.ClientID) == "" {
		return fmt.Errorf("ClientID: %w", ErrEmptyField)
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("Title: %w", ErrEmptyField)
	}
	if d.DueDate.Before(now) {
		return ErrInvalidDeadline
	}
	return nil
}

// validateTemplate проверяет шаблон документа.
func validateTemplate(t *model.DocumentTemplate) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("Name: %w", ErrEmptyField)
	}
	if strings.TrimSpace(t.Type) == "" {
		return fmt.Errorf("Type: %w", ErrEmptyField)
	}
	if strings.TrimSpace(t.TemplateFile) == "" {
		return fmt.Errorf("TemplateFile: %w", ErrEmptyField)
	}
	return nil
}

// validateClientDocument проверяет связь документа с клиентом.
func validateClientDocument(cd *model.ClientDocument) error {
	if strings.TrimSpace(cd.ClientID) == "" {
		return fmt.Errorf("ClientID: %w", ErrEmptyField)
	}
	if strings.TrimSpace(cd.DocumentID) == "" {
		return fmt.Errorf("DocumentID: %w", ErrEmptyField)
	}
	return nil
}

// validateTenant проверяет тенант.
func validateTenant(t *model.Tenant) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("Name: %w", ErrEmptyField)
	}
	if strings.TrimSpace(t.APIKey) == "" {
		return fmt.Errorf("APIKey: %w", ErrEmptyField)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ClientStore — интерфейс хранилища клиентов и истории переходов стадий.
// ---------------------------------------------------------------------------

// ClientStore определяет операции CRUD для сущности Client и историю переходов.
type ClientStore interface {
	// CreateClient создаёт нового клиента после валидации.
	CreateClient(ctx context.Context, client *model.Client) error

	// GetClient возвращает клиента по идентификатору.
	GetClient(ctx context.Context, id string) (*model.Client, error)

	// GetClientByINN возвращает клиента по ИНН.
	GetClientByINN(ctx context.Context, inn string) (*model.Client, error)

	// UpdateClient обновляет данные клиента после валидации.
	UpdateClient(ctx context.Context, client *model.Client) error

	// DeleteClient удаляет клиента по идентификатору.
	DeleteClient(ctx context.Context, id string) error

	// ListClients возвращает список клиентов tenant'а, опционально фильтруя по стадии.
	// Параметр stage может быть пустым — тогда фильтрация не применяется.
	ListClients(ctx context.Context, tenantID string, stage model.ResidencyStage) ([]*model.Client, error)

	// AddStageTransition добавляет запись о переходе между стадиями.
	AddStageTransition(ctx context.Context, transition *model.StageTransition) error

	// GetStageHistory возвращает всю историю переходов для клиента.
	GetStageHistory(ctx context.Context, clientID string) ([]*model.StageTransition, error)
}

// ---------------------------------------------------------------------------
// ChecklistStore — интерфейс хранилища чек-листов и статусов шагов.
// ---------------------------------------------------------------------------

// ChecklistStore определяет операции для шаблонов чек-листов и их выполнения клиентами.
type ChecklistStore interface {
	// CreateChecklist создаёт шаблон чек-листа после валидации.
	CreateChecklist(ctx context.Context, checklist *model.Checklist) error

	// GetChecklist возвращает шаблон чек-листа по идентификатору.
	GetChecklist(ctx context.Context, id string) (*model.Checklist, error)

	// ListChecklists возвращает все шаблоны указанного типа процедуры.
	// Параметр procedureType может быть пустым — тогда возвращаются все шаблоны.
	ListChecklists(ctx context.Context, procedureType model.ChecklistType) ([]*model.Checklist, error)

	// CreateClientChecklist привязывает чек-лист к клиенту после валидации.
	CreateClientChecklist(ctx context.Context, cc *model.ClientChecklist) error

	// GetClientChecklist возвращает привязку чек-листа к клиенту по идентификатору.
	GetClientChecklist(ctx context.Context, id string) (*model.ClientChecklist, error)

	// GetClientChecklists возвращает все чек-листы, привязанные к клиенту.
	GetClientChecklists(ctx context.Context, clientID string) ([]*model.ClientChecklist, error)

	// UpdateClientChecklist обновляет статус привязки чек-листа.
	UpdateClientChecklist(ctx context.Context, cc *model.ClientChecklist) error

	// CreateStepStatus создаёт статус шага чек-листа.
	CreateStepStatus(ctx context.Context, status *model.ChecklistStepStatus) error

	// UpdateStepStatus обновляет статус и заметки шага.
	UpdateStepStatus(ctx context.Context, id string, status model.StepStatus, notes string) error

	// GetStepStatuses возвращает все статусы шагов для привязки чек-листа.
	GetStepStatuses(ctx context.Context, clientChecklistID string) ([]*model.ChecklistStepStatus, error)
}

// ---------------------------------------------------------------------------
// DeadlineStore — интерфейс хранилища дедлайнов.
// ---------------------------------------------------------------------------

// DeadlineStore определяет операции CRUD для дедлайнов и уведомлений.
type DeadlineStore interface {
	// CreateDeadline создаёт дедлайн после валидации (дата не в прошлом).
	CreateDeadline(ctx context.Context, deadline *model.Deadline) error

	// GetDeadline возвращает дедлайн по идентификатору.
	GetDeadline(ctx context.Context, id string) (*model.Deadline, error)

	// UpdateDeadline обновляет данные дедлайна.
	UpdateDeadline(ctx context.Context, deadline *model.Deadline) error

	// ListDeadlines возвращает дедлайны клиента, которые наступят в ближайшие daysAhead дней.
	// Параметр daysAhead <= 0 означает отсутствие верхнего ограничения.
	ListDeadlines(ctx context.Context, clientID string, daysAhead int) ([]*model.Deadline, error)

	// ListOverdueDeadlines возвращает все просроченные дедлайны (status = overdue или дата прошла).
	ListOverdueDeadlines(ctx context.Context) ([]*model.Deadline, error)

	// MarkNotificationSent помечает дедлайн как уведомлённый.
	MarkNotificationSent(ctx context.Context, id string) error
}

// ---------------------------------------------------------------------------
// TemplateStore — интерфейс хранилища шаблонов документов.
// ---------------------------------------------------------------------------

// TemplateStore определяет операции CRUD для шаблонов документов.
type TemplateStore interface {
	// CreateTemplate создаёт шаблон документа после валидации.
	CreateTemplate(ctx context.Context, template *model.DocumentTemplate) error

	// GetTemplate возвращает шаблон по идентификатору.
	GetTemplate(ctx context.Context, id string) (*model.DocumentTemplate, error)

	// ListTemplates возвращает шаблоны указанного типа.
	// Параметр templateType может быть пустым — тогда возвращаются все шаблоны.
	ListTemplates(ctx context.Context, templateType string) ([]*model.DocumentTemplate, error)
}

// ---------------------------------------------------------------------------
// ClientDocumentStore — интерфейс хранилища связей документов с клиентами.
// ---------------------------------------------------------------------------

// ClientDocumentStore определяет операции CRUD для связей клиент—документ.
type ClientDocumentStore interface {
	// LinkDocument создаёт связь документа с клиентом после валидации.
	LinkDocument(ctx context.Context, clientDoc *model.ClientDocument) error

	// GetClientDocument возвращает связь по идентификатору.
	GetClientDocument(ctx context.Context, id string) (*model.ClientDocument, error)

	// ListClientDocuments возвращает все документы, привязанные к клиенту.
	ListClientDocuments(ctx context.Context, clientID string) ([]*model.ClientDocument, error)

	// UpdateClientDocument обновляет статус или роль связи.
	UpdateClientDocument(ctx context.Context, clientDoc *model.ClientDocument) error
}

// ---------------------------------------------------------------------------
// SubscriptionStore — интерфейс хранилища подписок клиентов на изменения.
// ---------------------------------------------------------------------------

// SubscriptionStore управляет подписками клиентов на категории уведомлений.
type SubscriptionStore interface {
	// GetSubscriptions возвращает список категорий, на которые подписан клиент.
	GetSubscriptions(ctx context.Context, clientID string) ([]string, error)

	// SetSubscriptions устанавливает полный набор подписок клиента,
	// заменяя предыдущие значения.
	SetSubscriptions(ctx context.Context, clientID string, categories []string) error
}

// ---------------------------------------------------------------------------
// TenantStore — интерфейс хранилища мульти-тенантов.
// ---------------------------------------------------------------------------

// TenantStore определяет операции CRUD для тенантов.
type TenantStore interface {
	// CreateTenant создаёт тенант после валидации.
	CreateTenant(ctx context.Context, tenant *model.Tenant) error

	// GetTenant возвращает тенант по идентификатору.
	GetTenant(ctx context.Context, id string) (*model.Tenant, error)

	// GetTenantByAPIKey возвращает тенант по API-ключу.
	GetTenantByAPIKey(ctx context.Context, apiKey string) (*model.Tenant, error)

	// ListTenants возвращает все тенанты.
	ListTenants(ctx context.Context) ([]*model.Tenant, error)

	// UpdateTenant обновляет данные тенанта.
	UpdateTenant(ctx context.Context, tenant *model.Tenant) error
}
