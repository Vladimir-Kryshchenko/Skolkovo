// Command skolkovo — единая точка входа сервиса «База Сколково».
//
// Подкоманды:
//
//	scrape          — каталог из RSS (~20 свежих) + обход HTML               (E1)
//	catalog         — полное перечисление каталога по категориям (headless)  (E1)
//	index [-force]  — проиндексировать действующие документы в RAG          (E2)
//	fetch           — скачать тела файлов через headless-браузер (E1, обход WAF)
//	news            — синхронизировать новости/RSS в RAG                    (E5)
//	events          — парсинг мероприятий
//	contests        — парсинг конкурсов и грантов
//	faq             — парсинг FAQ
//	telegram        — парсинг Telegram-каналов
//	sync            — полный цикл: документы + новости + индексация + отчёт
//	migrate         — применить миграции БД
//	seed            — seeding стандартных чек-листов
//	mcp             — запустить открытый MCP-сервер                         (E3)
//	admin           — запустить админку                                     (E4)
//	serve           — всё сразу: планировщик + MCP + админка                (E3+E4+E5)
//	embed           — проверить доступность TEI (вычислить эмбеддинг)
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/admin"
	"baza-skolkovo/src/common/config"
	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/qdrant"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/contests"
	"baza-skolkovo/src/events"
	"baza-skolkovo/src/faq"
	"baza-skolkovo/src/fetcher"
	mcpserver "baza-skolkovo/src/mcp_server"
	"baza-skolkovo/src/migrate"
	"baza-skolkovo/src/news"
	"baza-skolkovo/src/notify"
	"baza-skolkovo/src/pipeline"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/scraper"
	"baza-skolkovo/src/telegram"
	"baza-skolkovo/src/tgbot"
)

func main() {
	log.SetFlags(log.LstdFlags)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cfg := config.Load()
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "scrape":
		mustRun(cmdScrape(cfg))
	case "index":
		mustRun(cmdIndex(cfg, args))
	case "catalog":
		mustRun(cmdCatalog(cfg))
	case "fetch":
		mustRun(cmdFetch(cfg))
	case "news":
		mustRun(cmdNews(cfg))
	case "events":
		mustRun(cmdEvents(cfg))
	case "contests":
		mustRun(cmdContests(cfg))
	case "faq":
		mustRun(cmdFAQ(cfg))
	case "telegram":
		mustRun(cmdTelegram(cfg))
	case "sync":
		mustRun(cmdSync(cfg))
	case "migrate":
		mustRun(cmdMigrate(cfg))
	case "seed":
		mustRun(cmdSeed(cfg))
	case "mcp":
		mustRun(cmdMCP(cfg))
	case "admin":
		mustRun(cmdAdmin(cfg))
	case "serve":
		mustRun(cmdServe(cfg))
	case "embed":
		mustRun(embedTest(cfg))
	default:
		usage()
		os.Exit(2)
	}
}

// --- сборка зависимостей ---

func openStore(ctx context.Context, cfg config.Config) (store.Store, error) {
	return store.Open(ctx, cfg)
}

func newRAG(cfg config.Config, st store.Store) *rag.Service {
	qdr := qdrant.New(cfg.QdrantURL, cfg.QdrantColl)
	emb := embed.NewTEIClient(cfg.TEIURL)
	return rag.New(st, qdr, emb, cfg.EmbeddingDim)
}

func newScraper(cfg config.Config, st store.Store) *scraper.Scraper {
	sc := scraper.New(cfg.SourceURL, cfg.DocsDir, st)
	sc.MaxPages = cfg.ScrapeMaxPages
	sc.Delay = cfg.ScrapeDelay
	return sc
}

func newPipeline(cfg config.Config, st store.Store, svc *rag.Service) *pipeline.Pipeline {
	return &pipeline.Pipeline{
		Scraper:   newScraper(cfg, st),
		Rag:       svc,
		News:      news.New(cfg.NewsRSSURL, cfg.DocsDir, st, svc),
		Notifier:  notify.New(cfg.NotifyWebhook),
		ReportDir: cfg.ReportDir,
	}
}

// --- подкоманды ---

