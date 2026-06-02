package agents

import (
	"context"
	"strings"
	"testing"

	"baza-skolkovo/src/navindex"
)

type stubNav struct {
	hits []navindex.Hit
	err  error
}

func (s stubNav) Search(_ context.Context, _ string, _ int) ([]navindex.Hit, error) {
	return s.hits, s.err
}

func TestSearchNavFiltersByScore(t *testing.T) {
	a := NewConsultantAgent(nil, "", "").WithNavigation(stubNav{hits: []navindex.Hit{
		{PageTitle: "Карточка клиента", Score: 0.91},
		{PageTitle: "Слабый", Score: 0.40},
	}})
	got := a.searchNav(context.Background(), "где сменить стадию")
	if len(got) != 1 || got[0].PageTitle != "Карточка клиента" {
		t.Fatalf("ожидался 1 узел выше порога, получено %+v", got)
	}
}

func TestSearchNavNilWhenDisabled(t *testing.T) {
	a := NewConsultantAgent(nil, "", "") // без WithNavigation
	if got := a.searchNav(context.Background(), "q"); got != nil {
		t.Errorf("без навигации ожидался nil, получено %+v", got)
	}
}

func TestBuildNavBlockDedupAndLimit(t *testing.T) {
	hits := []navindex.Hit{
		{PageTitle: "Карточка клиента", Interface: "Резидентство-Админ", Port: ":8091", Route: "/clients/{id}", HowTo: "Кнопка «Карточка»", Score: 0.9},
		{PageTitle: "Карточка клиента", Interface: "Резидентство-Админ", Port: ":8091", Route: "/clients/{id}", HowTo: "дубль той же страницы", Score: 0.88},
		{PageTitle: "Клиенты", Interface: "Резидентство-Админ", Port: ":8091", Route: "/clients", HowTo: "Меню «Клиенты»", Score: 0.85},
	}
	block := buildNavBlock(hits)
	if !strings.Contains(block, "Где это в системе") {
		t.Error("нет заголовка навигационного блока")
	}
	if strings.Count(block, "/clients/{id}") != 1 {
		t.Errorf("страница не дедуплицирована:\n%s", block)
	}
	if !strings.Contains(block, "Кнопка «Карточка»") {
		t.Errorf("нет инструкции «как попасть»:\n%s", block)
	}
}

func TestBuildNavBlockEmpty(t *testing.T) {
	if buildNavBlock(nil) != "" {
		t.Error("для пустых результатов блок должен быть пустым")
	}
}
