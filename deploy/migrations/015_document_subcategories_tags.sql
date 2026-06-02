-- Migration 015: подкатегории и теги документов с авто-проставлением ИИ.
-- Для каждого действующего документа агент «Аннотатор документов» (doc_annotator)
-- автоматически проставляет КАТЕГОРИЮ (из канонических 8), уточняющую ПОДКАТЕГОРИЮ
-- и набор ТЕГОВ. Поля идут в RAG-эмбеддинг и payload (поиск с фильтром по тегам)
-- и в админку (/ фильтры по подкатегории и тегам, мультиселект).
--
-- Гибридный растущий словарь тегов document_tags: ИИ переиспользует уже
-- существующие теги и добавляет новые — фильтр остаётся согласованным, но
-- пополняется автоматически (как site_page_tags для страниц сайта).
--
-- enrich_hash = file_hash на момент разметки: переразмечаем документ только при
-- изменении файла. Поля НЕ трогаются обычным Upsert при переобходе (сохраняются
-- через ON CONFLICT), пишутся отдельным UpdateClassification.

BEGIN;

ALTER TABLE documents ADD COLUMN IF NOT EXISTS subcategory TEXT   NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS tags        TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS enriched_at TIMESTAMPTZ;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS enrich_hash TEXT   NOT NULL DEFAULT '';

-- Подробные индексы для фильтрации и поиска.
CREATE INDEX IF NOT EXISTS idx_documents_subcategory    ON documents (subcategory);
CREATE INDEX IF NOT EXISTS idx_documents_tags           ON documents USING GIN (tags);
CREATE INDEX IF NOT EXISTS idx_documents_status_category ON documents (status, category);

COMMENT ON COLUMN documents.subcategory IS 'Подкатегория документа (ИИ), уточняет category';
COMMENT ON COLUMN documents.tags        IS 'Авто-теги документа (ИИ), нормализованы против словаря document_tags';
COMMENT ON COLUMN documents.enrich_hash IS 'file_hash на момент ИИ-разметки — переразмечаем только при изменении файла';

-- Управляемый растущий словарь тегов (источник истины для фильтра и подсказок ИИ).
CREATE TABLE IF NOT EXISTS document_tags (
    tag         TEXT        PRIMARY KEY,
    usage_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE document_tags IS 'Словарь авто-тегов документов (гибрид: ИИ переиспользует существующие, новые пополняют словарь)';

COMMIT;
