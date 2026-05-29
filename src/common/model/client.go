// Package model описывает доменные сущности системы управления резидентством.
package model

import (
	"encoding/json"
	"time"
)

// ResidencyStage — стадия жизненного цикла резидента.
type ResidencyStage string

const (
	StageApplication      ResidencyStage = "подача_заявки"
	StageExamination      ResidencyStage = "экспертиза"
	StageDecision         ResidencyStage = "решение"
	StageContract         ResidencyStage = "договор"
	StageResident         ResidencyStage = "резидент"
	StageReporting        ResidencyStage = "отчётность"
	StageExtension        ResidencyStage = "продление"
	StageExit             ResidencyStage = "выход"
)

// allowedTransitions описывает допустимые переходы между стадиями.
var allowedTransitions = map[ResidencyStage][]ResidencyStage{
	StageApplication: {StageExamination},
	StageExamination: {StageDecision, StageApplication}, // возврат на доработку
	StageDecision:    {StageContract, StageApplication}, // отказ → повторная подача
	StageContract:    {StageResident, StageApplication},
	StageResident:    {StageReporting, StageExit},
	StageReporting:   {StageExtension, StageExit},
	StageExtension:   {StageResident, StageExit},
	StageExit:        {}, // терминальная стадия
}

// CanTransition проверяет, допустим ли переход from → to.
func (s ResidencyStage) CanTransition(to ResidencyStage) bool {
	next, ok := allowedTransitions[s]
	if !ok {
		return false
	}
	for _, n := range next {
		if n == to {
			return true
		}
	}
	return false
}

// IsTerminal сообщает, является ли стадия терминальной.
func (s ResidencyStage) IsTerminal() bool {
	return s == StageExit
}

// Client — клиент / заявитель.
type Client struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	INN             string         `json:"inn"`
	ContactEmail    string         `json:"contact_email"`
	ContactPhone    string         `json:"contact_phone"`
	ResidencyStage  ResidencyStage `json:"residency_stage"`
	TenantID        string         `json:"tenant_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// StageTransition — запись о переходе между стадиями.
type StageTransition struct {
	ID             string         `json:"id"`
	ClientID       string         `json:"client_id"`
	FromStage      ResidencyStage `json:"from_stage"`
	ToStage        ResidencyStage `json:"to_stage"`
	TransitionedAt time.Time      `json:"transitioned_at"`
	Notes          string         `json:"notes,omitempty"`
}

// ChecklistStepDef — определение шага чек-листа (хранится как JSON-массив в Checklist.Steps).
type ChecklistStepDef struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Order       int      `json:"order"`
	RequiredDoc []string `json:"required_docs"`
	DeadlineDays int     `json:"deadline_days"`
}

// ChecklistType — тип процедуры.
type ChecklistType string

const (
	ChecklistEntry     ChecklistType = "entry"
	ChecklistReporting ChecklistType = "reporting"
	ChecklistExtension ChecklistType = "extension"
	ChecklistExit      ChecklistType = "exit"
)

// Checklist — шаблон чек-листа процедуры.
type Checklist struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	ProcedureType ChecklistType   `json:"procedure_type"`
	Steps         json.RawMessage `json:"steps"` // []ChecklistStepDef
	Version       string          `json:"version"`
	CreatedAt     time.Time       `json:"created_at"`
}

// ParseSteps декодирует JSON шагов в слайс ChecklistStepDef.
func (c *Checklist) ParseSteps() ([]ChecklistStepDef, error) {
	var steps []ChecklistStepDef
	if c.Steps == nil {
		return steps, nil
	}
	return steps, json.Unmarshal(c.Steps, &steps)
}

// ChecklistStatus — статус чек-листа у клиента.
type ChecklistStatus string

const (
	ChecklistNotStarted  ChecklistStatus = "not_started"
	ChecklistInProgress  ChecklistStatus = "in_progress"
	ChecklistCompleted   ChecklistStatus = "completed"
)

// ClientChecklist — привязка чек-листа к клиенту.
type ClientChecklist struct {
	ID          string          `json:"id"`
	ClientID    string          `json:"client_id"`
	ChecklistID string          `json:"checklist_id"`
	Status      ChecklistStatus `json:"status"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// StepStatus — статус шага у клиента.
