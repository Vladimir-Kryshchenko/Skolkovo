// Package health отслеживает «свежесть» источников: когда каждый источник
// (документы, новости, мероприятия, конкурсы, FAQ, Telegram, льготы, НПА, загрузка
// файлов) последний раз успешно обновлялся, сколько элементов вернул и не сыпет ли
// ошибками. Это SLA-мониторинг: если источник «протух» или начал стабильно падать,
// консультант узнаёт об этом раньше, чем база тихо устареет.
package health

import (
	"context"
	"time"
)

// Status — состояние источника.
type Status string

const (
	StatusOK      Status = "ok"      // недавно успешно обновлялся
	StatusStale   Status = "stale"   // давно нет успешного обновления
	StatusFailing Status = "failing" // несколько прогонов подряд с ошибкой
	StatusUnknown Status = "unknown" // ещё ни разу не запускался
)

// Source — состояние одного источника.
type Source struct {
	Name                string     `json:"name"`
	LastRunAt           time.Time  `json:"last_run_at"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	ItemsLastRun        int        `json:"items_last_run"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastError           string     `json:"last_error,omitempty"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// State вычисляет статус источника на момент now с порогом устаревания staleAfter.
func (s Source) State(staleAfter time.Duration, now time.Time) Status {
	if s.ConsecutiveFailures >= 3 {
		return StatusFailing
	}
	if s.LastSuccessAt == nil {
		return StatusUnknown
	}
	if now.Sub(*s.LastSuccessAt) > staleAfter {
		return StatusStale
	}
	return StatusOK
}

// Store — хранилище состояния источников.
type Store interface {
	// Record фиксирует результат прогона источника. runErr=nil означает успех.
	Record(ctx context.Context, name string, items int, runErr error) error
	// List возвращает состояние всех источников.
	List(ctx context.Context) ([]Source, error)
	// Get возвращает состояние одного источника.
	Get(ctx context.Context, name string) (Source, error)
}
