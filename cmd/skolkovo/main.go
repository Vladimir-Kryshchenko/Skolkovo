// Command skolkovo — единая точка входа сервиса «База Сколково».
//
// Подкоманды:
//
//	scrape          — каталог из RSS (~20 свежих) + обход HTML               (E1)
//	catalog         — полное перечисление каталога по категориям (headless)  (E1)
//	index [-force]  — проиндексировать действующие документы в RAG          (E2)
//	navindex [act]  — навигационная карта сайта: render|export|index|search|all (для get_navigation)
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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"baza-skolkovo/src/admin"
	"baza-skolkovo/src/agents"
	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/audit"
	"baza-skolkovo/src/changes"
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
	"baza-skolkovo/src/generator"
	"baza-skolkovo/src/health"
	"baza-skolkovo/src/mailer"
	mcpserver "baza-skolkovo/src/mcp_server"
	"baza-skolkovo/src/migrate"
	"baza-skolkovo/src/navindex"
	"baza-skolkovo/src/news"
	"baza-skolkovo/src/notify"
	"baza-skolkovo/src/pipeline"
	"baza-skolkovo/src/portal"
	"baza-skolkovo/src/preferences"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/regulations"
	"baza-skolkovo/src/relevance"
	"baza-skolkovo/src/residents"
	"baza-skolkovo/src/scraper"
	"baza-skolkovo/src/sitepages"
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
	case "navindex":
		mustRun(cmdNavIndex(cfg, args))
	case "sitepages":
		mustRun(cmdSitePages(cfg, args))
	case "catalog":
		mustRun(cmdCatalog(cfg))
	case "crawl":
		mustRun(cmdCrawl(cfg))
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
	case "residents":
		mustRun(cmdResidents(cfg))
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
	case "generate":
		mustRun(cmdGenerate(cfg, args))
	case "audit":
		mustRun(cmdAudit(cfg))
	case "serve":
		mustRun(cmdServe(cfg))
	case "embed":
		mustRun(embedTest(cfg))
	case "doctor":
		mustRun(cmdDoctor(cfg))
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

// newNavQdrant создаёт Qdrant-клиент для отдельной коллекции навигации.
func newNavQdrant(cfg config.Config) *qdrant.Client {
	return qdrant.New(cfg.QdrantURL, cfg.NavColl)
}

// newNavIndexer создаёт индексатор навигации по сайту (коллекция NAV_COLLECTION).
func newNavIndexer(cfg config.Config) *navindex.Indexer {
	return navindex.NewIndexer(newNavQdrant(cfg), embed.NewTEIClient(cfg.TEIURL), cfg.EmbeddingDim)
}

// newSitePagesQdrant создаёт Qdrant-клиент для коллекции страниц публичного сайта.
func newSitePagesQdrant(cfg config.Config) *qdrant.Client {
	return qdrant.New(cfg.QdrantURL, cfg.SitePagesColl)
}

// newSitePagesIndexer создаёт индексатор страниц сайта (коллекция SITEPAGES_COLLECTION).
func newSitePagesIndexer(cfg config.Config) *sitepages.Indexer {
	return sitepages.NewIndexer(newSitePagesQdrant(cfg), embed.NewTEIClient(cfg.TEIURL), cfg.EmbeddingDim)
}

// sitePagesSeeds разбирает SITEPAGES_SEEDS (URL через запятую) в список.
func sitePagesSeeds(cfg config.Config) []string {
	var out []string
	for _, s := range strings.Split(cfg.SitePagesSeeds, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// cmdSitePages обходит страницы публичного сайта Сколково и индексирует их в
// отдельную Qdrant-коллекцию (отдельно от файлов-документов):
//
//	sitepages crawl        — обойти сайт и обновить таблицу site_pages;
//	sitepages enrich       — аннотировать ИИ страницы без аннотации (теги/описание/цели/тезисы/выводы);
//	sitepages enrich --all — переаннотировать все действующие страницы заново;
//	sitepages index        — переиндексировать все страницы в Qdrant;
//	sitepages search <q>   — смоук-проверка поиска (search_site_pages);
//	sitepages              — всё сразу (crawl + enrich + index).
func cmdSitePages(cfg config.Config, args []string) error {
	action := "all"
	if len(args) > 0 {
		action = args[0]
	}
	ctx := context.Background()

	// sitepages search <запрос> — смоук-проверка поиска по страницам.
	if action == "search" {
		query := strings.TrimSpace(strings.Join(args[1:], " "))
		if query == "" {
			return fmt.Errorf("использование: sitepages search <запрос>")
		}
		searcher := sitepages.NewSearcher(newSitePagesQdrant(cfg), embed.NewTEIClient(cfg.TEIURL))
		hits, err := searcher.Search(ctx, query, 5)
		if err != nil {
			return fmt.Errorf("поиск по страницам: %w", err)
		}
		for i, h := range hits {
			fmt.Printf("%d. [%.3f] %s — %s\n   %s\n", i+1, h.Score, h.Title, h.URL, h.Section)
		}
		return nil
	}

	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	ps, ok := st.(*store.PostgresStore)
	if !ok {
		return fmt.Errorf("sitepages требует backend=postgres")
	}
	pageStore, err := sitepages.NewPostgresStore(ctx, ps.Pool())
	if err != nil {
		return fmt.Errorf("хранилище страниц: %w", err)
	}

	doCrawl := action == "all" || action == "crawl"
	doEnrich := action == "all" || action == "enrich"
	doIndex := action == "all" || action == "index"
	forceEnrich := action == "enrich" && len(args) > 1 &&
		(args[1] == "--all" || args[1] == "all" || args[1] == "--force")

	if doCrawl {
		var rec changes.Recorder
		if cs, err := changes.NewPostgresStore(ctx, ps.Pool()); err == nil {
			rec = cs
		}
		cr := sitepages.New(sitePagesSeeds(cfg), pageStore)
		cr.MaxPages = cfg.SitePagesMaxPages
		cr.Delay = cfg.ScrapeDelay
		cr.Changes = rec
		cr.UseProxy(cfg.ProxyURL)
		rep, err := cr.Run(ctx)
		if err != nil {
			return fmt.Errorf("обход страниц: %w", err)
		}
		log.Printf("[sitepages] обход: посещено %d, новых %d, изменено %d, без изменений %d, ошибок %d",
			rep.Visited, rep.New, rep.Changed, rep.Unchanged, len(rep.Errors))
	}

	if doEnrich {
		aiStore := aimodels.NewStore(ps.Pool())
		_ = aiStore.EnsurePageAnnotatorAgent(ctx)
		enr := sitepages.NewEnricher(aiStore, pageStore, cfg.SitePagesEnrichDelay)
		var pend []*sitepages.Page
		if forceEnrich {
			pend, err = pageStore.ListAllForEnrichment(ctx, 0)
		} else {
			pend, err = pageStore.ListNeedingEnrichment(ctx, 0)
		}
		if err != nil {
			return fmt.Errorf("страницы для аннотирования: %w", err)
		}
		log.Printf("[sitepages] аннотирование: к обработке %d страниц", len(pend))
		d, s, f := enr.EnrichBatch(ctx, pend)
		log.Printf("[sitepages] аннотирование завершено: обогащено %d, пропущено %d, ошибок %d", d, s, f)
	}

	if doIndex {
		pages, err := pageStore.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("чтение страниц: %w", err)
		}
		n, err := newSitePagesIndexer(cfg).Reindex(ctx, pages)
		if err != nil {
			return fmt.Errorf("индексация страниц: %w", err)
		}
		log.Printf("[sitepages] проиндексировано %d страниц в коллекцию %q", n, cfg.SitePagesColl)
	}
	return nil
}

// cmdNavIndex собирает навигационную карту сайта из src/navindex (источник истины)
// и выгружает её во все формы:
//
//	navindex render — Markdown-карта (RAG_Структура_сайта.md) для каталога;
//	navindex export — navigation.json (источник истины в JSON);
//	navindex index  — векторный индекс в Qdrant-коллекцию навигации (для get_navigation);
//	navindex search <запрос> — смоук-проверка навигационного поиска;
//	navindex        — всё сразу (render + export + index).
func cmdNavIndex(cfg config.Config, args []string) error {
	action := "all"
	if len(args) > 0 {
		action = args[0]
	}

	// navindex search <запрос> — смоук-проверка навигационного поиска (get_navigation).
	if action == "search" {
		query := strings.TrimSpace(strings.Join(args[1:], " "))
		if query == "" {
			return fmt.Errorf("использование: navindex search <запрос>")
		}
		searcher := navindex.NewSearcher(newNavQdrant(cfg), embed.NewTEIClient(cfg.TEIURL))
		hits, err := searcher.Search(context.Background(), query, 5)
		if err != nil {
			return fmt.Errorf("поиск навигации: %w", err)
		}
		for i, h := range hits {
			fmt.Printf("%d. [%.3f] %s — %s (%s%s)\n   как попасть: %s\n",
				i+1, h.Score, h.PageTitle, h.Interface, h.Port, h.Route, h.HowTo)
		}
		return nil
	}

	tree := navindex.Tree()
	outDir := filepath.Join(cfg.DocsDir, "RAG_Структура_сайта")

	doRender := action == "all" || action == "render"
	doExport := action == "all" || action == "export"
	doIndex := action == "all" || action == "index"

	if doRender || doExport {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("создание каталога %s: %w", outDir, err)
		}
	}
	if doRender {
		mdPath := filepath.Join(outDir, "RAG_Структура_сайта.md")
		if err := os.WriteFile(mdPath, []byte(navindex.ToMarkdown(tree)), 0o644); err != nil {
			return fmt.Errorf("запись Markdown-карты: %w", err)
		}
		log.Printf("[navindex] Markdown-карта обновлена: %s", mdPath)
	}
	if doExport {
		jsonData, err := navindex.ToJSON(tree)
		if err != nil {
			return fmt.Errorf("сериализация JSON: %w", err)
		}
		jsonPath := filepath.Join(outDir, "navigation.json")
		if err := os.WriteFile(jsonPath, jsonData, 0o644); err != nil {
			return fmt.Errorf("запись navigation.json: %w", err)
		}
		log.Printf("[navindex] JSON-карта обновлена: %s", jsonPath)
	}
	if doIndex {
		ctx := context.Background()
		n, err := newNavIndexer(cfg).Reindex(ctx)
		if err != nil {
			return fmt.Errorf("индексация навигации: %w", err)
		}
		log.Printf("[navindex] навигация проиндексирована: %d узлов в коллекцию %q", n, cfg.NavColl)
	}
	return nil
}

