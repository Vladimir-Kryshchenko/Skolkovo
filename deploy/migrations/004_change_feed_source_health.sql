-- Migration 004: лента изменений (change feed) + мониторинг свежести источников
-- Фиксирует «что изменилось в Сколково» и SLA-состояние каждого источника данных.

BEGIN;

-- ---------------------------------------------------------------------------
-- change_events — лента изменений базы знаний.
-- Что появилось/обновилось/устарело: документы, новости, конкурсы, НПА, льготы.
-- Читается консультантом (MCP get_recent_changes), Telegram-нотификатором и агентами.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS change_events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  TEXT        NOT NULL,                  -- document | news | event | contest | npa | preference | faq
    entity_id    TEXT        NOT NULL,                  -- ID сущности в её таблице
    title        TEXT        NOT NULL,
    category     TEXT,
    kind         TEXT        NOT NULL,                  -- new | updated | outdated | removed
    source_url   TEXT,
    summary      TEXT,                                  -- краткое человекочитаемое описание изменения
    detected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    notified     BOOLEAN     NOT NULL DEFAULT FALSE     -- отправлен ли алерт консультанту
);

CREATE INDEX IF NOT EXISTS idx_change_events_detected   ON change_events (detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_change_events_unnotified ON change_events (detected_at) WHERE notified = FALSE;
CREATE INDEX IF NOT EXISTS idx_change_events_type       ON change_events (entity_type);

COMMENT ON TABLE  change_events           IS 'Лента изменений базы знаний Сколково';
COMMENT ON COLUMN change_events.kind      IS 'Тип изменения: new, updated, outdated, removed';
COMMENT ON COLUMN change_events.notified  IS 'TRUE — алерт об изменении уже отправлен консультанту';

-- ---------------------------------------------------------------------------
-- source_health — SLA-мониторинг свежести источников.
-- Когда источник последний раз успешно обновлялся, сколько вернул, не падает ли.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS source_health (
    name                 TEXT        PRIMARY KEY,       -- documents | news | events | contests | faq | telegram | preferences | regulations | fetch
    last_run_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at      TIMESTAMPTZ,                   -- NULL — ещё ни разу не было успеха
    items_last_run       INTEGER     NOT NULL DEFAULT 0,
    consecutive_failures INTEGER     NOT NULL DEFAULT 0,
    last_error           TEXT        NOT NULL DEFAULT '',
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE  source_health                       IS 'SLA-мониторинг свежести источников данных';
COMMENT ON COLUMN source_health.consecutive_failures  IS 'Сколько прогонов подряд завершились ошибкой';

COMMIT;