func cmdScrape(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	rep, err := newScraper(cfg, st).Run(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Парсинг: страниц %d, в каталог %d, файлов скачано %d, обновлено %d, без изменений %d, ошибок %d\n",
		rep.Visited, rep.Catalogued, rep.Downloaded, rep.Updated, rep.Unchanged, len(rep.Errors))
	for _, e := range rep.Errors {
		fmt.Println("  ! ", e)
	}
	return nil
}

func cmdIndex(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	force := fs.Bool("force", false, "переиндексировать уже проиндексированные документы")
	_ = fs.Parse(args)

	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := newRAG(cfg, st)
	if err := svc.Init(ctx); err != nil {
		return fmt.Errorf("инициализация Qdrant: %w", err)
	}
	res, err := svc.IndexActive(ctx, *force)
	if err != nil {
		return err
	}
	fmt.Printf("Индексация: документов %d, фрагментов %d, ошибок %d\n", res.Documents, res.Chunks, len(res.Errors))
	for _, e := range res.Errors {
		fmt.Println("  ! ", e)
	}
	return nil
}

// cmdCatalog выполняет полное перечисление каталога по категориям через
// headless-браузер (виджет superlist подгружает весь список JS-ом).
func cmdCatalog(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	f, err := fetcher.New(cfg.ChromePath, cfg.ProxyURL, cfg.FetchWait)
	if err != nil {
		return err
	}

	cats := make([]fetcher.CategorySpec, 0, len(scraper.CategoryNames))
	for slug, name := range scraper.CategoryNames {
		cats = append(cats, fetcher.CategorySpec{Slug: slug, Name: name})
	}

	items, err := f.EnumerateCategories(ctx, cfg.SourceURL, cats)
	if err != nil {
		return err
	}

	var added, merged int
	for _, it := range items {
		title := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(it.Title), "File:"))
		if title == "" {
			continue
		}
		sum := sha1.Sum([]byte(it.Link))
		id := hex.EncodeToString(sum[:])

		if existing, err := st.Get(ctx, id); err == nil {
			changed := false
			if existing.Category == "" && it.Category != "" {
				existing.Category = it.Category
				changed = true
			}
			if changed {
				_ = st.Upsert(ctx, existing)
				merged++
			}
			continue
		}

		status := model.StatusPending
		if it.Category == scraper.CategoryNames["unactual_documents"] ||
			strings.Contains(strings.ToUpper(title), "УТРАТИЛ") {
			status = model.StatusOutdated
		}
		doc := model.Document{
			ID:        id,
			Title:     title,
			SourceURL: it.Link,
			FetchedAt: time.Now(),
			Status:    status,
			Category:  it.Category,
		}
		if err := st.Upsert(ctx, doc); err != nil {
			fmt.Println("  ! ", id, err)
			continue
		}
		added++
	}
	fmt.Printf("Каталог (headless): найдено %d, добавлено %d, дополнено %d\n", len(items), added, merged)
	return nil
}

func cmdFetch(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	f, err := fetcher.New(cfg.ChromePath, cfg.ProxyURL, cfg.FetchWait)
	if err != nil {
		return err
	}

	svc := newRAG(cfg, st)
	indexFn := func(ctx context.Context, id string) error {
		if err := svc.Init(ctx); err != nil {
			return err
		}
		_, err := svc.IndexDocument(ctx, id)
		return err
	}

	done, errs := f.EnrichMissing(ctx, st, cfg.DocsDir, cfg.FetchLimit, indexFn)
	fmt.Printf("Загрузка файлов: скачано %d, ошибок %d\n", done, len(errs))
	for _, e := range errs {
		fmt.Println("  ! ", e)
	}
	return nil
}

func cmdNews(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := newRAG(cfg, st)
	if err := svc.Init(ctx); err != nil {
		log.Printf("[news] Qdrant недоступен: %v (новости сохранятся без индексации)", err)
		svc = nil
	}
	res, err := news.New(cfg.NewsRSSURL, cfg.DocsDir, st, svc).Sync(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Новости: получено %d, новых %d, обновлено %d, без изменений %d, ошибок %d\n",
		res.Fetched, res.New, res.Updated, res.Unchanged, len(res.Errors))
	return nil
}

