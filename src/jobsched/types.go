// Package jobsched — планировщик периодических заданий сбора с настраиваемым из
// админки интервалом по каждому источнику, журналом запусков и учётом расхода
// токенов ИИ-агентов.
//
// Источник истины по расписанию — таблица scheduler_jobs (миграция 014); журнал
// прогонов — scheduler_runs; учёт токенов — ai_usage_log. Раннер (runner.go)
// читает конфигурацию из БД на каждом тике, поэтому изменение интервала в админке
// применяется без перезапуска сервиса.
package jobsched

import "time"

// RunStatus — статус прогона задания.
type RunStatus string

const (
	StatusRunning RunStatus = "running"
	StatusSuccess RunStatus = "success"
	StatusError   RunStatus = "error"
	StatusPartial RunStatus = "partial"
	StatusSkipped RunStatus = "skipped"
)

// Trigger — чем инициирован прогон.
type Trigger string

const (
	TriggerSchedule Trigger = "schedule"
	TriggerManual   Trigger = "manual"
	TriggerStartup  Trigger = "startup"
)

// JobConfig — конфигурация расписания одного задания.
type JobConfig struct {
	Name            string     `json:"name"`
	Title           string     `json:"title"`
	IntervalSeconds int64      `json:"interval_seconds"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	LastStatus      string     `json:"last_status"`
	LastError       string     `json:"last_error"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Interval возвращает интервал задания как time.Duration.
func (j JobConfig) Interval() time.Duration {
	return time.Duration(j.IntervalSeconds) * time.Second
}

// RunResult — итог выполнения JobFunc (счётчики и произвольная разбивка).
type RunResult struct {
	ItemsNew     int            `json:"items_new"`
	ItemsUpdated int            `json:"items_updated"`
	ItemsTotal   int            `json:"items_total"`
	Details      map[string]any `json:"details,omitempty"`
}

// Run — запись журнала прогона.
type Run struct {
	ID           string         `json:"id"`
	JobName      string         `json:"job_name"`
	Trigger      string         `json:"trigger"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	DurationMs   int64          `json:"duration_ms"`
	Status       string         `json:"status"`
	ItemsNew     int            `json:"items_new"`
	ItemsUpdated int            `json:"items_updated"`
	ItemsTotal   int            `json:"items_total"`
	Error        string         `json:"error,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
}

// AIUsage — запись учёта расхода токенов одного ИИ-вызова.
type AIUsage struct {
	ID               string    `json:"id"`
	RunID            string    `json:"run_id,omitempty"`
	TenantID         string    `json:"tenant_id,omitempty"`
	AgentType        string    `json:"agent_type"`
	ModelID          string    `json:"model_id"`
	Provider         string    `json:"provider"`
	ModelLabel       string    `json:"model_label"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	DurationMs       int64     `json:"duration_ms"`
	Success          bool      `json:"success"`
	Error            string    `json:"error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RunFilter — фильтр выборки журнала прогонов.
type RunFilter struct {
	JobName   string
	Status    string
	SinceDays int
	Limit     int
}

// AIUsageFilter — фильтр выборки учёта токенов.
type AIUsageFilter struct {
	RunID     string
	TenantID  string
	AgentType string
	ModelID   string
	SinceDays int
	Limit     int
}

// TenantUsageStat — сводка расхода токенов и стоимость по одному тенанту.
type TenantUsageStat struct {
	TenantID         string  `json:"tenant_id"`
	TenantName       string  `json:"tenant_name"`
	Calls            int     `json:"calls"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// UsageAgg — агрегат расхода токенов (по модели или по агенту).
type UsageAgg struct {
	Key              string `json:"key"`   // model_id или agent_type
	Label            string `json:"label"` // человекочитаемая подпись
	Calls            int    `json:"calls"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Errors           int    `json:"errors"`
	AvgDurationMs    int64  `json:"avg_duration_ms"`
}
