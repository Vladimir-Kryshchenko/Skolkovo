package rag

import (
	"strings"
	"testing"
)

func TestChunkText(t *testing.T) {
	if got := chunkText("", 100, 10); got != nil {
		t.Fatalf("пустой текст должен дать nil, получили %v", got)
	}

	long := strings.Repeat("Сколково это инновационный центр. ", 200)
	chunks := chunkText(long, 300, 30)
	if len(chunks) < 2 {
		t.Fatalf("ожидали несколько фрагментов, получили %d", len(chunks))
	}
	for i, c := range chunks {
		if len([]rune(c)) > 400 {
			t.Errorf("фрагмент %d слишком длинный: %d рун", i, len([]rune(c)))
		}
		if strings.TrimSpace(c) == "" {
			t.Errorf("фрагмент %d пустой", i)
		}
	}
}

func TestChunkSmallText(t *testing.T) {
	chunks := chunkText("Короткий текст.", 1200, 100)
	if len(chunks) != 1 {
		t.Fatalf("короткий текст должен дать 1 фрагмент, получили %d", len(chunks))
	}
}
