// Package mcpserver поднимает открытый MCP-сервер (Streamable HTTP) над RAG-базой.
//
// Инструменты:
//   - search_documents       — семантический поиск по действующим документам;
//   - get_document            — метаданные документа по id;
//   - list_active_documents   — перечень действующих документов (опц. по категории).
//
// Доступ открытый, но защищён API-ключом и rate-limit.
package mcpserver

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// Server — HTTP-обёртка над MCP с авторизацией и лимитом запросов.
type Server struct {
	rag      *rag.Service
	store    store.Store
	apiKey   string
	limiters *limiterSet
	addr     string
	mcpSrv   *server.MCPServer // кэшированный MCPServer (ленивая инициализация)
}

// New создаёт MCP-сервер.
func New(addr, apiKey string, rateRPS int, ragSvc *rag.Service, st store.Store) *Server {
	return &Server{
		rag:      ragSvc,
		store:    st,
		apiKey:   apiKey,
		limiters: newLimiterSet(rateRPS),
		addr:     addr,
	}
}

// buildMCP регистрирует базовые инструменты MCP и возвращает MCPServer.
// Лениво создаёт и кэширует экземпляр.
func (s *Server) buildMCP() *server.MCPServer {
	if s.mcpSrv != nil {
		return s.mcpSrv
	}
	m := server.NewMCPServer("baza-skolkovo", "0.1.0", server.WithToolCapabilities(true))

	m.AddTool(
		mcp.NewTool("search_documents",
			mcp.WithDescription("Семантический поиск по действующим документам Фонда «Сколково». Возвращает релевантные фрагменты с источником."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Required(), mcp.Description("Поисковый запрос на естественном языке")),
			mcp.WithNumber("limit", mcp.Description("Сколько фрагментов вернуть (по умолчанию 5)")),
		),
		s.handleSearch,
	)

	m.AddTool(
		mcp.NewTool("get_document",
			mcp.WithDescription("Получить метаданные документа по идентификатору: название, статус, категорию, ссылку на первоисточник."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("id", mcp.Required(), mcp.Description("Идентификатор документа")),
		),
		s.handleGet,
	)

	m.AddTool(
		mcp.NewTool("list_active_documents",
			mcp.WithDescription("Список действующих документов. Можно отфильтровать по категории."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("category", mcp.Description("Категория (необязательно)")),
		),
		s.handleList,
	)

	s.mcpSrv = m
	return m
}

// MCPServer возвращает внутренний MCPServer для регистрации дополнительных инструментов.
func (s *Server) MCPServer() *server.MCPServer {
	return s.buildMCP()
}

func (s *Server) handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("параметр query обязателен"), nil
	}
	limit := req.GetInt("limit", 5)
	results, err := s.rag.Search(ctx, query, limit)
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска: " + err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(results)), nil
}

func (s *Server) handleGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("параметр id обязателен"), nil
	}
	doc, err := s.store.Get(ctx, id)
	if err != nil {
		return mcp.NewToolResultError("документ не найден"), nil
	}
	return mcp.NewToolResultText(toJSON(doc)), nil
}

func (s *Server) handleList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	docs, err := s.store.List(ctx, store.Filter{Status: model.StatusActive, Category: category})
	if err != nil {
		return mcp.NewToolResultError("ошибка списка: " + err.Error()), nil
	}
	type item struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Category  string `json:"category"`
		SourceURL string `json:"source_url"`
	}
	out := make([]item, 0, len(docs))
	for _, d := range docs {
		out = append(out, item{ID: d.ID, Title: d.Title, Category: d.Category, SourceURL: d.SourceURL})
	}
	return mcp.NewToolResultText(toJSON(out)), nil
}

// ListenAndServe запускает HTTP-сервер: /mcp (защищён) и /health (открыт).
func (s *Server) ListenAndServe() error {
	streamSrv := server.NewStreamableHTTPServer(s.buildMCP(), server.WithStateLess(true))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"baza-skolkovo-mcp"}`))
	})
	mux.Handle("/mcp", s.middleware(streamSrv))

	if s.apiKey == "" {
		log.Println("[mcp] ВНИМАНИЕ: MCP_API_KEY не задан — сервер работает без авторизации")
	}
	log.Printf("[mcp] открытый MCP-сервер слушает %s (endpoint /mcp)", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// middleware применяет rate-limit по IP, проверку API-ключа и логирование доступа.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !s.limiters.allow(ip) {
			log.Printf("[mcp] 429 %s rate-limit", ip)
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		if s.apiKey != "" && !s.authorized(r) {
			log.Printf("[mcp] 401 %s unauthorized", ip)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		log.Printf("[mcp] %s %s %s", ip, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// clientIP извлекает IP клиента с учётом reverse-proxy (X-Forwarded-For).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) authorized(r *http.Request) bool {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k == s.apiKey
	}
	const pfx = "Bearer "
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, pfx) {
		return strings.TrimPrefix(a, pfx) == s.apiKey
	}
	return false
}

func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
