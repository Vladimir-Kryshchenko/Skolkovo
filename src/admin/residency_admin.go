// Package admin — разделы админки для системы резидентства.
package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// Stores — набор хранилищ для админки резидентства.
type Stores struct {
	ClientStore    store.ClientStore
	ChecklistStore store.ChecklistStore
	DeadlineStore  store.DeadlineStore
	TemplateStore  store.TemplateStore
	TenantStore    store.TenantStore
	EventStore     store.EventStore
	ContestStore   store.ContestStore
	DocumentStore  store.Store
}

// ResidencyServer — HTTP-админка для системы резидентства.
type ResidencyServer struct {
	stores Stores
	addr   string
	user   string
	pass   string
}

// NewResidency создаёт админку резидентства.
func NewResidency(addr, user, pass string, stores Stores) *ResidencyServer {
	return &ResidencyServer{
		stores: stores,
		addr:   addr,
		user:   user,
		pass:   pass,
	}
}

// RegisterResidencyRoutes регистрирует маршруты админки резидентства на переданном mux.
// Если mux nil, создаётся новый.
func RegisterResidencyRoutes(mux *http.ServeMux, stores Stores) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	s := &ResidencyServer{stores: stores}

	// Клиенты
	mux.HandleFunc("GET /clients", s.handleClientsList)
	mux.HandleFunc("GET /clients/{id}", s.handleClientCard)
	mux.HandleFunc("POST /clients/{id}/stage", s.handleClientStageTransition)

	// Чек-листы
	mux.HandleFunc("GET /checklists", s.handleChecklists)

	// Дедлайны
	mux.HandleFunc("GET /deadlines", s.handleDeadlines)

	// Шаблоны документов
	mux.HandleFunc("GET /templates", s.handleTemplates)

	// Тенанты
	mux.HandleFunc("GET /tenants", s.handleTenants)
	mux.HandleFunc("POST /tenants", s.handleTenantCreate)

	// Мероприятия (контроль парсинга)
	mux.HandleFunc("GET /events-admin", s.handleEventsAdmin)

	// Конкурсы (контроль парсинга)
	mux.HandleFunc("GET /contests-admin", s.handleContestsAdmin)

	// JSON API для MCP
	mux.HandleFunc("GET /api/clients", s.handleAPIClients)
	mux.HandleFunc("GET /api/clients/{id}", s.handleAPIClientDetail)
	mux.HandleFunc("POST /api/clients", s.handleAPIClientCreate)
	mux.HandleFunc("POST /api/clients/{id}/stage", s.handleAPIClientStageTransition)

	return mux
}

// auth — middleware HTTP Basic Auth.
func (s *ResidencyServer) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.user == "" {
			next.ServeHTTP(w, r)
			return
		}
		// Для простоты используем те же креды, что и основная админка
		next.ServeHTTP(w, r)
	})
}

