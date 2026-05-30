// Package mcp_server — аудит-лог MCP-запросов.
// Каждый вызов инструмента логируется в таблицу mcp_audit_log
// с указанием tenant_id (из API-ключа), инструмента, времени выполнения и успеха.
package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogger записывает аудит-логи запросов к MCP в PostgreSQL.
type AuditLogger struct {
	pool    *pgxpool.Pool
	enabled bool
}

// AuditEntry — запись аудит-лога.
type AuditEntry struct {
	TenantID   string
	APIKeyPfx  string // первые 8 символов API-ключа
	ToolName   string
	InputHash  string
	InputBrief string
	ClientID   string
	RemoteAddr string
	DurationMS int
	Success    bool
	ErrorMsg   string
}

// NewAuditLogger создаёт логгер аудита. Если pool == nil, работает как no-op.
func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool, enabled: pool != nil}
}

// Log записывает одну запись аудит-лога. Ошибки записи не возвращаются
// вызывающему коду — аудит не должен блокировать бизнес-логику.
func (a *AuditLogger) Log(ctx context.Context, entry AuditEntry) {
	if !a.enabled {
		return
	}

	// Используем короткий контекст для записи, независимо от контекста запроса.
	writeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var tenantID, clientID, errorMsg *string
	if entry.TenantID != "" {
		tenantID = &entry.TenantID
	}
	if entry.ClientID != "" {
		clientID = &entry.ClientID
	}
	if entry.ErrorMsg != "" {
		errorMsg = &entry.ErrorMsg
	}

	_, err := a.pool.Exec(writeCtx, `
		INSERT INTO mcp_audit_log
			(tenant_id, api_key_pfx, tool_name, input_hash, input_brief,
			 client_id, remote_addr, duration_ms, success, error_msg)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		tenantID,
		entry.APIKeyPfx,
		entry.ToolName,
		entry.InputHash,
		entry.InputBrief,
		clientID,
		entry.RemoteAddr,
		entry.DurationMS,
		entry.Success,
		errorMsg,
	)
	if err != nil {
		log.Printf("[audit] ошибка записи лога: %v", err)
	}
}

// auditKey — ключ контекста для передачи аудит-информации через цепочку вызовов.
type auditKey struct{}

// AuditContext добавляет информацию об аудите в контекст запроса.
func AuditContext(ctx context.Context, apiKey, remoteAddr string) context.Context {
	return context.WithValue(ctx, auditKey{}, &auditInfo{
		APIKey:     apiKey,
		RemoteAddr: remoteAddr,
	})
}

type auditInfo struct {
	APIKey     string
	RemoteAddr string
	TenantID   string
}

// auditInfoFromCtx извлекает информацию об аудите из контекста.
func auditInfoFromCtx(ctx context.Context) *auditInfo {
	if v := ctx.Value(auditKey{}); v != nil {
		if info, ok := v.(*auditInfo); ok {
			return info
		}
	}
	return &auditInfo{}
}

// apiKeyPrefix возвращает первые 8 символов API-ключа для логирования.
func apiKeyPrefix(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:8]
}

// hashInput вычисляет SHA-256 от входных данных.
func hashInput(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])[:16]
}

// briefInput возвращает первые 200 символов входных данных для лога.
func briefInput(input string) string {
	input = strings.ReplaceAll(input, "\n", " ")
	input = strings.Join(strings.Fields(input), " ")
	if len(input) > 200 {
		return input[:197] + "..."
	}
	return input
}

// AuditMiddleware оборачивает http.Handler и записывает аудит-лог для каждого MCP-запроса.
func AuditMiddleware(logger *AuditLogger, next http.Handler) http.Handler {
	if logger == nil || !logger.enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Извлекаем API-ключ из заголовка Authorization.
		apiKey := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
		if apiKey == "" {
			apiKey = r.Header.Get("X-API-Key")
		}

		// Сохраняем информацию об аудите в контексте.
		ctx := AuditContext(r.Context(), apiKey, r.RemoteAddr)
		r = r.WithContext(ctx)

		// Оборачиваем ResponseWriter для перехвата статуса.
		rw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		// Логируем только запросы к /mcp (инструменты).
		if strings.HasPrefix(r.URL.Path, "/mcp") {
			durationMS := int(time.Since(start).Milliseconds())
			info := auditInfoFromCtx(ctx)

			entry := AuditEntry{
				TenantID:   info.TenantID,
				APIKeyPfx:  apiKeyPrefix(apiKey),
				ToolName:   extractToolName(r),
				RemoteAddr: r.RemoteAddr,
				DurationMS: durationMS,
				Success:    rw.status < 400,
			}
			if rw.status >= 400 {
				entry.ErrorMsg = fmt.Sprintf("HTTP %d", rw.status)
			}

			go logger.Log(context.Background(), entry)
		}
	})
}

// statusResponseWriter перехватывает HTTP-статус ответа.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// extractToolName пытается извлечь название инструмента из пути запроса.
func extractToolName(r *http.Request) string {
	path := r.URL.Path
	// MCP-запросы: путь /mcp или /mcp/tools/call
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "call" && i > 0 {
			// Может быть /mcp/tools/call?tool=search_documents
			if tool := r.URL.Query().Get("tool"); tool != "" {
				return tool
			}
		}
	}
	return "unknown"
}

// AuditStats — статистика использования MCP по tenant за период.
type AuditStats struct {
	TenantID    string
	TotalCalls  int
	UniqueTools []string
	ErrorCount  int
	AvgDuration float64
}

// GetAuditStats возвращает статистику использования MCP за последние N дней.
func GetAuditStats(ctx context.Context, pool *pgxpool.Pool, tenantID string, days int) ([]AuditStats, error) {
	if pool == nil {
		return nil, nil
	}

	query := `
		SELECT
			COALESCE(tenant_id, 'anonymous') as tid,
			COUNT(*)                          as total_calls,
			COUNT(*) FILTER (WHERE NOT success) as error_count,
			AVG(duration_ms)                  as avg_dur
		FROM mcp_audit_log
		WHERE created_at > now() - $1::interval
	`
	args := []any{fmt.Sprintf("%d days", days)}

	if tenantID != "" {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}

	query += " GROUP BY 1 ORDER BY total_calls DESC LIMIT 50"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []AuditStats
	for rows.Next() {
		var s AuditStats
		var avgDur *float64
		if err := rows.Scan(&s.TenantID, &s.TotalCalls, &s.ErrorCount, &avgDur); err != nil {
			continue
		}
		if avgDur != nil {
			s.AvgDuration = *avgDur
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
