package sitepages

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresEnrichmentIntegration проверяет SQL-методы обогащения против РЕАЛЬНОГО
// Postgres. Запускается только если задан SITEPAGES_TEST_DSN (отдельная переменная,
// чтобы тест не стартовал случайно против прод-БД). Все тестовые данные создаются с
// узнаваемыми префиксами и удаляются в конце.
//
// Пример запуска:
//
//	SITEPAGES_TEST_DSN='postgres://skolkovo:skolkovo@localhost:5432/skolkovo?sslmode=disable' \
//	  go test ./src/sitepages/ -run TestPostgresEnrichmentIntegration -v
func TestPostgresEnrichmentIntegration(t *testing.T) {
	dsn := os.Getenv("SITEPAGES_TEST_DSN")
	if dsn == "" {
		t.Skip("SITEPAGES_TEST_DSN не задан — пропускаем интеграционный тест Postgres")
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("подключение к Postgres: %v", err)
	}
	defer pool.Close()

	const (
		urlPrefix = "https://sitepages-itest.example/"
		tagPrefix = "itest-"
	)
	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM site_pages WHERE url LIKE $1`, urlPrefix+"%")
		_, _ = pool.Exec(ctx, `DELETE FROM site_page_tags WHERE tag LIKE $1`, tagPrefix+"%")
	}
	cleanup()
	defer cleanup()

	st, err := NewPostgresStore(ctx, pool)
	if err != nil {
		t.Fatalf("NewPostgresStore (применение схемы): %v", err)
	}

	// 1. Заводим страницу — она ещё не аннотирована.
	p1 := &Page{
		ID: pageID(urlPrefix + "a"), URL: urlPrefix + "a",
		Title: "Тестовая А", Summary: "сумма", Text: "текст А",
		Section: "itest", ContentHash: "hash-a-1", Status: StatusActive,
	}
	if _, err := st.Upsert(ctx, p1); err != nil {
		t.Fatalf("Upsert p1: %v", err)
	}

	// 2. ListNeedingEnrichment должен включать новую страницу.
	pend, err := st.ListNeedingEnrichment(ctx, 0)
	if err != nil {
		t.Fatalf("ListNeedingEnrichment: %v", err)
	}
	if !containsURL(pend, p1.URL) {
		t.Fatalf("новая страница не попала в ListNeedingEnrichment")
	}
	// Текст должен подгружаться (нужен для аннотирования).
	for _, p := range pend {
		if p.URL == p1.URL && p.Text != "текст А" {
			t.Errorf("ListNeedingEnrichment не вернул текст: %q", p.Text)
		}
	}

	// 3. Сохраняем аннотацию и фиксируем enrich_hash = content_hash.
	ann := Annotation{
		Tags:        []string{tagPrefix + "a", tagPrefix + "b"},
		Summary:     "ИИ-описание А",
		Goals:       "Цель А",
		Theses:      []string{"тезис 1", "тезис 2"},
		Conclusions: "Вывод А",
	}
	if err := st.UpdateEnrichment(ctx, p1.ID, ann, p1.ContentHash); err != nil {
		t.Fatalf("UpdateEnrichment: %v", err)
	}
	_ = st.BumpTags(ctx, ann.Tags)

	// 4. Get возвращает ИИ-поля; страница считается аннотированной.
	got, err := st.Get(ctx, p1.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Enriched() {
		t.Error("страница должна быть Enriched после UpdateEnrichment")
	}
	if len(got.Tags) != 2 || got.AISummary != "ИИ-описание А" || len(got.Theses) != 2 || got.Conclusions != "Вывод А" {
		t.Errorf("ИИ-поля не сохранились/не прочитались: %+v", got)
	}

	// 5. После аннотирования страница больше не «нуждается» (enrich_hash совпал).
	pend2, err := st.ListNeedingEnrichment(ctx, 0)
	if err != nil {
		t.Fatalf("ListNeedingEnrichment #2: %v", err)
	}
	if containsURL(pend2, p1.URL) {
		t.Error("аннотированная страница не должна попадать в ListNeedingEnrichment")
	}

	// 6. Словарь тегов пополнился.
	tags, err := st.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if !containsStr(tags, tagPrefix+"a") || !containsStr(tags, tagPrefix+"b") {
		t.Errorf("словарь тегов не содержит ожидаемых тегов: %v", tags)
	}

	// 7. RelatedByTags: вторая страница с пересекающимся тегом.
	p2 := &Page{
		ID: pageID(urlPrefix + "b"), URL: urlPrefix + "b",
		Title: "Тестовая Б", ContentHash: "hash-b-1", Status: StatusActive,
	}
	if _, err := st.Upsert(ctx, p2); err != nil {
		t.Fatalf("Upsert p2: %v", err)
	}
	if err := st.UpdateEnrichment(ctx, p2.ID, Annotation{Tags: []string{tagPrefix + "b", tagPrefix + "c"}}, p2.ContentHash); err != nil {
		t.Fatalf("UpdateEnrichment p2: %v", err)
	}
	rel, err := st.RelatedByTags(ctx, p1.ID, got.Tags, 6)
	if err != nil {
		t.Fatalf("RelatedByTags: %v", err)
	}
	var foundB bool
	for _, r := range rel {
		if r.URL == p2.URL {
			foundB = true
			if r.Shared < 1 {
				t.Errorf("ожидали ≥1 общий тег, получили %d", r.Shared)
			}
		}
	}
	if !foundB {
		t.Errorf("RelatedByTags не нашёл страницу с общим тегом: %+v", rel)
	}

	// 8. ContentHash изменился → страница снова нуждается в аннотировании.
	p1.ContentHash = "hash-a-2"
	if _, err := st.Upsert(ctx, p1); err != nil {
		t.Fatalf("Upsert p1 (изменение контента): %v", err)
	}
	pend3, err := st.ListNeedingEnrichment(ctx, 0)
	if err != nil {
		t.Fatalf("ListNeedingEnrichment #3: %v", err)
	}
	if !containsURL(pend3, p1.URL) {
		t.Error("после изменения контента страница должна снова нуждаться в аннотировании")
	}
}

func containsURL(pages []*Page, url string) bool {
	for _, p := range pages {
		if p.URL == url {
			return true
		}
	}
	return false
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
