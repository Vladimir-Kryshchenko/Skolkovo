// Package changes ведёт ленту изменений базы знаний: какие документы, новости,
// конкурсы, НПА и льготы появились, обновились или устарели. Лента — первоклассная
// сущность: её читают консультант (через MCP get_recent_changes), Telegram-нотификатор
// и клиентские агенты, чтобы всегда видеть «что изменилось в Сколково».
package changes

import (
	"context"
	"time"
)

// Kind — тип изменения сущности.
type Kind string

const (
	KindNew      Kind = "new"      // сущность впервые заведена в базу
	KindUpdated  Kind = "updated"  // содержимое/метаданные изменились
	KindOutdated Kind = "outdated" // переведена в статус «устарела/утратила силу»
	KindRemoved  Kind = "removed"  // удалена из источника
)

// Известные типы сущностей ленты изменений.
const (
	EntityDocument   = "document"
	EntityNews       = "news"
	EntityEvent      = "event"
	EntityContest    = "contest"
	EntityNPA        = "npa"
	EntityPreference = "preference"
	EntityFAQ        = "faq"
	EntityTelegram   = "telegram"
)

// Notify фиксирует изменение во всех переданных рекордерах (nil-элементы пропускаются).
// Удобно для парсеров, принимающих переменное число рекордеров (обычно 0 или 1).
func Notify(ctx context.Context, recs []Recorder, ev Event) {
	for _, r := range recs {
		if r == nil {
			continue
		}
		_ = r.Record(ctx, ev)
	}
}

// Event — единичное зафиксированное изменение.
type Event struct {
	ID         string    `json:"id"`
	EntityType string    `json:"entity_type"` // document | news | event | contest | npa | preference | faq
	EntityID   string    `json:"entity_id"`
	Title      string    `json:"title"`
	Category   string    `json:"category,omitempty"`
	Kind       Kind      `json:"kind"`
	SourceURL  string    `json:"source_url,omitempty"`
	Summary    string    `json:"summary,omitempty"` // краткое человекочитаемое описание изменения
	DetectedAt time.Time `json:"detected_at"`
	Notified   bool      `json:"notified"`
}

// Filter ограничивает выборку ленты изменений. Нулевые поля — без ограничения.
type Filter struct {
	Since      time.Time
	EntityType string
	Category   string
	Limit      int
}

// Recorder фиксирует изменение. Реализуется Store и передаётся в парсеры,
// чтобы те не зависели от конкретного хранилища. Парсеры держат поле типа
// Recorder и пропускают вызов, если оно nil.
type Recorder interface {
	Record(ctx context.Context, ev Event) error
}

// Store — хранилище ленты изменений.
type Store interface {
	Recorder
	// Recent возвращает последние изменения по фильтру (по убыванию времени).
	Recent(ctx context.Context, f Filter) ([]Event, error)
	// Unnotified возвращает изменения, по которым ещё не отправлено уведомление.
	Unnotified(ctx context.Context, limit int) ([]Event, error)
	// MarkNotified помечает изменения как отправленные.
	MarkNotified(ctx context.Context, ids []string) error
}
