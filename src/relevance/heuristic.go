package relevance

import (
	"fmt"
	"strings"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/diff"
)

// criticalMarkers — слова-маркеры изменений, требующих действий резидента.
var criticalMarkers = []string{
	"утратил силу", "утратила силу", "утратили силу", "признан утратившим силу",
	"отменён", "отменен", "отменена", "прекращает действие", "прекращён",
	"вступает в силу", "вступают в силу", "новая редакция", "в новой редакции",
	"изменён срок", "изменен срок", "изменены сроки", "изменены требования",
	"новые требования", "обязан", "штраф", "исключение из реестра",
}

// classifyHeuristic оценивает важность изменения без LLM: по доле изменённых
// строк и наличию критических маркеров в изменённом тексте.
func classifyHeuristic(oldText, newText string, d diff.DocumentDiff) (changes.Severity, string) {
	added, removed := d.Summary.TotalAdded, d.Summary.TotalRemoved
	changed := added + removed

	if changed == 0 {
		return changes.SeverityInfo, "Изменения в метаданных документа без правки текста."
	}

	changedText := strings.ToLower(collectChangedText(d))
	marker := firstMarker(changedText)

	total := countLines(newText)
	if total == 0 {
		total = changed
	}
	ratio := float64(changed) / float64(total)

	var sev changes.Severity
	switch {
	case marker != "" || ratio > 0.5:
		sev = changes.SeverityCritical
	case ratio >= 0.2 || changed >= 30:
		sev = changes.SeverityWarning
	default:
		sev = changes.SeverityInfo
	}

	summary := fmt.Sprintf("Изменения в документе: +%d / −%d строк.", added, removed)
	if marker != "" {
		summary += fmt.Sprintf(" Обнаружен значимый маркер: «%s».", marker)
	}
	return sev, summary
}

// collectChangedText собирает текст всех добавленных/удалённых строк.
func collectChangedText(d diff.DocumentDiff) string {
	var b strings.Builder
	for _, c := range d.AddedLines {
		b.WriteString(c.NewText)
		b.WriteByte('\n')
	}
	for _, c := range d.RemovedLines {
		b.WriteString(c.OldText)
		b.WriteByte('\n')
	}
	for _, sec := range d.ModifiedSections {
		for _, c := range sec.Changes {
			b.WriteString(c.OldText)
			b.WriteByte(' ')
			b.WriteString(c.NewText)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func firstMarker(lowerText string) string {
	for _, m := range criticalMarkers {
		if strings.Contains(lowerText, m) {
			return m
		}
	}
	return ""
}

func countLines(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
