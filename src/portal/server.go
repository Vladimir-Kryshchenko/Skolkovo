// Package portal — портал (личный кабинет) клиента.
package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/generator"
	"baza-skolkovo/src/mailer"
)

// PortalConfig — конфигурация портала.
type PortalConfig struct {
	Addr                string // например ":8092"
	BaseURL             string // например "http://portal.skolkovo.local"
	MCPURL              string // URL MCP-сервера для генерации документов
	MCPAPIKey           string // API-ключ для MCP
	TelegramBotUsername string // @username Telegram-бота, например @SkolkovoBot
}

// NotificationReader — доступ к персональным уведомлениям клиента (inbox).
// Реализуется *store.PostgresNotificationStore.
type NotificationReader interface {
	ListForClient(ctx context.Context, clientID string, limit int) ([]*model.ClientNotification, error)
	CountUnread(ctx context.Context, clientID string) (int, error)
	MarkRead(ctx context.Context, id, clientID string) error
}

// PortalStores — все хранилища, необходимые порталу.
type PortalStores struct {
	ClientStore       store.ClientStore
	ChecklistStore    store.ChecklistStore
	DeadlineStore     store.DeadlineStore
	TemplateStore     store.TemplateStore
	DocStore          store.ClientDocumentStore
	DocumentStore     store.Store                  // реестр документов (для скачивания)
	ChangeStore       changes.Store                // лента изменений; может быть nil
	NotifStore        NotificationReader           // inbox уведомлений клиента; может быть nil
	SubscriptionStore store.SubscriptionStore      // подписки на уведомления; может быть nil
	Generator         *generator.DocumentGenerator // генератор документов; может быть nil
	Mailer            *mailer.Mailer               // отправка ссылок входа; может быть nil
}

// PortalServer — HTTP-сервер личного кабинета.
type PortalServer struct {
	stores            PortalStores
	config            PortalConfig
	store             *magicStore // in-memory токены и сессии
	sessionCookieName string
}

// NewPortalServer создаёт сервер портала.
func NewPortalServer(config PortalConfig, stores PortalStores) *PortalServer {
	s := &PortalServer{
		stores:            stores,
		config:            config,
		store:             newMagicStore(),
		sessionCookieName: "portal_session",
	}
	// Запускаем фоновую очистку просроченных токенов/сессий
	go s.periodicCleanup()
	return s
}

// periodicCleanup удаляет просроченные записи каждые 10 минут.
func (ps *PortalServer) periodicCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ps.store.Cleanup()
	}
}

