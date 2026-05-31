package model

import "time"

// DocVersion — снимок одной редакции документа (для семантического диффа
// между версиями в треке «Актуальность изменений»).
type DocVersion struct {
	ID            string    `json:"id"`
	DocumentID    string    `json:"document_id"`
	VersionNo     int       `json:"version_no"`
	FileHash      string    `json:"file_hash"`
	ExtractedText string    `json:"extracted_text"`
	ArchivedPath  string    `json:"archived_path,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// ClientNotification — персональное уведомление резидента о касающемся его
// изменении. Читается порталом клиента (inbox), питается анализатором актуальности.
type ClientNotification struct {
	ID            string     `json:"id"`
	ClientID      string     `json:"client_id"`
	ChangeEventID string     `json:"change_event_id,omitempty"`
	Severity      string     `json:"severity"` // info | warning | critical
	Title         string     `json:"title"`
	Body          string     `json:"body,omitempty"`
	URL           string     `json:"url,omitempty"`
	Read          bool       `json:"read"`
	CreatedAt     time.Time  `json:"created_at"`
	EmailSentAt   *time.Time `json:"email_sent_at,omitempty"`
	TGSentAt      *time.Time `json:"tg_sent_at,omitempty"`
}
