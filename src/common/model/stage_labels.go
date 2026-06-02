// stage_labels.go — единый источник истины русских названий стадий резидентства.
// Раньше эта карта дублировалась в admin/consultant.go, admin/residency_admin.go,
// portal/server.go. Теперь все интерфейсы зовут model.StageLabel / model.StageProgress.
package model

// stageLabels — канонические русские названия стадий резидентства.
var stageLabels = map[ResidencyStage]string{
	StageApplication: "Подача заявки",
	StageExamination: "Экспертиза",
	StageDecision:    "Решение",
	StageContract:    "Договор",
	StageResident:    "Резидент",
	StageReporting:   "Отчётность",
	StageExtension:   "Продление",
	StageExit:        "Выход",
}

// StageOrder — стадии в порядке прохождения (для прогресса).
var StageOrder = []ResidencyStage{
	StageApplication, StageExamination, StageDecision, StageContract,
	StageResident, StageReporting, StageExtension, StageExit,
}

// StageLabel возвращает читаемое русское название стадии
// (или саму строку стадии, если она неизвестна).
func StageLabel(s ResidencyStage) string {
	if l, ok := stageLabels[s]; ok {
		return l
	}
	return string(s)
}

// StageLabels возвращает копию карты «стадия → русское название» (для шаблонов,
// которым нужна вся карта; копия — чтобы вызывающий не мутировал источник истины).
func StageLabels() map[ResidencyStage]string {
	out := make(map[ResidencyStage]string, len(stageLabels))
	for k, v := range stageLabels {
		out[k] = v
	}
	return out
}

// StageProgress возвращает процент прохождения по стадиям (позиция стадии 1..N → %).
func StageProgress(s ResidencyStage) int {
	for i, st := range StageOrder {
		if st == s {
			return int(float64(i+1) / float64(len(StageOrder)) * 100)
		}
	}
	return 0
}
