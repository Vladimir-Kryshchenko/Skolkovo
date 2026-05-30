// Package admin реализует веб-админку: классификация, валидация, версионирование
// и контроль статусов документов с триггером (пере)индексации в RAG.
package admin

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/analytics"
	"baza-skolkovo/src/collector"
	"baza-skolkovo/src/common/extract"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/diff"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/scheduler"
)

// Server — HTTP-админка.
type Server struct {
	store       store.Store
	linkStore   store.DocumentLinkStore
	rag         *rag.Service
	schedStore  *scheduler.Store
	reportStore *scheduler.ReportStore
	aiStore     *aimodels.Store // ИИ-модели и агенты (опционально, требует Postgres)
	addr        string
	user        string
	pass        string
	docsDir     string
	chromePath  string
	proxyURL    string
	fetchWait   time.Duration
	sourceURL   string
}

// New создаёт админку.
func New(addr, user, pass, docsDir, chromePath, proxyURL, sourceURL string,
	fetchWait time.Duration, st store.Store, ragSvc *rag.Service) *Server {
	return &Server{
		store: st, rag: ragSvc, addr: addr, user: user, pass: pass, docsDir: docsDir,
		chromePath: chromePath, proxyURL: proxyURL, fetchWait: fetchWait, sourceURL: sourceURL,
	}
}

// WithLinkStore устанавливает хранилище связей документов.
func (s *Server) WithLinkStore(ls store.DocumentLinkStore) *Server {
	s.linkStore = ls
	return s
}

// docView — строка таблицы для шаблона.
type docView struct {
	model.Document
	StatusStr string
	FileSize  string // человекочитаемый размер файла
	FileAge   string // время загрузки ("2 часа назад", "3 дня назад")
}

// stats — сводка по реестру.
type stats struct {
	Total, Active, Pending, Outdated, Archived, Rejected, Indexed int
}

type pageData struct {
	Docs         []docView
	Stats        stats
	Query        string
	Flash        string
	FlashKind    string
	FilterStatus string
	Tab          string
	Settings     model.SchedulerSettings
	Reports      []model.CollectorReport
	Validation   *model.ValidationReport
	NextRunStr   string
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
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /stats", s.handleStats)
	mux.HandleFunc("POST /documents/{id}/status", s.handleStatus)
	mux.HandleFunc("POST /documents/{id}/category", s.handleCategory)
	mux.HandleFunc("POST /documents/{id}/supersedes", s.handleSupersedes)
	mux.HandleFunc("POST /documents/{id}/upload", s.handleUpload)
	mux.HandleFunc("POST /documents/{id}/delete", s.handleDelete)
	mux.HandleFunc("GET /documents/{id}/view-original", s.handleViewOriginal)
	mux.HandleFunc("GET /documents/{id}/view-processed", s.handleViewProcessed)
	mux.HandleFunc("GET /documents/{id}/download", s.handleDownload)
	mux.HandleFunc("POST /documents/{id}/deindex", s.handleDeindex)

	// Старые API (обратная совместимость)
	mux.HandleFunc("POST /api/scrape", s.handleAPIScrape)
	mux.HandleFunc("POST /api/index", s.handleAPIIndex)
	mux.HandleFunc("POST /api/sync", s.handleAPISync)

	// API для коллектора (полный цикл)
	mux.HandleFunc("POST /api/collect", s.handleAPICollect)
	mux.HandleFunc("POST /api/validate", s.handleAPIValidate)

	// API для планировщика
	mux.HandleFunc("GET /api/settings", s.handleAPISettings)
	mux.HandleFunc("POST /api/settings", s.handleAPISettingsUpdate)
	mux.HandleFunc("GET /api/reports", s.handleAPIReports)

	// Diff — сравнение версий документов
	mux.HandleFunc("GET /diff", s.handleDiffPage)
	mux.HandleFunc("POST /diff", s.handleDiffCompare)
	mux.HandleFunc("GET /api/diff/{id1}/{id2}", s.handleAPIDiff)

	// Аналитика
	mux.HandleFunc("GET /analytics", s.handleAnalyticsPage)
	mux.HandleFunc("GET /api/analytics", s.handleAPIAnalytics)
	mux.HandleFunc("GET /api/analytics/export", s.handleAnalyticsExport)

	// Граф связей документов
	mux.HandleFunc("GET /graph", s.handleGraphPage)
	mux.HandleFunc("GET /api/graph/{document_id}", s.handleAPIGraphDoc)
	mux.HandleFunc("POST /api/graph", s.handleAPIGraphCreateLink)
	mux.HandleFunc("DELETE /api/graph/{link_id}", s.handleAPIGraphDeleteLink)

	// ИИ Конфигурация — модели и агенты
	mux.HandleFunc("GET /ai/models", s.handleAIModelsPage)
	mux.HandleFunc("GET /ai/models/new", s.handleAIModelNew)
	mux.HandleFunc("POST /ai/models/create", s.handleAIModelCreate)
	mux.HandleFunc("GET /ai/models/{id}/edit", s.handleAIModelEdit)
	mux.HandleFunc("POST /ai/models/{id}/update", s.handleAIModelUpdate)
	mux.HandleFunc("POST /api/ai/models/{id}/delete", s.handleAIModelDelete)
	mux.HandleFunc("POST /api/ai/models/{id}/test", s.handleAIModelTest)
	mux.HandleFunc("POST /api/ai/models/seed-qwen", s.handleAISeedQwen)

	mux.HandleFunc("GET /ai/agents", s.handleAIAgentsPage)
	mux.HandleFunc("GET /ai/agents/new", s.handleAIAgentNew)
	mux.HandleFunc("POST /ai/agents/create", s.handleAIAgentCreate)
	mux.HandleFunc("GET /ai/agents/{id}/edit", s.handleAIAgentEdit)
	mux.HandleFunc("POST /ai/agents/{id}/update", s.handleAIAgentUpdate)
	mux.HandleFunc("POST /api/ai/agents/{id}/delete", s.handleAIAgentDelete)
	mux.HandleFunc("POST /api/ai/agents/{id}/test", s.handleAIAgentTest)

	log.Printf("[admin] админка слушает %s (вкладки: документы, сбор, планировщик, ИИ)", s.addr)
	return http.ListenAndServe(s.addr, s.auth(mux))
}

