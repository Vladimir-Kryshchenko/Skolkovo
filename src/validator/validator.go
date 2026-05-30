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
	UnparsedDocuments     []string // IDs документов с ошибками парсинга
	EmptyTextDocuments    []string // IDs документов без текста
	UnclassifiedDocuments []string // IDs документов без категории
	MissingFileDocuments  []string // IDs документов без файла
	SourceDBMismatch      []string // Расхождения между RSS и БД
	Recommendations       []string // Рекомендации по устранению проблем
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

const validatorHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Отчёт о полноте базы данных</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --primary: #0073ea;
  --primary-light: #e6f2fc;
  --bg: #ffffff;
  --surface: #f6f8fa;
  --surface-border: #d1d5da;
  --text: #24292e;
  --text-secondary: #586069;
  --ok-bg: #e6ffed;
  --ok-text: #22863a;
  --ok-border: #34d058;
  --warn-bg: #ffeef0;
  --warn-text: #b31d28;
  --warn-border: #f97583;
  --shadow: 0 1px 3px rgba(0,0,0,0.08);
  --radius: 8px;
}

@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b;
    --surface: #23273a;
    --surface-border: #3a3f52;
    --text: #e6e6e6;
    --text-secondary: #a0a6b8;
    --primary-light: #1a2d42;
    --ok-bg: #1a2e23;
    --ok-text: #6ecb7e;
    --ok-border: #2d8a3e;
    --warn-bg: #2e1a1e;
    --warn-text: #f08a94;
    --warn-border: #c04a54;
    --shadow: 0 1px 3px rgba(0,0,0,0.3);
  }
}

[data-theme="dark"] {
  --bg: #181b2b;
  --surface: #23273a;
  --surface-border: #3a3f52;
  --text: #e6e6e6;
  --text-secondary: #a0a6b8;
  --primary-light: #1a2d42;
  --ok-bg: #1a2e23;
  --ok-text: #6ecb7e;
  --ok-border: #2d8a3e;
  --warn-bg: #2e1a1e;
  --warn-text: #f08a94;
  --warn-border: #c04a54;
  --shadow: 0 1px 3px rgba(0,0,0,0.3);
}

* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: var(--bg);
  color: var(--text);
  line-height: 1.6;
  padding: 24px;
}

.container { max-width: 960px; margin: 0 auto; }

/* Theme toggle */
.theme-toggle {
  position: fixed;
  top: 16px;
  right: 16px;
  background: var(--surface);
  border: 1px solid var(--surface-border);
  border-radius: var(--radius);
  padding: 8px 14px;
  cursor: pointer;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
  font-weight: 500;
  box-shadow: var(--shadow);
  z-index: 10;
}
.theme-toggle:hover { border-color: var(--primary); }

/* Header */
.page-header {
  margin-bottom: 24px;
}
.page-header h1 {
  font-size: 22px;
  font-weight: 700;
  color: var(--primary);
  margin-bottom: 4px;
}
.page-header p {
  font-size: 14px;
  color: var(--text-secondary);
}

/* Section card */
.section {
  background: var(--surface);
  border: 1px solid var(--surface-border);
  border-radius: var(--radius);
  margin-bottom: 16px;
  box-shadow: var(--shadow);
  overflow: hidden;
}
.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--surface-border);
  flex-wrap: wrap;
  gap: 8px;
}
.section-header h2 {
  font-size: 15px;
  font-weight: 600;
  color: var(--text);
}
.count-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 12px;
  font-size: 13px;
  font-weight: 600;
}
.count-ok { background: var(--ok-bg); color: var(--ok-text); }
.count-warn { background: var(--warn-bg); color: var(--warn-text); }

/* Table */
.table-wrap { overflow-x: auto; }
table {
  width: 100%;
  border-collapse: collapse;
}
th, td {
  padding: 10px 18px;
  text-align: left;
  font-size: 13px;
  border-bottom: 1px solid var(--surface-border);
}
th {
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--surface);
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
td { color: var(--text); }
td.warn-cell { color: var(--warn-text); font-weight: 500; }
tr:last-child td { border-bottom: none; }

/* OK message */
.ok-msg {
  padding: 14px 18px;
  color: var(--ok-text);
  background: var(--ok-bg);
  font-size: 13px;
  font-weight: 500;
}

/* Recommendation card */
.rec-card {
  background: var(--surface);
  border: 1px solid var(--surface-border);
  border-radius: var(--radius);
  padding: 14px 18px;
  margin-bottom: 8px;
  box-shadow: var(--shadow);
  font-size: 13px;
  line-height: 1.5;
  border-left: 3px solid var(--primary);
}

/* Empty state */
.empty {
  text-align: center;
  padding: 40px 20px;
  color: var(--text-secondary);
  font-size: 14px;
}

/* Responsive */
@media (max-width: 768px) {
  body { padding: 16px; }
  .section-header { padding: 12px 14px; }
  th, td { padding: 8px 14px; }
  .rec-card { padding: 12px 14px; }
}

