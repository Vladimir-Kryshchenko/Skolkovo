-- Migration 013: хранение API-ключей как hash+prefix (вместо открытого текста).
--
-- Цель: при компрометации БД ключи нельзя использовать (хранится только SHA-256).
-- Префикс (первые 16 символов) хранится отдельно — для идентификации ключа в UI.
--
-- Переход без бэкфилла и без pgcrypto: применяется ДВОЙНОЙ lookup
-- (api_key_hash ИЛИ legacy api_key). Существующие тенанты продолжают
-- аутентифицироваться по старому открытому ключу, пока его не ротируют;
-- новые ключи (и любая ротация) пишутся только как hash+prefix, а legacy
-- api_key очищается. Поэтому здесь снимаем NOT NULL/UNIQUE с tenants.api_key.
--
-- Также добавляем личный API-ключ клиенту-резиденту (clients.api_key_*),
-- чтобы клиент мог подключать своего агента под собственным ключом.

BEGIN;

-- --- tenants: hash+prefix ---
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS api_key_hash   TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS api_key_prefix TEXT;

-- Старый открытый ключ больше не обязателен и не уникален: новые строки пишут NULL.
ALTER TABLE tenants ALTER COLUMN api_key DROP NOT NULL;
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_api_key_key;

-- Уникальность по хэшу (несколько NULL допускаются — legacy-строки до ротации).
CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_api_key_hash ON tenants (api_key_hash) WHERE api_key_hash IS NOT NULL;

COMMENT ON COLUMN tenants.api_key_hash   IS 'SHA-256 (hex) MCP API-ключа тенанта; источник истины для авторизации';
COMMENT ON COLUMN tenants.api_key_prefix IS 'Первые 16 символов ключа — для идентификации в UI (не секрет)';
COMMENT ON COLUMN tenants.api_key        IS 'LEGACY: открытый ключ до перехода на hash; очищается при ротации';

-- --- clients: личный API-ключ резидента ---
ALTER TABLE clients ADD COLUMN IF NOT EXISTS api_key_hash   TEXT;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS api_key_prefix TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_api_key_hash ON clients (api_key_hash) WHERE api_key_hash IS NOT NULL;

COMMENT ON COLUMN clients.api_key_hash   IS 'SHA-256 (hex) личного API-ключа клиента-резидента (для подключения своего агента к MCP)';
COMMENT ON COLUMN clients.api_key_prefix IS 'Первые 16 символов личного ключа клиента — для идентификации в UI';

COMMIT;
