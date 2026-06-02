-- 008_client_subscriptions.sql
-- Таблица подписок клиентов на категории уведомлений об изменениях.

CREATE TABLE IF NOT EXISTS client_subscriptions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id  TEXT        NOT NULL,
    category   TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT client_subscriptions_unique UNIQUE (client_id, category)
);

CREATE INDEX IF NOT EXISTS idx_client_subscriptions_client ON client_subscriptions(client_id);
