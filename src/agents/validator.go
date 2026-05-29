package agents

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// ChecklistStore — интерфейс хранилища чек-листов (алиас из store).
type ChecklistStore = store.ChecklistStore

// ChecklistType — алиас типа процедуры из model.
type ChecklistType = model.ChecklistType

const (
	ChecklistEntry     ChecklistType = model.ChecklistEntry
	ChecklistReporting ChecklistType = model.ChecklistReporting
	ChecklistExtension ChecklistType = model.ChecklistExtension
	ChecklistExit      ChecklistType = model.ChecklistExit
)

// ValidationIssue — проблема, обнаруженная при валидации документа.
type ValidationIssue struct {
	// Type — тип проблемы: "error", "warning", "info".
	Type string `json:"type"`
	// Message — описание проблемы.
	Message string `json:"message"`
	// Field — поле или раздел документа, к которому относится проблема.
	Field string `json:"field,omitempty"`
	// Severity — числовая оценка серьёзности (1-10, где 10 — критично).
	Severity int `json:"severity"`
}

// ValidationReport — отчёт агента-валидатора.
type ValidationReport struct {
	// Issues — список обнаруженных проблем.
	Issues []ValidationIssue `json:"issues"`
	// Score — общая оценка документа от 0 до 100.
	Score int `json:"score"`
	// Passed — документ прошёл валидацию (нет ошибок типа "error").
	Passed bool `json:"passed"`
}

// ValidatorAgent — агент-валидатор, проверяющий документы по правилам.
type ValidatorAgent struct {
	ragService     *rag.Service
	checklistStore ChecklistStore
}

// NewValidatorAgent создаёт агента-валидатора.
func NewValidatorAgent(ragSvc *rag.Service, clStore ChecklistStore) *ValidatorAgent {
	return &ValidatorAgent{
		ragService:     ragSvc,
		checklistStore: clStore,
	}
}

// ValidateDocument проверяет документ по набору правил.
//
// Параметры:
//   - ctx — контекст с возможностью отмены.
//   - documentText — полный текст документа для проверки.
//   - procedureType — тип процедуры: "entry", "reporting", "extension", "exit".
//   - clientID — опционально, идентификатор клиента.
//
// Возвращает ValidationReport с обнаруженными проблемами, оценкой и флагом прохождения.
func (a *ValidatorAgent) ValidateDocument(ctx context.Context, documentText, procedureType, clientID string) (ValidationReport, error) {
	if strings.TrimSpace(documentText) == "" {
		return ValidationReport{
			Issues: []ValidationIssue{
				{Type: "error", Message: "Документ пуст", Field: "content", Severity: 10},
			},
			Score:  0,
			Passed: false,
		}, nil
	}

	var issues []ValidationIssue

	// 1. Проверка наличия обязательных разделов.
	issues = append(issues, checkRequiredSections(documentText)...)

	// 2. Проверка ссылок на нормативные документы (через RAG).
	ragIssues, err := a.checkRegulatoryReferences(ctx, documentText)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:     "info",
			Message:  fmt.Sprintf("Не удалось проверить нормативные ссылки: %v", err),
			Field:    "references",
			Severity: 1,
		})
	} else {
		issues = append(issues, ragIssues...)
	}

	// 3. Проверка полноты (минимальная длина).
	issues = append(issues, checkCompleteness(documentText)...)

	// 4. Проверка формата (даты, ИНН, ОГРН паттерны).
	issues = append(issues, checkFormatPatterns(documentText)...)

	// 5. Проверка по чек-листу процедуры.
	if procedureType != "" {
		checklistIssues, err := a.checkProcedureChecklist(ctx, procedureType, clientID, documentText)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:     "info",
				Message:  fmt.Sprintf("Не удалось проверить чек-лист: %v", err),
				Field:    "checklist",
				Severity: 1,
			})
		} else {
			issues = append(issues, checklistIssues...)
		}
	}

	// Рассчитываем итоговую оценку.
	score := calculateScore(issues)
	passed := !hasErrors(issues)

	return ValidationReport{
		Issues: issues,
		Score:  score,
		Passed: passed,
	}, nil
}

