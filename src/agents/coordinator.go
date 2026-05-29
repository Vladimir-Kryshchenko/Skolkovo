package agents

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// StepRecommendation — рекомендация следующего шага для клиента.
type StepRecommendation struct {
	// Title — название шага.
	Title string `json:"title"`
	// Description — описание, что нужно сделать.
	Description string `json:"description"`
	// Deadline — дедлайн шага (может быть пустым).
	Deadline *time.Time `json:"deadline,omitempty"`
	// Priority — приоритет: "high", "medium", "low".
	Priority string `json:"priority"`
	// TemplateURL — URL шаблона документа (если применимо).
	TemplateURL string `json:"template_url,omitempty"`
	// StepIndex — индекс шага в чек-листе.
	StepIndex int `json:"step_index"`
	// ChecklistID — идентификатор чек-листа.
	ChecklistID string `json:"checklist_id"`
}

// ClientProgress — прогресс клиента по процедуре.
type ClientProgress struct {
	// ClientID — идентификатор клиента.
	ClientID string `json:"client_id"`
	// Stage — текущая стадия резидентства.
	Stage string `json:"stage"`
	// ChecklistProgress — прогресс по каждому чек-листу (0-100%).
	ChecklistProgress []ChecklistProgressItem `json:"checklist_progress"`
	// OverallProgress — общий прогресс (0-100%).
	OverallProgress float64 `json:"overall_progress"`
	// Deadlines — ближайшие дедлайны.
	Deadlines []DeadlineInfo `json:"deadlines"`
	// Recommendations — рекомендации следующих шагов.
	Recommendations []StepRecommendation `json:"recommendations"`
	// LastActivity — дата последней активности.
	LastActivity *time.Time `json:"last_activity,omitempty"`
}

// ChecklistProgressItem — прогресс по одному чек-листу.
type ChecklistProgressItem struct {
	// ChecklistID — идентификатор чек-листа.
	ChecklistID string `json:"checklist_id"`
	// Title — название чек-листа.
	Title string `json:"title"`
	// Progress — процент выполнения (0-100).
	Progress float64 `json:"progress"`
	// CompletedSteps — количество выполненных шагов.
	CompletedSteps int `json:"completed_steps"`
	// TotalSteps — общее количество шагов.
	TotalSteps int `json:"total_steps"`
}

// DeadlineInfo — информация о дедлайне.
type DeadlineInfo struct {
	// Title — название дедлайна.
	Title string `json:"title"`
	// DueDate — дата дедлайна.
	DueDate time.Time `json:"due_date"`
	// Type — тип дедлайна.
	Type string `json:"type"`
	// IsOverdue — просрочен ли дедлайн.
	IsOverdue bool `json:"is_overdue"`
	// DaysLeft — дней осталось (отрицательное если просрочен).
	DaysLeft int `json:"days_left"`
}

// EscalationAlert — сигнал эскалации (клиент застрял или просрочил дедлайн).
type EscalationAlert struct {
	// AlertType — тип алерта: "stuck", "deadline_overdue", "no_activity".
	AlertType string `json:"alert_type"`
	// Message — описание ситуации.
	Message string `json:"message"`
	// ClientID — идентификатор клиента.
	ClientID string `json:"client_id"`
	// DaysStuck — дней без прогресса.
	DaysStuck int `json:"days_stuck"`
	// Severity — серьёзность: "critical", "warning".
	Severity string `json:"severity"`
	// RecommendedAction — рекомендуемое действие.
	RecommendedAction string `json:"recommended_action"`
}

// CoordinatorStores — набор хранилищ для агента-координатора.
type CoordinatorStores struct {
	ClientStore    store.ClientStore
	ChecklistStore store.ChecklistStore
	DeadlineStore  store.DeadlineStore
	TemplateStore  store.TemplateStore
}

// CoordinatorAgent — агент-координатор, рекомендующий шаги клиенту.
type CoordinatorAgent struct {
	clientStore    store.ClientStore
	checklistStore store.ChecklistStore
	deadlineStore  store.DeadlineStore
	templateStore  store.TemplateStore
}