func newScraper(cfg config.Config, st store.Store) *scraper.Scraper {
	sc := scraper.New(cfg.SourceURL, cfg.DocsDir, st)
	sc.MaxPages = cfg.ScrapeMaxPages
	sc.Delay = cfg.ScrapeDelay
	// На серверах без прямого доступа к dochub.sk.ru (гео/WAF) каталог ходит через прокси.
	sc.UseProxy(cfg.ProxyURL)
	return sc
}

// newPipeline собирает конвейер. proxyFn (опц.) — резолвер активного прокси из
// ProxyManager: если задан, каталог планировщика ходит через выбранный в админке
// прокси (динамически, без перезапуска). nil — статический PROXY_URL из env.
func newPipeline(cfg config.Config, st store.Store, svc *rag.Service, proxyFn func() string) *pipeline.Pipeline {
	sc := newScraper(cfg, st)
	if proxyFn != nil {
		sc.UseDynamicProxy(proxyFn)
	}
	newsMon := news.New(cfg.NewsRSSURL, cfg.DocsDir, st, svc)
	p := &pipeline.Pipeline{
		Scraper:   sc,
		Rag:       svc,
		News:      newsMon,
		Notifier:  notify.New(cfg.NotifyWebhook),
		ReportDir: cfg.ReportDir,
	}

	// Telegram-алерты консультанту об изменениях (no-op, если токен/чат не заданы).
	p.TG = notify.NewTelegramNotifier(os.Getenv("TELEGRAM_BOT_TOKEN"), cfg.ConsultantTelegramChatID)

	// Лента изменений, мониторинг свежести и трек «Актуальность изменений»
	// доступны только на Postgres-бэкенде.
	if ps, ok := st.(*store.PostgresStore); ok {
		ctx := context.Background()
		pool := ps.Pool()
		if cs, err := changes.NewPostgresStore(ctx, pool); err != nil {
			log.Printf("[pipeline] лента изменений недоступна: %v", err)
		} else {
			sc.Changes = cs
			newsMon.Changes = cs
			p.Changes = cs
		}
		if hs, err := health.NewPostgresStore(ctx, pool); err != nil {
			log.Printf("[pipeline] мониторинг свежести недоступен: %v", err)
		} else {
			p.Health = hs
		}

		// Трек «Актуальность изменений»: версии документов, анализатор важности
		// (LLM с эвристическим фоллбэком) и адресная рассылка уведомлений.
		versionStore := store.NewPostgresVersionStore(pool)
		sc.Versions = versionStore
		p.Analyzer = relevance.NewAnalyzer(versionStore, aimodels.NewStore(pool))
		mlr := mailer.New(mailer.Config{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		})
		p.Dispatcher = relevance.NewDispatcher(
			store.NewPostgresClientStore(pool),
			store.NewPostgresNotificationStore(pool),
			mlr,
			p.TG,
		)
	}

	return p
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
// applyFetchProfile проставляет фетчеру «человеческий» темп прогона из конфига.
func applyFetchProfile(f *fetcher.Fetcher, cfg config.Config) {
	f.BatchSize = cfg.FetchBatchSize
	f.BreakMin = cfg.FetchBreakMin
	f.BreakMax = cfg.FetchBreakMax
	f.LongPausePct = cfg.FetchLongPausePct
}

// catalogSpecs возвращает 8 канонических категорий документов.
func catalogSpecs() []fetcher.CategorySpec {
	cats := make([]fetcher.CategorySpec, 0, len(scraper.CategoryNames))
	for slug, name := range scraper.CategoryNames {
		cats = append(cats, fetcher.CategorySpec{Slug: slug, Name: name})
	}
	return cats
}

// upsertCatalogItems сохраняет найденные документы в реестр (дедуп по ссылке→ID).
// Возвращает (добавлено, дополнено категорией).
func upsertCatalogItems(ctx context.Context, st store.Store, items []fetcher.CatalogItem) (int, int) {
	var added, merged int
	for _, it := range items {
		title := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(it.Title), "File:"))
		if title == "" {
			continue
		}
		id := scraper.DocID(it.Link)

		if existing, err := st.Get(ctx, id); err == nil {
			if existing.Category == "" && it.Category != "" {
				existing.Category = it.Category
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
	return added, merged
}

// registerCookieDocs регистрирует файлы, скачанные по куке dochub (с прикреплённым
// телом), в реестр: дедуп по download-URL, переиндексация действующих. Возвращает
// число добавленных/обновлённых.
func registerCookieDocs(ctx context.Context, st store.Store, svc *rag.Service, docs []fetcher.CookieDoc) int {
	var n int
	for _, cd := range docs {
		if registerOneCookieDocCmd(ctx, st, svc, cd) {
			n++
		}
	}
	return n
}

// registerOneCookieDocCmd регистрирует один скачанный файл (инкрементально):
// обновляет существующую запись по download-URL либо создаёт новую с верным
// статусом; действующие сразу переиндексирует. Возвращает true при успехе.
func registerOneCookieDocCmd(ctx context.Context, st store.Store, svc *rag.Service, cd fetcher.CookieDoc) bool {
	id := scraper.DocID(cd.URL)
	if doc, err := st.Get(ctx, id); err == nil {
		doc.LocalPath, doc.FileHash, doc.Indexed = cd.LocalPath, cd.Hash, false
		if doc.Category == "" {
			doc.Category = cd.Category
		}
		if st.Upsert(ctx, doc) == nil {
			if svc != nil && doc.Status == model.StatusActive {
				if svc.Init(ctx) == nil {
					_, _ = svc.IndexDocument(ctx, id)
				}
			}
			return true
		}
		return false
	}
	doc := model.Document{
		ID: id, Title: cd.Title, SourceURL: cd.URL, Category: cd.Category,
		LocalPath: cd.LocalPath, FileHash: cd.Hash,
		Status: cookieDocStatus(cd.Title, cd.Category), FetchedAt: time.Now(),
	}
	return st.Upsert(ctx, doc) == nil
}

// cookieDocStatus определяет статус нового документа: «устарел» для категории
// утративших силу / заголовков с «УТРАТИЛ», иначе «на_проверке».
func cookieDocStatus(title, category string) model.Status {
	if category == scraper.CategoryNames["unactual_documents"] ||
		strings.Contains(strings.ToUpper(title), "УТРАТИЛ") {
		return model.StatusOutdated
	}
	return model.StatusPending
}

func cmdCatalog(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	f, err := fetcher.New(cfg.ChromePath, cfg.ProxyURL, cfg.FetchWait, func() string { return cfg.ProxyURL })
	if err != nil {
		return err
	}
	applyFetchProfile(f, cfg)

	cats := catalogSpecs()

	items, err := f.EnumerateCategoriesAuto(ctx, cfg.SourceURL, cats)
	if err != nil {
		return err
	}
	added, merged := upsertCatalogItems(ctx, st, items)
	fmt.Printf("Каталог: найдено %d, добавлено %d, дополнено %d\n", len(items), added, merged)
	return nil
}

// cmdCrawl — полный обход всего сайта документов (категории + sitemap + ссылки).
func cmdCrawl(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	f, err := fetcher.New(cfg.ChromePath, cfg.ProxyURL, cfg.FetchWait, func() string { return cfg.ProxyURL })
	if err != nil {
		return err
	}
	applyFetchProfile(f, cfg)

	items, err := f.EnumerateSiteAuto(ctx, cfg.SourceURL, catalogSpecs(), cfg.CrawlMaxPages)
	if err != nil {
		return err
	}
	added, merged := upsertCatalogItems(ctx, st, items)
	fmt.Printf("Обход сайта: найдено %d, добавлено %d, дополнено %d\n", len(items), added, merged)
	return nil
}

// cmdFetch скачивает тела файлов dochub. Основной путь — по сессионной куке
// (env DOCHUB_COOKIE или сохранённая в админке): обычный HTTP без браузера и
// прокси, обходит WAF. FETCH_LIMIT ограничивает число файлов за прогон (0 — все).
// Скачанные регистрируются «на_проверке»; индексируются после одобрения.
func cmdFetch(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := newRAG(cfg, st, nil)

	cookie := cfg.DochubCookie
	if cookie == "" {
		cookie = admin.LoadDochubCookie(cfg.DocsDir)
	}
	if cookie != "" {
		f, err := fetcher.New("", "", cfg.FetchWait, nil) // без Chrome и без прокси
		if err != nil {
			return err
		}
		applyFetchProfile(f, cfg)
		f.Cookie = cookie
		// Возобновляемость: не качаем то, что уже скачано (есть файл на диске).
		f.SkipURL = func(u string) bool {
			if d, derr := st.Get(ctx, scraper.DocID(u)); derr == nil && d.LocalPath != "" {
				if _, serr := os.Stat(d.LocalPath); serr == nil {
					return true
				}
			}
			return false
		}
		outDir := filepath.Join(cfg.DocsDir, "На_проверке", "Загружено")
		fmt.Println("Скачивание тел файлов dochub по куке (без браузера и прокси)…")
		n := 0
		docs, errs := f.CollectViaCookie(ctx, cfg.SourceURL, catalogSpecs(), outDir, cfg.FetchLimit,
			func(m string) { fmt.Println("  " + m) },
			func(cd fetcher.CookieDoc) { // регистрируем сразу, по мере скачивания
				if registerOneCookieDocCmd(ctx, st, svc, cd) {
					n++
				}
			})
		fmt.Printf("Готово: скачано файлов %d, в реестр %d, ошибок %d (часть — «мёртвые» дубли /m/docs/, это норма)\n", len(docs), n, len(errs))
		if len(docs) == 0 && len(errs) > 0 {
			fmt.Println("0 файлов — вероятно, кука dochub протухла. Обновите её (админка /proxy или DOCHUB_COOKIE).")
		}
		return nil
	}

	// Фоллбэк: старый headless+прокси путь (обычно блокируется WAF).
	fmt.Println("DOCHUB_COOKIE не задана — пробую headless+прокси (обычно блокируется WAF). Надёжнее задать куку в админке /proxy.")
	f, err := fetcher.New(cfg.ChromePath, cfg.ProxyURL, cfg.FetchWait, func() string { return cfg.ProxyURL })
	if err != nil {
		return err
	}
	applyFetchProfile(f, cfg)
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

func cmdResidents(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	ps, ok := st.(*store.PostgresStore)
	if !ok {
		log.Printf("[residents] предупреждение: требуется backend=postgres")
		fmt.Println("Резиденты: требуется backend=postgres")
		return nil
	}
	residentStore := store.NewPostgresSourceStore(ps.Pool())

	parsed, err := residents.ParseResidents(ctx, residents.Config{SourceURL: cfg.ResidentsURL}, nil)
	if err != nil {
		return fmt.Errorf("парсинг реестра резидентов: %w", err)
	}

	res, err := residents.IngestResidents(ctx, parsed, residentStore)
	if err != nil {
		return err
	}
	fmt.Printf("Резиденты: получено %d, новых %d, обновлено %d, ошибок %d\n",
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

// cmdMigrate применяет миграции БД.
func cmdMigrate(cfg config.Config) error {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	dir := resolveMigrationsDir(cfg.MigrationsDir)
	if dir == "" {
		return fmt.Errorf("директория миграций не найдена (искал %s, ./deploy/migrations)", cfg.MigrationsDir)
	}
	return migrate.ApplyMigrations(ctx, pool, dir)
}

// resolveMigrationsDir возвращает первую существующую директорию миграций из
// настроенного пути и распространённых fallback-расположений ("" — не найдена).
func resolveMigrationsDir(configured string) string {
	candidates := []string{configured, "./migrations", "./deploy/migrations"}
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d
		}
	}
	return ""
}

// autoMigrate применяет миграции при старте serve (best-effort): без директории
// миграций сервис продолжит работу, но реляционные таблицы могут отсутствовать.
func autoMigrate(ctx context.Context, cfg config.Config) {
	if !cfg.AutoMigrate {
		return
	}
	dir := resolveMigrationsDir(cfg.MigrationsDir)
	if dir == "" {
		log.Printf("[serve:migrate] предупреждение: директория миграций не найдена (%s) — пропуск авто-миграций", cfg.MigrationsDir)
		return
	}
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Printf("[serve:migrate] предупреждение: не удалось подключиться к БД: %v", err)
		return
	}
	defer pool.Close()
	if err := migrate.ApplyMigrations(ctx, pool, dir); err != nil {
		log.Printf("[serve:migrate] предупреждение: миграции не применены: %v", err)
	}
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

	svc := newRAG(cfg, st, nil)
	// Headless-обход сайта + скачивание тел файлов (обход WAF) до прочего цикла.
	if found, fetched, herr := headlessCollect(ctx, cfg, st, svc, nil /*pm unavailable in cmdSync*/); herr != nil {
		fmt.Printf("Sync: headless-сбор пропущен: %v\n", herr)
	} else {
		fmt.Printf("Sync: headless — найдено %d, скачано %d\n", found, fetched)
	}
	return newPipeline(cfg, st, svc, nil).RunOnce(ctx)
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
	registerExtraMCPTools(mcpSrv, st, cfg)

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
		cfg.ChromePath, cfg.ProxyURL, cfg.SourceURL, cfg.FetchWait, st, newRAG(cfg, st, nil)).
		WithNavIndexer(newNavIndexer(cfg))

	// Подключаем хранилище ленты изменений (только для Postgres-бэкенда).
	if ps, ok := st.(*store.PostgresStore); ok {
		if cs, err := changes.NewPostgresStore(ctx, ps.Pool()); err == nil {
			srv.WithChangeStore(cs)
		} else {
			log.Printf("[admin] хранилище изменений: %v", err)
		}
		// Мониторинг свежести источников для панели на /changes.
		if hs, err := health.NewPostgresStore(ctx, ps.Pool()); err == nil {
			srv.WithHealthStore(hs)
		}
		// Страницы публичного сайта (раздел «Страницы сайта»: список + просмотрщик).
		if sps, err := sitepages.NewPostgresStore(ctx, ps.Pool()); err == nil {
			srv.WithSitePageStore(sps)
			srv.WithSitePageSearcher(sitepages.NewSearcher(newSitePagesQdrant(cfg), embed.NewTEIClient(cfg.TEIURL)))
			enr := sitepages.NewEnricher(aimodels.NewStore(ps.Pool()), sps, cfg.SitePagesEnrichDelay)
			srv.WithSitePageActions(sitepages.NewAdminService(sps, enr, newSitePagesIndexer(cfg)))
		}
		// Подключаем хранилища льгот и НПА.
		pss := store.NewPostgresSourceStore(ps.Pool())
		srv.WithPreferenceStore(pss)
		srv.WithNPAStore(pss)
		// История версий документов — для автоматического ИИ-сравнения редакций (/diff).
		srv.WithVersionStore(store.NewPostgresVersionStore(ps.Pool()))
	}

	// Подключаем AI-хранилище (только для Postgres-бэкенда).
	if ps, ok := st.(*store.PostgresStore); ok {
		aiStore := aimodels.NewStore(ps.Pool())
		srv.WithAIStore(aiStore)
		if cfg.QwenAPIKey != "" {
			if err := aiStore.SeedQwenModels(ctx, cfg.QwenAPIKey); err != nil {
				log.Printf("[admin] AI seed Qwen: %v", err)
			}
			models, _ := aiStore.ListModels(ctx)
			if len(models) > 0 {
				if err := aiStore.SeedDefaultAgents(ctx, models[0].ID); err != nil {
					log.Printf("[admin] AI seed agents: %v", err)
				}
			}
		}
	}

	// Создаём mux для маршрутов резидентства и запускаем его на отдельном порту (BasicAuth).
	residencyMux := admin.RegisterResidencyRoutes(nil, buildResidencyStores(st))
	residencyAddr := ":8091"
	var residencyHandlerAdmin http.Handler = residencyMux
	if cfg.AdminUser != "" && cfg.AdminPassword != "" {
		residencyHandlerAdmin = admin.BasicAuth(cfg.AdminUser, cfg.AdminPassword, "Резидентство-Админ", residencyMux)
	}
	go func() {
		log.Printf("[admin:residency] запуск на %s (BasicAuth)", residencyAddr)
		if err := http.ListenAndServe(residencyAddr, residencyHandlerAdmin); err != nil {
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

	// --- Авто-миграции БД (создают реляционные таблицы до старта подсистем) ---
	autoMigrate(ctx, cfg)

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
	registerExtraMCPTools(mcpSrv.MCPServer(), st, cfg)
	registerAgentMCPTools(mcpSrv.MCPServer(), st, svc, cfg)

	// --- Навигационный индекс сайта (для get_navigation) ---
	// Пересобираем при старте из src/navindex (источник истины), чтобы чат-бот
	// всегда знал актуальную структуру страниц/блоков. Фоном, не блокируя старт.
	go func() {
		if n, err := newNavIndexer(cfg).Reindex(context.Background()); err != nil {
			log.Printf("[serve:navindex] навигация не проиндексирована: %v", err)
		} else {
			log.Printf("[serve:navindex] навигация проиндексирована: %d узлов", n)
		}
	}()

	// --- Индекс страниц публичного сайта (для search_site_pages) ---
	// Разогреваем коллекцию из уже собранных страниц (если backend=postgres).
	if cfg.SitePagesEnabled {
		if ps, ok := st.(*store.PostgresStore); ok {
			go func() {
				ctx := context.Background()
				pageStore, err := sitepages.NewPostgresStore(ctx, ps.Pool())
				if err != nil {
					log.Printf("[serve:sitepages] хранилище недоступно: %v", err)
					return
				}
				pages, err := pageStore.ListAll(ctx)
				if err != nil {
					log.Printf("[serve:sitepages] чтение страниц: %v", err)
					return
				}
				if n, err := newSitePagesIndexer(cfg).Reindex(ctx, pages); err != nil {
					log.Printf("[serve:sitepages] страницы не проиндексированы: %v", err)
				} else {
					log.Printf("[serve:sitepages] страницы проиндексированы: %d", n)
				}
			}()
		}
	}

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

	// --- Админка резидентства (BasicAuth) ---
	residencyMux := admin.RegisterResidencyRoutes(nil, buildResidencyStores(st))
	residencyAddr := ":8091"
	var residencyHandler http.Handler = residencyMux
	if cfg.AdminUser != "" && cfg.AdminPassword != "" {
		residencyHandler = admin.BasicAuth(cfg.AdminUser, cfg.AdminPassword, "Резидентство-Админ", residencyMux)
	}
	go func() {
		log.Printf("[admin:residency] запуск на %s (BasicAuth)", residencyAddr)
		if err := http.ListenAndServe(residencyAddr, residencyHandler); err != nil {
			log.Printf("[admin:residency] остановлен: %v", err)
		}
	}()

	adminSrv := admin.New(cfg.AdminAddr, cfg.AdminUser, cfg.AdminPassword, cfg.DocsDir,
		cfg.ChromePath, cfg.ProxyURL, cfg.SourceURL, cfg.FetchWait, st, svc).
		WithNavIndexer(newNavIndexer(cfg))

	// Подключаем хранилища льгот и НПА к админке.
	if ps, ok := st.(*store.PostgresStore); ok {
		pss := store.NewPostgresSourceStore(ps.Pool())
		adminSrv.WithPreferenceStore(pss)
		adminSrv.WithNPAStore(pss)
		// История версий документов — для автоматического ИИ-сравнения редакций (/diff).
		adminSrv.WithVersionStore(store.NewPostgresVersionStore(ps.Pool()))

		// Лента изменений нужна странице /changes (в режиме serve её раньше
		// не подключали — страница истории получалась пустой).
		if cs, err := changes.NewPostgresStore(ctx, ps.Pool()); err == nil {
			adminSrv.WithChangeStore(cs)
		} else {
			log.Printf("[serve:admin] лента изменений недоступна: %v", err)
		}
		// Мониторинг свежести источников для панели «когда обновлялось» на /changes.
		if hs, err := health.NewPostgresStore(ctx, ps.Pool()); err == nil {
			adminSrv.WithHealthStore(hs)
		} else {
			log.Printf("[serve:admin] мониторинг свежести недоступен: %v", err)
		}
		// Страницы публичного сайта (раздел «Страницы сайта»).
		if sps, err := sitepages.NewPostgresStore(ctx, ps.Pool()); err == nil {
			adminSrv.WithSitePageStore(sps)
			adminSrv.WithSitePageSearcher(sitepages.NewSearcher(newSitePagesQdrant(cfg), embed.NewTEIClient(cfg.TEIURL)))
			enr := sitepages.NewEnricher(aimodels.NewStore(ps.Pool()), sps, cfg.SitePagesEnrichDelay)
			adminSrv.WithSitePageActions(sitepages.NewAdminService(sps, enr, newSitePagesIndexer(cfg)))
		}
	}

	// Подключаем менеджер прокси к админке.
	pm := admin.NewProxyManager(filepath.Join(cfg.DocsDir, ".admin", "proxies.json"))
	adminSrv.WithProxyManager(pm).WithProxy6APIKey(cfg.Proxy6APIKey)

	// Подключаем AI-хранилище к админке (только для Postgres-бэкенда).
	if ps, ok := st.(*store.PostgresStore); ok {
		aiStore := aimodels.NewStore(ps.Pool())
		adminSrv.WithAIStore(aiStore)
		if cfg.QwenAPIKey != "" {
			if err := aiStore.SeedQwenModels(ctx, cfg.QwenAPIKey); err != nil {
				log.Printf("[serve] AI seed Qwen: %v", err)
			}
			models, _ := aiStore.ListModels(ctx)
			if len(models) > 0 {
				if err := aiStore.SeedDefaultAgents(ctx, models[0].ID); err != nil {
					log.Printf("[serve] AI seed agents: %v", err)
				}
			}
		}
	}

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
				Addr:                cfg.PortalAddr,
				BaseURL:             "http://localhost" + cfg.PortalAddr,
				MCPURL:              "http://localhost" + cfg.MCPAddr,
				MCPAPIKey:           cfg.MCPAPIKey,
				TelegramBotUsername: cfg.TelegramBotUsername,
			}
			stores := buildResidencyStores(st)
			gen := newGenerator(cfg, st)
			ensureDefaultTemplates(gen, cfg.GeneratorTemplateDir)
			mlr := mailer.New(mailer.Config{
				Host:     cfg.SMTPHost,
				Port:     cfg.SMTPPort,
				Username: cfg.SMTPUser,
				Password: cfg.SMTPPassword,
				From:     cfg.SMTPFrom,
			})
			portalStores := portal.PortalStores{
				ClientStore:    stores.ClientStore,
				ChecklistStore: stores.ChecklistStore,
				DeadlineStore:  stores.DeadlineStore,
				TemplateStore:  stores.TemplateStore,
				DocumentStore:  st,
				Generator:      gen,
				Mailer:         mlr,
			}
			if pgs, ok := st.(*store.PostgresStore); ok {
				pcs := store.NewPostgresClientStore(pgs.Pool())
				portalStores.NotifStore = store.NewPostgresNotificationStore(pgs.Pool())
				portalStores.DocStore = pcs
				portalStores.SubscriptionStore = pcs
				// Лента изменений для блока «Что нового в базе Сколково» в дашборде
				// (без неё getRecentChanges возвращает пусто — блок не рендерится).
				if cs, err := changes.NewPostgresStore(ctx, pgs.Pool()); err == nil {
					portalStores.ChangeStore = cs
				}
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
	// Прокси для каталога: сначала активный из ProxyManager (управляется в админке
	// :8090 /proxy), иначе статический PROXY_URL из env. Резолвится на каждый запрос.
	proxyResolver := func() string {
		if u := pm.GetActiveURL(); u != "" {
			return u
		}
		return cfg.ProxyURL
	}
	go newPipeline(cfg, st, svc, proxyResolver).Schedule(ctx, cfg.ScrapeInterval)

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
				if cs, err := changes.NewPostgresStore(context.Background(), ps.Pool()); err == nil {
					consultantStores.ChangesStore = cs
				}
			}
			if consultantStores.ClientStore != nil {
				h := admin.RegisterConsultantRoutes(nil, consultantStores, cfg.ConsultantUser, cfg.ConsultantPass)
				log.Printf("[consultant] дашборд запускается на %s", cfg.ConsultantAddr)
				if err := http.ListenAndServe(cfg.ConsultantAddr, h); err != nil {
					log.Printf("[consultant] остановлен: %v", err)
				}
			} else {
				log.Printf("[consultant] пропущен: требуется backend=postgres")
			}
		}()
	}

	// --- Планировщик для новых модулей ---
	go scheduleNewModules(ctx, cfg, st, svc, pm)

	// --- Ежедневная сводка консультанту ---
	if tgNotifier.Enabled() {
		go runDailySummary(ctx, cfg, st, tgNotifier)
	}

	<-ctx.Done()
	log.Println("[serve] останов по сигналу")
	return nil
}

