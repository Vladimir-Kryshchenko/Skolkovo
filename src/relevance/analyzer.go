package relevance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/diff"
)

// VersionReader отдаёт последние сохранённые версии документа (реализуется
// store.PostgresVersionStore).
type VersionReader interface {
	LatestVersions(ctx context.Context, documentID string, n int) ([]*model.DocVersion, error)
}

// ModelStore отдаёт конфигурацию агентов и моделей (реализуется *aimodels.Store).
// Может быть nil — тогда анализатор работает только на эвристике.
type ModelStore interface {
	ListAgents(ctx context.Context) ([]aimodels.Agent, error)
	GetModel(ctx context.Context, id string) (aimodels.Model, error)
}

// ChatFunc — вызов LLM с системным промптом агента (по умолчанию
// aimodels.ChatWithAgent; переопределяется в тестах).
type ChatFunc func(ctx context.Context, m aimodels.Model, a aimodels.Agent, userMessage string) (string, int, error)

// Result — итог анализа изменения документа.
type Result struct {
	Severity    changes.Severity
	Summary     string
	Stages      []model.ResidencyStage
	DiffAdded   int
	DiffRemoved int
	UsedLLM     bool
}

// ToEnrichment конвертирует результат в запись обогащения ленты изменений.
func (r Result) ToEnrichment() changes.Enrichment {
	stages := make([]string, 0, len(r.Stages))
	for _, s := range r.Stages {
		stages = append(stages, string(s))
	}
	return changes.Enrichment{
		Severity:        r.Severity,
		AnalysisSummary: r.Summary,
		AffectedStages:  stages,
		DiffAdded:       r.DiffAdded,
		DiffRemoved:     r.DiffRemoved,
	}
}

// Analyzer классифицирует важность изменений документов.
type Analyzer struct {
	Versions VersionReader // обязателен
	AI       ModelStore    // опционален: nil → только эвристика
	Chat     ChatFunc      // опционален: nil → aimodels.ChatWithAgent
}

// NewAnalyzer создаёт анализатор. ai может быть nil (тогда только эвристика).
func NewAnalyzer(versions VersionReader, ai ModelStore) *Analyzer {
	return &Analyzer{Versions: versions, AI: ai, Chat: aimodels.ChatWithAgent}
}

// Analyze оценивает важность изменения документа ev и определяет затронутые стадии.
func (a *Analyzer) Analyze(ctx context.Context, ev changes.Event) (Result, error) {
	base := StagesForCategory(ev.Category)

	versions, err := a.Versions.LatestVersions(ctx, ev.EntityID, 2)
	if err != nil {
		return Result{}, err
	}

	// Недостаточно версий для диффа — оцениваем по виду события.
	if len(versions) < 2 {
		sev := changes.SeverityInfo
		summary := "Документ заведён в базу знаний."
		if ev.Kind == changes.KindOutdated {
			sev, summary = changes.SeverityWarning, "Документ переведён в статус «устарел»."
		}
		return Result{Severity: sev, Summary: summary, Stages: base}, nil
	}

	newV, oldV := versions[0], versions[1]
	d := diff.CompareDocuments(oldV.ExtractedText, newV.ExtractedText)
	added, removed := d.Summary.TotalAdded, d.Summary.TotalRemoved

	// Пытаемся классифицировать через LLM; при любой неудаче — эвристика.
	if sev, summary, llmStages, ok := a.classifyLLM(ctx, ev, d); ok {
		return Result{
			Severity: sev, Summary: summary,
			Stages:    MergeStages(base, llmStages),
			DiffAdded: added, DiffRemoved: removed, UsedLLM: true,
		}, nil
	}

	sev, summary := classifyHeuristic(oldV.ExtractedText, newV.ExtractedText, d)
	return Result{
		Severity: sev, Summary: summary, Stages: base,
		DiffAdded: added, DiffRemoved: removed,
	}, nil
}

// classifyLLM запрашивает у LLM (агент типа «Монитор») оценку важности и суть
// изменений. Возвращает ok=false, если LLM недоступен или ответ не распарсился.
func (a *Analyzer) classifyLLM(ctx context.Context, ev changes.Event, d diff.DocumentDiff) (changes.Severity, string, []string, bool) {
	if a.AI == nil {
		return "", "", nil, false
	}
	mdl, agent, ok := a.monitorAgent(ctx)
	if !ok {
		return "", "", nil, false
	}
	chat := a.Chat
	if chat == nil {
		chat = aimodels.ChatWithAgent
	}

	prompt := buildPrompt(ev, d)
	raw, _, err := chat(ctx, mdl, agent, prompt)
	if err != nil {
		return "", "", nil, false
	}
	sev, summary, stages, ok := parseLLMResult(raw)
	if !ok {
		return "", "", nil, false
	}
	return sev, summary, stages, true
}

