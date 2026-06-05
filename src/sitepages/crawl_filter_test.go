package sitepages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestIsExcludedURL проверяет, что денилист отсекает тег-ловушки и служебные
// страницы Telligent, но пропускает реальный контент.
func TestIsExcludedURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		// тег-фильтры (комбинаторная ловушка) — исключаем
		{"https://dochub.sk.ru/net/1120657/b/news/archive/tags/_a_/_a+b_/default.aspx", true},
		{"https://dochub.sk.ru/tags/website/it/default.aspx", true},
		{"https://dochub.sk.ru/news/tags/1122578/_x_/default.aspx", true},
		{"https://dochub.sk.ru/news/tag/foo", true},
		// служебные/пользовательские разделы — исключаем
		{"https://dochub.sk.ru/user/createuser.aspx", true},
		{"https://dochub.sk.ru/user/emailforgottenpassword.aspx", true},
		{"https://dochub.sk.ru/members/123", true},
		{"https://dochub.sk.ru/search", true},
		{"https://dochub.sk.ru/login.aspx", true},
		// слишком глубокий путь — предохранитель
		{"https://dochub.sk.ru/a/b/c/d/e/f/g/h/i/j/k/l/m", true},
		// реальный контент — оставляем
		{"https://sk.ru/", false},
		{"https://sk.ru/foundation/events", false},
		{"https://dochub.sk.ru/news/b/news/archive/2020/03/25/sensorteh-pomozhet.aspx", false},
		{"https://dochub.sk.ru/foundation/documents", false},
	}
	for _, c := range cases {
		u, err := url.Parse(c.in)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		if got := isExcludedURL(u); got != c.want {
			t.Errorf("isExcludedURL(%q) = %v, ожидалось %v", c.in, got, c.want)
		}
	}
}

// TestCrawlerSkipsExcludedLinks проверяет, что краулер не ставит в очередь и не
// сохраняет страницы-ловушки (ссылки с сегментом tags), но обходит обычные.
func TestCrawlerSkipsExcludedLinks(t *testing.T) {
	const home = `<!DOCTYPE html><html><head><title>Главная</title></head><body>
<a href="/sub">Реальный подраздел</a>
<a href="/tags/website/it/default.aspx">Тег-фильтр (ловушка)</a>
<a href="/user/createuser.aspx">Регистрация (служебная)</a>
</body></html>`
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sub":
			w.Write([]byte(subHTML))
		default:
			w.Write([]byte(home))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	st := newMemStore()
	c := newCrawler(srv.URL, st)
	if _, err := c.Run(context.Background()); err != nil {
		t.Fatalf("обход: %v", err)
	}

	if _, ok := st.get(srv.URL + "/sub"); !ok {
		t.Error("реальный подраздел /sub должен быть сохранён")
	}
	if _, ok := st.get(srv.URL + "/tags/website/it/default.aspx"); ok {
		t.Error("тег-ловушка /tags/... не должна попадать в хранилище")
	}
	if _, ok := st.get(srv.URL + "/user/createuser.aspx"); ok {
		t.Error("служебная /user/... не должна попадать в хранилище")
	}
}