// cmdDoctor проверяет доступность инфраструктуры (PostgreSQL, Qdrant, TEI) и
// наличие миграций. Удобно для smoke-теста после развёртывания.
func cmdDoctor(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	type check struct {
		name string
		err  error
	}
	var checks []check

	// PostgreSQL
	func() {
		pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
		if err != nil {
			checks = append(checks, check{"PostgreSQL (подключение)", err})
			return
		}
		defer pool.Close()
		var one int
		err = pool.QueryRow(ctx, "SELECT 1").Scan(&one)
		checks = append(checks, check{"PostgreSQL (запрос)", err})
	}()

	// Qdrant
	checks = append(checks, check{"Qdrant", httpHealth(ctx, strings.TrimRight(cfg.QdrantURL, "/")+"/healthz")})

	// TEI (эмбеддинги)
	func() {
		c := embed.NewTEIClient(cfg.TEIURL)
		_, err := c.Embed(ctx, []string{embed.PrefixQuery + "проверка связи"})
		checks = append(checks, check{"TEI (эмбеддинги)", err})
	}()

	// Миграции на диске
	if dir := resolveMigrationsDir(cfg.MigrationsDir); dir == "" {
		checks = append(checks, check{"Миграции (каталог)", fmt.Errorf("не найден (%s)", cfg.MigrationsDir)})
	} else {
		checks = append(checks, check{"Миграции (каталог " + dir + ")", nil})
	}

	fmt.Println("Проверка инфраструктуры «База Сколково»:")
	allOK := true
	for _, c := range checks {
		if c.err != nil {
			allOK = false
			fmt.Printf("  ❌ %s: %v\n", c.name, c.err)
		} else {
			fmt.Printf("  ✅ %s\n", c.name)
		}
	}
	if !allOK {
		return fmt.Errorf("часть проверок не прошла")
	}
	fmt.Println("Все проверки пройдены — инфраструктура готова.")
	return nil
}

