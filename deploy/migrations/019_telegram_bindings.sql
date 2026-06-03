-- 019_telegram_bindings.sql
-- Персистентная привязка Telegram chat ID к клиенту.
-- Переживает рестарты бота; ограничена тенантом.
CREATE TABLE IF NOT EXISTS telegram_bindings (
    chat_id    BIGINT      NOT NULL,
    client_id  TEXT        NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    tenant_id  TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_telegram_bindings_tenant ON telegram_bindings(tenant_id);
