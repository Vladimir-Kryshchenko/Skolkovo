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
			Status: "active", StatusLabel: "доступна", LastChanged: now,
		}},
		Query: "льготы", Section: "residents / preferences", Status: "active",
		Sections: []string{"residents / preferences", "foundation / documents"},
		Total:    1, LastCrawl: now, HasStore: true,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sitepages-layout", data); err != nil {
		t.Fatalf("sitepages-layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Льготы резидентам", "Последний обход", "/sitepages/abc", "На сайте ↗", "Страницы сайта"} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе списка нет %q", want)
		}
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
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "sitepage-view-layout", data); err != nil {
		t.Fatalf("sitepage-view-layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Полный текст страницы", "Открыть на сайте Сколково", "К списку страниц", data.URL} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе просмотрщика нет %q", want)
		}
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
