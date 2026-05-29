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
	"encoding/json"
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
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"baza-skolkovo/src/admin"
	"baza-skolkovo/src/agents"
	chat_widget "baza-skolkovo/src/chat_widget"
	"baza-skolkovo/src/classifier"
	"baza-skolkovo/src/common/config"
	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/qdrant"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/contests"
	"baza-skolkovo/src/eligibility"
	"baza-skolkovo/src/events"
	"baza-skolkovo/src/faq"
	"baza-skolkovo/src/fetcher"
	mcpserver "baza-skolkovo/src/mcp_server"
	"baza-skolkovo/src/migrate"
	"baza-skolkovo/src/news"
	"baza-skolkovo/src/notify"
	"baza-skolkovo/src/pipeline"
	"baza-skolkovo/src/portal"
	"baza-skolkovo/src/preferences"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/regulations"
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
	case "preferences":
		mustRun(cmdPreferences(cfg))
	case "regulations":
		mustRun(cmdRegulations(cfg))
	case "eligibility":
		mustRun(cmdEligibility(cfg, args))
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

func newRAG(cfg config.Config, st store.Store, cls *classifier.DocumentClassifier) *rag.Service {
	qdr := qdrant.New(cfg.QdrantURL, cfg.QdrantColl)
	emb := embed.NewTEIClient(cfg.TEIURL)
	svc := rag.New(st, qdr, emb, cfg.EmbeddingDim)
	if cls != nil {
		svc.WithClassifier(cls)
	}
	return svc
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

	svc := newRAG(cfg, st, nil)
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

	svc := newRAG(cfg, st, nil)
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

	svc := newRAG(cfg, st, nil)
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
		kind    model.ChecklistType
		title   string
		desc    string
		version string
		steps   []model.ChecklistStepDef
	}{
		{
			kind:    model.ChecklistEntry,
			title:   "Вход в резидентство",
			desc:    "Договор, статус, документы для вступления в Сколково",
			version: "1.0",
			steps: []model.ChecklistStepDef{
				{ID: "entry-1", Title: "Подготовка документов", Description: "Подготовка выписки из ЕГРЮЛ, справки об отсутствии задолженности, описания проекта", Order: 1, DeadlineDays: 14},
				{ID: "entry-2", Title: "Подача заявки через личный кабинет", Description: "", Order: 2, DeadlineDays: 1},
				{ID: "entry-3", Title: "Прохождение экспертизы", Description: "Предоставление дополнительных документов по запросу", Order: 3, DeadlineDays: 30},
				{ID: "entry-4", Title: "Заключение договора с Фондом", Description: "", Order: 4, DeadlineDays: 7},
				{ID: "entry-5", Title: "Получение статуса резидента", Description: "", Order: 5, DeadlineDays: 1},
			},
		},
		{
			kind:    model.ChecklistReporting,
			title:   "Квартальная отчётность",
			desc:    "Финансовые и технические отчёты, сроки подачи",
			version: "1.0",
			steps: []model.ChecklistStepDef{
				{ID: "reporting-1", Title: "Сбор данных за квартал", Description: "Финансовые показатели, данные о проекте", Order: 1, DeadlineDays: 10},
				{ID: "reporting-2", Title: "Заполнение квартального отчёта", Description: "", Order: 2, DeadlineDays: 3},
				{ID: "reporting-3", Title: "Проверка отчёта", Description: "", Order: 3, DeadlineDays: 2},
				{ID: "reporting-4", Title: "Отправка отчёта в Фонд", Description: "", Order: 4, DeadlineDays: 1},
				{ID: "reporting-5", Title: "Подтверждение получения", Description: "", Order: 5, DeadlineDays: 5},
			},
		},
		{
			kind:    model.ChecklistExtension,
			title:   "Продление резидентства",
			desc:    "Заявление, обоснование, приложения для продления статуса",
			version: "1.0",
			steps: []model.ChecklistStepDef{
				{ID: "extension-1", Title: "Проверка условий продления", Description: "Отсутствие задолженности, выполнение программы", Order: 1, DeadlineDays: 14},
				{ID: "extension-2", Title: "Подготовка заявления на продление", Description: "", Order: 2, DeadlineDays: 3},
				{ID: "extension-3", Title: "Подача заявления", Description: "", Order: 3, DeadlineDays: 1},
				{ID: "extension-4", Title: "Прохождение проверки Фондом", Description: "", Order: 4, DeadlineDays: 30},
				{ID: "extension-5", Title: "Заключение дополнительного соглашения", Description: "", Order: 5, DeadlineDays: 7},
			},
		},
		{
			kind:    model.ChecklistExit,
			title:   "Выход из проекта",
			desc:    "Уведомление, закрытие обязательств при выходе из Сколково",
			version: "1.0",
			steps: []model.ChecklistStepDef{
				{ID: "exit-1", Title: "Уведомление Фонда о выходе", Description: "Письменное уведомление за 30 дней", Order: 1, DeadlineDays: 1},
				{ID: "exit-2", Title: "Подготовка заключительного отчёта", Description: "", Order: 2, DeadlineDays: 14},
				{ID: "exit-3", Title: "Возврат неиспользованных средств", Description: "Если применимо", Order: 3, DeadlineDays: 30},
				{ID: "exit-4", Title: "Подтверждение выхода", Description: "", Order: 4, DeadlineDays: 7},
			},
		},
	}

	log.Printf("[seed] проверка %d стандартных чек-листов", len(checklists))

	for _, cl := range checklists {
		// Проверяем существует ли чек-лист по procedure_type.
		existing, err := checklistStore.GetChecklist(ctx, string(cl.kind))
		if err == nil && existing != nil {
			fmt.Printf("  [%s] %s — уже существует, пропуск\n", cl.kind, cl.title)
			continue
		}

		// Сериализуем шаги в JSON.
		stepsJSON, err := json.Marshal(cl.steps)
		if err != nil {
			return fmt.Errorf("сериализация шагов [%s]: %w", cl.kind, err)
		}

		checklist := &model.Checklist{
			ID:            string(cl.kind),
			Title:         cl.title,
			ProcedureType: cl.kind,
			Steps:         stepsJSON,
			Version:       cl.version,
			CreatedAt:     time.Now(),
		}

		if err := checklistStore.CreateChecklist(ctx, checklist); err != nil {
			return fmt.Errorf("создание чек-листа [%s]: %w", cl.kind, err)
		}
		fmt.Printf("  [%s] %s — создан (%d шагов)\n", cl.kind, cl.title, len(cl.steps))
	}

	fmt.Println("Seed: чек-листы созданы")
	return nil
}

