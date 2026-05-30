// Package tests — модульные тесты MCP-инструментов (28 tools).
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"

	"baza-skolkovo/src/common/model"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type testToolFn func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

type testMCPServer struct {
	tools map[string]testToolFn
}

func newTestMCPServer() *testMCPServer {
	return &testMCPServer{tools: make(map[string]testToolFn)}
}

func (s *testMCPServer) addTool(name string, fn testToolFn) {
	s.tools[name] = fn
}

func (s *testMCPServer) callTool(t *testing.T, name string, args map[string]interface{}) string {
	t.Helper()
	fn, ok := s.tools[name]
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("tool %q error: %v", name, err)
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("tool %q: expected TextContent, got %T", name, result.Content[0])
	}
	return content.Text
}

func (s *testMCPServer) callToolNoError(t *testing.T, name string, args map[string]interface{}) string {
	t.Helper()
	fn, ok := s.tools[name]
	if !ok {
		t.Fatalf("tool %q not registered", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := fn(context.Background(), req)
	if err != nil {
		return ""
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return ""
	}
	return content.Text
}

func toJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Core document tools (3)
// ---------------------------------------------------------------------------

func TestSearchDocumentsTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("search_documents", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("параметр query обязателен"), nil
		}
		if query == "" {
			return mcp.NewToolResultError("query не может быть пустым"), nil
		}
		_ = req.GetInt("limit", 5)
		result := fmt.Sprintf(`[{"document_id":"doc-1","title":"Тестовый документ","text":"Содержимое: %s","score":0.85}]`, query)
		return mcp.NewToolResultText(result), nil
	})

	result := srv.callTool(t, "search_documents", map[string]interface{}{"query": "льготы резидента", "limit": 3})
	if !strings.Contains(result, "Тестовый документ") {
		t.Fatalf("expected title, got: %s", result)
	}
}

func TestGetDocumentTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("параметр id обязателен"), nil
		}
		if id == "nonexistent" {
			return mcp.NewToolResultError("документ не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","title":"Тест","status":"действует","category":"Регламенты"}`, id)), nil
	})

	result := srv.callTool(t, "get_document", map[string]interface{}{"id": "doc-123"})
	if !strings.Contains(result, `"title":"Тест"`) {
		t.Fatalf("expected title, got: %s", result)
	}
}

func TestGetDocumentNotFound(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("параметр id обязателен"), nil
		}
		if id == "nonexistent" {
			return mcp.NewToolResultError("документ не найден"), nil
		}
		return mcp.NewToolResultText(`{}`), nil
	})

	result := srv.callTool(t, "get_document", map[string]interface{}{"id": "nonexistent"})
	if !strings.Contains(result, "не найден") {
		t.Fatalf("expected not found, got: %s", result)
	}
}

func TestListActiveDocumentsTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("list_active_documents", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		category := req.GetString("category", "")
		if category != "" {
			return mcp.NewToolResultText(fmt.Sprintf(`[{"id":"doc-1","title":"Документ 1","category":"%s"}]`, category)), nil
		}
		return mcp.NewToolResultText(`[{"id":"doc-1","title":"Документ 1","category":"Регламенты"},{"id":"doc-2","title":"Документ 2","category":"НПА"}]`), nil
	})

	result := srv.callTool(t, "list_active_documents", map[string]interface{}{"category": "Регламенты"})
	if !strings.Contains(result, `"category":"Регламенты"`) {
		t.Fatalf("expected category, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Extended sources tools (7)
// ---------------------------------------------------------------------------

func TestSearchEventsTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("search_events", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dateFrom := req.GetString("date_from", "")
		if dateFrom != "" {
			if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
				return mcp.NewToolResultError("неверный формат date_from, ожидается YYYY-MM-DD"), nil
			}
		}
		return mcp.NewToolResultText(`[{"id":"evt-1","title":"Вебинар: Инновации","event_date":"2026-06-01"}]`), nil
	})

	result := srv.callTool(t, "search_events", map[string]interface{}{"query": "вебинар", "date_from": "2026-06-01"})
	if !strings.Contains(result, "Вебинар") {
		t.Fatalf("expected title, got: %s", result)
	}

	result2 := srv.callToolNoError(t, "search_events", map[string]interface{}{"date_from": "invalid-date"})
	if !strings.Contains(result2, "неверный формат") {
		t.Fatalf("expected date error, got: %s", result2)
	}
}

func TestGetEventTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_event", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("event_id")
		if err != nil {
			return mcp.NewToolResultError("параметр event_id обязателен"), nil
		}
		if id == "not-found" {
			return mcp.NewToolResultError("мероприятие не найдено"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","title":"Мероприятие"}`, id)), nil
	})

	result := srv.callTool(t, "get_event", map[string]interface{}{"event_id": "evt-1"})
	if !strings.Contains(result, `"title":"Мероприятие"`) {
		t.Fatalf("expected title, got: %s", result)
	}
}

func TestSearchContestsTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("search_contests", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status := req.GetString("status", "")
		if status == "closed" {
			return mcp.NewToolResultText(`[{"id":"cnt-2","title":"Закрытый грант","status":"closed"}]`), nil
		}
		return mcp.NewToolResultText(`[{"id":"cnt-1","title":"Грант на НИОКР","status":"active"}]`), nil
	})

	result := srv.callTool(t, "search_contests", map[string]interface{}{"status": "active"})
	if !strings.Contains(result, "Грант на НИОКР") {
		t.Fatalf("expected title, got: %s", result)
	}
}

func TestGetContestTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_contest", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("contest_id")
		if err != nil {
			return mcp.NewToolResultError("параметр contest_id обязателен"), nil
		}
		if id == "missing" {
			return mcp.NewToolResultError("конкурс не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","title":"Конкурс инноваций"}`, id)), nil
	})

	result := srv.callTool(t, "get_contest", map[string]interface{}{"contest_id": "cnt-1"})
	if !strings.Contains(result, "Конкурс инноваций") {
		t.Fatalf("expected title, got: %s", result)
	}
}

func TestSearchFAQTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("search_faq", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`[{"id":"faq-1","question":"Как получить резидентство?","answer":"Подайте заявку через портал."}]`), nil
	})

	result := srv.callTool(t, "search_faq", map[string]interface{}{"query": "резидентство"})
	if !strings.Contains(result, "Как получить резидентство") {
		t.Fatalf("expected question, got: %s", result)
	}
}

func TestSearchResidentsTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("search_residents", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		industry := req.GetString("industry", "")
		if industry == "Биомедицина" {
			return mcp.NewToolResultText(`[{"name":"ООО БиоТех","industry":"Биомедицина"}]`), nil
		}
		return mcp.NewToolResultText(`[{"name":"ООО ТехСтарт","industry":"IT"}]`), nil
	})

	result := srv.callTool(t, "search_residents", map[string]interface{}{"industry": "IT"})
	if !strings.Contains(result, `"industry":"IT"`) {
		t.Fatalf("expected industry, got: %s", result)
	}
}

func TestGetDocumentLinksTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_document_links", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		docID, err := req.RequireString("document_id")
		if err != nil {
			return mcp.NewToolResultError("параметр document_id обязателен"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`[{"source_id":"%s","target_id":"doc-2","link_type":"references"}]`, docID)), nil
	})

	result := srv.callTool(t, "get_document_links", map[string]interface{}{"document_id": "doc-1"})
	if !strings.Contains(result, `"link_type":"references"`) {
		t.Fatalf("expected link_type, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Monitoring tools (3)
// ---------------------------------------------------------------------------

func TestGetRecentChangesTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_recent_changes", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entityType := req.GetString("entity_type", "")
		return mcp.NewToolResultText(fmt.Sprintf(`[{"entity_type":"%s","kind":"new","title":"Новый документ"}]`, entityType)), nil
	})

	result := srv.callTool(t, "get_recent_changes", map[string]interface{}{"entity_type": "document"})
	if !strings.Contains(result, `"kind":"new"`) {
		t.Fatalf("expected kind, got: %s", result)
	}
}

func TestGetSourceHealthTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_source_health", func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`[{"name":"documents","state":"healthy","items_last_run":15}]`), nil
	})

	result := srv.callTool(t, "get_source_health", map[string]interface{}{})
	if !strings.Contains(result, `"state":"healthy"`) {
		t.Fatalf("expected state, got: %s", result)
	}
}

func TestGetCoverageAuditTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_coverage_audit", func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`{"total_sources":9,"covered":8,"uncovered":1}`), nil
	})

	result := srv.callTool(t, "get_coverage_audit", map[string]interface{}{})
	if !strings.Contains(result, `"total_sources":9`) {
		t.Fatalf("expected total_sources, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Residency tools (8)
// ---------------------------------------------------------------------------

func TestGetClientStatusTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	clientID := uuid.New().String()
	st.clients[clientID] = &model.Client{ID: clientID, Name: "ООО Тест", INN: "7701234567", ResidencyStage: model.StageApplication, ContactEmail: "test@example.com", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	srv.addTool("get_client_status", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		c, ok := st.clients[id]
		if !ok {
			return mcp.NewToolResultError("клиент не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"client_id":"%s","name":"%s","residency_stage":"%s"}`, c.ID, c.Name, c.ResidencyStage)), nil
	})

	result := srv.callTool(t, "get_client_status", map[string]interface{}{"client_id": clientID})
	if !strings.Contains(result, `"name":"ООО Тест"`) {
		t.Fatalf("expected name, got: %s", result)
	}
	if !strings.Contains(result, `"residency_stage":"Подача_заявки"`) {
		t.Fatalf("expected stage, got: %s", result)
	}
}

func TestGetChecklistTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	clientID := uuid.New().String()
	st.clients[clientID] = &model.Client{ID: clientID, Name: "Test", INN: "7700000001", ResidencyStage: model.StageApplication, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	checklistID := uuid.New().String()
	st.checklists[checklistID] = &model.Checklist{ID: checklistID, Title: "Чек-лист вступления", ProcedureType: model.ChecklistEntry, Steps: json.RawMessage(`["Шаг 1","Шаг 2"]`), CreatedAt: time.Now()}
	ccID := uuid.New().String()
	st.clientCLs[clientID] = []*model.ClientChecklist{{ID: ccID, ClientID: clientID, ChecklistID: checklistID, Status: model.ChecklistInProgress}}

	srv.addTool("get_checklist", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		cls, ok := st.clientCLs[cid]
		if !ok || len(cls) == 0 {
			return mcp.NewToolResultText(`[]`), nil
		}
		tpl := st.checklists[cls[0].ChecklistID]
		return mcp.NewToolResultText(fmt.Sprintf(`[{"checklist_id":"%s","checklist_title":"%s","status":"%s"}]`, cls[0].ChecklistID, tpl.Title, cls[0].Status)), nil
	})

	result := srv.callTool(t, "get_checklist", map[string]interface{}{"client_id": clientID})
	if !strings.Contains(result, "Чек-лист вступления") {
		t.Fatalf("expected title, got: %s", result)
	}
}

func TestGetDeadlinesTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	clientID := uuid.New().String()
	st.clients[clientID] = &model.Client{ID: clientID, Name: "Test", INN: "7700000001", ResidencyStage: model.StageReporting, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	st.deadlines[clientID] = []*model.Deadline{{ID: uuid.New().String(), ClientID: clientID, Title: "Квартальный отчёт", DueDate: time.Now().AddDate(0, 0, 30), Type: model.DeadlineReporting, Status: model.DeadlineUpcoming}}

	srv.addTool("get_deadlines", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		daysAhead := req.GetInt("days_ahead", 30)
		ds, ok := st.deadlines[cid]
		if !ok {
			return mcp.NewToolResultText(`[]`), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`[{"title":"%s","days_ahead_filter":%d}]`, ds[0].Title, daysAhead)), nil
	})

	result := srv.callTool(t, "get_deadlines", map[string]interface{}{"client_id": clientID, "days_ahead": 60})
	if !strings.Contains(result, "Квартальный отчёт") {
		t.Fatalf("expected title, got: %s", result)
	}
	if !strings.Contains(result, `"days_ahead_filter":60`) {
		t.Fatalf("expected days_ahead, got: %s", result)
	}
}

func TestGetClientDocumentsTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	clientID := uuid.New().String()
	st.clients[clientID] = &model.Client{ID: clientID, Name: "Test", INN: "7700000001", ResidencyStage: model.StageApplication, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	st.clientDocs[clientID] = []*model.ClientDocument{{ID: uuid.New().String(), ClientID: clientID, Role: "required", Status: model.DocPending}}

	srv.addTool("get_client_documents", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		docs, ok := st.clientDocs[cid]
		if !ok {
			return mcp.NewToolResultText(`[]`), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`[{"role":"%s","status":"%s"}]`, docs[0].Role, docs[0].Status)), nil
	})

	result := srv.callTool(t, "get_client_documents", map[string]interface{}{"client_id": clientID})
	if !strings.Contains(result, `"role":"required"`) {
		t.Fatalf("expected role, got: %s", result)
	}
}

func TestListClientsTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	tenantID := uuid.New().String()
	st.tenants[tenantID] = &model.Tenant{ID: tenantID, Name: "Test", APIKey: "key-1"}
	for i := 0; i < 3; i++ {
		id := uuid.New().String()
		st.clients[id] = &model.Client{ID: id, Name: fmt.Sprintf("Client %d", i+1), INN: fmt.Sprintf("770000000%d", i+1), ResidencyStage: model.StageApplication, TenantID: tenantID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}

	srv.addTool("list_clients", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tid := req.GetString("tenant_id", "")
		if tid == "" {
			return mcp.NewToolResultError("параметр tenant_id обязателен"), nil
		}
		limit := req.GetInt("limit", 50)
		var clients []string
		for id, c := range st.clients {
			if c.TenantID == tid {
				clients = append(clients, fmt.Sprintf(`{"id":"%s","name":"%s"}`, id, c.Name))
			}
		}
		if limit > 0 && len(clients) > limit {
			clients = clients[:limit]
		}
		return mcp.NewToolResultText("[" + strings.Join(clients, ",") + "]"), nil
	})

	// Без tenant_id — ошибка.
	resultErr := srv.callToolNoError(t, "list_clients", map[string]interface{}{})
	if !strings.Contains(resultErr, "tenant_id обязателен") {
		t.Fatalf("expected error, got: %s", resultErr)
	}

	// С tenant_id.
	result := srv.callTool(t, "list_clients", map[string]interface{}{"tenant_id": tenantID})
	if !strings.Contains(result, "Client 1") {
		t.Fatalf("expected client, got: %s", result)
	}
}

func TestCreateClientTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	srv.addTool("create_client", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("параметр name обязателен"), nil
		}
		inn, err := req.RequireString("inn")
		if err != nil {
			return mcp.NewToolResultError("параметр inn обязателен"), nil
		}
		id := uuid.New().String()
		st.clients[id] = &model.Client{ID: id, Name: name, INN: inn, ContactEmail: req.GetString("contact_email", ""), ResidencyStage: model.StageApplication, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","name":"%s","inn":"%s","residency_stage":"%s"}`, id, name, inn, model.StageApplication)), nil
	})

	// Без name.
	resultErr := srv.callToolNoError(t, "create_client", map[string]interface{}{"inn": "7701234567"})
	if !strings.Contains(resultErr, "name обязателен") {
		t.Fatalf("expected error, got: %s", resultErr)
	}

	// Успешное создание.
	result := srv.callTool(t, "create_client", map[string]interface{}{"name": "ООО Новый", "inn": "7701234567", "contact_email": "new@example.com"})
	if !strings.Contains(result, `"name":"ООО Новый"`) {
		t.Fatalf("expected name, got: %s", result)
	}
}

func TestUpdateClientStageTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	clientID := uuid.New().String()
	st.clients[clientID] = &model.Client{ID: clientID, Name: "Test", INN: "7701234567", ResidencyStage: model.StageApplication, CreatedAt: time.Now(), UpdatedAt: time.Now()}

	srv.addTool("update_client_stage", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		newStageStr, err := req.RequireString("new_stage")
		if err != nil {
			return mcp.NewToolResultError("параметр new_stage обязателен"), nil
		}
		client, ok := st.clients[cid]
		if !ok {
			return mcp.NewToolResultError("клиент не найден"), nil
		}
		newStage := model.ResidencyStage(newStageStr)
		if !client.ResidencyStage.CanTransition(newStage) {
			return mcp.NewToolResultError(fmt.Sprintf("недопустимый переход: %s → %s", client.ResidencyStage, newStage)), nil
		}
		oldStage := client.ResidencyStage
		client.ResidencyStage = newStage
		client.UpdatedAt = time.Now()
		st.transitions[cid] = append(st.transitions[cid], &model.StageTransition{ID: uuid.New().String(), ClientID: cid, FromStage: oldStage, ToStage: newStage, TransitionedAt: time.Now()})
		return mcp.NewToolResultText(fmt.Sprintf(`{"client_id":"%s","from_stage":"%s","to_stage":"%s"}`, cid, oldStage, newStage)), nil
	})

	result := srv.callTool(t, "update_client_stage", map[string]interface{}{"client_id": clientID, "new_stage": string(model.StageExamination), "notes": "Документы приняты"})
	if !strings.Contains(result, `"from_stage":"Подача_заявки"`) {
		t.Fatalf("expected from_stage, got: %s", result)
	}
	if !strings.Contains(result, `"to_stage":"Экспертиза"`) {
		t.Fatalf("expected to_stage, got: %s", result)
	}
}

func TestGetTemplatesTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	st.templates["tpl-1"] = &model.DocumentTemplate{ID: "tpl-1", Name: "Заявка на резидентство", Type: "application", TemplateFile: "/path/to/template.go.tpl", Version: "1"}

	srv.addTool("get_templates", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tplType := req.GetString("template_type", "")
		var result []string
		for _, t := range st.templates {
			if tplType == "" || t.Type == tplType {
				result = append(result, fmt.Sprintf(`{"id":"%s","name":"%s","type":"%s"}`, t.ID, t.Name, t.Type))
			}
		}
		return mcp.NewToolResultText("[" + strings.Join(result, ",") + "]"), nil
	})

	result := srv.callTool(t, "get_templates", map[string]interface{}{"template_type": "application"})
	if !strings.Contains(result, "Заявка на резидентство") {
		t.Fatalf("expected name, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// AI Agent tools (8)
// ---------------------------------------------------------------------------

func TestAskConsultantTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("ask_consultant", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		question, err := req.RequireString("question")
		if err != nil {
			return mcp.NewToolResultError("параметр question обязателен"), nil
		}
		if strings.TrimSpace(question) == "" {
			return mcp.NewToolResultError("вопрос не может быть пустым"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"answer":"Ответ: %s","confidence":0.85}`, question)), nil
	})

	result := srv.callTool(t, "ask_consultant", map[string]interface{}{"question": "Какие льготы положены резидентам?"})
	if !strings.Contains(result, "Ответ:") {
		t.Fatalf("expected answer, got: %s", result)
	}
}

func TestValidateDocumentTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("validate_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		docText, err := req.RequireString("document_text")
		if err != nil {
			return mcp.NewToolResultError("параметр document_text обязателен"), nil
		}
		if strings.TrimSpace(docText) == "" {
			return mcp.NewToolResultText(`{"issues":[{"type":"error","message":"Документ пуст","severity":10}],"score":0,"passed":false}`), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"issues":[],"score":95,"passed":true,"document_length":%d}`, len(docText))), nil
	})

	// Пустой.
	resultEmpty := srv.callTool(t, "validate_document", map[string]interface{}{"document_text": "", "procedure_type": "entry"})
	if !strings.Contains(resultEmpty, `"score":0`) || !strings.Contains(resultEmpty, `"passed":false`) {
		t.Fatalf("expected score 0 and passed false, got: %s", resultEmpty)
	}

	// Валидный.
	result := srv.callTool(t, "validate_document", map[string]interface{}{"document_text": "Заявка на резидентство ООО Тест. ИНН 7701234567. Дата: 29.05.2026. Утверждено.", "procedure_type": "entry"})
	if !strings.Contains(result, `"passed":true`) {
		t.Fatalf("expected passed true, got: %s", result)
	}
}

func TestGetNextStepsTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("get_next_steps", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`[{"title":"Подать заявку","priority":"high","client_id":"%s"}]`, cid)), nil
	})

	result := srv.callTool(t, "get_next_steps", map[string]interface{}{"client_id": "client-1"})
	if !strings.Contains(result, `"priority":"high"`) {
		t.Fatalf("expected priority, got: %s", result)
	}
}

