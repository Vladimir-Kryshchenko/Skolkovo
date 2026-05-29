package events

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
)

// ---------------------------------------------------------------------------
// RSS parsing tests
// ---------------------------------------------------------------------------

func TestParseEventsFromRSS_Success(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Мероприятия Сколково</title>
    <item>
      <title>Форум инноваций 2026</title>
      <link>https://sk.ru/events/innovations-forum-2026</link>
      <description>Ежегодный форум инноваций в Сколково</description>
      <pubDate>Mon, 15 Jun 2026 10:00:00 +0300</pubDate>
    </item>
    <item>
      <title>Хакатон AI</title>
      <link>https://sk.ru/events/ai-hackathon</link>
      <description>Хакатон по искусственному интеллекту</description>
      <pubDate>Mon, 01 Jan 2024 09:00:00 +0300</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	events, err := parseEventsFromRSS(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseEventsFromRSS error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Первое мероприятие — будущее (Active).
	if events[0].Title != "Форум инноваций 2026" {
		t.Errorf("unexpected title: %s", events[0].Title)
	}
	if events[0].Status != model.EventActive {
		t.Errorf("expected status active for future event, got %s", events[0].Status)
	}
	if !strings.Contains(events[0].SourceURL, "innovations-forum-2026") {
		t.Errorf("unexpected source URL: %s", events[0].SourceURL)
	}

	// Второе мероприятие — прошлое (Past).
	if events[1].Status != model.EventPast {
		t.Errorf("expected status past for past event, got %s", events[1].Status)
	}
}

func TestParseEventsFromRSS_EmptyItems(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Мероприятия</title>
    <item>
      <title></title>
      <link>https://sk.ru/events/empty</link>
    </item>
    <item>
      <title>Valid Event</title>
      <link></link>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	events, err := parseEventsFromRSS(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events (both invalid), got %d", len(events))
	}
}

func TestParseEventsFromRSS_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := parseEventsFromRSS(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HTML parsing tests
// ---------------------------------------------------------------------------

func TestParseEventsFromHTML_Cards(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <div class="event-card">
    <h3 class="event-title"><a href="/events/forum-2026">Форум инноваций 2026</a></h3>
    <div class="event-date">15.06.2026</div>
    <div class="event-location">Москва, Сколково</div>
    <div class="event-description">Ежегодный форум инноваций</div>
  </div>
  <div class="event-card">
    <h3 class="event-title"><a href="/events/workshop-ai">Воркшоп AI</a></h3>
    <div class="event-date">01.01.2024</div>
    <div class="event-location">Онлайн</div>
    <div class="event-description">Воркшоп по AI</div>
  </div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	events, err := parseEventsFromHTML(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseEventsFromHTML error: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}

	// Проверяем, что первое мероприятие найдено.
	found := false
	for _, ev := range events {
		if ev.Title == "Форум инноваций 2026" {
			found = true
			if ev.Status != model.EventActive {
				t.Errorf("expected active status, got %s", ev.Status)
			}
			if ev.Location != "Москва, Сколково" {
				t.Errorf("expected location 'Москва, Сколково', got '%s'", ev.Location)
			}
		}
	}
	if !found {
		t.Error("did not find 'Форум инноваций 2026' in events")
	}
}

func TestParseEventsFromHTML_LinksFallback(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <ul class="events-list">
    <li>
      <a href="/events/event-1">Конференция по биотеху</a>
      <span class="date">20.03.2026</span>
    </li>
    <li>
      <a href="/events/event-2">Семинар по робототехнике</a>
      <span class="date">10.10.2023</span>
    </li>
  </ul>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	events, err := parseEventsFromHTML(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseEventsFromHTML error: %v", err)
	}
	// Должен найти хотя бы 1 мероприятие.
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}
}

func TestParseEventsFromHTML_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := parseEventsFromHTML(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

// ---------------------------------------------------------------------------
// ParseEvents integration tests
// ---------------------------------------------------------------------------

func TestParseEvents_PrefersRSS(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item>
    <title>RSS Event</title>
    <link>https://sk.ru/events/rss-event</link>
    <pubDate>Mon, 15 Jun 2026 10:00:00 +0300</pubDate>
  </item>
</channel></rss>`

	rssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssBody))
	}))
	defer rssSrv.Close()

	htmlBody := `<html><body><div class="event-card"><h3><a href="/events/html">HTML Event</a></h3></div></body></html>`
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer htmlSrv.Close()

	cfg := EventsConfig{RSSURL: rssSrv.URL, SourceURL: htmlSrv.URL}
	events, err := ParseEvents(context.Background(), cfg, http.DefaultClient)
	if err != nil {
		t.Fatalf("ParseEvents error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event from RSS, got %d", len(events))
	}
	if events[0].Title != "RSS Event" {
		t.Errorf("expected 'RSS Event', got '%s'", events[0].Title)
	}
}

func TestParseEvents_FallbackToHTML(t *testing.T) {
	// RSS сервер возвращающий ошибку.
	rssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer rssSrv.Close()

	htmlBody := `<html><body>
  <div class="event-card">
    <h3><a href="/events/test">Тестовое мероприятие</a></h3>
    <div class="event-date">15.06.2026</div>
  </div>
</body></html>`
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer htmlSrv.Close()

	cfg := EventsConfig{RSSURL: rssSrv.URL, SourceURL: htmlSrv.URL}
	events, err := ParseEvents(context.Background(), cfg, http.DefaultClient)
	if err != nil {
		t.Fatalf("ParseEvents error: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event from HTML fallback, got %d", len(events))
	}
}

func TestParseEvents_NoConfig(t *testing.T) {
	cfg := EventsConfig{}
	_, err := ParseEvents(context.Background(), cfg, http.DefaultClient)
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

func TestIsEventURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://sk.ru/events/forum", true},
		{"https://sk.ru/events/meropriyatie-1", true},
		{"https://sk.ru/docs/doc1", false},
		{"https://sk.ru/news/news1", false},
		{"https://sk.ru/events/2026/test", true},
	}

	for _, tc := range tests {
		got := isEventURL(tc.url)
		if got != tc.want {
			t.Errorf("isEventURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestEventID(t *testing.T) {
	id1 := eventID("https://sk.ru/events/test")
	id2 := eventID("https://sk.ru/events/test")
	if id1 != id2 {
		t.Errorf("eventID not deterministic: %s != %s", id1, id2)
	}
	if !strings.HasPrefix(id1, "event-") {
		t.Errorf("eventID should start with 'event-', got %s", id1)
	}
	if len(id1) != len("event-")+16 {
		t.Errorf("eventID length wrong: got %d, want %d", len(id1), len("event-")+16)
	}
}

func TestEventID_Different(t *testing.T) {
	id1 := eventID("https://sk.ru/events/event1")
	id2 := eventID("https://sk.ru/events/event2")
	if id1 == id2 {
		t.Error("different URLs should produce different IDs")
	}
}

// ---------------------------------------------------------------------------
// Date comparison tests
// ---------------------------------------------------------------------------

func TestEventStatusAssignment(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	past := time.Now().Add(-30 * 24 * time.Hour)

	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item>
    <title>Future Event</title>
    <link>https://sk.ru/events/future</link>
    <pubDate>` + future.Format(time.RFC1123Z) + `</pubDate>
  </item>
  <item>
    <title>Past Event</title>
    <link>https://sk.ru/events/past</link>
    <pubDate>` + past.Format(time.RFC1123Z) + `</pubDate>
  </item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	events, err := parseEventsFromRSS(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("parseEventsFromRSS error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Status != model.EventActive {
		t.Errorf("future event should be active, got %s", events[0].Status)
	}
	if events[1].Status != model.EventPast {
		t.Errorf("past event should be past, got %s", events[1].Status)
	}
}

// ---------------------------------------------------------------------------
// Mock EventStore for IngestEvents tests
// ---------------------------------------------------------------------------

type mockEventStore struct {
	events map[string]*model.Event
}

func newMockEventStore() *mockEventStore {
	return &mockEventStore{events: make(map[string]*model.Event)}
}

func (m *mockEventStore) CreateEvent(ctx context.Context, event *model.Event) error {
	m.events[event.ID] = event
	return nil
}

func (m *mockEventStore) GetEvent(ctx context.Context, id string) (*model.Event, error) {
	if ev, ok := m.events[id]; ok {
		return ev, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockEventStore) ListEvents(ctx context.Context, category string, status model.EventStatus, dateFrom, dateTo *time.Time) ([]*model.Event, error) {
	var result []*model.Event
	for _, ev := range m.events {
		if category != "" && ev.Category != category {
			continue
		}
		if status != "" && ev.Status != status {
			continue
		}
		if dateFrom != nil && ev.EventDate.Before(*dateFrom) {
			continue
		}
		if dateTo != nil && ev.EventDate.After(*dateTo) {
			continue
		}
		result = append(result, ev)
	}
	return result, nil
}

func (m *mockEventStore) UpdateEvent(ctx context.Context, event *model.Event) error {
	m.events[event.ID] = event
	return nil
}

func (m *mockEventStore) DeleteEvent(ctx context.Context, id string) error {
	delete(m.events, id)
	return nil
}

func (m *mockEventStore) CountEvents(ctx context.Context) (int, error) {
	return len(m.events), nil
}

func TestIngestEvents_NewEvents(t *testing.T) {
	st := newMockEventStore()
	events := []*model.Event{
		{
			ID:        "event-001",
			Title:     "Новое мероприятие",
			EventDate: time.Now().Add(7 * 24 * time.Hour),
			SourceURL: "https://sk.ru/events/new",
			Status:    model.EventActive,
			Category:  "Мероприятия",
		},
	}

	res, err := IngestEvents(context.Background(), events, st, nil)
	if err != nil {
		t.Fatalf("IngestEvents error: %v", err)
	}
	if res.New != 1 {
		t.Errorf("expected 1 new event, got %d", res.New)
	}
	if res.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", res.Updated)
	}

	// Проверяем, что мероприятие сохранено.
	ev, err := st.GetEvent(context.Background(), "event-001")
	if err != nil {
		t.Fatalf("GetEvent error: %v", err)
	}
	if ev.Title != "Новое мероприятие" {
		t.Errorf("unexpected title: %s", ev.Title)
	}
}

func TestIngestEvents_UpdateExisting(t *testing.T) {
	st := newMockEventStore()

	// Сначала создаём мероприятие.
	st.CreateEvent(context.Background(), &model.Event{
		ID:        "event-002",
		Title:     "Старое название",
		EventDate: time.Now().Add(7 * 24 * time.Hour),
		SourceURL: "https://sk.ru/events/existing",
		Status:    model.EventActive,
		Category:  "Мероприятия",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	})

	// Теперь обновляем.
	events := []*model.Event{
		{
			ID:        "event-002",
			Title:     "Новое название",
			EventDate: time.Now().Add(7 * 24 * time.Hour),
			SourceURL: "https://sk.ru/events/existing",
			Status:    model.EventActive,
			Category:  "Мероприятия",
		},
	}

	res, err := IngestEvents(context.Background(), events, st, nil)
	if err != nil {
		t.Fatalf("IngestEvents error: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", res.Updated)
	}

	ev, err := st.GetEvent(context.Background(), "event-002")
	if err != nil {
		t.Fatalf("GetEvent error: %v", err)
	}
	if ev.Title != "Новое название" {
		t.Errorf("expected updated title, got %s", ev.Title)
	}
}

func TestIngestEvents_SkipsInvalid(t *testing.T) {
	st := newMockEventStore()
	events := []*model.Event{
		{
			ID:        "event-003",
			Title:     "", // пустой заголовок
			SourceURL: "https://sk.ru/events/invalid",
		},
	}

	res, err := IngestEvents(context.Background(), events, st, nil)
	if err != nil {
		t.Fatalf("IngestEvents error: %v", err)
	}
	if res.New != 0 {
		t.Errorf("expected 0 new (invalid title), got %d", res.New)
	}
	if len(res.Errors) == 0 {
		t.Error("expected errors for invalid events")
	}
}

func TestIngestEvents_Multiple(t *testing.T) {
	st := newMockEventStore()
	events := []*model.Event{
		{
			ID:        "event-a",
			Title:     "Event A",
			EventDate: time.Now().Add(7 * 24 * time.Hour),
			SourceURL: "https://sk.ru/events/a",
			Status:    model.EventActive,
		},
		{
			ID:        "event-b",
			Title:     "Event B",
			EventDate: time.Now().Add(-7 * 24 * time.Hour),
			SourceURL: "https://sk.ru/events/b",
			Status:    model.EventPast,
		},
		{
			ID:        "event-c",
			Title:     "Event C",
			EventDate: time.Now().Add(14 * 24 * time.Hour),
			SourceURL: "https://sk.ru/events/c",
			Status:    model.EventActive,
		},
	}

	res, err := IngestEvents(context.Background(), events, st, nil)
	if err != nil {
		t.Fatalf("IngestEvents error: %v", err)
	}
	if res.New != 3 {
		t.Errorf("expected 3 new, got %d", res.New)
	}
	if res.Fetched != 3 {
		t.Errorf("expected 3 fetched, got %d", res.Fetched)
	}

	count, _ := st.CountEvents(context.Background())
	if count != 3 {
		t.Errorf("expected 3 events in store, got %d", count)
	}
}
