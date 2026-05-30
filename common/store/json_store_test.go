package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
)

func TestJSONStoreRoundtrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "реестр.json")

	st, err := NewJSONStore(path)
	if err != nil {
		t.Fatal(err)
	}

	doc := model.Document{
		ID:        "d1",
		Title:     "Тестовый документ",
		SourceURL: "https://example.org/d1.pdf",
		FetchedAt: time.Now(),
		Status:    model.StatusPending,
		Category:  "Тест",
		FileHash:  "abc",
	}
	if err := st.Upsert(ctx, doc); err != nil {
		t.Fatal(err)
	}

	// Перечитываем с диска другим экземпляром — проверяем персистентность.
	st2, err := NewJSONStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st2.Get(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Тестовый документ" {
		t.Errorf("title = %q", got.Title)
	}

	if err := st2.SetStatus(ctx, "d1", model.StatusActive); err != nil {
		t.Fatal(err)
	}
	active, err := st2.List(ctx, Filter{Status: model.StatusActive})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("ожидали 1 действующий документ, получили %d", len(active))
	}

	if err := st2.Delete(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st2.Get(ctx, "d1"); err != ErrNotFound {
		t.Errorf("ожидали ErrNotFound, получили %v", err)
	}
}