func cmdSync(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	return newPipeline(cfg, st, newRAG(cfg, st, nil)).RunOnce(ctx)
}

func cmdMCP(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	svc := newRAG(cfg, st, nil)
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
		cfg.ChromePath, cfg.ProxyURL, cfg.SourceURL, cfg.FetchWait, st, newRAG(cfg, st, nil))

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

// cmdServe запускает планировщик, MCP-сервер, админку, портал, чат-виджет
// и регистрирует ИИ-агентов.
func cmdServe(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	// --- Классификатор ---
	var cls *classifier.DocumentClassifier
	if cfg.ClassifierEnabled {
		emb := embed.NewTEIClient(cfg.TEIURL)
		cls = classifier.NewDocumentClassifier(emb, classifier.ClassifierConfig{Threshold: 0.5})
		if err := cls.PrecomputeCategoryEmbeddings(ctx, classifier.DefaultCategories); err != nil {
			log.Printf("[serve:classifier] предупреждение: не удалось предвычислить эмбеддинги категорий: %v", err)
		} else {
			log.Printf("[serve:classifier] эмбеддинги %d категорий предвычислены", len(classifier.DefaultCategories))
		}
	}

	svc := newRAG(cfg, st, cls)

	// --- MCP-сервер ---
	mcpSrv := mcpserver.New(cfg.MCPAddr, cfg.MCPAPIKey, cfg.MCPRateLimitRPS, svc, st)
	registerExtraMCPTools(mcpSrv.MCPServer(), st)
	registerAgentMCPTools(mcpSrv.MCPServer(), st, svc, cfg)

	// --- Prometheus /metrics на отдельном порту ---
	promAddr := ":9090"
	promMux := http.NewServeMux()
	promMux.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("[prometheus] метрики доступны на %s/metrics", promAddr)
		if err := http.ListenAndServe(promAddr, promMux); err != nil {
			log.Printf("[prometheus] остановлен: %v", err)
		}
	}()

	// --- Админка резидентства ---
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

	// --- Портал клиента ---
	if cfg.PortalEnabled {
		go func() {
			portalCfg := portal.PortalConfig{
				Addr:      cfg.PortalAddr,
				BaseURL:   "http://localhost" + cfg.PortalAddr,
				MCPURL:    "http://localhost" + cfg.MCPAddr,
				MCPAPIKey: cfg.MCPAPIKey,
			}
			stores := buildResidencyStores(st)
			portalStores := portal.PortalStores{
				ClientStore:    stores.ClientStore,
				ChecklistStore: stores.ChecklistStore,
				DeadlineStore:  stores.DeadlineStore,
				TemplateStore:  stores.TemplateStore,
				DocumentStore:  st,
			}
			ps := portal.NewPortalServer(portalCfg, portalStores)
			log.Printf("[portal] запуск на %s", cfg.PortalAddr)
			if err := ps.Start(ctx); err != nil {
				log.Printf("[portal] остановлен: %v", err)
			}
		}()
	}

	// --- Чат-виджет ---
	if cfg.ChatWidgetEnabled {
		go func() {
			widgetCfg := chat_widget.WidgetConfig{
				MCPURL:         "http://localhost" + cfg.MCPAddr,
				MCPAPIKey:      cfg.MCPAPIKey,
				ListenAddr:     cfg.ChatWidgetAddr,
				WelcomeMessage: "Здравствуйте! Чем могу помочь?",
			}
			ws := chat_widget.NewWidgetServer(widgetCfg)
			log.Printf("[widget] запуск на %s", cfg.ChatWidgetAddr)
			if err := ws.Start(ctx); err != nil {
				log.Printf("[widget] остановлен: %v", err)
			}
		}()
	}

	// --- Планировщик ---
	go newPipeline(cfg, st, svc).Schedule(ctx, cfg.ScrapeInterval)

	// --- Telegram-нотификатор консультанту ---
	tgNotifier := notify.NewTelegramNotifier(
		os.Getenv("TELEGRAM_BOT_TOKEN"),
		cfg.ConsultantTelegramChatID,
	)
	if tgNotifier.Enabled() {
		log.Printf("[serve:notify] Telegram-уведомления консультанту включены")
	}

	// --- Telegram-бот ---
	if os.Getenv("TELEGRAM_BOT_TOKEN") != "" {
		go runTelegramBot(ctx, cfg, st)
	}

	// --- Консультантский дашборд ---
	if cfg.ConsultantEnabled {
		go func() {
			consultantStores := admin.ConsultantDashboardStores{}
			if ps, ok := st.(*store.PostgresStore); ok {
				pcs := store.NewPostgresClientStore(ps.Pool())
				consultantStores.ClientStore = pcs
				consultantStores.DeadlineStore = pcs
				consultantStores.ChecklistStore = pcs
			}
			if consultantStores.ClientStore != nil {
				mux := admin.RegisterConsultantRoutes(nil, consultantStores)
				log.Printf("[consultant] дашборд запускается на %s", cfg.ConsultantAddr)
				if err := http.ListenAndServe(cfg.ConsultantAddr, mux); err != nil {
					log.Printf("[consultant] остановлен: %v", err)
				}
			} else {
				log.Printf("[consultant] пропущен: требуется backend=postgres")
			}
		}()
	}

	// --- Планировщик для новых модулей ---
	go scheduleNewModules(ctx, cfg, st)

	// --- Ежедневная сводка консультанту ---
	if tgNotifier.Enabled() {
		go runDailySummary(ctx, cfg, st, tgNotifier)
	}

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

