// coverage.go — построение отчёта о полноте охвата источников Сколково.
// Вызывается MCP-инструментом get_coverage_audit и CLI-командой `audit`.
package audit

import (
	"context"
	"strings"
	"time"

	"baza-skolkovo/src/common/config"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/health"
)

// BuildCoverageReport собирает отчёт о полноте охвата источников из конфигурации,
// мониторинга свежести и фактических счётчиков в хранилищах.
func BuildCoverageReport(ctx context.Context, cfg config.Config, st store.Store) Report {
	// Состояние свежести по имени источника.
	healthByName := map[string]string{}
	itemsLastRun := map[string]int{}
	if ps, ok := st.(*store.PostgresStore); ok {
		if hs, err := health.NewPostgresStore(ctx, ps.Pool()); err == nil {
			if sources, err := hs.List(ctx); err == nil {
				now := time.Now()
				for _, s := range sources {
					healthByName[s.Name] = string(s.State(24*time.Hour, now))
					itemsLastRun[s.Name] = s.ItemsLastRun
				}
			}
		}
	}

	// Счётчики документов по категориям.
	docsTotal, newsN, prefN, npaN, fetchedN := -1, -1, -1, -1, -1
	if docs, err := st.List(ctx, store.Filter{}); err == nil {
		docsTotal, newsN, prefN, npaN, fetchedN = 0, 0, 0, 0, 0
		for _, d := range docs {
			switch d.Category {
			case "Новости":
				newsN++
			case "Льготы":
				prefN++
			case "НПА":
				npaN++
			default:
				docsTotal++
			}
			if strings.TrimSpace(d.LocalPath) != "" {
				fetchedN++
			}
		}
	}

	// Счётчики расширенных источников.
	eventsN, contestsN, faqN, tgN, residentsN := -1, -1, -1, -1, -1
	if ps, ok := st.(*store.PostgresStore); ok {
		pss := store.NewPostgresSourceStore(ps.Pool())
		if n, err := pss.CountEvents(ctx); err == nil {
			eventsN = n
		}
		if n, err := pss.CountActiveContests(ctx); err == nil {
			contestsN = n
		}
		if n, err := pss.CountFAQItems(ctx); err == nil {
			faqN = n
		}
		if n, err := pss.CountPosts(ctx, ""); err == nil {
			tgN = n
		}
		if n, err := pss.CountResidents(ctx); err == nil {
			residentsN = n
		}
	}

	cov := []Coverage{
		{Key: "documents", Name: "Документы dochub.sk.ru", URL: cfg.SourceURL, Enabled: cfg.SourceURL != "", HealthState: healthByName["documents"], ItemsLastRun: itemsLastRun["documents"], Items: docsTotal},
		{Key: "fetch", Name: "Тела файлов документов (обход WAF)", URL: cfg.SourceURL, Enabled: true, HealthState: healthByName["fetch"], ItemsLastRun: itemsLastRun["fetch"], Items: fetchedN},
		{Key: "news", Name: "Новости sk.ru", URL: cfg.NewsRSSURL, Enabled: cfg.NewsRSSURL != "", HealthState: healthByName["news"], ItemsLastRun: itemsLastRun["news"], Items: newsN},
		{Key: "events", Name: "Мероприятия", URL: cfg.EventsSourceURL, Enabled: cfg.EventsSourceURL != "", HealthState: healthByName["events"], ItemsLastRun: itemsLastRun["events"], Items: eventsN},
		{Key: "contests", Name: "Конкурсы и гранты", URL: cfg.ContestsURL, Enabled: cfg.ContestsURL != "", HealthState: healthByName["contests"], ItemsLastRun: itemsLastRun["contests"], Items: contestsN},
		{Key: "faq", Name: "FAQ", URL: cfg.FAQURL, Enabled: cfg.FAQURL != "", HealthState: healthByName["faq"], ItemsLastRun: itemsLastRun["faq"], Items: faqN},
		{Key: "preferences", Name: "Льготы резидентов", URL: cfg.PreferencesURL, Enabled: cfg.PreferencesEnabled, HealthState: healthByName["preferences"], ItemsLastRun: itemsLastRun["preferences"], Items: prefN},
		{Key: "regulations", Name: "НПА (244-ФЗ и поправки)", URL: cfg.RegulationsSearchURL, Enabled: cfg.RegulationsEnabled, HealthState: healthByName["regulations"], ItemsLastRun: itemsLastRun["regulations"], Items: npaN},
		{Key: "telegram", Name: "Telegram-каналы", URL: cfg.TelegramRssHubURL, Enabled: cfg.TelegramChannels != "", HealthState: healthByName["telegram"], ItemsLastRun: itemsLastRun["telegram"], Items: tgN},
		{Key: "residents", Name: "Реестр резидентов", URL: cfg.ResidentsURL, Enabled: cfg.ResidentsEnabled, HealthState: healthByName["residents"], ItemsLastRun: itemsLastRun["residents"], Items: residentsN},
	}

	rep := Build(cov)
	rep.GeneratedAt = time.Now()
	return rep
}
