// Package admin реализует веб-админку: классификация, валидация, версионирование
// и контроль статусов документов с триггером (пере)индексации в RAG.
package admin

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// Server — HTTP-админка.
type Server struct {
	store   store.Store
	rag     *rag.Service // может быть nil, тогда индексация пропускается
	addr    string
	user    string // basic-auth логин (пусто — без авторизации)
	pass    string
	docsDir string // корень Документы_Сколково для ручной загрузки файлов
}

// New создаёт админку. ragSvc может быть nil. user/pass пустые — без авторизации.
func New(addr, user, pass, docsDir string, st store.Store, ragSvc *rag.Service) *Server {
	return &Server{store: st, rag: ragSvc, addr: addr, user: user, pass: pass, docsDir: docsDir}
}

// docView — строка таблицы для шаблона.
type docView struct {
	model.Document
	StatusStr string
}

// stats — сводка по реестру.
type stats struct {
	Total, Active, Pending, Outdated, Archived, Rejected, Indexed int
}

type pageData struct {
	Docs      []docView
	Stats     stats
	Query     string
	Flash     string
	FlashKind string
}

// ListenAndServe запускает админку.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /stats", s.handleStats)
	mux.HandleFunc("POST /documents/{id}/status", s.handleStatus)
	mux.HandleFunc("POST /documents/{id}/category", s.handleCategory)
	mux.HandleFunc("POST /documents/{id}/supersedes", s.handleSupersedes)
	mux.HandleFunc("POST /documents/{id}/upload", s.handleUpload)
	mux.HandleFunc("POST /documents/{id}/delete", s.handleDelete)

	if s.user == "" {
		log.Println("[admin] ВНИМАНИЕ: ADMIN_USER не задан — админка без авторизации")
	}
	log.Printf("[admin] админка слушает %s", s.addr)
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
	status := model.Status(r.URL.Query().Get("status"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	docs, err := s.store.List(r.Context(), store.Filter{Status: status})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]docView, 0, len(docs))
	for _, d := range docs {
		if query != "" && !strings.Contains(strings.ToLower(d.Title), strings.ToLower(query)) {
			continue
		}
		views = append(views, docView{Document: d, StatusStr: string(d.Status)})
	}

	st, err := s.computeStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := pageData{
		Docs:      views,
		Stats:     st,
		Query:     query,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
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
