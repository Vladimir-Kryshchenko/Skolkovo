package contests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"

	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// HTML parsing tests
// ---------------------------------------------------------------------------

func TestParseContestsFromPage_Cards(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <div class="contest-card">
    <h3 class="contest-title"><a href="/contests/innovation-grant-2026">Грант на инновации 2026</a></h3>
    <div class="contest-date-start">01.06.2026</div>
    <div class="contest-date-end">30.09.2026</div>
    <div class="contest-requirements">Стартапы в сфере AI</div>
    <div class="contest-prize">5 000 000 рублей</div>
    <div class="contest-description">Грант для инновационных проектов</div>
  </div>
  <div class="contest-card">
    <h3 class="contest-title"><a href="/contests/biotech-contest">Конкурс Биотех</a></h3>
    <div class="contest-date-start">01.01.2024</div>
    <div class="contest-date-end">01.03.2024</div>
    <div class="contest-description">Завершённый конкурс</div>
  </div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	contests, err := parseContestsFromPage(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseContestsFromPage error: %v", err)
	}
	if len(contests) < 1 {
		t.Fatalf("expected at least 1 contest, got %d", len(contests))
	}

	// Проверяем, что первый конкурс найден.
	found := false
	for _, c := range contests {
		if c.Title == "Грант на инновации 2026" {
			found = true
			if c.Status != model.ContestActive {
				t.Errorf("expected active status, got %s", c.Status)
			}
			if c.Prize != "5 000 000 рублей" {
				t.Errorf("expected prize '5 000 000 рублей', got '%s'", c.Prize)
			}
			if c.Requirements != "Стартапы в сфере AI" {
				t.Errorf("expected requirements 'Стартапы в сфере AI', got '%s'", c.Requirements)
			}
		}
	}
	if !found {
		t.Error("did not find 'Грант на инновации 2026' in contests")
	}
}

func TestParseContestsFromPage_ClosedStatus(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <div class="contest-card">
    <h3><a href="/contests/old-contest">Старый конкурс</a></h3>
    <div class="contest-date-end">01.01.2020</div>
  </div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	contests, err := parseContestsFromPage(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseContestsFromPage error: %v", err)
	}
	if len(contests) < 1 {
		t.Fatalf("expected at least 1 contest, got %d", len(contests))
	}

	if contests[0].Status != model.ContestClosed {
		t.Errorf("expected closed status for past contest, got %s", contests[0].Status)
	}
}

func TestParseContestsFromPage_LinksFallback(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <ul class="contests-list">
    <li>
      <a href="/contests/grant-ai">Грант AI</a>
      <span class="date">20.03.2026</span>
    </li>
    <li>
      <a href="/grants/biotech">Грант Биотех</a>
      <span class="date">10.10.2023</span>
    </li>
  </ul>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	contests, err := parseContestsFromPage(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseContestsFromPage error: %v", err)
	}
	// Должен найти хотя бы 1 конкурс через fallback.
	if len(contests) < 1 {
		t.Fatalf("expected at least 1 contest from links fallback, got %d", len(contests))
	}
}

func TestParseContestsFromPage_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := parseContestsFromPage(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ParseContests integration tests
// ---------------------------------------------------------------------------

func TestParseContests_BothURLs(t *testing.T) {
	contestsBody := `<html><body>
  <div class="contest-card"><h3><a href="/contests/c1">Конкурс 1</a></h3>
    <div class="contest-date-end">31.12.2026</div></div>
</body></html>`
	contestsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(contestsBody))
	}))
	defer contestsSrv.Close()

	grantsBody := `<html><body>
  <div class="contest-card"><h3><a href="/grants/g1">Грант 1</a></h3>
    <div class="contest-date-end">31.12.2026</div></div>
</body></html>`
	grantsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(grantsBody))
	}))
	defer grantsSrv.Close()

	cfg := ContestsConfig{ContestsURL: contestsSrv.URL, GrantsURL: grantsSrv.URL}
	contests, err := ParseContests(context.Background(), cfg, http.DefaultClient)
	if err != nil {
		t.Fatalf("ParseContests error: %v", err)
	}
	if len(contests) != 2 {
		t.Errorf("expected 2 contests (1 from each URL), got %d", len(contests))
	}
}

func TestParseContests_FallbackWhenOneFails(t *testing.T) {
	// Конкурсы — ошибка.
	contestsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer contestsSrv.Close()

	// Гранты — успешный ответ.
	grantsBody := `<html><body>
  <div class="contest-card"><h3><a href="/grants/g1">Грант 1</a></h3></div>
</body></html>`
	grantsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(grantsBody))
	}))
	defer grantsSrv.Close()

	cfg := ContestsConfig{ContestsURL: contestsSrv.URL, GrantsURL: grantsSrv.URL}
	contests, err := ParseContests(context.Background(), cfg, http.DefaultClient)
	if err != nil {
		t.Fatalf("ParseContests error: %v", err)
	}
	if len(contests) < 1 {
		t.Fatalf("expected at least 1 contest from grants fallback, got %d", len(contests))
	}
}

