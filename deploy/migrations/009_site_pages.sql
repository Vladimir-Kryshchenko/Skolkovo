-- Migration 009: страницы публичного сайта Сколково (sk.ru/dochub.sk.ru).
-- Отдельный от файлов-документов слой: одна запись на страницу сайта
-- (URL, заголовок, краткое описание, раздел, хэш контента). Питает отдельную
-- Qdrant-коллекцию для поиска по страницам (MCP search_site_pages) и ленту
-- изменений (entity_type='sitepage').

BEGIN;

CREATE TABLE IF NOT EXISTS site_pages (
    id            TEXT        PRIMARY KEY,            -- детерминированный sha1 от нормализованного URL
    url           TEXT        NOT NULL UNIQUE,
    title         TEXT        NOT NULL DEFAULT '',
    summary       TEXT        NOT NULL DEFAULT '',    -- meta description или начало текста
    text          TEXT        NOT NULL DEFAULT '',    -- полный видимый текст страницы (для просмотрщика)
    section       TEXT        NOT NULL DEFAULT '',    -- раздел/хлебные крошки из пути URL
    content_hash  TEXT        NOT NULL DEFAULT '',    -- sha256 видимого текста (детектор изменений)
    status        TEXT        NOT NULL DEFAULT 'active', -- active | gone (404/410 при перекрауле)
    first_seen    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_changed  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Безопасно для уже существующей таблицы (если 009 запускалась до добавления text).
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS text TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_site_pages_section      ON site_pages (section);
CREATE INDEX IF NOT EXISTS idx_site_pages_last_changed ON site_pages (last_changed DESC);
CREATE INDEX IF NOT EXISTS idx_site_pages_status       ON site_pages (status);

COMMENT ON TABLE  site_pages              IS 'Страницы публичного сайта Сколково (отдельно от файлов-документов)';
COMMENT ON COLUMN site_pages.content_hash IS 'sha256 видимого текста страницы — смена хэша => last_changed обновляется';
COMMENT ON COLUMN site_pages.status       IS 'active — доступна; gone — отдаёт 404/410 при перекрауле';

COMMIT;