type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepInProgress StepStatus = "in_progress"
	StepDone       StepStatus = "done"
	StepSkipped    StepStatus = "skipped"
)

// ChecklistStepStatus — статус конкретного шага чек-листа.
type ChecklistStepStatus struct {
	ID                string     `json:"id"`
	ClientChecklistID string     `json:"client_checklist_id"`
	StepIndex         int        `json:"step_index"`
	Status            StepStatus `json:"status"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	Notes             string     `json:"notes,omitempty"`
}

// DeadlineType — тип дедлайна.
type DeadlineType string

const (
	DeadlineReporting        DeadlineType = "reporting"
	DeadlineExtension        DeadlineType = "extension"
	DeadlineApplication      DeadlineType = "application"
	DeadlineDocumentSubmission DeadlineType = "document_submission"
)

// DeadlineStatus — статус дедлайна.
type DeadlineStatus string

const (
	DeadlineUpcoming  DeadlineStatus = "upcoming"
	DeadlineOverdue   DeadlineStatus = "overdue"
	DeadlineCompleted DeadlineStatus = "completed"
)

// Deadline — дедлайн, привязанный к клиенту.
type Deadline struct {
	ID               string         `json:"id"`
	ClientID         string         `json:"client_id"`
	Title            string         `json:"title"`
	DueDate          time.Time      `json:"due_date"`
	Type             DeadlineType   `json:"type"`
	Status           DeadlineStatus `json:"status"`
	NotificationSent bool           `json:"notification_sent"`
	CreatedAt        time.Time      `json:"created_at"`
}

// IsOverdue проверяет, просрочен ли дедлайн.
func (d *Deadline) IsOverdue(now time.Time) bool {
	return d.Status != DeadlineCompleted && now.After(d.DueDate)
}

// DocumentTemplate — шаблон документа.
type DocumentTemplate struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	TemplateFile  string          `json:"template_file"`
	Variables     json.RawMessage `json:"variables"` // []string
	Version       string          `json:"version"`
	CreatedAt     time.Time       `json:"created_at"`
}

// ParseVariables декодирует JSON переменных в слайс строк.
func (dt *DocumentTemplate) ParseVariables() ([]string, error) {
	var vars []string
	if dt.Variables == nil {
		return vars, nil
	}
	return vars, json.Unmarshal(dt.Variables, &vars)
}

// ClientDocRole — роль документа у клиента.
type ClientDocRole string

const (
	DocRoleRequired ClientDocRole = "required"
	DocRoleOptional ClientDocRole = "optional"
	DocRoleSubmitted ClientDocRole = "submitted"
)

// ClientDocStatus — статус документа у клиента.
type ClientDocStatus string

const (
	DocPending  ClientDocStatus = "pending"
	DocSubmitted ClientDocStatus = "submitted"
	DocApproved ClientDocStatus = "approved"
	DocRejected ClientDocStatus = "rejected"
)

// ClientDocument — связь документа с клиентом.
type ClientDocument struct {
	ID          string          `json:"id"`
	ClientID    string          `json:"client_id"`
	DocumentID  string          `json:"document_id"`
	Role        ClientDocRole   `json:"role"`
	Status      ClientDocStatus `json:"status"`
	SubmittedAt *time.Time      `json:"submitted_at,omitempty"`
}

// Tenant — мульти-тенант.
type Tenant struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	APIKey    string          `json:"api_key"`
	Settings  json.RawMessage `json:"settings,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Active    bool            `json:"active"`
}

// ParseSettings декодирует JSON настроек тенанта в map.
func (t *Tenant) ParseSettings() (map[string]interface{}, error) {
	settings := make(map[string]interface{})
	if t.Settings == nil {
		return settings, nil
	}
	return settings, json.Unmarshal(t.Settings, &settings)
}
