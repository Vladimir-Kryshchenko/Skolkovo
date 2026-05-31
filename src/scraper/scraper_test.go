package scraper

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://dochub.sk.ru/m/docs/file.pdf":          "https://dochub.sk.ru/m/docs/file.pdf",
		"https://dochub.sk.ru/m/docs/file.pdf/":         "https://dochub.sk.ru/m/docs/file.pdf",
		"https://dochub.sk.ru/m/docs/file.pdf?x=1#frag": "https://dochub.sk.ru/m/docs/file.pdf",
		"HTTPS://DOCHUB.SK.RU/m/docs/file.pdf":          "https://dochub.sk.ru/m/docs/file.pdf",
		"https://dochub.sk.ru/":                         "https://dochub.sk.ru/",
		"not a url":                                     "not a url",
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q)=%q, want %q", in, got, want)
		}
	}
}

// TestDocIDDedup проверяет, что слегка различающиеся URL одного документа
// (хвостовой слэш, query, фрагмент, регистр хоста) дают один и тот же ID.
func TestDocIDDedup(t *testing.T) {
	canonical := DocID("https://dochub.sk.ru/m/docs/file.pdf")
	variants := []string{
		"https://dochub.sk.ru/m/docs/file.pdf/",
		"https://dochub.sk.ru/m/docs/file.pdf?utm=1",
		"https://dochub.sk.ru/m/docs/file.pdf#section",
		"https://DOCHUB.sk.ru/m/docs/file.pdf",
	}
	for _, v := range variants {
		if got := DocID(v); got != canonical {
			t.Errorf("DocID(%q)=%s, ожидался %s (как у канонического)", v, got, canonical)
		}
	}
	// Разные документы — разные ID.
	if DocID("https://dochub.sk.ru/m/docs/a.pdf") == DocID("https://dochub.sk.ru/m/docs/b.pdf") {
		t.Error("разные документы получили одинаковый ID")
	}
}
