-- Migration 005: конфигурация ИИ-моделей и агентов
-- Хранит настройки LLM-провайдеров и конфигурации агентов с промптами.

BEGIN;

-- ---------------------------------------------------------------------------
-- ai_models — зарегистрированные LLM-модели
-- Поддерживает любых провайдеров с OpenAI-совместимым API:
--   alibabacloud (Qwen), openai, anthropic, custom (self-hosted).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ai_models (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    provider    VARCHAR(100) NOT NULL DEFAULT 'custom',
    model_id    VARCHAR(255) NOT NULL,
    base_url    TEXT NOT NULL DEFAULT 'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
    api_key     TEXT NOT NULL DEFAULT '',
    max_tokens  INT NOT NULL DEFAULT 4096,
    temperature FLOAT NOT NULL DEFAULT 0.7,
    enabled     BOOL NOT NULL DEFAULT true,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- ai_agents — конфигурация ИИ-агентов с системными промптами
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ai_agents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    agent_type    VARCHAR(100) NOT NULL,
    model_id      UUID REFERENCES ai_models(id) ON DELETE SET NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    temperature   FLOAT NOT NULL DEFAULT 0.7,
    max_tokens    INT NOT NULL DEFAULT 4096,
    enabled       BOOL NOT NULL DEFAULT true,
    description   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_models_provider ON ai_models(provider);
CREATE INDEX IF NOT EXISTS idx_ai_models_enabled  ON ai_models(enabled);
CREATE INDEX IF NOT EXISTS idx_ai_agents_type     ON ai_agents(agent_type);
CREATE INDEX IF NOT EXISTS idx_ai_agents_enabled  ON ai_agents(enabled);

COMMIT;
