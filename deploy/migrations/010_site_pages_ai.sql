-- Migration 010: ИИ-обогащение страниц публичного сайта.
-- Для каждой страницы site_pages автоматически (через ИИ-агента «Аннотатор
-- страниц») генерируются теги, краткое описание, цели, важные тезисы и выводы.
-- Эти поля идут в RAG-коллекцию страниц (search_site_pages) и в админку
-- (/sitepages: фильтр по тегам, секции аннотаций, связанные страницы).
--
-- Гибридный словарь тегов: ИИ переиспользует уже существующие теги из
-- site_page_tags и добавляет новые — фильтр множественного выбора остаётся
-- согласованным, но пополняется автоматически.

BEGIN;

-- ИИ-поля страницы (идемпотентно — безопасно для уже наполненной таблицы).
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS tags         TEXT[]      NOT NULL DEFAULT '{}';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS ai_summary   TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS goals        TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS theses       TEXT[]      NOT NULL DEFAULT '{}';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS conclusions  TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enriched_at  TIMESTAMPTZ;
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enrich_hash  TEXT        NOT NULL DEFAULT '';

-- GIN-индекс по массиву тегов — для фильтра «страница содержит тег(и)».
CREATE INDEX IF NOT EXISTS idx_site_pages_tags ON site_pages USING GIN (tags);

COMMENT ON COLUMN site_pages.tags        IS 'Авто-теги страницы (ИИ), нормализованы против словаря site_page_tags';
COMMENT ON COLUMN site_pages.ai_summary  IS 'Краткое описание страницы (ИИ), отличается от summary (meta-description)';
COMMENT ON COLUMN site_pages.goals       IS 'Цели страницы (ИИ)';
COMMENT ON COLUMN site_pages.theses      IS 'Важные тезисы страницы (ИИ), список';
COMMENT ON COLUMN site_pages.conclusions IS 'Выводы по странице (ИИ)';
COMMENT ON COLUMN site_pages.enrich_hash IS 'content_hash на момент аннотирования — переаннотируем только при изменении контента';

-- Управляемый растущий словарь тегов (источник истины для фильтра и подсказок ИИ).
CREATE TABLE IF NOT EXISTS site_page_tags (
    tag         TEXT        PRIMARY KEY,
    usage_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE site_page_tags IS 'Словарь авто-тегов страниц сайта (гибрид: ИИ переиспользует существующие, новые пополняют словарь)';

COMMIT;