// httpHealth выполняет GET и считает успехом статус 2xx.
func httpHealth(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("статус %d", resp.StatusCode)
	}
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
func scheduleNewModules(ctx context.Context, cfg config.Config, st store.Store, svc *rag.Service, pm *admin.ProxyManager) {
	// Первый прогон — сразу при старте serve (After(0)), далее каждый интервал.
	// Иначе после рестарта источники (мероприятия/конкурсы/sitepages + скачивание
	// тел по куке) не обновлялись бы до первого тика (6 ч по умолчанию).
	tick := time.After(0)

	var eventStore store.EventStore
	var contestStore store.ContestStore
	var faqStore store.FAQStore
	var tgStore store.TelegramStore
	var residentStore store.ResidentStore
	var healthStore health.Store
	var changeStore changes.Recorder
	var sitePageStore *sitepages.PostgresStore
	var aiStore *aimodels.Store

	if ps, ok := st.(*store.PostgresStore); ok {
		pss := store.NewPostgresSourceStore(ps.Pool())
		eventStore = pss
		contestStore = pss
		faqStore = pss
		tgStore = pss
		residentStore = pss
		if hs, err := health.NewPostgresStore(ctx, ps.Pool()); err == nil {
			healthStore = hs
		}
		if cs, err := changes.NewPostgresStore(ctx, ps.Pool()); err == nil {
			changeStore = cs
		}
		if sps, err := sitepages.NewPostgresStore(ctx, ps.Pool()); err == nil {
			sitePageStore = sps
		}
		aiStore = aimodels.NewStore(ps.Pool())
	}

	// ИИ-обогащение страниц сайта (теги/описание/цели/тезисы/выводы). Безопасно,
	// если агент «Аннотатор страниц» не настроен — аннотирование просто пропускается.
	var sitePageEnricher *sitepages.Enricher
	if cfg.SitePagesEnrichEnabled && aiStore != nil && sitePageStore != nil {
		_ = aiStore.EnsurePageAnnotatorAgent(ctx)
		sitePageEnricher = sitepages.NewEnricher(aiStore, sitePageStore, cfg.SitePagesEnrichDelay)
		// Стартовый бэкфилл: единожды аннотируем все ещё не обогащённые страницы.
		go func() {
			pend, err := sitePageStore.ListNeedingEnrichment(ctx, 0)
			if err != nil || len(pend) == 0 {
				return
			}
			log.Printf("[serve:sitepages] стартовый бэкфилл аннотаций: %d страниц", len(pend))
			d, s, f := sitePageEnricher.EnrichBatch(ctx, pend)
			log.Printf("[serve:sitepages] бэкфилл аннотаций завершён: обогащено %d, пропущено %d, ошибок %d", d, s, f)
			if d > 0 {
				if pages, lerr := sitePageStore.ListAll(ctx); lerr == nil {
					if n, ierr := newSitePagesIndexer(cfg).Reindex(ctx, pages); ierr != nil {
						log.Printf("[serve:sitepages] переиндексация после бэкфилла: %v", ierr)
					} else {
						log.Printf("[serve:sitepages] переиндексировано %d страниц после бэкфилла", n)
					}
				}
			}
		}()
	}

	// proxyResolver: активный прокси из админки, иначе статический PROXY_URL.
	proxyResolver := func() string {
		if u := pm.GetActiveURL(); u != "" {
			return u
		}
		return cfg.ProxyURL
	}

	// recordHealth фиксирует результат прогона источника (no-op без Postgres).
	recordHealth := func(source string, items int, runErr error) {
		if healthStore == nil {
			return
		}
		_ = healthStore.Record(ctx, source, items, runErr)
	}

	httpCl := &http.Client{Timeout: 60 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			tick = time.After(cfg.ScrapeInterval) // запланировать следующий прогон
			// Скачивание тел файлов dochub (по куке) + обновление источников.
			runScheduledCollect(ctx, cfg, st, svc, pm, recordHealth)

			// Мероприятия
			if cfg.EventsSourceURL != "" && eventStore != nil {
				log.Printf("[serve:events] запуск парсинга мероприятий")
				evCfg := events.EventsConfig{RSSURL: cfg.EventsRSSURL, SourceURL: cfg.EventsSourceURL}
				parsed, err := events.ParseEvents(ctx, evCfg, httpCl)
				if err != nil {
					log.Printf("[serve:events] ошибка: %v", err)
					recordHealth("events", 0, err)
				} else {
					res, ingErr := events.IngestEvents(ctx, parsed, eventStore, nil, changeStore)
					recordHealth("events", res.New+res.Updated, ingErr)
				}
			}

			// Конкурсы
			if cfg.ContestsURL != "" && contestStore != nil {
				log.Printf("[serve:contests] запуск парсинга конкурсов")
				cCfg := contests.ContestsConfig{ContestsURL: cfg.ContestsURL, GrantsURL: cfg.GrantsURL}
				parsed, err := contests.ParseContests(ctx, cCfg, httpCl)
				if err != nil {
					log.Printf("[serve:contests] ошибка: %v", err)
					recordHealth("contests", 0, err)
				} else {
					res, ingErr := contests.IngestContests(ctx, parsed, contestStore, nil, changeStore)
					recordHealth("contests", res.New+res.Updated, ingErr)
				}
			}

			// FAQ
			if cfg.FAQURL != "" && faqStore != nil {
				log.Printf("[serve:faq] запуск парсинга FAQ")
				fCfg := faq.FAQConfig{FAQURL: cfg.FAQURL}
				parsed, err := faq.ParseFAQ(ctx, fCfg, httpCl)
				if err != nil {
					log.Printf("[serve:faq] ошибка: %v", err)
					recordHealth("faq", 0, err)
				} else {
					res, ingErr := faq.IngestFAQ(ctx, parsed, faqStore, nil, changeStore)
					recordHealth("faq", res.New+res.Updated, ingErr)
				}
			}

			// Реестр резидентов
			if cfg.ResidentsEnabled && cfg.ResidentsURL != "" && residentStore != nil {
				log.Printf("[serve:residents] запуск парсинга реестра резидентов")
				parsed, err := residents.ParseResidents(ctx, residents.Config{SourceURL: cfg.ResidentsURL}, httpCl)
				if err != nil {
					log.Printf("[serve:residents] ошибка: %v", err)
					recordHealth("residents", 0, err)
				} else {
					res, ingErr := residents.IngestResidents(ctx, parsed, residentStore, changeStore)
					recordHealth("residents", res.New+res.Updated, ingErr)
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
						recordHealth("telegram", 0, err)
					} else {
						res, ingErr := telegram.IngestPosts(ctx, parsed, tgStore)
						recordHealth("telegram", res.New, ingErr)
					}
				}
			}

			// Льготы резидентов
			if cfg.PreferencesEnabled {
				log.Printf("[serve:preferences] синхронизация льгот")
				prefCfg := preferences.PreferencesConfig{
					SourceURL: cfg.PreferencesURL,
					Category:  "Льготы",
				}
				mon := preferences.New(prefCfg, st)
				if res, err := mon.Run(ctx, changeStore); err != nil {
					log.Printf("[serve:preferences] ошибка: %v", err)
					recordHealth("preferences", 0, err)
				} else {
					log.Printf("[serve:preferences] готово: новых %d, обновлено %d", res.New, res.Updated)
					recordHealth("preferences", res.New+res.Updated, nil)
				}
			}

			// НПА
			if cfg.RegulationsEnabled {
				log.Printf("[serve:regulations] синхронизация НПА")
				regCfg := regulations.RegulationsConfig{
					SourceURL: cfg.RegulationsSearchURL,
					Category:  "НПА",
				}
				mon := regulations.New(regCfg, st)
				if res, err := mon.Run(ctx, changeStore); err != nil {
					log.Printf("[serve:regulations] ошибка: %v", err)
					recordHealth("regulations", 0, err)
				} else {
					log.Printf("[serve:regulations] готово: новых %d, обновлено %d", res.New, res.Updated)
					recordHealth("regulations", res.New+res.Updated, nil)
				}
			}

			// Страницы публичного сайта (отдельно от файлов-документов)
			if cfg.SitePagesEnabled && sitePageStore != nil {
				log.Printf("[serve:sitepages] обход страниц сайта")
				cr := sitepages.New(sitePagesSeeds(cfg), sitePageStore)
				cr.MaxPages = cfg.SitePagesMaxPages
				cr.Delay = cfg.ScrapeDelay
				cr.Changes = changeStore
				cr.UseDynamicProxy(proxyResolver)
				rep, err := cr.Run(ctx)
				if err != nil {
					log.Printf("[serve:sitepages] ошибка обхода: %v", err)
					recordHealth("sitepages", 0, err)
				} else {
					recordHealth("sitepages", rep.New+rep.Changed, nil)
					// Инкрементально доиндексируем изменённые/новые страницы.
					if rep.New+rep.Changed > 0 {
						// Сначала аннотируем новые/изменённые страницы через ИИ.
						if sitePageEnricher != nil {
							if pend, perr := sitePageStore.ListNeedingEnrichment(ctx, rep.New+rep.Changed); perr == nil && len(pend) > 0 {
								d, s, f := sitePageEnricher.EnrichBatch(ctx, pend)
								log.Printf("[serve:sitepages] аннотирование: обогащено %d, пропущено %d, ошибок %d", d, s, f)
							}
						}
						if pages, lerr := sitePageStore.ListRecent(ctx, rep.New+rep.Changed); lerr == nil {
							if n, ierr := newSitePagesIndexer(cfg).Reindex(ctx, pages); ierr != nil {
								log.Printf("[serve:sitepages] индексация: %v", ierr)
							} else {
								log.Printf("[serve:sitepages] готово: новых %d, изменено %d, проиндексировано %d", rep.New, rep.Changed, n)
							}
						}
					} else {
						log.Printf("[serve:sitepages] готово: без изменений (посещено %d)", rep.Visited)
					}
				}
			}
		}
	}
}

