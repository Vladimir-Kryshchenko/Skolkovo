package admin

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/changes"
)

// TestChangesTemplateRenders проверяет, что шаблон страницы истории изменений
// (с авто-тегами, облаком тегов и панелью свежести) парсится и исполняется.
func TestChangesTemplateRenders(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	ev := changes.Event{
		EntityType: changes.EntitySitePage,
		EntityID:   "abc123",
		Title:      "Страница про льготы",
		Category:   "foundation / residents",
		Kind:       changes.KindNew,
		SourceURL:  "https://sk.ru/residents/preferences/",
		DetectedAt: now,
	}
	data := changesPageData{
		Rows:    []changeRow{{Event: ev, Tags: deriveTags(ev)}},
		Tag:     "Страница сайта",
		AllTags: []tagCount{{Name: "Страница сайта", Count: 1, Enc: "%D0%A1"}},
		BaseQS:  "entity_type=sitepage",
		Health: []sourceHealthRow{{
			Label: "Страницы сайта", State: "ok", StateLabel: "актуально", LastSuccess: "31.05.2026 12:00", Items: 3,
		}},
		Stats: changesStats{Total: 1, New: 1, LastParse: now},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "changes-layout", data); err != nil {
		t.Fatalf("исполнение шаблона changes-layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Страница про льготы", // строка таблицы
		"tag-chip",            // облако тегов
		"Свежесть источников", // панель свежести
		"Страница сайта",      // авто-тег / опция
		"health-ok",           // состояние источника
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе шаблона нет %q", want)
		}
	}
}

func TestDeriveTags(t *testing.T) {
	ev := changes.Event{
		EntityType: changes.EntityDocument,
		Category:   "Антикоррупция",
		Kind:       changes.KindUpdated,
		SourceURL:  "https://www.dochub.sk.ru/m/docs/1",
	}
	tags := deriveTags(ev)
	for _, want := range []string{"Документ", "Антикоррупция", "Обновлено", "dochub.sk.ru"} {
		if !containsStr(tags, want) {
			t.Errorf("deriveTags не содержит %q (получено %v)", want, tags)
		}
	}
}