// Start запускает HTTP-сервер портала.
func (ps *PortalServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Публичные маршруты
	mux.HandleFunc("GET /", ps.handleIndex)
	mux.HandleFunc("GET /login", ps.handleLogin)
	mux.HandleFunc("POST /login", ps.handleLoginSubmit)
	mux.HandleFunc("GET /login/verify", ps.handleVerifyToken)

	// Маршруты, требующие авторизации
	mux.HandleFunc("GET /logout", ps.requireAuth(ps.handleLogout))
	mux.HandleFunc("GET /dashboard", ps.requireAuth(ps.handleDashboard))
	mux.HandleFunc("GET /checklists", ps.requireAuth(ps.handleChecklists))
	mux.HandleFunc("GET /deadlines", ps.requireAuth(ps.handleDeadlines))
	mux.HandleFunc("GET /documents", ps.requireAuth(ps.handleDocuments))
	mux.HandleFunc("GET /generate", ps.requireAuth(ps.handleGenerate))
	mux.HandleFunc("POST /generate", ps.requireAuth(ps.handleGenerateSubmit))
	mux.HandleFunc("GET /download", ps.requireAuth(ps.handleDownload))
	mux.HandleFunc("GET /documents/file", ps.requireAuth(ps.handleDocumentFile))
	mux.HandleFunc("GET /notifications", ps.requireAuth(ps.handleNotifications))
	mux.HandleFunc("POST /notifications/read", ps.requireAuth(ps.handleNotificationRead))
	mux.HandleFunc("GET /subscriptions", ps.requireAuth(ps.handleSubscriptions))
	mux.HandleFunc("POST /subscriptions", ps.requireAuth(ps.handleSubscriptionsSubmit))

	// JSON API
	mux.HandleFunc("GET /api/me", ps.requireAuthJSON(ps.apiMe))
	mux.HandleFunc("GET /api/checklists", ps.requireAuthJSON(ps.apiChecklists))
	mux.HandleFunc("GET /api/deadlines", ps.requireAuthJSON(ps.apiDeadlines))
	mux.HandleFunc("GET /api/documents", ps.requireAuthJSON(ps.apiDocuments))
	mux.HandleFunc("GET /api/notifications", ps.requireAuthJSON(ps.apiNotifications))

	log.Printf("[portal] портал клиента слушает %s", ps.config.Addr)

	server := &http.Server{
		Addr:    ps.config.Addr,
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return server.ListenAndServe()
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// getSessionID извлекает sessionID из cookie.
func (ps *PortalServer) getSessionID(r *http.Request) string {
	c, err := r.Cookie(ps.sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// requireAuth проверяет сессию и перенаправляет на /login при её отсутствии.
func (ps *PortalServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := ps.getSessionID(r)
		if sid == "" {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
			return
		}
		sess, err := ps.store.GetSession(sid)
		if err != nil {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
			return
		}
		// Передаём сессию через context
		ctx := context.WithValue(r.Context(), ctxSessionKey{}, sess)
		next(w, r.WithContext(ctx))
	}
}

type ctxSessionKey struct{}

// sessionFromContext извлекает сессию из context.
func sessionFromContext(r *http.Request) *Session {
	s, _ := r.Context().Value(ctxSessionKey{}).(*Session)
	return s
}

// requireAuthJSON — аналог requireAuth для JSON API.
func (ps *PortalServer) requireAuthJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := ps.getSessionID(r)
		if sid == "" {
			jsonError(w, http.StatusUnauthorized, "требуется авторизация")
			return
		}
		sess, err := ps.store.GetSession(sid)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "сессия истекла")
			return
		}
		ctx := context.WithValue(r.Context(), ctxSessionKey{}, sess)
		next(w, r.WithContext(ctx))
	}
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true", "msg": msg})
}

// ---------------------------------------------------------------------------
// Handlers — аутентификация
// ---------------------------------------------------------------------------

