-- Migration 011: дата публикации страницы на сайте Сколково.
-- Отдельная от first_seen/last_changed (времён обхода нашим краулером):
-- published_at — когда материал опубликован НА САМОМ САЙТЕ, как указано на
-- странице. Извлекается детерминированно из разметки (meta article:published_time,
-- JSON-LD datePublished, <time datetime>, видимая надпись .post-page__info .data
-- вида «25 мая 2017 г.»), а при отсутствии разметки — ИИ-аннотатором из текста.
-- См. src/sitepages/published.go.

BEGIN;

-- Идемпотентно — безопасно для уже наполненной таблицы site_pages (35k+ строк).
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

COMMENT ON COLUMN site_pages.published_at IS
  'Дата публикации страницы НА САЙТЕ Сколково (из разметки/видимой надписи или ИИ), отлична от first_seen/last_changed (времён обхода)';

-- Индекс для сортировки/фильтрации по дате публикации (NULL — в конце).
CREATE INDEX IF NOT EXISTS idx_site_pages_published ON site_pages (published_at DESC NULLS LAST);

COMMIT;
