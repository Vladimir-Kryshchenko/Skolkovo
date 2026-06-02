package docenrich

import (
	"reflect"
	"testing"
)

func TestCanonicalCategory(t *testing.T) {
	cases := map[string]string{
		"Законодательные акты":   "Законодательные акты",
		"законодательные  акты":  "Законодательные акты", // регистр + лишние пробелы
		"Утратившие силу":        "Утратившие силу",
		"Произвольная категория": "", // не из списка
		"":                       "",
	}
	for in, want := range cases {
		if got := canonicalCategory(in); got != want {
			t.Errorf("canonicalCategory(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	known := []string{"налоги", "градостроительство"}
	in := []string{"Налоги", "  ИНН  ", "налоги", "градостроительство", ""}
	got := normalizeTags(in, known, 8)
	// известные теги (налоги, градостроительство) — первыми; дубликаты убраны; нижний регистр.
	want := []string{"налоги", "градостроительство", "инн"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeTags = %v, want %v", got, want)
	}
	// лимит
	if g := normalizeTags([]string{"a", "b", "c"}, nil, 2); len(g) != 2 {
		t.Errorf("лимит: got %d тегов, want 2", len(g))
	}
	// всегда не-nil
	if g := normalizeTags(nil, nil, 8); g == nil {
		t.Error("normalizeTags(nil) вернул nil, ожидался пустой срез")
	}
}

func TestParseClassification(t *testing.T) {
	raw := "Вот результат:\n```json\n{\"category\":\"Антикоррупция\",\"subcategory\":\"Декларации\",\"tags\":[\"коррупция\",\"декларация\"]}\n```\nготово"
	c, err := parseClassification(raw)
	if err != nil {
		t.Fatalf("parseClassification: %v", err)
	}
	if c.Category != "Антикоррупция" || c.Subcategory != "Декларации" || len(c.Tags) != 2 {
		t.Errorf("неверный разбор: %+v", c)
	}
	if _, err := parseClassification("нет json"); err == nil {
		t.Error("ожидалась ошибка при отсутствии JSON")
	}
}
