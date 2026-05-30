// Package validator предоставляет средства для проверки полноты базы документов
// и генерации отчётов в форматах Markdown и HTML.
package validator

import (
	"context"
	"fmt"
	"strings"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// CompletenessReport — отчёт о полноте базы данных.
type CompletenessReport struct {
	UnparsedDocuments    []string // IDs документов с ошибками парсинга
	EmptyTextDocuments   []string // IDs документов без текста
	UnclassifiedDocuments []string // IDs документов без категории
	MissingFileDocuments []string // IDs документов без файла
	SourceDBMismatch     []string // Расхождения между RSS и БД
	Recommendations      []string // Рекомендации по устранению проблем
}

// ValidateCompleteness выполняет комплексную проверку полноты базы.
func ValidateCompleteness(
	docStore store.Store,
	eventStore store.EventStore,
	contestStore store.ContestStore,
	faqStore store.FAQStore,
	residentStore store.ResidentStore,
) CompletenessReport {
	ctx := context.Background()
	var report CompletenessReport

	// --- Проверка документов ---
	docs, err := docStore.List(ctx, store.Filter{})
	if err != nil {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Ошибка чтения реестра документов: %v", err))
		return report
	}

	for _, doc := range docs {
		// Документы без текста (пустой Title может сигнализировать об ошибке парсинга).
		if strings.TrimSpace(doc.Title) == "" {
			report.UnparsedDocuments = append(report.UnparsedDocuments, doc.ID)
		}

		// Документы без категории.
		if strings.TrimSpace(doc.Category) == "" {
			report.UnclassifiedDocuments = append(report.UnclassifiedDocuments, doc.ID)
		}

		// Документы без файла (пустой LocalPath и FileHash).
		if strings.TrimSpace(doc.LocalPath) == "" && strings.TrimSpace(doc.FileHash) == "" {
			report.MissingFileDocuments = append(report.MissingFileDocuments, doc.ID)
		}
	}

	// --- Проверка источников ---

	// Мероприятия.
	if eventStore != nil {
		events, err := eventStore.CountEvents(ctx)
		if err != nil {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				fmt.Sprintf("EventStore: ошибка чтения — %v", err))
		} else if events == 0 {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				"EventStore: нет записей мероприятий")
		}
	}

	// Конкурсы.
	if contestStore != nil {
		contests, err := contestStore.CountActiveContests(ctx)
		if err != nil {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				fmt.Sprintf("ContestStore: ошибка чтения — %v", err))
		} else if contests == 0 {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				"ContestStore: нет активных конкурсов")
		}
	}

	// FAQ.
	if faqStore != nil {
		faqs, err := faqStore.CountFAQItems(ctx)
		if err != nil {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				fmt.Sprintf("FAQStore: ошибка чтения — %v", err))
		} else if faqs == 0 {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				"FAQStore: нет записей FAQ")
		}
	}

	// Резиденты.
	if residentStore != nil {
		residents, err := residentStore.CountResidents(ctx)
		if err != nil {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				fmt.Sprintf("ResidentStore: ошибка чтения — %v", err))
		} else if residents == 0 {
			report.SourceDBMismatch = append(report.SourceDBMismatch,
				"ResidentStore: нет записей резидентов")
		}
	}

	// --- Генерация рекомендаций ---
	report.Recommendations = generateRecommendations(report)

	return report
}

// generateRecommendations формирует список рекомендаций на основе найденных проблем.
func generateRecommendations(r CompletenessReport) []string {
	var recs []string

	if len(r.UnparsedDocuments) > 0 {
		recs = append(recs, fmt.Sprintf(
			"Проверить %d документов с ошибками парсинга (IDs: %s)",
			len(r.UnparsedDocuments),
			strings.Join(r.UnparsedDocuments, ", ")))
	}

	if len(r.EmptyTextDocuments) > 0 {
		recs = append(recs, fmt.Sprintf(
			"Заполнить текст для %d документов без содержимого (IDs: %s)",
			len(r.EmptyTextDocuments),
			strings.Join(r.EmptyTextDocuments, ", ")))
	}

	if len(r.UnclassifiedDocuments) > 0 {
		recs = append(recs, fmt.Sprintf(
			"Классифицировать %d документов без категории (IDs: %s)",
			len(r.UnclassifiedDocuments),
			strings.Join(r.UnclassifiedDocuments, ", ")))
	}

	if len(r.MissingFileDocuments) > 0 {
		recs = append(recs, fmt.Sprintf(
			"Проверить наличие файлов для %d документов (IDs: %s)",
			len(r.MissingFileDocuments),
			strings.Join(r.MissingFileDocuments, ", ")))
	}

	if len(r.SourceDBMismatch) > 0 {
		recs = append(recs, fmt.Sprintf(
			"Устранить расхождения в источниках данных: %s",
			strings.Join(r.SourceDBMismatch, "; ")))
	}

	if len(recs) == 0 {
		recs = append(recs, "База данных полна, проблем не обнаружено")
	}

	return recs
}

