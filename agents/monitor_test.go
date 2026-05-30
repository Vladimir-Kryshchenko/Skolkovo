package agents

import (
	"context"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// --- Mock-хранилища для монитора ---

type mockMonitorDocStore struct {
	docs []model.Document
}

func (m *mockMonitorDocStore) Upsert(_ context.Context, _ model.Document) error        { return nil }
func (m *mockMonitorDocStore) Get(_ context.Context, _ string) (model.Document, error) { return model.Document{}, nil }
func (m *mockMonitorDocStore) List(_ context.Context, f store.Filter) ([]model.Document, error) {
	var result []model.Document
	for _, d := range m.docs {
		if f.Status == "" || d.Status == f.Status {
			result = append(result, d)
		}
	}
	return result, nil
}
func (m *mockMonitorDocStore) SetStatus(_ context.Context, _ string, _ model.Status) error { return nil }
func (m *mockMonitorDocStore) SetIndexed(_ context.Context, _ string, _ bool) error        { return nil }
func (m *mockMonitorDocStore) Delete(_ context.Context, _ string) error                    { return nil }
func (m *mockMonitorDocStore) Close() error                                                { return nil }

var _ store.Store = (*mockMonitorDocStore)(nil)

type mockMonitorClientStore struct {
	clients []*model.Client
}

func (m *mockMonitorClientStore) CreateClient(_ context.Context, _ *model.Client) error           { return nil }
func (m *mockMonitorClientStore) GetClient(_ context.Context, id string) (*model.Client, error) {
	for _, c := range m.clients {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (m *mockMonitorClientStore) GetClientByINN(_ context.Context, _ string) (*model.Client, error) {
	return nil, nil
}
func (m *mockMonitorClientStore) UpdateClient(_ context.Context, _ *model.Client) error { return nil }
func (m *mockMonitorClientStore) DeleteClient(_ context.Context, _ string) error        { return nil }
func (m *mockMonitorClientStore) ListClients(_ context.Context, _ string, _ model.ResidencyStage) ([]*model.Client, error) {
	return m.clients, nil
}
func (m *mockMonitorClientStore) AddStageTransition(_ context.Context, _ *model.StageTransition) error {
	return nil
}
func (m *mockMonitorClientStore) GetStageHistory(_ context.Context, _ string) ([]*model.StageTransition, error) {
	return nil, nil
}

var _ store.ClientStore = (*mockMonitorClientStore)(nil)

type mockEventStore struct{}

func (m *mockEventStore) CreateEvent(_ context.Context, _ *model.Event) error { return nil }
func (m *mockEventStore) GetEvent(_ context.Context, _ string) (*model.Event, error) {
	return nil, nil
}
func (m *mockEventStore) ListEvents(_ context.Context, _ string, _ model.EventStatus, _, _ *time.Time) ([]*model.Event, error) {
	return nil, nil
}
func (m *mockEventStore) UpdateEvent(_ context.Context, _ *model.Event) error { return nil }
func (m *mockEventStore) DeleteEvent(_ context.Context, _ string) error       { return nil }
func (m *mockEventStore) CountEvents(_ context.Context) (int, error)          { return 0, nil }

var _ store.EventStore = (*mockEventStore)(nil)

type mockContestStore struct{}

func (m *mockContestStore) CreateContest(_ context.Context, _ *model.Contest) error { return nil }
func (m *mockContestStore) GetContest(_ context.Context, _ string) (*model.Contest, error) {
	return nil, nil
}
func (m *mockContestStore) ListContests(_ context.Context, _ string, _ model.ContestStatus) ([]*model.Contest, error) {
	return nil, nil
}
func (m *mockContestStore) UpdateContest(_ context.Context, _ *model.Contest) error { return nil }
func (m *mockContestStore) DeleteContest(_ context.Context, _ string) error         { return nil }
func (m *mockContestStore) CountActiveContests(_ context.Context) (int, error) {
	return 0, nil
}

var _ store.ContestStore = (*mockContestStore)(nil)

type mockDeadlineStore struct{}

func (m *mockDeadlineStore) CreateDeadline(_ context.Context, _ *model.Deadline) error { return nil }
func (m *mockDeadlineStore) GetDeadline(_ context.Context, _ string) (*model.Deadline, error) {
	return nil, nil
}
func (m *mockDeadlineStore) UpdateDeadline(_ context.Context, _ *model.Deadline) error { return nil }
func (m *mockDeadlineStore) ListDeadlines(_ context.Context, _ string, _ int) ([]*model.Deadline, error) {
	return nil, nil
}
func (m *mockDeadlineStore) ListOverdueDeadlines(_ context.Context) ([]*model.Deadline, error) {
	return nil, nil
}
func (m *mockDeadlineStore) MarkNotificationSent(_ context.Context, _ string) error { return nil }

var _ store.DeadlineStore = (*mockDeadlineStore)(nil)

func TestNewMonitorAgent(t *testing.T) {
	stores := MonitorStores{
		DocStore:      &mockMonitorDocStore{},
		EventStore:    &mockEventStore{},
		ContestStore:  &mockContestStore{},
		ClientStore:   &mockMonitorClientStore{},
		DeadlineStore: &mockDeadlineStore{},
	}
	agent := NewMonitorAgent(stores)
	if agent == nil {
		t.Fatal("ожидался не-nil агент")
	}
}

func TestMonitorCheckForChanges_Empty(t *testing.T) {
	stores := MonitorStores{
		DocStore:      &mockMonitorDocStore{},
		EventStore:    &mockEventStore{},
		ContestStore:  &mockContestStore{},
		ClientStore:   &mockMonitorClientStore{},
		DeadlineStore: &mockDeadlineStore{},
	}
	agent := NewMonitorAgent(stores)

	notifications, err := agent.CheckForChanges(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(notifications) != 0 {
		t.Errorf("ожидалось 0 уведомлений, получил %d", len(notifications))
	}
}

func TestMonitorCheckForChanges_NewDocument(t *testing.T) {
	now := time.Now()
	ds := &mockMonitorDocStore{
		docs: []model.Document{
			{
				ID:        "doc-1",
				Title:     "Новый документ",
				Status:    model.StatusActive,
				Category:  "regulations",
				FetchedAt: now.Add(-1 * time.Hour), // загружен час назад (после since)
			},
		},
	}
	stores := MonitorStores{
		DocStore:      ds,
		EventStore:    &mockEventStore{},
		ContestStore:  &mockContestStore{},
		ClientStore:   &mockMonitorClientStore{},
		DeadlineStore: &mockDeadlineStore{},
	}
	agent := NewMonitorAgent(stores)

	notifications, err := agent.CheckForChanges(context.Background(), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("ожидалось 1 уведомление, получил %d", len(notifications))
	}
	if notifications[0].ChangeType != "new" {
		t.Errorf("changeType = %q, хотел %q", notifications[0].ChangeType, "new")
	}
	if notifications[0].DocumentID != "doc-1" {
		t.Errorf("DocumentID = %q, хотел %q", notifications[0].DocumentID, "doc-1")
	}
}

func TestMonitorCheckForChanges_OldDocument(t *testing.T) {
	ds := &mockMonitorDocStore{
		docs: []model.Document{
			{
				ID:        "doc-old",
				Title:     "Старый документ",
				Status:    model.StatusActive,
				Category:  "regulations",
				FetchedAt: time.Now().Add(-48 * time.Hour), // загружен 2 дня назад
			},
		},
	}
	stores := MonitorStores{
		DocStore:      ds,
		EventStore:    &mockEventStore{},
		ContestStore:  &mockContestStore{},
		ClientStore:   &mockMonitorClientStore{},
		DeadlineStore: &mockDeadlineStore{},
	}
	agent := NewMonitorAgent(stores)

	notifications, err := agent.CheckForChanges(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(notifications) != 0 {
		t.Errorf("ожидалось 0 уведомлений для старого документа, получил %d", len(notifications))
	}
}

func TestMonitorGenerateDigest(t *testing.T) {
	stores := MonitorStores{
		DocStore:      &mockMonitorDocStore{},
		EventStore:    &mockEventStore{},
		ContestStore:  &mockContestStore{},
		ClientStore:   &mockMonitorClientStore{},
		DeadlineStore: &mockDeadlineStore{},
	}
	agent := NewMonitorAgent(stores)

	digest, err := agent.GenerateDigest(context.Background(), "client-1", 7)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if digest.ClientID != "client-1" {
		t.Errorf("ClientID = %q, хотел %q", digest.ClientID, "client-1")
	}
	if digest.GeneratedAt.IsZero() {
		t.Error("GeneratedAt не установлен")
	}
}

func TestMonitorSubscriptions(t *testing.T) {
	stores := MonitorStores{
		DocStore:     &mockMonitorDocStore{},
		EventStore:   &mockEventStore{},
		ContestStore: &mockContestStore{},
		ClientStore:  &mockMonitorClientStore{},
	}
	agent := NewMonitorAgent(stores)

	// По умолчанию — все категории.
	subs := agent.GetSubscriptions(context.Background(), "client-1")
	if len(subs) == 0 {
		t.Error("ожидались подписки по умолчанию")
	}

	// Подписка на конкретную категорию.
	err := agent.Subscribe(context.Background(), "client-1", []string{"custom"})
	if err != nil {
		t.Fatalf("ошибка подписки: %v", err)
	}

	subs = agent.GetSubscriptions(context.Background(), "client-1")
	found := false
	for _, s := range subs {
		if s == "custom" {
			found = true
		}
	}
	if !found {
		t.Error("категория 'custom' не найдена в подписках")
	}

	// Отписка.
	err = agent.Unsubscribe(context.Background(), "client-1", []string{"custom"})
	if err != nil {
		t.Fatalf("ошибка отписки: %v", err)
	}

	subs = agent.GetSubscriptions(context.Background(), "client-1")
	for _, s := range subs {
		if s == "custom" {
			t.Error("категория 'custom' всё ещё в подписках после отписки")
		}
	}
}

func TestMonitorSubscribe_EmptyCategories(t *testing.T) {
	stores := MonitorStores{
		DocStore:     &mockMonitorDocStore{},
		EventStore:   &mockEventStore{},
		ContestStore: &mockContestStore{},
		ClientStore:  &mockMonitorClientStore{},
	}
	agent := NewMonitorAgent(stores)

	err := agent.Subscribe(context.Background(), "client-1", []string{})
	if err == nil {
		t.Fatal("ожидалась ошибка при пустых категориях")
	}
}

func TestMonitorUnsubscribe_EmptyCategories(t *testing.T) {
	stores := MonitorStores{
		DocStore:     &mockMonitorDocStore{},
		EventStore:   &mockEventStore{},
		ContestStore: &mockContestStore{},
		ClientStore:  &mockMonitorClientStore{},
	}
	agent := NewMonitorAgent(stores)

	err := agent.Unsubscribe(context.Background(), "client-1", []string{})
	if err == nil {
		t.Fatal("ожидалась ошибка при пустых категориях")
	}
}

func TestMonitorUnsubscribe_NotSubscribed(t *testing.T) {
	stores := MonitorStores{
		DocStore:     &mockMonitorDocStore{},
		EventStore:   &mockEventStore{},
		ContestStore: &mockContestStore{},
		ClientStore:  &mockMonitorClientStore{},
	}
	agent := NewMonitorAgent(stores)

	// Отписка клиента, который не подписан — не должно быть ошибки.
	err := agent.Unsubscribe(context.Background(), "new-client", []string{"regulations"})
	if err != nil {
		t.Errorf("неожиданная ошибка: %v", err)
	}
}

func TestClassifyChange(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		doc      model.Document
		since    time.Time
		wantType string
	}{
		{
			name: "new document fetched after since",
			doc: model.Document{
				ID:        "doc-1",
				Title:     "Test",
				Status:    model.StatusActive,
				FetchedAt: now.Add(-1 * time.Hour),
			},
			since:    now.Add(-24 * time.Hour),
			wantType: "new",
		},
		{
			name: "updated document published after since",
			doc: model.Document{
				ID:          "doc-2",
				Title:       "Test",
				Status:      model.StatusActive,
				FetchedAt:   now.Add(-48 * time.Hour),
				PublishedAt: ptrTime(now.Add(-1 * time.Hour)),
			},
			since:    now.Add(-24 * time.Hour),
			wantType: "updated",
		},
		{
			name: "outdated document",
			doc: model.Document{
				ID:        "doc-3",
				Title:     "Test",
				Status:    model.StatusOutdated,
				FetchedAt: now.Add(-48 * time.Hour),
			},
			since:    now.Add(-24 * time.Hour),
			wantType: "outdated",
		},
		{
			name: "old document no changes",
			doc: model.Document{
				ID:        "doc-4",
				Title:     "Test",
				Status:    model.StatusActive,
				FetchedAt: now.Add(-48 * time.Hour),
			},
			since:    now.Add(-24 * time.Hour),
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyChange(tt.doc, tt.since)
			if got != tt.wantType {
				t.Errorf("classifyChange = %q, хотел %q", got, tt.wantType)
			}
		})
	}
}

func TestFormatChangeDescription(t *testing.T) {
	doc := model.Document{Title: "Тестовый документ", Status: model.StatusActive}

	tests := []struct {
		changeType string
		want       string
	}{
		{"new", "Новый документ в базе: Тестовый документ"},
		{"updated", "Обновлён документ: Тестовый документ"},
		{"status_changed", "Изменён статус документа Тестовый документ на действует"},
		{"outdated", "Документ утратил силу: Тестовый документ"},
		{"other", "Изменение в документе: Тестовый документ"},
	}

	for _, tt := range tests {
		t.Run(tt.changeType, func(t *testing.T) {
			got := formatChangeDescription(tt.changeType, doc)
			if got != tt.want {
				t.Errorf("formatChangeDescription(%q) = %q, хотел %q", tt.changeType, got, tt.want)
			}
		})
	}
}

func TestFilterBySubscriptions(t *testing.T) {
	changes := []ChangeNotification{
		{Category: "regulations"},
		{Category: "events"},
		{Category: "contests"},
	}

	// Фильтр по одной категории.
	filtered := filterBySubscriptions(changes, []string{"regulations"})
	if len(filtered) != 1 {
		t.Errorf("filtered = %d, хотел 1", len(filtered))
	}
	if filtered[0].Category != "regulations" {
		t.Errorf("category = %q, хотел %q", filtered[0].Category, "regulations")
	}

	// Фильтр "all" — все категории.
	filtered = filterBySubscriptions(changes, []string{"all"})
	if len(filtered) != 3 {
		t.Errorf("filtered with 'all' = %d, хотел 3", len(filtered))
	}

	// Пустые подписки — все категории.
	filtered = filterBySubscriptions(changes, nil)
	if len(filtered) != 3 {
		t.Errorf("filtered with nil subs = %d, хотел 3", len(filtered))
	}
}

func TestGenerateSummary(t *testing.T) {
	changes := []ChangeNotification{
		{Title: "Документ 1"},
		{Title: "Документ 2"},
	}

	summary := generateSummary("client-1", 5, 7, "резидент", changes)
	if summary == "" {
		t.Error("summary пуст")
	}

	// Без изменений.
	summary = generateSummary("client-1", 0, 7, "", nil)
	if summary == "" {
		t.Error("summary для 0 изменений пуст")
	}
}

func TestIsClientAffected(t *testing.T) {
	tests := []struct {
		name   string
		stage  model.ResidencyStage
		doc    model.Document
		want   bool
	}{
		{
			name:  "reporting stage + reporting doc",
			stage: model.StageReporting,
			doc:   model.Document{Category: "отчётность"},
			want:  true,
		},
		{
			name:  "reporting stage + unrelated doc",
			stage: model.StageReporting,
			doc:   model.Document{Category: "конкурсы"},
			want:  false,
		},
		{
			name:  "exit stage + exit doc",
			stage: model.StageExit,
			doc:   model.Document{Category: "выход"},
			want:  true,
		},
		{
			name:  "application stage + entry doc",
			stage: model.StageApplication,
			doc:   model.Document{Category: "заявка"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &model.Client{ID: "client-1", ResidencyStage: tt.stage}
			got := isClientAffected(client, tt.doc)
			if got != tt.want {
				t.Errorf("isClientAffected = %v, хотел %v", got, tt.want)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
