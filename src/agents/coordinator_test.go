package agents

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// --- Mock-хранилища для координатора ---

type mockCoordClientStore struct {
	clients     map[string]*model.Client
	transitions map[string][]*model.StageTransition
	err         error
}

func (m *mockCoordClientStore) CreateClient(_ context.Context, _ *model.Client) error { return m.err }
func (m *mockCoordClientStore) GetClient(_ context.Context, id string) (*model.Client, error) {
	if m.err != nil {
		return nil, m.err
	}
	if c, ok := m.clients[id]; ok {
		return c, nil
	}
	return nil, m.err
}
func (m *mockCoordClientStore) GetClientByINN(_ context.Context, _ string) (*model.Client, error) {
	return nil, m.err
}
func (m *mockCoordClientStore) UpdateClient(_ context.Context, _ *model.Client) error { return m.err }
func (m *mockCoordClientStore) DeleteClient(_ context.Context, _ string) error        { return m.err }
func (m *mockCoordClientStore) ListClients(_ context.Context, _ string, _ model.ResidencyStage) ([]*model.Client, error) {
	var result []*model.Client
	for _, c := range m.clients {
		result = append(result, c)
	}
	return result, m.err
}
func (m *mockCoordClientStore) AddStageTransition(_ context.Context, _ *model.StageTransition) error {
	return m.err
}
func (m *mockCoordClientStore) GetStageHistory(_ context.Context, clientID string) ([]*model.StageTransition, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transitions[clientID], m.err
}

var _ store.ClientStore = (*mockCoordClientStore)(nil)

type mockCoordChecklistStore struct {
	checklists   map[string]*model.Checklist
	clientCLs    map[string][]*model.ClientChecklist
	stepStatuses map[string][]*model.ChecklistStepStatus
	err          error
}

func (m *mockCoordChecklistStore) CreateChecklist(_ context.Context, _ *model.Checklist) error {
	return m.err
}
func (m *mockCoordChecklistStore) GetChecklist(_ context.Context, id string) (*model.Checklist, error) {
	if m.err != nil {
		return nil, m.err
	}
	if cl, ok := m.checklists[id]; ok {
		return cl, nil
	}
	return nil, m.err
}
func (m *mockCoordChecklistStore) ListChecklists(_ context.Context, pt model.ChecklistType) ([]*model.Checklist, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []*model.Checklist
	for _, cl := range m.checklists {
		if pt == "" || cl.ProcedureType == pt {
			result = append(result, cl)
		}
	}
	return result, nil
}
func (m *mockCoordChecklistStore) CreateClientChecklist(_ context.Context, _ *model.ClientChecklist) error {
	return m.err
}
func (m *mockCoordChecklistStore) GetClientChecklist(_ context.Context, _ string) (*model.ClientChecklist, error) {
	return nil, m.err
}
func (m *mockCoordChecklistStore) GetClientChecklists(_ context.Context, clientID string) ([]*model.ClientChecklist, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.clientCLs[clientID], nil
}
func (m *mockCoordChecklistStore) UpdateClientChecklist(_ context.Context, _ *model.ClientChecklist) error {
	return m.err
}
func (m *mockCoordChecklistStore) CreateStepStatus(_ context.Context, _ *model.ChecklistStepStatus) error {
	return m.err
}
func (m *mockCoordChecklistStore) UpdateStepStatus(_ context.Context, _ string, _ model.StepStatus, _ string) error {
	return m.err
}
func (m *mockCoordChecklistStore) GetStepStatuses(_ context.Context, cclID string) ([]*model.ChecklistStepStatus, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stepStatuses[cclID], nil
}

var _ store.ChecklistStore = (*mockCoordChecklistStore)(nil)

type mockCoordDeadlineStore struct {
	deadlines []*model.Deadline
	err       error
}

