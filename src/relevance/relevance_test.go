package relevance

import (
	"context"
	"testing"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/diff"
)

func diffOf(oldT, newT string) diff.DocumentDiff {
	return diff.CompareDocuments(oldT, newT)
}

func TestStagesForCategory(t *testing.T) {
	cases := map[string]model.ResidencyStage{
		"Отчётность":           model.StageReporting,
		"Законодательные акты": model.StageApplication, // нормативка → все стадии, включая подачу
		"Конкурсы":             model.StageResident,
	}
	for cat, want := range cases {
		got := StagesForCategory(cat)
		if !containsStage(got, want) {
			t.Errorf("StagesForCategory(%q) = %v, ожидалась стадия %q", cat, got, want)
		}
	}
	// Неизвестная категория → активные резиденты (не пусто).
	if got := StagesForCategory("Какая-то новая категория"); len(got) == 0 {
		t.Error("неизвестная категория должна давать непустой набор стадий по умолчанию")
	}
	// Информационная категория → пусто.
	if got := StagesForCategory("Новости"); len(got) != 0 {
		t.Errorf("Новости не должны привязываться к стадиям, получено %v", got)
	}
}

func TestParseStageAndMerge(t *testing.T) {
	if st, ok := ParseStage("отчётность"); !ok || st != model.StageReporting {
		t.Errorf("ParseStage(отчётность) = %v,%v", st, ok)
	}
	if st, ok := ParseStage("reporting"); !ok || st != model.StageReporting {
		t.Errorf("ParseStage(reporting) = %v,%v", st, ok)
	}
	if _, ok := ParseStage("чепуха"); ok {
		t.Error("ParseStage(чепуха) должна вернуть ok=false")
	}

	base := []model.ResidencyStage{model.StageReporting}
	merged := MergeStages(base, []string{"reporting", "договор", "мусор"})
	if !containsStage(merged, model.StageReporting) || !containsStage(merged, model.StageContract) {
		t.Errorf("MergeStages не объединил стадии: %v", merged)
	}
	// Дубль reporting не должен повторяться.
	cnt := 0
	for _, s := range merged {
		if s == model.StageReporting {
			cnt++
		}
	}
	if cnt != 1 {
		t.Errorf("MergeStages продублировал стадию reporting: %v", merged)
	}
}

func TestClassifyHeuristic(t *testing.T) {
	// Критический маркер → critical.
	oldT := "Документ действует.\nСрок подачи отчёта 30 дней."
	newT := "Документ действует.\nДокумент утратил силу с 1 января."
	d := diffOf(oldT, newT)
	sev, summary := classifyHeuristic(oldT, newT, d)
	if sev != changes.SeverityCritical {
		t.Errorf("ожидался critical при маркере «утратил силу», получено %q (%s)", sev, summary)
	}

	// Мелкая правка одной строки в большом тексте → info.
	big := "строка\n" + repeat("строка\n", 50) + "ещё\n"
	bigNew := big + "одна новая строка\n"
	d2 := diffOf(big, bigNew)
	sev2, _ := classifyHeuristic(big, bigNew, d2)
	if sev2 != changes.SeverityInfo {
		t.Errorf("ожидался info при мелкой правке, получено %q", sev2)
	}
}

// --- стабы для Analyze ---

type stubVersions struct {
	versions []*model.DocVersion
}

func (s stubVersions) LatestVersions(_ context.Context, _ string, _ int) ([]*model.DocVersion, error) {
	return s.versions, nil
}

type stubAI struct{}

func (stubAI) ListAgents(_ context.Context) ([]aimodels.Agent, error) {
	return []aimodels.Agent{{AgentType: aimodels.AgentMonitor, Enabled: true, ModelID: "m1"}}, nil
}
func (stubAI) GetModel(_ context.Context, _ string) (aimodels.Model, error) {
	return aimodels.Model{ID: "m1", Enabled: true, APIKey: "key", ModelID: "test"}, nil
}

func TestAnalyzeWithStubLLM(t *testing.T) {
	versions := []*model.DocVersion{
		{DocumentID: "d1", VersionNo: 2, ExtractedText: "новый текст\nизменённый пункт"},
		{DocumentID: "d1", VersionNo: 1, ExtractedText: "старый текст\nстарый пункт"},
	}
	a := &Analyzer{
		Versions: stubVersions{versions: versions},
		AI:       stubAI{},
		Chat: func(_ context.Context, _ aimodels.Model, _ aimodels.Agent, _ string) (string, int, error) {
			return `тут пояснение {"severity":"critical","summary":"Изменены требования","affected_stages":["отчётность"]} конец`, 10, nil
		},
	}
	res, err := a.Analyze(context.Background(), changes.Event{EntityID: "d1", Category: "Требования", Kind: changes.KindUpdated})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !res.UsedLLM {
		t.Error("ожидалось использование LLM")
	}
	if res.Severity != changes.SeverityCritical {
		t.Errorf("severity = %q, ожидался critical", res.Severity)
	}
	if !containsStage(res.Stages, model.StageReporting) {
		t.Errorf("стадии должны включать отчётность из ответа LLM: %v", res.Stages)
	}
}

func TestAnalyzeFallbackWhenNoLLM(t *testing.T) {
	versions := []*model.DocVersion{
		{DocumentID: "d1", VersionNo: 2, ExtractedText: "текст\nдокумент отменён"},
		{DocumentID: "d1", VersionNo: 1, ExtractedText: "текст\nдокумент действует"},
	}
	a := NewAnalyzer(stubVersions{versions: versions}, nil) // AI=nil → эвристика
	res, err := a.Analyze(context.Background(), changes.Event{EntityID: "d1", Category: "Требования", Kind: changes.KindUpdated})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.UsedLLM {
		t.Error("без AI LLM использоваться не должен")
	}
	if res.Severity != changes.SeverityCritical {
		t.Errorf("эвристика должна дать critical по маркеру «отменён», получено %q", res.Severity)
	}
}

func TestAnalyzeSingleVersion(t *testing.T) {
	a := NewAnalyzer(stubVersions{versions: []*model.DocVersion{{DocumentID: "d1", VersionNo: 1}}}, nil)
	res, err := a.Analyze(context.Background(), changes.Event{EntityID: "d1", Kind: changes.KindNew, Category: "Требования"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Severity != changes.SeverityInfo {
		t.Errorf("новый документ без предыдущей версии → info, получено %q", res.Severity)
	}
	if len(res.Stages) == 0 {
		t.Error("стадии должны быть заполнены из категории")
	}
}

// --- helpers ---

func containsStage(list []model.ResidencyStage, want model.ResidencyStage) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
