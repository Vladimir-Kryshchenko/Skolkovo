-- Схема реестра документов «База Сколково» (PostgreSQL).
-- Применяется автоматически при STORE_BACKEND=postgres, продублирована здесь для справки.

CREATE TABLE IF NOT EXISTS documents (
    id            TEXT PRIMARY KEY,
    title         TEXT NOT NULL,
    source_url    TEXT NOT NULL,
    local_path    TEXT,
    published_at  DATE,
    fetched_at    TIMESTAMPTZ NOT NULL,
    status        TEXT NOT NULL,          -- на_проверке | действует | устарел | архив | отклонён
    category      TEXT,
    version_label TEXT,
    valid_from    DATE,
    valid_to      DATE,
    supersedes    TEXT,                   -- id заменённого документа
    file_hash     TEXT NOT NULL,          -- SHA-256
    indexed       BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_documents_status   ON documents (status);
CREATE INDEX IF NOT EXISTS idx_documents_category ON documents (category);
