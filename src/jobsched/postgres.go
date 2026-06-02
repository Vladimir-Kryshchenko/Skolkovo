package jobsched

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound возвращается, когда запрошенная запись отсутствует.
var ErrNotFound = errors.New("запись не найдена")

// schema — DDL планировщика (идемпотентно). Дублирует миграцию 014, чтобы
// хранилище работало и без отдельного прогона миграций (как в health/changes).
const schema = `
CREATE TABLE IF NOT EXISTS scheduler_jobs (
    name             TEXT        PRIMARY KEY,
    title            TEXT        NOT NULL DEFAULT '',
    interval_seconds BIGINT      NOT NULL DEFAULT 21600,
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    last_run_at      TIMESTAMPTZ,
    next_run_at      TIMESTAMPTZ,
    last_status      TEXT        NOT NULL DEFAULT '',
    last_error       TEXT        NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS scheduler_runs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_name      TEXT        NOT NULL,
    trigger       TEXT        NOT NULL DEFAULT 'schedule',
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ,
    duration_ms   BIGINT      NOT NULL DEFAULT 0,
    status        TEXT        NOT NULL DEFAULT 'running',
    items_new     INTEGER     NOT NULL DEFAULT 0,
    items_updated INTEGER     NOT NULL DEFAULT 0,
    items_total   INTEGER     NOT NULL DEFAULT 0,
    error         TEXT        NOT NULL DEFAULT '',
    details       JSONB
);
CREATE INDEX IF NOT EXISTS idx_scheduler_runs_job     ON scheduler_runs (job_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_scheduler_runs_started ON scheduler_runs (started_at DESC);
CREATE TABLE IF NOT EXISTS ai_usage_log (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id            UUID,
    agent_type        TEXT        NOT NULL DEFAULT '',
    model_id          TEXT        NOT NULL DEFAULT '',
    provider          TEXT        NOT NULL DEFAULT '',
    model_label       TEXT        NOT NULL DEFAULT '',
    prompt_tokens     INTEGER     NOT NULL DEFAULT 0,
    completion_tokens INTEGER     NOT NULL DEFAULT 0,
    total_tokens      INTEGER     NOT NULL DEFAULT 0,
    duration_ms       BIGINT      NOT NULL DEFAULT 0,
    success           BOOLEAN     NOT NULL DEFAULT TRUE,
    error             TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ai_usage_created ON ai_usage_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_usage_run     ON ai_usage_log (run_id) WHERE run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ai_usage_agent   ON ai_usage_log (agent_type, model_id);
`