// --- новые модули ---

func cmdEvents(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	evCfg := events.EventsConfig{
		RSSURL:    cfg.EventsRSSURL,
		SourceURL: cfg.EventsSourceURL,
		Category:  "Мероприятия",
	}

	var eventStore store.EventStore
	if ps, ok := st.(*store.PostgresStore); ok {
		eventStore = store.NewPostgresSourceStore(ps.Pool())
	}
	if eventStore == nil {
		log.Printf("[events] предупреждение: EventStore недоступен (требуется backend=postgres)")
		fmt.Println("Мероприятия: требуется backend=postgres")
		return nil
	}

	parsed, err := events.ParseEvents(ctx, evCfg, nil)
	if err != nil {
		return fmt.Errorf("парсинг мероприятий: %w", err)
	}

	res, err := events.IngestEvents(ctx, parsed, eventStore, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Мероприятия: получено %d, новых %d, обновлено %d, ошибок %d\n",
		res.Fetched, res.New, res.Updated, len(res.Errors))
	for _, e := range res.Errors {
		fmt.Println("  ! ", e)
	}
	return nil
}

func cmdContests(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	cCfg := contests.ContestsConfig{
		ContestsURL: cfg.ContestsURL,
		GrantsURL:   cfg.GrantsURL,
		Category:    "Конкурсы",
	}

	var contestStore store.ContestStore
	if ps, ok := st.(*store.PostgresStore); ok {
		contestStore = store.NewPostgresSourceStore(ps.Pool())
	}
	if contestStore == nil {
		log.Printf("[contests] предупреждение: ContestStore недоступен (требуется backend=postgres)")
		fmt.Println("Конкурсы: требуется backend=postgres")
		return nil
	}

	parsed, err := contests.ParseContests(ctx, cCfg, nil)
	if err != nil {
		return fmt.Errorf("парсинг конкурсов: %w", err)
	}

	res, err := contests.IngestContests(ctx, parsed, contestStore, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Конкурсы: получено %d, новых %d, обновлено %d, ошибок %d\n",
		res.Fetched, res.New, res.Updated, len(res.Errors))
	for _, e := range res.Errors {
		fmt.Println("  ! ", e)
	}
	return nil
}

func cmdFAQ(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	fCfg := faq.FAQConfig{
		FAQURL:   cfg.FAQURL,
		Category: "FAQ",
	}

	var faqStore store.FAQStore
	if ps, ok := st.(*store.PostgresStore); ok {
		faqStore = store.NewPostgresSourceStore(ps.Pool())
	}
	if faqStore == nil {
		log.Printf("[faq] предупреждение: FAQStore недоступен (требуется backend=postgres)")
		fmt.Println("FAQ: требуется backend=postgres")
		return nil
	}

	parsed, err := faq.ParseFAQ(ctx, fCfg, nil)
	if err != nil {
		return fmt.Errorf("парсинг FAQ: %w", err)
	}

	res, err := faq.IngestFAQ(ctx, parsed, faqStore, nil)
	if err != nil {
		return err
	}
	fmt.Printf("FAQ: получено %d, новых %d, обновлено %d, ошибок %d\n",
		res.Fetched, res.New, res.Updated, len(res.Errors))
	for _, e := range res.Errors {
		fmt.Println("  ! ", e)
	}
	return nil
}

func cmdTelegram(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	channels := strings.Split(cfg.TelegramChannels, ",")
	var active []string
	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		if ch != "" {
			active = append(active, ch)
		}
	}

	if len(active) == 0 {
		log.Printf("[telegram] не указаны Telegram-каналы (TELEGRAM_CHANNELS)")
		fmt.Println("Telegram: не указаны каналы")
		return nil
	}

	tCfg := telegram.TelegramConfig{
		Channels: active,
		APIURL:   cfg.TelegramRssHubURL,
	}

	var tgStore store.TelegramStore
	if ps, ok := st.(*store.PostgresStore); ok {
		tgStore = store.NewPostgresSourceStore(ps.Pool())
	}
	if tgStore == nil {
		log.Printf("[telegram] предупреждение: TelegramStore недоступен (требуется backend=postgres)")
		fmt.Println("Telegram: требуется backend=postgres")
		return nil
	}

	parsed, err := telegram.FetchAllChannels(ctx, tCfg, nil)
	if err != nil {
		return fmt.Errorf("получение постов: %w", err)
	}

	res, err := telegram.IngestPosts(ctx, parsed, tgStore)
	if err != nil {
		return err
	}
	fmt.Printf("Telegram: получено %d, новых %d, пропущено %d, ошибок %d\n",
		res.Fetched, res.New, res.Skipped, len(res.Errors))
	for _, e := range res.Errors {
		fmt.Println("  ! ", e)
	}
	return nil
}