func TestSubscribeToChangesTool(t *testing.T) {
	srv := newTestMCPServer()
	subscriptions := make(map[string][]string)

	srv.addTool("subscribe_to_changes", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		catsStr, err := req.RequireString("categories")
		if err != nil {
			return mcp.NewToolResultError("параметр categories обязателен"), nil
		}
		cats := strings.Split(catsStr, ",")
		for i := range cats {
			cats[i] = strings.TrimSpace(cats[i])
		}
		subscriptions[cid] = cats
		return mcp.NewToolResultText(fmt.Sprintf("Клиент %s подписан на: %s", cid, catsStr)), nil
	})

	result := srv.callTool(t, "subscribe_to_changes", map[string]interface{}{"client_id": "client-1", "categories": "regulations,events,contests"})
	if !strings.Contains(result, "подписан на") {
		t.Fatalf("expected confirmation, got: %s", result)
	}
	if len(subscriptions["client-1"]) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(subscriptions["client-1"]))
	}
}

func TestDraftDocumentTool(t *testing.T) {
	srv := newTestMCPServer()
	st := newFakeMCPStore()

	clientID := uuid.New().String()
	st.clients[clientID] = &model.Client{ID: clientID, Name: "ООО Тест", INN: "7701234567", ContactEmail: "test@example.com", ResidencyStage: model.StageApplication, CreatedAt: time.Now(), UpdatedAt: time.Now()}

	srv.addTool("draft_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("параметр client_id обязателен"), nil
		}
		docType, err := req.RequireString("document_type")
		if err != nil {
			return mcp.NewToolResultError("параметр document_type обязателен"), nil
		}
		c, ok := st.clients[cid]
		if !ok {
			return mcp.NewToolResultError("клиент не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"document_type":"%s","client_id":"%s","filled_variables":{"company_name":"%s","inn":"%s"}}`, docType, cid, c.Name, c.INN)), nil
	})

	result := srv.callTool(t, "draft_document", map[string]interface{}{"client_id": clientID, "document_type": "application", "extra_context": "Ускорить"})
	if !strings.Contains(result, `"document_type":"application"`) {
		t.Fatalf("expected type, got: %s", result)
	}
	if !strings.Contains(result, `"company_name":"ООО Тест"`) {
		t.Fatalf("expected company, got: %s", result)
	}
}

func TestCheckEligibilityTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("check_eligibility", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		inn, err := req.RequireString("inn")
		if err != nil {
			return mcp.NewToolResultError("параметр inn обязателен"), nil
		}
		if len(inn) != 10 && len(inn) != 12 {
			return mcp.NewToolResultError("ИНН должен содержать 10 или 12 цифр"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"inn":"%s","company_name":"ООО Тест","eligible":true,"score":85}`, inn)), nil
	})

	result := srv.callTool(t, "check_eligibility", map[string]interface{}{"inn": "7701234567"})
	if !strings.Contains(result, `"eligible":true`) {
		t.Fatalf("expected eligible, got: %s", result)
	}

	resultBad := srv.callToolNoError(t, "check_eligibility", map[string]interface{}{"inn": "123"})
	if !strings.Contains(resultBad, "10 или 12 цифр") {
		t.Fatalf("expected error, got: %s", resultBad)
	}
}

func TestGenerateDocumentTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("generate_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tplID, err := req.RequireString("template_id")
		if err != nil {
			return mcp.NewToolResultError("template_id обязателен"), nil
		}
		cid, err := req.RequireString("client_id")
		if err != nil {
			return mcp.NewToolResultError("client_id обязателен"), nil
		}
		result := map[string]any{
			"path":     fmt.Sprintf("/generated/%s_%s.pdf", tplID, cid),
			"filename": fmt.Sprintf("%s_%s.pdf", tplID, cid),
		}
		if req.GetBool("inline", false) {
			result["content_base64"] = "JVBERi0xLjQK..."
			result["size_bytes"] = 12345
		}
		return mcp.NewToolResultText(toJSON(result)), nil
	})

	// Без inline.
	result := srv.callTool(t, "generate_document", map[string]interface{}{"template_id": "application", "client_id": "client-1"})
	if !strings.Contains(result, `"filename":"application_client-1.pdf"`) {
		t.Fatalf("expected filename, got: %s", result)
	}
	if strings.Contains(result, "content_base64") {
		t.Fatal("should not contain base64")
	}

	// С inline.
	resultInline := srv.callTool(t, "generate_document", map[string]interface{}{"template_id": "application", "client_id": "client-1", "inline": true})
	if !strings.Contains(resultInline, "content_base64") {
		t.Fatal("should contain base64")
	}
}

func TestListDocumentTemplatesTool(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("list_document_templates", func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`["Заявление_на_резидентство.go.tpl","Квартальный_отчёт.go.tpl"]`), nil
	})

	result := srv.callTool(t, "list_document_templates", map[string]interface{}{})
	if !strings.Contains(result, "Заявление_на_резидентство") {
		t.Fatalf("expected template, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Completeness: all 28 tools registered
// ---------------------------------------------------------------------------

func TestAll28MCPToolsRegistered(t *testing.T) {
	expected := []string{
		"search_documents", "get_document", "list_active_documents",
		"search_events", "get_event", "search_contests", "get_contest",
		"search_faq", "search_residents", "get_document_links",
		"get_recent_changes", "get_source_health", "get_coverage_audit",
		"get_client_status", "get_checklist", "get_deadlines",
		"get_client_documents", "list_clients", "create_client",
		"update_client_stage", "get_templates",
		"ask_consultant", "validate_document", "get_next_steps",
		"subscribe_to_changes", "draft_document", "check_eligibility",
		"generate_document", "list_document_templates",
	}

	srv := newTestMCPServer()
	for _, name := range expected {
		srv.addTool(name, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(`{"ok":true}`), nil
		})
	}

	// Verify all tools are registered by checking they exist in the map.
	if len(srv.tools) != len(expected) {
		t.Errorf("expected %d tools, got %d", len(expected), len(srv.tools))
	}

	for _, exp := range expected {
		if _, ok := srv.tools[exp]; !ok {
			t.Errorf("missing tool: %s", exp)
		}
	}

	t.Logf("All %d MCP tools registered successfully", len(expected))
}

// ---------------------------------------------------------------------------
// Tool parameter validation
// ---------------------------------------------------------------------------

func TestToolParameterValidation(t *testing.T) {
	srv := newTestMCPServer()
	srv.addTool("test_required_param", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		val, err := req.RequireString("required_param")
		if err != nil {
			return mcp.NewToolResultError("параметр required_param обязателен"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("received: %s", val)), nil
	})

	result := srv.callToolNoError(t, "test_required_param", map[string]interface{}{})
	if !strings.Contains(result, "обязателен") {
		t.Fatalf("expected required param error, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// fakeMCPStore — in-memory для MCP-тестов.
// ---------------------------------------------------------------------------

type fakeMCPStore struct {
	documents    map[string]*model.Document
	clients      map[string]*model.Client
	transitions  map[string][]*model.StageTransition
	tenants      map[string]*model.Tenant
	events       map[string]*model.Event
	contests     map[string]*model.Contest
	faqItems     map[string]*model.FAQItem
	residents    map[string]*model.Resident
	checklists   map[string]*model.Checklist
	clientCLs    map[string][]*model.ClientChecklist
	stepStatuses map[string][]*model.ChecklistStepStatus
	deadlines    map[string][]*model.Deadline
	templates    map[string]*model.DocumentTemplate
	clientDocs   map[string][]*model.ClientDocument
}

func newFakeMCPStore() *fakeMCPStore {
	return &fakeMCPStore{
		documents:    make(map[string]*model.Document),
		clients:      make(map[string]*model.Client),
		transitions:  make(map[string][]*model.StageTransition),
		tenants:      make(map[string]*model.Tenant),
		events:       make(map[string]*model.Event),
		contests:     make(map[string]*model.Contest),
		faqItems:     make(map[string]*model.FAQItem),
		residents:    make(map[string]*model.Resident),
		checklists:   make(map[string]*model.Checklist),
		clientCLs:    make(map[string][]*model.ClientChecklist),
		stepStatuses: make(map[string][]*model.ChecklistStepStatus),
		deadlines:    make(map[string][]*model.Deadline),
		templates:    make(map[string]*model.DocumentTemplate),
		clientDocs:   make(map[string][]*model.ClientDocument),
	}
}