// ToMarkdown форматирует отчёт в Markdown.
func ToMarkdown(report CompletenessReport) string {
	var sb strings.Builder

	sb.WriteString("# Отчёт о полноте базы данных\n\n")

	// Unparsed.
	sb.WriteString(fmt.Sprintf("## Документы с ошибками парсинга: %d\n\n", len(report.UnparsedDocuments)))
	for _, id := range report.UnparsedDocuments {
		sb.WriteString(fmt.Sprintf("- `%s`\n", id))
	}
	if len(report.UnparsedDocuments) == 0 {
		sb.WriteString("_Нет проблем_\n")
	}
	sb.WriteString("\n")

	// Unclassified.
	sb.WriteString(fmt.Sprintf("## Документы без категории: %d\n\n", len(report.UnclassifiedDocuments)))
	for _, id := range report.UnclassifiedDocuments {
		sb.WriteString(fmt.Sprintf("- `%s`\n", id))
	}
	if len(report.UnclassifiedDocuments) == 0 {
		sb.WriteString("_Нет проблем_\n")
	}
	sb.WriteString("\n")

	// Missing files.
	sb.WriteString(fmt.Sprintf("## Документы без файла: %d\n\n", len(report.MissingFileDocuments)))
	for _, id := range report.MissingFileDocuments {
		sb.WriteString(fmt.Sprintf("- `%s`\n", id))
	}
	if len(report.MissingFileDocuments) == 0 {
		sb.WriteString("_Нет проблем_\n")
	}
	sb.WriteString("\n")

	// Source DB mismatch.
	sb.WriteString(fmt.Sprintf("## Расхождения источников данных: %d\n\n", len(report.SourceDBMismatch)))
	for _, item := range report.SourceDBMismatch {
		sb.WriteString(fmt.Sprintf("- %s\n", item))
	}
	if len(report.SourceDBMismatch) == 0 {
		sb.WriteString("_Нет расхождений_\n")
	}
	sb.WriteString("\n")

	// Recommendations.
	sb.WriteString("## Рекомендации\n\n")
	for _, rec := range report.Recommendations {
		sb.WriteString(fmt.Sprintf("- %s\n", rec))
	}
	sb.WriteString("\n")

	return sb.String()
}

// ToHTML генерирует HTML-отчёт для админки.
func ToHTML(report CompletenessReport) string {
	var sb strings.Builder

	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("<meta charset=\"utf-8\">\n")
	sb.WriteString("<style>\n")
	sb.WriteString("body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 20px; }\n")
	sb.WriteString("h1 { color: #24292e; }\n")
	sb.WriteString("h2 { color: #586069; border-bottom: 1px solid #eaecef; padding-bottom: 4px; }\n")
	sb.WriteString("table { border-collapse: collapse; width: 100%%; margin-bottom: 16px; }\n")
	sb.WriteString("th, td { border: 1px solid #dfe2e5; padding: 8px; text-align: left; }\n")
	sb.WriteString("th { background: #f6f8fa; }\n")
	sb.WriteString(".ok { color: #28a745; font-style: italic; }\n")
	sb.WriteString(".warn { color: #d73a49; font-weight: bold; }\n")
	sb.WriteString(".rec { background: #f6f8fa; padding: 8px; margin: 4px 0; border-left: 3px solid #0366d6; }\n")
	sb.WriteString("</style>\n</head>\n<body>\n")

	sb.WriteString("<h1>Отчёт о полноте базы данных</h1>\n")

	// Unparsed documents.
	sb.WriteString(fmt.Sprintf("<h2>Документы с ошибками парсинга: %d</h2>\n", len(report.UnparsedDocuments)))
	if len(report.UnparsedDocuments) > 0 {
		sb.WriteString("<table><tr><th>ID</th></tr>\n")
		for _, id := range report.UnparsedDocuments {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn\">%s</td></tr>\n", escapeHTML(id)))
		}
		sb.WriteString("</table>\n")
	} else {
		sb.WriteString("<p class=\"ok\">Нет проблем</p>\n")
	}

	// Unclassified.
	sb.WriteString(fmt.Sprintf("<h2>Документы без категории: %d</h2>\n", len(report.UnclassifiedDocuments)))
	if len(report.UnclassifiedDocuments) > 0 {
		sb.WriteString("<table><tr><th>ID</th></tr>\n")
		for _, id := range report.UnclassifiedDocuments {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn\">%s</td></tr>\n", escapeHTML(id)))
		}
		sb.WriteString("</table>\n")
	} else {
		sb.WriteString("<p class=\"ok\">Нет проблем</p>\n")
	}

	// Missing files.
	sb.WriteString(fmt.Sprintf("<h2>Документы без файла: %d</h2>\n", len(report.MissingFileDocuments)))
	if len(report.MissingFileDocuments) > 0 {
		sb.WriteString("<table><tr><th>ID</th></tr>\n")
		for _, id := range report.MissingFileDocuments {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn\">%s</td></tr>\n", escapeHTML(id)))
		}
		sb.WriteString("</table>\n")
	} else {
		sb.WriteString("<p class=\"ok\">Нет проблем</p>\n")
	}

	// Source DB mismatch.
	sb.WriteString(fmt.Sprintf("<h2>Расхождения источников данных: %d</h2>\n", len(report.SourceDBMismatch)))
	if len(report.SourceDBMismatch) > 0 {
		sb.WriteString("<table><tr><th>Проблема</th></tr>\n")
		for _, item := range report.SourceDBMismatch {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn\">%s</td></tr>\n", escapeHTML(item)))
		}
		sb.WriteString("</table>\n")
	} else {
		sb.WriteString("<p class=\"ok\">Нет расхождений</p>\n")
	}

	// Recommendations.
	sb.WriteString("<h2>Рекомендации</h2>\n")
	for _, rec := range report.Recommendations {
		sb.WriteString(fmt.Sprintf("<div class=\"rec\">%s</div>\n", escapeHTML(rec)))
	}

	sb.WriteString("</body>\n</html>")
	return sb.String()
}

// escapeHTML экранирует базовые HTML-символы.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// Предотвращаем неиспользуемый импорт model.
var _ = model.StatusActive
