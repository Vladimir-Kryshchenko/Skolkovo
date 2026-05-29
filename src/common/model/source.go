// Package model описывает доменные сущности расширенных источников данных «База Сколково».
package model

import "time"

// EventStatus — статус мероприятия.
type EventStatus string

const (
	EventActive    EventStatus = "active"
	EventPast      EventStatus = "past"
	EventCancelled EventStatus = "cancelled"
)

// Event — мероприятие.
type Event struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	EventDate   time.Time   `json:"event_date"`
	EventEndDate time.Time  `json:"event_end_date,omitempty"`
	Location    string      `json:"location,omitempty"`
	SourceURL   string      `json:"source_url"`
	Status      EventStatus `json:"status"`
	Category    string      `json:"category,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// IsUpcoming проверяет, является ли мероприятие предстоящим.
func (e *Event) IsUpcoming(now time.Time) bool {
	return e.EventDate.After(now)
}

// ContestStatus — статус конкурса/гранта.
type ContestStatus string

const (
	ContestActive         ContestStatus = "active"
	ContestClosed         ContestStatus = "closed"
	ContestWinnerSelected ContestStatus = "winner_selected"
)

// Contest — конкурс или грант.
type Contest struct {
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Description   string        `json:"description,omitempty"`
	StartDate     time.Time     `json:"start_date"`
	EndDate       time.Time     `json:"end_date"`
	Requirements  string        `json:"requirements,omitempty"`
	Prize         string        `json:"prize,omitempty"`
	SourceURL     string        `json:"source_url"`
	Status        ContestStatus `json:"status"`
	Category      string        `json:"category,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
}

// IsOverdue проверяет, истёк ли срок конкурса.
func (c *Contest) IsOverdue(now time.Time) bool {
	return c.Status != ContestWinnerSelected && now.After(c.EndDate)
}

// FAQItem — элемент FAQ.
type FAQItem struct {
	ID        string    `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Category  string    `json:"category,omitempty"`
	SourceURL string    `json:"source_url"`
	CreatedAt time.Time `json:"created_at"`
}

// TelegramPost — пост из Telegram.
type TelegramPost struct {
	ID          string    `json:"id"`
	Channel     string    `json:"channel"`
	Text        string    `json:"text"`
	PublishedAt time.Time `json:"published_at"`
	SourceURL   string    `json:"source_url"`
	MediaURLs   []string  `json:"media_urls,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ResidentStatus — статус резидента в реестре.
type ResidentStatus string

const (
	ResidentActive   ResidentStatus = "active"
	ResidentInactive ResidentStatus = "inactive"
)

// Resident — запись реестра резидентов.
type Resident struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	INN        string         `json:"inn"`
	Industry   string         `json:"industry,omitempty"`
	JoinDate   time.Time      `json:"join_date"`
	Status     ResidentStatus `json:"status"`
	SourceURL  string         `json:"source_url"`
	CreatedAt  time.Time      `json:"created_at"`
}

// DocumentLinkType — тип связи между документами.
type DocumentLinkType string

const (
	LinkReferences   DocumentLinkType = "references"
	LinkSupersedes   DocumentLinkType = "supersedes"
	LinkRelated      DocumentLinkType = "related"
)

// DocumentLink — связь между документами.
type DocumentLink struct {
	ID        string           `json:"id"`
	SourceID  string           `json:"source_id"`
	TargetID  string           `json:"target_id"`
	LinkType  DocumentLinkType `json:"link_type"`
	CreatedAt time.Time        `json:"created_at"`
}
