// Package sitepages ведёт отдельный от файлов-документов слой знаний —
// страницы публичного сайта Сколково (sk.ru, dochub.sk.ru). Одна запись на
// страницу: URL, заголовок, краткое описание, раздел и хэш контента.
//
// Зачем отдельно от документов: «файлы с сайта» (PDF/DOCX) и «информация по
// страницам сайта» — разные сущности с разным жизненным циклом и разными
// запросами. Страницы индексируются в собственную Qdrant-коллекцию и доступны
// через MCP-инструмент search_site_pages, не смешиваясь с search_documents.
package sitepages

import "time"

// Статусы страницы.
const (
	StatusActive = "active" // страница доступна
	StatusGone   = "gone"   // отдаёт 404/410 при перекрауле
)

// Результаты Upsert — что произошло со страницей при обходе.
const (
	UpsertNew       = "new"       // страница впервые добавлена
	UpsertChanged   = "changed"   // изменился контент (новый хэш)
	UpsertUnchanged = "unchanged" // контент не менялся
)

// Page — одна страница публичного сайта.
type Page struct {
	ID          string    `json:"id"`           // детерминированный sha1 от нормализованного URL
	URL         string    `json:"url"`          //
	Title       string    `json:"title"`        // <title> страницы
	Summary     string    `json:"summary"`      // meta description или начало текста
	Text        string    `json:"text,omitempty"` // полный видимый текст страницы (для просмотрщика)
	Section     string    `json:"section"`      // раздел/хлебные крошки из пути URL
	ContentHash string    `json:"content_hash"` // sha256 видимого текста
	Status      string    `json:"status"`       // active | gone
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	LastChanged time.Time `json:"last_changed"`
}
