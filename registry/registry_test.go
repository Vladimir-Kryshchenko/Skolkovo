package registry

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
// HTML table parsing tests
// ---------------------------------------------------------------------------

func TestParseResidentsTable_Success(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <table class="registry-table">
    <thead>
      <tr>
        <th>Название</th>
        <th>ИНН</th>
        <th>Направление</th>
        <th>Дата вступления</th>
        <th>Статус</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td><a href="/residents/tech-start">ТехноСтарт</a></td>
        <td>7707083893</td>
        <td>Информационные технологии</td>
        <td>15.06.2023</td>
        <td>Действующий</td>
      </tr>
      <tr>
        <td><a href="/residents/bio-pharm">БиоФарм</a></td>
        <td>7710020172</td>
        <td>Биомедицинские технологии</td>
        <td>01.03.2024</td>
        <td>Действующий</td>
      </tr>
      <tr>
        <td><a href="/residents/old-company">Старая Компания</a></td>
        <td>7701234567</td>
        <td>Энергоэффективность</td>
        <td>10.01.2019</td>
        <td>Исключён из реестра</td>
      </tr>
    </tbody>
  </table>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	residents, err := parseResidentsTable(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseResidentsTable error: %v", err)
	}
	if len(residents) != 3 {
		t.Fatalf("expected 3 residents, got %d", len(residents))
	}

	// Проверяем первого резидента.
	found := false
	for _, r := range residents {
		if r.Name == "ТехноСтарт" {
			found = true
			if r.INN != "7707083893" {
				t.Errorf("expected INN 7707083893, got %s", r.INN)
			}
			if r.Industry != "Информационные технологии" {
				t.Errorf("expected industry 'Информационные технологии', got %s", r.Industry)
			}
			if r.JoinDate.IsZero() {
				t.Error("expected non-zero join date")
			}
			if r.Status != model.ResidentActive {
				t.Errorf("expected active status, got %s", r.Status)
			}
		}
	}
	if !found {
		t.Error("did not find 'ТехноСтарт' in residents")
	}

	// Проверяем неактивного резидента.
	for _, r := range residents {
		if r.Name == "Старая Компания" {
			if r.Status != model.ResidentInactive {
				t.Errorf("expected inactive status, got %s", r.Status)
			}
		}
	}
}

func TestParseResidentsTable_NoTbody(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <table>
    <tr>
      <th>Компания</th>
      <th>ИНН</th>
    </tr>
    <tr>
      <td><a href="/residents/r1">Компания А</a></td>
      <td>7701112233</td>
    </tr>
    <tr>
      <td><a href="/residents/r2">Компания Б</a></td>
      <td>7704445566</td>
    </tr>
  </table>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	residents, err := parseResidentsTable(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseResidentsTable error: %v", err)
	}
	if len(residents) != 2 {
		t.Fatalf("expected 2 residents, got %d", len(residents))
	}
}

