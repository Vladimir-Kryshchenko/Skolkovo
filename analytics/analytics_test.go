package analytics

import (
	"context"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// ---------------------------------------------------------------------------
// Mock-реализации хранилищ
// ---------------------------------------------------------------------------

type mockDocStore struct {
	docs []model.Document
}

func (m *mockDocStore) Upsert(_ context.Context, _ model.Document) error { return nil }
func (m *mockDocStore) Get(_ context.Context, _ string) (model.Document, error) {
	return model.Document{}, store.ErrNotFound
}
func (m *mockDocStore) List(_ context.Context, _ store.Filter) ([]model.Document, error) {
	return m.docs, nil
}
func (m *mockDocStore) SetStatus(_ context.Context, _ string, _ model.Status) error { return nil }
func (m *mockDocStore) SetIndexed(_ context.Context, _ string, _ bool) error        { return nil }
func (m *mockDocStore) Delete(_ context.Context, _ string) error                    { return nil }
func (m *mockDocStore) Close() error                                                { return nil }

type mockClientStore struct {
	clients []*model.Client
}

func (m *mockClientStore) CreateClient(_ context.Context, _ *model.Client) error { return nil }
func (m *mockClientStore) GetClient(_ context.Context, _ string) (*model.Client, error) {
	return nil, nil
}
func (m *mockClientStore) GetClientByINN(_ context.Context, _ string) (*model.Client, error) {
	return nil, nil
}
func (m *mockClientStore) UpdateClient(_ context.Context, _ *model.Client) error { return nil }
func (m *mockClientStore) DeleteClient(_ context.Context, _ string) error        { return nil }
func (m *mockClientStore) ListClients(_ context.Context, _ string, _ model.ResidencyStage) ([]*model.Client, error) {
	return m.clients, nil
}
func (m *mockClientStore) AddStageTransition(_ context.Context, _ *model.StageTransition) error {
	return nil
}
func (m *mockClientStore) GetStageHistory(_ context.Context, _ string) ([]*model.StageTransition, error) {
	return nil, nil
}

type mockChecklistStore struct {
	checklists []*model.Checklist
}

func (m *mockChecklistStore) CreateChecklist(_ context.Context, _ *model.Checklist) error { return nil }
func (m *mockChecklistStore) GetChecklist(_ context.Context, _ string) (*model.Checklist, error) {
	return nil, nil
}
func (m *mockChecklistStore) ListChecklists(_ context.Context, _ model.ChecklistType) ([]*model.Checklist, error) {
	return m.checklists, nil
}
func (m *mockChecklistStore) CreateClientChecklist(_ context.Context, _ *model.ClientChecklist) error {
	return nil
}
func (m *mockChecklistStore) GetClientChecklist(_ context.Context, _ string) (*model.ClientChecklist, error) {
	return nil, nil
}
func (m *mockChecklistStore) GetClientChecklists(_ context.Context, _ string) ([]*model.ClientChecklist, error) {
	return nil, nil
}
func (m *mockChecklistStore) UpdateClientChecklist(_ context.Context, _ *model.ClientChecklist) error {
	return nil
}
func (m *mockChecklistStore) CreateStepStatus(_ context.Context, _ *model.ChecklistStepStatus) error {
	return nil
}
func (m *mockChecklistStore) UpdateStepStatus(_ context.Context, _ string, _ model.StepStatus, _ string) error {
	return nil
}
func (m *mockChecklistStore) GetStepStatuses(_ context.Context, _ string) ([]*model.ChecklistStepStatus, error) {
	return nil, nil
}

type mockDeadlineStore struct {
	overdue  []*model.Deadline
	upcoming []*model.Deadline
}

func (m *mockDeadlineStore) CreateDeadline(_ context.Context, _ *model.Deadline) error { return nil }
func (m *mockDeadlineStore) GetDeadline(_ context.Context, _ string) (*model.Deadline, error) {
	return nil, nil
}
func (m *mockDeadlineStore) UpdateDeadline(_ context.Context, _ *model.Deadline) error { return nil }
func (m *mockDeadlineStore) ListDeadlines(_ context.Context, _ string, _ int) ([]*model.Deadline, error) {
	return m.upcoming, nil
}
func (m *mockDeadlineStore) ListOverdueDeadlines(_ context.Context) ([]*model.Deadline, error) {
	return m.overdue, nil
}
func (m *mockDeadlineStore) MarkNotificationSent(_ context.Context, _ string) error { return nil }

type mockEventStore struct {
	total  int
	active []*model.Event
	past   []*model.Event
}

func (m *mockEventStore) CreateEvent(_ context.Context, _ *model.Event) error        { return nil }
func (m *mockEventStore) GetEvent(_ context.Context, _ string) (*model.Event, error) { return nil, nil }
func (m *mockEventStore) ListEvents(_ context.Context, _ string, status model.EventStatus, _, _ *time.Time) ([]*model.Event, error) {
	switch status {
	case model.EventActive:
		return m.active, nil
	case model.EventPast:
		return m.past, nil
	default:
		all := append([]*model.Event{}, m.active...)
		all = append(all, m.past...)
		return all, nil
	}
}
func (m *mockEventStore) UpdateEvent(_ context.Context, _ *model.Event) error { return nil }
func (m *mockEventStore) DeleteEvent(_ context.Context, _ string) error       { return nil }
func (m *mockEventStore) CountEvents(_ context.Context) (int, error)          { return m.total, nil }

type mockContestStore struct {
	activeCount int
	all         []*model.Contest
}

func (m *mockContestStore) CreateContest(_ context.Context, _ *model.Contest) error { return nil }
func (m *mockContestStore) GetContest(_ context.Context, _ string) (*model.Contest, error) {
	return nil, nil
}
func (m *mockContestStore) ListContests(_ context.Context, _ string, _ model.ContestStatus) ([]*model.Contest, error) {
	return m.all, nil
}
func (m *mockContestStore) UpdateContest(_ context.Context, _ *model.Contest) error { return nil }
func (m *mockContestStore) DeleteContest(_ context.Context, _ string) error         { return nil }
func (m *mockContestStore) CountActiveContests(_ context.Context) (int, error) {
	return m.activeCount, nil
}

// ---------------------------------------------------------------------------
// Тесты
// ---------------------------------------------------------------------------

func TestCollectReport_Documents(t *testing.T) {
	docStore := &mockDocStore{
		docs: []model.Document{
			{ID: "1", Status: model.StatusActive, Category: "regulation", Indexed: true},
			{ID: "2", Status: model.StatusActive, Category: "regulation", Indexed: true},
			{ID: "3", Status: model.StatusOutdated, Category: "guideline", Indexed: false},
			{ID: "4", Status: model.StatusArchived, Category: "", Indexed: false},
			{ID: "5", Status: model.StatusPending, Category: "regulation", Indexed: false},
		},
	}

	report := CollectReport(
		context.Background(),
		docStore,
		&mockClientStore{},
		&mockChecklistStore{},
		&mockDeadlineStore{},
		&mockEventStore{},
		&mockContestStore{},
	)

	if report.DocumentStats.Total != 5 {
		t.Errorf("expected 5 total docs, got %d", report.DocumentStats.Total)
	}
	if report.DocumentStats.ByStatus["действует"] != 2 {
		t.Errorf("expected 2 active docs, got %d", report.DocumentStats.ByStatus["действует"])
	}
	if report.DocumentStats.ByCategory["regulation"] != 3 {
		t.Errorf("expected 3 regulation docs, got %d", report.DocumentStats.ByCategory["regulation"])
	}
	if report.DocumentStats.IndexedCount != 2 {
		t.Errorf("expected 2 indexed docs, got %d", report.DocumentStats.IndexedCount)
	}
}

func TestCollectReport_Clients(t *testing.T) {
	now := time.Now()
	clients := []*model.Client{
		{ID: "1", ResidencyStage: model.StageResident, CreatedAt: now.Add(-2 * 24 * time.Hour)},
		{ID: "2", ResidencyStage: model.StageResident, CreatedAt: now.Add(-15 * 24 * time.Hour)},
		{ID: "3", ResidencyStage: model.StageApplication, CreatedAt: now.Add(-60 * 24 * time.Hour)},
		{ID: "4", ResidencyStage: model.StageReporting, CreatedAt: now.Add(-3 * 24 * time.Hour)},
	}

	report := CollectReport(
		context.Background(),
		&mockDocStore{},
		&mockClientStore{clients: clients},
		&mockChecklistStore{},
		&mockDeadlineStore{},
		&mockEventStore{},
		&mockContestStore{},
	)

	if report.ClientStats.Total != 4 {
		t.Errorf("expected 4 total clients, got %d", report.ClientStats.Total)
	}
	if report.ClientStats.ByStage["резидент"] != 2 {
		t.Errorf("expected 2 resident clients, got %d", report.ClientStats.ByStage["резидент"])
	}
	if report.ClientStats.NewThisWeek != 2 {
		t.Errorf("expected 2 new this week, got %d", report.ClientStats.NewThisWeek)
	}
	if report.ClientStats.NewThisMonth != 3 {
		t.Errorf("expected 3 new this month, got %d", report.ClientStats.NewThisMonth)
	}
}

func TestCollectReport_Deadlines(t *testing.T) {
	ds := &mockDeadlineStore{
		overdue: []*model.Deadline{
			{ID: "d1", Status: model.DeadlineOverdue},
			{ID: "d2", Status: model.DeadlineOverdue},
		},
		upcoming: []*model.Deadline{
			{ID: "d3", Status: model.DeadlineUpcoming},
			{ID: "d4", Status: model.DeadlineCompleted},
		},
	}

	report := CollectReport(
		context.Background(),
		&mockDocStore{},
		&mockClientStore{},
		&mockChecklistStore{},
		ds,
		&mockEventStore{},
		&mockContestStore{},
	)

	if report.DeadlineStats.Overdue != 2 {
		t.Errorf("expected 2 overdue, got %d", report.DeadlineStats.Overdue)
	}
	if report.DeadlineStats.Upcoming30d != 2 {
		t.Errorf("expected 2 upcoming 30d, got %d", report.DeadlineStats.Upcoming30d)
	}
	if report.DeadlineStats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", report.DeadlineStats.Completed)
	}
}