func (m *mockCoordDeadlineStore) CreateDeadline(_ context.Context, _ *model.Deadline) error {
	return m.err
}
func (m *mockCoordDeadlineStore) GetDeadline(_ context.Context, _ string) (*model.Deadline, error) {
	return nil, m.err
}
func (m *mockCoordDeadlineStore) UpdateDeadline(_ context.Context, _ *model.Deadline) error {
	return m.err
}
func (m *mockCoordDeadlineStore) ListDeadlines(_ context.Context, clientID string, _ int) ([]*model.Deadline, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []*model.Deadline
	for _, d := range m.deadlines {
		if d.ClientID == clientID {
			result = append(result, d)
		}
	}
	return result, nil
}
func (m *mockCoordDeadlineStore) ListOverdueDeadlines(_ context.Context) ([]*model.Deadline, error) {
	if m.err != nil {
		return nil, m.err
	}
	var result []*model.Deadline
	for _, d := range m.deadlines {
		if d.IsOverdue(time.Now()) {
			result = append(result, d)
		}
	}
	return result, nil
}
func (m *mockCoordDeadlineStore) MarkNotificationSent(_ context.Context, _ string) error {
	return m.err
}

var _ store.DeadlineStore = (*mockCoordDeadlineStore)(nil)

type mockCoordTemplateStore struct{}

func (m *mockCoordTemplateStore) CreateTemplate(_ context.Context, _ *model.DocumentTemplate) error {
	return nil
}
func (m *mockCoordTemplateStore) GetTemplate(_ context.Context, _ string) (*model.DocumentTemplate, error) {
	return nil, nil
}
func (m *mockCoordTemplateStore) ListTemplates(_ context.Context, _ string) ([]*model.DocumentTemplate, error) {
	return nil, nil
}

var _ store.TemplateStore = (*mockCoordTemplateStore)(nil)

func TestNewCoordinatorAgent(t *testing.T) {
	stores := CoordinatorStores{
		ClientStore:    &mockCoordClientStore{clients: make(map[string]*model.Client)},
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  &mockCoordDeadlineStore{},
		TemplateStore:  &mockCoordTemplateStore{},
	}
	agent := NewCoordinatorAgent(stores)
	if agent == nil {
		t.Fatal("ожидался не-nil агент")
	}
}

