package faq

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
// HTML parsing tests
// ---------------------------------------------------------------------------

func TestParseFAQFromHTML_FAQItems(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <div class="faq-section">
    <div class="faq-item">
      <div class="faq-question">Как стать резидентом Сколково?</div>
      <div class="faq-answer">Для этого необходимо подать заявку на сайте и пройти экспертную оценку проекта.</div>
    </div>
    <div class="faq-item">
      <div class="faq-question">Какие льготы получает резидент?</div>
      <div class="faq-answer">Налоговые льготы, гранты, доступ к инфраструктуре и менторству.</div>
    </div>
  </div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	cfg := FAQConfig{FAQURL: srv.URL, Category: "FAQ"}
	items, err := ParseFAQ(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatalf("ParseFAQ error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 FAQ items, got %d", len(items))
	}

	found := false
	for _, it := range items {
		if it.Question == "Как стать резидентом Сколково?" {
			found = true
			if !strings.Contains(it.Answer, "заявку") {
				t.Errorf("unexpected answer: %s", it.Answer)
			}
			if it.Category != "FAQ" {
				t.Errorf("expected category 'FAQ', got '%s'", it.Category)
			}
			if !strings.HasPrefix(it.ID, "faq-") {
				t.Errorf("ID should start with 'faq-', got %s", it.ID)
			}
		}
	}
	if !found {
		t.Error("did not find 'Как стать резидентом Сколково?' in FAQ items")
	}
}

