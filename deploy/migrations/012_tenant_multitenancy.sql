-- Migration 012: мульти-тенантность подключений.
-- Тенант (организация-заказчик) получает собственный MCP API-ключ (поле api_key
-- уже есть в tenants) и теперь — собственный Telegram-бот по своему токену.
-- Эти поля питают per-tenant авторизацию MCP-сервера и мультибот-менеджер (tgbot).
--
-- Идемпотентно: существующие тенанты получают NULL → полная обратная совместимость
-- (глобальный MCP_API_KEY и глобальный TELEGRAM_BOT_TOKEN продолжают работать).

BEGIN;

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS telegram_bot_token    TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS telegram_bot_username TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS key_rotated_at        TIMESTAMPTZ;

-- Частичный индекс: мультибот-менеджер при старте быстро выбирает активных
-- тенантов с заданным токеном бота.
CREATE INDEX IF NOT EXISTS idx_tenants_tg_bot ON tenants (active) WHERE telegram_bot_token IS NOT NULL;

COMMENT ON COLUMN tenants.telegram_bot_token    IS 'Токен Telegram-бота тенанта (@BotFather); пусто — бот не поднимается';
COMMENT ON COLUMN tenants.telegram_bot_username IS 'Кэш @username бота (заполняется менеджером после запуска) — для статуса в админке';
COMMENT ON COLUMN tenants.key_rotated_at        IS 'Когда последний раз ротировали MCP API-ключ';

COMMIT;
