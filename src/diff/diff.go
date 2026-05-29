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
	AddedLines     []Change
	RemovedLines   []Change
	ModifiedSections []DiffSection
	Summary        DiffSummary
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

// ToHTML генерирует HTML-представление diff с подсветкой строк.
// Удалённые строки — красный фон, добавленные — зелёный.
func ToHTML(diff DocumentDiff) string {
	var sb strings.Builder

	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("<meta charset=\"utf-8\">\n")
	sb.WriteString("<style>\n")
	sb.WriteString("body { font-family: monospace; margin: 20px; }\n")
	sb.WriteString(".removed { background-color: #ffeef0; color: #b31d28; padding: 2px 8px; display: block; }\n")
	sb.WriteString(".added { background-color: #e6ffed; color: #22863a; padding: 2px 8px; display: block; }\n")
	sb.WriteString(".section-title { font-weight: bold; margin-top: 16px; padding: 4px 8px; background: #f6f8fa; border: 1px solid #d1d5da; }\n")
	sb.WriteString(".summary { margin-bottom: 16px; padding: 8px; background: #f6f8fa; border: 1px solid #d1d5da; }\n")
	sb.WriteString("</style>\n</head>\n<body>\n")

	// Summary.
	sb.WriteString("<div class=\"summary\">")
	sb.WriteString(fmt.Sprintf("<strong>Сравнение документов:</strong> "))
	sb.WriteString(fmt.Sprintf("+%d ", diff.Summary.TotalAdded))
	sb.WriteString(fmt.Sprintf("-%d ", diff.Summary.TotalRemoved))
	sb.WriteString(fmt.Sprintf("~%d", diff.Summary.TotalModified))
	sb.WriteString("</div>\n")

	// Секции.
	for _, sec := range diff.ModifiedSections {
		sb.WriteString(fmt.Sprintf("<div class=\"section-title\">%s</div>\n", escapeHTML(sec.Title)))
		for _, ch := range sec.Changes {
			switch ch.Type {
			case ChangeAdded:
				sb.WriteString(fmt.Sprintf("<span class=\"added\">+ %s</span>\n", escapeHTML(ch.NewText)))
			case ChangeRemoved:
				sb.WriteString(fmt.Sprintf("<span class=\"removed\">- %s</span>\n", escapeHTML(ch.OldText)))
			}
		}
	}

	// Отдельные добавленные строки (не вошедшие в секции).
	if len(diff.AddedLines) > 0 && len(diff.ModifiedSections) == 0 {
		sb.WriteString("<div class=\"section-title\">Добавленные строки</div>\n")
		for _, ch := range diff.AddedLines {
			sb.WriteString(fmt.Sprintf("<span class=\"added\">+ %s</span>\n", escapeHTML(ch.NewText)))
		}
	}

	// Отдельные удалённые строки (не вошедшие в секции).
	if len(diff.RemovedLines) > 0 && len(diff.ModifiedSections) == 0 {
		sb.WriteString("<div class=\"section-title\">Удалённые строки</div>\n")
		for _, ch := range diff.RemovedLines {
			sb.WriteString(fmt.Sprintf("<span class=\"removed\">- %s</span>\n", escapeHTML(ch.OldText)))
		}
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
