// scheduler_tools.go — MCP-инструменты планировщика заданий и учёта токенов ИИ.
//
// Инструменты:
//   - get_scheduler_runs — журнал запусков сбора: какие задания и когда отработали,
//     сколько нашли, статус и ошибки;
//   - get_ai_usage       — расход токенов ИИ-агентов по моделям и агентам.
package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/jobsched"
)

// RegisterSchedulerTools регистрирует get_scheduler_runs и get_ai_usage.
func RegisterSchedulerTools(srv *server.MCPServer, js *jobsched.Store) {
	if js == nil {
		return
	}

	// --- get_scheduler_runs ---
	srv.AddTool(
		mcp.NewTool("get_scheduler_runs",
			mcp.WithDescription("Журнал запусков планировщика сбора: какие задания (документы, мероприятия, конкурсы, FAQ, резиденты, Telegram, льготы, НПА, страницы сайта) и когда отработали, сколько нашли новых/обновлённых, статус (success/error/partial/skipped) и текст ошибки. Записи по убыванию времени."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("job_name", mcp.Description("Имя задания: documents, events, contests, faq, residents, telegram, preferences, regulations, sitepages, fetch_bodies (опционально)")),
			mcp.WithString("status", mcp.Description("Статус: success, error, partial, skipped, running (опционально)")),
			mcp.WithNumber("since_days", mcp.Description("За сколько последних дней (по умолчанию все)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (по умолчанию 50)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			runs, err := js.ListRuns(ctx, jobsched.RunFilter{
				JobName:   req.GetString("job_name", ""),
				Status:    req.GetString("status", ""),
				SinceDays: req.GetInt("since_days", 0),
				Limit:     req.GetInt("limit", 50),
			})
			if err != nil {
				return mcp.NewToolResultError("ошибка журнала запусков: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(runs)), nil
		},
	)

	// --- get_ai_usage ---
	srv.AddTool(
		mcp.NewTool("get_ai_usage",
			mcp.WithDescription("Учёт расхода токенов ИИ-агентов: сколько токенов (prompt/completion/total) потрачено по каждой модели и каждому типу агента (consultant, validator, monitor, coordinator, page_annotator), длительность и ошибки вызовов. Возвращает агрегаты по моделям и по агентам, а также последние вызовы."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("agent_type", mcp.Description("Тип агента: consultant, validator, monitor, coordinator, page_annotator (опционально)")),
			mcp.WithString("model_id", mcp.Description("Идентификатор модели в API, например qwen-max (опционально)")),
			mcp.WithNumber("since_days", mcp.Description("За сколько последних дней (по умолчанию 30)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей в списке вызовов (по умолчанию 50)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sinceDays := req.GetInt("since_days", 30)
			usage, err := js.ListUsage(ctx, jobsched.AIUsageFilter{
				AgentType: req.GetString("agent_type", ""),
				ModelID:   req.GetString("model_id", ""),
				SinceDays: sinceDays,
				Limit:     req.GetInt("limit", 50),
			})
			if err != nil {
				return mcp.NewToolResultError("ошибка учёта токенов: " + err.Error()), nil
			}
			byModel, _ := js.UsageByModel(ctx, sinceDays)
			byAgent, _ := js.UsageByAgent(ctx, sinceDays)
			out := struct {
				ByModel []jobsched.UsageAgg `json:"by_model"`
				ByAgent []jobsched.UsageAgg `json:"by_agent"`
				Recent  []jobsched.AIUsage  `json:"recent"`
			}{ByModel: byModel, ByAgent: byAgent, Recent: usage}
			return mcp.NewToolResultText(toJSON(out)), nil
		},
	)
}