// Store — хранилище планировщика поверх PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore применяет схему и возвращает хранилище.
func NewStore(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// ---------------------------------------------------------------------------
// scheduler_jobs
// ---------------------------------------------------------------------------

// UpsertJobDefault регистрирует задание с дефолтным интервалом, если его ещё нет.
// Существующую запись не трогает (интервал/enabled, заданные админом, сохраняются),
// обновляет только человекочитаемое название.
func (s *Store) UpsertJobDefault(ctx context.Context, name, title string, intervalSeconds int64) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO scheduler_jobs (name, title, interval_seconds, enabled, updated_at)
VALUES ($1, $2, $3, TRUE, now())
ON CONFLICT (name) DO UPDATE SET title = EXCLUDED.title`,
		name, title, intervalSeconds)
	return err
}

const jobCols = `SELECT name, title, interval_seconds, enabled, last_run_at, next_run_at,
       last_status, last_error, updated_at FROM scheduler_jobs`

// ListJobs возвращает все задания (по имени).
func (s *Store) ListJobs(ctx context.Context) ([]JobConfig, error) {
	rows, err := s.pool.Query(ctx, jobCols+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobConfig
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// GetJob возвращает одно задание по имени.
func (s *Store) GetJob(ctx context.Context, name string) (JobConfig, error) {
	row := s.pool.QueryRow(ctx, jobCols+` WHERE name = $1`, name)
	j, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobConfig{}, ErrNotFound
	}
	return j, err
}

// UpdateJobConfig сохраняет настройки расписания (интервал + вкл/выкл),
// заданные админом. Сбрасывает next_run_at, чтобы новый интервал применился сразу.
func (s *Store) UpdateJobConfig(ctx context.Context, name string, intervalSeconds int64, enabled bool) error {
	ct, err := s.pool.Exec(ctx, `
UPDATE scheduler_jobs
   SET interval_seconds = $2, enabled = $3, next_run_at = NULL, updated_at = now()
 WHERE name = $1`, name, intervalSeconds, enabled)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetJobRunState фиксирует итог прогона задания в его конфигурации.
func (s *Store) SetJobRunState(ctx context.Context, name string, lastRunAt, nextRunAt time.Time, status, lastErr string) error {
	_, err := s.pool.Exec(ctx, `
UPDATE scheduler_jobs
   SET last_run_at = $2, next_run_at = $3, last_status = $4, last_error = $5, updated_at = now()
 WHERE name = $1`, name, lastRunAt, nextRunAt, status, lastErr)
	return err
}

func scanJob(r scanner) (JobConfig, error) {
	var j JobConfig
	err := r.Scan(&j.Name, &j.Title, &j.IntervalSeconds, &j.Enabled, &j.LastRunAt,
		&j.NextRunAt, &j.LastStatus, &j.LastError, &j.UpdatedAt)
	return j, err
}

// ---------------------------------------------------------------------------
// scheduler_runs
// ---------------------------------------------------------------------------

// StartRun открывает запись прогона (status=running) и возвращает её id.
func (s *Store) StartRun(ctx context.Context, jobName string, trigger Trigger) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
INSERT INTO scheduler_runs (job_name, trigger, started_at, status)
VALUES ($1, $2, now(), 'running')
RETURNING id`, jobName, string(trigger)).Scan(&id)
	return id, err
}

// FinishRun закрывает запись прогона итогом.
func (s *Store) FinishRun(ctx context.Context, runID string, status RunStatus, res RunResult, errStr string, dur time.Duration) error {
	var details []byte
	if len(res.Details) > 0 {
		if b, err := json.Marshal(res.Details); err == nil {
			details = b
		}
	}
	_, err := s.pool.Exec(ctx, `
UPDATE scheduler_runs
   SET finished_at = now(), duration_ms = $2, status = $3,
       items_new = $4, items_updated = $5, items_total = $6, error = $7, details = $8
 WHERE id = $1`,
		runID, dur.Milliseconds(), string(status),
		res.ItemsNew, res.ItemsUpdated, res.ItemsTotal, errStr, details)
	return err
}

const runCols = `SELECT id, job_name, trigger, started_at, finished_at, duration_ms,
       status, items_new, items_updated, items_total, error, details FROM scheduler_runs`

