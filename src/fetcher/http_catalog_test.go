package fetcher

import (
	"net/url"
	"testing"
)

// fixtureCategoryHTML воспроизводит реальную структуру страницы категории
// dochub.sk.ru: документы — это <ul class="unstyled" data-contenttype="file"
// data-contentid="..."> с <a href=".../m/docs/{id}/download.aspx">Заголовок</a>.
const fixtureCategoryHTML = `<!DOCTYPE html><html lang="en"><head>
<meta name="title" content="Иные нормативные документы" /></head><body>
<div class="row-fluid"><div class="superlist-column span12">
  <div class='well well-light'>
    <ul class="unstyled" data-contenttype="file" data-contentid="24812" data-sectionid="abc">
      <div class="mediaFile">
        <img src="/cfs-filesystemfile.ashx/.../pdf.png" alt="" />
        <div>
          <a href="/foundation/documents/m/docs/24812/download.aspx">Приказ 143-Пр.  Регламент   контроля доступа</a>
        </div>
      </div>
    </ul>
  </div>
  <div class='well well-light'>
    <ul class="unstyled" data-contenttype="file" data-contentid="24788">
      <a href="/foundation/documents/m/docs/24788/download.aspx">Федеральный закон №243</a>
    </ul>
  </div>
  <!-- внешняя/служебная ссылка — не документ -->
  <a href="/foundation/documents/p/other.aspx">Назад к категориям</a>
  <a href="https://example.com/external">Внешний сайт</a>
</div></div>
<!-- скрытая кнопка next (display:none) — пагинации нет -->
<a href="/foundation/documents/p/other.aspx?pi123=2" class="next" style="display:none"><span></span></a>
</body></html>`

// fixtureWithPagination — страница с ВИДИМОЙ кнопкой next (есть след. страница).
const fixtureWithPagination = `<html><body>
<ul class="unstyled" data-contenttype="file" data-contentid="100">
  <a href="/foundation/documents/m/docs/100/download.aspx">Документ 100</a>
</ul>
<a href="/foundation/documents/p/unactual_documents.aspx?pi999=2" class="next"><span></span></a>
</body></html>`

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

func TestParseCategoryHTML_Docs(t *testing.T) {
	base := mustURL(t, "https://dochub.sk.ru/foundation/documents/p/other.aspx")
	docs, next, err := parseCategoryHTML([]byte(fixtureCategoryHTML), base)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("ожидалось 2 документа, получено %d: %+v", len(docs), docs)
	}
	// Ссылки абсолютизированы.
	want := map[string]string{
		"https://dochub.sk.ru/foundation/documents/m/docs/24812/download.aspx": "Приказ 143-Пр. Регламент контроля доступа",
		"https://dochub.sk.ru/foundation/documents/m/docs/24788/download.aspx": "Федеральный закон №243",
	}
	for _, d := range docs {
		title, ok := want[d.Link]
		if !ok {
			t.Errorf("неожиданная ссылка: %s", d.Link)
			continue
		}
		if d.Title != title {
			t.Errorf("заголовок для %s = %q, ожидался %q (пробелы должны схлопываться)", d.Link, d.Title, title)
		}
	}
	// Скрытая кнопка next → пагинации нет.
	if next != "" {
		t.Errorf("next должен быть пустым (кнопка display:none), получено %q", next)
	}
}

func TestParseCategoryHTML_Pagination(t *testing.T) {
	base := mustURL(t, "https://dochub.sk.ru/foundation/documents/p/unactual_documents.aspx")
	docs, next, err := parseCategoryHTML([]byte(fixtureWithPagination), base)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("ожидался 1 документ, получено %d", len(docs))
	}
	wantNext := "https://dochub.sk.ru/foundation/documents/p/unactual_documents.aspx?pi999=2"
	if next != wantNext {
		t.Errorf("next = %q, ожидался %q", next, wantNext)
	}
}

func TestParseCategoryHTML_Empty(t *testing.T) {
	base := mustURL(t, "https://dochub.sk.ru/foundation/documents/p/x.aspx")
	docs, next, err := parseCategoryHTML([]byte(`<html><body><p>нет документов</p></body></html>`), base)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(docs) != 0 || next != "" {
		t.Errorf("ожидалось 0 документов и пустой next, получено docs=%d next=%q", len(docs), next)
	}
}

func TestIsDocDownloadLink(t *testing.T) {
	cases := map[string]bool{
		"https://dochub.sk.ru/foundation/documents/m/docs/24812/download.aspx": true,
		"https://dochub.sk.ru/m/docs/100/index.aspx":                           true,
		"https://dochub.sk.ru/foundation/documents/p/other.aspx":               false,
		"https://example.com/external":                                         false,
		"https://dochub.sk.ru/foundation/documents/rss.aspx":                   false,
	}
	for u, want := range cases {
		if got := isDocDownloadLink(u); got != want {
			t.Errorf("isDocDownloadLink(%q)=%v, want %v", u, got, want)
		}
	}
}

func TestResolveRef(t *testing.T) {
	base := mustURL(t, "https://dochub.sk.ru/foundation/documents/p/other.aspx")
	cases := map[string]string{
		"/foundation/documents/m/docs/1/download.aspx": "https://dochub.sk.ru/foundation/documents/m/docs/1/download.aspx",
		"https://x.ru/a":     "https://x.ru/a",
		"#anchor":            "",
		"mailto:a@b.c":       "",
		"javascript:void(0)": "",
	}
	for in, want := range cases {
		if got := resolveRef(base, in); got != want {
			t.Errorf("resolveRef(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestCollapseSpaces(t *testing.T) {
	cases := map[string]string{
		"  a   b\n\tc ": "a b c",
		"single":        "single",
		"":              "",
		"   ":           "",
	}
	for in, want := range cases {
		if got := collapseSpaces(in); got != want {
			t.Errorf("collapseSpaces(%q)=%q, want %q", in, got, want)
		}
	}
}