// checkRequiredSections проверяет наличие обязательных разделов документа.
func checkRequiredSections(text string) []ValidationIssue {
	var issues []ValidationIssue

	requiredSections := []string{
		"заголовок", "заглавие", "наименование",
	}

	hasSection := false
	for _, section := range requiredSections {
		if strings.Contains(strings.ToLower(text), section) {
			hasSection = true
			break
		}
	}
	if !hasSection {
		// Проверяем, есть ли хоть какой-то заголовок (первая строка непустая).
		lines := strings.SplitN(text, "\n", 5)
		if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
			issues = append(issues, ValidationIssue{
				Type:     "error",
				Message:  "Отсутствует заголовок документа",
				Field:    "title",
				Severity: 8,
			})
		}
	}

	// Проверка даты.
	if !datePattern.MatchString(text) {
		issues = append(issues, ValidationIssue{
			Type:     "warning",
			Message:  "Не найдена дата документа",
			Field:    "date",
			Severity: 5,
		})
	}

	// Проверка подписи/автора.
	signatureKeywords := []string{"подпись", "утверждено", "согласовано", "подписано", "должность"}
	hasSignature := false
	for _, kw := range signatureKeywords {
		if strings.Contains(strings.ToLower(text), kw) {
			hasSignature = true
			break
		}
	}
	if !hasSignature {
		issues = append(issues, ValidationIssue{
			Type:     "warning",
			Message:  "Не найдены признаки подписи или утверждения документа",
			Field:    "signature",
			Severity: 4,
		})
	}

	return issues
}

// datePattern — паттерн для поиска дат в тексте (ДД.ММ.ГГГГ, ДД/ММ/ГГГГ, ГГГГ-ММ-ДД).
var datePattern = regexp.MustCompile(`\d{2}[./-]\d{2}[./-]\d{4}|\d{4}[./-]\d{2}[./-]\d{2}`)

// innPattern — паттерн для поиска ИНН (10 или 12 цифр после слова ИНН).
var innPattern = regexp.MustCompile(`ИНН\s*[:№]?\s*\d{10,12}`)

// ogrnPattern — паттерн для поиска ОГРН (13 или 15 цифр после слова ОГРН).
var ogrnPattern = regexp.MustCompile(`ОГРН\s*[:№]?\s*\d{13,15}`)

// checkRegulatoryReferences проверяет ссылки на нормативные документы через RAG.
func (a *ValidatorAgent) checkRegulatoryReferences(ctx context.Context, text string) ([]ValidationIssue, error) {
	var issues []ValidationIssue

	if a.ragService == nil {
		return issues, nil
	}

	// Ищем паттерны ссылок на нормативные документы.
	refPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?:ФЗ|Федеральный закон)\s*["№]?\s*[\d]+\b`),
		regexp.MustCompile(`(?:Постановление|Приказ|Распоряжение)\s+[№]?\s*\S+`),
		regexp.MustCompile(`[Сс]колково`),
	}

	foundRefs := 0
	for _, pat := range refPatterns {
		matches := pat.FindAllString(text, -1)
		for _, m := range matches {
			foundRefs++
			// Проверяем ссылку через RAG-поиск.
			results, err := a.ragService.Search(ctx, m, 1)
			if err != nil {
				continue
			}
			if len(results) == 0 {
				issues = append(issues, ValidationIssue{
					Type:     "warning",
					Message:  fmt.Sprintf("Ссылка на нормативный документ не найдена в базе: %s", m),
					Field:    "references",
					Severity: 3,
				})
			}
		}
	}

	if foundRefs == 0 {
		issues = append(issues, ValidationIssue{
			Type:     "info",
			Message:  "Ссылки на нормативные документы не обнаружены",
			Field:    "references",
			Severity: 0,
		})
	}

	return issues, nil
}

// checkCompleteness проверяет полноту документа по минимальной длине.
func checkCompleteness(text string) []ValidationIssue {
	var issues []ValidationIssue

	words := strings.Fields(text)
	wordCount := len(words)

	if wordCount < 50 {
		issues = append(issues, ValidationIssue{
			Type:     "error",
			Message:  fmt.Sprintf("Документ слишком короткий (%d слов), минимум 50 слов", wordCount),
			Field:    "content",
			Severity: 7,
		})
	} else if wordCount < 100 {
		issues = append(issues, ValidationIssue{
			Type:     "warning",
			Message:  fmt.Sprintf("Документ короткий (%d слов), рекомендуется не менее 100 слов", wordCount),
			Field:    "content",
			Severity: 4,
		})
	}

	return issues
}