// NewCoordinatorAgent создаёт агента-координатора.
func NewCoordinatorAgent(stores CoordinatorStores) *CoordinatorAgent {
	return &CoordinatorAgent{
		clientStore:    stores.ClientStore,
		checklistStore: stores.ChecklistStore,
		deadlineStore:  stores.DeadlineStore,
		templateStore:  stores.TemplateStore,
	}
}

// GetNextSteps возвращает рекомендации следующих шагов для клиента.
//
// Логика:
//  1. Определяем текущую стадию клиента.
//  2. Получаем чек-лист для этой стадии.
//  3. Находим первый невыполненный шаг.
//  4. Учитываем дедлайны.
func (a *CoordinatorAgent) GetNextSteps(ctx context.Context, clientID string) ([]StepRecommendation, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("clientID не указан")
	}

	// 1. Получаем клиента.
	client, err := a.clientStore.GetClient(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("получение клиента: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("клиент %s не найден", clientID)
	}

	// 2. Определяем тип чек-листа по стадии.
	checklistType := stageToChecklistType(client.ResidencyStage)
	if checklistType == "" {
		return nil, fmt.Errorf("нет чек-листа для стадии %s", client.ResidencyStage)
	}

	// 3. Получаем чек-листы этого типа.
	checklists, err := a.checklistStore.ListChecklists(ctx, checklistType)
	if err != nil {
		return nil, fmt.Errorf("получение чек-листов: %w", err)
	}

	if len(checklists) == 0 {
		return nil, fmt.Errorf("чек-лист для типа %s не найден", checklistType)
	}

	// 4. Получаем привязки чек-листов клиента.
	clientChecklists, err := a.checklistStore.GetClientChecklists(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("получение чек-листов клиента: %w", err)
	}

	// 5. Для каждого чек-листа находим следующий шаг.
	var recommendations []StepRecommendation

	for _, cl := range checklists {
		steps, err := cl.ParseSteps()
		if err != nil {
			continue
		}

		// Находим привязку этого чек-листа к клиенту.
		var clientCL *model.ClientChecklist
		for _, ccl := range clientChecklists {
			if ccl.ChecklistID == cl.ID {
				clientCL = ccl
				break
			}
		}

		// Получаем статусы шагов.
		var stepStatuses []*model.ChecklistStepStatus
		if clientCL != nil {
			stepStatuses, err = a.checklistStore.GetStepStatuses(ctx, clientCL.ID)
			if err != nil {
				stepStatuses = nil
			}
		}

		// Находим первый невыполненный шаг.
		nextStep := findNextStep(steps, stepStatuses)
		if nextStep == nil {
			continue
		}

		// Определяем приоритет по дедлайну.
		priority := determinePriority(nextStep, clientID, a.deadlineStore, ctx)

		rec := StepRecommendation{
			Title:       nextStep.Title,
			Description: nextStep.Description,
			Priority:    priority,
			StepIndex:   nextStep.Order,
			ChecklistID: cl.ID,
		}

		if nextStep.DeadlineDays > 0 {
			deadline := time.Now().AddDate(0, 0, nextStep.DeadlineDays)
			rec.Deadline = &deadline
		}

		recommendations = append(recommendations, rec)
	}

	// Сортируем по приоритету.
	sort.Slice(recommendations, func(i, j int) bool {
		return priorityOrder(recommendations[i].Priority) < priorityOrder(recommendations[j].Priority)
	})

	return recommendations, nil
}

