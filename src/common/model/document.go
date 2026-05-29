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
