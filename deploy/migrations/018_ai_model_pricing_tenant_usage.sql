-- Migration 018: стоимость ИИ-моделей + привязка расхода токенов к тенантам.
--
-- 1. Добавляет cost_per_million_input / cost_per_million_output в ai_models (USD/1M токенов).
--    Поля редактируются в админке (/ai/models) и используются для расчёта стоимости
--    в разделе «Тенанты и токены» (/tenants) и в отчёте расхода токенов (/jobs/usage).
--
-- 2. Добавляет tenant_id в ai_usage_log: привязывает каждый ИИ-вызов к тенанту,
--    чей API-ключ использовался при запросе (MCP, бот, виджет).
--    Старые записи остаются с NULL — для них стоимость суммируется как «без тенанта».

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Стоимость моделей
-- ---------------------------------------------------------------------------
ALTER TABLE ai_models
    ADD COLUMN IF NOT EXISTS cost_per_million_input  NUMERIC(12,4) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cost_per_million_output NUMERIC(12,4) NOT NULL DEFAULT 0;

COMMENT ON COLUMN ai_models.cost_per_million_input  IS 'Стоимость 1М входных токенов, USD (из официальной документации провайдера)';
COMMENT ON COLUMN ai_models.cost_per_million_output IS 'Стоимость 1М выходных токенов, USD (из официальной документации провайдера)';

-- Заполняем цены для известных моделей по model_id.
-- Источник: официальная документация провайдеров (июнь 2025).

