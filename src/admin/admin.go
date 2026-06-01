// Package admin реализует веб-админку: классификация, валидация, версионирование
// и контроль статусов документов с триггером (пере)индексации в RAG.
package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/analytics"
	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/collector"
	"baza-skolkovo/src/common/extract"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/diff"
	"baza-skolkovo/src/fetcher"
	"baza-skolkovo/src/health"
	"baza-skolkovo/src/navindex"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/relevance"
	"baza-skolkovo/src/scheduler"
	"baza-skolkovo/src/scraper"
	"baza-skolkovo/src/sitepages"
)

// SitePageReader — что админке нужно от хранилища страниц публичного сайта.
type SitePageReader interface {
	ListRecent(ctx context.Context, limit int) ([]*sitepages.Page, error)
	GetWithText(ctx context.Context, id string) (*sitepages.Page, error)
	RelatedByTags(ctx context.Context, id string, tags []string, limit int) ([]sitepages.RelatedPage, error)
}

// SitePageRelated — семантический поиск похожих страниц (реализуется
// *sitepages.Searcher). Опционален: без него блок «похожие по смыслу» скрыт.
type SitePageRelated interface {
	Related(ctx context.Context, p *sitepages.Page, limit int) ([]sitepages.Hit, error)
}

// SitePageActions — ручные операции над страницей сайта (реализуется
// *sitepages.AdminService). Опционален: без него кнопка «Переаннотировать» и
// форма правки аннотации в просмотрщике скрыты.
type SitePageActions interface {
	ReannotateOne(ctx context.Context, id string) (string, error)
	SaveAnnotation(ctx context.Context, id string, a sitepages.Annotation) error
}

// VersionStore — что админке нужно от хранилища версий документов для
// автоматического сравнения редакций (страница /diff). Удовлетворяется
// *store.PostgresVersionStore; метод LatestVersions заодно делает его
// пригодным как relevance.VersionReader для повторного ИИ-анализа.
type VersionStore interface {
	DocumentsWithVersions(ctx context.Context, minVersions int) ([]store.DocVersionSummary, error)
	LatestVersions(ctx context.Context, documentID string, n int) ([]*model.DocVersion, error)
}

// Server — HTTP-админка.
type Server struct {
	store        store.Store
	linkStore    store.DocumentLinkStore
	changeStore  changes.Store
	healthStore  health.Store
	sitePages    SitePageReader
	sitePageSim  SitePageRelated // семантически похожие страницы (опционально)
	sitePageOps  SitePageActions // ручное переаннотирование/правка (опционально)
	versionStore VersionStore // история версий документов (для /diff); опционально
	prefStore    store.PreferenceStore
	npaStore     store.NPAStore
	rag          *rag.Service
	schedStore   *scheduler.Store
	reportStore  *scheduler.ReportStore
	aiStore      *aimodels.Store    // ИИ-модели и агенты (опционально, требует Postgres)
	navIndexer   *navindex.Indexer  // Переиндексация навигации по сайту (опционально)
	proxyManager *ProxyManager      // Управление прокси
	proxy6APIKey string             // API-ключ proxy6.net для автопоиска российских прокси
	cookieStore  *DochubCookieStore // сессионная кука dochub для HTTP-скачивания тел файлов
	addr         string
	user         string
	pass         string
	docsDir      string
	chromePath   string
	proxyURL     string
	fetchWait    time.Duration
	sourceURL    string
	authStore    *adminAuthStore
}

// envOrDefault возвращает значение переменной окружения или значение по умолчанию.
func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// New создаёт админку.
func New(addr, user, pass, docsDir, chromePath, proxyURL, sourceURL string,
	fetchWait time.Duration, st store.Store, ragSvc *rag.Service) *Server {
	authStore := newAdminAuthStore()
	// Администратор — обязателен, задаётся через ENV (ADMIN_USER/ADMIN_PASSWORD).
	if user != "" && pass != "" {
		authStore.AddUser(user, pass, "Администратор", RoleAdmin)
	}
	// Доп. учётки (редактор/наблюдатель) создаются ТОЛЬКО если их пароль задан
	// через ENV. Никаких дефолтных слабых паролей — иначе это дыра в безопасности.
	if p := os.Getenv("ADMIN_EDITOR_PASSWORD"); p != "" {
		authStore.AddUser(envOrDefault("ADMIN_EDITOR_USER", "editor"), p, "Редактор", RoleUser)
	}
	if p := os.Getenv("ADMIN_VIEWER_PASSWORD"); p != "" {
		authStore.AddUser(envOrDefault("ADMIN_VIEWER_USER", "viewer"), p, "Наблюдатель", RoleViewer)
	}

	// Инициализируем менеджер прокси и хранилище куки dochub.
	proxyManager := NewProxyManager(filepath.Join(docsDir, ".admin", "proxies.json"))
	cookieStore := NewDochubCookieStore(filepath.Join(docsDir, ".admin", "dochub_cookie.json"))

	return &Server{
		store: st, rag: ragSvc, addr: addr, user: user, pass: pass, docsDir: docsDir,
		chromePath: chromePath, proxyURL: proxyURL, fetchWait: fetchWait, sourceURL: sourceURL,
		authStore: authStore, proxyManager: proxyManager, cookieStore: cookieStore,
	}
}

// WithLinkStore устанавливает хранилище связей документов.
func (s *Server) WithLinkStore(ls store.DocumentLinkStore) *Server {
	s.linkStore = ls
	return s
}

// WithChangeStore устанавливает хранилище ленты изменений.
func (s *Server) WithChangeStore(cs changes.Store) *Server {
	s.changeStore = cs
	return s
}

// WithHealthStore устанавливает мониторинг свежести источников (для панели
// «когда обновлялось» на странице /changes).
func (s *Server) WithHealthStore(hs health.Store) *Server {
	s.healthStore = hs
	return s
}

// WithSitePageStore подключает хранилище страниц публичного сайта (для раздела
// «Страницы сайта»: список + просмотрщик).
func (s *Server) WithSitePageStore(r SitePageReader) *Server {
	s.sitePages = r
	return s
}

// WithSitePageSearcher подключает семантический поиск похожих страниц (блок
// «похожие по смыслу» в просмотрщике страницы сайта).
func (s *Server) WithSitePageSearcher(sim SitePageRelated) *Server {
	s.sitePageSim = sim
	return s
}

// WithSitePageActions подключает ручные операции над страницей: переаннотирование
// и сохранение правок аннотации куратором (кнопка и форма в просмотрщике).
func (s *Server) WithSitePageActions(ops SitePageActions) *Server {
	s.sitePageOps = ops
	return s
}

// WithVersionStore подключает хранилище версий документов — питает страницу
// автоматического ИИ-сравнения версий (/diff) и повторный анализ «Монитором».
func (s *Server) WithVersionStore(vs VersionStore) *Server {
	s.versionStore = vs
	return s
}

// WithPreferenceStore устанавливает хранилище льгот.
func (s *Server) WithPreferenceStore(ps store.PreferenceStore) *Server {
	s.prefStore = ps
	return s
}

// WithNPAStore устанавливает хранилище НПА.
func (s *Server) WithNPAStore(ns store.NPAStore) *Server {
	s.npaStore = ns
	return s
}

// WithNavIndexer устанавливает индексатор навигации по сайту (для кнопки
// «Переиндексировать навигацию» — пересборка коллекции skolkovo_navigation).
func (s *Server) WithNavIndexer(ix *navindex.Indexer) *Server {
	s.navIndexer = ix
	return s
}

// WithProxyManager устанавливает менеджер прокси.
func (s *Server) WithProxyManager(pm *ProxyManager) *Server {
	s.proxyManager = pm
	return s
}

// WithProxy6APIKey устанавливает API-ключ proxy6.net для автопоиска российских прокси.
func (s *Server) WithProxy6APIKey(key string) *Server {
	s.proxy6APIKey = key
	return s
}

// WithDochubCookieStore подключает хранилище сессионной куки dochub — питает
// HTTP-скачивание тел файлов по куке (страница «Прокси» → поле куки, кнопка
// «Скачать файлы»).
func (s *Server) WithDochubCookieStore(cs *DochubCookieStore) *Server {
	s.cookieStore = cs
	return s
}

// docView — строка таблицы для шаблона.
type docView struct {
	model.Document
	StatusStr      string
	FileSize       string // человекочитаемый размер файла
	FileAge        string // время загрузки ("2 часа назад", "3 дня назад")
	SourceLinkURL  string // URL для кнопки «источник»: /api/proxy-source?id=... или SourceURL
	SourceLinkText string // подпись ссылки: «источник» или «источник (локальный)»
	WebURL         string // прямая http(s)-ссылка на оригинал (для «Открыть на сайте»); пусто для file://
}

// stats — сводка по реестру.
type stats struct {
	Total, Active, Pending, Outdated, Archived, Rejected, Indexed int
}

type pageData struct {
	Docs           []docView
	Stats          stats
	Query          string
	Flash          string
	FlashKind      string
	FilterStatus   string
	FilterCategory string   // выбранная категория
	UpdatedFrom    string   // fetched_at от (YYYY-MM-DD)
	UpdatedTo      string   // fetched_at до
	PublishedFrom  string   // published_at (загрузка на sk.ru) от
	PublishedTo    string   // published_at до
	Categories     []string // список категорий для выпадающего фильтра
	BaseQS         string   // прочие фильтры без status — для ссылок-вкладок статусов
	Tab            string
	Settings       model.SchedulerSettings
	Reports        []model.CollectorReport
	Validation     *model.ValidationReport
	NextRunStr     string
	CurrentUser    *AdminSession
	PendingCount   int // количество документов «на проверке» для кнопки «Одобрить все»
}

// loginPageData — данные для страницы входа.
type loginPageData struct {
	Flash     string
	FlashKind string
}

