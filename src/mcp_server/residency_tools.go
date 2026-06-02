// residency_tools.go — MCP-инструменты для системы управления резидентством.
//
// Инструменты:
//   - get_client_status       — статус клиента, дата обновления, история переходов;
//   - get_checklist           — чек-листы клиента с прогрессом шагов;
//   - get_deadlines           — дедлайны клиента с фильтрацией по горизонту;
//   - get_client_documents    — список документов клиента;
//   - list_clients            — список клиентов (опц. tenant, stage, limit);
//   - create_client           — создание нового клиента;
//   - update_client_stage     — переход клиента на новую стадию;
//   - get_templates           — список шаблонов документов (опц. по типу).
package mcpserver

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// RegisterResidencyTools регистрирует инструменты управления резидентством на MCP-сервере.
// clientDocStore может быть nil — тогда get_client_documents вернёт понятную ошибку.
func RegisterResidencyTools(
	srv *server.MCPServer,
	st store.ClientStore,
	checklistStore store.ChecklistStore,
	deadlineStore store.DeadlineStore,
	templateStore store.TemplateStore,
	clientDocStore store.ClientDocumentStore,
) {
	registerResidencyTools(srv, st, checklistStore, deadlineStore, templateStore, clientDocStore)
}

// registerResidencyTools регистрирует инструменты управления резидентством на MCP-сервере.
func registerResidencyTools(
	srv *server.MCPServer,
	st store.ClientStore,
	checklistStore store.ChecklistStore,
	deadlineStore store.DeadlineStore,
	templateStore store.TemplateStore,
	clientDocStore store.ClientDocumentStore,
) {
	// --- get_client_status ---
	srv.AddTool(
		mcp.NewTool("get_client_status",
			mcp.WithDescription("Получить статус клиента: текущую стадию, дату обновления и историю переходов."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetClientStatus(ctx, req, st)
		},
	)

	// --- get_checklist ---
	srv.AddTool(
		mcp.NewTool("get_checklist",
			mcp.WithDescription("Получить чек-листы клиента с прогрессом по шагам. Можно отфильтровать по типу процедуры."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithString("procedure_type", mcp.Description("Тип процедуры: entry, reporting, extension, exit")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetChecklist(ctx, req, checklistStore)
		},
	)

	// --- get_deadlines ---
	srv.AddTool(
		mcp.NewTool("get_deadlines",
			mcp.WithDescription("Получить дедлайны клиента. Параметр days_ahead задаёт горизонт в днях (по умолчанию 30)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithNumber("days_ahead", mcp.Description("Горизонт в днях (по умолчанию 30)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetDeadlines(ctx, req, deadlineStore)
		},
	)

	// --- get_client_documents ---
	srv.AddTool(
		mcp.NewTool("get_client_documents",
			mcp.WithDescription("Получить список документов, привязанных к клиенту."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if clientDocStore == nil {
				return mcp.NewToolResultError("хранилище документов клиента не настроено"), nil
			}
			clientID, err := req.RequireString("client_id")
			if err != nil {
				return mcp.NewToolResultError("параметр client_id обязателен"), nil
			}
			docs, err := clientDocStore.ListClientDocuments(ctx, clientID)
			if err != nil {
				return mcp.NewToolResultError("ошибка получения документов клиента: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(docs)), nil
		},
	)

	// --- list_clients ---
	srv.AddTool(
		mcp.NewTool("list_clients",
			mcp.WithDescription("Получить список клиентов. Можно отфильтровать по tenant_id и стадии, ограничить количество."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("tenant_id", mcp.Description("Идентификатор тенанта")),
			mcp.WithString("stage", mcp.Description("Стадия резидентства")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (по умолчанию 50)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListClients(ctx, req, st)
		},
	)

	// --- create_client ---
	srv.AddTool(
		mcp.NewTool("create_client",
			mcp.WithDescription("Создать нового клиента. Обязательные поля: name, inn."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("name", mcp.Required(), mcp.Description("Наименование организации")),
			mcp.WithString("inn", mcp.Required(), mcp.Description("ИНН организации (10 или 12 цифр)")),
			mcp.WithString("contact_email", mcp.Description("Контактный email")),
			mcp.WithString("contact_phone", mcp.Description("Контактный телефон")),
			mcp.WithString("tenant_id", mcp.Description("Идентификатор тенанта")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleCreateClient(ctx, req, st)
		},
	)

	// --- update_client_stage ---
	srv.AddTool(
		mcp.NewTool("update_client_stage",
			mcp.WithDescription("Перевести клиента на новую стадию резидентства. Автоматически создаёт запись перехода."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("client_id", mcp.Required(), mcp.Description("Идентификатор клиента")),
			mcp.WithString("new_stage", mcp.Required(), mcp.Description("Новая стадия резидентства")),
			mcp.WithString("notes", mcp.Description("Примечания к переходу")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleUpdateClientStage(ctx, req, st)
		},
	)

	// --- get_templates ---
	srv.AddTool(
		mcp.NewTool("get_templates",
			mcp.WithDescription("Получить список шаблонов документов. Можно отфильтровать по типу."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("template_type", mcp.Description("Тип шаблона")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetTemplates(ctx, req, templateStore)
		},
	)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleGetClientStatus(ctx context.Context, req mcp.CallToolRequest, st store.ClientStore) (*mcp.CallToolResult, error) {
	clientID, err := req.RequireString("client_id")
	if err != nil {
		return mcp.NewToolResultError("параметр client_id обязателен"), nil
	}

	client, err := st.GetClient(ctx, clientID)
	if err != nil {
		return mcp.NewToolResultError("клиент не найден: " + err.Error()), nil
	}

	transitions, err := st.GetStageHistory(ctx, clientID)
	if err != nil {
		transitions = nil // не блокируем ответ, если истории нет
	}

	type statusResponse struct {
		ClientID       string                   `json:"client_id"`
		Name           string                   `json:"name"`
		ResidencyStage model.ResidencyStage     `json:"residency_stage"`
		UpdatedAt      time.Time                `json:"updated_at"`
		StageHistory   []*model.StageTransition `json:"stage_history,omitempty"`
	}

	resp := statusResponse{
		ClientID:       client.ID,
		Name:           client.Name,
		ResidencyStage: client.ResidencyStage,
		UpdatedAt:      client.UpdatedAt,
		StageHistory:   transitions,
	}
	return mcp.NewToolResultText(toJSON(resp)), nil
}

func handleGetChecklist(ctx context.Context, req mcp.CallToolRequest, cs store.ChecklistStore) (*mcp.CallToolResult, error) {
	clientID, err := req.RequireString("client_id")
	if err != nil {
		return mcp.NewToolResultError("параметр client_id обязателен"), nil
	}

	procType := req.GetString("procedure_type", "")

	clientChecklists, err := cs.GetClientChecklists(ctx, clientID)
	if err != nil {
		return mcp.NewToolResultError("ошибка получения чек-листов: " + err.Error()), nil
	}

	type stepInfo struct {
		StepIndex int              `json:"step_index"`
		Status    model.StepStatus `json:"status"`
		Notes     string           `json:"notes,omitempty"`
	}

	type checklistResult struct {
		ChecklistID    string                `json:"checklist_id"`
		ChecklistTitle string                `json:"checklist_title"`
		ProcedureType  model.ChecklistType   `json:"procedure_type"`
		Status         model.ChecklistStatus `json:"status"`
		Steps          []stepInfo            `json:"steps"`
	}

	var results []checklistResult
	for _, cc := range clientChecklists {
		// фильтруем по типу процедуры, если указан
		if procType != "" {
			tpl, err := cs.GetChecklist(ctx, cc.ChecklistID)
			if err != nil {
				continue
			}
			if string(tpl.ProcedureType) != procType {
				continue
			}
		}

		steps, err := cs.GetStepStatuses(ctx, cc.ID)
		if err != nil {
			steps = nil
		}

		stepInfos := make([]stepInfo, 0, len(steps))
		for _, s := range steps {
			stepInfos = append(stepInfos, stepInfo{
				StepIndex: s.StepIndex,
				Status:    s.Status,
				Notes:     s.Notes,
			})
		}

		tpl, err := cs.GetChecklist(ctx, cc.ChecklistID)
		title := ""
		pt := cc.ChecklistID // fallback
		if err == nil && tpl != nil {
			title = tpl.Title
			pt = string(tpl.ProcedureType)
		}

		results = append(results, checklistResult{
			ChecklistID:    cc.ChecklistID,
			ChecklistTitle: title,
			ProcedureType:  model.ChecklistType(pt),
			Status:         cc.Status,
			Steps:          stepInfos,
		})
	}

	return mcp.NewToolResultText(toJSON(results)), nil
}

func handleGetDeadlines(ctx context.Context, req mcp.CallToolRequest, ds store.DeadlineStore) (*mcp.CallToolResult, error) {
	clientID, err := req.RequireString("client_id")
	if err != nil {
		return mcp.NewToolResultError("параметр client_id обязателен"), nil
	}

	daysAhead := req.GetInt("days_ahead", 30)

	deadlines, err := ds.ListDeadlines(ctx, clientID, daysAhead)
	if err != nil {
		return mcp.NewToolResultError("ошибка получения дедлайнов: " + err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(deadlines)), nil
}

func handleListClients(ctx context.Context, req mcp.CallToolRequest, st store.ClientStore) (*mcp.CallToolResult, error) {
	tenantID := req.GetString("tenant_id", "")
	stageStr := req.GetString("stage", "")
	limit := req.GetInt("limit", 50)

	if tenantID == "" {
		return mcp.NewToolResultError("параметр tenant_id обязателен для list_clients"), nil
	}

	var stage model.ResidencyStage
	if stageStr != "" {
		stage = model.ResidencyStage(stageStr)
	}

	clients, err := st.ListClients(ctx, tenantID, stage)
	if err != nil {
		return mcp.NewToolResultError("ошибка списка клиентов: " + err.Error()), nil
	}

	if limit > 0 && len(clients) > limit {
		clients = clients[:limit]
	}

	type clientSummary struct {
		ID             string               `json:"id"`
		Name           string               `json:"name"`
		INN            string               `json:"inn"`
		ResidencyStage model.ResidencyStage `json:"residency_stage"`
		UpdatedAt      time.Time            `json:"updated_at"`
	}

	out := make([]clientSummary, 0, len(clients))
	for _, c := range clients {
		out = append(out, clientSummary{
			ID:             c.ID,
			Name:           c.Name,
			INN:            c.INN,
			ResidencyStage: c.ResidencyStage,
			UpdatedAt:      c.UpdatedAt,
		})
	}

	return mcp.NewToolResultText(toJSON(out)), nil
}

func handleCreateClient(ctx context.Context, req mcp.CallToolRequest, st store.ClientStore) (*mcp.CallToolResult, error) {
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("параметр name обязателен"), nil
	}
	inn, err := req.RequireString("inn")
	if err != nil {
		return mcp.NewToolResultError("параметр inn обязателен"), nil
	}

	client := &model.Client{
		ID:             uuid.New().String(),
		Name:           name,
		INN:            inn,
		ContactEmail:   req.GetString("contact_email", ""),
		ContactPhone:   req.GetString("contact_phone", ""),
		TenantID:       req.GetString("tenant_id", ""),
		ResidencyStage: model.StageApplication,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := st.CreateClient(ctx, client); err != nil {
		return mcp.NewToolResultError("ошибка создания клиента: " + err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(client)), nil
}

func handleUpdateClientStage(ctx context.Context, req mcp.CallToolRequest, st store.ClientStore) (*mcp.CallToolResult, error) {
	clientID, err := req.RequireString("client_id")
	if err != nil {
		return mcp.NewToolResultError("параметр client_id обязателен"), nil
	}
	newStageStr, err := req.RequireString("new_stage")
	if err != nil {
		return mcp.NewToolResultError("параметр new_stage обязателен"), nil
	}

	client, err := st.GetClient(ctx, clientID)
	if err != nil {
		return mcp.NewToolResultError("клиент не найден: " + err.Error()), nil
	}

	newStage := model.ResidencyStage(newStageStr)
	if !client.ResidencyStage.CanTransition(newStage) {
		return mcp.NewToolResultError("недопустимый переход: " + string(client.ResidencyStage) + " → " + string(newStage)), nil
	}

	transition := &model.StageTransition{
		ID:             uuid.New().String(),
		ClientID:       clientID,
		FromStage:      client.ResidencyStage,
		ToStage:        newStage,
		TransitionedAt: time.Now(),
		Notes:          req.GetString("notes", ""),
	}

	if err := st.AddStageTransition(ctx, transition); err != nil {
		return mcp.NewToolResultError("ошибка добавления перехода: " + err.Error()), nil
	}

	client.ResidencyStage = newStage
	client.UpdatedAt = time.Now()
	if err := st.UpdateClient(ctx, client); err != nil {
		return mcp.NewToolResultError("ошибка обновления клиента: " + err.Error()), nil
	}

	autoCreateStageDeadline(ctx, st, clientID, newStage)

	type updateResponse struct {
		Client     *model.Client          `json:"client"`
		Transition *model.StageTransition `json:"transition"`
	}

	return mcp.NewToolResultText(toJSON(updateResponse{Client: client, Transition: transition})), nil
}

// autoCreateStageDeadline создаёт дедлайн квартальной отчётности при переходе
// клиента на стадию «отчётность». Делегирует единой логике store.EnsureReportingDeadline
// (общей с Резидентство-Админ). No-op, если хранилище не умеет дедлайны.
func autoCreateStageDeadline(ctx context.Context, st store.ClientStore, clientID string, to model.ResidencyStage) {
	if dc, ok := st.(store.DeadlineEnsurer); ok {
		store.EnsureReportingDeadline(ctx, dc, clientID, to)
	}
}

func handleGetTemplates(ctx context.Context, req mcp.CallToolRequest, ts store.TemplateStore) (*mcp.CallToolResult, error) {
	templateType := req.GetString("template_type", "")

	templates, err := ts.ListTemplates(ctx, templateType)
	if err != nil {
		return mcp.NewToolResultError("ошибка получения шаблонов: " + err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(templates)), nil
}