// GetProgress возвращает полный прогресс клиента.
func (a *CoordinatorAgent) GetProgress(ctx context.Context, clientID string) (ClientProgress, error) {
	if strings.TrimSpace(clientID) == "" {
		return ClientProgress{}, fmt.Errorf("clientID не указан")
	}

	// Получаем клиента.
	client, err := a.clientStore.GetClient(ctx, clientID)
	if err != nil {
		return ClientProgress{}, fmt.Errorf("получение клиента: %w", err)
	}
	if client == nil {
		return ClientProgress{}, fmt.Errorf("клиент %s не найден", clientID)
	}

	// Получаем чек-листы клиента.
	clientChecklists, err := a.checklistStore.GetClientChecklists(ctx, clientID)
	if err != nil {
		return ClientProgress{}, fmt.Errorf("получение чек-листов клиента: %w", err)
	}

	// Прогресс по каждому чек-листу.
	var checklistProgress []ChecklistProgressItem
	totalSteps := 0
	completedSteps := 0

	for _, ccl := range clientChecklists {
		cl, err := a.checklistStore.GetChecklist(ctx, ccl.ChecklistID)
		if err != nil {
			continue
		}

		steps, err := cl.ParseSteps()
		if err != nil {
			continue
		}

		stepStatuses, err := a.checklistStore.GetStepStatuses(ctx, ccl.ID)
		if err != nil {
			stepStatuses = nil
		}

		sCompleted := countDone(stepStatuses)
		sTotal := len(steps)
		progress := 0.0
		if sTotal > 0 {
			progress = math.Round(float64(sCompleted) / float64(sTotal) * 100)
		}

		checklistProgress = append(checklistProgress, ChecklistProgressItem{
			ChecklistID:    cl.ID,
			Title:          cl.Title,
			Progress:       progress,
			CompletedSteps: sCompleted,
			TotalSteps:     sTotal,
		})

		totalSteps += sTotal
		completedSteps += sCompleted
	}

	// Общий прогресс.
	overallProgress := 0.0
	if totalSteps > 0 {
		overallProgress = math.Round(float64(completedSteps) / float64(totalSteps) * 100)
	}

	// Дедлайны.
	deadlines := a.getDeadlines(ctx, clientID)

	// Рекомендации.
	recommendations, _ := a.GetNextSteps(ctx, clientID)

	// Последняя активность — дата последнего перехода стадии.
	lastActivity := a.getLastActivity(ctx, clientID)

	return ClientProgress{
		ClientID:          clientID,
		Stage:             string(client.ResidencyStage),
		ChecklistProgress: checklistProgress,
		OverallProgress:   overallProgress,
		Deadlines:         deadlines,
		Recommendations:   recommendations,
		LastActivity:      lastActivity,
	}, nil
}

// GenerateEscalation генерирует сигнал эскалации, если клиент застрял или просрочил дедлайн.
func (a *CoordinatorAgent) GenerateEscalation(ctx context.Context, clientID string) (*EscalationAlert, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("clientID не указан")
	}

	// Проверяем просроченные дедлайны.
	if a.deadlineStore != nil {
		overdue, err := a.deadlineStore.ListOverdueDeadlines(ctx)
		if err == nil {
			for _, d := range overdue {
				if d.ClientID == clientID {
					daysOverdue := int(time.Since(d.DueDate).Hours() / 24)
					return &EscalationAlert{
						AlertType:         "deadline_overdue",
						Message:           fmt.Sprintf("Просрочен дедлайн: %s (просрочен на %d дней)", d.Title, daysOverdue),
						ClientID:          clientID,
						DaysStuck:         daysOverdue,
						Severity:          severityForDays(daysOverdue),
						RecommendedAction: fmt.Sprintf("Свяжитесь с клиентом по поводу просроченного дедлайна «%s»", d.Title),
					}, nil
				}
			}
		}
	}

	// Проверяем, застрял ли клиент (нет прогресса > N дней).
	daysStuck := a.calculateDaysStuck(ctx, clientID)
	const stuckThreshold = 14 // дней без прогресса

	if daysStuck >= stuckThreshold {
		return &EscalationAlert{
			AlertType:         "stuck",
			Message:           fmt.Sprintf("Клиент не двигается %d дней (порог: %d дней)", daysStuck, stuckThreshold),
			ClientID:          clientID,
			DaysStuck:         daysStuck,
			Severity:          severityForDays(daysStuck),
			RecommendedAction: "Проверьте, есть ли блокирующие проблемы, и свяжитесь с клиентом",
		}, nil
	}

	// Нет причин для эскалации.
	return nil, nil
}