func TestCollectReport_Events(t *testing.T) {
	report := CollectReport(
		context.Background(),
		&mockDocStore{},
		&mockClientStore{},
		&mockChecklistStore{},
		&mockDeadlineStore{},
		&mockEventStore{
			total:  10,
			active: []*model.Event{{ID: "e1"}, {ID: "e2"}, {ID: "e3"}, {ID: "e4"}},
			past:   []*model.Event{{ID: "e5"}, {ID: "e6"}},
		},
		&mockContestStore{},
	)

	if report.EventStats.Total != 10 {
		t.Errorf("expected 10 total events, got %d", report.EventStats.Total)
	}
	if report.EventStats.Active != 4 {
		t.Errorf("expected 4 active events, got %d", report.EventStats.Active)
	}
	if report.EventStats.Past != 2 {
		t.Errorf("expected 2 past events, got %d", report.EventStats.Past)
	}
}

func TestCollectReport_Contests(t *testing.T) {
	now := time.Now()
	contests := []*model.Contest{
		{ID: "c1", Status: model.ContestActive, StartDate: now.Add(-10 * 24 * time.Hour), EndDate: now.Add(10 * 24 * time.Hour)},
		{ID: "c2", Status: model.ContestActive, StartDate: now.Add(-5 * 24 * time.Hour), EndDate: now.Add(20 * 24 * time.Hour)},
		{ID: "c3", Status: model.ContestClosed, StartDate: now.Add(-60 * 24 * time.Hour), EndDate: now.Add(-30 * 24 * time.Hour)},
	}

	report := CollectReport(
		context.Background(),
		&mockDocStore{},
		&mockClientStore{},
		&mockChecklistStore{},
		&mockDeadlineStore{},
		&mockEventStore{},
		&mockContestStore{activeCount: 2, all: contests},
	)

	if report.ContestStats.Total != 3 {
		t.Errorf("expected 3 total contests, got %d", report.ContestStats.Total)
	}
	if report.ContestStats.Active != 2 {
		t.Errorf("expected 2 active contests, got %d", report.ContestStats.Active)
	}
	if report.ContestStats.Closed != 1 {
		t.Errorf("expected 1 closed contest, got %d", report.ContestStats.Closed)
	}
}

