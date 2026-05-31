package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
)

// ---------------------------------------------------------------------------
// RSS fetching tests
// ---------------------------------------------------------------------------

func TestFetchFromRSS_Success(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Skolkovo Telegram</title>
    <item>
      <title>Новый акселератор для стартапов</title>
      <link>https://t.me/skolkovo/123</link>
      <description>Сколково запускает новую программу акселерации для технологических стартапов.</description>
      <pubDate>Mon, 15 Jun 2026 10:00:00 +0300</pubDate>
    </item>
    <item>
      <title>Гранты до 30 млн рублей</title>
      <link>https://t.me/skolkovo/124</link>
      <description>Открыт приём заявок на гранты для резидентов.</description>
      <pubDate>Sun, 14 Jun 2026 09:00:00 +0300</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	posts, err := fetchFromRSS(context.Background(), srv.URL, "skolkovo", srv.Client())
	if err != nil {
		t.Fatalf("fetchFromRSS error: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}

	if posts[0].Channel != "@skolkovo" {
		t.Errorf("expected channel '@skolkovo', got '%s'", posts[0].Channel)
	}
	if posts[0].PublishedAt.IsZero() {
		t.Error("expected non-zero published date")
	}
	if !strings.HasPrefix(posts[0].ID, "tg-") {
		t.Errorf("ID should start with 'tg-', got %s", posts[0].ID)
	}
}

func TestFetchFromRSS_EmptyItems(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Empty Channel</title>
    <item>
      <title></title>
      <link></link>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	posts, err := fetchFromRSS(context.Background(), srv.URL, "skolkovo", srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(posts) != 0 {
		t.Errorf("expected 0 posts (both empty), got %d", len(posts))
	}
}

func TestFetchFromRSS_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchFromRSS(context.Background(), srv.URL, "skolkovo", srv.Client())
	if err == nil {
		t.Fatal("expected error for 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// FetchChannelPosts tests
// ---------------------------------------------------------------------------

func TestFetchChannelPosts_ViaRSS(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item>
    <title>Пост из канала</title>
    <link>https://t.me/testchannel/1</link>
    <description>Описание поста</description>
    <pubDate>Mon, 15 Jun 2026 10:00:00 +0300</pubDate>
  </item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, что запрос идёт на rsshub URL.
		if strings.Contains(r.URL.Path, "telegram") {
			w.Write([]byte(rssBody))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	posts, err := FetchChannelPosts(context.Background(), "testchannel", srv.URL+"/telegram/channel/", srv.Client())
	if err != nil {
		t.Fatalf("FetchChannelPosts error: %v", err)
	}
	// RSS должен вернуть хотя бы 1 пост.
	if len(posts) < 1 {
		t.Errorf("expected at least 1 post, got %d", len(posts))
	}
}

func TestFetchChannelPosts_NoStubOnFailure(t *testing.T) {
	// Сервер, который всегда возвращает 404 — RSS недоступен.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	posts, err := FetchChannelPosts(context.Background(), "testchannel", srv.URL, srv.Client())
	// При недоступности RSS возвращаем ошибку и НЕ создаём постов-заглушек,
	// чтобы не засорять базу знаний плейсхолдерами.
	if err == nil {
		t.Fatal("expected error when RSS is unavailable")
	}
	if len(posts) != 0 {
		t.Fatalf("expected no posts on failure, got %d", len(posts))
	}
}

func TestFetchChannelPosts_EmptyChannel(t *testing.T) {
	_, err := FetchChannelPosts(context.Background(), "", "", http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for empty channel")
	}
}

// ---------------------------------------------------------------------------
// FetchAllChannels tests
// ---------------------------------------------------------------------------

func TestFetchAllChannels_Multiple(t *testing.T) {
	rssBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item>
    <title>Post</title>
    <link>https://t.me/ch/1</link>
    <pubDate>Mon, 15 Jun 2026 10:00:00 +0300</pubDate>
  </item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	cfg := TelegramConfig{
		Channels: []string{"channel1", "channel2"},
		APIURL:   srv.URL,
	}
	posts, err := FetchAllChannels(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatalf("FetchAllChannels error: %v", err)
	}
	if len(posts) != 2 {
		t.Errorf("expected 2 posts (1 per channel), got %d", len(posts))
	}
}

func TestFetchAllChannels_SkipsFailed(t *testing.T) {
	// Сервер, который возвращает 404 для одного канала и OK для другого.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><title>OK Post</title><link>https://t.me/ch/1</link></item>
</channel></rss>`))
		}
	}))
	defer srv.Close()

	cfg := TelegramConfig{
		Channels: []string{"bad_channel", "good_channel"},
		APIURL:   srv.URL,
	}
	posts, err := FetchAllChannels(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatalf("FetchAllChannels error: %v", err)
	}
	// Должен получить пост только из good_channel (bad_channel fallback на stub).
	if len(posts) < 1 {
		t.Errorf("expected at least 1 post, got %d", len(posts))
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestBuildRSSURL(t *testing.T) {
	tests := []struct {
		channel string
		apiURL  string
		want    string
	}{
		{"skolkovo", "https://rsshub.app/telegram/channel/", "https://rsshub.app/telegram/channel/skolkovo"},
		{"@skolkovo", "https://rsshub.app/telegram/channel/", "https://rsshub.app/telegram/channel/skolkovo"},
		{"skolkovo", "https://custom.rss/api/", "https://custom.rss/api/skolkovo"},
	}

	for _, tc := range tests {
		got := buildRSSURL(tc.channel, tc.apiURL)
		if got != tc.want {
			t.Errorf("buildRSSURL(%q, %q) = %q, want %q", tc.channel, tc.apiURL, got, tc.want)
		}
	}
}

func TestBuildTelegraphRSSURL(t *testing.T) {
	got := buildTelegraphRSSURL("skolkovo")
	want := "https://telegra.ph/rss/skolkovo"
	if got != want {
		t.Errorf("buildTelegraphRSSURL(%q) = %q, want %q", "skolkovo", got, want)
	}

	got = buildTelegraphRSSURL("@skolkovo")
	want = "https://telegra.ph/rss/skolkovo"
	if got != want {
		t.Errorf("buildTelegraphRSSURL(%q) = %q, want %q", "@skolkovo", got, want)
	}
}

func TestPostID(t *testing.T) {
	id1 := postID("https://t.me/ch/1", "ch", time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC))
	id2 := postID("https://t.me/ch/1", "ch", time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC))
	if id1 != id2 {
		t.Errorf("postID not deterministic: %s != %s", id1, id2)
	}
	if !strings.HasPrefix(id1, "tg-") {
		t.Errorf("postID should start with 'tg-', got %s", id1)
	}
	if len(id1) != len("tg-")+16 {
		t.Errorf("postID length wrong: got %d, want %d", len(id1), len("tg-")+16)
	}
}

func TestPostID_Different(t *testing.T) {
	id1 := postID("https://t.me/ch/1", "ch", time.Time{})
	id2 := postID("https://t.me/ch/2", "ch", time.Time{})
	if id1 == id2 {
		t.Error("different URLs should produce different IDs")
	}
}

func TestParseRSSTime(t *testing.T) {
	tests := []struct {
		input string
		want  bool // true = не zero time
	}{
		{"Mon, 15 Jun 2026 10:00:00 +0300", true},
		{"2026-06-15T10:00:00Z", true},
		{"2026-06-15", true},
		{"", false},
		{"not a date", false},
	}

	for _, tc := range tests {
		got := parseRSSTime(tc.input)
		if tc.want && got.IsZero() {
			t.Errorf("parseRSSTime(%q) = zero, want non-zero", tc.input)
		}
		if !tc.want && !got.IsZero() {
			t.Errorf("parseRSSTime(%q) = %v, want zero", tc.input, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Mock TelegramStore for IngestPosts tests
// ---------------------------------------------------------------------------

type mockTelegramStore struct {
	posts      []*model.TelegramPost
	latestDate map[string]*time.Time
}

func newMockTelegramStore() *mockTelegramStore {
	return &mockTelegramStore{
		latestDate: make(map[string]*time.Time),
	}
}

func (m *mockTelegramStore) CreateTelegramPost(ctx context.Context, post *model.TelegramPost) error {
	m.posts = append(m.posts, post)
	if !post.PublishedAt.IsZero() {
		if ld, ok := m.latestDate[post.Channel]; !ok || post.PublishedAt.After(*ld) {
			t := post.PublishedAt
			m.latestDate[post.Channel] = &t
		}
	}
	return nil
}

func (m *mockTelegramStore) ListTelegramPosts(ctx context.Context, channel string, limit int) ([]*model.TelegramPost, error) {
	var result []*model.TelegramPost
	for _, p := range m.posts {
		if channel != "" && p.Channel != channel {
			continue
		}
		result = append(result, p)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockTelegramStore) GetLatestPostDate(ctx context.Context, channel string) (*time.Time, error) {
	if d, ok := m.latestDate[channel]; ok {
		return d, nil
	}
	return nil, nil
}

func (m *mockTelegramStore) CountPosts(ctx context.Context, channel string) (int, error) {
	count := 0
	for _, p := range m.posts {
		if channel == "" || p.Channel == channel {
			count++
		}
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// IngestPosts tests
// ---------------------------------------------------------------------------

func TestIngestPosts_NewPosts(t *testing.T) {
	st := newMockTelegramStore()
	now := time.Now()
	posts := []*model.TelegramPost{
		{
			ID:          "tg-001",
			Channel:     "@skolkovo",
			Text:        "Новый пост из канала Сколково",
			PublishedAt: now.Add(-2 * time.Hour),
			SourceURL:   "https://t.me/skolkovo/100",
		},
		{
			ID:          "tg-002",
			Channel:     "@skolkovo",
			Text:        "Ещё один пост",
			PublishedAt: now.Add(-1 * time.Hour), // более свежий — не будет пропущен
			SourceURL:   "https://t.me/skolkovo/99",
		},
	}

	res, err := IngestPosts(context.Background(), posts, st)
	if err != nil {
		t.Fatalf("IngestPosts error: %v", err)
	}
	if res.New != 2 {
		t.Errorf("expected 2 new posts, got %d", res.New)
	}

	count, _ := st.CountPosts(context.Background(), "@skolkovo")
	if count != 2 {
		t.Errorf("expected 2 posts in store, got %d", count)
	}
}

func TestIngestPosts_SkipsDuplicates(t *testing.T) {
	st := newMockTelegramStore()

	// Сначала создаём пост.
	st.CreateTelegramPost(context.Background(), &model.TelegramPost{
		ID:          "tg-existing",
		Channel:     "@skolkovo",
		Text:        "Старый пост",
		PublishedAt: time.Now().Add(-24 * time.Hour),
		SourceURL:   "https://t.me/skolkovo/50",
	})

	// Теперь пытаемся добавить пост с более ранней датой — должен быть пропущен.
	posts := []*model.TelegramPost{
		{
			ID:          "tg-duplicate",
			Channel:     "@skolkovo",
			Text:        "Дубликат",
			PublishedAt: time.Now().Add(-48 * time.Hour), // раньше существующего
			SourceURL:   "https://t.me/skolkovo/49",
		},
		{
			ID:          "tg-newer",
			Channel:     "@skolkovo",
			Text:        "Более свежий пост",
			PublishedAt: time.Now().Add(-1 * time.Hour), // позже существующего
			SourceURL:   "https://t.me/skolkovo/51",
		},
	}

	res, err := IngestPosts(context.Background(), posts, st)
	if err != nil {
		t.Fatalf("IngestPosts error: %v", err)
	}
	if res.Skipped != 1 {
		t.Errorf("expected 1 skipped (duplicate), got %d", res.Skipped)
	}
	if res.New != 1 {
		t.Errorf("expected 1 new, got %d", res.New)
	}
}

func TestIngestPosts_SkipsInvalid(t *testing.T) {
	st := newMockTelegramStore()
	posts := []*model.TelegramPost{
		{
			ID:        "tg-inv1",
			Channel:   "", // пустой канал
			Text:      "Текст",
			SourceURL: "https://t.me/empty/1",
		},
		{
			ID:        "tg-inv2",
			Channel:   "@skolkovo",
			Text:      "", // пустой текст
			SourceURL: "https://t.me/skolkovo/1",
		},
		{
			ID:        "tg-inv3",
			Channel:   "@skolkovo",
			Text:      "Текст",
			SourceURL: "", // пустой URL
		},
	}

	res, err := IngestPosts(context.Background(), posts, st)
	if err != nil {
		t.Fatalf("IngestPosts error: %v", err)
	}
	if res.New != 0 {
		t.Errorf("expected 0 new (all invalid), got %d", res.New)
	}
	if len(res.Errors) < 1 {
		t.Error("expected errors for invalid posts")
	}
}

func TestIngestPosts_MixedChannels(t *testing.T) {
	st := newMockTelegramStore()
	posts := []*model.TelegramPost{
		{
			ID:          "tg-a",
			Channel:     "@skolkovo",
			Text:        "Post from skolkovo",
			PublishedAt: time.Now(),
			SourceURL:   "https://t.me/skolkovo/1",
		},
		{
			ID:          "tg-b",
			Channel:     "@skolkovo_ventures",
			Text:        "Post from ventures",
			PublishedAt: time.Now(),
			SourceURL:   "https://t.me/skolkovo_ventures/1",
		},
	}

	res, err := IngestPosts(context.Background(), posts, st)
	if err != nil {
		t.Fatalf("IngestPosts error: %v", err)
	}
	if res.New != 2 {
		t.Errorf("expected 2 new, got %d", res.New)
	}

	// Проверяем посты из каждого канала.
	count1, _ := st.CountPosts(context.Background(), "@skolkovo")
	count2, _ := st.CountPosts(context.Background(), "@skolkovo_ventures")
	if count1 != 1 || count2 != 1 {
		t.Errorf("expected 1 post per channel, got %d and %d", count1, count2)
	}
}