// stageToChecklistType преобразует стадию резидентства в тип чек-листа.
func stageToChecklistType(stage model.ResidencyStage) model.ChecklistType {
	switch stage {
	case model.StageApplication, model.StageExamination, model.StageDecision, model.StageContract, model.StageResident:
		return model.ChecklistEntry
	case model.StageReporting:
		return model.ChecklistReporting
	case model.StageExtension:
		return model.ChecklistExtension
	case model.StageExit:
		return model.ChecklistExit
	default:
		return ""
	}
}

// findNextStep находит первый невыполненный шаг.
func findNextStep(steps []model.ChecklistStepDef, statuses []*model.ChecklistStepStatus) *model.ChecklistStepDef {
	doneMap := make(map[int]bool)
	for _, s := range statuses {
		if s.Status == model.StepDone || s.Status == model.StepSkipped {
			doneMap[s.StepIndex] = true
		}
	}

	for i, step := range steps {
		if !doneMap[i] {
			return &step
		}
	}

	return nil
}

// determinePriority определяет приоритет шага.
func determinePriority(step *model.ChecklistStepDef, clientID string, deadlineStore store.DeadlineStore, ctx context.Context) string {
	if deadlineStore == nil {
		return "medium"
	}

	deadlines, err := deadlineStore.ListDeadlines(ctx, clientID, 0)
	if err != nil || len(deadlines) == 0 {
		return "medium"
	}

	now := time.Now()
	for _, d := range deadlines {
		if d.IsOverdue(now) {
			return "high"
		}
		daysLeft := int(d.DueDate.Sub(now).Hours() / 24)
		if daysLeft <= 3 {
			return "high"
		}
		if daysLeft <= 7 {
			return "medium"
		}
	}

	return "low"
}

// priorityOrder возвращает числовой порядок приоритета (меньше = важнее).
func priorityOrder(p string) int {
	switch p {
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 2
	}
}

// countDone считает количество шагов со статусом "done".
func countDone(statuses []*model.ChecklistStepStatus) int {
	count := 0
	for _, s := range statuses {
		if s.Status == model.StepDone {
			count++
		}
	}
	return count
}

// getDeadlines получает ближайшие дедлайны клиента.
func (a *CoordinatorAgent) getDeadlines(ctx context.Context, clientID string) []DeadlineInfo {
	if a.deadlineStore == nil {
		return nil
	}

	deadlines, err := a.deadlineStore.ListDeadlines(ctx, clientID, 30)
	if err != nil {
		return nil
	}

	now := time.Now()
	var result []DeadlineInfo
	for _, d := range deadlines {
		daysLeft := int(d.DueDate.Sub(now).Hours() / 24)
		result = append(result, DeadlineInfo{
			Title:     d.Title,
			DueDate:   d.DueDate,
			Type:      string(d.Type),
			IsOverdue: d.IsOverdue(now),
			DaysLeft:  daysLeft,
		})
	}

	return result
}

// getLastActivity возвращает дату последней активности клиента (последний переход стадии).
func (a *CoordinatorAgent) getLastActivity(ctx context.Context, clientID string) *time.Time {
	if a.clientStore == nil {
		return nil
	}

	transitions, err := a.clientStore.GetStageHistory(ctx, clientID)
	if err != nil || len(transitions) == 0 {
		return nil
	}

	// Возвращаем дату последнего перехода.
	last := transitions[len(transitions)-1]
	return &last.TransitionedAt
}

// calculateDaysStuck считает, сколько дней клиент без прогресса.
func (a *CoordinatorAgent) calculateDaysStuck(ctx context.Context, clientID string) int {
	lastActivity := a.getLastActivity(ctx, clientID)
	if lastActivity == nil {
		// Если нет истории переходов, считаем от даты создания клиента.
		client, err := a.clientStore.GetClient(ctx, clientID)
		if err != nil || client == nil {
			return 0
		}
		return int(time.Since(client.CreatedAt).Hours() / 24)
	}

	return int(time.Since(*lastActivity).Hours() / 24)
}

// severityForDays возвращает серьёзность алерта в зависимости от количества дней.
func severityForDays(days int) string {
	if days >= 30 {
		return "critical"
	}
	return "warning"
}
