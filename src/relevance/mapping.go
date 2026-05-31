// Package relevance — трек «Актуальность изменений»: при обновлении документа
// считает структурный дифф, через LLM (с эвристическим фоллбэком) классифицирует
// важность и «что изменилось», определяет затронутые стадии резидентства и
// рассылает уведомления консультанту и клиентам.
package relevance

import (
	"strings"

	"baza-skolkovo/src/common/model"
)

// allStages — все стадии резидентства (нормативные изменения касаются всех).
var allStages = []model.ResidencyStage{
	model.StageApplication, model.StageExamination, model.StageDecision,
	model.StageContract, model.StageResident, model.StageReporting,
	model.StageExtension, model.StageExit,
}

// activeResidentStages — стадии активного резидента (договор → продление):
// именно их касается большинство процедурных и конкурсных изменений.
var activeResidentStages = []model.ResidencyStage{
	model.StageContract, model.StageResident, model.StageReporting, model.StageExtension,
}

// categoryStages сопоставляет категорию документа со стадиями, которых касается
// изменение. Ключи — человекочитаемые названия категорий (см. scraper.CategoryNames
// и classifier.DefaultCategories). Регистр и пробелы нормализуются.
var categoryStages = map[string][]model.ResidencyStage{
	// Нормативка — касается всех стадий.
	"законодательные акты":             allStages,
	"правила проектирования":           allStages,
	"иные нормативные документы":       allStages,
	"антикоррупция":                    allStages,
	"кибербезопасность и перс. данные": allStages,
	"развитие территории":              allStages,
	"закупки и тендеры":                allStages,
	"утратившие силу":                  allStages,
	"требования":                       allStages,
	"npa":                              allStages,
	"regulation":                       allStages,
	// Отчётность.
	"отчётность": {model.StageReporting},
	"отчетность": {model.StageReporting},
	// Процедуры и документы фонда — активные резиденты.
	"процедуры":       activeResidentStages,
	"документы фонда": activeResidentStages,
	"guidance":        activeResidentStages,
	// Конкурсы/гранты — действующие резиденты и отчётность.
	"конкурсы": {model.StageResident, model.StageReporting},
	// Информационные категории — без персональной привязки к стадии.
	"новости":     {},
	"мероприятия": {},
	"faq":         {},
	"телеграм":    {},
}

// StagesForCategory возвращает базовый набор стадий, которых касается изменение
// документа данной категории. Неизвестная категория → активные резиденты (
// безопасное умолчание: предупреждаем тех, кто проходит процедуры).
func StagesForCategory(category string) []model.ResidencyStage {
	key := strings.ToLower(strings.TrimSpace(category))
	if st, ok := categoryStages[key]; ok {
		return st
	}
	return activeResidentStages
}

// stageAliases — соответствие текстовых обозначений стадий от LLM значениям модели.
var stageAliases = map[string]model.ResidencyStage{
	"подача_заявки": model.StageApplication, "подача заявки": model.StageApplication, "application": model.StageApplication,
	"экспертиза": model.StageExamination, "examination": model.StageExamination,
	"решение": model.StageDecision, "decision": model.StageDecision,
	"договор": model.StageContract, "contract": model.StageContract,
	"резидент": model.StageResident, "resident": model.StageResident,
	"отчётность": model.StageReporting, "отчетность": model.StageReporting, "reporting": model.StageReporting,
	"продление": model.StageExtension, "extension": model.StageExtension,
	"выход": model.StageExit, "exit": model.StageExit,
}

// ParseStage преобразует текстовое обозначение стадии (от LLM) в модельное значение.
func ParseStage(s string) (model.ResidencyStage, bool) {
	st, ok := stageAliases[strings.ToLower(strings.TrimSpace(s))]
	return st, ok
}

// MergeStages объединяет базовый набор стадий с распознанными из ответа LLM,
// сохраняя порядок и убирая дубли.
func MergeStages(base []model.ResidencyStage, llm []string) []model.ResidencyStage {
	seen := make(map[model.ResidencyStage]bool)
	var out []model.ResidencyStage
	add := func(st model.ResidencyStage) {
		if st != "" && !seen[st] {
			seen[st] = true
			out = append(out, st)
		}
	}
	for _, st := range base {
		add(st)
	}
	for _, s := range llm {
		if st, ok := ParseStage(s); ok {
			add(st)
		}
	}
	return out
}