// runDailySummary отправляет ежедневную сводку консультанту в заданный час.
func runDailySummary(ctx context.Context, cfg config.Config, st store.Store, tg *notify.TelegramNotifier) {
	for {
		now := time.Now()
		nextRun := time.Date(now.Year(), now.Month(), now.Day(), cfg.DailySummaryHour, 0, 0, 0, now.Location())
		if !nextRun.After(now) {
			nextRun = nextRun.Add(24 * time.Hour)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextRun)):
		}

		if err := sendDailySummary(ctx, cfg, st, tg); err != nil {
			log.Printf("[daily-summary] ошибка: %v", err)
		}
	}
}

// sendDailySummary собирает статистику и отправляет сводку.
func sendDailySummary(ctx context.Context, cfg config.Config, st store.Store, tg *notify.TelegramNotifier) error {
	summary := notify.DailySummary{}

	if ps, ok := st.(*store.PostgresStore); ok {
		pcs := store.NewPostgresClientStore(ps.Pool())

		clients, err := pcs.ListClients(ctx, "", model.ResidencyStage(""))
		if err == nil {
			summary.ActiveClients = len(clients)
			now := time.Now()

			for _, c := range clients {
				deadlines, _ := pcs.ListDeadlines(ctx, c.ID, 30)
				for _, d := range deadlines {
					daysLeft := int(d.DueDate.Sub(now).Hours() / 24)
					if daysLeft < 0 {
						summary.OverdueCount++
						summary.UrgentClients = append(summary.UrgentClients, notify.UrgentClientInfo{
							Name:   c.Name,
							ID:     c.ID,
							Reason: fmt.Sprintf("просрочен: %s", d.Title),
						})
					} else if daysLeft <= 3 {
						summary.UrgentCount++
						summary.UrgentClients = append(summary.UrgentClients, notify.UrgentClientInfo{
							Name:   c.Name,
							ID:     c.ID,
							Reason: fmt.Sprintf("дедлайн через %d дн.: %s", daysLeft, d.Title),
						})
					}
				}
				// Проверяем "застрявших" клиентов (нет изменений > 14 дней).
				if int(now.Sub(c.UpdatedAt).Hours()/24) >= 14 {
					summary.StuckCount++
				}
			}
		}
	}

	// Число изменений документов за сутки.
	docs, err := st.List(ctx, store.Filter{Status: model.StatusActive})
	if err == nil {
		yesterday := time.Now().AddDate(0, 0, -1)
		for _, d := range docs {
			if d.FetchedAt.After(yesterday) {
				summary.ChangedDocs++
			}
		}
	}

	return tg.SendDailySummary(ctx, summary)
}