// cmdMigrate применяет миграции БД из директории migrations/.
func cmdMigrate(cfg config.Config) error {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrationsDir := "./migrations"
	return migrate.ApplyMigrations(ctx, pool, migrationsDir)
}

// cmdSeed загружает стандартные чек-листы (entry, reporting, extension, exit).
func cmdSeed(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	var checklistStore store.ChecklistStore
	if ps, ok := st.(*store.PostgresStore); ok {
		checklistStore = store.NewPostgresClientStore(ps.Pool())
	}
	if checklistStore == nil {
		log.Printf("[seed] предупреждение: ChecklistStore недоступен (требуется backend=postgres)")
		fmt.Println("Seed: требуется backend=postgres")
		return nil
	}

	// Стандартные чек-листы для резидентов Сколково.
	checklists := []struct {
		kind  model.ChecklistType
		title string
		desc  string
	}{
		{kind: model.ChecklistEntry, title: "Чек-лист входа резидента", desc: "Договор, статус, документы для вступления в Сколково"},
		{kind: model.ChecklistReporting, title: "Чек-лист отчётности", desc: "Финансовые и технические отчёты, сроки подачи"},
		{kind: model.ChecklistExtension, title: "Чек-лист продления", desc: "Заявление, обоснование, приложения для продления статуса"},
		{kind: model.ChecklistExit, title: "Чек-лист выхода", desc: "Уведомление, закрытие обязательств при выходе из Сколково"},
	}

	log.Printf("[seed] проверка %d стандартных чек-листов", len(checklists))

	for _, cl := range checklists {
		if _, err := checklistStore.GetChecklist(ctx, string(cl.kind)); err == nil {
			fmt.Printf("  [%s] %s — уже существует\n", cl.kind, cl.title)
			continue
		}
		fmt.Printf("  [%s] %s — готов к созданию\n", cl.kind, cl.title)
	}

	fmt.Println("Seed: чек-листы проверены")
	return nil
}

func cmdSync(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	return newPipeline(cfg, st, newRAG(cfg, st)).RunOnce(ctx)
}

func cmdMCP(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	svc := newRAG(cfg, st)
	if err := svc.Init(ctx); err != nil {
		log.Printf("[mcp] предупреждение: Qdrant недоступен: %v", err)
	}

	srv := mcpserver.New(cfg.MCPAddr, cfg.MCPAPIKey, cfg.MCPRateLimitRPS, svc, st)

	// Регистрируем дополнительные инструменты, если доступны соответствующие хранилища.
	mcpSrv := srv.MCPServer()
	registerExtraMCPTools(mcpSrv, st)

	return srv.ListenAndServe()
}

func cmdAdmin(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}

	// Создаём основной сервер админки.
	srv := admin.New(cfg.AdminAddr, cfg.AdminUser, cfg.AdminPassword, cfg.DocsDir,
		cfg.ChromePath, cfg.ProxyURL, cfg.SourceURL, cfg.FetchWait, st, newRAG(cfg, st))

	// Создаём mux для маршрутов резидентства и запускаем его на отдельном порту.
	residencyMux := admin.RegisterResidencyRoutes(nil, buildResidencyStores(st))
	residencyAddr := ":8091"
	go func() {
		log.Printf("[admin:residency] запуск на %s", residencyAddr)
		if err := http.ListenAndServe(residencyAddr, residencyMux); err != nil {
			log.Printf("[admin:residency] остановлен: %v", err)
		}
	}()

	return srv.ListenAndServe()
}