// ListenAndServe запускает админку.
func (s *Server) ListenAndServe() error {
	if s.user == "" || s.pass == "" {
		log.Fatal("[admin] ОШИБКА: ADMIN_USER и ADMIN_PASSWORD должны быть заданы")
	}

	// Инициализация scheduler stores
	dataDir := filepath.Join(s.docsDir, "Метаданные")
	if s.schedStore == nil {
		var err error
		s.schedStore, err = scheduler.New(dataDir)
		if err != nil {
			return err
		}
	}
	if s.reportStore == nil {
		var err error
		s.reportStore, err = scheduler.NewReportStore(dataDir)
		if err != nil {
			return err
		}
	}

	mux := http.NewServeMux()

	// Публичные маршруты (без авторизации)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("GET /logout", s.handleLogout)

	// Защищённые маршруты (требуют сессии)
	mux.HandleFunc("GET /", s.requireAuth(s.handleIndex))
	mux.HandleFunc("GET /stats", s.requireAuth(s.handleStats))
	mux.HandleFunc("POST /documents/{id}/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("POST /documents/{id}/category", s.requireAuth(s.handleCategory))
	mux.HandleFunc("POST /documents/{id}/supersedes", s.requireAuth(s.handleSupersedes))
	mux.HandleFunc("POST /documents/{id}/upload", s.requireAuth(s.handleUpload))
	mux.HandleFunc("POST /documents/{id}/delete", s.requireAuth(s.handleDelete))
	mux.HandleFunc("GET /documents/{id}/view-original", s.requireAuth(s.handleViewOriginal))
	mux.HandleFunc("GET /documents/{id}/view-processed", s.requireAuth(s.handleViewProcessed))
	mux.HandleFunc("GET /documents/{id}/download", s.requireAuth(s.handleDownload))
	mux.HandleFunc("POST /documents/{id}/deindex", s.requireAuth(s.handleDeindex))
	mux.HandleFunc("GET /documents/{id}/source", s.requireAuth(s.handleDocSource))

	// Массовое одобрение документов
	mux.HandleFunc("POST /api/approve-all", s.requireAuthJSON(s.handleAPIApproveAll))

	// Старые API (обратная совместимость)
	mux.HandleFunc("POST /api/scrape", s.requireAuthJSON(s.handleAPIScrape))
	mux.HandleFunc("POST /api/fetch", s.requireAuthJSON(s.handleAPIFetch))
	mux.HandleFunc("POST /api/crawl", s.requireAuthJSON(s.handleAPICrawl))
	mux.HandleFunc("POST /api/index", s.requireAuthJSON(s.handleAPIIndex))
	mux.HandleFunc("POST /api/sync", s.requireAuthJSON(s.handleAPISync))
	mux.HandleFunc("POST /api/seed-local", s.requireAuthJSON(s.handleAPISeedLocal))
	mux.HandleFunc("POST /api/navindex", s.requireAuthJSON(s.handleAPINavIndex))

	// API для коллектора (полный цикл)
	mux.HandleFunc("POST /api/collect", s.requireAuthJSON(s.handleAPICollect))
	mux.HandleFunc("POST /api/validate", s.requireAuthJSON(s.handleAPIValidate))

	// API для планировщика
	mux.HandleFunc("GET /api/settings", s.requireAuthJSON(s.handleAPISettings))
	mux.HandleFunc("POST /api/settings", s.requireAuthJSON(s.handleAPISettingsUpdate))
	mux.HandleFunc("GET /api/reports", s.requireAuthJSON(s.handleAPIReports))

	// Diff — автоматическое сравнение версий документов силами ИИ-агента «Монитор».
	mux.HandleFunc("GET /diff", s.requireAuth(s.handleDiffPage))
	mux.HandleFunc("GET /diff/{id}/full", s.requireAuth(s.handleDiffFull))
	mux.HandleFunc("POST /api/diff/{id}/analyze", s.requireAuthJSON(s.handleAPIReanalyze))
	mux.HandleFunc("GET /api/diff/{id1}/{id2}", s.requireAuthJSON(s.handleAPIDiff))

	// Аналитика
	mux.HandleFunc("GET /analytics", s.requireAuth(s.handleAnalyticsPage))
	mux.HandleFunc("GET /api/analytics", s.requireAuthJSON(s.handleAPIAnalytics))
	mux.HandleFunc("GET /api/analytics/export", s.requireAuth(s.handleAnalyticsExport))

	// Граф связей документов
	mux.HandleFunc("GET /graph", s.requireAuth(s.handleGraphPage))
	mux.HandleFunc("GET /api/graph/{document_id}", s.requireAuthJSON(s.handleAPIGraphDoc))
	mux.HandleFunc("POST /api/graph", s.requireAuthJSON(s.handleAPIGraphCreateLink))
	mux.HandleFunc("DELETE /api/graph/{link_id}", s.requireAuthJSON(s.handleAPIGraphDeleteLink))

	// Лента изменений (история обновлений)
	mux.HandleFunc("GET /changes", s.requireAuth(s.handleChangesPage))
	mux.HandleFunc("GET /api/changes", s.requireAuthJSON(s.handleAPIChanges))

	// Страницы публичного сайта (список + просмотрщик)
	mux.HandleFunc("GET /sitepages", s.requireAuth(s.handleSitePagesPage))
	mux.HandleFunc("GET /sitepages/{id}", s.requireAuth(s.handleSitePageView))
	mux.HandleFunc("POST /sitepages/{id}/reannotate", s.requireAuth(s.handleSitePageReannotate))
	mux.HandleFunc("POST /sitepages/{id}/annotation", s.requireAuth(s.handleSitePageSaveAnnotation))
	mux.HandleFunc("GET /api/sitepages", s.requireAuthJSON(s.handleAPISitePages))

	// Льготы и НПА
	mux.HandleFunc("GET /regulations", s.requireAuth(s.handleRegulationsPage))
	mux.HandleFunc("POST /regulations/preferences", s.requireAuth(s.handleCreatePreference))
	mux.HandleFunc("POST /regulations/preferences/{id}/delete", s.requireAuth(s.handleDeletePreference))
	mux.HandleFunc("POST /regulations/npa", s.requireAuth(s.handleCreateNPA))
	mux.HandleFunc("POST /regulations/npa/{id}/delete", s.requireAuth(s.handleDeleteNPA))

	// Управление прокси
	mux.HandleFunc("GET /api/proxy/list", s.requireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		s.proxyManager.mu.Lock()
		defer s.proxyManager.mu.Unlock()
		json.NewEncoder(w).Encode(s.proxyManager.Proxies)
	}))
	mux.HandleFunc("POST /api/proxy/add", s.requireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Name, Type, URL string }
		json.NewDecoder(r.Body).Decode(&req)
		id := s.proxyManager.AddProxy(req.Name, req.Type, req.URL)
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	}))
	mux.HandleFunc("POST /api/proxy/activate", s.requireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ ID string }
		json.NewDecoder(r.Body).Decode(&req)
		s.proxyManager.ActivateProxy(req.ID)
		w.WriteHeader(200)
	}))
	mux.HandleFunc("POST /api/proxy/remove", s.requireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ ID string }
		json.NewDecoder(r.Body).Decode(&req)
		s.proxyManager.RemoveProxy(req.ID)
		w.WriteHeader(200)
	}))
	mux.HandleFunc("POST /api/proxy/test", s.requireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ ID string }
		json.NewDecoder(r.Body).Decode(&req)
		ok, ip, err := s.proxyManager.TestProxy(req.ID)
		res := map[string]interface{}{"ok": ok, "ip": ip}
		if err != nil {
			res["error"] = err.Error()
		}
		json.NewEncoder(w).Encode(res)
	}))
	mux.HandleFunc("POST /api/proxy/auto-switch", s.requireAuthJSON(func(w http.ResponseWriter, r *http.Request) {
		id := s.proxyManager.AutoSwitch()
		json.NewEncoder(w).Encode(map[string]string{"active_id": id})
	}))
	mux.HandleFunc("POST /api/proxy/find-russian", s.requireAuthJSON(s.handleAPIFindRussianProxy))
	mux.HandleFunc("POST /api/dochub-cookie", s.requireAuthJSON(s.handleAPISetDochubCookie))
	mux.HandleFunc("GET /proxy", s.requireAuth(s.handleProxyPage)) // UI страница

	// ИИ Конфигурация — модели и агенты
	mux.HandleFunc("GET /ai/models", s.requireAuth(s.handleAIModelsPage))
	mux.HandleFunc("GET /ai/models/new", s.requireAuth(s.handleAIModelNew))
	mux.HandleFunc("POST /ai/models/create", s.requireAuth(s.handleAIModelCreate))
	mux.HandleFunc("GET /ai/models/{id}/edit", s.requireAuth(s.handleAIModelEdit))
	mux.HandleFunc("POST /ai/models/{id}/update", s.requireAuth(s.handleAIModelUpdate))
	mux.HandleFunc("POST /api/ai/models/{id}/delete", s.requireAuthJSON(s.handleAIModelDelete))
	mux.HandleFunc("POST /api/ai/models/{id}/test", s.requireAuthJSON(s.handleAIModelTest))
	mux.HandleFunc("POST /api/ai/models/seed-qwen", s.requireAuthJSON(s.handleAISeedQwen))

	mux.HandleFunc("GET /ai/agents", s.requireAuth(s.handleAIAgentsPage))
	mux.HandleFunc("GET /ai/agents/new", s.requireAuth(s.handleAIAgentNew))
	mux.HandleFunc("POST /ai/agents/create", s.requireAuth(s.handleAIAgentCreate))
	mux.HandleFunc("GET /ai/agents/{id}/edit", s.requireAuth(s.handleAIAgentEdit))
	mux.HandleFunc("POST /ai/agents/{id}/update", s.requireAuth(s.handleAIAgentUpdate))
	mux.HandleFunc("POST /api/ai/agents/{id}/delete", s.requireAuthJSON(s.handleAIAgentDelete))
	mux.HandleFunc("POST /api/ai/agents/{id}/test", s.requireAuthJSON(s.handleAIAgentTest))

	log.Printf("[admin] админка слушает %s (вкладки: документы, сбор, планировщик, ИИ)", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// sessionCookieName — имя cookie сессии.
const sessionCookieName = "admin_session"

// getSessionID извлекает sessionID из cookie.
func (s *Server) getSessionID(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// requireAuth проверяет сессию и перенаправляет на /login при её отсутствии.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := s.getSessionID(r)
		if sid == "" {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
			return
		}
		sess, err := s.authStore.GetSession(sid)
		if err != nil {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
			return
		}
		// Передаём сессию через context
		ctx := context.WithValue(r.Context(), ctxAdminSessionKey{}, sess)
		next(w, r.WithContext(ctx))
	}
}

type ctxAdminSessionKey struct{}

// sessionFromContext извлекает сессию из context.
func sessionFromContext(r *http.Request) *AdminSession {
	s, _ := r.Context().Value(ctxAdminSessionKey{}).(*AdminSession)
	return s
}

// requireAuthJSON — аналог requireAuth для JSON API.
func (s *Server) requireAuthJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := s.getSessionID(r)
		if sid == "" {
			jsonErrorAdmin(w, http.StatusUnauthorized, "требуется авторизация")
			return
		}
		sess, err := s.authStore.GetSession(sid)
		if err != nil {
			jsonErrorAdmin(w, http.StatusUnauthorized, "сессия истекла")
			return
		}
		ctx := context.WithValue(r.Context(), ctxAdminSessionKey{}, sess)
		next(w, r.WithContext(ctx))
	}
}

func jsonErrorAdmin(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleLoginPage показывает страницу входа.
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// Если уже авторизован — редирект на главную
	sid := s.getSessionID(r)
	if sid != "" {
		if _, err := s.authStore.GetSession(sid); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	data := loginPageData{
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "login", data); err != nil {
		log.Println("[admin] шаблон login:", err)
	}
}

// handleLoginSubmit обрабатывает форму входа.
func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Redirect(w, r, "/login?msg=Введите+логин+и+пароль&kind=err", http.StatusSeeOther)
		return
	}

	user, ok := s.authStore.Authenticate(username, password)
	if !ok {
		http.Redirect(w, r, "/login?msg=Неверный+логин+или+пароль&kind=err", http.StatusSeeOther)
		return
	}

	// Создаём сессию
	sessionID := s.authStore.CreateSession(username, user.Role)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	next := r.URL.Query().Get("next")
	if next == "" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// handleLogout завершает сессию.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sid := s.getSessionID(r)
	if sid != "" {
		s.authStore.DeleteSession(sid)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) computeStats(ctx context.Context) (stats, error) {
	all, err := s.store.List(ctx, store.Filter{})
	if err != nil {
		return stats{}, err
	}
	var st stats
	st.Total = len(all)
	for _, d := range all {
		if d.Indexed {
			st.Indexed++
		}
		switch d.Status {
		case model.StatusActive:
			st.Active++
		case model.StatusPending:
			st.Pending++
		case model.StatusOutdated:
			st.Outdated++
		case model.StatusArchived:
			st.Archived++
		case model.StatusRejected:
			st.Rejected++
		}
	}
	return st, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tab := orDefault(r.URL.Query().Get("tab"), "documents")
	status := model.Status(r.URL.Query().Get("status"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	category := strings.TrimSpace(r.URL.Query().Get("category"))
	updatedFrom := r.URL.Query().Get("updated_from")
	updatedTo := r.URL.Query().Get("updated_to")
	publishedFrom := r.URL.Query().Get("published_from")
	publishedTo := r.URL.Query().Get("published_to")

	// parseDay разбирает YYYY-MM-DD в локальной зоне (end=true — конец дня).
	parseDay := func(v string, end bool) time.Time {
		if v == "" {
			return time.Time{}
		}
		layout, val := "2006-01-02", v
		if end {
			layout, val = "2006-01-02 15:04", v+" 23:59"
		}
		t, err := time.ParseInLocation(layout, val, time.Local)
		if err != nil {
			return time.Time{}
		}
		return t
	}
	updFrom, updTo := parseDay(updatedFrom, false), parseDay(updatedTo, true)
	pubFrom, pubTo := parseDay(publishedFrom, false), parseDay(publishedTo, true)

	var docs []docView
	categorySet := map[string]bool{}
	if tab == "documents" {
		allDocs, err := s.store.List(r.Context(), store.Filter{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		docs = make([]docView, 0, len(allDocs))
		qLower := strings.ToLower(query)
		for _, d := range allDocs {
			if d.Category != "" {
				categorySet[d.Category] = true
			}
			if status != "" && d.Status != status {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(d.Title), qLower) {
				continue
			}
			if category != "" && d.Category != category {
				continue
			}
			if !updFrom.IsZero() && d.FetchedAt.Before(updFrom) {
				continue
			}
			if !updTo.IsZero() && d.FetchedAt.After(updTo) {
				continue
			}
			if !pubFrom.IsZero() && (d.PublishedAt == nil || d.PublishedAt.Before(pubFrom)) {
				continue
			}
			if !pubTo.IsZero() && (d.PublishedAt == nil || d.PublishedAt.After(pubTo)) {
				continue
			}
			v := docView{Document: d, StatusStr: string(d.Status)}
			if d.LocalPath != "" {
				v.FileSize = formatFileSize(d.LocalPath)
				v.FileAge = humanTimeAgo(d.FetchedAt)
			}
			v.SourceLinkURL, v.SourceLinkText = makeSourceLink(d.ID, d.SourceURL, d.LocalPath)
			if strings.HasPrefix(d.SourceURL, "http://") || strings.HasPrefix(d.SourceURL, "https://") {
				v.WebURL = d.SourceURL
			}
			docs = append(docs, v)
		}
	}

	categories := make([]string, 0, len(categorySet))
	for c := range categorySet {
		categories = append(categories, c)
	}
	sort.Strings(categories)

	// base — прочие фильтры (без status) для ссылок-вкладок статусов.
	base := url.Values{}
	for k, v := range map[string]string{
		"q": query, "category": category,
		"updated_from": updatedFrom, "updated_to": updatedTo,
		"published_from": publishedFrom, "published_to": publishedTo,
	} {
		if v != "" {
			base.Set(k, v)
		}
	}

	st, err := s.computeStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	settings := s.schedStore.GetSettings()
	nextRunStr := "—"
	if settings.NextRun != nil {
		nextRunStr = settings.NextRun.Format("02.01.2006 15:04")
	}

	reports, _ := s.reportStore.GetReports(20)

	var valRep *model.ValidationReport
	if tab == "validator" {
		docsAll, _ := s.store.List(r.Context(), store.Filter{})
		valRep = &model.ValidationReport{TotalDocs: len(docsAll)}
		for _, d := range docsAll {
			valid := d.Title != "" && d.SourceURL != ""
			if d.Status == model.StatusActive && d.LocalPath == "" {
				valRep.MissingFiles++
			}
			if valid {
				valRep.ValidDocs++
			} else {
				valRep.InvalidDocs++
			}
		}
	}

	data := pageData{
		Docs:           docs,
		Stats:          st,
		Query:          query,
		Flash:          r.URL.Query().Get("msg"),
		FlashKind:      orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterStatus:   string(status),
		FilterCategory: category,
		UpdatedFrom:    updatedFrom,
		UpdatedTo:      updatedTo,
		PublishedFrom:  publishedFrom,
		PublishedTo:    publishedTo,
		Categories:     categories,
		BaseQS:         base.Encode(),
		Tab:            tab,
		Settings:       settings,
		Reports:        reports,
		Validation:     valRep,
		NextRunStr:     nextRunStr,
		CurrentUser:    sessionFromContext(r),
		PendingCount:   st.Pending,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Println("[admin] шаблон:", err)
	}
}

// handleStats отдаёт сводку в JSON (метрики актуальности базы).
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.computeStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// handleStatus меняет статус документа и синхронизирует индекс.
// При переходе в «действует» документа, который что-то заменяет,
// заменяемый документ автоматически переводится в «устарел» и убирается из индекса.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	newStatus := model.Status(r.FormValue("status"))

	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		redirect(w, r, "Документ не найден", "err")
		return
	}
	if err := s.store.SetStatus(r.Context(), id, newStatus); err != nil {
		redirect(w, r, "Ошибка смены статуса: "+err.Error(), "err")
		return
	}

	msg := "Статус обновлён: " + string(newStatus)
	if s.rag != nil {
		go s.syncIndex(id, newStatus)
		if newStatus == model.StatusActive {
			msg += " (запущена индексация)"
			if doc.Supersedes != "" {
				go s.outdate(doc.Supersedes)
				msg += "; документ " + doc.Supersedes + " помечен устаревшим"
			}
		}
	}
	redirect(w, r, msg, "ok")
}

// syncIndex выполняет (пере)индексацию или удаление из индекса в фоне.
func (s *Server) syncIndex(id string, status model.Status) {
	ctx := context.Background()
	if status == model.StatusActive {
		if n, err := s.rag.IndexDocument(ctx, id); err != nil {
			log.Printf("[admin] индексация %s: %v", id, err)
		} else {
			log.Printf("[admin] документ %s проиндексирован (%d фрагментов)", id, n)
		}
		return
	}
	if err := s.rag.RemoveDocument(ctx, id); err != nil {
		log.Printf("[admin] удаление из индекса %s: %v", id, err)
	}
}

// outdate переводит заменяемый документ в «устарел» и убирает его из индекса.
func (s *Server) outdate(id string) {
	ctx := context.Background()
	if err := s.store.SetStatus(ctx, id, model.StatusOutdated); err != nil {
		log.Printf("[admin] устаревание %s: %v", id, err)
		return
	}
	if s.rag != nil {
		if err := s.rag.RemoveDocument(ctx, id); err != nil {
			log.Printf("[admin] деиндексация %s: %v", id, err)
		}
	}
}

func (s *Server) handleCategory(w http.ResponseWriter, r *http.Request) {
	s.patch(w, r, func(d *model.Document) { d.Category = r.FormValue("category") }, "Категория обновлена")
}

func (s *Server) handleSupersedes(w http.ResponseWriter, r *http.Request) {
	s.patch(w, r, func(d *model.Document) { d.Supersedes = strings.TrimSpace(r.FormValue("supersedes")) }, "Связь версии обновлена")
}

// patch загружает документ, применяет изменение и сохраняет.
func (s *Server) patch(w http.ResponseWriter, r *http.Request, fn func(*model.Document), okMsg string) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		redirect(w, r, "Документ не найден", "err")
		return
	}
	fn(&doc)
	if err := s.store.Upsert(r.Context(), doc); err != nil {
		redirect(w, r, "Ошибка: "+err.Error(), "err")
		return
	}
	redirect(w, r, okMsg, "ok")
}