-- Alibaba Cloud / Qwen (https://www.alibabacloud.com/help/en/model-studio/developer-reference/billing-for-model-api-calls)
UPDATE ai_models SET cost_per_million_input = 1.60,  cost_per_million_output = 6.40  WHERE provider = 'alibabacloud' AND model_id IN ('qwen-max', 'qwen-max-latest', 'qwen-max-2024-09-19', 'qwen-max-2025-01-25');
UPDATE ai_models SET cost_per_million_input = 0.40,  cost_per_million_output = 1.20  WHERE provider = 'alibabacloud' AND model_id IN ('qwen-plus', 'qwen-plus-latest', 'qwen-plus-2025-01-25');
UPDATE ai_models SET cost_per_million_input = 0.15,  cost_per_million_output = 0.60  WHERE provider = 'alibabacloud' AND model_id IN ('qwen-turbo', 'qwen-turbo-latest', 'qwen-turbo-2024-11-01');
UPDATE ai_models SET cost_per_million_input = 0.05,  cost_per_million_output = 0.20  WHERE provider = 'alibabacloud' AND model_id IN ('qwen-long');
UPDATE ai_models SET cost_per_million_input = 0.40,  cost_per_million_output = 1.20  WHERE provider = 'alibabacloud' AND model_id IN ('qwen2.5-72b-instruct');
UPDATE ai_models SET cost_per_million_input = 0.20,  cost_per_million_output = 0.60  WHERE provider = 'alibabacloud' AND model_id IN ('qwen2.5-32b-instruct');
UPDATE ai_models SET cost_per_million_input = 0.10,  cost_per_million_output = 0.30  WHERE provider = 'alibabacloud' AND model_id IN ('qwen2.5-14b-instruct');
UPDATE ai_models SET cost_per_million_input = 0.04,  cost_per_million_output = 0.12  WHERE provider = 'alibabacloud' AND model_id IN ('qwen2.5-7b-instruct');
UPDATE ai_models SET cost_per_million_input = 1.50,  cost_per_million_output = 4.50  WHERE provider = 'alibabacloud' AND model_id IN ('qwen-vl-plus', 'qwen-vl-max');
UPDATE ai_models SET cost_per_million_input = 0.50,  cost_per_million_output = 2.00  WHERE provider = 'alibabacloud' AND model_id LIKE 'qwen3%';

-- OpenAI (https://openai.com/api/pricing/)
UPDATE ai_models SET cost_per_million_input = 2.50,  cost_per_million_output = 10.00 WHERE provider = 'openai' AND model_id LIKE 'gpt-4o%' AND model_id NOT LIKE '%mini%';
UPDATE ai_models SET cost_per_million_input = 0.15,  cost_per_million_output = 0.60  WHERE provider = 'openai' AND model_id LIKE 'gpt-4o-mini%';
UPDATE ai_models SET cost_per_million_input = 10.00, cost_per_million_output = 30.00 WHERE provider = 'openai' AND model_id LIKE 'gpt-4-turbo%';
UPDATE ai_models SET cost_per_million_input = 30.00, cost_per_million_output = 60.00 WHERE provider = 'openai' AND model_id = 'gpt-4';
UPDATE ai_models SET cost_per_million_input = 0.50,  cost_per_million_output = 1.50  WHERE provider = 'openai' AND model_id LIKE 'gpt-3.5-turbo%';
UPDATE ai_models SET cost_per_million_input = 15.00, cost_per_million_output = 60.00 WHERE provider = 'openai' AND model_id LIKE 'o1%' AND model_id NOT LIKE '%mini%' AND model_id NOT LIKE '%preview%';
UPDATE ai_models SET cost_per_million_input = 3.00,  cost_per_million_output = 12.00 WHERE provider = 'openai' AND model_id LIKE 'o1-mini%';
UPDATE ai_models SET cost_per_million_input = 1.10,  cost_per_million_output = 4.40  WHERE provider = 'openai' AND model_id LIKE 'o3-mini%';
UPDATE ai_models SET cost_per_million_input = 10.00, cost_per_million_output = 40.00 WHERE provider = 'openai' AND model_id LIKE 'o3%' AND model_id NOT LIKE '%mini%';

-- Anthropic (https://www.anthropic.com/pricing#api)
UPDATE ai_models SET cost_per_million_input = 3.00,  cost_per_million_output = 15.00 WHERE provider = 'anthropic' AND model_id LIKE 'claude-3-5-sonnet%';
UPDATE ai_models SET cost_per_million_input = 0.80,  cost_per_million_output = 4.00  WHERE provider = 'anthropic' AND model_id LIKE 'claude-3-5-haiku%';
UPDATE ai_models SET cost_per_million_input = 15.00, cost_per_million_output = 75.00 WHERE provider = 'anthropic' AND model_id LIKE 'claude-3-opus%';
UPDATE ai_models SET cost_per_million_input = 3.00,  cost_per_million_output = 15.00 WHERE provider = 'anthropic' AND model_id LIKE 'claude-3-sonnet%';
UPDATE ai_models SET cost_per_million_input = 0.25,  cost_per_million_output = 1.25  WHERE provider = 'anthropic' AND model_id LIKE 'claude-3-haiku%';
UPDATE ai_models SET cost_per_million_input = 3.00,  cost_per_million_output = 15.00 WHERE provider = 'anthropic' AND model_id LIKE 'claude-sonnet-4%';
UPDATE ai_models SET cost_per_million_input = 15.00, cost_per_million_output = 75.00 WHERE provider = 'anthropic' AND model_id LIKE 'claude-opus-4%';
UPDATE ai_models SET cost_per_million_input = 0.80,  cost_per_million_output = 4.00  WHERE provider = 'anthropic' AND model_id LIKE 'claude-haiku-4%';

-- ---------------------------------------------------------------------------
-- 2. Привязка расхода токенов к тенанту
-- ---------------------------------------------------------------------------
ALTER TABLE ai_usage_log
    ADD COLUMN IF NOT EXISTS tenant_id UUID;

CREATE INDEX IF NOT EXISTS idx_ai_usage_tenant
    ON ai_usage_log (tenant_id, created_at DESC)
    WHERE tenant_id IS NOT NULL;

COMMENT ON COLUMN ai_usage_log.tenant_id IS 'Тенант, чей API-ключ инициировал вызов (NULL = системный вызов или вызов до введения мультитенантности)';

COMMIT;