func TestCoordinatorGetNextSteps_EmptyClientID(t *testing.T) {
	stores := CoordinatorStores{
		ClientStore:    &mockCoordClientStore{clients: make(map[string]*model.Client)},
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	_, err := agent.GetNextSteps(context.Background(), "")
	if err == nil {
		t.Fatal("ожидалась ошибка при пустом clientID")
	}
}

func TestCoordinatorGetNextSteps_ClientNotFound(t *testing.T) {
	cs := &mockCoordClientStore{
		clients: make(map[string]*model.Client),
		err:     nil, // не возвращает ошибку, но nil client
	}
	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	_, err := agent.GetNextSteps(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствии клиента")
	}
}

func TestCoordinatorGetNextSteps_NoChecklist(t *testing.T) {
	cs := &mockCoordClientStore{
		clients: map[string]*model.Client{
			"client-1": {
				ID:             "client-1",
				Name:           "Тест",
				INN:            "7707083893",
				TenantID:       "tenant-1",
				ResidencyStage: model.StageResident,
			},
		},
	}

	clStore := &mockCoordChecklistStore{
		checklists: make(map[string]*model.Checklist),
		clientCLs:  make(map[string][]*model.ClientChecklist),
	}

	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: clStore,
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	_, err := agent.GetNextSteps(context.Background(), "client-1")
	if err == nil {
		t.Log("ожидалась ошибка об отсутствии чек-листа")
	}
}

func TestCoordinatorGetNextSteps_WithChecklist(t *testing.T) {
	steps, _ := json.Marshal([]model.ChecklistStepDef{
		{
			ID:           "step-1",
			Title:        "Подготовить заявку",
			Description:  "Собрать документы и подать заявку",
			Order:        0,
			DeadlineDays: 14,
		},
		{
			ID:           "step-2",
			Title:        "Пройти экспертизу",
			Description:  "Предоставить дополнительные документы",
			Order:        1,
			DeadlineDays: 30,
		},
	})

	cs := &mockCoordClientStore{
		clients: map[string]*model.Client{
			"client-1": {
				ID:             "client-1",
				Name:           "Тест",
				INN:            "7707083893",
				TenantID:       "tenant-1",
				ResidencyStage: model.StageApplication,
			},
		},
	}

	clID := "cl-entry"
	clStore := &mockCoordChecklistStore{
		checklists: map[string]*model.Checklist{
			clID: {
				ID:            clID,
				Title:         "Вход",
				ProcedureType: model.ChecklistEntry,
				Steps:         steps,
				Version:       "1.0",
			},
		},
		clientCLs: map[string][]*model.ClientChecklist{
			"client-1": {
				{
					ID:          "ccl-1",
					ClientID:    "client-1",
					ChecklistID: clID,
					Status:      model.ChecklistInProgress,
				},
			},
		},
		stepStatuses: make(map[string][]*model.ChecklistStepStatus),
	}

	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: clStore,
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	steps_result, err := agent.GetNextSteps(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(steps_result) == 0 {
		t.Fatal("ожидался хотя бы один следующий шаг")
	}

	// Первый шаг должен быть "Подготовить заявку".
	if steps_result[0].Title != "Подготовить заявку" {
		t.Errorf("Title = %q, хотел %q", steps_result[0].Title, "Подготовить заявку")
	}
}

func TestCoordinatorGetNextSteps_AllDone(t *testing.T) {
	steps, _ := json.Marshal([]model.ChecklistStepDef{
		{
			ID:    "step-1",
			Title: "Шаг 1",
			Order: 0,
		},
	})

	cs := &mockCoordClientStore{
		clients: map[string]*model.Client{
			"client-1": {
				ID:             "client-1",
				Name:           "Тест",
				INN:            "7707083893",
				TenantID:       "tenant-1",
				ResidencyStage: model.StageApplication,
			},
		},
	}

	clID := "cl-entry"
	clStore := &mockCoordChecklistStore{
		checklists: map[string]*model.Checklist{
			clID: {
				ID:            clID,
				Title:         "Вход",
				ProcedureType: model.ChecklistEntry,
				Steps:         steps,
				Version:       "1.0",
			},
		},
		clientCLs: map[string][]*model.ClientChecklist{
			"client-1": {
				{
					ID:          "ccl-1",
					ClientID:    "client-1",
					ChecklistID: clID,
					Status:      model.ChecklistCompleted,
				},
			},
		},
		stepStatuses: map[string][]*model.ChecklistStepStatus{
			"ccl-1": {
				{
					ID:                "cs-1",
					ClientChecklistID: "ccl-1",
					StepIndex:         0,
					Status:            model.StepDone,
				},
			},
		},
	}

	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: clStore,
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	steps_result, err := agent.GetNextSteps(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(steps_result) != 0 {
		t.Errorf("ожидалось 0 шагов (все выполнены), получил %d", len(steps_result))
	}
}

func TestCoordinatorGetProgress(t *testing.T) {
	steps, _ := json.Marshal([]model.ChecklistStepDef{
		{ID: "step-1", Title: "Шаг 1", Order: 0},
		{ID: "step-2", Title: "Шаг 2", Order: 1},
		{ID: "step-3", Title: "Шаг 3", Order: 2},
	})

	cs := &mockCoordClientStore{
		clients: map[string]*model.Client{
			"client-1": {
				ID:             "client-1",
				Name:           "Тест",
				INN:            "7707083893",
				TenantID:       "tenant-1",
				ResidencyStage: model.StageApplication,
			},
		},
	}

	clID := "cl-entry"
	clStore := &mockCoordChecklistStore{
		checklists: map[string]*model.Checklist{
			clID: {
				ID:            clID,
				Title:         "Вход",
				ProcedureType: model.ChecklistEntry,
				Steps:         steps,
				Version:       "1.0",
			},
		},
		clientCLs: map[string][]*model.ClientChecklist{
			"client-1": {
				{
					ID:          "ccl-1",
					ClientID:    "client-1",
					ChecklistID: clID,
					Status:      model.ChecklistInProgress,
				},
			},
		},
		stepStatuses: map[string][]*model.ChecklistStepStatus{
			"ccl-1": {
				{StepIndex: 0, Status: model.StepDone},
				{StepIndex: 1, Status: model.StepDone},
				{StepIndex: 2, Status: model.StepPending},
			},
		},
	}

	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: clStore,
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	progress, err := agent.GetProgress(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if progress.ClientID != "client-1" {
		t.Errorf("ClientID = %q, хотел %q", progress.ClientID, "client-1")
	}
	if progress.Stage != string(model.StageApplication) {
		t.Errorf("Stage = %q, хотел %q", progress.Stage, model.StageApplication)
	}
	if len(progress.ChecklistProgress) != 1 {
		t.Fatalf("ChecklistProgress len = %d, хотел 1", len(progress.ChecklistProgress))
	}
	// 2 из 3 шагов = 67%.
	expectedProgress := 67.0
	if progress.ChecklistProgress[0].Progress != expectedProgress {
		t.Errorf("Progress = %.0f, хотел %.0f", progress.ChecklistProgress[0].Progress, expectedProgress)
	}
}

func TestCoordinatorGenerateEscalation_OverdueDeadline(t *testing.T) {
	cs := &mockCoordClientStore{
		clients: map[string]*model.Client{
			"client-1": {
				ID:       "client-1",
				Name:     "Тест",
				INN:      "7707083893",
				TenantID: "tenant-1",
			},
		},
	}

	ds := &mockCoordDeadlineStore{
		deadlines: []*model.Deadline{
			{
				ID:       "dl-1",
				ClientID: "client-1",
				Title:    "Сдать отчёт",
				DueDate:  time.Now().Add(-5 * 24 * time.Hour), // 5 дней назад
				Status:   model.DeadlineUpcoming,
			},
		},
	}

	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  ds,
	}
	agent := NewCoordinatorAgent(stores)

	alert, err := agent.GenerateEscalation(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if alert == nil {
		t.Fatal("ожидался алерт о просроченном дедлайне")
	}
	if alert.AlertType != "deadline_overdue" {
		t.Errorf("AlertType = %q, хотел %q", alert.AlertType, "deadline_overdue")
	}
	if alert.ClientID != "client-1" {
		t.Errorf("ClientID = %q, хотел %q", alert.ClientID, "client-1")
	}
	if alert.DaysStuck < 4 {
		t.Errorf("DaysStuck = %d, ожидалось >= 4", alert.DaysStuck)
	}
}

func TestCoordinatorGenerateEscalation_NoReason(t *testing.T) {
	cs := &mockCoordClientStore{
		clients: map[string]*model.Client{
			"client-1": {
				ID:       "client-1",
				Name:     "Тест",
				INN:      "7707083893",
				TenantID: "tenant-1",
			},
		},
		transitions: map[string][]*model.StageTransition{
			"client-1": {
				{
					ClientID:       "client-1",
					FromStage:      model.StageApplication,
					ToStage:        model.StageExamination,
					TransitionedAt: time.Now().Add(-1 * time.Hour), // недавно
				},
			},
		},
	}

	ds := &mockCoordDeadlineStore{
		deadlines: []*model.Deadline{
			{
				ID:       "dl-1",
				ClientID: "client-1",
				Title:    "Сдать отчёт",
				DueDate:  time.Now().Add(30 * 24 * time.Hour), // через 30 дней
				Status:   model.DeadlineUpcoming,
			},
		},
	}

	stores := CoordinatorStores{
		ClientStore:    cs,
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  ds,
	}
	agent := NewCoordinatorAgent(stores)

	alert, err := agent.GenerateEscalation(context.Background(), "client-1")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if alert != nil {
		t.Errorf("ожидался nil алерт, получил %+v", alert)
	}
}

func TestCoordinatorGenerateEscalation_EmptyClientID(t *testing.T) {
	stores := CoordinatorStores{
		ClientStore:    &mockCoordClientStore{clients: make(map[string]*model.Client)},
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	_, err := agent.GenerateEscalation(context.Background(), "")
	if err == nil {
		t.Fatal("ожидалась ошибка при пустом clientID")
	}
}

func TestStageToChecklistType(t *testing.T) {
	tests := []struct {
		stage model.ResidencyStage
		want  model.ChecklistType
	}{
		{model.StageApplication, model.ChecklistEntry},
		{model.StageExamination, model.ChecklistEntry},
		{model.StageDecision, model.ChecklistEntry},
		{model.StageContract, model.ChecklistEntry},
		{model.StageResident, model.ChecklistEntry},
		{model.StageReporting, model.ChecklistReporting},
		{model.StageExtension, model.ChecklistExtension},
		{model.StageExit, model.ChecklistExit},
	}

	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			got := stageToChecklistType(tt.stage)
			if got != tt.want {
				t.Errorf("stageToChecklistType(%q) = %q, хотел %q", tt.stage, got, tt.want)
			}
		})
	}
}

func TestFindNextStep(t *testing.T) {
	steps := []model.ChecklistStepDef{
		{ID: "s1", Title: "Шаг 1", Order: 0},
		{ID: "s2", Title: "Шаг 2", Order: 1},
		{ID: "s3", Title: "Шаг 3", Order: 2},
	}

	// Все шаги pending.
	result := findNextStep(steps, nil)
	if result == nil {
		t.Fatal("ожидался следующий шаг")
	}
	if result.Title != "Шаг 1" {
		t.Errorf("Title = %q, хотел %q", result.Title, "Шаг 1")
	}

	// Первый шаг выполнен.
	statuses := []*model.ChecklistStepStatus{
		{StepIndex: 0, Status: model.StepDone},
	}
	result = findNextStep(steps, statuses)
	if result == nil {
		t.Fatal("ожидался следующий шаг после выполненного")
	}
	if result.Title != "Шаг 2" {
		t.Errorf("Title = %q, хотел %q", result.Title, "Шаг 2")
	}

	// Все шаги выполнены.
	allDone := []*model.ChecklistStepStatus{
		{StepIndex: 0, Status: model.StepDone},
		{StepIndex: 1, Status: model.StepDone},
		{StepIndex: 2, Status: model.StepDone},
	}
	result = findNextStep(steps, allDone)
	if result != nil {
		t.Errorf("ожидался nil, получил %v", result.Title)
	}

	// Пропущенный шаг тоже считается.
	skipped := []*model.ChecklistStepStatus{
		{StepIndex: 0, Status: model.StepSkipped},
	}
	result = findNextStep(steps, skipped)
	if result == nil {
		t.Fatal("ожидался следующий шаг после пропущенного")
	}
	if result.Title != "Шаг 2" {
		t.Errorf("Title = %q, хотел %q", result.Title, "Шаг 2")
	}
}

func TestDeterminePriority(t *testing.T) {
	ctx := context.Background()

	// Просроченный дедлайн = high.
	ds := &mockCoordDeadlineStore{
		deadlines: []*model.Deadline{
			{
				ClientID: "client-1",
				DueDate:  time.Now().Add(-1 * 24 * time.Hour),
				Status:   model.DeadlineUpcoming,
			},
		},
	}
	priority := determinePriority(&model.ChecklistStepDef{Title: "test"}, "client-1", ds, ctx)
	if priority != "high" {
		t.Errorf("priority = %q, хотел %q", priority, "high")
	}

	// Дедлайн через 2 дня = high.
	ds2 := &mockCoordDeadlineStore{
		deadlines: []*model.Deadline{
			{
				ClientID: "client-1",
				DueDate:  time.Now().Add(2 * 24 * time.Hour),
				Status:   model.DeadlineUpcoming,
			},
		},
	}
	priority = determinePriority(&model.ChecklistStepDef{Title: "test"}, "client-1", ds2, ctx)
	if priority != "high" {
		t.Errorf("priority = %q, хотел %q", priority, "high")
	}

	// Дедлайн через 5 дней = medium.
	ds3 := &mockCoordDeadlineStore{
		deadlines: []*model.Deadline{
			{
				ClientID: "client-1",
				DueDate:  time.Now().Add(5 * 24 * time.Hour),
				Status:   model.DeadlineUpcoming,
			},
		},
	}
	priority = determinePriority(&model.ChecklistStepDef{Title: "test"}, "client-1", ds3, ctx)
	if priority != "medium" {
		t.Errorf("priority = %q, хотел %q", priority, "medium")
	}

	// Дедлайн через 20 дней = low.
	ds4 := &mockCoordDeadlineStore{
		deadlines: []*model.Deadline{
			{
				ClientID: "client-1",
				DueDate:  time.Now().Add(20 * 24 * time.Hour),
				Status:   model.DeadlineUpcoming,
			},
		},
	}
	priority = determinePriority(&model.ChecklistStepDef{Title: "test"}, "client-1", ds4, ctx)
	if priority != "low" {
		t.Errorf("priority = %q, хотел %q", priority, "low")
	}

	// Нет дедлайнов = medium.
	ds5 := &mockCoordDeadlineStore{deadlines: nil}
	priority = determinePriority(&model.ChecklistStepDef{Title: "test"}, "client-1", ds5, ctx)
	if priority != "medium" {
		t.Errorf("priority = %q, хотел %q", priority, "medium")
	}

	// Nil deadline store = medium.
	priority = determinePriority(&model.ChecklistStepDef{Title: "test"}, "client-1", nil, ctx)
	if priority != "medium" {
		t.Errorf("priority = %q, хотел %q", priority, "medium")
	}
}

func TestPriorityOrder(t *testing.T) {
	tests := []struct {
		priority string
		want     int
	}{
		{"high", 1},
		{"medium", 2},
		{"low", 3},
		{"unknown", 2},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got := priorityOrder(tt.priority)
			if got != tt.want {
				t.Errorf("priorityOrder(%q) = %d, хотел %d", tt.priority, got, tt.want)
			}
		})
	}
}

