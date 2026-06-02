package sitepages

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseSitemap(t *testing.T) {
	t.Run("urlset", func(t *testing.T) {
		xml := `<?xml version="1.0"?><urlset><url><loc>https://sk.ru/a</loc></url><url><loc>https://sk.ru/b</loc></url></urlset>`
		locs, sms := parseSitemap([]byte(xml), "https://sk.ru/sitemap.xml")
		if len(locs) != 2 || len(sms) != 0 {
			t.Fatalf("ожидали 2 url и 0 sitemap, получили %d/%d", len(locs), len(sms))
		}
	})
	t.Run("index", func(t *testing.T) {
		xml := `<?xml version="1.0"?><sitemapindex><sitemap><loc>https://sk.ru/sm1.xml</loc></sitemap></sitemapindex>`
		locs, sms := parseSitemap([]byte(xml), "https://sk.ru/sitemap_index.xml")
		if len(locs) != 0 || len(sms) != 1 {
			t.Fatalf("ожидали 0 url и 1 sitemap, получили %d/%d", len(locs), len(sms))
		}
	})
}

func TestSitemapsFromRobots(t *testing.T) {
	robots := "User-agent: *\nDisallow: /admin\nSitemap: https://sk.ru/sitemap.xml\nsitemap:  https://sk.ru/news_sitemap.xml\n"
	got := sitemapsFromRobots([]byte(robots))
	if len(got) != 2 || got[0] != "https://sk.ru/sitemap.xml" || got[1] != "https://sk.ru/news_sitemap.xml" {
		t.Fatalf("неверно распознаны Sitemap-директивы: %v", got)
	}
}

// TestCrawlerSeedsFromSitemap проверяет, что страница-сирота (на неё нет ссылок)
// находится через sitemap, объявленный в robots.txt. discoverFromSitemaps берёт
// origin (scheme://host) из seed-а, поэтому http-тест-сервер работает.
func TestCrawlerSeedsFromSitemap(t *testing.T) {
	var host string
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "User-agent: *\nSitemap: %s/sitemap.xml\n", host)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0"?><urlset><url><loc>%s/orphan</loc></url></urlset>`, host)
	})
	mux.HandleFunc("/orphan", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Сирота</title></head><body>Никто не ссылается</body></html>`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Главная</title></head><body>без ссылок на сироту</body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	host = srv.URL

	st := newMemStore()
	if _, err := newCrawler(srv.URL, st).Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := st.get(srv.URL + "/orphan"); !ok {
		t.Error("страница-сирота не найдена через sitemap")
	}
}
