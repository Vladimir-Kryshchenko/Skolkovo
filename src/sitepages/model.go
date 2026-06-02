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

	// ИИ-обогащение (заполняется агентом «Аннотатор страниц», не краулером).
	Tags        []string   `json:"tags,omitempty"`        // авто-теги (нормализованы против словаря)
	AISummary   string     `json:"ai_summary,omitempty"`  // краткое описание (ИИ), отличается от Summary
	Goals       string     `json:"goals,omitempty"`       // цели страницы (ИИ)
	Theses      []string   `json:"theses,omitempty"`      // важные тезисы (ИИ)
	Conclusions string     `json:"conclusions,omitempty"` // выводы (ИИ)
	EnrichedAt  *time.Time `json:"enriched_at,omitempty"` // когда аннотировано (nil — ещё нет)
	EnrichHash  string     `json:"-"`                     // content_hash на момент аннотирования
}

// Enriched сообщает, аннотирована ли страница ИИ для текущего контента.
func (p *Page) Enriched() bool {
	return p.EnrichedAt != nil
}
