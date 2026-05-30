package residents

import "testing"

func TestIsResidentURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://sk.ru/residents/acme-llc/", true},
		{"https://sk.ru/resident/12345/", true},
		{"https://sk.ru/company/innotech/", true},
		{"https://sk.ru/residents/", false}, // сам каталог, не профиль
		{"https://sk.ru/news/", false},
		{"https://sk.ru/about/", false},
	}
	for _, tt := range tests {
		if got := isResidentURL(tt.url); got != tt.want {
			t.Errorf("isResidentURL(%q) = %v, ожидалось %v", tt.url, got, tt.want)
		}
	}
}

func TestParseResidentsHTML(t *testing.T) {
	html := `<html><body>
		<a href="/residents/">Все резиденты</a>
		<a href="/residents/acme-llc/">ООО «Акме»</a>
		<a href="/residents/beta-corp/">Бета Корп</a>
		<a href="/news/2026/">Новость</a>
		<a href="/residents/acme-llc/">ООО «Акме»</a>
	</body></html>`

	list, err := parseResidentsHTML("https://sk.ru/residents/", "IT", []byte(html))
	if err != nil {
		t.Fatalf("parseResidentsHTML error: %v", err)
	}
	// Два уникальных профиля (дубль и каталог/новость отсеяны).
	if len(list) != 2 {
		t.Fatalf("ожидалось 2 резидента, получено %d", len(list))
	}
	if list[0].Name != "ООО «Акме»" {
		t.Errorf("Name = %q, ожидалось ООО «Акме»", list[0].Name)
	}
	if list[0].Industry != "IT" {
		t.Errorf("Industry = %q, ожидалось IT", list[0].Industry)
	}
	if list[0].SourceURL != "https://sk.ru/residents/acme-llc/" {
		t.Errorf("SourceURL = %q", list[0].SourceURL)
	}
}
