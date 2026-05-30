// Package diff предоставляет утилиты для сравнения текстовых документов
// с использованием LCS-алгоритма и генерации отчётов в HTML.
package diff

import (
	"fmt"
	"strings"
)

// ChangeType описывает тип единичного изменения.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeRemoved  ChangeType = "removed"
	ChangeModified ChangeType = "modified"
)

// Change — единичное изменение в документе.
type Change struct {
	Type       ChangeType
	OldText    string
	NewText    string
	LineNumber int
}

// DiffSection — секция изменений с заголовком.
type DiffSection struct {
	Title   string
	Changes []Change
}

// DiffSummary — сводка по результатам сравнения.
type DiffSummary struct {
	TotalAdded    int
	TotalRemoved  int
	TotalModified int
}

// DocumentDiff — полный результат сравнения двух документов.
type DocumentDiff struct {
	AddedLines       []Change
	RemovedLines     []Change
	ModifiedSections []DiffSection
	Summary          DiffSummary
}

// CompareDocuments сравнивает два текста и возвращает структурированную разницу.
// Использует LCS (longest common subsequence) алгоритм.
func CompareDocuments(text1, text2 string) DocumentDiff {
	lines1 := splitLines(text1)
	lines2 := splitLines(text2)

	lcs := computeLCS(lines1, lines2)
	diff := buildDiff(lines1, lines2, lcs)

	return groupIntoSections(diff)
}

// splitLines разбивает текст на строки, убирая пустые финальные элементы.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	// Убираем trailing empty line от последнего \n
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeLCS возвращает матрицу длин общей подпоследовательности.
func computeLCS(a, b []string) [][]int {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}
	return dp
}

// buildDiff восстанавливает список изменений из LCS-матрицы.
func buildDiff(lines1, lines2 []string, lcs [][]int) []Change {
	var changes []Change
	i, j := len(lines1), len(lines2)

	// Собираем изменения в обратном порядке.
	var reversed []Change
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && lines1[i-1] == lines2[j-1] {
			// Строка совпадает — не является изменением.
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			// Строка добавлена во втором тексте.
			reversed = append(reversed, Change{
				Type:       ChangeAdded,
				NewText:    lines2[j-1],
				LineNumber: j,
			})
			j--
		} else if i > 0 {
			// Строка удалена из первого текста.
			reversed = append(reversed, Change{
				Type:       ChangeRemoved,
				OldText:    lines1[i-1],
				LineNumber: i,
			})
			i--
		}
	}

	// Разворачиваем в прямой порядок.
	for k := len(reversed) - 1; k >= 0; k-- {
		changes = append(changes, reversed[k])
	}

	return changes
}

// groupIntoSections группирует плоский список изменений в секции.
// Смежные добавления/удаления объединяются в modified-секции.
func groupIntoSections(changes []Change) DocumentDiff {
	var diff DocumentDiff

	if len(changes) == 0 {
		return diff
	}

	// Разделяем на added/removed/modified.
	for _, ch := range changes {
		switch ch.Type {
		case ChangeAdded:
			diff.AddedLines = append(diff.AddedLines, ch)
			diff.Summary.TotalAdded++
		case ChangeRemoved:
			diff.RemovedLines = append(diff.RemovedLines, ch)
			diff.Summary.TotalRemoved++
		}
	}

	// Формируем секции: идём по изменениям, группируем смежие.
	var currentSection *DiffSection
	flushSection := func() {
		if currentSection != nil && len(currentSection.Changes) > 0 {
			// Определяем заголовок секции.
			currentSection.Title = deriveSectionTitle(currentSection.Changes)
			diff.ModifiedSections = append(diff.ModifiedSections, *currentSection)
			diff.Summary.TotalModified++
		}
		currentSection = nil
	}

	for _, ch := range changes {
		// Added/removed подряд — это modified-секция.
		if ch.Type == ChangeAdded || ch.Type == ChangeRemoved {
			if currentSection == nil {
				currentSection = &DiffSection{}
			}
			currentSection.Changes = append(currentSection.Changes, ch)
		} else {
			flushSection()
		}
	}
	flushSection()

	return diff
}

// deriveSectionTitle выводит заголовок секции по содержимому изменений.
func deriveSectionTitle(changes []Change) string {
	var added, removed int
	for _, ch := range changes {
		switch ch.Type {
		case ChangeAdded:
			added++
		case ChangeRemoved:
			removed++
		}
	}
	if added > 0 && removed > 0 {
		return fmt.Sprintf("Изменено (%d добавлено, %d удалено)", added, removed)
	}
	if added > 0 {
		return fmt.Sprintf("Добавлено (%d строк)", added)
	}
	return fmt.Sprintf("Удалено (%d строк)", removed)
}

const diffHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Сравнение документов</title>
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
  --added-bg: #e6ffed;
  --added-text: #22863a;
  --added-border: #34d058;
  --removed-bg: #ffeef0;
  --removed-text: #b31d28;
  --removed-border: #f97583;
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
    --added-bg: #1a2e23;
    --added-text: #6ecb7e;
    --added-border: #2d8a3e;
    --removed-bg: #2e1a1e;
    --removed-text: #f08a94;
    --removed-border: #c04a54;
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
  --added-bg: #1a2e23;
  --added-text: #6ecb7e;
  --added-border: #2d8a3e;
  --removed-bg: #2e1a1e;
  --removed-text: #f08a94;
  --removed-border: #c04a54;
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