func TestParseFAQFromHTML_DetailsTags(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <details class="faq-entry">
    <summary>Что такое Сколково?</summary>
    <p>Сколково — это инновационный центр с особыми условиями для ведения бизнеса.</p>
  </details>
  <details class="faq-entry">
    <summary>Какие направления существуют?</summary>
    <p>Энергоэффективность, IT, биомед, космос и ядерные технологии.</p>
  </details>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	cfg := FAQConfig{FAQURL: srv.URL, Category: "Общее"}
	items, err := ParseFAQ(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatalf("ParseFAQ error: %v", err)
	}
	if len(items) < 1 {
		t.Fatalf("expected at least 1 FAQ item, got %d", len(items))
	}

	// Проверяем, что <details> распарсен.
	found := false
	for _, it := range items {
		if strings.Contains(it.Question, "Сколково") {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find FAQ item about 'Сколково'")
	}
}

func TestParseFAQFromHTML_HeadingPairs(t *testing.T) {
	htmlBody := `<!DOCTYPE html>
<html>
<body>
  <div id="faq">
    <h3>Какой срок рассмотрения заявки?</h3>
    <p>Обычно от 2 до 4 недель с момента подачи полного пакета документов.</p>
    <h3>Можно ли подать заявку повторно?</h3>
    <p>Да, при наличии доработанного проекта.</p>
  </div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBody))
	}))
	defer srv.Close()

	cfg := FAQConfig{FAQURL: srv.URL, Category: "Заявки"}
	items, err := ParseFAQ(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatalf("ParseFAQ error: %v", err)
	}
	if len(items) < 1 {
		t.Fatalf("expected at least 1 FAQ item, got %d", len(items))
	}
}

func TestParseFAQFromHTML_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := FAQConfig{FAQURL: srv.URL}
	_, err := ParseFAQ(context.Background(), cfg, srv.Client())
	if err == nil {
		t.Fatal("expected error for 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestParseFAQ_NoURL(t *testing.T) {
	cfg := FAQConfig{}
	_, err := ParseFAQ(context.Background(), cfg, http.DefaultClient)
	if err == nil {
		t.Fatal("expected error when no FAQURL configured")
	}
}

// ---------------------------------------------------------------------------
// faqID tests
// ---------------------------------------------------------------------------

func TestFAQID(t *testing.T) {
	id1 := faqID("test question", "https://sk.ru/faq")
	id2 := faqID("test question", "https://sk.ru/faq")
	if id1 != id2 {
		t.Errorf("faqID not deterministic: %s != %s", id1, id2)
	}
	if !strings.HasPrefix(id1, "faq-") {
		t.Errorf("faqID should start with 'faq-', got %s", id1)
	}
	if len(id1) != len("faq-")+16 {
		t.Errorf("faqID length wrong: got %d, want %d", len(id1), len("faq-")+16)
	}
}

func TestFAQID_Different(t *testing.T) {
	id1 := faqID("question 1", "https://sk.ru/faq")
	id2 := faqID("question 2", "https://sk.ru/faq")
	if id1 == id2 {
		t.Error("different questions should produce different IDs")
	}
}

// ---------------------------------------------------------------------------
// Mock FAQStore for IngestFAQ tests
// ---------------------------------------------------------------------------

type mockFAQStore struct {
	items map[string]*model.FAQItem
}

func newMockFAQStore() *mockFAQStore {
	return &mockFAQStore{items: make(map[string]*model.FAQItem)}
}

func (m *mockFAQStore) CreateFAQItem(ctx context.Context, item *model.FAQItem) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockFAQStore) GetFAQItem(ctx context.Context, id string) (*model.FAQItem, error) {
	if it, ok := m.items[id]; ok {
		return it, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockFAQStore) ListFAQItems(ctx context.Context, category string) ([]*model.FAQItem, error) {
	var result []*model.FAQItem
	for _, it := range m.items {
		if category != "" && it.Category != category {
			continue
		}
		result = append(result, it)
	}
	return result, nil
}

func (m *mockFAQStore) UpdateFAQItem(ctx context.Context, item *model.FAQItem) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockFAQStore) DeleteFAQItem(ctx context.Context, id string) error {
	delete(m.items, id)
	return nil
}

func (m *mockFAQStore) CountFAQItems(ctx context.Context) (int, error) {
	return len(m.items), nil
}

// ---------------------------------------------------------------------------
// IngestFAQ tests
// ---------------------------------------------------------------------------

func TestIngestFAQ_NewItems(t *testing.T) {
	st := newMockFAQStore()
	items := []*model.FAQItem{
		{
			ID:        "faq-001",
			Question:  "Как получить грант?",
			Answer:    "Подайте заявку через портал.",
			Category:  "FAQ",
			SourceURL: "https://sk.ru/faq/grants",
		},
	}

	res, err := IngestFAQ(context.Background(), items, st, nil)
	if err != nil {
		t.Fatalf("IngestFAQ error: %v", err)
	}
	if res.New != 1 {
		t.Errorf("expected 1 new item, got %d", res.New)
	}
	if res.Updated != 0 {
		t.Errorf("expected 0 updated, got %d", res.Updated)
	}

	it, err := st.GetFAQItem(context.Background(), "faq-001")
	if err != nil {
		t.Fatalf("GetFAQItem error: %v", err)
	}
	if it.Question != "Как получить грант?" {
		t.Errorf("unexpected question: %s", it.Question)
	}
}

func TestIngestFAQ_UpdateExisting(t *testing.T) {
	st := newMockFAQStore()

	// Сначала создаём элемент.
	st.CreateFAQItem(context.Background(), &model.FAQItem{
		ID:        "faq-002",
		Question:  "Старый вопрос",
		Answer:    "Старый ответ",
		Category:  "FAQ",
		SourceURL: "https://sk.ru/faq/old",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	})

	// Теперь обновляем.
	items := []*model.FAQItem{
		{
			ID:        "faq-002",
			Question:  "Новый вопрос",
			Answer:    "Новый ответ",
			Category:  "FAQ",
			SourceURL: "https://sk.ru/faq/old",
		},
	}

	res, err := IngestFAQ(context.Background(), items, st, nil)
	if err != nil {
		t.Fatalf("IngestFAQ error: %v", err)
	}
	if res.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", res.Updated)
	}

	it, err := st.GetFAQItem(context.Background(), "faq-002")
	if err != nil {
		t.Fatalf("GetFAQItem error: %v", err)
	}
	if it.Question != "Новый вопрос" {
		t.Errorf("expected updated question, got %s", it.Question)
	}
}

func TestIngestFAQ_SkipsInvalid(t *testing.T) {
	st := newMockFAQStore()
	items := []*model.FAQItem{
		{
			ID:        "faq-003",
			Question:  "", // пустой вопрос
			Answer:    "Ответ",
			SourceURL: "https://sk.ru/faq/invalid",
		},
		{
			ID:        "faq-004",
			Question:  "Вопрос",
			Answer:    "", // пустой ответ
			SourceURL: "https://sk.ru/faq/invalid2",
		},
	}

	res, err := IngestFAQ(context.Background(), items, st, nil)
	if err != nil {
		t.Fatalf("IngestFAQ error: %v", err)
	}
	if res.New != 0 {
		t.Errorf("expected 0 new (invalid), got %d", res.New)
	}
	if len(res.Errors) < 1 {
		t.Error("expected errors for invalid items")
	}
}

func TestIngestFAQ_Multiple(t *testing.T) {
	st := newMockFAQStore()
	items := []*model.FAQItem{
		{
			ID:        "faq-a",
			Question:  "Вопрос A",
			Answer:    "Ответ A",
			SourceURL: "https://sk.ru/faq/a",
		},
		{
			ID:        "faq-b",
			Question:  "Вопрос B",
			Answer:    "Ответ B",
			SourceURL: "https://sk.ru/faq/b",
		},
		{
			ID:        "faq-c",
			Question:  "Вопрос C",
			Answer:    "Ответ C",
			SourceURL: "https://sk.ru/faq/c",
		},
	}

	res, err := IngestFAQ(context.Background(), items, st, nil)
	if err != nil {
		t.Fatalf("IngestFAQ error: %v", err)
	}
	if res.New != 3 {
		t.Errorf("expected 3 new, got %d", res.New)
	}
	if res.Fetched != 3 {
		t.Errorf("expected 3 fetched, got %d", res.Fetched)
	}

	count, _ := st.CountFAQItems(context.Background())
	if count != 3 {
		t.Errorf("expected 3 items in store, got %d", count)
	}
}
