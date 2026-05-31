-- Migration 007: relevance & alerts (трек «Актуальность изменений»)
-- Created: 2026-05-31
-- Description: Обогащает ленту изменений важностью и описанием «что изменилось»,
-- добавляет историю версий документов (для семантического диффа) и inbox
-- уведомлений клиента. Питает дашборд консультанта, портал клиента и MCP.

BEGIN;

-- ─── Шаг 1: обогащение ленты изменений ─────────────────────────────────────
-- severity         — важность изменения: info | warning | critical
-- analysis_summary — человекочитаемое «что изменилось по сути» (LLM или эвристика)
-- affected_stages  — стадии резидентства, которых касается изменение
-- diff_added/removed — статистика структурного диффа (число строк)
-- analyzed         — обработано ли событие анализатором (для очереди обработки)
ALTER TABLE change_events ADD COLUMN IF NOT EXISTS severity         TEXT    NOT NULL DEFAULT '';
ALTER TABLE change_events ADD COLUMN IF NOT EXISTS analysis_summary TEXT    NOT NULL DEFAULT '';
ALTER TABLE change_events ADD COLUMN IF NOT EXISTS affected_stages  TEXT[]  NOT NULL DEFAULT '{}';
ALTER TABLE change_events ADD COLUMN IF NOT EXISTS diff_added       INT     NOT NULL DEFAULT 0;
ALTER TABLE change_events ADD COLUMN IF NOT EXISTS diff_removed     INT     NOT NULL DEFAULT 0;
ALTER TABLE change_events ADD COLUMN IF NOT EXISTS analyzed         BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_change_events_unanalyzed
    ON change_events (detected_at) WHERE analyzed = FALSE;
CREATE INDEX IF NOT EXISTS idx_change_events_severity
    ON change_events (severity, detected_at DESC);

-- ─── Шаг 2: история версий документов ──────────────────────────────────────
-- Снимок извлечённого текста каждой редакции документа. Нужен, чтобы при
-- обновлении документа можно было сравнить старую и новую версию (дифф).
CREATE TABLE IF NOT EXISTS doc_versions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id    TEXT        NOT NULL,
    version_no     INT         NOT NULL,
    file_hash      TEXT        NOT NULL DEFAULT '',
    extracted_text TEXT        NOT NULL DEFAULT '',
    archived_path  TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, version_no)
);
CREATE INDEX IF NOT EXISTS idx_doc_versions_doc
    ON doc_versions (document_id, version_no DESC);

-- ─── Шаг 3: inbox уведомлений клиента ──────────────────────────────────────
-- Персональные уведомления резидента о касающихся его изменениях. Читаются
-- порталом (:8092). Поля email_sent_at/tg_sent_at дедуплицируют доставку.
CREATE TABLE IF NOT EXISTS client_notifications (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id       TEXT        NOT NULL,
    change_event_id UUID,
    severity        TEXT        NOT NULL DEFAULT 'info',
    title           TEXT        NOT NULL,
    body            TEXT        NOT NULL DEFAULT '',
    url             TEXT        NOT NULL DEFAULT '',
    read            BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    email_sent_at   TIMESTAMPTZ,
    tg_sent_at      TIMESTAMPTZ,
    UNIQUE (client_id, change_event_id)
);
CREATE INDEX IF NOT EXISTS idx_client_notifications_client
    ON client_notifications (client_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_client_notifications_unread
    ON client_notifications (client_id) WHERE read = FALSE;

COMMIT;
