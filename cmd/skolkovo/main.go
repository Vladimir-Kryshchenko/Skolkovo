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
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/admin"
	"baza-skolkovo/src/common/config"
	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/qdrant"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/fetcher"
	mcpserver "baza-skolkovo/src/mcp_server"
	"baza-skolkovo/src/migrate"
	"baza-skolkovo/src/news"
	"baza-skolkovo/src/notify"
	"baza-skolkovo/src/pipeline"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/scraper"
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

	log.Printf("[events] парсинг мероприятий из %s", cfg.EventsSourceURL)
	// TODO: реализовать парсер мероприятий (RSS + headless-обход)
	fmt.Println("Мероприятия: модуль в разработке")
	return nil
}

func cmdContests(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	log.Printf("[contests] парсинг конкурсов из %s и грантов из %s", cfg.ContestsURL, cfg.GrantsURL)
	// TODO: реализовать парсер конкурсов и грантов
	fmt.Println("Конкурсы: модуль в разработке")
	return nil
}

func cmdFAQ(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	log.Printf("[faq] парсинг FAQ из %s", cfg.FAQURL)
	// TODO: реализовать парсер FAQ
	fmt.Println("FAQ: модуль в разработке")
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

	log.Printf("[telegram] парсинг %d каналов через %s", len(active), cfg.TelegramRssHubURL)
	// TODO: реализовать парсер Telegram-каналов через RSSHub
	fmt.Println("Telegram: модуль в разработке")
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

	// Стандартные чек-листы для резидентов Сколково.
	checklists := map[string]string{
		"entry":     "Чек-лист входа резидента: договор, статус, документы",
		"reporting": "Чек-лист отчётности: финансовые и технические отчёты, сроки",
		"extension": "Чек-лист продления: заявление, обоснование, приложения",
		"exit":      "Чек-лист выхода: уведомление, закрытие обязательств",
	}

	log.Printf("[seed] загрузка %d стандартных чек-листов", len(checklists))
	// TODO: реализовать запись чек-листов в БД / RAG
	for kind, desc := range checklists {
		fmt.Printf("  [%s] %s\n", kind, desc)
	}
	fmt.Println("Seed: модуль в разработке")
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
	return mcpserver.New(cfg.MCPAddr, cfg.MCPAPIKey, cfg.MCPRateLimitRPS, svc, st).ListenAndServe()
}

func cmdAdmin(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	return admin.New(cfg.AdminAddr, cfg.AdminUser, cfg.AdminPassword, cfg.DocsDir,
		cfg.ChromePath, cfg.ProxyURL, cfg.SourceURL, cfg.FetchWait, st, newRAG(cfg, st)).ListenAndServe()
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

	// Регистрируем новые MCP-инструменты для событий, конкурсов, FAQ и Telegram.
	// TODO: передать обработчики новых инструментов в mcpserver.New()
	_ = cfg.EventsRSSURL // placeholder для будущих MCP tools
	_ = cfg.ContestsURL
	_ = cfg.FAQURL
	_ = cfg.TelegramChannels

	mcpSrv := mcpserver.New(cfg.MCPAddr, cfg.MCPAPIKey, cfg.MCPRateLimitRPS, svc, st)

	// Админка с маршрутами для новых модулей.
	// TODO: передать конфигурацию новых модулей в admin.New() для регистрации
	// маршрутов /events, /contests, /faq, /telegram
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

	// Планировщик для новых модулей (заглушки).
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
// FAQ и Telegram-каналов. Заглушка — реальная реализация появится с модулями.
func scheduleNewModules(ctx context.Context, cfg config.Config, st store.Store) {
	_ = st // TODO: использовать при реализации парсеров
	ticker := time.NewTicker(cfg.ScrapeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cfg.EventsSourceURL != "" {
				log.Printf("[serve:events] запуск парсинга мероприятий")
				// TODO: вызвать парсер мероприятий
			}
			if cfg.ContestsURL != "" {
				log.Printf("[serve:contests] запуск парсинга конкурсов")
				// TODO: вызвать парсер конкурсов
			}
			if cfg.FAQURL != "" {
				log.Printf("[serve:faq] запуск парсинга FAQ")
				// TODO: вызвать парсер FAQ
			}
			if cfg.TelegramChannels != "" {
				log.Printf("[serve:telegram] запуск парсинга Telegram-каналов")
				// TODO: вызвать парсер Telegram
			}
		}
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
