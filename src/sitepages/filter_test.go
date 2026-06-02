package sitepages

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestSearchFilterExcludesGone проверяет, что поиск по страницам всегда требует
// status=active (мёртвые страницы не попадают в выдачу) и корректно добавляет
// фильтр по тегам.
func TestSearchFilterExcludesGone(t *testing.T) {
	statusCond := map[string]any{"key": "status", "match": map[string]any{"value": StatusActive}}

	t.Run("без тегов — только активные", func(t *testing.T) {
		f := searchFilter(nil)
		must, _ := f["must"].([]any)
		if len(must) != 1 || !reflect.DeepEqual(must[0], statusCond) {
			t.Fatalf("ожидали единственное условие status=active, получили %v", must)
		}
	})

	t.Run("с тегами — активные И все теги", func(t *testing.T) {
		f := searchFilter([]string{"льготы", "", "резиденты"})
		must, _ := f["must"].([]any)
		// status + 2 непустых тега = 3 условия (пустой тег отброшен).
		if len(must) != 3 {
			t.Fatalf("ожидали 3 условия (status + 2 тега), получили %d: %v", len(must), must)
		}
		if !reflect.DeepEqual(must[0], statusCond) {
			t.Errorf("первое условие должно быть status=active, получили %v", must[0])
		}
	})
}

// TestBuildSitePageWhere проверяет, что условие WHERE и аргументы собираются
// согласованно (правильная нумерация плейсхолдеров $1..$N и набор условий).
func TestBuildSitePageWhere(t *testing.T) {
	t.Run("пустой фильтр — без WHERE", func(t *testing.T) {
		where, args := buildSitePageWhere(PageFilter{})
		if where != "" || args != nil {
			t.Fatalf("ожидали пустой фильтр, получили where=%q args=%v", where, args)
		}
	})

	t.Run("все поля", func(t *testing.T) {
		since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		until := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
		where, args := buildSitePageWhere(PageFilter{
			Query:   "льготы",
			Section: "residents",
			Status:  "active",
			Tags:    []string{"налоги", "резиденты"},
			Since:   since,
			Until:   until,
		})
		if !strings.HasPrefix(where, " WHERE ") {
			t.Fatalf("ожидали префикс WHERE, получили %q", where)
		}
		// 6 условий → 6 аргументов (теги передаются одним аргументом-массивом).
		if len(args) != 6 {
			t.Fatalf("ожидали 6 аргументов, получили %d: %v", len(args), args)
		}
		for _, want := range []string{"title ILIKE $1", "section = $2", "status = $3", "tags @> $4::text[]", "last_changed >= $5", "last_changed <= $6"} {
			if !strings.Contains(where, want) {
				t.Errorf("в WHERE нет %q: %s", want, where)
			}
		}
		// Поисковый аргумент обёрнут в проценты для ILIKE.
		if args[0] != "%льготы%" {
			t.Errorf("ожидали %%льготы%%, получили %v", args[0])
		}
	})

	t.Run("только теги — нумерация с $1", func(t *testing.T) {
		where, args := buildSitePageWhere(PageFilter{Tags: []string{"a"}})
		if !strings.Contains(where, "tags @> $1::text[]") {
			t.Errorf("ожидали tags @> $1, получили %q", where)
		}
		if len(args) != 1 {
			t.Errorf("ожидали 1 аргумент, получили %d", len(args))
		}
	})
}
