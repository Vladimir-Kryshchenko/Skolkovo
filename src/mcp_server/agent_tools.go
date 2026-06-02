// agent_tools.go — MCP-инструменты на базе ИИ-агентов.
//
// Инструменты:
//   - ask_consultant       — вопрос ИИ-консультанту;
//   - validate_document    — проверка документа по чек-листу;
//   - get_next_steps       — рекомендации координатора;
//   - subscribe_to_changes — подписка клиента на категории изменений;
//   - draft_document       — LLM-черновик документа;
//   - check_eligibility    — проверка компании по ИНН;
//   - generate_document    — генерация PDF/DOCX из шаблона;
//   - list_document_templates — список доступных шаблонов;
//   - get_coverage_audit   — полнота охвата источников.
package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/agents"
	"baza-skolkovo/src/audit"
	"baza-skolkovo/src/common/config"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/eligibility"
	"baza-skolkovo/src/generator"
)

// AgentToolDeps — набор зависимостей для регистрации агентских MCP-инструментов.
type AgentToolDeps struct {
	Consultant  *agents.ConsultantAgent
	Validator   *agents.ValidatorAgent
	Monitor     *agents.MonitorAgent
	Coordinator *agents.CoordinatorAgent
	Drafter     *agents.DocumentDraftingAgent
	Eligibility *eligibility.Checker  // может быть nil
	Generator   *generator.DocumentGenerator // может быть nil
	Store       store.Store
	Config      config.Config
}