// handleUpload принимает файл вручную (обход WAF), сохраняет его, привязывает
// к документу и при статусе «действует» запускает индексацию.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		redirect(w, r, "Документ не найден", "err")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		redirect(w, r, "Файл слишком большой или ошибка формы", "err")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		redirect(w, r, "Файл не выбран", "err")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		redirect(w, r, "Ошибка чтения файла: "+err.Error(), "err")
		return
	}
	dir := filepath.Join(s.docsDir, statusDir(doc.Status), sanitize(doc.Category))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		redirect(w, r, "Ошибка папки: "+err.Error(), "err")
		return
	}
	localPath := filepath.Join(dir, sanitize(header.Filename))
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		redirect(w, r, "Ошибка записи: "+err.Error(), "err")
		return
	}
	sum := sha256.Sum256(data)
	doc.LocalPath = localPath
	doc.FileHash = hex.EncodeToString(sum[:])
	doc.Indexed = false
	if err := s.store.Upsert(r.Context(), doc); err != nil {
		redirect(w, r, "Ошибка сохранения: "+err.Error(), "err")
		return
	}

	msg := "Файл загружен"
	if s.rag != nil && doc.Status == model.StatusActive {
		go s.syncIndex(id, model.StatusActive)
		msg += " (запущена индексация)"
	}
	redirect(w, r, msg, "ok")
}

func statusDir(st model.Status) string {
	switch st {
	case model.StatusActive:
		return "Действующие"
	case model.StatusOutdated, model.StatusArchived:
		return "Архив"
	default:
		return "На_проверке"
	}
}

func sanitize(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Без_категории"
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	return strings.ReplaceAll(name, " ", "_")
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.rag != nil {
		go func() { _ = s.rag.RemoveDocument(context.Background(), id) }()
	}
	if err := s.store.Delete(r.Context(), id); err != nil {
		redirect(w, r, "Ошибка удаления: "+err.Error(), "err")
		return
	}
	redirect(w, r, "Документ удалён", "ok")
}

