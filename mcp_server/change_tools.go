// change_tools.go — MCP-инструменты ленты изменений и мониторинга свежести.
//
// Инструменты:
//   - get_recent_changes — что изменилось в базе Сколково за период (документы,
//     новости, конкурсы, НПА, льготы): новые, обновлённые, устаревшие;
//   - get_source_health  — состояние источников: когда обновлялись, не «протухли» ли.
package mcpserver

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/health"
)

// staleAfter — порог, после которого источник считается «протухшим» (нет успешных
// обновлений). По умолчанию сутки: парсинг идёт минимум раз в 6 часов.
const staleAfter = 24 * time.Hour

// RegisterChangeTools регистрирует get_recent_changes на MCP-сервере.
func RegisterChangeTools(srv *server.MCPServer, cs changes.Store) {
	if cs == nil {
		return
	}
	srv.AddTool(
		mcp.NewTool("get_recent_changes",
			mcp.WithDescription("Лента изменений базы знаний Сколково: какие документы, новости, конкурсы, НПА и льготы появились, обновились или устарели за указанный период. Возвращает записи по убыванию времени."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithNumber("since_days", mcp.Description("За сколько последних дней показать изменения (по умолчанию 7)")),
			mcp.WithString("entity_type", mcp.Description("Тип сущности: document, news, event, contest, npa, preference, faq (опционально)")),
			mcp.WithString("category", mcp.Description("Категория (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (по умолчанию 20)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sinceDays := req.GetInt("since_days", 7)
			limit := req.GetInt("limit", 20)
			f := changes.Filter{
				EntityType: req.GetString("entity_type", ""),
				Category:   req.GetString("category", ""),
				Limit:      limit,
			}
			if sinceDays > 0 {
				f.Since = time.Now().AddDate(0, 0, -sinceDays)
			}
			evs, err := cs.Recent(ctx, f)
			if err != nil {
				return mcp.NewToolResultError("ошибка ленты изменений: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(evs)), nil
		},
	)
}

// RegisterHealthTools регистрирует get_source_health на MCP-сервере.
func RegisterHealthTools(srv *server.MCPServer, hs health.Store) {
	if hs == nil {
		return
	}
	srv.AddTool(
		mcp.NewTool("get_source_health",
			mcp.WithDescription("Состояние источников данных: когда каждый источник (документы, новости, мероприятия, конкурсы, FAQ, Telegram, льготы, НПА, загрузка файлов) последний раз успешно обновлялся и не «протух» ли. Помогает убедиться, что база актуальна."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
		),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sources, err := hs.List(ctx)
			if err != nil {
				return mcp.NewToolResultError("ошибка мониторинга свежести: " + err.Error()), nil
			}
			now := time.Now()
			type item struct {
				Name                string     `json:"name"`
				State               string     `json:"state"`
				LastRunAt           time.Time  `json:"last_run_at"`
				LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
				ItemsLastRun        int        `json:"items_last_run"`
				ConsecutiveFailures int        `json:"consecutive_failures"`
				LastError           string     `json:"last_error,omitempty"`
			}
			out := make([]item, 0, len(sources))
			for _, s := range sources {
				out = append(out, item{
					Name:                s.Name,
					State:               string(s.State(staleAfter, now)),
					LastRunAt:           s.LastRunAt,
					LastSuccessAt:       s.LastSuccessAt,
					ItemsLastRun:        s.ItemsLastRun,
					ConsecutiveFailures: s.ConsecutiveFailures,
					LastError:           s.LastError,
				})
			}
			return mcp.NewToolResultText(toJSON(out)), nil
		},
	)
}
