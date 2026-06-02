package navindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedArtifactsUpToDate страж рассинхрона: сгенерированные из Tree()
// карты (RAG_Структура_сайта.md и navigation.json) должны совпадать с тем, что
// закоммичено. Если упал — структура сайта поменялась в коде, а артефакты не
// пересобраны: запустите `go run ./cmd/skolkovo navindex render export`.
func TestGeneratedArtifactsUpToDate(t *testing.T) {
	dir := filepath.Join("..", "..", "Документы_Сколково", "RAG_Структура_сайта")
	tree := Tree()

	jsonWant, err := ToJSON(tree)
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	cases := map[string]string{
		"RAG_Структура_сайта.md": ToMarkdown(tree),
		"navigation.json":        string(jsonWant),
	}
	for file, want := range cases {
		got, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("не прочитать %s (пересоберите: `navindex render export`): %v", file, err)
		}
		if norm(string(got)) != norm(want) {
			t.Errorf("%s устарел относительно Tree() — пересоберите: `go run ./cmd/skolkovo navindex render export`", file)
		}
	}
}

// norm убирает различия переводов строк (CRLF↔LF), чтобы сравнение не зависело
// от настроек git autocrlf на Windows.
func norm(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
