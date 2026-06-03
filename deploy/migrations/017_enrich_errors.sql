-- Migration 017: фиксация ошибок ИИ-разметки на уровне записи (страницы и документы).
-- Зачем: часть страниц/документов стабильно не размечается (например, контент-
-- модерация провайдера `data_inspection_failed` — отклоняется всеми моделями).
-- Раньше такие записи бесконечно крутились в очереди на разметку (повторные
-- вызовы всех моделей впустую). Теперь по каждой записи сохраняются:
--   enrich_error        — текст последней ошибки разметки (пусто, если успех);
--   enrich_attempts     — число неуспешных попыток подряд;
--   enrich_attempted_at — когда была последняя попытка.
-- Выборка на разметку исключает записи, превысившие лимит попыток (см. код:
-- maxEnrichAttempts) — они перестают зацикливаться и видны в админке для разбора
-- (фильтр «с ошибками разметки» + ручная переразметка после исправления).

BEGIN;

ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enrich_error        TEXT        NOT NULL DEFAULT '';
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enrich_attempts     INT         NOT NULL DEFAULT 0;
ALTER TABLE site_pages ADD COLUMN IF NOT EXISTS enrich_attempted_at TIMESTAMPTZ;

ALTER TABLE documents  ADD COLUMN IF NOT EXISTS enrich_error        TEXT        NOT NULL DEFAULT '';
ALTER TABLE documents  ADD COLUMN IF NOT EXISTS enrich_attempts     INT         NOT NULL DEFAULT 0;
ALTER TABLE documents  ADD COLUMN IF NOT EXISTS enrich_attempted_at TIMESTAMPTZ;

-- Частичные индексы — быстро находить записи с ошибкой разметки (для админки).
CREATE INDEX IF NOT EXISTS idx_site_pages_enrich_error ON site_pages (enrich_attempted_at DESC) WHERE enrich_error <> '';
CREATE INDEX IF NOT EXISTS idx_documents_enrich_error  ON documents  (enrich_attempted_at DESC) WHERE enrich_error <> '';

COMMENT ON COLUMN site_pages.enrich_error IS 'Текст последней ошибки ИИ-разметки (пусто — успех)';
COMMENT ON COLUMN documents.enrich_error  IS 'Текст последней ошибки ИИ-разметки (пусто — успех)';

COMMIT;