func redirect(w http.ResponseWriter, r *http.Request, msg, kind string) {
	http.Redirect(w, r, "/?msg="+url.QueryEscape(msg)+"&kind="+kind, http.StatusSeeOther)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// jsonResp отправляет JSON-ответ.
func jsonResp(w http.ResponseWriter, ok bool, msg, errStr string) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{"ok": ok}
	if msg != "" {
		resp["msg"] = msg
	}
	if errStr != "" {
		resp["error"] = errStr
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAPIScrape запускает парсинг RSS в фоне.
func (s *Server) handleAPIScrape(w http.ResponseWriter, r *http.Request) {
	if s.sourceURL == "" {
		jsonResp(w, false, "", "SOURCE_URL не задан — парсинг невозможен")
		return
	}
	jsonResp(w, true, "Парсинг RSS запущен в фоне. Обновите страницу через минуту.", "")
	go func() {
		ctx := context.Background()
		sc := &scraper.Scraper{
			RSSURL: scraper.DeriveRSS(s.sourceURL),
			Store:  s.store,
			Delay:  2 * time.Second,
		}
		sc.UseDynamicProxy(s.proxyManager.GetActiveURL)
		rep, err := sc.Run(ctx)
		if err != nil {
			log.Printf("[admin/api] парсинг RSS: %v", err)
			return
		}
		log.Printf("[admin/api] парсинг RSS завершён: добавлено %d, обновлено %d, ошибок %d",
			rep.Catalogued, rep.Updated, len(rep.Errors))
	}()
}

// handleAPICrawl запускает полный headless-обход сайта документов (категории +
// sitemap + внутренние ссылки) через активный прокси и пополняет каталог.
func (s *Server) handleAPICrawl(w http.ResponseWriter, r *http.Request) {
	log.Println("[admin/api] запуск полного обхода сайта (crawl)")
	activeProxyURL := s.proxyManager.GetActiveURL()

	go func() {
		ctx := context.Background()
		f, err := fetcher.New(s.chromePath, activeProxyURL, s.fetchWait, s.proxyManager.GetActiveURL)
		if err != nil {
			log.Printf("[admin/api] crawl: %v", err)
			return
		}
		// При WAF-бане автоматически переключаемся на рабочий прокси.
		f.OnWAFBlocked = func() string {
			s.proxyManager.AutoSwitch()
			return s.proxyManager.GetActiveURL()
		}

		cats := make([]fetcher.CategorySpec, 0, len(scraper.CategoryNames))
		for slug, name := range scraper.CategoryNames {
			cats = append(cats, fetcher.CategorySpec{Slug: slug, Name: name})
		}

		items, err := f.EnumerateSiteAuto(ctx, s.sourceURL, cats, 0)
		if err != nil {
			log.Printf("[admin/api] crawl: %v", err)
			return
		}
		added, merged := s.upsertCatalogItems(ctx, items)
		log.Printf("[admin/api] обход завершён: найдено %d, добавлено %d, дополнено %d", len(items), added, merged)
	}()

	jsonResp(w, true, "Полный обход сайта запущен в фоне. Используется активный прокси.", "")
}

// upsertCatalogItems сохраняет найденные при обходе документы в реестр
// (дедуп по нормализованной ссылке → ID). Возвращает (добавлено, дополнено).
func (s *Server) upsertCatalogItems(ctx context.Context, items []fetcher.CatalogItem) (int, int) {
	var added, merged int
	for _, it := range items {
		title := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(it.Title), "File:"))
		if title == "" {
			continue
		}
		id := scraper.DocID(it.Link)
		if existing, err := s.store.Get(ctx, id); err == nil {
			if existing.Category == "" && it.Category != "" {
				existing.Category = it.Category
				_ = s.store.Upsert(ctx, existing)
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
		if err := s.store.Upsert(ctx, doc); err != nil {
			log.Printf("[admin/api] upsert %s: %v", id, err)
			continue
		}
		added++
	}
	return added, merged
}

// handleAPISetDochubCookie сохраняет сессионную куку dochub (из браузера,
// прошедшего WAF) — с ней HTTP-скачивание тел файлов работает без браузера и
// без прокси. Принимает поле "cookie" (form или JSON).
func (s *Server) handleAPISetDochubCookie(w http.ResponseWriter, r *http.Request) {
	if s.cookieStore == nil {
		jsonResp(w, false, "", "хранилище куки не подключено")
		return
	}
	cookie := strings.TrimSpace(r.FormValue("cookie"))
	if cookie == "" {
		// Пробуем JSON-тело.
		var body struct {
			Cookie string `json:"cookie"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cookie = strings.TrimSpace(body.Cookie)
	}
	if cookie == "" {
		jsonResp(w, false, "", "пустая кука")
		return
	}
	if err := s.cookieStore.Set(cookie); err != nil {
		jsonResp(w, false, "", "сохранение: "+err.Error())
		return
	}
	log.Println("[admin/api] кука dochub обновлена")
	jsonResp(w, true, "Кука dochub сохранена. Теперь «Скачать файлы» качает тела по куке (без браузера и прокси).", "")
}

// handleAPIFetch запускает скачивание тел файлов. Основной путь — по сессионной
// куке dochub (обычный HTTP, без браузера/прокси): обходит WAF, т.к. кука выдана
// реальному браузеру, прошедшему проверку. Если куки нет — фоллбэк на старый
// headless+прокси путь (обычно блокируется WAF; оставлен для совместимости).
func (s *Server) handleAPIFetch(w http.ResponseWriter, r *http.Request) {
	cookie := ""
	if s.cookieStore != nil {
		cookie = s.cookieStore.Get()
	}

	if cookie != "" {
		log.Println("[admin/api] скачивание тел файлов по куке dochub (HTTP, без браузера)")
		go func() {
			ctx := context.Background()
			f, err := fetcher.New("", "", s.fetchWait, nil) // без Chrome и без прокси — кука работает с любого IP
			if err != nil {
				log.Printf("[admin/api] fetch(cookie): %v", err)
				return
			}
			f.Cookie = cookie
			// Возобновляемость: пропускаем уже скачанные файлы (есть на диске).
			f.SkipURL = func(u string) bool {
				if d, derr := s.store.Get(ctx, scraper.DocID(u)); derr == nil && d.LocalPath != "" {
					if _, serr := os.Stat(d.LocalPath); serr == nil {
						return true
					}
				}
				return false
			}
			cats := make([]fetcher.CategorySpec, 0, len(scraper.CategoryNames))
			for slug, name := range scraper.CategoryNames {
				cats = append(cats, fetcher.CategorySpec{Slug: slug, Name: name})
			}
			outDir := filepath.Join(s.docsDir, "На_проверке", "Загружено")
			byTitle := s.cookieTitleIndex(ctx) // индекс для реконсиляции, строим один раз
			added := 0
			docs, errs := f.CollectViaCookie(ctx, s.sourceURL, cats, outDir, 0,
				func(m string) { log.Printf("[admin/api] %s", m) },
				func(cd fetcher.CookieDoc) { // регистрируем сразу, по мере скачивания
					if s.registerOneCookieDoc(ctx, cd, byTitle) {
						added++
					}
				})
			log.Printf("[admin/api] скачивание по куке завершено: файлов %d, в реестр %d, ошибок %d (часть ошибок — «мёртвые» дубли /m/docs/, это норма)", len(docs), added, len(errs))
			// Фиксируем результат в мониторинге свежести, чтобы провал (например,
			// протухшая кука → 403 на всё) был виден в админке, а не только в логах.
			var runErr error
			if len(docs) == 0 && len(errs) > 0 {
				runErr = fmt.Errorf("скачано 0 файлов, ошибок %d — вероятно, кука dochub протухла, обновите её: %s", len(errs), errs[0])
			}
			if s.healthStore != nil {
				_ = s.healthStore.Record(ctx, "collect", len(docs), runErr)
			}
		}()
		jsonResp(w, true, "Скачивание тел файлов по куке запущено в фоне (без браузера и прокси). Обновите страницу через пару минут.", "")
		return
	}

	// Фоллбэк: headless + прокси (кука не задана). Обычно режется WAF dochub.
	log.Println("[admin/api] скачивание файлов: кука не задана — пробую headless+прокси (часто блокируется WAF)")
	activeProxyURL := s.proxyManager.GetActiveURL()
	go func() {
		ctx := context.Background()
		f, err := fetcher.New(s.chromePath, activeProxyURL, s.fetchWait, s.proxyManager.GetActiveURL)
		if err != nil {
			log.Printf("[admin/api] fetch: %v", err)
			return
		}
		f.OnWAFBlocked = func() string {
			s.proxyManager.AutoSwitch()
			return s.proxyManager.GetActiveURL()
		}
		done, errs := f.EnrichMissing(ctx, s.store, s.docsDir, 0, nil)
		log.Printf("[admin/api] скачивание завершено: скачано %d, ошибок %d", done, len(errs))
	}()
	jsonResp(w, true, "Кука dochub не задана — пробую headless+прокси (обычно блокируется WAF). Надёжный способ: задайте куку на странице «Прокси».", "")
}

// cookieTitleIndex один раз строит индекс «нормализованный заголовок → документ»
// для реконсиляции скачанных файлов с записями каталога (без файла).
func (s *Server) cookieTitleIndex(ctx context.Context) map[string]model.Document {
	existing, _ := s.store.List(ctx, store.Filter{})
	idx := make(map[string]model.Document, len(existing))
	for _, d := range existing {
		if k := cookieDocTitleKey(d.Title); k != "" {
			idx[k] = d
		}
	}
	return idx
}

// registerOneCookieDoc регистрирует ОДИН скачанный файл (инкрементально, по мере
// скачивания — чтобы прогресс не терялся при обрыве). Реконсиляция, чтобы не
// плодить дубли: 1) запись с этим download-URL уже есть — обновляем файл;
// 2) есть документ с тем же заголовком без файла — прикрепляем к нему; 3) иначе
// создаём новую запись (утратившие силу — «устарел», иначе «на_проверке»).
// Возвращает true, если запись добавлена/обновлена.
func (s *Server) registerOneCookieDoc(ctx context.Context, cd fetcher.CookieDoc, byTitle map[string]model.Document) bool {
	id := scraper.DocID(cd.URL)
	if doc, err := s.store.Get(ctx, id); err == nil {
		doc.LocalPath, doc.FileHash, doc.Indexed = cd.LocalPath, cd.Hash, false
		if doc.Category == "" {
			doc.Category = cd.Category
		}
		if s.store.Upsert(ctx, doc) == nil {
			s.maybeReindex(doc)
			return true
		}
		return false
	}
	if k := cookieDocTitleKey(cd.Title); k != "" {
		if ex, ok := byTitle[k]; ok && ex.LocalPath == "" {
			ex.LocalPath, ex.FileHash, ex.Indexed = cd.LocalPath, cd.Hash, false
			if ex.Category == "" {
				ex.Category = cd.Category
			}
			if s.store.Upsert(ctx, ex) == nil {
				s.maybeReindex(ex)
				return true
			}
			return false
		}
	}
	status := model.StatusPending
	if cd.Category == scraper.CategoryNames["unactual_documents"] ||
		strings.Contains(strings.ToUpper(cd.Title), "УТРАТИЛ") {
		status = model.StatusOutdated
	}
	doc := model.Document{
		ID: id, Title: cd.Title, SourceURL: cd.URL, Category: cd.Category,
		LocalPath: cd.LocalPath, FileHash: cd.Hash,
		Status: status, FetchedAt: time.Now(),
	}
	return s.store.Upsert(ctx, doc) == nil
}

// maybeReindex переиндексирует документ, если он действует и RAG подключён
// (чтобы текст только что прикреплённого файла попал в поиск).
func (s *Server) maybeReindex(doc model.Document) {
	if s.rag != nil && doc.Status == model.StatusActive {
		go s.syncIndex(doc.ID, model.StatusActive)
	}
}

// cookieDocTitleKey — нормализованный ключ заголовка для реконсиляции дублей.
func cookieDocTitleKey(t string) string {
	return strings.ToLower(strings.Join(strings.Fields(t), " "))
}

// handleAPIApproveAll переводит все «на_проверке» документы в «действует»
// и запускает их индексацию в RAG.
func (s *Server) handleAPIApproveAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	docs, err := s.store.List(ctx, store.Filter{Status: model.StatusPending})
	if err != nil {
		jsonResp(w, false, "", "Ошибка получения документов: "+err.Error())
		return
	}
	if len(docs) == 0 {
		jsonResp(w, true, "Нет документов «на проверке».", "")
		return
	}
	approved := 0
	for _, d := range docs {
		if err := s.store.SetStatus(ctx, d.ID, model.StatusActive); err != nil {
			log.Printf("[admin] approve-all: SetStatus %s: %v", d.ID, err)
			continue
		}
		approved++
	}
	// Запускаем индексацию одобренных документов в фоне.
	if s.rag != nil {
		go func() {
			bgCtx := context.Background()
			indexed := 0
			for _, d := range docs {
				n, err := s.rag.IndexDocument(bgCtx, d.ID)
				if err != nil {
					log.Printf("[admin] approve-all: indexing %s: %v", d.ID, err)
				} else {
					indexed += n
				}
			}
			log.Printf("[admin] approve-all: одобрено %d, проиндексировано %d фрагментов", approved, indexed)
		}()
	}
	jsonResp(w, true, fmt.Sprintf("Одобрено %d документов, индексация запущена в фоне.", approved), "")
}

// handleAPIFindRussianProxy ищет работающий российский прокси и добавляет его в ProxyManager.
func (s *Server) handleAPIFindRussianProxy(w http.ResponseWriter, r *http.Request) {
	if s.proxyManager == nil {
		jsonResp(w, false, "", "ProxyManager не подключён")
		return
	}
	finder := &fetcher.RussianProxyFinder{Proxy6APIKey: s.proxy6APIKey}
	ctx := r.Context()
	proxyURL, err := finder.Find(ctx)
	if err != nil {
		jsonResp(w, false, "", fmt.Sprintf("Не найден рабочий российский прокси: %v", err))
		return
	}
	id := s.proxyManager.AddProxy("auto-ru", "http", proxyURL)
	s.proxyManager.ActivateProxy(id)
	jsonResp(w, true, fmt.Sprintf("Найден и активирован российский прокси (ID %s).", id), "")
}

// handleAPIIndex запускает индексацию всех «действует» документов.
func (s *Server) handleAPIIndex(w http.ResponseWriter, r *http.Request) {
	if s.rag == nil {
		jsonResp(w, false, "", "RAG-сервис не подключён")
		return
	}
	log.Println("[admin/api] запуск индексации")
	go func() {
		ctx := context.Background()
		docs, err := s.store.List(ctx, store.Filter{Status: model.StatusActive})
		if err != nil {
			log.Printf("[admin/api] индексация: %v", err)
			return
		}
		for _, d := range docs {
			if d.Indexed {
				continue
			}
			n, err := s.rag.IndexDocument(ctx, d.ID)
			if err != nil {
				log.Printf("[admin/api] индексация %s: %v", d.ID, err)
			} else {
				log.Printf("[admin/api] %s проиндексирован (%d фрагментов)", d.ID, n)
			}
		}
		log.Println("[admin/api] индексация завершена")
	}()
	jsonResp(w, true, "Индексация запущена в фоне.", "")
}

// handleAPISync запускает полный цикл: RSS-парсинг + индексация.
func (s *Server) handleAPISync(w http.ResponseWriter, r *http.Request) {
	if s.sourceURL == "" && s.rag == nil {
		jsonResp(w, false, "", "SOURCE_URL и RAG-сервис не заданы")
		return
	}
	jsonResp(w, true, "Полный синк запущен в фоне (RSS → индексация). Обновите страницу через 2-3 минуты.", "")
	go func() {
		ctx := context.Background()
		// Шаг 1: RSS-парсинг
		if s.sourceURL != "" {
			sc := &scraper.Scraper{
				RSSURL: scraper.DeriveRSS(s.sourceURL),
				Store:  s.store,
				Delay:  2 * time.Second,
			}
			sc.UseDynamicProxy(s.proxyManager.GetActiveURL)
			rep, err := sc.Run(ctx)
			if err != nil {
				log.Printf("[admin/api] sync: парсинг RSS: %v", err)
			} else {
				log.Printf("[admin/api] sync: парсинг завершён: добавлено %d, обновлено %d",
					rep.Catalogued, rep.Updated)
			}
		}
		// Шаг 2: Индексация всех «действует»
		if s.rag != nil {
			docs, err := s.store.List(ctx, store.Filter{Status: model.StatusActive})
			if err != nil {
				log.Printf("[admin/api] sync: список для индексации: %v", err)
				return
			}
			var indexed int
			for _, doc := range docs {
				if _, err := s.rag.IndexDocument(ctx, doc.ID); err != nil {
					log.Printf("[admin/api] sync: индексация %s: %v", doc.ID, err)
					continue
				}
				indexed++
			}
			log.Printf("[admin/api] sync: проиндексировано %d документов", indexed)
		}
	}()
}

// handleAPISeedLocal регистрирует и индексирует все .md-файлы из DocsDir в RAG.
// Идемпотентно: уже зарегистрированные файлы (по LocalPath) не дублируются.
// handleAPINavIndex пересобирает навигационный индекс сайта (коллекция
// skolkovo_navigation) из src/navindex — питает MCP-инструмент get_navigation.
func (s *Server) handleAPINavIndex(w http.ResponseWriter, r *http.Request) {
	if s.navIndexer == nil {
		jsonResp(w, false, "", "Индексатор навигации не подключён (нужен Qdrant + TEI)")
		return
	}
	jsonResp(w, true, "Переиндексация навигации запущена в фоне.", "")
	go func() {
		n, err := s.navIndexer.Reindex(context.Background())
		if err != nil {
			log.Printf("[admin/navindex] ошибка переиндексации: %v", err)
			return
		}
		log.Printf("[admin/navindex] навигация переиндексирована: %d узлов", n)
	}()
}

func (s *Server) handleAPISeedLocal(w http.ResponseWriter, r *http.Request) {
	if s.rag == nil {
		jsonResp(w, false, "", "RAG-сервис не подключён")
		return
	}
	if s.docsDir == "" {
		jsonResp(w, false, "", "DocsDir не задан")
		return
	}
	jsonResp(w, true, "Индексация локальных документов запущена в фоне.", "")
	go func() {
		ctx := context.Background()

		// Собираем уже известные LocalPath чтобы не дублировать.
		existing, _ := s.store.List(ctx, store.Filter{})
		knownPaths := make(map[string]bool, len(existing))
		for _, d := range existing {
			if d.LocalPath != "" {
				knownPaths[d.LocalPath] = true
			}
		}

		var added, indexed, skipped int
		err := filepath.WalkDir(s.docsDir, func(path string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() {
				return err
			}
			if strings.ToLower(filepath.Ext(path)) != ".md" {
				return nil
			}
			absPath := filepath.ToSlash(path)
			if knownPaths[absPath] {
				skipped++
				return nil
			}

			// Генерируем детерминированный ID из пути.
			h := sha256.Sum256([]byte(absPath))
			docID := "local-" + hex.EncodeToString(h[:8])

			// Выводим заголовок из первой строки файла.
			title := filepath.Base(path)
			if data, rerr := os.ReadFile(path); rerr == nil {
				for _, line := range strings.SplitN(string(data), "\n", 10) {
					line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
					if line != "" && !strings.HasPrefix(line, "---") {
						title = line
						break
					}
				}
			}

			// Категорию навешиваем только файлам навигационной карты сайта;
			// остальной локальный markdown оставляем без категории, чтобы его
			// классифицировал классификатор при индексации (а не мислейблил).
			category := ""
			if strings.Contains(absPath, "RAG_Структура_сайта") {
				category = "RAG Структура сайта"
			}

			now := time.Now()
			doc := model.Document{
				ID:        docID,
				Title:     title,
				LocalPath: absPath,
				SourceURL: "file://" + absPath,
				Status:    model.StatusActive,
				Category:  category,
				FetchedAt: now,
			}
			if uerr := s.store.Upsert(ctx, doc); uerr != nil {
				log.Printf("[admin/seed-local] upsert %s: %v", absPath, uerr)
				return nil
			}
			added++
			knownPaths[absPath] = true

			n, ierr := s.rag.IndexDocument(ctx, docID)
			if ierr != nil {
				log.Printf("[admin/seed-local] index %s: %v", absPath, ierr)
			} else {
				log.Printf("[admin/seed-local] %s проиндексирован (%d фрагментов)", title, n)
				indexed++
			}
			return nil
		})
		if err != nil {
			log.Printf("[admin/seed-local] walkdir: %v", err)
		}
		log.Printf("[admin/seed-local] завершено: добавлено=%d проиндексировано=%d пропущено=%d",
			added, indexed, skipped)
	}()
}

// handleViewOriginal извлекает текст из исходного файла и показывает его.
// Для PDF — показывает через iframe (встроенный просмотрщик браузера).
func (s *Server) handleViewOriginal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Документ не найден", http.StatusNotFound)
		return
	}
	if doc.LocalPath == "" {
		http.Error(w, "У документа нет локального файла", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(doc.LocalPath); os.IsNotExist(err) {
		http.Error(w, "Файл не найден на диске: "+doc.LocalPath, http.StatusNotFound)
		return
	}

	ext := strings.ToLower(filepath.Ext(doc.LocalPath))

	// PDF — показываем через iframe
	if ext == ".pdf" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PDF — %s</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #181b2b;
  --surface: #23273a;
  --surface-hover: #2a2f45;
  --border: #2e3348;
  --text: #e8eaed;
  --text-muted: #9aa0b0;
  --primary: #0073ea;
  --primary-hover: #005bb5;
  --danger: #e5484d;
  --danger-hover: #cd3a40;
  --radius: 8px;
  --shadow: 0 1px 3px rgba(0,0,0,.4), 0 1px 2px rgba(0,0,0,.3);
}
@media (prefers-color-scheme: light) {
  :root:not([data-theme="dark"]) {
    --bg: #f6f8fa;
    --surface: #ffffff;
    --surface-hover: #f0f2f5;
    --border: #d8dde6;
    --text: #1a1d23;
    --text-muted: #6b7280;
    --shadow: 0 1px 3px rgba(0,0,0,.08), 0 1px 2px rgba(0,0,0,.06);
  }
}
* { box-sizing: border-box; }
body { font-family: 'Figtree', -apple-system, BlinkMacSystemFont, sans-serif; background: var(--bg); color: var(--text); margin: 0; padding: 24px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid var(--border); flex-wrap: wrap; gap: 12px; }
.header h1 { font-size: 18px; margin: 0; font-weight: 600; display: flex; align-items: center; gap: 8px; min-width: 0; word-break: break-word; overflow-wrap: anywhere; }
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; background: var(--primary); color: #fff; border: none; border-radius: var(--radius); cursor: pointer; text-decoration: none; font-size: 13px; font-weight: 500; font-family: inherit; transition: background .15s; }
.btn:hover { background: var(--primary-hover); }
.btn-danger { background: var(--danger); }
.btn-danger:hover { background: var(--danger-hover); }
[data-tooltip] { position: relative; }
[data-tooltip]::after { content: attr(data-tooltip); position: absolute; bottom: calc(100%% + 6px); left: 50%%; transform: translateX(-50%%); background: #111327; color: #e8eaed; padding: 4px 8px; border-radius: 4px; font-size: 11px; white-space: nowrap; opacity: 0; pointer-events: none; transition: opacity .15s; z-index: 10; }
[data-tooltip]:hover::after { opacity: 1; }
iframe { width: 100%%; height: calc(100vh - 120px); border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); }
.meta { font-size: 12px; color: var(--text-muted); margin-top: 12px; word-break: break-word; overflow-wrap: break-word; }
@media (max-width: 768px) {
  body { padding: 16px; }
  .header { flex-direction: column; align-items: flex-start; }
  .header > div { display: flex; gap: 8px; flex-wrap: wrap; }
}
@media (max-width: 480px) {
  body { padding: 12px; }
  .header h1 { font-size: 15px; }
  .btn { padding: 6px 12px; font-size: 12px; }
  iframe { height: calc(100vh - 140px); }
}
</style>
</head>
<body>
<div class="header">
  <h1><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg> PDF: %s</h1>
  <div>
    <a href="/documents/%s/download" class="btn" data-tooltip="Скачать файл"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg> Скачать</a>
    <a href="javascript:window.close()" class="btn btn-danger" data-tooltip="Закрыть"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg> Закрыть</a>
  </div>
</div>
<iframe src="/documents/%s/download"></iframe>
<div class="meta">Файл: %s | Хеш: %s | Размер: %s</div>
</body></html>`, doc.Title, doc.Title, doc.ID, doc.ID, doc.LocalPath, doc.FileHash, formatFileSize(doc.LocalPath))
		return
	}

	// Остальные форматы — извлекаем текст
	var text string
	if extract.IsSupported(doc.LocalPath) {
		text, err = extract.Text(doc.LocalPath)
		if err != nil {
			http.Error(w, "Ошибка извлечения текста: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		data, err := os.ReadFile(doc.LocalPath)
		if err != nil {
			http.Error(w, "Ошибка чтения файла: "+err.Error(), http.StatusInternalServerError)
			return
		}
		text = string(data)
	}

	// Ограничиваем вывод для производительности (первые 50000 символов)
	const maxLen = 50000
	truncated := false
	if len(text) > maxLen {
		text = text[:maxLen]
		truncated = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Исходный документ — %s</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #181b2b;
  --surface: #23273a;
  --surface-hover: #2a2f45;
  --border: #2e3348;
  --text: #e8eaed;
  --text-muted: #9aa0b0;
  --primary: #0073ea;
  --primary-hover: #005bb5;
  --danger: #e5484d;
  --danger-hover: #cd3a40;
  --warning-bg: #3d2e00;
  --warning-text: #fbbf24;
  --radius: 8px;
  --shadow: 0 1px 3px rgba(0,0,0,.4), 0 1px 2px rgba(0,0,0,.3);
}
@media (prefers-color-scheme: light) {
  :root:not([data-theme="dark"]) {
    --bg: #f6f8fa;
    --surface: #ffffff;
    --surface-hover: #f0f2f5;
    --border: #d8dde6;
    --text: #1a1d23;
    --text-muted: #6b7280;
    --warning-bg: #fef3c7;
    --warning-text: #92400e;
    --shadow: 0 1px 3px rgba(0,0,0,.08), 0 1px 2px rgba(0,0,0,.06);
  }
}
* { box-sizing: border-box; }
body { font-family: 'Figtree', -apple-system, BlinkMacSystemFont, sans-serif; background: var(--bg); color: var(--text); margin: 0; padding: 24px; line-height: 1.6; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid var(--border); flex-wrap: wrap; gap: 12px; }
.header h1 { font-size: 18px; margin: 0; font-weight: 600; display: flex; align-items: center; gap: 8px; min-width: 0; word-break: break-word; overflow-wrap: anywhere; }
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; background: var(--primary); color: #fff; border: none; border-radius: var(--radius); cursor: pointer; text-decoration: none; font-size: 13px; font-weight: 500; font-family: inherit; transition: background .15s; }
.btn:hover { background: var(--primary-hover); }
.btn-danger { background: var(--danger); }
.btn-danger:hover { background: var(--danger-hover); }
[data-tooltip] { position: relative; }
[data-tooltip]::after { content: attr(data-tooltip); position: absolute; bottom: calc(100%% + 6px); left: 50%%; transform: translateX(-50%%); background: #111327; color: #e8eaed; padding: 4px 8px; border-radius: 4px; font-size: 11px; white-space: nowrap; opacity: 0; pointer-events: none; transition: opacity .15s; z-index: 10; }
[data-tooltip]:hover::after { opacity: 1; }
.content { background: var(--surface); padding: 20px; border-radius: var(--radius); box-shadow: var(--shadow); white-space: pre-wrap; word-break: break-word; overflow-wrap: break-word; font-size: 14px; max-height: 80vh; overflow-y: auto; color: var(--text); }
.meta { font-size: 12px; color: var(--text-muted); margin-top: 12px; word-break: break-word; overflow-wrap: break-word; }
.truncated { background: var(--warning-bg); padding: 8px 12px; border-radius: var(--radius); margin-bottom: 12px; font-size: 13px; color: var(--warning-text); display: flex; align-items: center; gap: 8px; }
.truncated svg { flex-shrink: 0; }
@media (max-width: 768px) {
  body { padding: 16px; }
  .header { flex-direction: column; align-items: flex-start; }
  .header > div { display: flex; gap: 8px; flex-wrap: wrap; }
}
@media (max-width: 480px) {
  body { padding: 12px; }
  .header h1 { font-size: 15px; }
  .btn { padding: 6px 12px; font-size: 12px; }
}
</style>
</head>
<body>
<div class="header">
  <h1><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg> Исходный документ: %s</h1>
  <div>
    <a href="/documents/%s/download" class="btn" data-tooltip="Скачать файл"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg> Скачать</a>
    <a href="javascript:window.close()" class="btn btn-danger" data-tooltip="Закрыть"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg> Закрыть</a>
  </div>
</div>
`, doc.Title, doc.Title, doc.ID)

	if truncated {
		fmt.Fprintf(w, `<div class="truncated"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg> Показаны первые %d символов из %d. <a href="/documents/%s/download">Скачайте файл</a> для просмотра целиком.</div>`, maxLen, len(text), doc.ID)
	}

	fmt.Fprintf(w, `<div class="content">%s</div>`, html.EscapeString(text))
	fmt.Fprintf(w, `<div class="meta">Файл: %s | Размер: %s | Хеш: %s</div>`, doc.LocalPath, formatFileSize(doc.LocalPath), doc.FileHash)
	fmt.Fprint(w, `</body></html>`)
}

func formatFileSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "н/д"
	}
	bytes := info.Size()
	if bytes < 1024 {
		return fmt.Sprintf("%d Б", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f КБ", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f МБ", float64(bytes)/(1024*1024))
}

func humanTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	if diff < time.Minute {
		return "только что"
	}
	if diff < time.Hour {
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%d мин. назад", minutes)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%d ч. назад", hours)
	}
	days := int(diff.Hours() / 24)
	if days == 1 {
		return "вчера"
	}
	if days < 7 {
		return fmt.Sprintf("%d дн. назад", days)
	}
	return t.Format("02.01.2006")
}

// handleViewProcessed показывает обработанные чанки документа из Qdrant.
func (s *Server) handleViewProcessed(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Документ не найден", http.StatusNotFound)
		return
	}
	if !doc.Indexed {
		http.Error(w, "Документ ещё не проиндексирован", http.StatusBadRequest)
		return
	}
	if s.rag == nil || s.rag.Qdr == nil {
		http.Error(w, "RAG-сервис не подключён", http.StatusInternalServerError)
		return
	}

	// Получаем чанки из Qdrant
	ctx := r.Context()
	chunks, err := s.getDocumentChunks(ctx, id)
	if err != nil {
		http.Error(w, "Ошибка получения чанков: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(chunks) == 0 {
		http.Error(w, "Чанки не найдены в индексе", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Обработанный документ — %s</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #181b2b;
  --surface: #23273a;
  --surface-hover: #2a2f45;
  --border: #2e3348;
  --text: #e8eaed;
  --text-muted: #9aa0b0;
  --primary: #0073ea;
  --primary-hover: #005bb5;
  --danger: #e5484d;
  --danger-hover: #cd3a40;
  --radius: 8px;
  --shadow: 0 1px 3px rgba(0,0,0,.4), 0 1px 2px rgba(0,0,0,.3);
}
@media (prefers-color-scheme: light) {
  :root:not([data-theme="dark"]) {
    --bg: #f6f8fa;
    --surface: #ffffff;
    --surface-hover: #f0f2f5;
    --border: #d8dde6;
    --text: #1a1d23;
    --text-muted: #6b7280;
    --shadow: 0 1px 3px rgba(0,0,0,.08), 0 1px 2px rgba(0,0,0,.06);
  }
}
* { box-sizing: border-box; }
body { font-family: 'Figtree', -apple-system, BlinkMacSystemFont, sans-serif; background: var(--bg); color: var(--text); margin: 0; padding: 24px; line-height: 1.6; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 1px solid var(--border); flex-wrap: wrap; gap: 12px; }
.header h1 { font-size: 18px; margin: 0; font-weight: 600; display: flex; align-items: center; gap: 8px; min-width: 0; word-break: break-word; overflow-wrap: anywhere; }
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; background: var(--primary); color: #fff; border: none; border-radius: var(--radius); cursor: pointer; text-decoration: none; font-size: 13px; font-weight: 500; font-family: inherit; transition: background .15s; }
.btn:hover { background: var(--primary-hover); }
.btn-danger { background: var(--danger); }
.btn-danger:hover { background: var(--danger-hover); }
[data-tooltip] { position: relative; }
[data-tooltip]::after { content: attr(data-tooltip); position: absolute; bottom: calc(100%% + 6px); left: 50%%; transform: translateX(-50%%); background: #111327; color: #e8eaed; padding: 4px 8px; border-radius: 4px; font-size: 11px; white-space: nowrap; opacity: 0; pointer-events: none; transition: opacity .15s; z-index: 10; }
[data-tooltip]:hover::after { opacity: 1; }
.chunk { background: var(--surface); padding: 16px; border-radius: var(--radius); box-shadow: var(--shadow); margin-bottom: 12px; }
.chunk-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; padding-bottom: 8px; border-bottom: 1px solid var(--border); flex-wrap: wrap; gap: 4px; }
.chunk-num { font-weight: 600; color: var(--primary); font-size: 13px; }
.chunk-len { font-size: 11px; color: var(--text-muted); }
.chunk-text { white-space: pre-wrap; word-break: break-word; overflow-wrap: break-word; font-size: 14px; color: var(--text); }
.meta { font-size: 12px; color: var(--text-muted); margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--border); word-break: break-word; overflow-wrap: break-word; }
.stats { display: flex; gap: 16px; margin-bottom: 16px; flex-wrap: wrap; }
.stat { background: var(--surface); padding: 12px 20px; border-radius: var(--radius); text-align: center; box-shadow: var(--shadow); min-width: 100px; }
.stat .n { font-size: 24px; font-weight: 700; color: var(--primary); }
.stat .l { font-size: 11px; color: var(--text-muted); text-transform: uppercase; margin-top: 4px; letter-spacing: .5px; }
@media (max-width: 768px) {
  body { padding: 16px; }
  .header { flex-direction: column; align-items: flex-start; }
  .stats { flex-direction: column; }
}
@media (max-width: 480px) {
  body { padding: 12px; }
  .header h1 { font-size: 15px; }
  .btn { padding: 6px 12px; font-size: 12px; }
  .stat { padding: 10px 16px; }
}
</style>
</head>
<body>
<div class="header">
  <h1><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a7 7 0 0 1 7 7c0 3-3 5.4-5 7.2L12 18l-2-1.8C8 14.4 5 12 5 9a7 7 0 0 1 7-7z"/><circle cx="12" cy="9" r="2.5"/><line x1="12" y1="18" x2="12" y2="22"/><line x1="8" y1="22" x2="16" y2="22"/></svg> Обработанный документ: %s</h1>
  <a href="javascript:window.close()" class="btn btn-danger" data-tooltip="Закрыть"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg> Закрыть</a>
</div>

<div class="stats">
  <div class="stat" data-tooltip="Количество фрагментов (чанков), на которые разбит документ для индексации"><div class="n">%d</div><div class="l">Чанков</div></div>
  <div class="stat" data-tooltip="Суммарная длина текста всех чанков в символах"><div class="n">%d</div><div class="l">Всего символов</div></div>
</div>
`, doc.Title, doc.Title, len(chunks), s.totalChars(chunks))

	for i, chunk := range chunks {
		fmt.Fprintf(w, `<div class="chunk">
<div class="chunk-header">
  <span class="chunk-num"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:-1px;margin-right:4px"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="9" y1="3" x2="9" y2="21"/></svg> Чанк #%d</span>
  <span class="chunk-len">%d символов</span>
</div>
<div class="chunk-text">%s</div>
</div>`, i+1, len(chunk.Text), html.EscapeString(chunk.Text))
	}

	fmt.Fprintf(w, `<div class="meta">Документ: %s | Статус: %s | Индексирован: %v</div>`, doc.Title, doc.Status, doc.Indexed)
	fmt.Fprint(w, `</body></html>`)
}

// getDocumentChunks получает все чанки документа из Qdrant через Scroll API.
func (s *Server) getDocumentChunks(ctx context.Context, docID string) ([]model.Chunk, error) {
	if s.rag == nil || s.rag.Qdr == nil {
		return nil, fmt.Errorf("Qdrant не подключён")
	}

	points, err := s.rag.Qdr.ScrollByDocument(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("scroll в Qdrant: %w", err)
	}

	var chunks []model.Chunk
	for _, p := range points {
		chunk := model.Chunk{
			ID:         p.ID,
			DocumentID: asString(p.Payload["document_id"]),
			Index:      asInt(p.Payload["chunk_index"]),
			Text:       asString(p.Payload["text"]),
		}
		chunks = append(chunks, chunk)
	}

	// Сортируем по chunk_index
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Index < chunks[j].Index
	})

	return chunks, nil
}

// totalChars считает общее количество символов в чанках.
func (s *Server) totalChars(chunks []model.Chunk) int {
	total := 0
	for _, c := range chunks {
		total += len(c.Text)
	}
	return total
}

// handleDownload отдаёт файл документа на скачивание.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Документ не найден", http.StatusNotFound)
		return
	}
	if doc.LocalPath == "" {
		http.Error(w, "У документа нет локального файла", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(doc.LocalPath); os.IsNotExist(err) {
		http.Error(w, "Файл не найден на диске", http.StatusNotFound)
		return
	}

	// Определяем MIME-тип
	ext := strings.ToLower(filepath.Ext(doc.LocalPath))
	mimeType := "application/octet-stream"
	switch ext {
	case ".pdf":
		mimeType = "application/pdf"
	case ".docx":
		mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".doc":
		mimeType = "application/msword"
	case ".txt":
		mimeType = "text/plain"
	case ".md":
		mimeType = "text/markdown"
	case ".html", ".htm":
		mimeType = "text/html"
	}

	// Отдаём файл
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(doc.LocalPath)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize(doc.LocalPath)))
	http.ServeFile(w, r, doc.LocalPath)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// asString преобразует интерфейс в строку.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// asInt преобразует интерфейс в int.
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// handleDeindex удаляет документ из индекса Qdrant (без изменения статуса).
func (s *Server) handleDeindex(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		redirect(w, r, "Документ не найден", "err")
		return
	}
	if !doc.Indexed {
		redirect(w, r, "Документ не проиндексирован", "err")
		return
	}
	if s.rag == nil {
		redirect(w, r, "RAG-сервис не подключён", "err")
		return
	}

	if err := s.rag.RemoveDocument(r.Context(), id); err != nil {
		redirect(w, r, "Ошибка удаления из индекса: "+err.Error(), "err")
		return
	}
	redirect(w, r, "Документ удалён из индекса", "ok")
}