// redirect — редирект с flash-сообщением.
func residencyRedirect(w http.ResponseWriter, r *http.Request, path, msg, kind string) {
	q := url.Values{}
	if msg != "" {
		q.Set("msg", msg)
		q.Set("kind", kind)
	}
	target := path
	if q.Encode() != "" {
		target += "?" + q.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// residencyJSONResp — JSON-ответ.
func residencyJSONResp(w http.ResponseWriter, ok bool, msg, errStr string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{"ok": ok}
	if msg != "" {
		resp["msg"] = msg
	}
	if errStr != "" {
		resp["error"] = errStr
	}
	if data != nil {
		resp["data"] = data
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// /clients — список клиентов с фильтрацией и поиском
// ---------------------------------------------------------------------------

type clientsPageData struct {
	Clients     []*model.Client
	Flash       string
	FlashKind   string
	FilterStage string
	SearchQuery string
	StageCounts map[model.ResidencyStage]int
	TotalCount  int
}

func (s *ResidencyServer) handleClientsList(w http.ResponseWriter, r *http.Request) {
	if s.stores.ClientStore == nil {
		http.Error(w, "ClientStore не настроен", http.StatusInternalServerError)
		return
	}

	filterStage := model.ResidencyStage(r.URL.Query().Get("stage"))
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))

	ctx := r.Context()

	// Получаем всех клиентов (без фильтрации по tenant — для админки показываем всех)
	// Для простоты собираем по стадиям
	allClients := make([]*model.Client, 0)
	stageCounts := make(map[model.ResidencyStage]int)

	stages := []model.ResidencyStage{
		model.StageApplication, model.StageExamination, model.StageDecision,
		model.StageContract, model.StageResident, model.StageReporting,
		model.StageExtension, model.StageExit,
	}

	for _, stage := range stages {
		clients, err := s.stores.ClientStore.ListClients(ctx, "", stage)
		if err != nil {
			log.Printf("[residency] list clients stage=%s: %v", stage, err)
			continue
		}
		stageCounts[stage] = len(clients)
		if filterStage == "" || filterStage == stage {
			allClients = append(allClients, clients...)
		}
	}

	// Поиск по ИНН или имени
	if searchQuery != "" {
		filtered := make([]*model.Client, 0, len(allClients))
		lq := strings.ToLower(searchQuery)
		for _, c := range allClients {
			if strings.Contains(strings.ToLower(c.Name), lq) || strings.Contains(c.INN, searchQuery) {
				filtered = append(filtered, c)
			}
		}
		allClients = filtered
	}

	// Сортировка по updatedAt (сначала новые)
	sort.Slice(allClients, func(i, j int) bool {
		return allClients[i].UpdatedAt.After(allClients[j].UpdatedAt)
	})

	data := clientsPageData{
		Clients:     allClients,
		Flash:       r.URL.Query().Get("msg"),
		FlashKind:   orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterStage: string(filterStage),
		SearchQuery: searchQuery,
		StageCounts: stageCounts,
		TotalCount:  len(allClients),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := residencyTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон:", err)
	}
}

// ---------------------------------------------------------------------------
// /clients/{id} — карточка клиента
// ---------------------------------------------------------------------------

type clientCardData struct {
	Client      *model.Client
	Transitions []*model.StageTransition
	Deadlines   []*model.Deadline
	Checklists  []*model.ClientChecklist
	Flash       string
	FlashKind   string
	StageLabels map[model.ResidencyStage]string
}

func (s *ResidencyServer) handleClientCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	client, err := s.stores.ClientStore.GetClient(ctx, id)
	if err != nil {
		http.Error(w, "Клиент не найден: "+err.Error(), http.StatusNotFound)
		return
	}

	transitions, _ := s.stores.ClientStore.GetStageHistory(ctx, id)
	var deadlines []*model.Deadline
	if s.stores.DeadlineStore != nil {
		deadlines, _ = s.stores.DeadlineStore.ListDeadlines(ctx, id, 90)
	}
	var checklists []*model.ClientChecklist
	if s.stores.ChecklistStore != nil {
		checklists, _ = s.stores.ChecklistStore.GetClientChecklists(ctx, id)
	}

	stageLabels := map[model.ResidencyStage]string{
		model.StageApplication: "Подача заявки",
		model.StageExamination: "Экспертиза",
		model.StageDecision:    "Решение",
		model.StageContract:    "Договор",
		model.StageResident:    "Резидент",
		model.StageReporting:   "Отчётность",
		model.StageExtension:   "Продление",
		model.StageExit:        "Выход",
	}

	data := clientCardData{
		Client:      client,
		Transitions: transitions,
		Deadlines:   deadlines,
		Checklists:  checklists,
		Flash:       r.URL.Query().Get("msg"),
		FlashKind:   orDefault(r.URL.Query().Get("kind"), "ok"),
		StageLabels: stageLabels,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := clientCardTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон карточки:", err)
	}
}

// ---------------------------------------------------------------------------
// /clients/{id}/stage — POST: смена стадии
// ---------------------------------------------------------------------------

func (s *ResidencyServer) handleClientStageTransition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	client, err := s.stores.ClientStore.GetClient(ctx, id)
	if err != nil {
		residencyRedirect(w, r, "/clients", "Клиент не найден", "err")
		return
	}

	toStage := model.ResidencyStage(r.FormValue("to_stage"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	if toStage == "" {
		residencyRedirect(w, r, "/clients/"+id, "Не указана целевая стадия", "err")
		return
	}

	if !client.ResidencyStage.CanTransition(toStage) {
		residencyRedirect(w, r, "/clients/"+id,
			fmt.Sprintf("Переход %s → %s недопустим", client.ResidencyStage, toStage), "err")
		return
	}

	transition := &model.StageTransition{
		ID:             generateUUID(),
		ClientID:       id,
		FromStage:      client.ResidencyStage,
		ToStage:        toStage,
		TransitionedAt: time.Now(),
		Notes:          notes,
	}
	if err := s.stores.ClientStore.AddStageTransition(ctx, transition); err != nil {
		residencyRedirect(w, r, "/clients/"+id, "Ошибка сохранения перехода: "+err.Error(), "err")
		return
	}

	client.ResidencyStage = toStage
	client.UpdatedAt = time.Now()
	if err := s.stores.ClientStore.UpdateClient(ctx, client); err != nil {
		residencyRedirect(w, r, "/clients/"+id, "Ошибка обновления клиента: "+err.Error(), "err")
		return
	}

	residencyRedirect(w, r, "/clients/"+id,
		fmt.Sprintf("Стадия изменена: %s → %s", transition.FromStage, transition.ToStage), "ok")
}

// ---------------------------------------------------------------------------
// /checklists — список чек-листов по типам процедур
// ---------------------------------------------------------------------------

type checklistsPageData struct {
	Checklists []*model.Checklist
	Flash      string
	FlashKind  string
	FilterType string
	TypeCounts map[model.ChecklistType]int
	TypeLabels map[model.ChecklistType]string
}

func (s *ResidencyServer) handleChecklists(w http.ResponseWriter, r *http.Request) {
	if s.stores.ChecklistStore == nil {
		http.Error(w, "ChecklistStore не настроен", http.StatusInternalServerError)
		return
	}

	filterType := model.ChecklistType(r.URL.Query().Get("type"))
	ctx := r.Context()

	typeCounts := make(map[model.ChecklistType]int)
	typeLabels := map[model.ChecklistType]string{
		model.ChecklistEntry:     "Вступление",
		model.ChecklistReporting: "Отчётность",
		model.ChecklistExtension: "Продление",
		model.ChecklistExit:      "Выход",
	}

	allChecklists := make([]*model.Checklist, 0)
	for _, ct := range []model.ChecklistType{
		model.ChecklistEntry, model.ChecklistReporting, model.ChecklistExtension, model.ChecklistExit,
	} {
		list, err := s.stores.ChecklistStore.ListChecklists(ctx, ct)
		if err != nil {
			log.Printf("[residency] list checklists type=%s: %v", ct, err)
			continue
		}
		typeCounts[ct] = len(list)
		if filterType == "" || filterType == ct {
			allChecklists = append(allChecklists, list...)
		}
	}

	sort.Slice(allChecklists, func(i, j int) bool {
		return allChecklists[i].CreatedAt.After(allChecklists[j].CreatedAt)
	})

	data := checklistsPageData{
		Checklists: allChecklists,
		Flash:      r.URL.Query().Get("msg"),
		FlashKind:  orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterType: string(filterType),
		TypeCounts: typeCounts,
		TypeLabels: typeLabels,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := checklistsTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон чек-листов:", err)
	}
}

// ---------------------------------------------------------------------------
// /deadlines — дашборд дедлайнов
// ---------------------------------------------------------------------------

type deadlinesPageData struct {
	Upcoming  []*model.Deadline
	Overdue   []*model.Deadline
	Completed []*model.Deadline
	Flash     string
	FlashKind string
	Now       time.Time
}

func (s *ResidencyServer) handleDeadlines(w http.ResponseWriter, r *http.Request) {
	if s.stores.DeadlineStore == nil {
		http.Error(w, "DeadlineStore не настроен", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	now := time.Now()

	overdue, err := s.stores.DeadlineStore.ListOverdueDeadlines(ctx)
	if err != nil {
		log.Printf("[residency] list overdue: %v", err)
	}

	upcoming, err := s.stores.DeadlineStore.ListDeadlines(ctx, "", 30)
	if err != nil {
		log.Printf("[residency] list upcoming: %v", err)
	}

	// Фильтруем completed из upcoming
	var upcomingFiltered []*model.Deadline
	for _, d := range upcoming {
		if d.Status == model.DeadlineCompleted {
			// Собираем completed отдельно
			continue
		}
		upcomingFiltered = append(upcomingFiltered, d)
	}

	// Сортируем upcoming по дате
	sort.Slice(upcomingFiltered, func(i, j int) bool {
		return upcomingFiltered[i].DueDate.Before(upcomingFiltered[j].DueDate)
	})

	// Сортируем overdue по дате (самые старые сверху)
	sort.Slice(overdue, func(i, j int) bool {
		return overdue[i].DueDate.Before(overdue[j].DueDate)
	})

	data := deadlinesPageData{
		Upcoming:  upcomingFiltered,
		Overdue:   overdue,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
		Now:       now,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := deadlinesTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон дедлайнов:", err)
	}
}

// ---------------------------------------------------------------------------
// /templates — управление шаблонами документов
// ---------------------------------------------------------------------------

type templatesPageData struct {
	Templates  []*model.DocumentTemplate
	Flash      string
	FlashKind  string
	FilterType string
}

func (s *ResidencyServer) handleTemplates(w http.ResponseWriter, r *http.Request) {
	if s.stores.TemplateStore == nil {
		http.Error(w, "TemplateStore не настроен", http.StatusInternalServerError)
		return
	}

	filterType := r.URL.Query().Get("type")
	ctx := r.Context()

	templates, err := s.stores.TemplateStore.ListTemplates(ctx, filterType)
	if err != nil {
		http.Error(w, "Ошибка загрузки шаблонов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(templates, func(i, j int) bool {
		return templates[i].CreatedAt.After(templates[j].CreatedAt)
	})

	data := templatesPageData{
		Templates:  templates,
		Flash:      r.URL.Query().Get("msg"),
		FlashKind:  orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterType: filterType,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templatesTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон templates:", err)
	}
}

// ---------------------------------------------------------------------------
// /tenants — управление тенантами
// ---------------------------------------------------------------------------

type tenantsPageData struct {
	Tenants   []*model.Tenant
	Flash     string
	FlashKind string
}

func (s *ResidencyServer) handleTenants(w http.ResponseWriter, r *http.Request) {
	if s.stores.TenantStore == nil {
		http.Error(w, "TenantStore не настроен", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	tenants, err := s.stores.TenantStore.ListTenants(ctx)
	if err != nil {
		http.Error(w, "Ошибка загрузки тенантов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(tenants, func(i, j int) bool {
		return tenants[i].CreatedAt.After(tenants[j].CreatedAt)
	})

	data := tenantsPageData{
		Tenants:   tenants,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tenantsTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон tenants:", err)
	}
}

func (s *ResidencyServer) handleTenantCreate(w http.ResponseWriter, r *http.Request) {
	if s.stores.TenantStore == nil {
		http.Error(w, "TenantStore не настроен", http.StatusInternalServerError)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	if name == "" || apiKey == "" {
		residencyRedirect(w, r, "/tenants", "Имя и API-ключ обязательны", "err")
		return
	}

	tenant := &model.Tenant{
		ID:        generateUUID(),
		Name:      name,
		APIKey:    apiKey,
		CreatedAt: time.Now(),
		Active:    true,
	}

	ctx := r.Context()
	if err := s.stores.TenantStore.CreateTenant(ctx, tenant); err != nil {
		residencyRedirect(w, r, "/tenants", "Ошибка создания тенанта: "+err.Error(), "err")
		return
	}

	residencyRedirect(w, r, "/tenants", "Тенант создан: "+name, "ok")
}

// ---------------------------------------------------------------------------
// /events-admin — список мероприятий
// ---------------------------------------------------------------------------

type eventsPageData struct {
	Events       []*model.Event
	Flash        string
	FlashKind    string
	FilterStatus string
	Upcoming     int
	Past         int
	Cancelled    int
}

func (s *ResidencyServer) handleEventsAdmin(w http.ResponseWriter, r *http.Request) {
	if s.stores.EventStore == nil {
		http.Error(w, "EventStore не настроен", http.StatusInternalServerError)
		return
	}

	filterStatus := model.EventStatus(r.URL.Query().Get("status"))
	ctx := r.Context()

	events, err := s.stores.EventStore.ListEvents(ctx, "", filterStatus, nil, nil)
	if err != nil {
		http.Error(w, "Ошибка загрузки мероприятий: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].EventDate.Before(events[j].EventDate)
	})

	var upcoming, past, cancelled int
	for _, e := range events {
		switch e.Status {
		case model.EventActive:
			if e.IsUpcoming(time.Now()) {
				upcoming++
			} else {
				past++
			}
		case model.EventPast:
			past++
		case model.EventCancelled:
			cancelled++
		}
	}

	data := eventsPageData{
		Events:       events,
		Flash:        r.URL.Query().Get("msg"),
		FlashKind:    orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterStatus: string(filterStatus),
		Upcoming:     upcoming,
		Past:         past,
		Cancelled:    cancelled,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := eventsTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон events:", err)
	}
}

// ---------------------------------------------------------------------------
// /contests-admin — список конкурсов
// ---------------------------------------------------------------------------

type contestsPageData struct {
	Contests     []*model.Contest
	Flash        string
	FlashKind    string
	FilterStatus string
	Active       int
	Closed       int
}

func (s *ResidencyServer) handleContestsAdmin(w http.ResponseWriter, r *http.Request) {
	if s.stores.ContestStore == nil {
		http.Error(w, "ContestStore не настроен", http.StatusInternalServerError)
		return
	}

	filterStatus := model.ContestStatus(r.URL.Query().Get("status"))
	ctx := r.Context()

	contests, err := s.stores.ContestStore.ListContests(ctx, "", filterStatus)
	if err != nil {
		http.Error(w, "Ошибка загрузки конкурсов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(contests, func(i, j int) bool {
		return contests[i].StartDate.Before(contests[j].StartDate)
	})

	var active, closed int
	for _, c := range contests {
		switch c.Status {
		case model.ContestActive:
			active++
		case model.ContestClosed:
			closed++
		}
	}

	data := contestsPageData{
		Contests:     contests,
		Flash:        r.URL.Query().Get("msg"),
		FlashKind:    orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterStatus: string(filterStatus),
		Active:       active,
		Closed:       closed,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := contestsTmpl.Execute(w, data); err != nil {
		log.Println("[residency] шаблон contests:", err)
	}
}

// ---------------------------------------------------------------------------
// /api/clients — JSON API для MCP
// ---------------------------------------------------------------------------

func (s *ResidencyServer) handleAPIClients(w http.ResponseWriter, r *http.Request) {
	if s.stores.ClientStore == nil {
		residencyJSONResp(w, false, "", "ClientStore не настроен", nil)
		return
	}

	ctx := r.Context()
	stage := model.ResidencyStage(r.URL.Query().Get("stage"))
	tenantID := r.URL.Query().Get("tenant_id")

	clients, err := s.stores.ClientStore.ListClients(ctx, tenantID, stage)
	if err != nil {
		residencyJSONResp(w, false, "", err.Error(), nil)
		return
	}

	residencyJSONResp(w, true, "", "", map[string]interface{}{
		"clients": clients,
		"total":   len(clients),
	})
}

func (s *ResidencyServer) handleAPIClientDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	client, err := s.stores.ClientStore.GetClient(ctx, id)
	if err != nil {
		residencyJSONResp(w, false, "Клиент не найден", err.Error(), nil)
		return
	}

	transitions, _ := s.stores.ClientStore.GetStageHistory(ctx, id)

	result := map[string]interface{}{
		"client":      client,
		"transitions": transitions,
	}

	residencyJSONResp(w, true, "", "", result)
}

func (s *ResidencyServer) handleAPIClientCreate(w http.ResponseWriter, r *http.Request) {
	if s.stores.ClientStore == nil {
		residencyJSONResp(w, false, "", "ClientStore не настроен", nil)
		return
	}

	var req struct {
		Name         string `json:"name"`
		INN          string `json:"inn"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
		TenantID     string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		residencyJSONResp(w, false, "", "Ошибка разбора JSON", nil)
		return
	}

	if req.Name == "" || req.INN == "" {
		residencyJSONResp(w, false, "", "Name и INN обязательны", nil)
		return
	}

	client := &model.Client{
		ID:             generateUUID(),
		Name:           req.Name,
		INN:            req.INN,
		ContactEmail:   req.ContactEmail,
		ContactPhone:   req.ContactPhone,
		ResidencyStage: model.StageApplication,
		TenantID:       req.TenantID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	ctx := r.Context()
	if err := s.stores.ClientStore.CreateClient(ctx, client); err != nil {
		residencyJSONResp(w, false, "", err.Error(), nil)
		return
	}

	residencyJSONResp(w, true, "Клиент создан", "", client)
}

func (s *ResidencyServer) handleAPIClientStageTransition(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()

	if s.stores.ClientStore == nil {
		residencyJSONResp(w, false, "", "ClientStore не настроен", nil)
		return
	}

	client, err := s.stores.ClientStore.GetClient(ctx, id)
	if err != nil {
		residencyJSONResp(w, false, "Клиент не найден", err.Error(), nil)
		return
	}

	var req struct {
		ToStage model.ResidencyStage `json:"to_stage"`
		Notes   string               `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		residencyJSONResp(w, false, "", "Ошибка разбора JSON", nil)
		return
	}

	if !client.ResidencyStage.CanTransition(req.ToStage) {
		residencyJSONResp(w, false, "",
			fmt.Sprintf("Переход %s → %s недопустим", client.ResidencyStage, req.ToStage), nil)
		return
	}

	transition := &model.StageTransition{
		ID:             generateUUID(),
		ClientID:       id,
		FromStage:      client.ResidencyStage,
		ToStage:        req.ToStage,
		TransitionedAt: time.Now(),
		Notes:          req.Notes,
	}
	if err := s.stores.ClientStore.AddStageTransition(ctx, transition); err != nil {
		residencyJSONResp(w, false, "", err.Error(), nil)
		return
	}

	client.ResidencyStage = req.ToStage
	client.UpdatedAt = time.Now()
	if err := s.stores.ClientStore.UpdateClient(ctx, client); err != nil {
		residencyJSONResp(w, false, "", err.Error(), nil)
		return
	}

	residencyJSONResp(w, true, "Стадия изменена", "", map[string]interface{}{
		"transition": transition,
		"client":     client,
	})
}

// ---------------------------------------------------------------------------
// Template helper functions
// ---------------------------------------------------------------------------

var residencyFuncs = template.FuncMap{
	"FormatStage":     formatStage,
	"StepsCount":      stepsCount,
	"VarsCount":       varsCount,
	"maskAPI":         maskAPI,
	"truncate":        truncateStr,
	"StatusBg":        eventStatusBg,
	"ContestStatusBg": contestStatusBg,
	"DaysSince":       daysSince,
	"DaysUntil":       daysUntil,
}

func formatStage(s model.ResidencyStage) string {
	labels := map[model.ResidencyStage]string{
		model.StageApplication: "Подача заявки",
		model.StageExamination: "Экспертиза",
		model.StageDecision:    "Решение",
		model.StageContract:    "Договор",
		model.StageResident:    "Резидент",
		model.StageReporting:   "Отчётность",
		model.StageExtension:   "Продление",
		model.StageExit:        "Выход",
	}
	if l, ok := labels[s]; ok {
		return l
	}
	return string(s)
}

func stepsCount(raw json.RawMessage) int {
	var steps []interface{}
	if raw == nil {
		return 0
	}
	if err := json.Unmarshal(raw, &steps); err != nil {
		return 0
	}
	return len(steps)
}

func varsCount(raw json.RawMessage) int {
	var vars []interface{}
	if raw == nil {
		return 0
	}
	if err := json.Unmarshal(raw, &vars); err != nil {
		return 0
	}
	return len(vars)
}

func maskAPI(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func truncateStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func eventStatusBg(s model.EventStatus) string {
	switch s {
	case model.EventActive:
		return "var(--green-bg)"
	case model.EventPast:
		return "var(--gray-bg)"
	case model.EventCancelled:
		return "var(--red-bg)"
	}
	return "var(--gray-bg)"
}

func contestStatusBg(s model.ContestStatus) string {
	switch s {
	case model.ContestActive:
		return "var(--green-bg)"
	case model.ContestClosed:
		return "var(--gray-bg)"
	case model.ContestWinnerSelected:
		return "var(--purple-bg)"
	}
	return "var(--gray-bg)"
}

func daysSince(due time.Time, now time.Time) int {
	if due.After(now) {
		return 0
	}
	return int(now.Sub(due).Hours() / 24)
}

func daysUntil(due time.Time, now time.Time) int {
	if due.Before(now) {
		return 0
	}
	return int(due.Sub(now).Hours() / 24)
}

// generateUUID — простая генерация UUID-подобного ID.
func generateUUID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ---------------------------------------------------------------------------
// Inline HTML templates — резидентство
// ---------------------------------------------------------------------------

// CSS variables — общие для всех шаблонов резидентства.
const residencyCSS = `
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f0f2f5; --surface: #fff; --primary: #1e40af; --primary-hover: #1e3a8a;
  --primary-light: #eff6ff; --text: #1e293b; --text-secondary: #64748b;
  --border: #e2e8f0; --radius: 8px; --shadow: 0 1px 3px rgba(0,0,0,.08);
  --shadow-lg: 0 10px 15px -3px rgba(0,0,0,.1);
  --green: #16a34a; --green-bg: #f0fdf4; --yellow: #ca8a04; --yellow-bg: #fefce8;
  --red: #dc2626; --red-bg: #fef2f2; --blue: #2563eb; --purple: #7c3aed; --purple-bg: #f5f3ff;
  --gray: #6b7280; --gray-bg: #f3f4f6; --orange: #ea580c; --orange-bg: #fff7ed;
}
body { font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; }
header { background: linear-gradient(135deg, var(--primary) 0%, #3b82f6 100%); color: #fff; padding: 16px 28px; display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; box-shadow: 0 2px 8px rgba(0,0,0,.15); position: sticky; top: 0; z-index: 100; }
header h1 { font-size: 18px; font-weight: 600; }
header a { color: #fff; text-decoration: none; padding: 7px 14px; border-radius: 6px; font-size: 13px; background: rgba(255,255,255,.15); border: 1px solid rgba(255,255,255,.25); transition: all .2s; }
header a:hover { background: rgba(255,255,255,.25); }
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 12px; margin-bottom: 20px; }
.stat { background: var(--surface); border-radius: var(--radius); padding: 14px 16px; box-shadow: var(--shadow); text-align: center; cursor: pointer; transition: transform .15s; }
.stat:hover { transform: translateY(-2px); box-shadow: var(--shadow-lg); }
.stat .n { font-size: 28px; font-weight: 700; }
.stat .l { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-top: 4px; font-weight: 500; }
.toolbar { background: var(--surface); border-radius: var(--radius); padding: 14px 18px; margin-bottom: 16px; box-shadow: var(--shadow); display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.toolbar label { font-size: 13px; color: var(--text-secondary); font-weight: 500; }
.filter-tabs { display: flex; gap: 4px; }
.filter-tab { padding: 5px 12px; border-radius: 20px; font-size: 12px; font-weight: 500; text-decoration: none; color: var(--text-secondary); transition: all .15s; border: 1px solid transparent; cursor: pointer; }
.filter-tab:hover { background: var(--primary-light); color: var(--primary); }
.filter-tab.active { background: var(--primary); color: #fff; border-color: var(--primary); }
.search-box { flex: 1; min-width: 180px; max-width: 360px; position: relative; }
.search-box input { width: 100%; padding: 7px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; }
.search-box input:focus { border-color: var(--primary); }
.table-wrap { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow); overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
thead th { background: #f8fafc; padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; border-bottom: 2px solid var(--border); }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; }
tbody tr:hover { background: #f8fafc; }
.badge { display: inline-block; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; }
.stage-подача_заявки { background: var(--gray-bg); color: var(--gray); }
.stage-экспертиза { background: var(--yellow-bg); color: var(--yellow); }
.stage-решение { background: var(--orange-bg); color: var(--orange); }
.stage-договор { background: var(--purple-bg); color: var(--purple); }
.stage-резидент { background: var(--green-bg); color: var(--green); }
.stage-отчётность { background: var(--blue); color: #fff; }
.stage-продление { background: var(--primary-light); color: var(--primary); }
.stage-выход { background: var(--red-bg); color: var(--red); }
.btn { display: inline-flex; align-items: center; gap: 4px; padding: 7px 14px; border: none; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; text-decoration: none; transition: all .15s; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-success { background: var(--green); color: #fff; }
.btn-danger { background: var(--red); color: #fff; }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--gray-bg); }
.btn-sm { padding: 4px 10px; font-size: 12px; }
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; }
.flash.ok { background: var(--green-bg); color: #15803d; border: 1px solid #bbf7d0; }
.flash.err { background: var(--red-bg); color: #b91c1c; border: 1px solid #fecaca; }
select, input[type=text], textarea { padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; font-family: inherit; }
select:focus, input[type=text]:focus, textarea:focus { border-color: var(--primary); }
textarea { resize: vertical; min-height: 60px; }
.card { background: var(--surface); border-radius: var(--radius); padding: 20px; box-shadow: var(--shadow); margin-bottom: 16px; }
.card h3 { font-size: 15px; margin-bottom: 12px; padding-bottom: 8px; border-bottom: 1px solid var(--border); }
.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.grid-3 { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
.meta { font-size: 12px; color: var(--text-secondary); }
.empty { text-align: center; padding: 48px 24px; color: var(--text-secondary); }
.empty .icon { font-size: 48px; margin-bottom: 12px; }
a.link { color: var(--blue); text-decoration: none; }
a.link:hover { text-decoration: underline; }
.deadline-overdue { border-left: 3px solid var(--red); }
.deadline-upcoming { border-left: 3px solid var(--yellow); }
.deadline-completed { border-left: 3px solid var(--green); }
.timeline { position: relative; padding-left: 24px; }
.timeline::before { content: ''; position: absolute; left: 8px; top: 0; bottom: 0; width: 2px; background: var(--border); }
.timeline-item { position: relative; margin-bottom: 16px; }
.timeline-item::before { content: ''; position: absolute; left: -20px; top: 4px; width: 12px; height: 12px; border-radius: 50%; background: var(--primary); border: 2px solid #fff; }
.form-group { margin-bottom: 12px; }
.form-group label { display: block; font-size: 12px; font-weight: 500; color: var(--text-secondary); margin-bottom: 4px; }
.form-group input, .form-group select, .form-group textarea { width: 100%; }
@media (max-width: 768px) {
  main { padding: 16px; }
  .stats { grid-template-columns: repeat(2, 1fr); }
  .grid-2, .grid-3 { grid-template-columns: 1fr; }
  .toolbar { flex-direction: column; }
}
`

// Шаблон списка клиентов.
var residencyTmpl = template.Must(template.New("residency-clients").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Клиенты — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>🏢 Клиенты</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat"><div class="n">{{.TotalCount}}</div><div class="l">Всего</div></div>
  {{range $stage, $count := .StageCounts}}
  <div class="stat"><div class="n">{{$count}}</div><div class="l">{{FormatStage $stage}}</div></div>
  {{end}}
</div>

<div class="toolbar">
  <label>Стадия:</label>
  <div class="filter-tabs">
    <a class="filter-tab{{if eq .FilterStage ""}} active{{end}}" href="/clients">Все</a>
    <a class="filter-tab{{if eq .FilterStage "подача_заявки"}} active{{end}}" href="/clients?stage=подача_заявки">Подача заявки</a>
    <a class="filter-tab{{if eq .FilterStage "экспертиза"}} active{{end}}" href="/clients?stage=экспертиза">Экспертиза</a>
    <a class="filter-tab{{if eq .FilterStage "решение"}} active{{end}}" href="/clients?stage=решение">Решение</a>
    <a class="filter-tab{{if eq .FilterStage "договор"}} active{{end}}" href="/clients?stage=договор">Договор</a>
    <a class="filter-tab{{if eq .FilterStage "резидент"}} active{{end}}" href="/clients?stage=резидент">Резидент</a>
    <a class="filter-tab{{if eq .FilterStage "отчётность"}} active{{end}}" href="/clients?stage=отчётность">Отчётность</a>
    <a class="filter-tab{{if eq .FilterStage "продление"}} active{{end}}" href="/clients?stage=продление">Продление</a>
    <a class="filter-tab{{if eq .FilterStage "выход"}} active{{end}}" href="/clients?stage=выход">Выход</a>
  </div>
  <div class="search-box">
    <form method="get" action="/clients">
      <input type="hidden" name="stage" value="{{.FilterStage}}">
      <input type="text" name="q" value="{{.SearchQuery}}" placeholder="Поиск по ИНН или имени…">
    </form>
  </div>
</div>

{{if .Clients}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th style="width:35%">Клиент</th>
      <th>ИНН</th>
      <th>Email</th>
      <th>Стадия</th>
      <th>Тенант</th>
      <th>Обновлён</th>
      <th>Действия</th>
    </tr>
  </thead>
  <tbody>
  {{range .Clients}}
  <tr>
    <td><strong>{{.Name}}</strong></td>
    <td><code style="background:var(--gray-bg);padding:2px 6px;border-radius:3px;font-size:12px">{{.INN}}</code></td>
    <td>{{.ContactEmail}}</td>
    <td><span class="badge stage-{{.ResidencyStage}}">{{FormatStage .ResidencyStage}}</span></td>
    <td>{{.TenantID}}</td>
    <td class="meta">{{.UpdatedAt.Format "02.01.2006 15:04"}}</td>
    <td><a href="/clients/{{.ID}}" class="btn btn-ghost btn-sm">📋 Карточка</a></td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">📭</div>
  <p><strong>Нет клиентов</strong></p>
  <p>Клиенты появятся после подачи заявки на резидентство</p>
</div>
{{end}}
</main>
</body>
</html>`))

// Шаблон карточки клиента.
var clientCardTmpl = template.Must(template.New("client-card").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Client.Name}} — Карточка клиента</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>👤 {{.Client.Name}}</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">← Назад к списку</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="grid-2">
  <div class="card">
    <h3>Основная информация</h3>
    <div class="grid-2">
      <div><div class="meta">ИНН</div><strong>{{.Client.INN}}</strong></div>
      <div><div class="meta">Стадия</div><span class="badge stage-{{.Client.ResidencyStage}}">{{index .StageLabels .Client.ResidencyStage}}</span></div>
      <div><div class="meta">Email</div>{{or .Client.ContactEmail "—"}}</div>
      <div><div class="meta">Телефон</div>{{or .Client.ContactPhone "—"}}</div>
      <div><div class="meta">Тенант</div>{{.Client.TenantID}}</div>
      <div><div class="meta">Создан</div>{{.Client.CreatedAt.Format "02.01.2006 15:04"}}</div>
    </div>
  </div>

  <div class="card">
    <h3>Сменить стадию</h3>
    <form method="POST" action="/clients/{{.Client.ID}}/stage">
      <div class="form-group">
        <label>Целевая стадия</label>
        <select name="to_stage">
          {{$current := .Client.ResidencyStage}}
          {{range $stage, $label := .StageLabels}}
            <option value="{{$stage}}" {{if eq $stage $current}}disabled{{end}}>{{$label}}</option>
          {{end}}
        </select>
      </div>
      <div class="form-group">
        <label>Примечание</label>
        <textarea name="notes" placeholder="Причина перехода, комментарий…"></textarea>
      </div>
      <button type="submit" class="btn btn-primary">Перевести</button>
    </form>
  </div>
</div>

<div class="card">
  <h3>📜 История переходов</h3>
  {{if .Transitions}}
  <div class="timeline">
    {{range .Transitions}}
    <div class="timeline-item">
      <div class="meta">{{.TransitionedAt.Format "02.01.2006 15:04"}}</div>
      <div><strong>{{FormatStage .FromStage}}</strong> → <strong>{{FormatStage .ToStage}}</strong></div>
      {{if .Notes}}<div class="meta" style="margin-top:4px">{{.Notes}}</div>{{end}}
    </div>
    {{end}}
  </div>
  {{else}}
  <div class="meta">История переходов пуста</div>
  {{end}}
</div>

<div class="grid-2">
  <div class="card">
    <h3>⏰ Дедлайны</h3>
    {{if .Deadlines}}
    {{range .Deadlines}}
    <div class="card deadline-{{.Status}}" style="margin-bottom:8px;padding:12px">
      <div><strong>{{.Title}}</strong></div>
      <div class="meta">Срок: {{.DueDate.Format "02.01.2006"}} | Статус: {{.Status}}</div>
    </div>
    {{end}}
    {{else}}
    <div class="meta">Нет дедлайнов</div>
    {{end}}
  </div>

  <div class="card">
    <h3>✅ Чек-листы</h3>
    {{if .Checklists}}
    {{range .Checklists}}
    <div class="card" style="margin-bottom:8px;padding:12px">
      <div><strong>{{.ID}}</strong></div>
      <div class="meta">Статус: {{.Status}} | Начат: {{if .StartedAt}}{{.StartedAt.Format "02.01.2006"}}{{else}}—{{end}}</div>
    </div>
    {{end}}
    {{else}}
    <div class="meta">Нет чек-листов</div>
    {{end}}
  </div>
</div>
</main>
</body>
</html>`))

// Шаблон чек-листов.
var checklistsTmpl = template.Must(template.New("checklists").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Чек-листы — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>✅ Чек-листы</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  {{range $t, $count := .TypeCounts}}
  <div class="stat"><div class="n">{{$count}}</div><div class="l">{{index $.TypeLabels $t}}</div></div>
  {{end}}
</div>

<div class="toolbar">
  <label>Тип процедуры:</label>
  <div class="filter-tabs">
    <a class="filter-tab{{if eq .FilterType ""}} active{{end}}" href="/checklists">Все</a>
    <a class="filter-tab{{if eq .FilterType "entry"}} active{{end}}" href="/checklists?type=entry">Вступление</a>
    <a class="filter-tab{{if eq .FilterType "reporting"}} active{{end}}" href="/checklists?type=reporting">Отчётность</a>
    <a class="filter-tab{{if eq .FilterType "extension"}} active{{end}}" href="/checklists?type=extension">Продление</a>
    <a class="filter-tab{{if eq .FilterType "exit"}} active{{end}}" href="/checklists?type=exit">Выход</a>
  </div>
</div>

{{if .Checklists}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th>Название</th>
      <th>Тип процедуры</th>
      <th>Версия</th>
      <th>Создан</th>
      <th>Шаги</th>
    </tr>
  </thead>
  <tbody>
  {{range .Checklists}}
  <tr>
    <td><strong>{{.Title}}</strong></td>
    <td><code style="background:var(--gray-bg);padding:2px 6px;border-radius:3px;font-size:12px">{{.ProcedureType}}</code></td>
    <td>{{.Version}}</td>
    <td class="meta">{{.CreatedAt.Format "02.01.2006"}}</td>
    <td class="meta">{{.Steps | StepsCount}} шагов</td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">📋</div>
  <p><strong>Нет чек-листов</strong></p>
  <p>Шаблоны чек-листов создаются через API или CLI</p>
</div>
{{end}}
</main>
</body>
</html>`))

// Шаблон дедлайнов.
var deadlinesTmpl = template.Must(template.New("deadlines").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Дедлайны — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>⏰ Дедлайны</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat" style="border-left:3px solid var(--red)"><div class="n" style="color:var(--red)">{{len .Overdue}}</div><div class="l">Просроченные</div></div>
  <div class="stat" style="border-left:3px solid var(--yellow)"><div class="n" style="color:var(--yellow)">{{len .Upcoming}}</div><div class="l">Ближайшие (30 дн.)</div></div>
</div>

{{if .Overdue}}
<div class="card">
  <h3 style="color:var(--red)">🔴 Просроченные дедлайны</h3>
  {{range .Overdue}}
  <div class="card deadline-overdue" style="margin-bottom:8px;padding:12px">
    <div><strong>{{.Title}}</strong></div>
    <div class="meta">Клиент: {{.ClientID}} | Срок: {{.DueDate.Format "02.01.2006"}} | Просрочено на {{DaysSince .DueDate $.Now}} дн.</div>
  </div>
  {{end}}
</div>
{{end}}

{{if .Upcoming}}
<div class="card">
  <h3>🟡 Ближайшие дедлайны</h3>
  {{range .Upcoming}}
  <div class="card deadline-upcoming" style="margin-bottom:8px;padding:12px">
    <div><strong>{{.Title}}</strong></div>
    <div class="meta">Клиент: {{.ClientID}} | Срок: {{.DueDate.Format "02.01.2006"}} | Осталось {{DaysUntil .DueDate $.Now}} дн.</div>
  </div>
  {{end}}
</div>
{{end}}

{{if and (not .Overdue) (not .Upcoming)}}
<div class="empty">
  <div class="icon">✅</div>
  <p><strong>Нет активных дедлайнов</strong></p>
  <p>Все дедлайны выполнены или ещё не назначены</p>
</div>
{{end}}
</main>
</body>
</html>`))

// Шаблон шаблонов документов.
var templatesTmpl = template.Must(template.New("templates").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Шаблоны документов — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>📄 Шаблоны документов</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

{{if .Templates}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th>Название</th>
      <th>Тип</th>
      <th>Файл</th>
      <th>Версия</th>
      <th>Переменные</th>
      <th>Создан</th>
    </tr>
  </thead>
  <tbody>
  {{range .Templates}}
  <tr>
    <td><strong>{{.Name}}</strong></td>
    <td><code style="background:var(--gray-bg);padding:2px 6px;border-radius:3px;font-size:12px">{{.Type}}</code></td>
    <td class="meta">{{.TemplateFile}}</td>
    <td>{{.Version}}</td>
    <td class="meta">{{.Variables | VarsCount}} переменных</td>
    <td class="meta">{{.CreatedAt.Format "02.01.2006"}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">📄</div>
  <p><strong>Нет шаблонов</strong></p>
  <p>Шаблоны документов создаются через API или CLI</p>
</div>
{{end}}
</main>
</body>
</html>`))

// Шаблон тенантов.
var tenantsTmpl = template.Must(template.New("tenants").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Тенанты — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>🏢 Тенанты</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="card">
  <h3>Создать тенант</h3>
  <form method="POST" action="/tenants">
    <div class="grid-2">
      <div class="form-group">
        <label>Название</label>
        <input type="text" name="name" placeholder="Название организации" required>
      </div>
      <div class="form-group">
        <label>API-ключ</label>
        <input type="text" name="api_key" placeholder="sk-xxxxxxxxxxxxxxxx" required>
      </div>
    </div>
    <button type="submit" class="btn btn-primary">Создать</button>
  </form>
</div>

{{if .Tenants}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th>Название</th>
      <th>API-ключ</th>
      <th>Активен</th>
      <th>Создан</th>
    </tr>
  </thead>
  <tbody>
  {{range .Tenants}}
  <tr>
    <td><strong>{{.Name}}</strong></td>
    <td><code style="background:var(--gray-bg);padding:2px 6px;border-radius:3px;font-size:12px">{{maskAPI .APIKey}}</code></td>
    <td>{{if .Active}}<span class="badge" style="background:var(--green-bg);color:var(--green)">Да</span>{{else}}<span class="badge" style="background:var(--gray-bg);color:var(--gray)">Нет</span>{{end}}</td>
    <td class="meta">{{.CreatedAt.Format "02.01.2006 15:04"}}</td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">🏢</div>
  <p><strong>Нет тенантов</strong></p>
  <p>Создайте первый тенант через форму выше</p>
</div>
{{end}}
</main>
</body>
</html>`))

// Шаблон мероприятий.
var eventsTmpl = template.Must(template.New("events-admin").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Мероприятия — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>📅 Мероприятия</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat" style="border-left:3px solid var(--green)"><div class="n" style="color:var(--green)">{{.Upcoming}}</div><div class="l">Предстоящие</div></div>
  <div class="stat" style="border-left:3px solid var(--gray)"><div class="n" style="color:var(--gray)">{{.Past}}</div><div class="l">Прошедшие</div></div>
  <div class="stat" style="border-left:3px solid var(--red)"><div class="n" style="color:var(--red)">{{.Cancelled}}</div><div class="l">Отменённые</div></div>
</div>

<div class="toolbar">
  <label>Статус:</label>
  <div class="filter-tabs">
    <a class="filter-tab{{if eq .FilterStatus ""}} active{{end}}" href="/events-admin">Все</a>
    <a class="filter-tab{{if eq .FilterStatus "active"}} active{{end}}" href="/events-admin?status=active">Активные</a>
    <a class="filter-tab{{if eq .FilterStatus "past"}} active{{end}}" href="/events-admin?status=past">Прошедшие</a>
    <a class="filter-tab{{if eq .FilterStatus "cancelled"}} active{{end}}" href="/events-admin?status=cancelled">Отменённые</a>
  </div>
</div>

{{if .Events}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th style="width:30%">Название</th>
      <th>Описание</th>
      <th>Дата начала</th>
      <th>Дата окончания</th>
      <th>Место</th>
      <th>Статус</th>
      <th>Источник</th>
    </tr>
  </thead>
  <tbody>
  {{range .Events}}
  <tr>
    <td><strong>{{.Title}}</strong></td>
    <td class="meta">{{truncate .Description 80}}</td>
    <td>{{.EventDate.Format "02.01.2006"}}</td>
    <td>{{if not .EventEndDate.IsZero}}{{.EventEndDate.Format "02.01.2006"}}{{else}}—{{end}}</td>
    <td class="meta">{{or .Location "—"}}</td>
    <td><span class="badge" style="background:{{StatusBg .Status}}">{{.Status}}</span></td>
    <td class="meta"><a href="{{.SourceURL}}" target="_blank" class="link" style="font-size:12px">ссылка</a></td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">📅</div>
  <p><strong>Нет мероприятий</strong></p>
  <p>Мероприятия добавляются через парсинг или API</p>
</div>
{{end}}
</main>
</body>
</html>`))

// Шаблон конкурсов.
var contestsTmpl = template.Must(template.New("contests-admin").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Конкурсы — Резидентство</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
</head>
<body>
<header>
  <h1>🏆 Конкурсы</h1>
  <div style="display:flex;gap:8px">
    <a href="/clients">Клиенты</a>
    <a href="/checklists">Чек-листы</a>
    <a href="/deadlines">Дедлайны</a>
    <a href="/templates">Шаблоны</a>
    <a href="/tenants">Тенанты</a>
    <a href="/events-admin">Мероприятия</a>
    <a href="/contests-admin">Конкурсы</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat" style="border-left:3px solid var(--green)"><div class="n" style="color:var(--green)">{{.Active}}</div><div class="l">Активные</div></div>
  <div class="stat" style="border-left:3px solid var(--gray)"><div class="n" style="color:var(--gray)">{{.Closed}}</div><div class="l">Закрытые</div></div>
</div>

<div class="toolbar">
  <label>Статус:</label>
  <div class="filter-tabs">
    <a class="filter-tab{{if eq .FilterStatus ""}} active{{end}}" href="/contests-admin">Все</a>
    <a class="filter-tab{{if eq .FilterStatus "active"}} active{{end}}" href="/contests-admin?status=active">Активные</a>
    <a class="filter-tab{{if eq .FilterStatus "closed"}} active{{end}}" href="/contests-admin?status=closed">Закрытые</a>
    <a class="filter-tab{{if eq .FilterStatus "winner_selected"}} active{{end}}" href="/contests-admin?status=winner_selected">Определён победитель</a>
  </div>
</div>

{{if .Contests}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th style="width:25%">Название</th>
      <th>Описание</th>
      <th>Дата начала</th>
      <th>Дата окончания</th>
      <th>Приз</th>
      <th>Статус</th>
      <th>Источник</th>
    </tr>
  </thead>
  <tbody>
  {{range .Contests}}
  <tr>
    <td><strong>{{.Title}}</strong></td>
    <td class="meta">{{truncate .Description 80}}</td>
    <td>{{.StartDate.Format "02.01.2006"}}</td>
    <td>{{.EndDate.Format "02.01.2006"}}</td>
    <td class="meta">{{or .Prize "—"}}</td>
    <td><span class="badge" style="background:{{ContestStatusBg .Status}}">{{.Status}}</span></td>
    <td class="meta"><a href="{{.SourceURL}}" target="_blank" class="link" style="font-size:12px">ссылка</a></td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">🏆</div>
  <p><strong>Нет конкурсов</strong></p>
  <p>Конкурсы добавляются через парсинг или API</p>
</div>
{{end}}
</main>
</body>
</html>`))