// scheduleNewModules запускает периодический парсинг мероприятий, конкурсов,
// FAQ, Telegram-каналов, льгот и НПА.
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

			// Льготы резидентов
			if cfg.PreferencesEnabled {
				log.Printf("[serve:preferences] синхронизация льгот")
				prefCfg := preferences.PreferencesConfig{
					SourceURLs: []string{cfg.PreferencesURL},
					Category:   "Льготы",
				}
				mon := preferences.NewMonitor(prefCfg, st, nil)
				if res, err := mon.Run(ctx); err != nil {
					log.Printf("[serve:preferences] ошибка: %v", err)
				} else {
					log.Printf("[serve:preferences] готово: новых %d, обновлено %d", res.New, res.Updated)
				}
			}

			// НПА
			if cfg.RegulationsEnabled {
				log.Printf("[serve:regulations] синхронизация НПА")
				var extraURLs []string
				if cfg.RegulationsExtraURLs != "" {
					for _, u := range strings.Split(cfg.RegulationsExtraURLs, ",") {
						if u = strings.TrimSpace(u); u != "" {
							extraURLs = append(extraURLs, u)
						}
					}
				}
				regCfg := regulations.RegulationsConfig{
					SearchURL:   cfg.RegulationsSearchURL,
					ExtraURLs:   extraURLs,
					SearchQuery: "Сколково",
					Category:    "НПА",
				}
				mon := regulations.NewMonitor(regCfg, st, nil)
				if res, err := mon.Run(ctx); err != nil {
					log.Printf("[serve:regulations] ошибка: %v", err)
				} else {
					log.Printf("[serve:regulations] готово: новых %d, обновлено %d", res.New, res.Updated)
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

// registerAgentMCPTools создаёт ИИ-агентов и регистрирует их MCP-инструменты.
func registerAgentMCPTools(mcpSrv *server.MCPServer, st store.Store, ragSvc *rag.Service, cfg config.Config) {
	ps, ok := st.(*store.PostgresStore)
	if !ok {
		log.Printf("[mcp:agents] требуется backend=postgres для регистрации агентов")
		return
	}
	pool := ps.Pool()
	pcs := store.NewPostgresClientStore(pool)

	// Создаём агентов.
	consultant := agents.NewConsultantAgent(ragSvc, "http://"+cfg.MCPAddr, cfg.MCPAPIKey)
	validator := agents.NewValidatorAgent(ragSvc, pcs)
	monitorStores := agents.MonitorStores{
		DocStore:      st,
		EventStore:    store.NewPostgresSourceStore(pool),
		ContestStore:  store.NewPostgresSourceStore(pool),
		ClientStore:   pcs,
		DeadlineStore: pcs,
	}
	monitor := agents.NewMonitorAgent(monitorStores)
	coordStores := agents.CoordinatorStores{
		ClientStore:    pcs,
		ChecklistStore: pcs,
		DeadlineStore:  pcs,
		TemplateStore:  pcs,
	}
	coordinator := agents.NewCoordinatorAgent(coordStores)

	// ask_consultant — вопрос к консультанту.
	mcpSrv.AddTool(
		mcp.NewTool("ask_consultant",
			mcp.WithDescription("Задать вопрос ИИ-консультанту по базе документов Сколково. Возвращает ответ с источниками."),
			mcp.WithString("question", mcp.Required(), mcp.Description("Текст вопроса")),
			mcp.WithString("client_id", mcp.Description("Идентификатор клиента (необязательно)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			question := req.GetString("question", "")
			clientID := req.GetString("client_id", "")
			resp, err := consultant.Ask(ctx, question, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка консультанта: " + err.Error()), nil
			}
			return mcp.NewToolResultText(resp.Answer), nil
		},
	)

	// validate_document — валидация документа по чек-листу.
	mcpSrv.AddTool(
		mcp.NewTool("validate_document",
			mcp.WithDescription("Проверить документ по чек-листу процедуры. Возвращает отчёт с проблемами и оценкой."),
			mcp.WithString("document_text", mcp.Required(), mcp.Description("Полный текст документа")),
			mcp.WithString("procedure_type", mcp.Required(), mcp.Description("Тип процедуры: entry, reporting, extension, exit")),
			mcp.WithString("client_id", mcp.Description("Идентификатор клиента (необязательно)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			docText := req.GetString("document_text", "")
			procType := req.GetString("procedure_type", "")
			clientID := req.GetString("client_id", "")
			report, err := validator.ValidateDocument(ctx, docText, procType, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка валидации: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(report)), nil
		},
	)

	// get_next_steps — рекомендации следующих шагов для клиента.
	mcpSrv.AddTool(
		mcp.NewTool("get_next_steps",
			mcp.WithDescription("Получить рекомендации следующих шагов для клиента по чек-листу."),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clientID := req.GetString("client_id", "")
			steps, err := coordinator.GetNextSteps(ctx, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка координатора: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(steps)), nil
		},
	)

	// subscribe_to_changes — подписка на изменения документов.
	mcpSrv.AddTool(
		mcp.NewTool("subscribe_to_changes",
			mcp.WithDescription("Подписать клиента на уведомления об изменениях в указанных категориях документов."),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithString("categories", mcp.Required(), mcp.Description("Категории через запятую: regulations, events, contests, reporting")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clientID := req.GetString("client_id", "")
			catsStr := req.GetString("categories", "")
			cats := strings.Split(catsStr, ",")
			for i := range cats {
				cats[i] = strings.TrimSpace(cats[i])
			}
			if err := monitor.Subscribe(ctx, clientID, cats); err != nil {
				return mcp.NewToolResultError("ошибка подписки: " + err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Клиент %s подписан на: %s", clientID, catsStr)), nil
		},
	)

	// draft_document — подготовка черновика документа для клиента.
	draftingStores := agents.DraftingStores{
		ClientStore:    pcs,
		TemplateStore:  pcs,
		ChecklistStore: pcs,
	}
	drafter := agents.NewDocumentDraftingAgent(draftingStores, ragSvc)

	mcpSrv.AddTool(
		mcp.NewTool("draft_document",
			mcp.WithDescription("Подготовить черновик документа для клиента (заявка, описание проекта, отчёт, продление, выход). Возвращает заполненный текст в Markdown."),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithString("document_type", mcp.Required(), mcp.Description("Тип документа: application, project_description, report, extension_request, exit_notice, ird_description")),
			mcp.WithString("extra_context", mcp.Description("Дополнительный контекст от консультанта")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			draftReq := agents.DraftRequest{
				ClientID:     req.GetString("client_id", ""),
				DocumentType: req.GetString("document_type", ""),
				ExtraContext: req.GetString("extra_context", ""),
			}
			result, err := drafter.Draft(ctx, draftReq)
			if err != nil {
				return mcp.NewToolResultError("ошибка подготовки документа: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(result)), nil
		},
	)

	// check_eligibility — проверка компании по ИНН.
	if cfg.EligibilityEnabled {
		eligChecker := eligibility.NewChecker(eligibility.Config{
			DadataAPIKey: cfg.DadataAPIKey,
		})
		mcpSrv.AddTool(
			mcp.NewTool("check_eligibility",
				mcp.WithDescription("Проверить, может ли компания стать резидентом Сколково. Принимает ИНН, возвращает отчёт с оценкой, проблемами и рекомендациями."),
				mcp.WithString("inn", mcp.Required(), mcp.Description("ИНН компании (10 или 12 цифр)")),
			),
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				inn := req.GetString("inn", "")
				report, err := eligChecker.CheckByINN(ctx, inn)
				if err != nil {
					return mcp.NewToolResultError("ошибка проверки: " + err.Error()), nil
				}
				return mcp.NewToolResultText(toJSON(report)), nil
			},
		)
	}

	log.Printf("[mcp:agents] зарегистрированы инструменты: ask_consultant, validate_document, get_next_steps, subscribe_to_changes, draft_document, check_eligibility")
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

	// Создаём консультанта для бота
	consultant := agents.NewConsultantAgent(nil, "http://"+cfg.MCPAddr, cfg.MCPAPIKey)

	bot, err := tgbot.NewBot(botCfg, botStores, consultant)
	if err != nil {
		log.Printf("[tgbot] ошибка создания бота: %v", err)
		return
	}

	log.Printf("[tgbot] запуск бота")
	if err := bot.Run(ctx); err != nil {
		log.Printf("[tgbot] остановлен: %v", err)
	}
}

// cmdPreferences парсит льготы резидентов Сколково и сохраняет в хранилище.
func cmdPreferences(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := newRAG(cfg, st, nil)
	if err := svc.Init(ctx); err != nil {
		log.Printf("[preferences] Qdrant недоступен: %v (без индексации)", err)
		svc = nil
	}

	prefCfg := preferences.PreferencesConfig{
		SourceURLs: []string{cfg.PreferencesURL},
		Category:   "Льготы",
	}
	mon := preferences.NewMonitor(prefCfg, st, svc)
	res, err := mon.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Льготы: получено %d, новых %d, обновлено %d, ошибок %d\n",
		res.Fetched, res.New, res.Updated, len(res.Errors))
	return nil
}

// cmdRegulations парсит НПА по Сколково и сохраняет в хранилище.
func cmdRegulations(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	var extraURLs []string
	if cfg.RegulationsExtraURLs != "" {
		for _, u := range strings.Split(cfg.RegulationsExtraURLs, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				extraURLs = append(extraURLs, u)
			}
		}
	}

	regCfg := regulations.RegulationsConfig{
		SearchURL:  cfg.RegulationsSearchURL,
		ExtraURLs:  extraURLs,
		SearchQuery: "Сколково",
		Category:   "НПА",
	}
	mon := regulations.NewMonitor(regCfg, st, nil)
	res, err := mon.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("НПА: получено %d, новых %d, обновлено %d, ошибок %d\n",
		res.Fetched, res.New, res.Updated, len(res.Errors))
	return nil
}

// cmdEligibility проверяет компанию по ИНН на соответствие требованиям Сколково.
func cmdEligibility(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("использование: skolkovo eligibility <ИНН>")
	}
	inn := strings.TrimSpace(args[0])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checker := eligibility.NewChecker(eligibility.Config{
		DadataAPIKey: cfg.DadataAPIKey,
	})

	report, err := checker.CheckByINN(ctx, inn)
	if err != nil {
		return fmt.Errorf("проверка ИНН %s: %w", inn, err)
	}

	fmt.Printf("\n=== Проверка eligibility для ИНН %s ===\n", inn)
	if report.Company != nil {
		fmt.Printf("Компания: %s\n", report.Company.FullName)
		fmt.Printf("Статус: %s\n", report.Company.Status)
		fmt.Printf("МСП: %v (%s)\n", report.Company.IsMSP, report.Company.MSPCategory)
	}
	fmt.Printf("Оценка: %d/100\n", report.Score)
	fmt.Printf("Может стать резидентом: %v\n", report.Eligible)
	if len(report.Issues) > 0 {
		fmt.Println("\nПроблемы:")
		for _, iss := range report.Issues {
			fmt.Printf("  ❌ %s\n", iss)
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Println("\nПредупреждения:")
		for _, w := range report.Warnings {
			fmt.Printf("  ⚠️  %s\n", w)
		}
	}
	if len(report.Recommendations) > 0 {
		fmt.Println("\nРекомендации:")
		for _, r := range report.Recommendations {
			fmt.Printf("  → %s\n", r)
		}
	}
	return nil
}

func mustRun(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Использование: skolkovo <подкоманда>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Сбор данных:")
	fmt.Fprintln(os.Stderr, "  scrape        — каталог документов из RSS+HTML dochub.sk.ru")
	fmt.Fprintln(os.Stderr, "  catalog       — полное перечисление каталога (headless browser)")
	fmt.Fprintln(os.Stderr, "  fetch         — скачать тела файлов (обход WAF, chromedp)")
	fmt.Fprintln(os.Stderr, "  news          — синхронизировать новости из RSS")
	fmt.Fprintln(os.Stderr, "  events        — парсинг мероприятий")
	fmt.Fprintln(os.Stderr, "  contests      — парсинг конкурсов и грантов")
	fmt.Fprintln(os.Stderr, "  faq           — парсинг FAQ")
	fmt.Fprintln(os.Stderr, "  telegram      — парсинг Telegram-каналов")
	fmt.Fprintln(os.Stderr, "  preferences   — льготы резидентов Сколково (налоговые, таможенные)")
	fmt.Fprintln(os.Stderr, "  regulations   — НПА по Сколково (244-ФЗ, постановления Правительства)")
	fmt.Fprintln(os.Stderr, "  sync          — полный цикл: документы + новости + льготы + НПА + индексация")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Аналитика:")
	fmt.Fprintln(os.Stderr, "  index         — проиндексировать активные документы в Qdrant (RAG)")
	fmt.Fprintln(os.Stderr, "  eligibility   — проверить компанию по ИНН: соответствует ли требованиям")
	fmt.Fprintln(os.Stderr, "  embed         — проверить доступность TEI (вычислить тестовый эмбеддинг)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "База данных:")
	fmt.Fprintln(os.Stderr, "  migrate       — применить миграции PostgreSQL")
	fmt.Fprintln(os.Stderr, "  seed          — загрузить стандартные чек-листы")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Серверы:")
	fmt.Fprintln(os.Stderr, "  mcp           — MCP-сервер (открытый API)")
	fmt.Fprintln(os.Stderr, "  admin         — веб-панель администратора")
	fmt.Fprintln(os.Stderr, "  serve         — всё сразу: планировщик + MCP + админка + портал + бот + дашборд")
}