// registerExtraMCPTools регистрирует дополнительные MCP-инструменты (резидентство,
// источники, навигацию по сайту).
func registerExtraMCPTools(mcpSrv *server.MCPServer, st store.Store, cfg config.Config) {
	// Навигация по сайту (get_navigation) работает на любом бэкенде — индекс
	// в отдельной Qdrant-коллекции, не зависит от Postgres.
	navSearcher := navindex.NewSearcher(newNavQdrant(cfg), embed.NewTEIClient(cfg.TEIURL))
	mcpserver.RegisterNavigationTools(mcpSrv, navSearcher)

	// Поиск по страницам публичного сайта (search_site_pages) — отдельная
	// Qdrant-коллекция, не зависит от Postgres.
	pageSearcher := sitepages.NewSearcher(newSitePagesQdrant(cfg), embed.NewTEIClient(cfg.TEIURL))
	mcpserver.RegisterSitePageTools(mcpSrv, pageSearcher)

	ps, ok := st.(*store.PostgresStore)
	if !ok {
		log.Printf("[mcp] дополнительные инструменты: требуется backend=postgres")
		return
	}
	pool := ps.Pool()
	pcs := store.NewPostgresClientStore(pool)
	pss := store.NewPostgresSourceStore(pool)

	// Регистрируем инструменты резидентства.
	mcpserver.RegisterResidencyTools(mcpSrv, pcs, pcs, pcs, pcs, pcs)

	// Регистрируем инструменты источников (включая реестр резидентов).
	mcpserver.RegisterSourceTools(mcpSrv, pss, pss, pss, pss, nil)

	// Лента изменений: get_recent_changes.
	ctx := context.Background()
	if cs, err := changes.NewPostgresStore(ctx, pool); err != nil {
		log.Printf("[mcp] get_recent_changes недоступен: %v", err)
	} else {
		mcpserver.RegisterChangeTools(mcpSrv, cs)
	}

	// Мониторинг свежести источников: get_source_health.
	if hs, err := health.NewPostgresStore(ctx, pool); err != nil {
		log.Printf("[mcp] get_source_health недоступен: %v", err)
	} else {
		mcpserver.RegisterHealthTools(mcpSrv, hs)
	}
}