/* Summary card */
.summary {
  background: var(--surface);
  border: 1px solid var(--surface-border);
  border-radius: var(--radius);
  padding: 16px 20px;
  margin-bottom: 20px;
  box-shadow: var(--shadow);
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.summary-title {
  font-weight: 700;
  font-size: 15px;
  color: var(--primary);
}
.badge {
  display: inline-flex;
  align-items: center;
  padding: 3px 10px;
  border-radius: 12px;
  font-size: 13px;
  font-weight: 600;
}
.badge-added { background: var(--added-bg); color: var(--added-text); }
.badge-removed { background: var(--removed-bg); color: var(--removed-text); }
.badge-modified { background: var(--primary-light); color: var(--primary); }

/* Section */
.section {
  background: var(--surface);
  border: 1px solid var(--surface-border);
  border-radius: var(--radius);
  margin-bottom: 12px;
  box-shadow: var(--shadow);
  overflow: hidden;
}
.section-title {
  font-weight: 600;
  font-size: 14px;
  padding: 12px 16px;
  background: var(--primary-light);
  color: var(--primary);
  border-bottom: 1px solid var(--surface-border);
}

/* Diff lines */
.diff-line {
  display: block;
  padding: 6px 16px;
  font-family: 'Figtree', monospace;
  font-size: 13px;
  line-height: 1.5;
  border-left: 3px solid transparent;
  word-break: break-word;
}
.diff-line-added {
  background: var(--added-bg);
  color: var(--added-text);
  border-left-color: var(--added-border);
}
.diff-line-removed {
  background: var(--removed-bg);
  color: var(--removed-text);
  border-left-color: var(--removed-border);
}
.diff-prefix {
  font-weight: 700;
  margin-right: 8px;
  user-select: none;
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
  .summary { padding: 12px 16px; gap: 10px; }
  .section-title { padding: 10px 14px; }
  .diff-line { padding: 5px 14px; }
}

@media (max-width: 480px) {
  body { padding: 12px; }
  .summary { flex-direction: column; align-items: flex-start; }
  .theme-toggle { top: 12px; right: 12px; padding: 6px 10px; font-size: 12px; }
}
</style>
</head>
<body>
<div class="container">
<button class="theme-toggle" onclick="toggleTheme()">Сменить тему</button>
<script>
(function(){var t=localStorage.getItem('diff-theme');if(t){document.documentElement.setAttribute('data-theme',t)}else if(window.matchMedia('(prefers-color-scheme:dark)').matches){document.documentElement.setAttribute('data-theme','dark')}})();
function toggleTheme(){var c=document.documentElement.getAttribute('data-theme');var n=c==='dark'?'light':'dark';document.documentElement.setAttribute('data-theme',n);localStorage.setItem('diff-theme',n)}
</script>
`

const diffHTMLTail = `
</div>
</body>
</html>`

// ToHTML генерирует HTML-представление diff с подсветкой строк.
// Удалённые строки — красный фон, добавленные — зелёный.
func ToHTML(diff DocumentDiff) string {
	var sb strings.Builder

	sb.WriteString(diffHTML)

	// Summary.
	sb.WriteString("<div class=\"summary\">")
	sb.WriteString(fmt.Sprintf("<span class=\"summary-title\">Сравнение документов</span>"))
	sb.WriteString(fmt.Sprintf("<span class=\"badge badge-added\">+%d</span>", diff.Summary.TotalAdded))
	sb.WriteString(fmt.Sprintf("<span class=\"badge badge-removed\">-%d</span>", diff.Summary.TotalRemoved))
	sb.WriteString(fmt.Sprintf("<span class=\"badge badge-modified\">~%d</span>", diff.Summary.TotalModified))
	sb.WriteString("</div>\n")

	// Секции.
	for _, sec := range diff.ModifiedSections {
		sb.WriteString(fmt.Sprintf("<div class=\"section\"><div class=\"section-title\">%s</div>\n", escapeHTML(sec.Title)))
		for _, ch := range sec.Changes {
			switch ch.Type {
			case ChangeAdded:
				sb.WriteString(fmt.Sprintf("<span class=\"diff-line diff-line-added\"><span class=\"diff-prefix\">+</span>%s</span>\n", escapeHTML(ch.NewText)))
			case ChangeRemoved:
				sb.WriteString(fmt.Sprintf("<span class=\"diff-line diff-line-removed\"><span class=\"diff-prefix\">-</span>%s</span>\n", escapeHTML(ch.OldText)))
			}
		}
		sb.WriteString("</div>\n")
	}

	// Отдельные добавленные строки (не вошедшие в секции).
	if len(diff.AddedLines) > 0 && len(diff.ModifiedSections) == 0 {
		sb.WriteString("<div class=\"section\"><div class=\"section-title\">Добавленные строки</div>\n")
		for _, ch := range diff.AddedLines {
			sb.WriteString(fmt.Sprintf("<span class=\"diff-line diff-line-added\"><span class=\"diff-prefix\">+</span>%s</span>\n", escapeHTML(ch.NewText)))
		}
		sb.WriteString("</div>\n")
	}

	// Отдельные удалённые строки (не вошедшие в секции).
	if len(diff.RemovedLines) > 0 && len(diff.ModifiedSections) == 0 {
		sb.WriteString("<div class=\"section\"><div class=\"section-title\">Удалённые строки</div>\n")
		for _, ch := range diff.RemovedLines {
			sb.WriteString(fmt.Sprintf("<span class=\"diff-line diff-line-removed\"><span class=\"diff-prefix\">-</span>%s</span>\n", escapeHTML(ch.OldText)))
		}
		sb.WriteString("</div>\n")
	}

	// Empty state.
	if len(diff.ModifiedSections) == 0 && len(diff.AddedLines) == 0 && len(diff.RemovedLines) == 0 {
		sb.WriteString("<div class=\"empty\">Нет изменений между документами</div>\n")
	}

	sb.WriteString(diffHTMLTail)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