func (ps *PortalServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	sid := ps.getSessionID(r)
	if sid != "" {
		if _, err := ps.store.GetSession(sid); err == nil {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (ps *PortalServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	data := loginData{
		Next:      next,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "login", data); err != nil {
		log.Println("[portal] шаблон login:", err)
	}
}

func (ps *PortalServer) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		http.Redirect(w, r, "/login?msg=Введите+email&kind=err", http.StatusSeeOther)
		return
	}

	// Ищем клиента по email
	client, err := ps.lookupClientByEmail(r.Context(), email)
	if err != nil {
		http.Redirect(w, r, "/login?msg=Клиент+с+таким+email+не+найден&kind=err", http.StatusSeeOther)
		return
	}

	// Генерируем magic link
	link, err := ps.store.GenerateMagicLink(email, client.ID, ps.config.BaseURL)
	if err != nil {
		http.Redirect(w, r, "/login?msg=Ошибка+генерации+ссылки&kind=err", http.StatusSeeOther)
		return
	}

	// Если настроен SMTP — отправляем ссылку на email и не раскрываем её на странице.
	if ps.stores.Mailer != nil && ps.stores.Mailer.Enabled() {
		body := fmt.Sprintf("Здравствуйте!\n\nДля входа в личный кабинет «База Сколково» перейдите по ссылке (действует 15 минут):\n\n%s\n\nЕсли вы не запрашивали вход, проигнорируйте это письмо.", link)
		if err := ps.stores.Mailer.Send(r.Context(), email, "Вход в личный кабинет «База Сколково»", body); err != nil {
			log.Printf("[portal] не удалось отправить ссылку на %s: %v", email, err)
			http.Redirect(w, r, "/login?msg=Не+удалось+отправить+письмо,+попробуйте+позже&kind=err", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/login?msg=Ссылка+для+входа+отправлена+на+ваш+email&kind=ok", http.StatusSeeOther)
		return
	}

	// Без SMTP ссылку показываем на странице ТОЛЬКО в явном dev-режиме
	// (PORTAL_DEV_LOGIN_LINK=true). Иначе это утечка: любой, кто знает email
	// клиента, получил бы рабочую ссылку входа прямо на экране.
	if os.Getenv("PORTAL_DEV_LOGIN_LINK") == "true" {
		http.Redirect(w, r, "/login?msg=Ссылка+для+входа+(dev):+&link="+url.QueryEscape(link)+"&kind=ok", http.StatusSeeOther)
		return
	}
	log.Printf("[portal] SMTP не настроен — ссылка для %s не отправлена; вход по email недоступен", email)
	http.Redirect(w, r, "/login?msg=Вход+по+email+временно+недоступен,+обратитесь+к+администратору&kind=err", http.StatusSeeOther)
}

func (ps *PortalServer) handleVerifyToken(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login?msg=Токен+не+указан&kind=err", http.StatusSeeOther)
		return
	}

	clientID, email, err := ps.store.VerifyMagicLink(token)
	if err != nil {
		http.Redirect(w, r, "/login?msg="+url.QueryEscape(err.Error())+"&kind=err", http.StatusSeeOther)
		return
	}

	// Создаём сессию
	sessionID := ps.store.CreateSession(clientID, email)

	http.SetCookie(w, &http.Cookie{
		Name:     ps.sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/dashboard?msg=Добро+пожаловать!", http.StatusSeeOther)
}

func (ps *PortalServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	sid := ps.getSessionID(r)
	if sid != "" {
		ps.store.DeleteSession(sid)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   ps.sessionCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// lookupClientByEmail ищет клиента по email во всех тенантах.
func (ps *PortalServer) lookupClientByEmail(ctx context.Context, email string) (*model.Client, error) {
	if ps.stores.ClientStore == nil {
		return nil, fmt.Errorf("ClientStore не настроен")
	}
	// Пустой tenantID → все клиенты; сопоставляем по email без учёта регистра.
	clients, err := ps.stores.ClientStore.ListClients(ctx, "", model.ResidencyStage(""))
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	for _, c := range clients {
		if strings.ToLower(strings.TrimSpace(c.ContactEmail)) == target {
			return c, nil
		}
	}
	return nil, fmt.Errorf("клиент с email %q не найден", email)
}

// ---------------------------------------------------------------------------
// Handlers — дашборд
// ---------------------------------------------------------------------------

func (ps *PortalServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	client, err := ps.getClient(r.Context(), sess.ClientID)
	if err != nil {
		http.Error(w, "Клиент не найден", http.StatusNotFound)
		return
	}

	data := dashboardData{
		Client:              client,
		Deadlines:           ps.getDeadlines(r.Context(), client.ID),
		Checklists:          ps.getClientChecklists(r.Context(), client.ID),
		Documents:           ps.getClientDocuments(r.Context(), client.ID),
		Flash:               r.URL.Query().Get("msg"),
		RecentChanges:       ps.getRecentChanges(r.Context()),
		TelegramBotUsername: ps.config.TelegramBotUsername,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "dashboard", data); err != nil {
		log.Println("[portal] шаблон dashboard:", err)
	}
}

// ---------------------------------------------------------------------------
// Handlers — чек-листы
// ---------------------------------------------------------------------------

func (ps *PortalServer) handleChecklists(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	data := checklistsData{
		Client:     ps.mustGetClient(r.Context(), sess.ClientID),
		Checklists: ps.getClientChecklists(r.Context(), sess.ClientID),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "checklists", data); err != nil {
		log.Println("[portal] шаблон checklists:", err)
	}
}

// ---------------------------------------------------------------------------
// Handlers — дедлайны
// ---------------------------------------------------------------------------

func (ps *PortalServer) handleDeadlines(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	data := deadlinesData{
		Client:    ps.mustGetClient(r.Context(), sess.ClientID),
		Deadlines: ps.getDeadlines(r.Context(), sess.ClientID),
		Overdue:   ps.getOverdueDeadlines(r.Context()),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "deadlines", data); err != nil {
		log.Println("[portal] шаблон deadlines:", err)
	}
}

// ---------------------------------------------------------------------------
// Handlers — документы
// ---------------------------------------------------------------------------

func (ps *PortalServer) handleDocuments(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	clientDocs := ps.getClientDocuments(r.Context(), sess.ClientID)
	docs := ps.enrichClientDocuments(r.Context(), clientDocs)

	data := documentsData{
		Client:    ps.mustGetClient(r.Context(), sess.ClientID),
		Documents: docs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "documents", data); err != nil {
		log.Println("[portal] шаблон documents:", err)
	}
}

// ---------------------------------------------------------------------------
// Handlers — генерация документов
// ---------------------------------------------------------------------------

func (ps *PortalServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)

	var templates []*model.DocumentTemplate
	if ps.stores.Generator != nil {
		// Источник истины — файловые шаблоны генератора.
		if list, err := ps.stores.Generator.ListTemplateInfos(r.Context()); err == nil {
			for i := range list {
				t := list[i]
				templates = append(templates, &t)
			}
		}
	} else if ps.stores.TemplateStore != nil {
		templates, _ = ps.stores.TemplateStore.ListTemplates(r.Context(), "")
	}

	data := generateData{
		Client:    ps.mustGetClient(r.Context(), sess.ClientID),
		Templates: templates,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "generate", data); err != nil {
		log.Println("[portal] шаблон generate:", err)
	}
}

func (ps *PortalServer) handleGenerateSubmit(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	templateID := r.FormValue("template_id")

	if templateID == "" {
		http.Redirect(w, r, "/generate?msg=Выберите+шаблон&kind=err", http.StatusSeeOther)
		return
	}

	if ps.stores.Generator == nil {
		http.Redirect(w, r, "/generate?msg=Генерация+документов+не+настроена&kind=err", http.StatusSeeOther)
		return
	}

	// Рендерим документ на основе данных профиля клиента.
	outPath, err := ps.stores.Generator.RenderTemplate(r.Context(), templateID, sess.ClientID, nil)
	if err != nil {
		http.Redirect(w, r, "/generate?msg="+url.QueryEscape("Ошибка генерации: "+err.Error())+"&kind=err", http.StatusSeeOther)
		return
	}

	// Перенаправляем на скачивание готового файла.
	http.Redirect(w, r, "/download?file="+url.QueryEscape(filepath.Base(outPath)), http.StatusSeeOther)
}

// handleDownload отдаёт сгенерированный документ из выходной директории генератора.
func (ps *PortalServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	if ps.stores.Generator == nil {
		http.Error(w, "генерация документов не настроена", http.StatusNotFound)
		return
	}
	// filepath.Base отсекает любые попытки выхода за пределы директории.
	name := filepath.Base(r.URL.Query().Get("file"))
	if name == "." || name == "/" || name == "" {
		http.Error(w, "некорректное имя файла", http.StatusBadRequest)
		return
	}
	full := filepath.Join(ps.stores.Generator.OutputDir(), name)
	if _, err := os.Stat(full); err != nil {
		http.Error(w, "файл не найден", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeFile(w, r, full)
}

// handleDocumentFile отдаёт оригинальный файл документа из реестра по его id.
// Документы реестра — публичные материалы Сколково; доступ только авторизованному клиенту.
func (ps *PortalServer) handleDocumentFile(w http.ResponseWriter, r *http.Request) {
	if ps.stores.DocumentStore == nil {
		http.Error(w, "реестр документов недоступен", http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "не указан идентификатор документа", http.StatusBadRequest)
		return
	}
	doc, err := ps.stores.DocumentStore.Get(r.Context(), id)
	if err != nil || doc.LocalPath == "" {
		http.Error(w, "файл документа не найден", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(doc.LocalPath)+"\"")
	http.ServeFile(w, r, doc.LocalPath)
}

// ---------------------------------------------------------------------------
// API Handlers
// ---------------------------------------------------------------------------

func (ps *PortalServer) apiMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	client, err := ps.getClient(r.Context(), sess.ClientID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "клиент не найден")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(client)
}

func (ps *PortalServer) apiChecklists(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	checklists := ps.getClientChecklists(r.Context(), sess.ClientID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(checklists)
}

func (ps *PortalServer) apiDeadlines(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	deadlines := ps.getDeadlines(r.Context(), sess.ClientID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(deadlines)
}

func (ps *PortalServer) apiDocuments(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	clientDocs := ps.getClientDocuments(r.Context(), sess.ClientID)
	docs := ps.enrichClientDocuments(r.Context(), clientDocs)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(docs)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (ps *PortalServer) getClient(ctx context.Context, id string) (*model.Client, error) {
	return ps.stores.ClientStore.GetClient(ctx, id)
}

func (ps *PortalServer) mustGetClient(ctx context.Context, id string) *model.Client {
	c, err := ps.stores.ClientStore.GetClient(ctx, id)
	if err != nil {
		return &model.Client{ID: id, Name: "Загрузка..."}
	}
	return c
}

func (ps *PortalServer) getDeadlines(ctx context.Context, clientID string) []*model.Deadline {
	if ps.stores.DeadlineStore == nil {
		return nil
	}
	d, _ := ps.stores.DeadlineStore.ListDeadlines(ctx, clientID, 30)
	return d
}

func (ps *PortalServer) getOverdueDeadlines(ctx context.Context) []*model.Deadline {
	if ps.stores.DeadlineStore == nil {
		return nil
	}
	d, _ := ps.stores.DeadlineStore.ListOverdueDeadlines(ctx)
	return d
}

func (ps *PortalServer) getClientChecklists(ctx context.Context, clientID string) []*model.ClientChecklist {
	if ps.stores.ChecklistStore == nil {
		return nil
	}
	cc, _ := ps.stores.ChecklistStore.GetClientChecklists(ctx, clientID)
	return cc
}

func (ps *PortalServer) getClientDocuments(ctx context.Context, clientID string) []*model.ClientDocument {
	if ps.stores.DocStore == nil {
		return nil
	}
	d, _ := ps.stores.DocStore.ListClientDocuments(ctx, clientID)
	return d
}

// enrichClientDocuments обогащает ClientDocument данными из реестра документов
// (название, source_url, статус) для отображения в портале.
func (ps *PortalServer) enrichClientDocuments(ctx context.Context, clientDocs []*model.ClientDocument) []*portalDocInfo {
	if len(clientDocs) == 0 {
		return nil
	}
	result := make([]*portalDocInfo, 0, len(clientDocs))
	for _, cd := range clientDocs {
		info := &portalDocInfo{
			ID:   cd.ID,
			Role: string(cd.Role),
		}
		// Пытаемся получить данные из основного реестра документов.
		if ps.stores.DocumentStore != nil && cd.DocumentID != "" {
			if doc, err := ps.stores.DocumentStore.Get(ctx, cd.DocumentID); err == nil {
				info.Name = doc.Title
				info.SourceURL = doc.SourceURL
				if doc.PublishedAt != nil {
					info.Date = doc.PublishedAt.Format("02.01.2006")
				}
			}
		}
		// Fallback: если не нашли в реестре — используем базовые поля.
		if info.Name == "" {
			info.Name = "Документ " + cd.DocumentID
		}
		info.Status = string(cd.Status)
		info.StatusClass = docStatusClass(cd)
		info.StatusLabel = docStatusLabel(cd)
		result = append(result, info)
	}
	return result
}

// getRecentChanges возвращает последние изменения базы знаний для портала.
func (ps *PortalServer) getRecentChanges(ctx context.Context) []recentChange {
	if ps.stores.ChangeStore == nil {
		return nil
	}
	events, err := ps.stores.ChangeStore.Recent(ctx, changes.Filter{
		Since: time.Now().AddDate(0, 0, -7), // Последние 7 дней
		Limit: 10,
	})
	if err != nil {
		return nil
	}
	result := make([]recentChange, 0, len(events))
	for _, ev := range events {
		result = append(result, recentChange{
			Title:      ev.Title,
			Kind:       string(ev.Kind),
			EntityType: ev.EntityType,
			DetectedAt: humanTime(ev.DetectedAt),
			Summary:    ev.Summary,
		})
	}
	return result
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	return strings.ReplaceAll(name, " ", "_")
}

func stageProgress(stage model.ResidencyStage) int {
	return model.StageProgress(stage)
}

func stageLabel(stage model.ResidencyStage) string {
	return model.StageLabel(stage)
}

func deadlineStatusClass(d *model.Deadline) string {
	now := time.Now()
	if d.Status == model.DeadlineCompleted {
		return "completed"
	}
	if d.IsOverdue(now) {
		return "overdue"
	}
	return "upcoming"
}

func docStatusLabel(d *model.ClientDocument) string {
	return model.DocStatusLabel(d.Status)
}

func docStatusClass(d *model.ClientDocument) string {
	classes := map[model.ClientDocStatus]string{
		model.DocPending:   "pending",
		model.DocSubmitted: "submitted",
		model.DocApproved:  "approved",
		model.DocRejected:  "rejected",
	}
	if c, ok := classes[d.Status]; ok {
		return c
	}
	return "pending"
}

func humanTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	if diff < time.Minute {
		return "только что"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%d мин. назад", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%d ч. назад", int(diff.Hours()))
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

// filename — извлекает имя файла из полного пути.
func filename(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