func TestCollectReport_Checklists(t *testing.T) {
	checklists := []*model.Checklist{
		{ID: "cl1", Title: "Entry checklist", ProcedureType: model.ChecklistEntry},
		{ID: "cl2", Title: "Reporting checklist", ProcedureType: model.ChecklistReporting},
		{ID: "cl3", Title: "Exit checklist", ProcedureType: model.ChecklistExit},
	}

	report := CollectReport(
		context.Background(),
		&mockDocStore{},
		&mockClientStore{},
		&mockChecklistStore{checklists: checklists},
		&mockDeadlineStore{},
		&mockEventStore{},
		&mockContestStore{},
	)

	if report.ChecklistStats.Total != 3 {
		t.Errorf("expected 3 total checklists, got %d", report.ChecklistStats.Total)
	}
}

func TestCollectReport_MCPStats_IsZero(t *testing.T) {
	report := CollectReport(
		context.Background(),
		&mockDocStore{},
		&mockClientStore{},
		&mockChecklistStore{},
		&mockDeadlineStore{},
		&mockEventStore{},
		&mockContestStore{},
	)

	if report.MCPStats.TotalRequests != 0 {
		t.Errorf("expected 0 MCP requests, got %d", report.MCPStats.TotalRequests)
	}
	if report.MCPStats.AvgLatencyMs != 0 {
		t.Errorf("expected 0 avg latency, got %.2f", report.MCPStats.AvgLatencyMs)
	}
	if report.MCPStats.ErrorRate != 0 {
		t.Errorf("expected 0 error rate, got %.4f", report.MCPStats.ErrorRate)
	}
}

