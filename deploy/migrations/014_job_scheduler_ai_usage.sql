-- Migration 014: планировщик заданий (per-source расписание) + журнал запусков
-- + учёт расхода токенов ИИ-агентов.
--
-- Питает раздел админки «Расписание и журнал» (:8090 /jobs) и MCP-инструменты
-- get_scheduler_runs / get_ai_usage. Заменяет жёсткий SCRAPE_INTERVAL из env
-- на настраиваемый из интерфейса интервал по каждому источнику.

BEGIN;

-- ---------------------------------------------------------------------------
-- scheduler_jobs — конфигурация расписания по каждому источнику/заданию.
-- Интервал и вкл/выкл редактируются админом в /jobs; next_run_at ведёт раннер.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS scheduler_jobs (
    name             TEXT        PRIMARY KEY,            -- documents | events | contests | faq | residents | telegram | preferences | regulations | sitepages | fetch_bodies
    title            TEXT        NOT NULL DEFAULT '',    -- человекочитаемое название
    interval_seconds BIGINT      NOT NULL DEFAULT 21600, -- период между запусками (по умолчанию 6 ч)
    enabled          BOOLEAN     NOT NULL DEFAULT TRUE,
    last_run_at      TIMESTAMPTZ,
    next_run_at      TIMESTAMPTZ,                        -- NULL — запустить при ближайшем тике (startup)
    last_status      TEXT        NOT NULL DEFAULT '',    -- success | error | partial | skipped
    last_error       TEXT        NOT NULL DEFAULT '',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE  scheduler_jobs                  IS 'Расписание периодических заданий сбора (настраивается в админке)';
COMMENT ON COLUMN scheduler_jobs.interval_seconds IS 'Период между запусками задания в секундах';
COMMENT ON COLUMN scheduler_jobs.next_run_at      IS 'Когда задание должно запуститься в следующий раз (NULL — при ближайшем тике)';

-- ---------------------------------------------------------------------------
-- scheduler_runs — журнал прогонов: что отработало, сколько нашло, ошибки.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS scheduler_runs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    job_name      TEXT        NOT NULL,
    trigger       TEXT        NOT NULL DEFAULT 'schedule'
                  CHECK (trigger IN ('schedule', 'manual', 'startup')),
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ,                           -- NULL — прогон ещё выполняется
    duration_ms   BIGINT      NOT NULL DEFAULT 0,
    status        TEXT        NOT NULL DEFAULT 'running'  -- running | success | error | partial | skipped
                  CHECK (status IN ('running', 'success', 'error', 'partial', 'skipped')),
    items_new     INTEGER     NOT NULL DEFAULT 0,
    items_updated INTEGER     NOT NULL DEFAULT 0,
    items_total   INTEGER     NOT NULL DEFAULT 0,
    error         TEXT        NOT NULL DEFAULT '',
    details       JSONB                                  -- произвольная разбивка по подэтапам
);

CREATE INDEX IF NOT EXISTS idx_scheduler_runs_job     ON scheduler_runs (job_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_scheduler_runs_started ON scheduler_runs (started_at DESC);

COMMENT ON TABLE  scheduler_runs         IS 'Журнал запусков планировщика: результат и ошибки каждого прогона';
COMMENT ON COLUMN scheduler_runs.trigger IS 'Чем инициирован прогон: schedule (по расписанию), manual (кнопка), startup (первый при старте)';

-- ---------------------------------------------------------------------------
-- ai_usage_log — учёт расхода токенов ИИ-агентов по моделям.
-- Заполняется хуком aimodels.SetUsageRecorder на каждый вызов ChatWithAgent.
-- run_id связывает вызов с прогоном планировщика (если вызов был внутри задания).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ai_usage_log (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id            UUID,                              -- NULL — вызов вне планировщика (чат-виджет, бот, ручной)
    agent_type        TEXT        NOT NULL DEFAULT '',   -- consultant | validator | monitor | coordinator | page_annotator
    model_id          TEXT        NOT NULL DEFAULT '',   -- идентификатор модели в API (e.g. qwen-max)
    provider          TEXT        NOT NULL DEFAULT '',   -- alibabacloud | openai | anthropic | custom
    model_label       TEXT        NOT NULL DEFAULT '',   -- человекочитаемое имя модели из конфигурации
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

COMMENT ON TABLE  ai_usage_log        IS 'Учёт расхода токенов ИИ-агентов: сколько и по какой модели потрачено';
COMMENT ON COLUMN ai_usage_log.run_id IS 'Прогон планировщика, в рамках которого был вызов (NULL — вне расписания)';

COMMIT;