// formatConsultantAnswer добавляет к ответу консультанта список источников,
// чтобы пользователь (виджет/бот/агент) видел, на чём основан ответ.
func formatConsultantAnswer(resp agents.ConsultantResponse) string {
	out := resp.Answer
	if len(resp.Sources) > 0 {
		out += "\n\n📚 Источники:"
		for _, s := range resp.Sources {
			line := "\n• " + s.Title
			if s.SourceURL != "" {
				line += " — " + s.SourceURL
			}
			out += line
		}
	}
	return out
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

	consultant := agents.NewConsultantAgent(ragSvc, "http://"+cfg.MCPAddr, cfg.MCPAPIKey).
		WithLLM(aimodels.NewStore(pool)).
		WithNavigation(navindex.NewSearcher(newNavQdrant(cfg), embed.NewTEIClient(cfg.TEIURL)))
	validator := agents.NewValidatorAgent(ragSvc, pcs)
	monitor := agents.NewMonitorAgent(agents.MonitorStores{
		DocStore:      st,
		EventStore:    store.NewPostgresSourceStore(pool),
		ContestStore:  store.NewPostgresSourceStore(pool),
		ClientStore:   pcs,
		DeadlineStore: pcs,
	})
	coordinator := agents.NewCoordinatorAgent(agents.CoordinatorStores{
		ClientStore:    pcs,
		ChecklistStore: pcs,
		DeadlineStore:  pcs,
		TemplateStore:  pcs,
	})
	drafter := agents.NewDocumentDraftingAgent(agents.DraftingStores{
		ClientStore:    pcs,
		TemplateStore:  pcs,
		ChecklistStore: pcs,
	}, ragSvc)

	deps := mcpserver.AgentToolDeps{
		Consultant:  consultant,
		Validator:   validator,
		Monitor:     monitor,
		Coordinator: coordinator,
		Drafter:     drafter,
		Store:       st,
		Config:      cfg,
	}
	if cfg.EligibilityEnabled {
		deps.Eligibility = eligibility.NewChecker(eligibility.Config{DadataAPIKey: cfg.DadataAPIKey})
	}
	deps.Generator = newGenerator(cfg, st)

	mcpserver.RegisterAgentTools(mcpSrv, deps)
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

	// Создаём консультанта для бота: реальный RAG + LLM-синтез (если настроен).
	consultant := agents.NewConsultantAgent(newRAG(cfg, st, nil), "http://"+cfg.MCPAddr, cfg.MCPAPIKey)
	if ps, ok := st.(*store.PostgresStore); ok {
		consultant = consultant.WithLLM(aimodels.NewStore(ps.Pool()))
	}

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
		SourceURL: cfg.PreferencesURL,
		Category:  "Льготы",
	}
	mon := preferences.New(prefCfg, st)
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

	regCfg := regulations.RegulationsConfig{
		SourceURL: cfg.RegulationsSearchURL,
		Category:  "НПА",
	}
	mon := regulations.New(regCfg, st)
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

