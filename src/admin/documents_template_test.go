package admin

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
)

// TestDocumentsTemplateRenders проверяет, что главная страница документов с новыми
// фильтрами (категория, даты) и колонкой файла («Открыть на сайте» / «Просмотр»/«Скачать»)
// парсится и исполняется без ошибок.
func TestDocumentsTemplateRenders(t *testing.T) {
	pub := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	noFile := docView{
		Document: model.Document{ID: "doc1", Title: "Антикоррупционная политика", Category: "Антикоррупция",
			Status: model.StatusActive, SourceURL: "https://dochub.sk.ru/foundation/documents/m/docs/24539/download.aspx", PublishedAt: &pub},
		StatusStr:      "действует",
		SourceLinkURL:  "https://dochub.sk.ru/foundation/documents/m/docs/24539/download.aspx",
		SourceLinkText: "открыть на сайте ↗",
		WebURL:         "https://dochub.sk.ru/foundation/documents/m/docs/24539/download.aspx",
	}
	withFile := docView{
		Document:  model.Document{ID: "doc2", Title: "Локальный документ", Category: "Правила проектирования", Status: model.StatusActive, LocalPath: "/data/docs/x.pdf", Indexed: true},
		StatusStr: "действует", FileSize: "120 КБ", FileAge: "2 дн. назад",
	}
	data := pageData{
		Stats:          stats{Total: 2, Active: 2},
		Docs:           []docView{noFile, withFile},
		FilterCategory: "Антикоррупция",
		Categories:     []string{"Антикоррупция", "Правила проектирования"},
		BaseQS:         "category=%D0%90",
		FlashKind:      "ok",
		Tab:            "documents",
		Settings:       model.SchedulerSettings{},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("исполнение шаблона layout: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Открыть на сайте",      // кнопка для документа без файла
		"Все категории",         // фильтр категорий
		"Загружено на sk.ru от", // фильтр по дате публикации
		"Обновлено от",          // фильтр по дате обновления
		"Применить",             // кнопка фильтра
		"Антикоррупционная политика",
		"Просмотр", // действие для документа с файлом
		"Скачать",
		"Переиндексировать поиск", // переименованная кнопка обслуживания
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в выводе документов нет %q", want)
		}
	}
}
