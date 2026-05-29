package validator

import (
	"context"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// ---------------------------------------------------------------------------
// Mock: store.Store
// ---------------------------------------------------------------------------

type mockDocStore struct {
	docs []model.Document
	err  error
}

func (m *mockDocStore) Upsert(_ context.Context, _ model.Document) error { return m.err }
func (m *mockDocStore) Get(_ context.Context, _ string) (model.Document, error) {
	return model.Document{}, m.err
}
func (m *mockDocStore) List(_ context.Context, _ store.Filter) ([]model.Document, error) {
	return m.docs, m.err
}
func (m *mockDocStore) SetStatus(_ context.Context, _ string, _ model.Status) error { return m.err }
func (m *mockDocStore) SetIndexed(_ context.Context, _ string, _ bool) error        { return m.err }
func (m *mockDocStore) Delete(_ context.Context, _ string) error                    { return m.err }
func (m *mockDocStore) Close() error                                                { return m.err }

var _ store.Store = (*mockDocStore)(nil)

// ---------------------------------------------------------------------------
// Mock: EventStore
// ---------------------------------------------------------------------------

type mockEventStore struct {
	count int
	err   error
}

func (m *mockEventStore) CreateEvent(_ context.Context, _ *model.Event) error { return m.err }
func (m *mockEventStore) GetEvent(_ context.Context, _ string) (*model.Event, error) {
	return nil, m.err
}
func (m *mockEventStore) ListEvents(_ context.Context, _ string, _ model.EventStatus, _, _ *time.Time) ([]*model.Event, error) {
	return nil, m.err
}
func (m *mockEventStore) UpdateEvent(_ context.Context, _ *model.Event) error { return m.err }
func (m *mockEventStore) DeleteEvent(_ context.Context, _ string) error       { return m.err }
func (m *mockEventStore) CountEvents(_ context.Context) (int, error)          { return m.count, m.err }

var _ store.EventStore = (*mockEventStore)(nil)

// ---------------------------------------------------------------------------
// Mock: ContestStore
// ---------------------------------------------------------------------------

type mockContestStore struct {
	activeCount int
	err         error
}

func (m *mockContestStore) CreateContest(_ context.Context, _ *model.Contest) error { return m.err }
func (m *mockContestStore) GetContest(_ context.Context, _ string) (*model.Contest, error) {
	return nil, m.err
}
func (m *mockContestStore) ListContests(_ context.Context, _ string, _ model.ContestStatus) ([]*model.Contest, error) {
	return nil, m.err
}
func (m *mockContestStore) UpdateContest(_ context.Context, _ *model.Contest) error { return m.err }
func (m *mockContestStore) DeleteContest(_ context.Context, _ string) error         { return m.err }
func (m *mockContestStore) CountActiveContests(_ context.Context) (int, error) {
	return m.activeCount, m.err
}

var _ store.ContestStore = (*mockContestStore)(nil)

// ---------------------------------------------------------------------------
// Mock: FAQStore
// ---------------------------------------------------------------------------

type mockFAQStore struct {
	count int
	err   error
}

func (m *mockFAQStore) CreateFAQItem(_ context.Context, _ *model.FAQItem) error { return m.err }
func (m *mockFAQStore) GetFAQItem(_ context.Context, _ string) (*model.FAQItem, error) {
	return nil, m.err
}
func (m *mockFAQStore) ListFAQItems(_ context.Context, _ string) ([]*model.FAQItem, error) {
	return nil, m.err
}
func (m *mockFAQStore) UpdateFAQItem(_ context.Context, _ *model.FAQItem) error { return m.err }
func (m *mockFAQStore) DeleteFAQItem(_ context.Context, _ string) error         { return m.err }
func (m *mockFAQStore) CountFAQItems(_ context.Context) (int, error)            { return m.count, m.err }

var _ store.FAQStore = (*mockFAQStore)(nil)

// ---------------------------------------------------------------------------
// Mock: ResidentStore
// ---------------------------------------------------------------------------

type mockResidentStore struct {
	count int
	err   error
}

func (m *mockResidentStore) CreateResident(_ context.Context, _ *model.Resident) error { return m.err }
func (m *mockResidentStore) GetResident(_ context.Context, _ string) (*model.Resident, error) {
	return nil, m.err
}
func (m *mockResidentStore) ListResidents(_ context.Context, _ string, _ model.ResidentStatus, _ string) ([]*model.Resident, error) {
	return nil, m.err
}
func (m *mockResidentStore) UpdateResident(_ context.Context, _ *model.Resident) error { return m.err }
func (m *mockResidentStore) DeleteResident(_ context.Context, _ string) error          { return m.err }
func (m *mockResidentStore) CountResidents(_ context.Context) (int, error)             { return m.count, m.err }

var _ store.ResidentStore = (*mockResidentStore)(nil)

// ---------------------------------------------------------------------------
// Тесты ValidateCompleteness
// ---------------------------------------------------------------------------

func TestValidateCompleteness_EmptyDB(t *testing.T) {
	docStore := &mockDocStore{docs: []model.Document{}}
	eventStore := &mockEventStore{count: 0}
	contestStore := &mockContestStore{activeCount: 0}
	faqStore := &mockFAQStore{count: 0}
	residentStore := &mockResidentStore{count: 0}

	report := ValidateCompleteness(docStore, eventStore, contestStore, faqStore, residentStore)

	if len(report.UnparsedDocuments) != 0 {
		t.Errorf("expected 0 unparsed, got %d", len(report.UnparsedDocuments))
	}
	if len(report.UnclassifiedDocuments) != 0 {
		t.Errorf("expected 0 unclassified, got %d", len(report.UnclassifiedDocuments))
	}
	// Пустые источники должны попасть в SourceDBMismatch.
	if len(report.SourceDBMismatch) == 0 {
		t.Error("expected source DB mismatch for empty stores")
	}
}

func TestValidateCompleteness_DocWithoutCategory(t *testing.T) {
	docs := []model.Document{
		{
			ID:       "doc-1",
			Title:    "Valid Title",
			Category: "",
			FileHash: "abc123",
			Status:   model.StatusActive,
		},
	}
	docStore := &mockDocStore{docs: docs}
	eventStore := &mockEventStore{count: 5}
	contestStore := &mockContestStore{activeCount: 2}
	faqStore := &mockFAQStore{count: 10}
	residentStore := &mockResidentStore{count: 3}

	report := ValidateCompleteness(docStore, eventStore, contestStore, faqStore, residentStore)

	if len(report.UnclassifiedDocuments) != 1 {
		t.Errorf("expected 1 unclassified, got %d", len(report.UnclassifiedDocuments))
	}
	if report.UnclassifiedDocuments[0] != "doc-1" {
		t.Errorf("expected doc-1 unclassified, got %s", report.UnclassifiedDocuments[0])
	}
}

func TestValidateCompleteness_DocWithoutFile(t *testing.T) {
	docs := []model.Document{
		{
			ID:        "doc-2",
			Title:     "Has Title",
			Category:  "Правила",
			LocalPath: "",
			FileHash:  "",
			Status:    model.StatusActive,
		},
	}
	docStore := &mockDocStore{docs: docs}
	eventStore := &mockEventStore{count: 1}
	contestStore := &mockContestStore{activeCount: 1}
	faqStore := &mockFAQStore{count: 1}
	residentStore := &mockResidentStore{count: 1}

	report := ValidateCompleteness(docStore, eventStore, contestStore, faqStore, residentStore)

	if len(report.MissingFileDocuments) != 1 {
		t.Errorf("expected 1 missing file, got %d", len(report.MissingFileDocuments))
	}
}

func TestValidateCompleteness_UnparsedDocument(t *testing.T) {
	docs := []model.Document{
		{
			ID:       "doc-3",
			Title:    "",
			Category: "Новости",
			FileHash: "hash1",
			Status:   model.StatusPending,
		},
	}
	docStore := &mockDocStore{docs: docs}
	eventStore := &mockEventStore{count: 1}
	contestStore := &mockContestStore{activeCount: 1}
	faqStore := &mockFAQStore{count: 1}
	residentStore := &mockResidentStore{count: 1}

	report := ValidateCompleteness(docStore, eventStore, contestStore, faqStore, residentStore)

	if len(report.UnparsedDocuments) != 1 {
		t.Errorf("expected 1 unparsed, got %d", len(report.UnparsedDocuments))
	}
}

func TestValidateCompleteness_RecommendationsGenerated(t *testing.T) {
	docs := []model.Document{
		{ID: "d1", Title: "", Category: "", LocalPath: "", FileHash: ""},
	}
	docStore := &mockDocStore{docs: docs}
	eventStore := &mockEventStore{count: 0}
	contestStore := &mockContestStore{activeCount: 0}
	faqStore := &mockFAQStore{count: 0}
	residentStore := &mockResidentStore{count: 0}

	report := ValidateCompleteness(docStore, eventStore, contestStore, faqStore, residentStore)

	if len(report.Recommendations) == 0 {
		t.Error("expected recommendations for problematic documents")
	}
}

func TestValidateCompleteness_NilStores(t *testing.T) {
	docs := []model.Document{
		{ID: "d1", Title: "OK", Category: "Test", FileHash: "h1", Status: model.StatusActive},
	}
	docStore := &mockDocStore{docs: docs}

	// nil-хранилища не должны вызывать панику.
	report := ValidateCompleteness(docStore, nil, nil, nil, nil)

	if len(report.SourceDBMismatch) != 0 {
		t.Errorf("expected 0 mismatches for nil stores, got %d", len(report.SourceDBMismatch))
	}
}

func TestValidateCompleteness_StoreErrors(t *testing.T) {
	docStore := &mockDocStore{docs: []model.Document{}, err: nil}
	eventStore := &mockEventStore{count: 0, err: nil}
	contestStore := &mockContestStore{activeCount: 0, err: nil}
	faqStore := &mockFAQStore{count: 0, err: nil}
	residentStore := &mockResidentStore{count: 0, err: nil}

	report := ValidateCompleteness(docStore, eventStore, contestStore, faqStore, residentStore)

	// Все пустые источники = расхождения.
	if len(report.SourceDBMismatch) == 0 {
		t.Error("expected source DB mismatch when all stores return zero counts")
	}
}

// ---------------------------------------------------------------------------
// Тесты ToMarkdown
// ---------------------------------------------------------------------------

func TestToMarkdown_NotEmpty(t *testing.T) {
	report := CompletenessReport{
		UnparsedDocuments:     []string{"doc-1"},
		UnclassifiedDocuments: []string{"doc-2"},
		MissingFileDocuments:  []string{"doc-3"},
		SourceDBMismatch:      []string{"EventStore: нет записей"},
		Recommendations:       []string{"Fix all issues"},
	}

	md := ToMarkdown(report)

	if md == "" {
		t.Fatal("ToMarkdown returned empty string")
	}
	if !strings.Contains(md, "# Отчёт о полноте базы данных") {
		t.Error("ToMarkdown should contain main heading")
	}
	if !strings.Contains(md, "`doc-1`") {
		t.Error("ToMarkdown should contain document IDs")
	}
	if !strings.Contains(md, "## Рекомендации") {
		t.Error("ToMarkdown should contain recommendations section")
	}
}

func TestToMarkdown_NoProblems(t *testing.T) {
	report := CompletenessReport{
		Recommendations: []string{"База данных полна, проблем не обнаружено"},
	}

	md := ToMarkdown(report)

	if !strings.Contains(md, "_Нет проблем_") {
		t.Error("ToMarkdown should show 'Нет проблем' for empty lists")
	}
}

// ---------------------------------------------------------------------------
// Тесты ToHTML
// ---------------------------------------------------------------------------

func TestToHTML_NotEmpty(t *testing.T) {
	report := CompletenessReport{
		UnparsedDocuments:     []string{"doc-1"},
		UnclassifiedDocuments: []string{"doc-2"},
		SourceDBMismatch:      []string{"FAQStore: нет записей FAQ"},
		Recommendations:       []string{"Проверить парсинг"},
	}

	html := ToHTML(report)

	if html == "" {
		t.Fatal("ToHTML returned empty string")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("ToHTML should produce valid HTML document")
	}
	if !strings.Contains(html, "doc-1") {
		t.Error("ToHTML should contain document IDs")
	}
	if !strings.Contains(html, "Проверить парсинг") {
		t.Error("ToHTML should contain recommendations")
	}
}

func TestToHTML_NoProblems(t *testing.T) {
	report := CompletenessReport{
		Recommendations: []string{"Всё в порядке"},
	}

	html := ToHTML(report)

	if !strings.Contains(html, "Нет проблем") {
		t.Error("ToHTML should show 'Нет проблем' for empty lists")
	}
}

func TestToHTML_Escaping(t *testing.T) {
	report := CompletenessReport{
		UnparsedDocuments: []string{"<script>alert('xss')</script>"},
		Recommendations:   []string{"Fix <b>urgent</b>"},
	}

	html := ToHTML(report)

	if strings.Contains(html, "<script>") {
		t.Error("ToHTML should escape HTML special characters in document IDs")
	}
	if strings.Contains(html, "<b>") {
		t.Error("ToHTML should escape HTML special characters in recommendations")
	}
}

// ---------------------------------------------------------------------------
// Тесты generateRecommendations
// ---------------------------------------------------------------------------

func TestGenerateRecommendations_NoProblems(t *testing.T) {
	report := CompletenessReport{}
	recs := generateRecommendations(report)

	if len(recs) != 1 {
		t.Errorf("expected 1 recommendation for clean report, got %d", len(recs))
	}
	if !strings.Contains(recs[0], "полна") {
		t.Errorf("expected 'полна' in recommendation, got %q", recs[0])
	}
}

func TestGenerateRecommendations_WithProblems(t *testing.T) {
	report := CompletenessReport{
		UnparsedDocuments:     []string{"d1", "d2"},
		UnclassifiedDocuments: []string{"d3"},
	}
	recs := generateRecommendations(report)

	if len(recs) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(recs))
	}
}