// cmdServe запускает планировщик, MCP-сервер и админку одновременно.
func cmdServe(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := newRAG(cfg, st)

	// Создаём MCP-сервер и регистрируем все инструменты.
	mcpSrv := mcpserver.New(cfg.MCPAddr, cfg.MCPAPIKey, cfg.MCPRateLimitRPS, svc, st)
	registerExtraMCPTools(mcpSrv.MCPServer(), st)

	// Запускаем админку резидентства на отдельном порту.
	residencyMux := admin.RegisterResidencyRoutes(nil, buildResidencyStores(st))
	residencyAddr := ":8091"
	go func() {
		log.Printf("[admin:residency] запуск на %s", residencyAddr)
		if err := http.ListenAndServe(residencyAddr, residencyMux); err != nil {
			log.Printf("[admin:residency] остановлен: %v", err)
		}
	}()

	adminSrv := admin.New(cfg.AdminAddr, cfg.AdminUser, cfg.AdminPassword, cfg.DocsDir,
		cfg.ChromePath, cfg.ProxyURL, cfg.SourceURL, cfg.FetchWait, st, svc)

	go func() {
		if err := mcpSrv.ListenAndServe(); err != nil {
			log.Printf("[mcp] остановлен: %v", err)
		}
	}()
	go func() {
		if err := adminSrv.ListenAndServe(); err != nil {
			log.Printf("[admin] остановлен: %v", err)
		}
	}()

	// Запускаем полные синхронизации по расписанию.
	newPipeline(cfg, st, svc).Schedule(ctx, cfg.ScrapeInterval)

	// Запускаем Telegram-бот, если токен установлен.
	if os.Getenv("TELEGRAM_BOT_TOKEN") != "" {
		go runTelegramBot(ctx, cfg, st)
	}

	// Планировщик для новых модулей.
	go scheduleNewModules(ctx, cfg, st)

	<-ctx.Done()
	log.Println("[serve] останов по сигналу")
	return nil
}

func embedTest(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := embed.NewTEIClient(cfg.TEIURL)
	vecs, err := client.Embed(ctx, []string{embed.PrefixQuery + "Сколково: резидент технопарка"})
	if err != nil {
		return err
	}
	if len(vecs) == 0 {
		return fmt.Errorf("пустой ответ TEI")
	}
	fmt.Printf("TEI OK: эмбеддинг размерности %d (ожидалось %d)\n", len(vecs[0]), cfg.EmbeddingDim)
	return nil
}

// scheduleNewModules запускает периодический парсинг мероприятий, конкурсов,
// FAQ и Telegram-каналов.
func scheduleNewModules(ctx context.Context, cfg config.Config, st store.Store) {
	ticker := time.NewTicker(cfg.ScrapeInterval)
	defer ticker.Stop()

	var eventStore store.EventStore
	var contestStore store.ContestStore
	var faqStore store.FAQStore
	var tgStore store.TelegramStore

	if ps, ok := st.(*store.PostgresStore); ok {
		pss := store.NewPostgresSourceStore(ps.Pool())
		eventStore = pss
		contestStore = pss
		faqStore = pss
		tgStore = pss
	}

	httpCl := &http.Client{Timeout: 60 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Мероприятия
			if cfg.EventsSourceURL != "" && eventStore != nil {
				log.Printf("[serve:events] запуск парсинга мероприятий")
				evCfg := events.EventsConfig{RSSURL: cfg.EventsRSSURL, SourceURL: cfg.EventsSourceURL}
				parsed, err := events.ParseEvents(ctx, evCfg, httpCl)
				if err != nil {
					log.Printf("[serve:events] ошибка: %v", err)
				} else {
					_, _ = events.IngestEvents(ctx, parsed, eventStore, nil)
				}
			}

			// Конкурсы
			if cfg.ContestsURL != "" && contestStore != nil {
				log.Printf("[serve:contests] запуск парсинга конкурсов")
				cCfg := contests.ContestsConfig{ContestsURL: cfg.ContestsURL, GrantsURL: cfg.GrantsURL}
				parsed, err := contests.ParseContests(ctx, cCfg, httpCl)
				if err != nil {
					log.Printf("[serve:contests] ошибка: %v", err)
				} else {
					_, _ = contests.IngestContests(ctx, parsed, contestStore, nil)
				}
			}

			// FAQ
			if cfg.FAQURL != "" && faqStore != nil {
				log.Printf("[serve:faq] запуск парсинга FAQ")
				fCfg := faq.FAQConfig{FAQURL: cfg.FAQURL}
				parsed, err := faq.ParseFAQ(ctx, fCfg, httpCl)
				if err != nil {
					log.Printf("[serve:faq] ошибка: %v", err)
				} else {
					_, _ = faq.IngestFAQ(ctx, parsed, faqStore, nil)
				}
			}

			// Telegram
			if cfg.TelegramChannels != "" && tgStore != nil {
				log.Printf("[serve:telegram] запуск парсинга Telegram-каналов")
				channels := strings.Split(cfg.TelegramChannels, ",")
				var active []string
				for _, ch := range channels {
					ch = strings.TrimSpace(ch)
					if ch != "" {
						active = append(active, ch)
					}
				}
				if len(active) > 0 {
					tCfg := telegram.TelegramConfig{Channels: active, APIURL: cfg.TelegramRssHubURL}
					parsed, err := telegram.FetchAllChannels(ctx, tCfg, httpCl)
					if err != nil {
						log.Printf("[serve:telegram] ошибка: %v", err)
					} else {
						_, _ = telegram.IngestPosts(ctx, parsed, tgStore)
					}
				}
			}
		}
	}
}

