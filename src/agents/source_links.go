// source_links.go — выбор рабочей ссылки на источник для ответов консультанта.
//
// Зачем: WAF dochub блокирует прямые внешние переходы на тело документа в разделе
// `/m/docs/` (HTTP 403 — пользователь видит «Запрещено»). Поэтому ссылку-источник
// нельзя отдавать «как есть». Гибридная логика (bestSourceURL):
//  1. если у документа есть скачанная копия и задан публичный базовый URL —
//     ведём на нашу копию (<base>/files/{id}), которая открывается всегда;
//  2. иначе заблокированную WAF ссылку на /m/docs/ заменяем на страницу-листинг
//     категории dochub (она отдаётся обычным GET, 200);
//  3. рабочие ссылки (раздел /news/m/wiki/, мероприятия, конкурсы, FAQ и пр.)
//     оставляем без изменений.
package agents

import (
	"context"
	"net/url"
	"strings"

	rag "baza-skolkovo/src/rag_service"
)

// categorySlugByName — обратная карта «название категории → слаг страницы-листинга
// dochub» (/foundation/documents/p/{slug}.aspx). Источник истины —
// scraper.CategoryNames (slug→name); продублирована здесь локально, чтобы пакет
// agents не зависел от scraper. Набор категорий стабилен.
var categorySlugByName = map[string]string{
	"Законодательные акты":             "legislative_acts",
	"Правила проектирования":           "design_rules",
	"Иные нормативные документы":       "other",
	"Развитие территории":              "development",
	"Закупки и тендеры":                "tenders",
	"Утратившие силу":                  "unactual_documents",
	"Антикоррупция":                    "anti_corruption",
	"Кибербезопасность и перс. данные": "cybersec_and_persdata",
}

// bestSourceURL выбирает рабочую ссылку на источник (см. комментарий пакета-файла).
func (a *ConsultantAgent) bestSourceURL(ctx context.Context, r rag.Result) string {
	raw := strings.TrimSpace(r.SourceURL)

	// Сущности кроме документов-файлов (мероприятия, конкурсы, FAQ, новости, НПА,
	// льготы) имеют обычные открытые URL — оставляем как есть.
	if r.EntityType != "" && r.EntityType != "document" {
		return raw
	}

	// 1) Наша скачанная копия, если файл есть и задан публичный базовый URL.
	if a.fileBaseURL != "" && r.DocumentID != "" && a.ragService != nil && a.ragService.Store != nil {
		if doc, err := a.ragService.Store.Get(ctx, r.DocumentID); err == nil && strings.TrimSpace(doc.LocalPath) != "" {
			return strings.TrimRight(a.fileBaseURL, "/") + "/files/" + r.DocumentID
		}
	}

	// 2) Прямую ссылку на тело документа в /m/docs/ блокирует WAF — заменяем на
	//    открытую страницу-листинг категории.
	if isWAFBlockedDocURL(raw) {
		if cat := categoryPageURL(raw, r.Category); cat != "" {
			return cat
		}
	}

	// 3) Иначе исходная ссылка (рабочие /news/m/wiki/ и пр.).
	return raw
}

// isWAFBlockedDocURL сообщает, ведёт ли ссылка на тело документа в разделе
// /m/docs/, который WAF dochub блокирует для прямых внешних переходов (HTTP 403).
// Рабочий раздел /news/m/wiki/ (вложения новостей) под это не подпадает.
func isWAFBlockedDocURL(raw string) bool {
	return strings.Contains(strings.ToLower(raw), "/m/docs/")
}

// categoryPageURL строит URL открытой страницы-листинга категории dochub из
// исходной (заблокированной) ссылки и названия категории. Если категория
// неизвестна — ведёт на корневой раздел документов (он тоже открыт). Возвращает
// "", если из исходной ссылки не удалось извлечь схему и хост.
func categoryPageURL(raw, categoryName string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	base := u.Scheme + "://" + u.Host
	if slug := categorySlugByName[strings.TrimSpace(categoryName)]; slug != "" {
		return base + "/foundation/documents/p/" + slug + ".aspx"
	}
	return base + "/foundation/documents/"
}
