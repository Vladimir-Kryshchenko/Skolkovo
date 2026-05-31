package sitepages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// memStore — потокобезопасное хранилище страниц в памяти для тестов.
// Повторяет контракт PostgresStore.Upsert: new | changed | unchanged по хэшу.
type memStore struct {
	mu    sync.Mutex
	pages map[string]Page
}

func newMemStore() *memStore { return &memStore{pages: map[string]Page{}} }

func (m *memStore) Upsert(_ context.Context, p *Page) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ex, ok := m.pages[p.ID]; ok {
		if ex.ContentHash == p.ContentHash {
			return UpsertUnchanged, nil
		}
		m.pages[p.ID] = *p
		return UpsertChanged, nil
	}
	m.pages[p.ID] = *p
	return UpsertNew, nil
}

func (m *memStore) get(url string) (Page, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pages[pageID(url)]
	return p, ok
}

const homeHTML = `<!DOCTYPE html><html><head>
<title>Главная — Сколково</title>
<meta name="description" content="Описание главной страницы фонда">
<style>.x{color:red}</style></head>
<body><h1>Фонд Сколково</h1><p>Текст главной страницы.</p>
<a href="/sub">Подраздел</a>
<a href="/doc.pdf">Файл (не страница)</a>
<script>var x=1;</script></body></html>`

const subHTML = `<!DOCTYPE html><html><head><title>Подраздел</title></head>
<body><p>Содержимое подраздела.</p></body></html>`

func newTestServer(homeBody *string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sub" {
			w.Write([]byte(subHTML))
			return
		}
		w.Write([]byte(*homeBody))
	})
	return httptest.NewServer(mux)
}

func newCrawler(seed string, st Store) *Crawler {
	c := New([]string{seed}, st)
	c.Delay = 0 // тесты не должны ждать
	c.MaxPages = 10
	return c
}

func TestCrawlerExtractsAndCrawls(t *testing.T) {
	body := homeHTML
	srv := newTestServer(&body)
	defer srv.Close()

	st := newMemStore()
	rep, err := newCrawler(srv.URL, st).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Главная и /sub — обе страницы; /doc.pdf пропущена как файл.
	if rep.New != 2 {
		t.Fatalf("ожидалось 2 новые страницы, получено %d (visited=%d, errs=%v)", rep.New, rep.Visited, rep.Errors)
	}

	home, ok := st.get(srv.URL)
	if !ok {
		t.Fatal("главная страница не сохранена")
	}
	if home.Title != "Главная — Сколково" {
		t.Errorf("Title = %q", home.Title)
	}
	if home.Summary != "Описание главной страницы фонда" {
		t.Errorf("Summary (ожидался meta description) = %q", home.Summary)
	}
	if home.Section != "Главная" {
		t.Errorf("Section = %q (ожидалось «Главная»)", home.Section)
	}
	if home.ContentHash == "" {
		t.Error("ContentHash пуст")
	}
	if _, ok := st.get(srv.URL + "/sub"); !ok {
		t.Error("подраздел /sub не сохранён")
	}
}

func TestCrawlerDetectsUnchangedAndChanged(t *testing.T) {
	body := homeHTML
	srv := newTestServer(&body)
	defer srv.Close()
	st := newMemStore()

	// Первый обход — всё новое.
	if _, err := newCrawler(srv.URL, st).Run(context.Background()); err != nil {
		t.Fatalf("Run #1: %v", err)
	}

	// Второй обход без изменений контента — ничего не «изменилось».
	rep2, err := newCrawler(srv.URL, st).Run(context.Background())
	if err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if rep2.New != 0 || rep2.Changed != 0 {
		t.Fatalf("повторный обход: ожидалось 0 новых/изменённых, получено new=%d changed=%d", rep2.New, rep2.Changed)
	}
	if rep2.Unchanged == 0 {
		t.Fatal("повторный обход: ожидались страницы без изменений")
	}

	// Меняем тело главной — фиксируется изменение.
	body = `<!DOCTYPE html><html><head><title>Главная — Сколково</title>
<meta name="description" content="Новое описание"></head>
<body><p>Обновлённый текст.</p><a href="/sub">Подраздел</a></body></html>`
	rep3, err := newCrawler(srv.URL, st).Run(context.Background())
	if err != nil {
		t.Fatalf("Run #3: %v", err)
	}
	if rep3.Changed != 1 {
		t.Fatalf("после правки: ожидалось 1 изменение, получено %d", rep3.Changed)
	}
	home, _ := st.get(srv.URL)
	if home.Summary != "Новое описание" {
		t.Errorf("после правки Summary = %q", home.Summary)
	}
}

func TestSectionFromURL(t *testing.T) {
	cases := map[string]string{
		"https://sk.ru/":                                  "Главная",
		"https://sk.ru/foundation/documents/":             "foundation / documents",
		"https://dochub.sk.ru/foundation/documents/x.aspx": "foundation / documents",
	}
	for in, want := range cases {
		if got := sectionFromURL(in); got != want {
			t.Errorf("sectionFromURL(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}