// ListRuns возвращает журнал прогонов по фильтру (свежие сверху).
func (s *Store) ListRuns(ctx context.Context, f RunFilter) ([]Run, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	since := f.SinceDays
	if since <= 0 {
		since = 36500 // ~без ограничения
	}
	rows, err := s.pool.Query(ctx, runCols+`
 WHERE ($1 = '' OR job_name = $1)
   AND ($2 = '' OR status = $2)
   AND started_at >= now() - make_interval(days => $3)
 ORDER BY started_at DESC
 LIMIT $4`, f.JobName, f.Status, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRun возвращает один прогон по id.
func (s *Store) GetRun(ctx context.Context, id string) (Run, error) {
	row := s.pool.QueryRow(ctx, runCols+` WHERE id = $1`, id)
	r, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return r, err
}

func scanRun(r scanner) (Run, error) {
	var run Run
	var details []byte
	err := r.Scan(&run.ID, &run.JobName, &run.Trigger, &run.StartedAt, &run.FinishedAt,
		&run.DurationMs, &run.Status, &run.ItemsNew, &run.ItemsUpdated, &run.ItemsTotal,
		&run.Error, &details)
	if err != nil {
		return run, err
	}
	if len(details) > 0 {
		_ = json.Unmarshal(details, &run.Details)
	}
	return run, nil
}

// ---------------------------------------------------------------------------
// ai_usage_log
// ---------------------------------------------------------------------------

// RecordUsage сохраняет запись учёта расхода токенов. runID пустой → NULL.
func (s *Store) RecordUsage(ctx context.Context, u AIUsage) error {
	var runID any
	if u.RunID != "" {
		runID = u.RunID
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO ai_usage_log (run_id, agent_type, model_id, provider, model_label,
       prompt_tokens, completion_tokens, total_tokens, duration_ms, success, error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		runID, u.AgentType, u.ModelID, u.Provider, u.ModelLabel,
		u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.DurationMs, u.Success, u.Error)
	return err
}

const usageCols = `SELECT id, run_id, agent_type, model_id, provider, model_label,
       prompt_tokens, completion_tokens, total_tokens, duration_ms, success, error, created_at
FROM ai_usage_log`

// ListUsage возвращает записи учёта токенов по фильтру (свежие сверху).
func (s *Store) ListUsage(ctx context.Context, f AIUsageFilter) ([]AIUsage, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	since := f.SinceDays
	if since <= 0 {
		since = 36500
	}
	rows, err := s.pool.Query(ctx, usageCols+`
 WHERE ($1 = '' OR run_id = $1::uuid)
   AND ($2 = '' OR agent_type = $2)
   AND ($3 = '' OR model_id = $3)
   AND created_at >= now() - make_interval(days => $4)
 ORDER BY created_at DESC
 LIMIT $5`, f.RunID, f.AgentType, f.ModelID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AIUsage
	for rows.Next() {
		u, err := scanUsage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func scanUsage(r scanner) (AIUsage, error) {
	var u AIUsage
	var runID *string
	err := r.Scan(&u.ID, &runID, &u.AgentType, &u.ModelID, &u.Provider, &u.ModelLabel,
		&u.PromptTokens, &u.CompletionTokens, &u.TotalTokens, &u.DurationMs, &u.Success,
		&u.Error, &u.CreatedAt)
	if runID != nil {
		u.RunID = *runID
	}
	return u, err
}

// usageAgg группирует расход токенов по произвольной колонке (model_id|agent_type).
func (s *Store) usageAgg(ctx context.Context, col string, sinceDays int) ([]UsageAgg, error) {
	if sinceDays <= 0 {
		sinceDays = 36500
	}
	// col подставляется только из доверенных констант (см. публичные обёртки).
	rows, err := s.pool.Query(ctx, `
SELECT `+col+` AS k,
       count(*)                                  AS calls,
       coalesce(sum(prompt_tokens), 0)           AS pt,
       coalesce(sum(completion_tokens), 0)       AS ct,
       coalesce(sum(total_tokens), 0)            AS tt,
       count(*) FILTER (WHERE NOT success)        AS errs,
       coalesce(avg(duration_ms), 0)::bigint      AS avg_ms
  FROM ai_usage_log
 WHERE created_at >= now() - make_interval(days => $1)
 GROUP BY `+col+`
 ORDER BY tt DESC`, sinceDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageAgg
	for rows.Next() {
		var a UsageAgg
		if err := rows.Scan(&a.Key, &a.Calls, &a.PromptTokens, &a.CompletionTokens,
			&a.TotalTokens, &a.Errors, &a.AvgDurationMs); err != nil {
			return nil, err
		}
		a.Label = a.Key
		out = append(out, a)
	}
	return out, rows.Err()
}

// UsageByModel возвращает агрегат расхода токенов по моделям за период.
func (s *Store) UsageByModel(ctx context.Context, sinceDays int) ([]UsageAgg, error) {
	return s.usageAgg(ctx, "model_id", sinceDays)
}

// UsageByAgent возвращает агрегат расхода токенов по типам агентов за период.
func (s *Store) UsageByAgent(ctx context.Context, sinceDays int) ([]UsageAgg, error) {
	return s.usageAgg(ctx, "agent_type", sinceDays)
}

type scanner interface {
	Scan(dest ...any) error
}
