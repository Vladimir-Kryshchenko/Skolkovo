package sitepages

import "testing"

func TestParseAnyDate(t *testing.T) {
	cases := []struct {
		in      string
		ok      bool
		y, m, d int
	}{
		{"25 мая 2017 г.", true, 2017, 5, 25},
		{"2 июня 2026", true, 2026, 6, 2},
		{"  1 января 2020  ", true, 2020, 1, 1},
		{"2017-05-25", true, 2017, 5, 25},
		{"2017-05-25T13:40:00+03:00", true, 2017, 5, 25},
		{"25.05.2017", true, 2017, 5, 25},
		{"опубликовано 25.05.2017 в 13:40", true, 2017, 5, 25},
		{"", false, 0, 0, 0},
		{"нет даты тут", false, 0, 0, 0},
		{"32 мартобря 1500", false, 0, 0, 0}, // год вне диапазона/несуществующий месяц
	}
	for _, c := range cases {
		got, ok := parseAnyDate(c.in)
		if ok != c.ok {
			t.Errorf("parseAnyDate(%q) ok=%v, want %v", c.in, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.Year() != c.y || int(got.Month()) != c.m || got.Day() != c.d {
			t.Errorf("parseAnyDate(%q) = %04d-%02d-%02d, want %04d-%02d-%02d",
				c.in, got.Year(), got.Month(), got.Day(), c.y, c.m, c.d)
		}
	}
}

// TestParseExtractsSkRuNewsDate проверяет, что дата публикации новости sk.ru
// (видимая надпись .post-page__info .data) извлекается при разборе страницы.
func TestParseExtractsSkRuNewsDate(t *testing.T) {
	html := `<html><head>
<meta property="og:type" content="article"/>
<title>Тестовая новость</title>
</head><body>
<div class="post-page"><div class="post-page__info"><div class="name-data"><div class="data">25 мая 2017 г.</div></div></div>
<div class="post-page__content">Текст новости.</div></div>
</body></html>`
	c := New(nil, nil)
	page, _ := c.parse([]byte(html), "https://sk.ru/news/test/")
	if page.PublishedAt == nil {
		t.Fatal("дата публикации не извлечена из разметки sk.ru")
	}
	if y, m, d := page.PublishedAt.Date(); y != 2017 || m != 5 || d != 25 {
		t.Errorf("извлечена дата %v, ожидалось 2017-05-25", page.PublishedAt)
	}
}

// TestParseExtractsMetaAndJSONLD проверяет машинные источники даты.
func TestParseExtractsMetaAndJSONLD(t *testing.T) {
	meta := `<html><head>
<meta property="article:published_time" content="2021-03-08T10:00:00+03:00"/>
<title>X</title></head><body>текст</body></html>`
	c := New(nil, nil)
	p, _ := c.parse([]byte(meta), "https://sk.ru/x/")
	if p.PublishedAt == nil || p.PublishedAt.Year() != 2021 || p.PublishedAt.Day() != 8 {
		t.Errorf("meta article:published_time не разобран: %v", p.PublishedAt)
	}

	ld := `<html><head><script type="application/ld+json">
{"@type":"NewsArticle","datePublished":"2019-11-12"}
</script><title>Y</title></head><body>текст</body></html>`
	p2, _ := c.parse([]byte(ld), "https://sk.ru/y/")
	if p2.PublishedAt == nil || p2.PublishedAt.Year() != 2019 || int(p2.PublishedAt.Month()) != 11 {
		t.Errorf("JSON-LD datePublished не разобран: %v", p2.PublishedAt)
	}
}

// TestParseNoDate — страница без даты не должна получать ложную дату.
func TestParseNoDate(t *testing.T) {
	html := `<html><head><title>Раздел</title></head><body>
<nav>Меню</nav><p>Описание раздела без дат.</p></body></html>`
	c := New(nil, nil)
	p, _ := c.parse([]byte(html), "https://sk.ru/section/")
	if p.PublishedAt != nil {
		t.Errorf("ложная дата на странице без даты: %v", p.PublishedAt)
	}
}