// --- API: Полный цикл сбора данных ---

func (s *Server) handleAPICollect(w http.ResponseWriter, r *http.Request) {
	indexFn := func(ctx context.Context, id string) error {
		if s.rag == nil {
			return nil
		}
		if err := s.rag.Init(ctx); err != nil {
			return err
		}
		_, err := s.rag.IndexDocument(ctx, id)
		return err
	}

	c := collector.New(s.chromePath, s.proxyURL, s.sourceURL, s.docsDir, s.fetchWait,
		s.store, indexFn, s.schedStore.GetSettings().AutoApprove)

	rep, err := c.FullCycle(r.Context())
	if err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}

	_ = s.reportStore.AddReport(*rep)
	_ = s.schedStore.MarkRun()

	jsonResp(w, true, fmt.Sprintf("Сбор завершён: новых %d, обновлено %d, файлов %d, индексировано %d",
		rep.DocumentsNew, rep.DocumentsUpd, rep.FilesDownloaded, rep.Indexed), "")
}

func (s *Server) handleAPIValidate(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.List(r.Context(), store.Filter{})
	if err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}
	rep := &model.ValidationReport{TotalDocs: len(docs)}
	for _, d := range docs {
		valid := d.Title != "" && d.SourceURL != ""
		if d.Status == model.StatusActive && d.LocalPath == "" {
			rep.MissingFiles++
		}
		if valid {
			rep.ValidDocs++
		} else {
			rep.InvalidDocs++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}

// --- API: Настройки планировщика ---

func (s *Server) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.schedStore.GetSettings())
}

