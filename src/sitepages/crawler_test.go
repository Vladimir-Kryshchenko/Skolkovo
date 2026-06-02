package sitepages

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (m *memStore) MarkGone(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.pages[id]
	if !ok || p.Status == StatusGone {
		return false, nil
	}
	p.Status = StatusGone
	m.pages[id] = p
	return true, nil
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

// TestCrawlerFollowsQueryPagination проверяет, что страницы с query-параметрами
// (?page=2) обходятся как отдельные, а utm-метки не плодят дубликаты.
func TestCrawlerFollowsQueryPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/list" && r.URL.Query().Get("page") == "2":
			w.Write([]byte(`<html><head><title>Список стр.2</title></head><body>Вторая страница списка</body></html>`))
		case r.URL.Path == "/list":
			// Ссылка на стр.2 и на ту же главную с utm-хвостом (не должна дублироваться).
			w.Write([]byte(`<html><head><title>Список</title></head><body>
				<a href="/list?page=2">Следующая</a>
				<a href="/list?utm_source=ad">Та же страница с меткой</a></body></html>`))
		default:
			w.Write([]byte(`<html><head><title>Главная</title></head><body><a href="/list">Список</a></body></html>`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st := newMemStore()
	rep, err := newCrawler(srv.URL, st).Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Главная, /list, /list?page=2 — три страницы; utm-вариант схлопнулся в /list.
	if _, ok := st.get(srv.URL + "/list?page=2"); !ok {
		t.Errorf("страница пагинации /list?page=2 не обойдена (new=%d, errs=%v)", rep.New, rep.Errors)
	}
	if rep.New != 3 {
		t.Errorf("ожидалось 3 новые страницы, получено %d", rep.New)
	}
}

// TestCrawlerMarksGone проверяет, что исчезнувшая (404) страница помечается gone.
func TestCrawlerMarksGone(t *testing.T) {
	var listGone bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list" {
			if listGone {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write([]byte(`<html><head><title>Список</title></head><body>есть</body></html>`))
			return
		}
		w.Write([]byte(`<html><head><title>Главная</title></head><body><a href="/list">Список</a></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st := newMemStore()
	if _, err := newCrawler(srv.URL, st).Run(context.Background()); err != nil {
		t.Fatalf("Run #1: %v", err)
	}
	if p, ok := st.get(srv.URL + "/list"); !ok || p.Status != StatusActive {
		t.Fatalf("после первого обхода /list должна быть active")
	}

	// Страница исчезает → 404 → пометка gone.
	listGone = true
	rep, err := newCrawler(srv.URL, st).Run(context.Background())
	if err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if rep.Gone != 1 {
		t.Errorf("ожидалась 1 страница gone, получено %d", rep.Gone)
	}
	if p, _ := st.get(srv.URL + "/list"); p.Status != StatusGone {
		t.Errorf("/list должна быть gone, статус = %q", p.Status)
	}
}

// TestCrawlerConcurrent проверяет, что при Concurrency>1 обходятся все страницы
// (без потерь и дублей) — корректность важнее запускать под `go test -race`.
func TestCrawlerConcurrent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			// Главная ссылается на 20 подстраниц.
			var b strings.Builder
			b.WriteString(`<html><head><title>Главная</title></head><body>`)
			for i := 0; i < 20; i++ {
				fmt.Fprintf(&b, `<a href="/p/%d">стр %d</a>`, i, i)
			}
			b.WriteString(`</body></html>`)
			w.Write([]byte(b.String()))
			return
		}
		fmt.Fprintf(w, `<html><head><title>Стр %s</title></head><body>контент %s</body></html>`, r.URL.Path, r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st := newMemStore()
	cr := New([]string{srv.URL}, st)
	cr.Delay = 0
	cr.MaxPages = 100
	cr.Concurrency = 5
	rep, err := cr.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Главная + 20 подстраниц = 21.
	if rep.New != 21 {
		t.Errorf("ожидался 21 новый документ, получено %d (visited=%d, errs=%v)", rep.New, rep.Visited, rep.Errors)
	}
	for i := 0; i < 20; i++ {
		if _, ok := st.get(fmt.Sprintf("%s/p/%d", srv.URL, i)); !ok {
			t.Errorf("подстраница /p/%d не сохранена", i)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://SK.ru/Foo/":                   "https://sk.ru/Foo",
		"https://sk.ru/list?page=2#top":        "https://sk.ru/list?page=2",
		"https://sk.ru/x?utm_source=ad&page=3": "https://sk.ru/x?page=3",
		"https://sk.ru/x?b=2&a=1":              "https://sk.ru/x?a=1&b=2", // ключи сортируются
		"https://sk.ru/only?utm_medium=email":  "https://sk.ru/only",      // только метка — query исчезает
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

func TestSectionFromURL(t *testing.T) {
	cases := map[string]string{
		"https://sk.ru/":                                   "Главная",
		"https://sk.ru/foundation/documents/":              "foundation / documents",
		"https://dochub.sk.ru/foundation/documents/x.aspx": "foundation / documents",
	}
	for in, want := range cases {
		if got := sectionFromURL(in); got != want {
			t.Errorf("sectionFromURL(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}