// checkFormatPatterns проверяет формат данных в документе.
func checkFormatPatterns(text string) []ValidationIssue {
	var issues []ValidationIssue

	// Проверка ИНН: если упоминается, должен быть корректного формата.
	if innPattern.MatchString(text) {
		// ИНН найден, проверяем валидность (10 или 12 цифр для юрлиц/ИП).
		// Для MVP — просто фиксируем наличие.
	}

	// Проверка ОГРН: если упоминается, должен быть 13 или 15 цифр.
	if ogrnPattern.MatchString(text) {
		// ОГРН найден, для MVP — просто фиксируем наличие.
	}

	// Проверка корректности дат.
	dateMatches := datePattern.FindAllString(text, -1)
	for _, dateStr := range dateMatches {
		// Пробуем распарсить дату.
		dateStr = strings.ReplaceAll(dateStr, "/", "-")
		_, err := time.Parse("02-01-2006", dateStr)
		if err != nil {
			_, err = time.Parse("2006-01-02", dateStr)
			if err != nil {
				issues = append(issues, ValidationIssue{
					Type:     "warning",
					Message:  fmt.Sprintf("Возможно некорректный формат даты: %s", dateStr),
					Field:    "date_format",
					Severity: 3,
				})
			}
		}
	}

	return issues
}

// checkProcedureChecklist проверяет документ против чек-листа процедуры.
func (a *ValidatorAgent) checkProcedureChecklist(ctx context.Context, procedureType, clientID, text string) ([]ValidationIssue, error) {
	var issues []ValidationIssue

	if a.checklistStore == nil {
		return issues, nil
	}

	var ct ChecklistType
	switch procedureType {
	case "entry":
		ct = ChecklistEntry
	case "reporting":
		ct = ChecklistReporting
	case "extension":
		ct = ChecklistExtension
	case "exit":
		ct = ChecklistExit
	default:
		return issues, fmt.Errorf("неизвестный тип процедуры: %s", procedureType)
	}

	checklists, err := a.checklistStore.ListChecklists(ctx, ct)
	if err != nil {
		return nil, fmt.Errorf("получение чек-листов: %w", err)
	}

	if len(checklists) == 0 {
		issues = append(issues, ValidationIssue{
			Type:     "info",
			Message:  fmt.Sprintf("Чек-лист для процедуры %s не найден", procedureType),
			Field:    "checklist",
			Severity: 0,
		})
		return issues, nil
	}

	// Берём первый чек-лист (в MVP — один чек-лист на тип процедуры).
	cl := checklists[0]
	steps, err := cl.ParseSteps()
	if err != nil {
		return nil, fmt.Errorf("парсинг шагов чек-листа: %w", err)
	}

	for _, step := range steps {
		// Для каждого шага с requiredDoc проверяем, упоминается ли документ.
		for _, reqDoc := range step.RequiredDoc {
			if !strings.Contains(strings.ToLower(text), strings.ToLower(reqDoc)) {
				issues = append(issues, ValidationIssue{
					Type:     "warning",
					Message:  fmt.Sprintf("Не найден упоминание требуемого документа: %s (шаг: %s)", reqDoc, step.Title),
					Field:    "required_documents",
					Severity: 5,
				})
			}
		}
	}

	return issues, nil
}

// calculateScore вычисляет итоговую оценку от 0 до 100.
func calculateScore(issues []ValidationIssue) int {
	score := 100

	for _, issue := range issues {
		switch issue.Type {
		case "error":
			score -= issue.Severity * 5
		case "warning":
			score -= issue.Severity * 2
		case "info":
			// Info не снижает оценку.
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

// hasErrors проверяет, есть ли в списке проблемы типа "error".
func hasErrors(issues []ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Type == "error" {
			return false
		}
	}
	return true
}