func (s *Server) handleAPISettingsUpdate(w http.ResponseWriter, r *http.Request) {
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		jsonResp(w, false, "", "Ошибка разбора JSON")
		return
	}
	if err := s.schedStore.UpdateSettings(updates); err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}
	jsonResp(w, true, "Настройки сохранены", "")
}

func (s *Server) handleAPIReports(w http.ResponseWriter, r *http.Request) {
	reports, err := s.reportStore.GetReports(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reports)
}

// ===========================================================================
// Diff — сравнение версий документов
// ===========================================================================

// diffDocCard — карточка документа с историей версий: автоматически
// сопоставленные две последние редакции + вердикт ИИ-агента «Монитор».
type diffDocCard struct {
	DocID        string
	Title        string
	Category     string
	VersionCount int
	NewNo        int    // номер новой (последней) версии
	OldNo        int    // номер предыдущей версии
	NewAt        string // дата новой версии
	OldAt        string // дата предыдущей версии
	Added        int    // добавлено строк (по дифф-снимкам)
	Removed      int    // удалено строк
	Severity     string // info|warning|critical|"" — важность по ИИ-вердикту
	SeverityText string // человекочитаемая важность
	Summary      string // «что изменилось по сути» (ИИ или эвристика)
	Stages       []string
	Analyzed     bool   // есть зафиксированный ИИ-вердикт из ленты изменений
	AnalyzedAt   string // когда зафиксирован вердикт
}

type diffPageData struct {
	Cards       []diffDocCard
	Error       string
	HasAI       bool // настроен ли включённый агент «Монитор»
	Unavailable bool // хранилище версий не подключено (не Postgres)
}

// handleDiffPage показывает автоматическую ленту сравнения версий: документы,
// у которых накоплено ≥2 редакций, с готовым вердиктом ИИ-агента «Монитор»
// (гибрид: показываем посчитанное в пайплайне, плюс кнопка «Переанализировать»).
func (s *Server) handleDiffPage(w http.ResponseWriter, r *http.Request) {
	data := diffPageData{}
	if s.versionStore == nil {
		data.Unavailable = true
		s.renderDiffPage(w, data)
		return
	}

	summaries, err := s.versionStore.DocumentsWithVersions(r.Context(), 2)
	if err != nil {
		data.Error = "Не удалось получить историю версий: " + err.Error()
		s.renderDiffPage(w, data)
		return
	}

	// Гибрид: подмешиваем зафиксированные ИИ-вердикты из ленты изменений
	// (их уже посчитал «Монитор» в пайплайне) — без новых вызовов LLM.
	verdicts := s.documentVerdicts(r.Context())
	data.HasAI = s.monitorAgentConfigured(r.Context())

	for _, sm := range summaries {
		card := diffDocCard{
			DocID:        sm.DocumentID,
			VersionCount: sm.VersionCount,
			NewNo:        sm.LatestNo,
			NewAt:        sm.LatestAt.Format("02.01.2006 15:04"),
			Title:        sm.DocumentID,
		}
		if doc, err := s.store.Get(r.Context(), sm.DocumentID); err == nil && doc.Title != "" {
			card.Title = doc.Title
			card.Category = doc.Category
		}
		// Статистика правок по двум последним снимкам (дёшево, без LLM).
		if vers, err := s.versionStore.LatestVersions(r.Context(), sm.DocumentID, 2); err == nil && len(vers) >= 2 {
			newV, oldV := vers[0], vers[1]
			card.OldNo = oldV.VersionNo
			card.OldAt = oldV.CreatedAt.Format("02.01.2006 15:04")
			d := diff.CompareDocuments(oldV.ExtractedText, newV.ExtractedText)
			card.Added = d.Summary.TotalAdded
			card.Removed = d.Summary.TotalRemoved
		}
		if v, ok := verdicts[sm.DocumentID]; ok {
			card.Severity = string(v.Severity)
			card.SeverityText = severityLabel(string(v.Severity))
			card.Summary = v.AnalysisSummary
			card.Stages = v.AffectedStages
			card.Analyzed = true
			if !v.DetectedAt.IsZero() {
				card.AnalyzedAt = v.DetectedAt.Format("02.01.2006 15:04")
			}
		}
		data.Cards = append(data.Cards, card)
	}

	s.renderDiffPage(w, data)
}

// documentVerdicts собирает самые свежие зафиксированные ИИ-вердикты по
// документам из ленты изменений (entity_type=document) — по одному на документ.
func (s *Server) documentVerdicts(ctx context.Context) map[string]changes.Event {
	if s.changeStore == nil {
		return map[string]changes.Event{}
	}
	// Быстрый путь: один точечный запрос «последний вердикт на документ».
	if ps, ok := s.changeStore.(*changes.PostgresStore); ok {
		if m, err := ps.LatestAnalyzedByType(ctx, changes.EntityDocument); err == nil {
			return m
		}
	}
	// Фолбэк (не-Postgres бэкенд): скан последних событий ленты.
	evs, err := s.changeStore.Recent(ctx, changes.Filter{EntityType: changes.EntityDocument, Limit: 1000})
	if err != nil {
		return map[string]changes.Event{}
	}
	return latestVerdicts(evs)
}

// latestVerdicts из списка событий, отсортированного по убыванию времени,
// выбирает по одному — самому свежему — событию с непустым ИИ-вердиктом на
// документ. Вынесено отдельно для прямого юнит-тестирования без БД.
func latestVerdicts(evs []changes.Event) map[string]changes.Event {
	out := map[string]changes.Event{}
	for _, ev := range evs {
		if _, seen := out[ev.EntityID]; seen {
			continue
		}
		if ev.Severity == "" {
			continue // вердикта пока нет — ждём следующее (более старое) событие
		}
		out[ev.EntityID] = ev
	}
	return out
}

// monitorAgentConfigured сообщает, настроен ли включённый агент «Монитор»
// (для подсказки на странице: будет ли реальный ИИ-анализ или эвристика).
func (s *Server) monitorAgentConfigured(ctx context.Context) bool {
	if s.aiStore == nil {
		return false
	}
	agents, err := s.aiStore.ListAgents(ctx)
	if err != nil {
		return false
	}
	for _, a := range agents {
		if a.AgentType == aimodels.AgentMonitor && a.Enabled && a.ModelID != "" {
			return true
		}
	}
	return false
}

// handleDiffFull отдаёт полный визуальный дифф двух последних версий документа
// (самостоятельный HTML-документ — встраивается в iframe на странице сравнения).
func (s *Server) handleDiffFull(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.versionStore == nil {
		http.Error(w, "хранилище версий не подключено", http.StatusServiceUnavailable)
		return
	}
	vers, err := s.versionStore.LatestVersions(r.Context(), id, 2)
	if err != nil || len(vers) < 2 {
		http.Error(w, "недостаточно версий для сравнения", http.StatusNotFound)
		return
	}
	newV, oldV := vers[0], vers[1]
	result := diff.CompareDocuments(oldV.ExtractedText, newV.ExtractedText)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, diff.ToHTML(result))
}

// handleAPIReanalyze заново запускает агента «Монитор» на двух последних
// версиях документа и возвращает свежий ИИ-вердикт (важность, суть, стадии).
// При отсутствии настроенного агента отрабатывает эвристика анализатора.
func (s *Server) handleAPIReanalyze(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.versionStore == nil {
		jsonResp(w, false, "", "хранилище версий не подключено")
		return
	}
	doc, _ := s.store.Get(r.Context(), id)
	ev := changes.Event{
		EntityType: changes.EntityDocument,
		EntityID:   id,
		Title:      doc.Title,
		Category:   doc.Category,
		Kind:       changes.KindUpdated,
	}

	// aiStore передаём только если он реально подключён — иначе типизированный
	// nil-указатель в интерфейсе ModelStore привёл бы к панике в анализаторе.
	var modelStore relevance.ModelStore
	if s.aiStore != nil {
		modelStore = s.aiStore
	}
	analyzer := relevance.NewAnalyzer(s.versionStore, modelStore)
	res, err := analyzer.Analyze(r.Context(), ev)
	if err != nil {
		jsonResp(w, false, "", "анализ не выполнен: "+err.Error())
		return
	}

	// Сохраняем свежий вердикт в ленту изменений (best-effort): дописываем его в
	// самое свежее событие документа, чтобы он пережил перезагрузку страницы.
	persisted := s.persistVerdict(r.Context(), id, res)

	stages := make([]string, 0, len(res.Stages))
	for _, st := range res.Stages {
		stages = append(stages, string(st))
	}
	resp := map[string]interface{}{
		"ok":            true,
		"severity":      string(res.Severity),
		"severity_text": severityLabel(string(res.Severity)),
		"summary":       res.Summary,
		"stages":        stages,
		"added":         res.DiffAdded,
		"removed":       res.DiffRemoved,
		"used_llm":      res.UsedLLM,
		"persisted":     persisted,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// persistVerdict дописывает свежий вердикт анализатора в самое свежее событие
// ленты по документу. Возвращает true, если вердикт сохранён. Работает только
// с Postgres-лентой (у не-Postgres бэкендов постоянного хранилища ленты нет).
func (s *Server) persistVerdict(ctx context.Context, docID string, res relevance.Result) bool {
	ps, ok := s.changeStore.(*changes.PostgresStore)
	if !ok {
		return false
	}
	ev, found, err := ps.LatestByEntity(ctx, changes.EntityDocument, docID)
	if err != nil || !found {
		return false
	}
	if err := ps.Enrich(ctx, ev.ID, res.ToEnrichment()); err != nil {
		return false
	}
	return true
}

// handleAPIDiff отдаёт результат сравнения в JSON.
func (s *Server) handleAPIDiff(w http.ResponseWriter, r *http.Request) {
	id1 := r.PathValue("id1")
	id2 := r.PathValue("id2")

	if id1 == "" || id2 == "" {
		jsonResp(w, false, "", "Укажите оба ID документов")
		return
	}

	text1, _, err := s.extractDocText(r.Context(), id1)
	if err != nil {
		jsonResp(w, false, "", "Документ 1: "+err.Error())
		return
	}
	text2, _, err := s.extractDocText(r.Context(), id2)
	if err != nil {
		jsonResp(w, false, "", "Документ 2: "+err.Error())
		return
	}

	result := diff.CompareDocuments(text1, text2)

	doc1Info, _ := s.store.Get(r.Context(), id1)
	doc2Info, _ := s.store.Get(r.Context(), id2)
	resp := map[string]interface{}{
		"ok":       true,
		"doc1":     doc1Info.Title,
		"doc2":     doc2Info.Title,
		"summary":  result.Summary,
		"added":    len(result.AddedLines),
		"removed":  len(result.RemovedLines),
		"sections": len(result.ModifiedSections),
		"html":     diff.ToHTML(result),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// renderDiffPage рисует страницу автоматического сравнения версий.
func (s *Server) renderDiffPage(w http.ResponseWriter, data diffPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "diff-layout", data); err != nil {
		log.Println("[admin] diff шаблон:", err)
	}
}

// extractDocText извлекает текст из документа (файл или заглушка).
func (s *Server) extractDocText(ctx context.Context, id string) (string, model.Document, error) {
	doc, err := s.store.Get(ctx, id)
	if err != nil {
		return "", model.Document{}, err
	}
	if doc.LocalPath == "" {
		return "", doc, fmt.Errorf("нет локального файла")
	}
	if _, err := os.Stat(doc.LocalPath); os.IsNotExist(err) {
		return "", doc, fmt.Errorf("файл не найден: %s", doc.LocalPath)
	}

	ext := strings.ToLower(filepath.Ext(doc.LocalPath))
	if ext == ".pdf" {
		if extract.IsSupported(doc.LocalPath) {
			text, err := extract.Text(doc.LocalPath)
			if err != nil {
				return "", doc, fmt.Errorf("извлечение текста: %w", err)
			}
			return text, doc, nil
		}
		// Fallback: read as raw
		data, err := os.ReadFile(doc.LocalPath)
		if err != nil {
			return "", doc, err
		}
		return string(data), doc, nil
	}

	if extract.IsSupported(doc.LocalPath) {
		text, err := extract.Text(doc.LocalPath)
		if err != nil {
			return "", doc, fmt.Errorf("извлечение текста: %w", err)
		}
		return text, doc, nil
	}

	// Текстовые форматы
	data, err := os.ReadFile(doc.LocalPath)
	if err != nil {
		return "", doc, err
	}
	return string(data), doc, nil
}

// ===========================================================================
// Аналитика
// ===========================================================================

// handleAnalyticsPage показывает HTML-дашборд аналитики.
func (s *Server) handleAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	report := s.collectAnalyticsReport(r.Context())
	htmlContent := analytics.ToHTML(report)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlContent)
}

// handleAPIAnalytics отдаёт отчёт аналитики в JSON.
func (s *Server) handleAPIAnalytics(w http.ResponseWriter, r *http.Request) {
	report := s.collectAnalyticsReport(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

// handleAnalyticsExport экспортирует отчёт в CSV.
func (s *Server) handleAnalyticsExport(w http.ResponseWriter, r *http.Request) {
	report := s.collectAnalyticsReport(r.Context())
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}

	switch format {
	case "csv":
		csvContent := analytics.ToCSV(report)
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=analytics.csv")
		fmt.Fprint(w, csvContent)
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=analytics.json")
		_ = json.NewEncoder(w).Encode(report)
	default:
		http.Error(w, "Unsupported format: "+format, http.StatusBadRequest)
	}
}

// collectAnalyticsReport собирает отчёт из доступных хранилищ.
func (s *Server) collectAnalyticsReport(ctx context.Context) *analytics.AnalyticsReport {
	// Заглушки для отсутствующих хранилищ — передаём nil-совместимые заглушки
	report := analytics.CollectReport(
		ctx,
		s.store,
		nil, // clientStore
		nil, // checklistStore
		nil, // deadlineStore
		nil, // eventStore
		nil, // contestStore
	)
	return report
}

// ===========================================================================
// Граф связей документов
// ===========================================================================

type graphData struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

type graphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"`
	Title string `json:"title"`
}

type graphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty"`
	Color  string `json:"color,omitempty"`
	Dashes bool   `json:"dashes,omitempty"`
}

// handleGraphPage показывает визуализацию графа связей.
func (s *Server) handleGraphPage(w http.ResponseWriter, r *http.Request) {
	graph := s.buildGraphData(r.Context())

	data := struct {
		GraphJSON string
	}{
		GraphJSON: graphToJSON(graph),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "graph-layout", data); err != nil {
		log.Println("[admin] graph шаблон:", err)
	}
}

