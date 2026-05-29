package news

import "testing"

func TestShortHash(t *testing.T) {
	h := shortHash("https://sk.ru/n/1")
	if len(h) != 16 {
		t.Fatalf("ожидали хэш длиной 16, получили %d", len(h))
	}
	if shortHash("a") == shortHash("b") {
		t.Error("разные входы дали одинаковый хэш")
	}
}