// monitorAgent находит включённого агента-монитора и его включённую модель.
func (a *Analyzer) monitorAgent(ctx context.Context) (aimodels.Model, aimodels.Agent, bool) {
	agents, err := a.AI.ListAgents(ctx)
	if err != nil {
		return aimodels.Model{}, aimodels.Agent{}, false
	}
	for _, ag := range agents {
		if ag.AgentType != aimodels.AgentMonitor || !ag.Enabled || ag.ModelID == "" {
			continue
		}
		mdl, err := a.AI.GetModel(ctx, ag.ModelID)
		if err != nil || !mdl.Enabled || strings.TrimSpace(mdl.APIKey) == "" {
			continue
		}
		return mdl, ag, true
	}
	return aimodels.Model{}, aimodels.Agent{}, false
}

// buildPrompt формирует запрос к LLM: метаданные + статистика диффа + образец
// изменённых строк, с требованием вернуть строгий JSON.
func buildPrompt(ev changes.Event, d diff.DocumentDiff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Документ: %s\n", ev.Title)
	if ev.Category != "" {
		fmt.Fprintf(&b, "Категория: %s\n", ev.Category)
	}
	fmt.Fprintf(&b, "Статистика изменений: добавлено строк %d, удалено %d.\n\n",
		d.Summary.TotalAdded, d.Summary.TotalRemoved)

	b.WriteString("Образец изменений (+ добавлено, − удалено):\n")
	writeSample(&b, d, 30)

	b.WriteString("\nОцени важность изменения для резидентов Сколково и кратко опиши суть.\n")
	b.WriteString("Возможные стадии резидентства: подача_заявки, экспертиза, решение, договор, резидент, отчётность, продление, выход.\n")
	b.WriteString(`Ответь СТРОГО одним JSON-объектом без пояснений вида:
{"severity":"info|warning|critical","summary":"1-2 предложения что изменилось","affected_stages":["отчётность"]}`)
	return b.String()
}

// writeSample выводит до limit изменённых строк из диффа.
func writeSample(b *strings.Builder, d diff.DocumentDiff, limit int) {
	n := 0
	emit := func(prefix, text string) bool {
		text = strings.TrimSpace(text)
		if text == "" {
			return true
		}
		fmt.Fprintf(b, "%s %s\n", prefix, truncateLine(text, 200))
		n++
		return n < limit
	}
	for _, c := range d.AddedLines {
		if !emit("+", c.NewText) {
			return
		}
	}
	for _, c := range d.RemovedLines {
		if !emit("−", c.OldText) {
			return
		}
	}
	for _, sec := range d.ModifiedSections {
		for _, c := range sec.Changes {
			txt := c.NewText
			pref := "+"
			if c.Type == diff.ChangeRemoved {
				txt, pref = c.OldText, "−"
			}
			if !emit(pref, txt) {
				return
			}
		}
	}
}

// parseLLMResult извлекает JSON-объект из ответа LLM и валидирует severity.
func parseLLMResult(raw string) (changes.Severity, string, []string, bool) {
	js := extractJSON(raw)
	if js == "" {
		return "", "", nil, false
	}
	var parsed struct {
		Severity       string   `json:"severity"`
		Summary        string   `json:"summary"`
		AffectedStages []string `json:"affected_stages"`
	}
	if err := json.Unmarshal([]byte(js), &parsed); err != nil {
		return "", "", nil, false
	}
	sev := normalizeSeverity(parsed.Severity)
	if sev == "" || strings.TrimSpace(parsed.Summary) == "" {
		return "", "", nil, false
	}
	return sev, strings.TrimSpace(parsed.Summary), parsed.AffectedStages, true
}

// extractJSON возвращает подстроку от первой '{' до последней '}'.
func extractJSON(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return s[i : j+1]
}

func normalizeSeverity(s string) changes.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "критично", "критический", "критическая":
		return changes.SeverityCritical
	case "warning", "важно", "важное", "предупреждение":
		return changes.SeverityWarning
	case "info", "информационно", "информация", "информационное":
		return changes.SeverityInfo
	default:
		return ""
	}
}

func truncateLine(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
