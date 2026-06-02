package sitepages

import (
	"strings"
	"testing"
)

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(Annotation) bool
	}{
		{
			name: "чистый JSON",
			raw:  `{"tags":["льготы","резиденты"],"summary":"О льготах","goals":"Помочь резиденту","theses":["тезис 1","тезис 2"],"conclusions":"Вывод"}`,
			check: func(a Annotation) bool {
				return len(a.Tags) == 2 && a.Summary == "О льготах" && len(a.Theses) == 2 && a.Conclusions == "Вывод"
			},
		},
		{
			name: "в markdown-ограждении",
			raw:  "```json\n{\"tags\":[\"гранты\"],\"summary\":\"S\",\"goals\":\"G\",\"theses\":[\"t\"],\"conclusions\":\"C\"}\n```",
			check: func(a Annotation) bool {
				return len(a.Tags) == 1 && a.Tags[0] == "гранты"
			},
		},
		{
			name: "с поясняющим текстом вокруг",
			raw:  "Вот аннотация:\n{\"tags\":[\"a\"],\"summary\":\"s\"}\nНадеюсь, помогло.",
			check: func(a Annotation) bool {
				return a.Summary == "s" && len(a.Tags) == 1
			},
		},
		{
			name:    "нет JSON",
			raw:     "К сожалению, не могу аннотировать.",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := parseAnnotation(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получили %+v", a)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if tc.check != nil && !tc.check(a) {
				t.Errorf("проверка не прошла для %+v", a)
			}
		})
	}
}

func TestNormalizeTags(t *testing.T) {
	t.Run("регистр, пробелы, дедуп", func(t *testing.T) {
		got := normalizeTags([]string{" Льготы ", "ЛЬГОТЫ", "резиденты"}, nil, 8)
		if len(got) != 2 {
			t.Fatalf("ожидали 2 уникальных тега, получили %v", got)
		}
		if got[0] != "льготы" || got[1] != "резиденты" {
			t.Errorf("ожидали нижний регистр и дедуп, получили %v", got)
		}
	})

	t.Run("известные теги имеют приоритет при обрезке", func(t *testing.T) {
		known := []string{"гранты"}
		// max=1: известный «гранты» должен вытеснить новый «новьё».
		got := normalizeTags([]string{"новьё", "гранты"}, known, 1)
		if len(got) != 1 || got[0] != "гранты" {
			t.Errorf("ожидали приоритет известного тега, получили %v", got)
		}
	})

	t.Run("всегда не-nil и не превышает лимит", func(t *testing.T) {
		got := normalizeTags(nil, nil, 3)
		if got == nil {
			t.Fatal("ожидали не-nil срез")
		}
		many := []string{"a", "b", "c", "d", "e"}
		if g := normalizeTags(many, nil, 3); len(g) != 3 {
			t.Errorf("ожидали обрезку до 3, получили %v", g)
		}
	})

	t.Run("пустые и слишком длинные отбрасываются", func(t *testing.T) {
		long := strings.Repeat("я", 50)
		got := normalizeTags([]string{"", "  ", long, "ок"}, nil, 8)
		if len(got) != 1 || got[0] != "ок" {
			t.Errorf("ожидали только «ок», получили %v", got)
		}
	})
}

func TestCleanList(t *testing.T) {
	got := cleanList([]string{" тезис ", "тезис", "", "Другой"}, 8)
	if len(got) != 2 {
		t.Fatalf("ожидали 2 элемента после дедупа/очистки, получили %v", got)
	}
	if got[0] != "тезис" || got[1] != "Другой" {
		t.Errorf("неверная очистка: %v", got)
	}
	if g := cleanList([]string{"1", "2", "3"}, 2); len(g) != 2 {
		t.Errorf("ожидали обрезку до 2, получили %v", g)
	}
}

func TestBuildAnnotatePrompt(t *testing.T) {
	p := &Page{
		Title:   "Льготы резидентам",
		URL:     "https://sk.ru/residents/preferences/",
		Section: "residents / preferences",
		Text:    strings.Repeat("длинный текст ", 1000), // заведомо длиннее лимита
	}
	out := buildAnnotatePrompt(p, []string{"льготы", "резиденты"})
	if !strings.Contains(out, "Льготы резидентам") {
		t.Error("в промпте нет заголовка")
	}
	if !strings.Contains(out, "льготы") {
		t.Error("в промпте нет списка известных тегов")
	}
	if !strings.Contains(out, "…") {
		t.Error("длинный текст должен быть усечён (маркер …)")
	}
	if len([]rune(out)) > maxPromptText+1000 {
		t.Errorf("промпт неожиданно длинный: %d рун", len([]rune(out)))
	}
}