func TestToHTML_ContainsChartJS(t *testing.T) {
	report := &AnalyticsReport{
		DocumentStats: DocumentStats{
			Total:      10,
			ByStatus:   map[string]int{"действует": 6, "устарел": 4},
			ByCategory: map[string]int{"regulation": 7, "guideline": 3},
		},
		ClientStats: ClientStats{
			Total:   20,
			ByStage: map[string]int{"резидент": 12, "подача_заявки": 8},
		},
		DeadlineStats:  DeadlineStats{Total: 15, Overdue: 3, Upcoming30d: 5, Completed: 7},
		EventStats:     EventStats{Total: 10, Active: 4, Past: 6},
		ContestStats:   ContestStats{Total: 5, Active: 2, Closed: 3},
		ChecklistStats: ChecklistStats{Total: 4, InProgress: 2, Completed: 2},
		MCPStats:       MCPStats{TotalRequests: 100, AvgLatencyMs: 45.3, ErrorRate: 0.02},
		Period:         Period{From: time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2025, 5, 29, 0, 0, 0, 0, time.UTC)},
	}

	html := ToHTML(report)

	if !strings.Contains(html, "chart.js") {
		t.Error("HTML should contain Chart.js CDN")
	}
	if !strings.Contains(html, "chartDocStatus") {
		t.Error("HTML should contain chartDocStatus canvas")
	}
	if !strings.Contains(html, "chartClientStage") {
		t.Error("HTML should contain chartClientStage canvas")
	}
	if !strings.Contains(html, "type: 'pie'") {
		t.Error("HTML should contain pie chart type")
	}
	if !strings.Contains(html, "type: 'bar'") {
		t.Error("HTML should contain bar chart type")
	}
	if !strings.Contains(html, "Аналитика") {
		t.Error("HTML should contain title")
	}
}