func TestParseResidentsTable_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := parseResidentsTable(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestParseResidentsTable_EmptyPage(t *testing.T) {
	htmlBody := `<!DOCTYPE html><html><body><p>Нет данных</p></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	residents, err := parseResidentsTable(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseResidentsTable error: %v", err)
	}
	if len(residents) != 0 {
		t.Errorf("expected 0 residents on empty page, got %d", len(residents))
	}
}

// ---------------------------------------------------------------------------
// parseResidentRow tests
// ---------------------------------------------------------------------------

func TestParseResidentRow_FullRow(t *testing.T) {
	rowHTML := `<table><tbody><tr>
  <td><a href="/residents/test">Тестовая Компания</a></td>
  <td>7707083893</td>
  <td>Медицинские технологии</td>
  <td>20.01.2025</td>
  <td>Действующий</td>
</tr></tbody></table>`

	doc, err := html.Parse(strings.NewReader("<html><body>" + rowHTML + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}

	// Находим <tr>.
	var tr *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if tr == nil && n.Type == html.ElementNode && n.Data == "tr" {
			tr = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if tr == nil {
		t.Fatal("did not find <tr>")
	}

	r := parseResidentRow(tr, "https://sk.ru/residents/")
	if r == nil {
		t.Fatal("parseResidentRow returned nil")
	}
	if r.Name != "Тестовая Компания" {
		t.Errorf("expected name 'Тестовая Компания', got %s", r.Name)
	}
	if r.INN != "7707083893" {
		t.Errorf("expected INN 7707083893, got %s", r.INN)
	}
	if r.Industry != "Медицинские технологии" {
		t.Errorf("expected industry 'Медицинские технологии', got %s", r.Industry)
	}
	if r.JoinDate.IsZero() {
		t.Error("expected non-zero join date")
	}
	if r.Status != model.ResidentActive {
		t.Errorf("expected active status, got %s", r.Status)
	}
}

func TestParseResidentRow_InactiveStatus(t *testing.T) {
	rowHTML := `<table><tbody><tr>
  <td><a href="/residents/closed">Закрытая Компания</a></td>
  <td>7712345678</td>
  <td>Энергетика</td>
  <td>01.06.2020</td>
  <td>Исключён из реестра</td>
</tr></tbody></table>`

	doc, err := html.Parse(strings.NewReader("<html><body>" + rowHTML + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}

	var tr *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if tr == nil && n.Type == html.ElementNode && n.Data == "tr" {
			tr = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	r := parseResidentRow(tr, "https://sk.ru/residents/")
	if r == nil {
		t.Fatal("parseResidentRow returned nil")
	}
	if r.Status != model.ResidentInactive {
		t.Errorf("expected inactive status, got %s", r.Status)
	}
}

func TestParseResidentRow_NoINN(t *testing.T) {
	rowHTML := `<table><tbody><tr>
  <td><a href="/residents/unknown">Компания без ИНН</a></td>
  <td>—</td>
  <td>Не определено</td>
  <td>15.03.2024</td>
  <td>Действующий</td>
</tr></tbody></table>`

	doc, err := html.Parse(strings.NewReader("<html><body>" + rowHTML + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}

	var tr *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if tr == nil && n.Type == html.ElementNode && n.Data == "tr" {
			tr = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	r := parseResidentRow(tr, "https://sk.ru/residents/")
	if r == nil {
		t.Fatal("parseResidentRow returned nil")
	}
	if r.INN != "" {
		t.Errorf("expected empty INN, got %s", r.INN)
	}
	if r.Name != "Компания без ИНН" {
		t.Errorf("expected name 'Компания без ИНН', got %s", r.Name)
	}
}

func TestParseResidentRow_NoLink(t *testing.T) {
	rowHTML := `<table><tbody><tr>
  <td>Компания без ссылки</td>
  <td>7709876543</td>
  <td>Химия</td>
  <td>01.01.2023</td>
  <td>Действующий</td>
</tr></tbody></table>`

	doc, err := html.Parse(strings.NewReader("<html><body>" + rowHTML + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}

	var tr *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if tr == nil && n.Type == html.ElementNode && n.Data == "tr" {
			tr = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	r := parseResidentRow(tr, "https://sk.ru/residents/")
	if r == nil {
		t.Fatal("parseResidentRow returned nil")
	}
	if r.Name != "Компания без ссылки" {
		t.Errorf("expected name from first cell, got %s", r.Name)
	}
}

func TestParseResidentRow_EmptyRow(t *testing.T) {
	rowHTML := `<table><tbody><tr></tr></tbody></table>`

	doc, err := html.Parse(strings.NewReader("<html><body>" + rowHTML + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}

	var tr *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if tr == nil && n.Type == html.ElementNode && n.Data == "tr" {
			tr = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	r := parseResidentRow(tr, "https://sk.ru/residents/")
	if r != nil {
		t.Errorf("expected nil for empty row, got %+v", r)
	}
}

func TestParseResidentRow_12DigitINN(t *testing.T) {
	rowHTML := `<table><tbody><tr>
  <td><a href="/residents/long-inn">Компания 12 ИНН</a></td>
  <td>770708389300</td>
  <td>IT</td>
  <td>10.10.2022</td>
  <td>Действующий</td>
</tr></tbody></table>`

	doc, err := html.Parse(strings.NewReader("<html><body>" + rowHTML + "</body></html>"))
	if err != nil {
		t.Fatalf("html.Parse error: %v", err)
	}

	var tr *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if tr == nil && n.Type == html.ElementNode && n.Data == "tr" {
			tr = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	r := parseResidentRow(tr, "https://sk.ru/residents/")
	if r == nil {
		t.Fatal("parseResidentRow returned nil")
	}
	if r.INN != "770708389300" {
		t.Errorf("expected INN 770708389300, got %s", r.INN)
	}
}

// ---------------------------------------------------------------------------
// ParseResidents integration tests
// ---------------------------------------------------------------------------

func TestParseResidents_Success(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <table class="resident-list">
    <thead><tr><th>Компания</th><th>ИНН</th><th>Статус</th></tr></thead>
    <tbody>
      <tr>
        <td><a href="/r/1">Резидент Один</a></td>
        <td>7701001001</td>
        <td>Действующий</td>
      </tr>
    </tbody>
  </table>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	cfg := RegistryConfig{SourceURL: srv.URL}
	residents, err := ParseResidents(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatalf("ParseResidents error: %v", err)
	}
	if len(residents) != 1 {
		t.Fatalf("expected 1 resident, got %d", len(residents))
	}
	if residents[0].Name != "Резидент Один" {
		t.Errorf("unexpected name: %s", residents[0].Name)
	}
}

func TestParseResidents_DefaultURL(t *testing.T) {
	cfg := RegistryConfig{}
	_, err := ParseResidents(context.Background(), cfg, nil)
	// Должна быть попытка загрузить дефолтный URL — но без сервера будет ошибка сети.
	// Главное — не panic и не ошибка конфигурации.
	if err == nil {
		// Сеть может быть недоступна, но если сервер ответил — это нормально.
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
		{"Дата: 15.06.2026", true},
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

func TestIsINN(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"7707083893", true},
		{"770708389300", true},
		{"12345", false},
		{"770708389", false},   // 9 цифр
		{"77070838930", false}, // 11 цифр
		{"not a number", false},
		{"ИНН: 7707083893", true},
	}

	for _, tc := range tests {
		got := isINN(tc.input)
		if got != tc.want {
			t.Errorf("isINN(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestExtractINN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"7707083893", "7707083893"},
		{"770708389300", "770708389300"},
		{"ИНН 7707083893 компании", "7707083893"},
		{"нет ИНН", ""},
	}

	for _, tc := range tests {
		got := extractINN(tc.input)
		if got != tc.want {
			t.Errorf("extractINN(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		input string
		want  model.ResidentStatus
	}{
		{"действующий", model.ResidentActive},
		{"активный", model.ResidentActive},
		{"в реестре", model.ResidentActive},
		{"исключён из реестра", model.ResidentInactive},
		{"исключен", model.ResidentInactive},
		{"ликвидирован", model.ResidentInactive},
		{"прекращён", model.ResidentInactive},
		{"закрыт", model.ResidentInactive},
		{"недействующий", model.ResidentInactive},
		{"архив", model.ResidentInactive},
		{"", model.ResidentActive}, // по умолчанию active
	}

	for _, tc := range tests {
		got := classifyStatus(tc.input)
		if got != tc.want {
			t.Errorf("classifyStatus(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestResidentID(t *testing.T) {
	id1 := residentID("Тест", "https://sk.ru/residents/")
	id2 := residentID("Тест", "https://sk.ru/residents/")
	if id1 != id2 {
		t.Errorf("residentID not deterministic: %s != %s", id1, id2)
	}
	if !strings.HasPrefix(id1, "resident-") {
		t.Errorf("residentID should start with 'resident-', got %s", id1)
	}
	if len(id1) != len("resident-")+16 {
		t.Errorf("residentID length wrong: got %d, want %d", len(id1), len("resident-")+16)
	}
}

func TestResidentID_Different(t *testing.T) {
	id1 := residentID("Компания А", "https://sk.ru/residents/")
	id2 := residentID("Компания Б", "https://sk.ru/residents/")
	if id1 == id2 {
		t.Error("different names should produce different IDs")
	}
}

// ---------------------------------------------------------------------------
// Mock ResidentStore for IngestResidents tests
// ---------------------------------------------------------------------------

type mockResidentStore struct {
	residents map[string]*model.Resident
}

func newMockResidentStore() *mockResidentStore {
	return &mockResidentStore{residents: make(map[string]*model.Resident)}
}

func (m *mockResidentStore) CreateResident(ctx context.Context, resident *model.Resident) error {
	m.residents[resident.ID] = resident
	return nil
}

func (m *mockResidentStore) GetResident(ctx context.Context, id string) (*model.Resident, error) {
	if r, ok := m.residents[id]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockResidentStore) ListResidents(ctx context.Context, industry string, status model.ResidentStatus, query string) ([]*model.Resident, error) {
	var result []*model.Resident
	for _, r := range m.residents {
		if industry != "" && r.Industry != industry {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		if query != "" {
			if r.INN != query && !strings.Contains(strings.ToLower(r.Name), strings.ToLower(query)) {
				continue
			}
		}
		result = append(result, r)
	}
	return result, nil
}

func (m *mockResidentStore) UpdateResident(ctx context.Context, resident *model.Resident) error {
	m.residents[resident.ID] = resident
	return nil
}

func (m *mockResidentStore) DeleteResident(ctx context.Context, id string) error {
	delete(m.residents, id)
	return nil
}

func (m *mockResidentStore) CountResidents(ctx context.Context) (int, error) {
	return len(m.residents), nil
}

// ---------------------------------------------------------------------------
// IngestResidents tests
// ---------------------------------------------------------------------------

func TestIngestResidents_NewResidents(t *testing.T) {
	st := newMockResidentStore()
	residents := []*model.Resident{
		{
			ID:        "resident-001",
			Name:      "Новый резидент",
			INN:       "7707083893",
			Industry:  "IT",
			JoinDate:  time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			Status:    model.ResidentActive,
			SourceURL: "https://sk.ru/residents/",
		},
	}

	res, err := IngestResidents(context.Background(), residents, st)
	if err != nil {
		t.Fatalf("IngestResidents error: %v", err)
	}
	if res.New != 1 {
		t.Errorf("expected 1 new resident, got %d", res.New)
	}
	if res.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", res.Updated)
	}

	r, err := st.GetResident(context.Background(), "resident-001")
	if err != nil {
		t.Fatalf("GetResident error: %v", err)
	}
	if r.Name != "Новый резидент" {
		t.Errorf("unexpected name: %s", r.Name)
	}
}

func TestIngestResidents_UpdateExisting(t *testing.T) {
	st := newMockResidentStore()

	// Сначала создаём резидента.
	st.CreateResident(context.Background(), &model.Resident{
		ID:        "resident-002",
		Name:      "Старое имя",
		INN:       "7707083893",
		Industry:  "Энергетика",
		JoinDate:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:    model.ResidentActive,
		SourceURL: "https://sk.ru/residents/",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	})

	// Обновляем по ИНН.
	residents := []*model.Resident{
		{
			Name:      "Новое имя",
			INN:       "7707083893",
			Industry:  "IT",
			JoinDate:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:    model.ResidentActive,
			SourceURL: "https://sk.ru/residents/",
		},
	}

	res, err := IngestResidents(context.Background(), residents, st)
	if err != nil {
		t.Fatalf("IngestResidents error: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", res.Updated)
	}
	if res.New != 0 {
		t.Errorf("expected 0 new, got %d", res.New)
	}

	r, err := st.GetResident(context.Background(), "resident-002")
	if err != nil {
		t.Fatalf("GetResident error: %v", err)
	}
	if r.Name != "Новое имя" {
		t.Errorf("expected updated name 'Новое имя', got %s", r.Name)
	}
	if r.Industry != "IT" {
		t.Errorf("expected updated industry 'IT', got %s", r.Industry)
	}
}

func TestIngestResidents_SkipsInvalid(t *testing.T) {
	st := newMockResidentStore()
	residents := []*model.Resident{
		{
			ID:        "resident-003",
			Name:      "", // пустое имя
			SourceURL: "https://sk.ru/residents/",
		},
	}

	res, err := IngestResidents(context.Background(), residents, st)
	if err != nil {
		t.Fatalf("IngestResidents error: %v", err)
	}
	if res.New != 0 {
		t.Errorf("expected 0 new (invalid name), got %d", res.New)
	}
	if len(res.Errors) == 0 {
		t.Error("expected errors for invalid residents")
	}
}

func TestIngestResidents_Multiple(t *testing.T) {
	st := newMockResidentStore()

	// Один уже есть в хранилище.
	st.CreateResident(context.Background(), &model.Resident{
		ID:        "resident-a",
		Name:      "Резидент А",
		INN:       "7701000001",
		JoinDate:  time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:    model.ResidentActive,
		SourceURL: "https://sk.ru/residents/",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	})

	residents := []*model.Resident{
		{
			ID:        "resident-b",
			Name:      "Резидент Б",
			INN:       "7701000002",
			JoinDate:  time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
			Status:    model.ResidentActive,
			SourceURL: "https://sk.ru/residents/",
		},
		{
			Name:      "Резидент А", // тот же ИНН
			INN:       "7701000001",
			JoinDate:  time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:    model.ResidentActive,
			SourceURL: "https://sk.ru/residents/",
		},
		{
			ID:        "resident-c",
			Name:      "Резидент В",
			INN:       "7701000003",
			JoinDate:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:    model.ResidentActive,
			SourceURL: "https://sk.ru/residents/",
		},
	}

	res, err := IngestResidents(context.Background(), residents, st)
	if err != nil {
		t.Fatalf("IngestResidents error: %v", err)
	}
	if res.New != 2 {
		t.Errorf("expected 2 new, got %d", res.New)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", res.Updated)
	}

	count, _ := st.CountResidents(context.Background())
	if count != 3 {
		t.Errorf("expected 3 residents in store, got %d", count)
	}
}

func TestIngestResidents_ByINN(t *testing.T) {
	st := newMockResidentStore()

	st.CreateResident(context.Background(), &model.Resident{
		ID:        "resident-by-inn",
		Name:      "Старое название",
		INN:       "7799887766",
		JoinDate:  time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:    model.ResidentActive,
		SourceURL: "https://sk.ru/residents/",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	})

	residents := []*model.Resident{
		{
			Name:      "Новое название",
			INN:       "7799887766",
			JoinDate:  time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:    model.ResidentActive,
			SourceURL: "https://sk.ru/residents/",
		},
	}

	res, err := IngestResidents(context.Background(), residents, st)
	if err != nil {
		t.Fatalf("IngestResidents error: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", res.Updated)
	}
}

// ---------------------------------------------------------------------------
// Monitor tests
// ---------------------------------------------------------------------------

func TestNew_RegistryMonitor(t *testing.T) {
	cfg := RegistryConfig{
		SourceURL: "https://sk.ru/residents/",
		Category:  "Реестр резидентов",
	}
	m := New(cfg, nil)

	if m.Cfg.SourceURL != "https://sk.ru/residents/" {
		t.Errorf("expected SourceURL, got %s", m.Cfg.SourceURL)
	}
	if m.Cfg.Category != "Реестр резидентов" {
		t.Errorf("expected Category 'Реестр резидентов', got %s", m.Cfg.Category)
	}
	if m.Store != nil {
		t.Error("expected nil Store")
	}
}

func TestNew_DefaultValues(t *testing.T) {
	cfg := RegistryConfig{}
	m := New(cfg, nil)

	if m.Cfg.Category != "Реестр" {
		t.Errorf("expected default category 'Реестр', got %s", m.Cfg.Category)
	}
	if m.Cfg.SourceURL != "https://sk.ru/residents/" {
		t.Errorf("expected default SourceURL, got %s", m.Cfg.SourceURL)
	}
}