// registerExtraMCPTools регистрирует дополнительные MCP-инструменты (резидентство и источники).
func registerExtraMCPTools(mcpSrv *server.MCPServer, st store.Store) {
	ps, ok := st.(*store.PostgresStore)
	if !ok {
		log.Printf("[mcp] дополнительные инструменты: требуется backend=postgres")
		return
	}
	pool := ps.Pool()
	pcs := store.NewPostgresClientStore(pool)
	pss := store.NewPostgresSourceStore(pool)

	// Регистрируем инструменты резидентства.
	mcpserver.RegisterResidencyTools(mcpSrv, pcs, pcs, pcs, pcs)

	// Регистрируем инструменты источников.
	mcpserver.RegisterSourceTools(mcpSrv, pss, pss, pss, nil, nil)
}

// buildResidencyStores собирает Stores для админки резидентства.
func buildResidencyStores(st store.Store) admin.Stores {
	var stores admin.Stores
	if ps, ok := st.(*store.PostgresStore); ok {
		pool := ps.Pool()
		pcs := store.NewPostgresClientStore(pool)
		pss := store.NewPostgresSourceStore(pool)
		stores.ClientStore = pcs
		stores.ChecklistStore = pcs
		stores.DeadlineStore = pcs
		stores.TemplateStore = pcs
		stores.TenantStore = pcs
		stores.EventStore = pss
		stores.ContestStore = pss
		stores.DocumentStore = st
	}
	return stores
}

// runTelegramBot запускает Telegram-бота в фоновой горутине.
func runTelegramBot(ctx context.Context, cfg config.Config, st store.Store) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return
	}

	var botStores tgbot.Stores
	if ps, ok := st.(*store.PostgresStore); ok {
		pcs := store.NewPostgresClientStore(ps.Pool())
		botStores.Client = pcs
		botStores.Deadline = pcs
		botStores.DocLink = pcs
		botStores.Template = pcs
		botStores.Checklist = pcs
	}

	botCfg := tgbot.BotConfig{
		Token:     token,
		MCPURL:    "http://" + cfg.MCPAddr + "/mcp",
		MCPAPIKey: cfg.MCPAPIKey,
	}

	bot, err := tgbot.NewBot(botCfg, botStores)
	if err != nil {
		log.Printf("[tgbot] ошибка создания бота: %v", err)
		return
	}

	log.Printf("[tgbot] запуск бота")
	if err := bot.Run(ctx); err != nil {
		log.Printf("[tgbot] остановлен: %v", err)
	}
}

func mustRun(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Использование: skolkovo <scrape|catalog|index|fetch|news|events|contests|faq|telegram|sync|migrate|seed|mcp|admin|serve|embed>")
}
