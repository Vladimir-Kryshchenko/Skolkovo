// Package model описывает доменные сущности «База Сколково».
package model

import "time"

// Status — статус жизненного цикла документа.
type Status string

const (
	StatusPending  Status = "на_проверке"
	StatusActive   Status = "действует"
	StatusOutdated Status = "устарел"
	StatusArchived Status = "архив"
	StatusRejected Status = "отклонён"
)

// Document — запись реестра документов (см. Документы_Сколково/Метаданные/реестр_документов.schema.json).
type Document struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	SourceURL    string     `json:"source_url"`
	LocalPath    string     `json:"local_path,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	FetchedAt    time.Time  `json:"fetched_at"`
	Status       Status     `json:"status"`
	Category     string     `json:"category,omitempty"`
	VersionLabel string     `json:"version_label,omitempty"`
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidTo      *time.Time `json:"valid_to,omitempty"`
	Supersedes   string     `json:"supersedes,omitempty"`
	FileHash     string     `json:"file_hash"`
	Indexed      bool       `json:"indexed"`
}

// IsRetrievable сообщает, должен ли документ участвовать в поиске RAG.
func (d Document) IsRetrievable() bool {
	return d.Status == StatusActive
}

// Chunk — фрагмент документа для индексации в векторной БД.
type Chunk struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	Index      int       `json:"chunk_index"`
	Text       string    `json:"text"`
	Embedding  []float32 `json:"-"`
}

// SchedulerSettings — настройки планировщика сбора данных.
type SchedulerSettings struct {
	// Enabled — включён ли автоматический сбор
	Enabled bool `json:"enabled"`
	// IntervalDays — интервал между запусками (в днях)
	IntervalDays int `json:"interval_days"`
	// LastRun — время последнего запуска
	LastRun *time.Time `json:"last_run,omitempty"`
	// NextRun — запланированное время следующего запуска
	NextRun *time.Time `json:"next_run,omitempty"`
	// AutoApprove — автоматически подтверждать документы со статусом "действует"
	AutoApprove bool `json:"auto_approve"`
	// AutoIndex — автоматически индексировать подтверждённые документы
	AutoIndex bool `json:"auto_index"`
	// AutoValidate — запускать валидацию после сбора
	AutoValidate bool `json:"auto_validate"`
}

// CollectorReport — отчёт одного цикла сбора данных.
type CollectorReport struct {
	ID               string    `json:"id"`
	StartedAt        time.Time `json:"started_at"`
	FinishedAt       time.Time `json:"finished_at"`
	DocumentsNew     int       `json:"documents_new"`
	DocumentsUpd     int       `json:"documents_updated"`
	DocumentsSame    int       `json:"documents_same"`
	DocumentsRemoved int       `json:"documents_removed"`
	FilesDownloaded  int       `json:"files_downloaded"`
	FilesErrors      int       `json:"files_errors"`
	Indexed          int       `json:"indexed"`
	ValidationErrors int       `json:"validation_errors"`
	Status           string    `json:"status"` // running, done, error
	Error            string    `json:"error,omitempty"`
}

// ValidationReport — отчёт валидации.
type ValidationReport struct {
	TotalDocs    int      `json:"total_docs"`
	ValidDocs    int      `json:"valid_docs"`
	InvalidDocs  int      `json:"invalid_docs"`
	MissingFiles int      `json:"missing_files"`
	StaleDocs    []string `json:"stale_docs,omitempty"` // ID документов, устаревших на источнике
}

// ChangeEntry — запись об изменении в документе.
type ChangeEntry struct {
	DocumentID  string    `json:"document_id"`
	ChangeType  string    `json:"change_type"` // new, updated, removed, status_changed
	OldValue    string    `json:"old_value,omitempty"`
	NewValue    string    `json:"new_value,omitempty"`
	DetectedAt  time.Time `json:"detected_at"`
	Description string    `json:"description"`
}