// handleAPIGraphDoc отдаёт связи конкретного документа в JSON.
func (s *Server) handleAPIGraphDoc(w http.ResponseWriter, r *http.Request) {
	if s.linkStore == nil {
		jsonResp(w, false, "", "Хранилище связей не настроено")
		return
	}

	docID := r.PathValue("document_id")
	linkType := model.DocumentLinkType(r.URL.Query().Get("type"))

	links, err := s.linkStore.GetDocumentLinks(r.Context(), docID, linkType)
	if err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}

	// Собираем граф для одного документа
	nodes := make(map[string]*model.Document)
	for _, l := range links {
		if _, ok := nodes[l.SourceID]; !ok {
			if d, err := s.store.Get(r.Context(), l.SourceID); err == nil {
				nodes[l.SourceID] = &d
			}
		}
		if _, ok := nodes[l.TargetID]; !ok {
			if d, err := s.store.Get(r.Context(), l.TargetID); err == nil {
				nodes[l.TargetID] = &d
			}
		}
	}

	graph := graphData{}
	for id, doc := range nodes {
		graph.Nodes = append(graph.Nodes, graphNode{
			ID:    id,
			Label: doc.Title,
			Group: doc.Category,
			Title: fmt.Sprintf("%s [%s]", doc.Title, id),
		})
	}
	for _, l := range links {
		graph.Edges = append(graph.Edges, graphEdge{
			From:  l.SourceID,
			To:    l.TargetID,
			Label: string(l.LinkType),
			Color: linkTypeColor(l.LinkType),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(graph)
}

// handleAPIGraphCreateLink создаёт новую связь между документами.
func (s *Server) handleAPIGraphCreateLink(w http.ResponseWriter, r *http.Request) {
	if s.linkStore == nil {
		jsonResp(w, false, "", "Хранилище связей не настроено")
		return
	}

	var req struct {
		SourceID string `json:"source_id"`
		TargetID string `json:"target_id"`
		LinkType string `json:"link_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResp(w, false, "", "Ошибка разбора JSON")
		return
	}

	link := &model.DocumentLink{
		SourceID:  req.SourceID,
		TargetID:  req.TargetID,
		LinkType:  model.DocumentLinkType(req.LinkType),
		CreatedAt: time.Now(),
	}

	if err := s.linkStore.CreateDocumentLink(r.Context(), link); err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}

	jsonResp(w, true, "Связь создана", "")
}

// handleAPIGraphDeleteLink удаляет связь между документами.
func (s *Server) handleAPIGraphDeleteLink(w http.ResponseWriter, r *http.Request) {
	if s.linkStore == nil {
		jsonResp(w, false, "", "Хранилище связей не настроено")
		return
	}

	linkID := r.PathValue("link_id")
	if err := s.linkStore.DeleteDocumentLink(r.Context(), linkID); err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}

	jsonResp(w, true, "Связь удалена", "")
}

// buildGraphData строит полный граф из всех связей.
func (s *Server) buildGraphData(ctx context.Context) graphData {
	var links []*model.DocumentLink
	var err error

	if s.linkStore != nil {
		links, err = s.linkStore.ListAllLinks(ctx)
		if err != nil {
			log.Printf("[admin/graph] ошибка загрузки связей: %v", err)
		}
	}

	// Также собираем связи из Supersedes
	docs, _ := s.store.List(ctx, store.Filter{})
	for _, d := range docs {
		if d.Supersedes != "" {
			links = append(links, &model.DocumentLink{
				ID:        "supersedes-" + d.ID,
				SourceID:  d.ID,
				TargetID:  d.Supersedes,
				LinkType:  model.LinkSupersedes,
				CreatedAt: time.Now(),
			})
		}
	}

	// Собираем уникальные документы
	docMap := make(map[string]*model.Document)
	for _, l := range links {
		if _, ok := docMap[l.SourceID]; !ok {
			if d, err := s.store.Get(ctx, l.SourceID); err == nil {
				docMap[l.SourceID] = &d
			} else {
				docMap[l.SourceID] = &model.Document{ID: l.SourceID, Title: l.SourceID, Category: "unknown"}
			}
		}
		if _, ok := docMap[l.TargetID]; !ok {
			if d, err := s.store.Get(ctx, l.TargetID); err == nil {
				docMap[l.TargetID] = &d
			} else {
				docMap[l.TargetID] = &model.Document{ID: l.TargetID, Title: l.TargetID, Category: "unknown"}
			}
		}
	}

	graph := graphData{}
	// Nodes
	for id, doc := range docMap {
		group := doc.Category
		if group == "" {
			group = "uncategorized"
		}
		graph.Nodes = append(graph.Nodes, graphNode{
			ID:    id,
			Label: truncate(doc.Title, 60),
			Group: group,
			Title: fmt.Sprintf("%s [%s]\nСтатус: %s", doc.Title, id, doc.Status),
		})
	}
	// Edges
	for _, l := range links {
		graph.Edges = append(graph.Edges, graphEdge{
			From:   l.SourceID,
			To:     l.TargetID,
			Label:  string(l.LinkType),
			Color:  linkTypeColor(l.LinkType),
			Dashes: l.LinkType == model.LinkSupersedes,
		})
	}

	return graph
}

func linkTypeColor(lt model.DocumentLinkType) string {
	switch lt {
	case model.LinkReferences:
		return "#2563eb" // blue
	case model.LinkSupersedes:
		return "#dc2626" // red
	case model.LinkRelated:
		return "#16a34a" // green
	default:
		return "#6b7280" // gray
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func graphToJSON(g graphData) string {
	data, _ := json.Marshal(g)
	return string(data)
}

// ===========================================================================
// Лента изменений (история обновлений базы)
// ===========================================================================

type changesPageData struct {
	Rows       []changeRow
	Flash      string
	FlashKind  string
	Query      string
	EntityType string
	Category   string
	DateFrom   string
	DateTo     string
	Tag        string
	AllTags    []tagCount
	BaseQS     string // строка запроса прочих фильтров (без tag) для ссылок-чипов
	Health     []sourceHealthRow
	Stats      changesStats
}

// changeRow — изменение вместе с авто-тегами для отображения и фильтрации.
type changeRow struct {
	Event changes.Event
	Tags  []string
}

// tagCount — тег и сколько изменений им помечено (для облака тегов).
type tagCount struct {
	Name  string
	Count int
	Enc   string // url.QueryEscape(Name) — для ссылки-чипа
}

// sourceHealthRow — строка панели «когда обновлялось» по одному источнику.
type sourceHealthRow struct {
	Label       string
	State       string // ok|stale|failing|unknown
	StateLabel  string
	LastSuccess string
	Items       int
	LastError   string // текст последней ошибки (показывается для failing/stale)
}

type changesStats struct {
	Total     int
	New       int
	Updated   int
	Outdated  int
	Removed   int
	LastParse time.Time
}

// handleChangesPage показывает HTML-страницу истории изменений.
func (s *Server) handleChangesPage(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	entityType := r.URL.Query().Get("entity_type")
	category := r.URL.Query().Get("category")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))

	// Период по умолчанию: последние 30 дней
	since := time.Now().AddDate(0, 0, -30)
	if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			since = t
		}
	}

	filter := changes.Filter{
		Since:      since,
		EntityType: entityType,
		Category:   category,
		Limit:      500,
	}

	var events []changes.Event
	if s.changeStore != nil {
		var err error
		events, err = s.changeStore.Recent(r.Context(), filter)
		if err != nil {
			log.Printf("[admin/changes] ошибка загрузки: %v", err)
			events = nil
		}
	}

	// Фильтрация по поиску на стороне Go (для заголовков)
	if query != "" && len(events) > 0 {
		filtered := make([]changes.Event, 0, len(events))
		qLower := strings.ToLower(query)
		for _, ev := range events {
			if strings.Contains(strings.ToLower(ev.Title), qLower) ||
				strings.Contains(strings.ToLower(ev.Summary), qLower) ||
				strings.Contains(strings.ToLower(ev.EntityID), qLower) {
				filtered = append(filtered, ev)
			}
		}
		events = filtered
	}

	// Ограничение по date_to (в локальной зоне — DetectedAt из TIMESTAMPTZ tz-aware).
	if dateTo != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", dateTo+" 23:59", time.Local); err == nil {
			filtered := make([]changes.Event, 0, len(events))
			for _, ev := range events {
				if !ev.DetectedAt.After(t) {
					filtered = append(filtered, ev)
				}
			}
			events = filtered
		}
	}

	// Авто-теги из метаданных + облако тегов. Облако считаем до фильтра по тегу,
	// чтобы чипы отражали весь период, а не уже отфильтрованную выборку.
	rows := make([]changeRow, 0, len(events))
	tagFreq := map[string]int{}
	for _, ev := range events {
		tags := deriveTags(ev)
		rows = append(rows, changeRow{Event: ev, Tags: tags})
		for _, t := range tags {
			tagFreq[t]++
		}
	}

	// Фильтр по выбранному тегу.
	if tag != "" {
		filtered := make([]changeRow, 0, len(rows))
		for _, row := range rows {
			if containsStr(row.Tags, tag) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	// Статистика по показанным изменениям.
	var st changesStats
	st.Total = len(rows)
	for _, row := range rows {
		switch row.Event.Kind {
		case changes.KindNew:
			st.New++
		case changes.KindUpdated:
			st.Updated++
		case changes.KindOutdated:
			st.Outdated++
		case changes.KindRemoved:
			st.Removed++
		}
	}

	// Время последнего парсинга — берём из отчётов коллектора
	if s.reportStore != nil {
		if reports, err := s.reportStore.GetReports(1); err == nil && len(reports) > 0 {
			st.LastParse = reports[0].StartedAt
		}
	}

	allTags := topTags(tagFreq, 24)
	for i := range allTags {
		allTags[i].Enc = url.QueryEscape(allTags[i].Name)
	}

	// Строка запроса прочих фильтров (без tag) — чтобы чипы тегов их сохраняли.
	base := url.Values{}
	if query != "" {
		base.Set("q", query)
	}
	if entityType != "" {
		base.Set("entity_type", entityType)
	}
	if category != "" {
		base.Set("category", category)
	}
	if dateFrom != "" {
		base.Set("date_from", dateFrom)
	}
	if dateTo != "" {
		base.Set("date_to", dateTo)
	}

	data := changesPageData{
		Rows:       rows,
		Query:      query,
		EntityType: entityType,
		Category:   category,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Tag:        tag,
		AllTags:    allTags,
		BaseQS:     base.Encode(),
		Health:     s.sourceHealthRows(r.Context()),
		Stats:      st,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "changes-layout", data); err != nil {
		log.Println("[admin] changes шаблон:", err)
	}
}

// deriveTags выводит авто-теги изменения из его метаданных: тип сущности,
// категория, вид изменения, домен источника и важность (если задана). Ручной
// разметки не требуется — теги вычисляются на лету.
func deriveTags(ev changes.Event) []string {
	var tags []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !containsStr(tags, s) {
			tags = append(tags, s)
		}
	}
	add(entityTypeLabel(ev.EntityType))
	add(ev.Category)
	add(kindLabel(string(ev.Kind)))
	add(sourceDomain(ev.SourceURL))
	add(severityLabel(string(ev.Severity)))
	return tags
}

// entityTypeLabel — человекочитаемое имя типа сущности для тегов/фильтров.
func entityTypeLabel(t string) string {
	switch t {
	case changes.EntityDocument:
		return "Документ"
	case changes.EntityNews:
		return "Новость"
	case changes.EntityEvent:
		return "Мероприятие"
	case changes.EntityContest:
		return "Конкурс/грант"
	case changes.EntityNPA:
		return "НПА"
	case changes.EntityPreference:
		return "Льгота"
	case changes.EntityFAQ:
		return "FAQ"
	case changes.EntityTelegram:
		return "Telegram"
	case changes.EntitySitePage:
		return "Страница сайта"
	default:
		return t
	}
}

func kindLabel(k string) string {
	switch k {
	case string(changes.KindNew):
		return "Новое"
	case string(changes.KindUpdated):
		return "Обновлено"
	case string(changes.KindOutdated):
		return "Устарело"
	case string(changes.KindRemoved):
		return "Удалено"
	default:
		return ""
	}
}

func severityLabel(s string) string {
	switch s {
	case "info":
		return "Инфо"
	case "warning":
		return "Важное"
	case "critical":
		return "Критично"
	default:
		return ""
	}
}

// sourceDomain возвращает домен источника без префикса www (для тега «sk.ru»).
func sourceDomain(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// topTags возвращает не более limit самых частых тегов, отсортированных по
// убыванию частоты, затем по алфавиту.
func topTags(freq map[string]int, limit int) []tagCount {
	out := make([]tagCount, 0, len(freq))
	for name, n := range freq {
		out = append(out, tagCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// sourceHealthRows строит панель «когда обновлялось» по всем источникам.
func (s *Server) sourceHealthRows(ctx context.Context) []sourceHealthRow {
	if s.healthStore == nil {
		return nil
	}
	sources, err := s.healthStore.List(ctx)
	if err != nil {
		log.Printf("[admin/changes] свежесть источников: %v", err)
		return nil
	}
	now := time.Now()
	rows := make([]sourceHealthRow, 0, len(sources))
	for _, src := range sources {
		state := src.State(24*time.Hour, now)
		last := "—"
		if src.LastSuccessAt != nil {
			last = src.LastSuccessAt.Format("02.01.2006 15:04")
		}
		// Текст ошибки показываем только когда источник реально проблемный —
		// чтобы админ видел ЧТО чинить, а не просто «ошибки».
		lastErr := ""
		if src.LastError != "" && (state == health.StatusFailing || state == health.StatusStale) {
			lastErr = src.LastError
		}
		rows = append(rows, sourceHealthRow{
			Label:       sourceLabel(src.Name),
			State:       string(state),
			StateLabel:  healthStateLabel(state),
			LastSuccess: last,
			Items:       src.ItemsLastRun,
			LastError:   lastErr,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Label < rows[j].Label })
	return rows
}

// sourceLabel — человекочитаемое имя источника для панели свежести.
func sourceLabel(name string) string {
	switch name {
	case "documents":
		return "Документы"
	case "news":
		return "Новости"
	case "events":
		return "Мероприятия"
	case "contests":
		return "Конкурсы/гранты"
	case "faq":
		return "FAQ"
	case "telegram":
		return "Telegram"
	case "preferences":
		return "Льготы"
	case "regulations":
		return "НПА"
	case "residents":
		return "Резиденты"
	case "sitepages":
		return "Страницы сайта"
	case "fetch":
		return "Загрузка файлов"
	default:
		return name
	}
}

func healthStateLabel(st health.Status) string {
	switch st {
	case health.StatusOK:
		return "актуально"
	case health.StatusStale:
		return "устарело"
	case health.StatusFailing:
		return "ошибки"
	default:
		return "нет данных"
	}
}

// handleAPIChanges отдаёт ленту изменений в JSON.
func (s *Server) handleAPIChanges(w http.ResponseWriter, r *http.Request) {
	if s.changeStore == nil {
		jsonResp(w, false, "", "Хранилище изменений не подключено")
		return
	}

	sinceDays := 30
	if v := r.URL.Query().Get("since_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			sinceDays = n
		}
	}

	since := time.Now().AddDate(0, 0, -sinceDays)
	filter := changes.Filter{
		Since:      since,
		EntityType: r.URL.Query().Get("entity_type"),
		Category:   r.URL.Query().Get("category"),
		Limit:      200,
	}

	events, err := s.changeStore.Recent(r.Context(), filter)
	if err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}

	// Фильтр по авто-тегу (паритет с HTML-страницей /changes).
	if tag := strings.TrimSpace(r.URL.Query().Get("tag")); tag != "" {
		filtered := events[:0]
		for _, ev := range events {
			if containsStr(deriveTags(ev), tag) {
				filtered = append(filtered, ev)
			}
		}
		events = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":     true,
		"events": events,
		"count":  len(events),
	})
}

// ---------------------------------------------------------------------------
// Страницы публичного сайта: список + просмотрщик.
// ---------------------------------------------------------------------------

type sitePagesPageData struct {
	Rows         []sitePageRow
	Query        string
	Section      string
	Status       string
	DateFrom     string
	DateTo       string
	Sections     []string
	AllTags      []string        // все теги для фильтра множественного выбора
	SelectedTags []string        // выбранные теги
	SelectedSet  map[string]bool // для отметки checkbox в шаблоне
	Total        int
	LastCrawl    time.Time
	HasStore     bool
}

type sitePageRow struct {
	ID          string
	URL         string
	Title       string
	Section     string
	Status      string
	StatusLabel string
	Tags        []string
	LastChanged time.Time
}

// relRow — ссылка на связанную страницу (по тегам или семантически).
type relRow struct {
	ID      string
	URL     string
	Title   string
	Section string
	Shared  int // число общих тегов (для связи по тегам)
}

type sitePageViewData struct {
	ID              string
	URL             string
	Title           string
	Section         string
	Status          string
	StatusLabel     string
	Summary         string
	Text            string
	HasText         bool
	FirstSeen       time.Time
	LastSeen        time.Time
	LastChanged     time.Time
	Enriched        bool
	AISummary       string
	Goals           string
	Theses          []string
	Conclusions     string
	Tags            []string
	RelatedByTags   []relRow
	RelatedSemantic []relRow
	CanEdit         bool   // доступны ли ручные действия (переаннотировать/править)
	TagsCSV         string // теги через запятую (для формы правки)
	ThesesText      string // тезисы по строкам (для textarea правки)
}

// sitePageStatusLabel — человекочитаемый статус страницы сайта.
func sitePageStatusLabel(st string) string {
	switch st {
	case sitepages.StatusActive:
		return "доступна"
	case sitepages.StatusGone:
		return "недоступна (404)"
	default:
		return st
	}
}

// handleSitePagesPage — список страниц публичного сайта с фильтрами
// (поиск, раздел, статус, период по дате изменения).
func (s *Server) handleSitePagesPage(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	section := r.URL.Query().Get("section")
	status := r.URL.Query().Get("status")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	selectedTags := normalizeTagParams(r.URL.Query()["tags"])

	var pages []*sitepages.Page
	if s.sitePages != nil {
		if ps, err := s.sitePages.ListRecent(r.Context(), 2000); err == nil {
			pages = ps
		} else {
			log.Printf("[admin/sitepages] ошибка загрузки: %v", err)
		}
	}

	// Списки разделов и тегов для фильтров (по всем страницам, до фильтрации).
	sectionSet := map[string]bool{}
	tagSet := map[string]bool{}
	var lastCrawl time.Time
	for _, p := range pages {
		if p.Section != "" {
			sectionSet[p.Section] = true
		}
		for _, t := range p.Tags {
			tagSet[t] = true
		}
		if p.LastSeen.After(lastCrawl) {
			lastCrawl = p.LastSeen
		}
	}
	// Время последнего обхода — точнее из мониторинга свежести.
	if s.healthStore != nil {
		if src, err := s.healthStore.Get(r.Context(), "sitepages"); err == nil && src.LastSuccessAt != nil {
			lastCrawl = *src.LastSuccessAt
		}
	}

	var since, until time.Time
	if dateFrom != "" {
		if t, err := time.ParseInLocation("2006-01-02", dateFrom, time.Local); err == nil {
			since = t
		}
	}
	if dateTo != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04", dateTo+" 23:59", time.Local); err == nil {
			until = t
		}
	}

	qLower := strings.ToLower(query)
	rows := make([]sitePageRow, 0, len(pages))
	for _, p := range pages {
		if query != "" {
			if !strings.Contains(strings.ToLower(p.Title), qLower) &&
				!strings.Contains(strings.ToLower(p.URL), qLower) &&
				!strings.Contains(strings.ToLower(p.Section), qLower) &&
				!strings.Contains(strings.ToLower(p.Summary), qLower) {
				continue
			}
		}
		if section != "" && p.Section != section {
			continue
		}
		if status != "" && p.Status != status {
			continue
		}
		if !since.IsZero() && p.LastChanged.Before(since) {
			continue
		}
		if !until.IsZero() && p.LastChanged.After(until) {
			continue
		}
		// Фильтр по тегам: страница должна содержать ВСЕ выбранные теги (AND).
		if len(selectedTags) > 0 && !hasAllTags(p.Tags, selectedTags) {
			continue
		}
		rows = append(rows, sitePageRow{
			ID:          p.ID,
			URL:         p.URL,
			Title:       p.Title,
			Section:     p.Section,
			Status:      p.Status,
			StatusLabel: sitePageStatusLabel(p.Status),
			Tags:        p.Tags,
			LastChanged: p.LastChanged,
		})
	}

	sections := make([]string, 0, len(sectionSet))
	for sec := range sectionSet {
		sections = append(sections, sec)
	}
	sort.Strings(sections)

	allTags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		allTags = append(allTags, t)
	}
	sort.Strings(allTags)

	selectedSet := make(map[string]bool, len(selectedTags))
	for _, t := range selectedTags {
		selectedSet[t] = true
	}

	data := sitePagesPageData{
		Rows:         rows,
		Query:        query,
		Section:      section,
		Status:       status,
		DateFrom:     dateFrom,
		DateTo:       dateTo,
		Sections:     sections,
		AllTags:      allTags,
		SelectedTags: selectedTags,
		SelectedSet:  selectedSet,
		Total:        len(rows),
		LastCrawl:    lastCrawl,
		HasStore:     s.sitePages != nil,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "sitepages-layout", data); err != nil {
		log.Println("[admin] sitepages шаблон:", err)
	}
}

// handleSitePageView — просмотрщик одной страницы сайта: сохранённая информация
// (заголовок, раздел, текст) + кнопка «Открыть на сайте Сколково».
func (s *Server) handleSitePageView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.sitePages == nil {
		http.Error(w, "хранилище страниц сайта не подключено", http.StatusServiceUnavailable)
		return
	}
	p, err := s.sitePages.GetWithText(r.Context(), id)
	if errors.Is(err, sitepages.ErrNotFound) {
		http.Error(w, "страница не найдена", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "ошибка загрузки: "+err.Error(), http.StatusInternalServerError)
		return
	}

	text := p.Text
	if strings.TrimSpace(text) == "" {
		text = p.Summary
	}
	data := sitePageViewData{
		ID:          p.ID,
		URL:         p.URL,
		Title:       p.Title,
		Section:     p.Section,
		Status:      p.Status,
		StatusLabel: sitePageStatusLabel(p.Status),
		Summary:     p.Summary,
		Text:        text,
		HasText:     strings.TrimSpace(text) != "",
		FirstSeen:   p.FirstSeen,
		LastSeen:    p.LastSeen,
		LastChanged: p.LastChanged,
		Enriched:    p.Enriched(),
		AISummary:   p.AISummary,
		Goals:       p.Goals,
		Theses:      p.Theses,
		Conclusions: p.Conclusions,
		Tags:        p.Tags,
		CanEdit:     s.sitePageOps != nil,
		TagsCSV:     strings.Join(p.Tags, ", "),
		ThesesText:  strings.Join(p.Theses, "\n"),
	}

	// Связанные страницы по общим тегам (из БД).
	if len(p.Tags) > 0 {
		if rel, err := s.sitePages.RelatedByTags(r.Context(), p.ID, p.Tags, 6); err == nil {
			for _, rp := range rel {
				data.RelatedByTags = append(data.RelatedByTags, relRow{
					ID: rp.ID, URL: rp.URL, Title: rp.Title, Section: rp.Section, Shared: rp.Shared,
				})
			}
		}
	}
	// Семантически близкие страницы (из Qdrant, если подключён поиск).
	if s.sitePageSim != nil {
		if hits, err := s.sitePageSim.Related(r.Context(), p, 6); err == nil {
			for _, h := range hits {
				data.RelatedSemantic = append(data.RelatedSemantic, relRow{
					ID: sitepages.IDForURL(h.URL), URL: h.URL, Title: h.Title, Section: h.Section,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "sitepage-view-layout", data); err != nil {
		log.Println("[admin] sitepage-view шаблон:", err)
	}
}

// handleSitePageReannotate перезапускает ИИ-аннотирование одной страницы и
// переиндексирует её. Доступно только при подключённых ручных операциях.
func (s *Server) handleSitePageReannotate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.sitePageOps == nil {
		http.Error(w, "ручные операции над страницами не подключены", http.StatusServiceUnavailable)
		return
	}
	if _, err := s.sitePageOps.ReannotateOne(r.Context(), id); err != nil {
		http.Error(w, "ошибка аннотирования: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sitepages/"+id, http.StatusSeeOther)
}

// handleSitePageSaveAnnotation сохраняет ручную правку аннотации куратором.
func (s *Server) handleSitePageSaveAnnotation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.sitePageOps == nil {
		http.Error(w, "ручные операции над страницами не подключены", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "некорректная форма", http.StatusBadRequest)
		return
	}
	ann := sitepages.Annotation{
		Tags:        splitCSV(r.FormValue("tags")),
		Summary:     strings.TrimSpace(r.FormValue("ai_summary")),
		Goals:       strings.TrimSpace(r.FormValue("goals")),
		Theses:      splitLines(r.FormValue("theses")),
		Conclusions: strings.TrimSpace(r.FormValue("conclusions")),
	}
	if err := s.sitePageOps.SaveAnnotation(r.Context(), id, ann); err != nil {
		http.Error(w, "ошибка сохранения: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/sitepages/"+id, http.StatusSeeOther)
}

// splitCSV разбивает строку «a, b, c» в список без пустых элементов.
func splitCSV(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitLines разбивает многострочный ввод (textarea) в список непустых строк.
func splitLines(raw string) []string {
	var out []string
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.TrimSpace(strings.TrimRight(l, "\r")); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// normalizeTagParams чистит и дедуплицирует теги из query-параметров (?tags=…).
func normalizeTagParams(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range raw {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// hasAllTags сообщает, содержит ли страница все указанные теги (регистронезависимо).
func hasAllTags(pageTags, want []string) bool {
	set := make(map[string]bool, len(pageTags))
	for _, t := range pageTags {
		set[strings.ToLower(t)] = true
	}
	for _, w := range want {
		if !set[strings.ToLower(w)] {
			return false
		}
	}
	return true
}

// handleAPISitePages отдаёт страницы сайта в JSON (для интеграций).
func (s *Server) handleAPISitePages(w http.ResponseWriter, r *http.Request) {
	if s.sitePages == nil {
		jsonResp(w, false, "", "Хранилище страниц сайта не подключено")
		return
	}
	pages, err := s.sitePages.ListRecent(r.Context(), 2000)
	if err != nil {
		jsonResp(w, false, "", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"pages": pages,
		"count": len(pages),
	})
}

// makeSourceLink вычисляет URL и подпись для кнопки «источник» в списке документов.
// Локальные file:// ссылки заменяются на внутренний admin-endpoint просмотра.
// HTTP-ссылки проксируются через /documents/{id}/source (server-side fetch с прокси).
func makeSourceLink(id, sourceURL, localPath string) (linkURL, linkText string) {
	if sourceURL == "" {
		return "", ""
	}
	if strings.HasPrefix(sourceURL, "file://") {
		if localPath != "" {
			return "/documents/" + id + "/view-original", "источник"
		}
		return "", ""
	}
	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		// Прямая ссылка на оригинал — открывается в браузере пользователя.
		// Серверный прокси бесполезен без доступа сервера к источнику (WAF/гео) — 502.
		return sourceURL, "открыть на сайте ↗"
	}
	return sourceURL, "источник"
}

// handleDocSource проксирует запрос к SourceURL документа через активный прокси сервера.
// Это позволяет администратору открывать ссылки на dochub.sk.ru даже если его браузер
// не имеет доступа к сайту напрямую (например, находится за пределами России).
func (s *Server) handleDocSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Документ не найден", http.StatusNotFound)
		return
	}

	// Локальный файл — отдаём через стандартный endpoint
	if strings.HasPrefix(doc.SourceURL, "file://") {
		if doc.LocalPath != "" {
			http.Redirect(w, r, "/documents/"+id+"/view-original", http.StatusFound)
			return
		}
		http.Error(w, "Файл недоступен", http.StatusNotFound)
		return
	}

	if doc.SourceURL == "" || (!strings.HasPrefix(doc.SourceURL, "http://") && !strings.HasPrefix(doc.SourceURL, "https://")) {
		http.Error(w, "Источник недоступен", http.StatusNotFound)
		return
	}

	// Строим HTTP-клиент с активным прокси (если настроен)
	transport := &http.Transport{}
	if proxyAddr := s.proxyManager.GetActiveURL(); proxyAddr != "" {
		if proxyU, parseErr := url.Parse(proxyAddr); parseErr == nil {
			transport.Proxy = http.ProxyURL(proxyU)
		}
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("слишком много редиректов")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", doc.SourceURL, nil)
	if err != nil {
		http.Error(w, "Ошибка запроса: "+err.Error(), http.StatusBadGateway)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Ошибка получения источника: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Пробрасываем Content-Type и статус
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