func TestParseContests_NoConfig(t *testing.T) {
	cfg := ContestsConfig{}
	_, err := ParseContests(context.Background(), cfg, http.DefaultClient)
	if err == nil {
		t.Fatal("expected error when no URLs configured")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestParseDate(t *testing.T) {
	tests := []struct {
		input string
		want  bool // true = не zero time
	}{
		{"15.06.2026", true},
		{"15/06/2026", true},
		{"2026-06-15", true},
		{"", false},
		{"not a date", false},
		{"Event date: 15.06.2026 at 10:00", true},
	}

	for _, tc := range tests {
		got := parseDate(tc.input)
		if tc.want && got.IsZero() {
			t.Errorf("parseDate(%q) = zero, want non-zero", tc.input)
		}
		if !tc.want && !got.IsZero() {
			t.Errorf("parseDate(%q) = %v, want zero", tc.input, got)
		}
	}
}

func TestIsContestURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://sk.ru/contests/grant", true},
		{"https://sk.ru/grants/biotech", true},
		{"https://sk.ru/konkurs/1", true},
		{"https://sk.ru/granty/2", true},
		{"https://sk.ru/docs/doc1", false},
		{"https://sk.ru/news/news1", false},
	}

	for _, tc := range tests {
		got := isContestURL(tc.url)
		if got != tc.want {
			t.Errorf("isContestURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestContestID(t *testing.T) {
	id1 := contestID("https://sk.ru/contests/test")
	id2 := contestID("https://sk.ru/contests/test")
	if id1 != id2 {
		t.Errorf("contestID not deterministic: %s != %s", id1, id2)
	}
	if !strings.HasPrefix(id1, "contest-") {
		t.Errorf("contestID should start with 'contest-', got %s", id1)
	}
	if len(id1) != len("contest-")+16 {
		t.Errorf("contestID length wrong: got %d, want %d", len(id1), len("contest-")+16)
	}
}

func TestContestID_Different(t *testing.T) {
	id1 := contestID("https://sk.ru/contests/contest1")
	id2 := contestID("https://sk.ru/contests/contest2")
	if id1 == id2 {
		t.Error("different URLs should produce different IDs")
	}
}

func TestFindDateInText(t *testing.T) {
	text := "Конкурс проводится с 15.06.2026 по 30.09.2026"
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}
	first := findDateInText(doc)
	if first != "15.06.2026" {
		t.Errorf("findDateInText = %q, want %q", first, "15.06.2026")
	}
}

func TestFindSecondDateInText(t *testing.T) {
	text := "Конкурс проводится с 15.06.2026 по 30.09.2026"
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}
	second := findSecondDateInText(doc)
	if second != "30.09.2026" {
		t.Errorf("findSecondDateInText = %q, want %q", second, "30.09.2026")
	}
}

// ---------------------------------------------------------------------------
// Mock ContestStore for IngestContests tests
// ---------------------------------------------------------------------------

type mockContestStore struct {
	contests map[string]*model.Contest
}

func newMockContestStore() *mockContestStore {
	return &mockContestStore{contests: make(map[string]*model.Contest)}
}

func (m *mockContestStore) CreateContest(ctx context.Context, contest *model.Contest) error {
	m.contests[contest.ID] = contest
	return nil
}

func (m *mockContestStore) GetContest(ctx context.Context, id string) (*model.Contest, error) {
	if c, ok := m.contests[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockContestStore) ListContests(ctx context.Context, category string, status model.ContestStatus) ([]*model.Contest, error) {
	var result []*model.Contest
	for _, c := range m.contests {
		if category != "" && c.Category != category {
			continue
		}
		if status != "" && c.Status != status {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (m *mockContestStore) UpdateContest(ctx context.Context, contest *model.Contest) error {
	m.contests[contest.ID] = contest
	return nil
}

func (m *mockContestStore) DeleteContest(ctx context.Context, id string) error {
	delete(m.contests, id)
	return nil
}

func (m *mockContestStore) CountActiveContests(ctx context.Context) (int, error) {
	count := 0
	for _, c := range m.contests {
		if c.Status == model.ContestActive {
			count++
		}
	}
	return count, nil
}

func TestIngestContests_NewContests(t *testing.T) {
	st := newMockContestStore()
	contests := []*model.Contest{
		{
			ID:        "contest-001",
			Title:     "Новый конкурс",
			StartDate: time.Now(),
			EndDate:   time.Now().Add(30 * 24 * time.Hour),
			SourceURL: "https://sk.ru/contests/new",
			Status:    model.ContestActive,
			Category:  "Конкурсы",
		},
	}

	res, err := IngestContests(context.Background(), contests, st, nil)
	if err != nil {
		t.Fatalf("IngestContests error: %v", err)
	}
	if res.New != 1 {
		t.Errorf("expected 1 new contest, got %d", res.New)
	}
	if res.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", res.Updated)
	}

	// Проверяем, что конкурс сохранён.
	c, err := st.GetContest(context.Background(), "contest-001")
	if err != nil {
		t.Fatalf("GetContest error: %v", err)
	}
	if c.Title != "Новый конкурс" {
		t.Errorf("unexpected title: %s", c.Title)
	}
}

func TestIngestContests_UpdateExisting(t *testing.T) {
	st := newMockContestStore()

	// Сначала создаём конкурс.
	st.CreateContest(context.Background(), &model.Contest{
		ID:        "contest-002",
		Title:     "Старое название",
		StartDate: time.Now(),
		EndDate:   time.Now().Add(30 * 24 * time.Hour),
		SourceURL: "https://sk.ru/contests/existing",
		Status:    model.ContestActive,
		Category:  "Конкурсы",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	})

	// Теперь обновляем.
	contests := []*model.Contest{
		{
			ID:        "contest-002",
			Title:     "Новое название",
			StartDate: time.Now(),
			EndDate:   time.Now().Add(30 * 24 * time.Hour),
			SourceURL: "https://sk.ru/contests/existing",
			Status:    model.ContestActive,
			Category:  "Конкурсы",
		},
	}

	res, err := IngestContests(context.Background(), contests, st, nil)
	if err != nil {
		t.Fatalf("IngestContests error: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", res.Updated)
	}

	c, err := st.GetContest(context.Background(), "contest-002")
	if err != nil {
		t.Fatalf("GetContest error: %v", err)
	}
	if c.Title != "Новое название" {
		t.Errorf("expected updated title, got %s", c.Title)
	}
}

func TestIngestContests_SkipsInvalid(t *testing.T) {
	st := newMockContestStore()
	contests := []*model.Contest{
		{
			ID:        "contest-003",
			Title:     "", // пустой заголовок
			SourceURL: "https://sk.ru/contests/invalid",
		},
	}

	res, err := IngestContests(context.Background(), contests, st, nil)
	if err != nil {
		t.Fatalf("IngestContests error: %v", err)
	}
	if res.New != 0 {
		t.Errorf("expected 0 new (invalid title), got %d", res.New)
	}
	if len(res.Errors) == 0 {
		t.Error("expected errors for invalid contests")
	}
}

func TestIngestContests_Multiple(t *testing.T) {
	st := newMockContestStore()
	contests := []*model.Contest{
		{
			ID:        "contest-a",
			Title:     "Contest A",
			StartDate: time.Now(),
			EndDate:   time.Now().Add(30 * 24 * time.Hour),
			SourceURL: "https://sk.ru/contests/a",
			Status:    model.ContestActive,
		},
		{
			ID:        "contest-b",
			Title:     "Contest B",
			StartDate: time.Now().Add(-60 * 24 * time.Hour),
			EndDate:   time.Now().Add(-30 * 24 * time.Hour),
			SourceURL: "https://sk.ru/contests/b",
			Status:    model.ContestClosed,
		},
		{
			ID:        "contest-c",
			Title:     "Contest C",
			StartDate: time.Now(),
			EndDate:   time.Now().Add(60 * 24 * time.Hour),
			SourceURL: "https://sk.ru/contests/c",
			Status:    model.ContestActive,
		},
	}

	res, err := IngestContests(context.Background(), contests, st, nil)
	if err != nil {
		t.Fatalf("IngestContests error: %v", err)
	}
	if res.New != 3 {
		t.Errorf("expected 3 new, got %d", res.New)
	}
	if res.Fetched != 3 {
		t.Errorf("expected 3 fetched, got %d", res.Fetched)
	}

	count, _ := st.CountActiveContests(context.Background())
	if count != 2 {
		t.Errorf("expected 2 active contests, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Monitor tests
// ---------------------------------------------------------------------------

func TestNew_ContestsMonitor(t *testing.T) {
	cfg := ContestsConfig{
		ContestsURL: "https://sk.ru/contests",
		GrantsURL:   "https://sk.ru/grants",
		Category:    "Конкурсы и гранты",
	}
	m := New(cfg, nil, nil)

	if m.Cfg.ContestsURL != "https://sk.ru/contests" {
		t.Errorf("expected ContestsURL, got %s", m.Cfg.ContestsURL)
	}
	if m.Cfg.GrantsURL != "https://sk.ru/grants" {
		t.Errorf("expected GrantsURL, got %s", m.Cfg.GrantsURL)
	}
	if m.Cfg.Category != "Конкурсы и гранты" {
		t.Errorf("expected Category, got %s", m.Cfg.Category)
	}
	if m.Store != nil {
		t.Error("expected nil Store")
	}
	if m.Rag != nil {
		t.Error("expected nil Rag")
	}
}

func TestNew_DefaultCategory(t *testing.T) {
	cfg := ContestsConfig{ContestsURL: "https://sk.ru/contests"}
	m := New(cfg, nil, nil)

	if m.Cfg.Category != "Конкурсы" {
		t.Errorf("expected default category 'Конкурсы', got %s", m.Cfg.Category)
	}
}
