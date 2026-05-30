-- Migration 006: source_url NOT NULL for extended sources
-- Created: 2026-05-30
-- Description: Гарантирует наличие ссылки на источник для всех сущностей,
-- скачиваемых с сайта Сколково. До этого source_url был nullable, что допускало
-- записи без ссылки на источник.

BEGIN;

-- Шаг 1: заполняем пустые source_url заглушкой для существующих записей,
-- чтобы ALTER TABLE не упал на NOT NULL проверке.
UPDATE events       SET source_url = '' WHERE source_url IS NULL OR source_url = '';
UPDATE contests     SET source_url = '' WHERE source_url IS NULL OR source_url = '';
UPDATE faq_items    SET source_url = '' WHERE source_url IS NULL OR source_url = '';
UPDATE telegram_posts SET source_url = '' WHERE source_url IS NULL OR source_url = '';
UPDATE residents    SET source_url = '' WHERE source_url IS NULL OR source_url = '';
UPDATE preferences  SET source_url = '' WHERE source_url IS NULL OR source_url = '';
UPDATE npa_documents SET source_url = '' WHERE source_url IS NULL OR source_url = '';

-- Шаг 2: делаем source_url NOT NULL.
ALTER TABLE events          ALTER COLUMN source_url SET NOT NULL;
ALTER TABLE contests        ALTER COLUMN source_url SET NOT NULL;
ALTER TABLE faq_items       ALTER COLUMN source_url SET NOT NULL;
ALTER TABLE telegram_posts  ALTER COLUMN source_url SET NOT NULL;
ALTER TABLE residents       ALTER COLUMN source_url SET NOT NULL;
ALTER TABLE preferences     ALTER COLUMN source_url SET NOT NULL;
ALTER TABLE npa_documents   ALTER COLUMN source_url SET NOT NULL;

-- Шаг 3: обновляем комментарии.
COMMENT ON COLUMN events.source_url          IS 'URL of the original source — обязателен';
COMMENT ON COLUMN contests.source_url        IS 'URL of the original source — обязателен';
COMMENT ON COLUMN faq_items.source_url       IS 'URL of the original source — обязателен';
COMMENT ON COLUMN telegram_posts.source_url  IS 'URL of the original Telegram post — обязателен';
COMMENT ON COLUMN residents.source_url       IS 'URL of the original source — обязателен';
COMMENT ON COLUMN preferences.source_url     IS 'URL of the original source (льгота) — обязателен';
COMMENT ON COLUMN npa_documents.source_url   IS 'URL of the original source (НПА) — обязателен';

COMMIT;
