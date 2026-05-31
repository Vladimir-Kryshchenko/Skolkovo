package navindex

import (
	"strings"
	"testing"
)

// TestTreeCoverage проверяет, что карта покрывает все 6 интерфейсов и порты.
func TestTreeCoverage(t *testing.T) {
	tree := Tree()
	wantPorts := []string{":8080", ":8090", ":8091", ":8092", ":8093", ":8094"}
	got := map[string]bool{}
	for _, iface := range tree {
		if len(iface.Pages) == 0 {
			t.Errorf("интерфейс %s (%s) без страниц", iface.Name, iface.Port)
		}
		got[iface.Port] = true
	}
	for _, p := range wantPorts {
		if !got[p] {
			t.Errorf("в карте нет интерфейса на порту %s", p)
		}
	}
}

// TestFlattenNodes проверяет разворачивание в узлы: страница + блоки, тексты не пусты.
func TestFlattenNodes(t *testing.T) {
	nodes := Flatten(Tree())
	if len(nodes) < 50 {
		t.Fatalf("ожидалось хотя бы 50 навигационных узлов, получено %d", len(nodes))
	}
	pages := 0
	for _, n := range nodes {
		if strings.TrimSpace(n.Text) == "" {
			t.Errorf("узел %s без текста", n.ID)
		}
		if n.Route == "" {
			t.Errorf("узел %s без маршрута", n.ID)
		}
		if n.Kind == "page" {
			pages++
		}
	}
	if pages == 0 {
		t.Error("нет ни одного узла-обзора страницы")
	}
}

// TestNodeIDStable проверяет детерминированность ID (нет дублей при переиндексации).
func TestNodeIDStable(t *testing.T) {
	a := Flatten(Tree())
	b := Flatten(Tree())
	if len(a) != len(b) {
		t.Fatalf("нестабильное число узлов: %d != %d", len(a), len(b))
	}
	seen := map[string]bool{}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("ID узла %d нестабилен: %s != %s", i, a[i].ID, b[i].ID)
		}
		if seen[a[i].ID] {
			t.Errorf("дублирующийся ID узла: %s (%s / %s)", a[i].ID, a[i].Route, a[i].Block)
		}
		seen[a[i].ID] = true
	}
}

// TestMarkdownRender проверяет, что Markdown-карта содержит ключевые элементы.
func TestMarkdownRender(t *testing.T) {
	md := ToMarkdown(Tree())
	for _, want := range []string{"База Сколково", ":8092", "Личный кабинет", "get_navigation", "Карта интерфейсов"} {
		if !strings.Contains(md, want) {
			t.Errorf("в Markdown-карте нет ожидаемого фрагмента: %q", want)
		}
	}
}
