package admin

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestSitePagesTemplatesRender проверяет, что шаблоны раздела «Страницы сайта»
// (список и просмотрщик) парсятся и исполняются без ошибок.
func TestSitePagesListTemplateRenders(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	data := sitePagesPageData{
		Rows: []sitePageRow{{
			ID: "abc", URL: "https://sk.ru/residents/preferences/",
			Title: "Льготы резидентам", Section: "residents / preferences",
			Status: "active", StatusLabel: "доступна",
			Tags: []string{"льготы", "резиденты"}, LastChanged: now,
		}},
		Query: "льготы", Section: "residents / preferences", Status: "active",
		Sections:     []string{"residents / preferences", "foundation / documents"},
		AllTags:      []string{"льготы", "резиденты", "гранты"},
		SelectedTags: []string{"льготы"},
		SelectedSet:  map[string]bool{"льготы": true},
		Total:        120, GrandTotal: 2500, LastCrawl: now, HasStore: true,
		Page: 2, PerPage: 50, TotalPages: 3, RangeFrom: 51, RangeTo: 100,
		HasPrev: true, HasNext: true,
		PrevURL: "/sitepages?page=1", NextURL: "/sitepages?page=3",
		PageLinks: []pageLink{
			{Num: 1, URL: "/sitepages?page=1"},
			{Num: 2, URL: "/sitepages?page=2", Current: true},
			{Num: 3, URL: "/sitepages?page=3"},
		},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sitepages-layout", data); err != nil {
		t.Fatalf("sitepages-layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Льготы резидентам", "Последний обход", "/sitepages/abc", "На сайте ↗", "Страницы сайта",
		"ms-panel",                   // мультиселект тегов отрендерился
		`value="гранты"`,             // опция тега
		"sp-tag",                     // чипы тегов в строке
		"51–100", "120",              // диапазон строк и общий счётчик по фильтру
		"всего в базе: 2500",         // grand total
		"/sitepages?page=3", "Вперёд", // пагинатор
		"стр. 2 из 3",                // индикатор страницы
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе списка нет %q", want)
		}
	}
	// Выбранный тег «льготы» должен быть отмечен (checked).
	if !strings.Contains(out, `value="льготы" checked`) {
		t.Error("выбранный тег «льготы» не отмечен checked в мультиселекте")
	}
}

func TestSitePageViewTemplateRenders(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	data := sitePageViewData{
		ID: "abc", URL: "https://sk.ru/residents/preferences/",
		Title: "Льготы резидентам", Section: "residents / preferences",
		Status: "active", StatusLabel: "доступна",
		Summary: "Кратко о льготах", Text: "Полный текст страницы про льготы резидентам.",
		HasText: true, FirstSeen: now, LastSeen: now, LastChanged: now,
		Enriched:    true,
		AISummary:   "Страница о налоговых льготах для резидентов.",
		Goals:       "Объяснить резиденту доступные льготы.",
		Theses:      []string{"Освобождение от НДС", "Пониженные страховые взносы"},
		Conclusions: "Резидент получает существенную экономию.",
		Tags:        []string{"льготы", "резиденты"},
		RelatedByTags: []relRow{{
			ID: "def", URL: "https://sk.ru/residents/", Title: "Резидентам", Section: "residents", Shared: 2,
		}},
		RelatedSemantic: []relRow{{
			ID: "ghi", URL: "https://sk.ru/foundation/", Title: "О фонде", Section: "foundation",
		}},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sitepage-view-layout", data); err != nil {
		t.Fatalf("sitepage-view-layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Полный текст страницы", "Открыть на сайте Сколково", "К списку страниц", data.URL,
		"Краткое описание", "Страница о налоговых льготах",
		"Цели страницы", "Важные тезисы", "Освобождение от НДС", "Выводы",
		"Связанные страницы по общим тегам", "/sitepages/def", "2 общих",
		"Похожие страницы по смыслу", "/sitepages/ghi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе просмотрщика нет %q", want)
		}
	}
}

// TestSitePageViewNotEnriched проверяет плашку «ещё не аннотировано».
func TestSitePageViewNotEnriched(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	data := sitePageViewData{
		ID: "abc", URL: "https://sk.ru/x/", Title: "X", Status: "active", StatusLabel: "доступна",
		Text: "текст", HasText: true, FirstSeen: now, LastSeen: now, LastChanged: now,
		Enriched: false,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sitepage-view-layout", data); err != nil {
		t.Fatalf("sitepage-view-layout: %v", err)
	}
	if !strings.Contains(buf.String(), "ещё не сформирована") {
		t.Error("ожидали плашку «ИИ-аннотация ещё не сформирована»")
	}
}

func TestSitePageStatusLabel(t *testing.T) {
	if sitePageStatusLabel("active") != "доступна" {
		t.Error("active label")
	}
	if sitePageStatusLabel("gone") != "недоступна (404)" {
		t.Error("gone label")
	}
}
