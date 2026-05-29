-- Migration 003: MCP audit log + preferences + regulations tables
-- Аудит-лог запросов к MCP-серверу, таблицы льгот и НПА

BEGIN;

-- ---------------------------------------------------------------------------
-- MCP Audit Log — лог всех запросов к MCP-серверу с группировкой по tenant
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mcp_audit_log (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    TEXT,                                  -- tenant (может быть NULL для анонимных)
    api_key_pfx  TEXT,                                  -- первые 8 символов API-ключа (не весь ключ!)
    tool_name    TEXT        NOT NULL,                  -- название инструмента (search_documents, ask_consultant, ...)
    input_hash   TEXT,                                  -- SHA-256 от входных параметров (без чувствительных данных)
    input_brief  TEXT,                                  -- краткое описание запроса (первые 200 символов)
    client_id    TEXT,                                  -- ID клиента если был передан в запросе
    remote_addr  TEXT,                                  -- IP-адрес клиента
    duration_ms  INTEGER,                               -- время выполнения в мс
    success      BOOLEAN     NOT NULL DEFAULT TRUE,     -- запрос выполнен успешно
    error_msg    TEXT,                                  -- текст ошибки (если success=false)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mcp_audit_tenant_id   ON mcp_audit_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_mcp_audit_tool_name   ON mcp_audit_log(tool_name);
CREATE INDEX IF NOT EXISTS idx_mcp_audit_created_at  ON mcp_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_audit_client_id   ON mcp_audit_log(client_id) WHERE client_id IS NOT NULL;

COMMENT ON TABLE  mcp_audit_log            IS 'Аудит-лог всех запросов к MCP-серверу';
COMMENT ON COLUMN mcp_audit_log.api_key_pfx IS 'Первые 8 символов API-ключа — для идентификации без хранения полного ключа';
COMMENT ON COLUMN mcp_audit_log.input_brief IS 'Краткое описание входных параметров (не более 200 симв.), без чувствительных данных';

-- ---------------------------------------------------------------------------
-- Preferences (льготы резидентов) — отдельная таблица для структурированного хранения
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS preferences (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    ext_id       TEXT        UNIQUE NOT NULL,           -- внешний ID (из парсера)
    title        TEXT        NOT NULL,
    pref_type    TEXT        NOT NULL                   -- tax_profit, insurance, vat, customs, other
                             CHECK (pref_type IN ('tax_profit','insurance','vat','customs','other')),
    benefit_desc TEXT,                                  -- краткое описание льготы
    legal_ref    TEXT,                                  -- ссылка на НПА (244-ФЗ, статья НК)
    source_url   TEXT,
    content      TEXT,                                  -- полный текст секции
    status       TEXT        NOT NULL DEFAULT 'active'
                             CHECK (status IN ('active','outdated')),
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_preferences_pref_type ON preferences(pref_type);
CREATE INDEX IF NOT EXISTS idx_preferences_status    ON preferences(status);

COMMENT ON TABLE preferences IS 'Льготы резидентов Сколково (налоговые, таможенные, страховые)';

-- ---------------------------------------------------------------------------
-- NPA (нормативно-правовые акты) — структурированное хранение НПА
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS npa_documents (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    ext_id       TEXT        UNIQUE NOT NULL,           -- внешний ID
    title        TEXT        NOT NULL,
    npa_number   TEXT,                                  -- номер НПА (например «244-ФЗ»)
    npa_type     TEXT,                                  -- тип: закон, постановление, приказ, решение
    issued_by    TEXT,                                  -- орган-издатель
    issued_at    DATE,                                  -- дата принятия
    effective_at DATE,                                  -- дата вступления в силу
    source_url   TEXT,
    summary      TEXT,                                  -- краткое содержание
    status       TEXT        NOT NULL DEFAULT 'active'
                             CHECK (status IN ('active','amended','revoked')),
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_npa_status    ON npa_documents(status);
CREATE INDEX IF NOT EXISTS idx_npa_issued_at ON npa_documents(issued_at DESC);
CREATE INDEX IF NOT EXISTS idx_npa_npa_type  ON npa_documents(npa_type);

COMMENT ON TABLE npa_documents IS 'Нормативно-правовые акты, регулирующие деятельность Сколково';

-- ---------------------------------------------------------------------------
-- Eligibility checks — история проверок компаний по ИНН
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS eligibility_checks (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    inn          TEXT        NOT NULL,
    company_name TEXT,
    status       TEXT,                                  -- active, liquidated, bankrupt, ...
    is_msp       BOOLEAN,
    msp_category TEXT,
    score        INTEGER,                               -- 0-100
    eligible     BOOLEAN,
    issues       TEXT[],
    warnings     TEXT[],
    checked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    checked_by   TEXT                                   -- tenant_id кто проверял
);

CREATE INDEX IF NOT EXISTS idx_eligibility_inn        ON eligibility_checks(inn);
CREATE INDEX IF NOT EXISTS idx_eligibility_checked_at ON eligibility_checks(checked_at DESC);

COMMENT ON TABLE eligibility_checks IS 'История проверок компаний на соответствие требованиям резидентства';

COMMIT;