// cmdAudit строит отчёт о полноте охвата источников и сохраняет его в ReportDir.
func cmdAudit(cfg config.Config) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	rep := audit.BuildCoverageReport(ctx, cfg, st)
	md := audit.ToMarkdown(rep)

	if err := os.MkdirAll(cfg.ReportDir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("ОТЧЕТ_Охват_источников_%s.md", time.Now().Format("2006-01-02_150405"))
	path := filepath.Join(cfg.ReportDir, name)
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return err
	}

	fmt.Printf("Аудит охвата: покрыто %d из %d источников. Отчёт: %s\n", rep.CoveredN, rep.TotalN, path)
	for _, c := range rep.Sources {
		fmt.Printf("  [%s] %s — записей %s\n", c.Status, c.Name, itemsStr(c.Items))
	}
	return nil
}

func itemsStr(n int) string {
	if n < 0 {
		return "—"
	}
	return fmt.Sprintf("%d", n)
}

// genClientStore адаптирует PostgresClientStore к generator.ClientStore
// (ListClientDocuments возвращает значения вместо указателей).
type genClientStore struct {
	pcs *store.PostgresClientStore
}

func (g genClientStore) GetClient(ctx context.Context, id string) (*model.Client, error) {
	return g.pcs.GetClient(ctx, id)
}

