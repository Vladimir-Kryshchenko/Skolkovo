-- Migration 016: категория и подкатегория страниц публичного сайта.
-- Дополняет ИИ-обогащение страниц (у которых уже есть теги/описание/цели/тезисы/
-- выводы из миграции 010) тематической КАТЕГОРИЕЙ и уточняющей ПОДКАТЕГОРИЕЙ —
-- по аналогии с документами (миграция 015). Проставляются агентом «Аннотатор
-- страниц» автоматически при аннотировании (новые и изменённые страницы), идут в
-- RAG-payload и в админку (/sitepages: фильтр и отображение).
--
-- Категория/подкатегория страниц — свободные тематические (сайт sk.ru широкий:
-- новости, резидентам, услуги, мероприятия и т.п.), не из закрытого списка
-- документов.

BEGIN;

ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS category    TEXT NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS subcategory TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_site_pages_category    ON site_pages (category);
CREATE INDEX IF NOT EXISTS idx_site_pages_subcategory ON site_pages (subcategory);

COMMENT ON COLUMN site_pages.category    IS 'Тематическая категория страницы (ИИ), свободная';
COMMENT ON COLUMN site_pages.subcategory IS 'Уточняющая подкатегория страницы (ИИ)';

COMMIT;