@media (max-width: 480px) {
  body { padding: 12px; }
  .page-header h1 { font-size: 18px; }
  .section-header { flex-direction: column; align-items: flex-start; }
  .theme-toggle { top: 12px; right: 12px; padding: 6px 10px; font-size: 12px; }
}
</style>
</head>
<body>
<div class="container">
<button class="theme-toggle" onclick="toggleTheme()">Сменить тему</button>
<script>
(function(){var t=localStorage.getItem('validator-theme');if(t){document.documentElement.setAttribute('data-theme',t)}else if(window.matchMedia('(prefers-color-scheme:dark)').matches){document.documentElement.setAttribute('data-theme','dark')}})();
function toggleTheme(){var c=document.documentElement.getAttribute('data-theme');var n=c==='dark'?'light':'dark';document.documentElement.setAttribute('data-theme',n);localStorage.setItem('validator-theme',n)}
</script>
`

const validatorHTMLTail = `
</div>
</body>
</html>`

// ToHTML генерирует HTML-отчёт для админки.
func ToHTML(report CompletenessReport) string {
	var sb strings.Builder

	sb.WriteString(validatorHTML)

	// Header.
	sb.WriteString("<div class=\"page-header\">\n")
	sb.WriteString("<h1>Отчёт о полноте базы данных</h1>\n")
	sb.WriteString(fmt.Sprintf("<p>Результаты проверки целостности и полноты данных</p>\n"))
	sb.WriteString("</div>\n")

	// Unparsed documents.
	sb.WriteString(fmt.Sprintf("<div class=\"section\"><div class=\"section-header\"><h2>Документы с ошибками парсинга</h2><span class=\"count-badge %s\">%d</span></div>\n", countClass(len(report.UnparsedDocuments)), len(report.UnparsedDocuments)))
	if len(report.UnparsedDocuments) > 0 {
		sb.WriteString("<div class=\"table-wrap\"><table><tr><th>ID документа</th></tr>\n")
		for _, id := range report.UnparsedDocuments {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn-cell\">%s</td></tr>\n", escapeHTML(id)))
		}
		sb.WriteString("</table></div>\n")
	} else {
		sb.WriteString("<div class=\"ok-msg\">Нет проблем</div>\n")
	}
	sb.WriteString("</div>\n")

	// Unclassified.
	sb.WriteString(fmt.Sprintf("<div class=\"section\"><div class=\"section-header\"><h2>Документы без категории</h2><span class=\"count-badge %s\">%d</span></div>\n", countClass(len(report.UnclassifiedDocuments)), len(report.UnclassifiedDocuments)))
	if len(report.UnclassifiedDocuments) > 0 {
		sb.WriteString("<div class=\"table-wrap\"><table><tr><th>ID документа</th></tr>\n")
		for _, id := range report.UnclassifiedDocuments {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn-cell\">%s</td></tr>\n", escapeHTML(id)))
		}
		sb.WriteString("</table></div>\n")
	} else {
		sb.WriteString("<div class=\"ok-msg\">Нет проблем</div>\n")
	}
	sb.WriteString("</div>\n")

	// Missing files.
	sb.WriteString(fmt.Sprintf("<div class=\"section\"><div class=\"section-header\"><h2>Документы без файла</h2><span class=\"count-badge %s\">%d</span></div>\n", countClass(len(report.MissingFileDocuments)), len(report.MissingFileDocuments)))
	if len(report.MissingFileDocuments) > 0 {
		sb.WriteString("<div class=\"table-wrap\"><table><tr><th>ID документа</th></tr>\n")
		for _, id := range report.MissingFileDocuments {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn-cell\">%s</td></tr>\n", escapeHTML(id)))
		}
		sb.WriteString("</table></div>\n")
	} else {
		sb.WriteString("<div class=\"ok-msg\">Нет проблем</div>\n")
	}
	sb.WriteString("</div>\n")

	// Source DB mismatch.
	sb.WriteString(fmt.Sprintf("<div class=\"section\"><div class=\"section-header\"><h2>Расхождения источников данных</h2><span class=\"count-badge %s\">%d</span></div>\n", countClass(len(report.SourceDBMismatch)), len(report.SourceDBMismatch)))
	if len(report.SourceDBMismatch) > 0 {
		sb.WriteString("<div class=\"table-wrap\"><table><tr><th>Проблема</th></tr>\n")
		for _, item := range report.SourceDBMismatch {
			sb.WriteString(fmt.Sprintf("<tr><td class=\"warn-cell\">%s</td></tr>\n", escapeHTML(item)))
		}
		sb.WriteString("</table></div>\n")
	} else {
		sb.WriteString("<div class=\"ok-msg\">Нет расхождений</div>\n")
	}
	sb.WriteString("</div>\n")

	// Recommendations.
	sb.WriteString("<h2 style=\"font-size:16px;font-weight:600;color:var(--text);margin:24px 0 12px;\">Рекомендации</h2>\n")
	for _, rec := range report.Recommendations {
		sb.WriteString(fmt.Sprintf("<div class=\"rec-card\">%s</div>\n", escapeHTML(rec)))
	}

	sb.WriteString(validatorHTMLTail)
	return sb.String()
}

// countClass возвращает CSS-класс для бейджа счётчика.
func countClass(n int) string {
	if n == 0 {
		return "count-ok"
	}
	return "count-warn"
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