func (g genClientStore) ListClientDocuments(ctx context.Context, id string) ([]model.ClientDocument, error) {
	ptrs, err := g.pcs.ListClientDocuments(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]model.ClientDocument, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

// newGenerator собирает генератор документов с файловым хранилищем шаблонов.
// Возвращает nil, если backend не Postgres (нужен доступ к данным клиентов).
func newGenerator(cfg config.Config, st store.Store) *generator.DocumentGenerator {
	ps, ok := st.(*store.PostgresStore)
	if !ok {
		return nil
	}
	pcs := store.NewPostgresClientStore(ps.Pool())
	gcfg := generator.GeneratorConfig{
		TemplateDir: cfg.GeneratorTemplateDir,
		OutputDir:   cfg.GeneratorOutputDir,
		ChromePath:  cfg.ChromePath,
	}
	return generator.NewDocumentGenerator(gcfg, genClientStore{pcs: pcs},
		generator.NewFileTemplateStore(cfg.GeneratorTemplateDir))
}

// ensureDefaultTemplates создаёт стандартные шаблоны, если в директории их ещё нет
// (не перезаписывает существующие/кастомизированные шаблоны).
func ensureDefaultTemplates(gen *generator.DocumentGenerator, dir string) {
	if gen == nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Name()), ".tpl") {
				return // шаблоны уже есть
			}
		}
	}
	if err := gen.CreateDefaultTemplates(); err != nil {
		log.Printf("[generator] не удалось создать стандартные шаблоны: %v", err)
	} else {
		log.Printf("[generator] созданы стандартные шаблоны в %s", dir)
	}
}

// cmdGenerate генерирует документ из шаблона для клиента.
// Использование: skolkovo generate <template_id> <client_id> [key=value ...]
// Без аргументов — создаёт стандартные шаблоны и печатает их список.
func cmdGenerate(cfg config.Config, args []string) error {
	ctx := context.Background()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	gen := newGenerator(cfg, st)
	if gen == nil {
		return fmt.Errorf("генерация документов требует backend=postgres")
	}

	// Без аргументов: создаём стандартные шаблоны и показываем доступные.
	if len(args) < 2 {
		if err := gen.CreateDefaultTemplates(); err != nil {
			return fmt.Errorf("создание стандартных шаблонов: %w", err)
		}
		names, err := gen.ListAvailableTemplates(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Стандартные шаблоны созданы в %s. Доступно %d:\n", cfg.GeneratorTemplateDir, len(names))
		for _, n := range names {
			fmt.Printf("  - %s\n", n)
		}
		fmt.Println("\nИспользование: skolkovo generate <template_id> <client_id> [key=value ...]")
		return nil
	}

	templateID, clientID := args[0], args[1]
	vars := map[string]string{}
	for _, kv := range args[2:] {
		if i := strings.IndexByte(kv, '='); i > 0 {
			vars[kv[:i]] = kv[i+1:]
		}
	}

	out, err := gen.RenderTemplate(ctx, templateID, clientID, vars)
	if err != nil {
		return err
	}
	fmt.Printf("Документ сгенерирован: %s\n", out)
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
	fmt.Fprintln(os.Stderr, "  catalog       — полное перечисление каталога по категориям (headless browser)")
	fmt.Fprintln(os.Stderr, "  crawl         — полный обход всего сайта документов (категории + sitemap + ссылки)")
	fmt.Fprintln(os.Stderr, "  fetch         — скачать тела файлов (обход WAF, chromedp)")
	fmt.Fprintln(os.Stderr, "  news          — синхронизировать новости из RSS")
	fmt.Fprintln(os.Stderr, "  events        — парсинг мероприятий")
	fmt.Fprintln(os.Stderr, "  contests      — парсинг конкурсов и грантов")
	fmt.Fprintln(os.Stderr, "  faq           — парсинг FAQ")
	fmt.Fprintln(os.Stderr, "  residents     — парсинг реестра резидентов Сколково")
	fmt.Fprintln(os.Stderr, "  telegram      — парсинг Telegram-каналов")
	fmt.Fprintln(os.Stderr, "  preferences   — льготы резидентов Сколково (налоговые, таможенные)")
	fmt.Fprintln(os.Stderr, "  regulations   — НПА по Сколково (244-ФЗ, постановления Правительства)")
	fmt.Fprintln(os.Stderr, "  sync          — полный цикл: документы + новости + льготы + НПА + индексация")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Аналитика:")
	fmt.Fprintln(os.Stderr, "  index         — проиндексировать активные документы в Qdrant (RAG)")
	fmt.Fprintln(os.Stderr, "  eligibility   — проверить компанию по ИНН: соответствует ли требованиям")
	fmt.Fprintln(os.Stderr, "  generate      — сгенерировать документ из шаблона (PDF/DOCX) для клиента")
	fmt.Fprintln(os.Stderr, "  audit         — отчёт о полноте охвата источников Сколково")
	fmt.Fprintln(os.Stderr, "  embed         — проверить доступность TEI (вычислить тестовый эмбеддинг)")
	fmt.Fprintln(os.Stderr, "  doctor        — smoke-тест инфраструктуры: PostgreSQL + Qdrant + TEI + миграции")
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