func TestCountDone(t *testing.T) {
	statuses := []*model.ChecklistStepStatus{
		{Status: model.StepDone},
		{Status: model.StepPending},
		{Status: model.StepDone},
		{Status: model.StepInProgress},
	}

	got := countDone(statuses)
	if got != 2 {
		t.Errorf("countDone = %d, хотел 2", got)
	}
}

func TestSeverityForDays(t *testing.T) {
	tests := []struct {
		days int
		want string
	}{
		{0, "warning"},
		{10, "warning"},
		{29, "warning"},
		{30, "critical"},
		{100, "critical"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := severityForDays(tt.days)
			if got != tt.want {
				t.Errorf("severityForDays(%d) = %q, хотел %q", tt.days, got, tt.want)
			}
		})
	}
}

func TestCoordinatorGetProgress_EmptyClientID(t *testing.T) {
	stores := CoordinatorStores{
		ClientStore:    &mockCoordClientStore{clients: make(map[string]*model.Client)},
		ChecklistStore: &mockCoordChecklistStore{checklists: make(map[string]*model.Checklist)},
		DeadlineStore:  &mockCoordDeadlineStore{},
	}
	agent := NewCoordinatorAgent(stores)

	_, err := agent.GetProgress(context.Background(), "")
	if err == nil {
		t.Fatal("ожидалась ошибка при пустом clientID")
	}
}