func TestToCSV_Output(t *testing.T) {
	report := &AnalyticsReport{
		DocumentStats: DocumentStats{
			Total:        10,
			ByStatus:     map[string]int{"действует": 6, "устарел": 4},
			ByCategory:   map[string]int{"regulation": 7},
			IndexedCount: 5,
		},
		ClientStats: ClientStats{
			Total:        20,
			ByStage:      map[string]int{"резидент": 12},
			NewThisWeek:  3,
			NewThisMonth: 8,
		},
		DeadlineStats:  DeadlineStats{Total: 15, Overdue: 3, Upcoming30d: 5, Completed: 7},
		EventStats:     EventStats{Total: 10, Active: 4, Past: 6},
		ContestStats:   ContestStats{Total: 5, Active: 2, Closed: 3},
		ChecklistStats: ChecklistStats{Total: 4, InProgress: 2, Completed: 2},
		MCPStats:       MCPStats{TotalRequests: 100, AvgLatencyMs: 45.30, ErrorRate: 0.02},
		Period:         Period{From: time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2025, 5, 29, 0, 0, 0, 0, time.UTC)},
	}

	csv := ToCSV(report)

	if !strings.Contains(csv, "Section,Metric,Value") {
		t.Error("CSV should contain header row")
	}
	if !strings.Contains(csv, "Documents,total,10") {
		t.Error("CSV should contain documents total")
	}
	if !strings.Contains(csv, "Clients,new_this_week,3") {
		t.Error("CSV should contain new_this_week")
	}
	if !strings.Contains(csv, "Deadlines,overdue,3") {
		t.Error("CSV should contain overdue")
	}
	if !strings.Contains(csv, "MCP,avg_latency_ms,45.30") {
		t.Error("CSV should contain avg_latency_ms")
	}
	if !strings.Contains(csv, "Period,from,2025-05-01") {
		t.Error("CSV should contain period from")
	}
}

func TestGetPopularQueries(t *testing.T) {
	queries := GetPopularQueries()

	if len(queries) == 0 {
		t.Error("expected non-empty popular queries list")
	}

	for _, q := range queries {
		if q.Query == "" {
			t.Error("query should not be empty")
		}
		if q.Count <= 0 {
			t.Errorf("query %q should have positive count, got %d", q.Query, q.Count)
		}
	}
}

func TestMapToRows_Sorted(t *testing.T) {
	m := map[string]int{"banana": 3, "apple": 1, "cherry": 2}
	rs := mapToRows(m)

	if rs.labels[0] != "apple" || rs.labels[1] != "banana" || rs.labels[2] != "cherry" {
		t.Errorf("labels should be sorted, got %v", rs.labels)
	}
	if rs.values[0] != 1 || rs.values[1] != 3 || rs.values[2] != 2 {
		t.Errorf("values should match sorted labels, got %v", rs.values)
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"a < b", "a &lt; b"},
		{"x & y", "x &amp; y"},
		{`"quoted"`, "&quot;quoted&quot;"},
	}
	for _, tt := range tests {
		got := escapeHTML(tt.input)
		if got != tt.want {
			t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCollectReport_NilSafe(t *testing.T) {
	// Проверяем, что CollectReport не паникует при nil-хранилищах
	// (mock-реализации возвращают nil/пустые значения)
	report := CollectReport(
		context.Background(),
		&mockDocStore{docs: nil},
		&mockClientStore{clients: nil},
		&mockChecklistStore{checklists: nil},
		&mockDeadlineStore{},
		&mockEventStore{},
		&mockContestStore{},
	)

	if report == nil {
		t.Fatal("report should not be nil")
	}
}

func TestToHTML_EmptyReport(t *testing.T) {
	report := &AnalyticsReport{}
	html := ToHTML(report)

	// Даже пустой отчёт должен содержать базовую структуру
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("empty report HTML should contain DOCTYPE")
	}
	if !strings.Contains(html, "chart.js") {
		t.Error("empty report HTML should contain Chart.js")
	}
}

func TestToCSV_EmptyReport(t *testing.T) {
	report := &AnalyticsReport{}
	csv := ToCSV(report)

	if !strings.Contains(csv, "Section,Metric,Value") {
		t.Error("empty CSV should contain header row")
	}
}