// auth — middleware HTTP Basic Auth (если заданы логин/пароль).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.user == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(s.pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Baza Skolkovo Admin"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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

	var docs []docView
	if tab == "documents" {
		allDocs, err := s.store.List(r.Context(), store.Filter{Status: status})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		docs = make([]docView, 0, len(allDocs))
		for _, d := range allDocs {
			if query != "" && !strings.Contains(strings.ToLower(d.Title), strings.ToLower(query)) {
				continue
			}
			v := docView{Document: d, StatusStr: string(d.Status)}
			if d.LocalPath != "" {
				v.FileSize = formatFileSize(d.LocalPath)
				v.FileAge = humanTimeAgo(d.FetchedAt)
			}
			docs = append(docs, v)
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
		Docs:         docs,
		Stats:        st,
		Query:        query,
		Flash:        r.URL.Query().Get("msg"),
		FlashKind:    orDefault(r.URL.Query().Get("kind"), "ok"),
		FilterStatus: string(status),
		Tab:          tab,
		Settings:     settings,
		Reports:      reports,
		Validation:   valRep,
		NextRunStr:   nextRunStr,
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
	log.Println("[admin/api] запуск парсинга RSS")
	go func() {
		// Здесь можно вызвать scraper если он доступен
		log.Println("[admin/api] парсинг RSS запущен (заглушка — используйте CLI для полного парсинга)")
	}()
	jsonResp(w, true, "Парсинг RSS запущен в фоне. Обновите страницу через минуту.", "")
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

// handleAPISync запускает полный цикл (заглушка — CLI делает основную работу).
func (s *Server) handleAPISync(w http.ResponseWriter, r *http.Request) {
	log.Println("[admin/api] запуск полного синка")
	go func() {
		log.Println("[admin/api] полный синк запущен (заглушка — используйте skolkovo sync)")
	}()
	jsonResp(w, true, "Полный синк запущен в фоне.", "")
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
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PDF — %s</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
<style>
body { font-family: 'Inter', sans-serif; background: #f8fafc; color: #1e293b; margin: 0; padding: 24px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 2px solid #e2e8f0; }
.header h1 { font-size: 18px; margin: 0; }
.btn { padding: 8px 16px; background: #1e40af; color: #fff; border: none; border-radius: 6px; cursor: pointer; text-decoration: none; font-size: 13px; }
.btn:hover { background: #1e3a8a; }
iframe { width: 100%%; height: calc(100vh - 120px); border: 1px solid #e2e8f0; border-radius: 8px; }
.meta { font-size: 12px; color: #64748b; margin-top: 12px; }
</style>
</head>
<body>
<div class="header">
  <h1>📄 PDF: %s</h1>
  <div>
    <a href="/documents/%s/download" class="btn">⬇️ Скачать</a>
    <a href="javascript:window.close()" class="btn">✕ Закрыть</a>
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
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Исходный документ — %s</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
<style>
body { font-family: 'Inter', sans-serif; background: #f8fafc; color: #1e293b; margin: 0; padding: 24px; line-height: 1.6; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 2px solid #e2e8f0; }
.header h1 { font-size: 18px; margin: 0; }
.btn { padding: 8px 16px; background: #1e40af; color: #fff; border: none; border-radius: 6px; cursor: pointer; text-decoration: none; font-size: 13px; }
.btn:hover { background: #1e3a8a; }
.content { background: #fff; padding: 20px; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,.1); white-space: pre-wrap; word-wrap: break-word; font-size: 14px; max-height: 80vh; overflow-y: auto; }
.meta { font-size: 12px; color: #64748b; margin-top: 12px; }
.truncated { background: #fef3c7; padding: 8px 12px; border-radius: 6px; margin-bottom: 12px; font-size: 13px; color: #92400e; }
</style>
</head>
<body>
<div class="header">
  <h1>📄 Исходный документ: %s</h1>
  <div>
    <a href="/documents/%s/download" class="btn">⬇️ Скачать</a>
    <a href="javascript:window.close()" class="btn">✕ Закрыть</a>
  </div>
</div>
`, doc.Title, doc.Title, doc.ID)

	if truncated {
		fmt.Fprintf(w, `<div class="truncated">⚠️ Показаны первые %d символов из %d. <a href="/documents/%s/download">Скачайте файл</a> для просмотра целиком.</div>`, maxLen, len(text), doc.ID)
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
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Обработанный документ — %s</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
<style>
body { font-family: 'Inter', sans-serif; background: #f8fafc; color: #1e293b; margin: 0; padding: 24px; line-height: 1.6; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 12px; border-bottom: 2px solid #e2e8f0; }
.header h1 { font-size: 18px; margin: 0; }
.btn { padding: 8px 16px; background: #1e40af; color: #fff; border: none; border-radius: 6px; cursor: pointer; text-decoration: none; font-size: 13px; }
.btn:hover { background: #1e3a8a; }
.chunk { background: #fff; padding: 16px; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,.1); margin-bottom: 12px; }
.chunk-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; padding-bottom: 8px; border-bottom: 1px solid #e2e8f0; }
.chunk-num { font-weight: 600; color: #1e40af; font-size: 13px; }
.chunk-len { font-size: 11px; color: #64748b; }
.chunk-text { white-space: pre-wrap; word-wrap: break-word; font-size: 14px; }
.meta { font-size: 12px; color: #64748b; margin-top: 16px; padding-top: 12px; border-top: 1px solid #e2e8f0; }
.stats { display: flex; gap: 16px; margin-bottom: 16px; }
.stat { background: #eff6ff; padding: 12px 16px; border-radius: 8px; text-align: center; }
.stat .n { font-size: 24px; font-weight: 700; color: #1e40af; }
.stat .l { font-size: 11px; color: #64748b; text-transform: uppercase; margin-top: 4px; }
</style>
</head>
<body>
<div class="header">
  <h1>🧠 Обработанный документ: %s</h1>
  <a href="javascript:window.close()" class="btn">✕ Закрыть</a>
</div>

<div class="stats">
  <div class="stat"><div class="n">%d</div><div class="l">Чанков</div></div>
  <div class="stat"><div class="n">%d</div><div class="l">Всего символов</div></div>
</div>
`, doc.Title, doc.Title, len(chunks), s.totalChars(chunks))

	for i, chunk := range chunks {
		fmt.Fprintf(w, `<div class="chunk">
<div class="chunk-header">
  <span class="chunk-num">Чанк #%d</span>
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

type diffPageData struct {
	Docs     []model.Document
	Doc1ID   string
	Doc2ID   string
	DiffHTML string
	Error    string
}

// handleDiffPage показывает форму выбора двух документов для сравнения.
func (s *Server) handleDiffPage(w http.ResponseWriter, r *http.Request) {
	docs, _ := s.store.List(r.Context(), store.Filter{})
	sort.Slice(docs, func(i, j int) bool { return docs[i].Title < docs[j].Title })

	data := diffPageData{Docs: docs}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "diff-layout", data); err != nil {
		log.Println("[admin] diff шаблон:", err)
	}
}

// handleDiffCompare обрабатывает POST-форму сравнения.
func (s *Server) handleDiffCompare(w http.ResponseWriter, r *http.Request) {
	id1 := r.FormValue("doc1")
	id2 := r.FormValue("doc2")

	if id1 == "" || id2 == "" {
		http.Error(w, "Выберите два документа", http.StatusBadRequest)
		return
	}

	s.renderDiff(w, r, id1, id2)
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

// renderDiff загружает документы, сравнивает и показывает HTML-результат.
func (s *Server) renderDiff(w http.ResponseWriter, r *http.Request, id1, id2 string) {
	text1, _, err := s.extractDocText(r.Context(), id1)
	if err != nil {
		s.renderDiffPage(w, r, id1, id2, "", "Документ 1: "+err.Error())
		return
	}
	text2, _, err := s.extractDocText(r.Context(), id2)
	if err != nil {
		s.renderDiffPage(w, r, id1, id2, "", "Документ 2: "+err.Error())
		return
	}

	result := diff.CompareDocuments(text1, text2)
	htmlContent := diff.ToHTML(result)
	s.renderDiffPage(w, r, id1, id2, htmlContent, "")
}

// renderDiffPage рисует страницу diff с формой и результатом.
func (s *Server) renderDiffPage(w http.ResponseWriter, r *http.Request, id1, id2, diffHTML, errMsg string) {
	docs, _ := s.store.List(r.Context(), store.Filter{})
	sort.Slice(docs, func(i, j int) bool { return docs[i].Title < docs[j].Title })

	data := diffPageData{
		Docs:     docs,
		Doc1ID:   id1,
		Doc2ID:   id2,
		DiffHTML: diffHTML,
		Error:    errMsg,
	}
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