// RegisterAgentTools регистрирует агентские MCP-инструменты на сервере.
func RegisterAgentTools(srv *server.MCPServer, deps AgentToolDeps) {
	// --- ask_consultant ---
	srv.AddTool(
		mcp.NewTool("ask_consultant",
			mcp.WithDescription("Задать вопрос ИИ-консультанту по базе документов Сколково. Возвращает ответ с источниками."),
			mcp.WithString("question", mcp.Required(), mcp.Description("Текст вопроса")),
			mcp.WithString("client_id", mcp.Description("Идентификатор клиента (необязательно)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			question := req.GetString("question", "")
			clientID := req.GetString("client_id", "")
			resp, err := deps.Consultant.Ask(ctx, question, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка консультанта: " + err.Error()), nil
			}
			return mcp.NewToolResultText(formatAgentAnswer(resp)), nil
		},
	)

	// --- validate_document ---
	srv.AddTool(
		mcp.NewTool("validate_document",
			mcp.WithDescription("Проверить документ по чек-листу процедуры. Возвращает отчёт с проблемами и оценкой."),
			mcp.WithString("document_text", mcp.Required(), mcp.Description("Полный текст документа")),
			mcp.WithString("procedure_type", mcp.Required(), mcp.Description("Тип процедуры: entry, reporting, extension, exit")),
			mcp.WithString("client_id", mcp.Description("Идентификатор клиента (необязательно)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			docText := req.GetString("document_text", "")
			procType := req.GetString("procedure_type", "")
			clientID := req.GetString("client_id", "")
			report, err := deps.Validator.ValidateDocument(ctx, docText, procType, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка валидации: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(report)), nil
		},
	)

	// --- get_next_steps ---
	srv.AddTool(
		mcp.NewTool("get_next_steps",
			mcp.WithDescription("Получить рекомендации следующих шагов для клиента по чек-листу."),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clientID := req.GetString("client_id", "")
			steps, err := deps.Coordinator.GetNextSteps(ctx, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка координатора: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(steps)), nil
		},
	)

	// --- subscribe_to_changes ---
	srv.AddTool(
		mcp.NewTool("subscribe_to_changes",
			mcp.WithDescription("Подписать клиента на уведомления об изменениях в указанных категориях документов."),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithString("categories", mcp.Required(), mcp.Description("Категории через запятую: regulations, events, contests, reporting")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			clientID := req.GetString("client_id", "")
			catsStr := req.GetString("categories", "")
			cats := strings.Split(catsStr, ",")
			for i := range cats {
				cats[i] = strings.TrimSpace(cats[i])
			}
			if err := deps.Monitor.Subscribe(ctx, clientID, cats); err != nil {
				return mcp.NewToolResultError("ошибка подписки: " + err.Error()), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Клиент %s подписан на: %s", clientID, catsStr)), nil
		},
	)

	// --- draft_document ---
	srv.AddTool(
		mcp.NewTool("draft_document",
			mcp.WithDescription("Подготовить черновик документа для клиента (заявка, описание проекта, отчёт, продление, выход). Возвращает заполненный текст в Markdown."),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithString("document_type", mcp.Required(), mcp.Description("Тип документа: application, project_description, report, extension_request, exit_notice, ird_description")),
			mcp.WithString("extra_context", mcp.Description("Дополнительный контекст от консультанта")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			draftReq := agents.DraftRequest{
				ClientID:     req.GetString("client_id", ""),
				DocumentType: req.GetString("document_type", ""),
				ExtraContext: req.GetString("extra_context", ""),
			}
			result, err := deps.Drafter.Draft(ctx, draftReq)
			if err != nil {
				return mcp.NewToolResultError("ошибка подготовки документа: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(result)), nil
		},
	)

	// --- check_eligibility (опциональный) ---
	if deps.Eligibility != nil {
		srv.AddTool(
			mcp.NewTool("check_eligibility",
				mcp.WithDescription("Проверить, может ли компания стать резидентом Сколково. Принимает ИНН, возвращает отчёт с оценкой, проблемами и рекомендациями."),
				mcp.WithString("inn", mcp.Required(), mcp.Description("ИНН компании (10 или 12 цифр)")),
			),
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				inn := req.GetString("inn", "")
				report, err := deps.Eligibility.CheckByINN(ctx, inn)
				if err != nil {
					return mcp.NewToolResultError("ошибка проверки: " + err.Error()), nil
				}
				return mcp.NewToolResultText(toJSON(report)), nil
			},
		)
	}

	// --- generate_document + list_document_templates (опциональные) ---
	if deps.Generator != nil {
		srv.AddTool(
			mcp.NewTool("generate_document",
				mcp.WithDescription("Сгенерировать готовый файл документа (PDF/DOCX) для клиента из шаблона. Возвращает путь к файлу; при inline=true также base64-содержимое для скачивания. Список шаблонов: list_document_templates."),
				mcp.WithString("template_id", mcp.Required(), mcp.Description("Идентификатор шаблона (имя файла, напр. Заявление_на_резидентство.go.tpl)")),
				mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
				mcp.WithString("variables", mcp.Description("Доп. переменные в формате key=value через запятую")),
				mcp.WithBoolean("inline", mcp.Description("Вернуть содержимое файла в base64 (для скачивания)")),
			),
			func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				templateID := req.GetString("template_id", "")
				clientID := req.GetString("client_id", "")
				vars := map[string]string{}
				if raw := req.GetString("variables", ""); raw != "" {
					for _, kv := range strings.Split(raw, ",") {
						if i := strings.IndexByte(kv, '='); i > 0 {
							vars[strings.TrimSpace(kv[:i])] = strings.TrimSpace(kv[i+1:])
						}
					}
				}
				out, err := deps.Generator.RenderTemplate(ctx, templateID, clientID, vars)
				if err != nil {
					return mcp.NewToolResultError("ошибка генерации: " + err.Error()), nil
				}
				result := map[string]any{
					"path":     out,
					"filename": filepath.Base(out),
				}
				if req.GetBool("inline", false) {
					data, rerr := os.ReadFile(out)
					if rerr != nil {
						return mcp.NewToolResultError("файл сгенерирован, но не читается: " + rerr.Error()), nil
					}
					result["content_base64"] = base64.StdEncoding.EncodeToString(data)
					result["size_bytes"] = len(data)
				}
				return mcp.NewToolResultText(toJSON(result)), nil
			},
		)

		srv.AddTool(
			mcp.NewTool("list_document_templates",
				mcp.WithDescription("Список доступных шаблонов документов для генерации (generate_document)."),
				mcp.WithReadOnlyHintAnnotation(true),
			),
			func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				names, err := deps.Generator.ListAvailableTemplates(ctx)
				if err != nil {
					return mcp.NewToolResultError("ошибка списка шаблонов: " + err.Error()), nil
				}
				return mcp.NewToolResultText(toJSON(names)), nil
			},
		)
	}

	// --- get_coverage_audit ---
	srv.AddTool(
		mcp.NewTool("get_coverage_audit",
			mcp.WithDescription("Отчёт о полноте охвата источников Сколково: какие источники (документы, новости, мероприятия, конкурсы, FAQ, льготы, НПА, Telegram, резиденты) покрыты, а какие не настроены/устарели/без данных."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
		),
		func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			rep := audit.BuildCoverageReport(ctx, deps.Config, deps.Store)
			return mcp.NewToolResultText(toJSON(rep)), nil
		},
	)

	log.Printf("[mcp:agents] зарегистрированы инструменты: ask_consultant, validate_document, get_next_steps, subscribe_to_changes, draft_document, check_eligibility, generate_document, list_document_templates, get_coverage_audit")
}

// formatAgentAnswer добавляет список источников к ответу консультанта.
func formatAgentAnswer(resp agents.ConsultantResponse) string {
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
