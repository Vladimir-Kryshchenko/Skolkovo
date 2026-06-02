package agents

import (
	"context"
	"testing"

	rag "baza-skolkovo/src/rag_service"
)

func TestIsWAFBlockedDocURL(t *testing.T) {
	cases := map[string]bool{
		"https://dochub.sk.ru/foundation/documents/m/docs/24873/download.aspx": true,
		"https://dochub.sk.ru/foundation/documents/m/docs/24899.aspx":          true,
		"https://dochub.sk.ru/m/docs/24873":                                    true,
		"https://dochub.sk.ru/news/m/wiki/19409/download.aspx":                 false, // рабочий раздел
		"https://sk.ru/events/123":                                            false,
		"":                                                                     false,
	}
	for in, want := range cases {
		if got := isWAFBlockedDocURL(in); got != want {
			t.Errorf("isWAFBlockedDocURL(%q) = %v, ожидалось %v", in, got, want)
		}
	}
}

func TestCategoryPageURL(t *testing.T) {
	blocked := "https://dochub.sk.ru/foundation/documents/m/docs/24873/download.aspx"

	// Известная категория → страница-листинг по слагу.
	got := categoryPageURL(blocked, "Законодательные акты")
	want := "https://dochub.sk.ru/foundation/documents/p/legislative_acts.aspx"
	if got != want {
		t.Errorf("известная категория: got %q, want %q", got, want)
	}

	// Неизвестная категория → корневой раздел документов.
	got = categoryPageURL(blocked, "Неизвестная категория")
	want = "https://dochub.sk.ru/foundation/documents/"
	if got != want {
		t.Errorf("неизвестная категория: got %q, want %q", got, want)
	}

	// Невалидный URL без хоста → пусто.
	if got := categoryPageURL("not a url", "Антикоррупция"); got != "" {
		t.Errorf("невалидный URL: got %q, ожидалось пусто", got)
	}
}

// TestBestSourceURL_CategoryFallback проверяет фолбэк на категорию без публичного
// базового URL (fileBaseURL пуст → ветка «наша копия» не задействуется, Store не
// требуется).
func TestBestSourceURL_CategoryFallback(t *testing.T) {
	a := &ConsultantAgent{} // fileBaseURL == "", ragService == nil
	ctx := context.Background()

	// Заблокированная ссылка на документ → страница категории.
	r := rag.Result{
		DocumentID: "abc",
		EntityType: "document",
		SourceURL:  "https://dochub.sk.ru/foundation/documents/m/docs/24894/download.aspx",
		Category:   "Иные нормативные документы",
	}
	got := a.bestSourceURL(ctx, r)
	want := "https://dochub.sk.ru/foundation/documents/p/other.aspx"
	if got != want {
		t.Errorf("блокированный документ: got %q, want %q", got, want)
	}

	// Рабочая ссылка из /news/m/wiki/ → без изменений.
	r = rag.Result{EntityType: "document", SourceURL: "https://dochub.sk.ru/news/m/wiki/19409/download.aspx"}
	if got := a.bestSourceURL(ctx, r); got != r.SourceURL {
		t.Errorf("рабочая wiki-ссылка: got %q, ожидалось без изменений %q", got, r.SourceURL)
	}

	// Не-документ (мероприятие) → без изменений.
	r = rag.Result{EntityType: "event", SourceURL: "https://sk.ru/events/42"}
	if got := a.bestSourceURL(ctx, r); got != r.SourceURL {
		t.Errorf("мероприятие: got %q, ожидалось без изменений %q", got, r.SourceURL)
	}
}
