// Package tests — тесты бизнес-логики: полные рабочие процессы.
package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"

	"baza-skolkovo/src/common/model"
)

// ---------------------------------------------------------------------------
// Business workflow test harness
// ---------------------------------------------------------------------------

type workflowStore struct {
	clients       map[string]*model.Client
	transitions   map[string][]*model.StageTransition
	documents     map[string]*model.Document
	checklists    map[string]*model.Checklist
	clientCLs     map[string][]*model.ClientChecklist
	deadlines     map[string][]*model.Deadline
	templates     map[string]*model.DocumentTemplate
	subscriptions map[string][]string
}

func newWorkflowStore() *workflowStore {
	return &workflowStore{
		clients:       make(map[string]*model.Client),
		transitions:   make(map[string][]*model.StageTransition),
		documents:     make(map[string]*model.Document),
		checklists:    make(map[string]*model.Checklist),
		clientCLs:     make(map[string][]*model.ClientChecklist),
		deadlines:     make(map[string][]*model.Deadline),
		templates:     make(map[string]*model.DocumentTemplate),
		subscriptions: make(map[string][]string),
	}
}

func registerWorkflowTools(srv *testMCPServer, store *workflowStore) {
	srv.addTool("create_client", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		inn, _ := req.RequireString("inn")
		id := uuid.New().String()
		c := &model.Client{
			ID: id, Name: name, INN: inn, ContactEmail: req.GetString("contact_email", ""),
			TenantID: req.GetString("tenant_id", ""), ResidencyStage: model.StageApplication,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		store.clients[id] = c
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","name":"%s","inn":"%s","stage":"%s"}`, id, name, inn, model.StageApplication)), nil
	})

	srv.addTool("update_client_stage", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, _ := req.RequireString("client_id")
		newStageStr, _ := req.RequireString("new_stage")
		c, ok := store.clients[cid]
		if !ok {
			return mcp.NewToolResultError("клиент не найден"), nil
		}
		newStage := model.ResidencyStage(newStageStr)
		if !c.ResidencyStage.CanTransition(newStage) {
			return mcp.NewToolResultError(fmt.Sprintf("недопустимый переход: %s → %s", c.ResidencyStage, newStage)), nil
		}
		oldStage := c.ResidencyStage
		c.ResidencyStage = newStage
		c.UpdatedAt = time.Now()
		store.transitions[cid] = append(store.transitions[cid], &model.StageTransition{
			ID: uuid.New().String(), ClientID: cid, FromStage: oldStage, ToStage: newStage,
			TransitionedAt: time.Now(), Notes: req.GetString("notes", ""),
		})
		return mcp.NewToolResultText(fmt.Sprintf(`{"client_id":"%s","from":"%s","to":"%s"}`, cid, oldStage, newStage)), nil
	})

	srv.addTool("get_client_status", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, _ := req.RequireString("client_id")
		c, ok := store.clients[cid]
		if !ok {
			return mcp.NewToolResultError("клиент не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","name":"%s","inn":"%s","stage":"%s","updated_at":"%s"}`, c.ID, c.Name, c.INN, c.ResidencyStage, c.UpdatedAt.Format(time.RFC3339))), nil
	})

	srv.addTool("get_checklist", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, _ := req.RequireString("client_id")
		cls, ok := store.clientCLs[cid]
		if !ok || len(cls) == 0 {
			return mcp.NewToolResultText(`[]`), nil
		}
		var items []string
		for _, cl := range cls {
			tpl := store.checklists[cl.ChecklistID]
			title := ""
			if tpl != nil {
				title = tpl.Title
			}
			items = append(items, fmt.Sprintf(`{"checklist_id":"%s","title":"%s","status":"%s"}`, cl.ChecklistID, title, cl.Status))
		}
		return mcp.NewToolResultText("[" + strings.Join(items, ",") + "]"), nil
	})

	srv.addTool("get_deadlines", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, _ := req.RequireString("client_id")
		ds, ok := store.deadlines[cid]
		if !ok {
			return mcp.NewToolResultText(`[]`), nil
		}
		var items []string
		for _, d := range ds {
			items = append(items, fmt.Sprintf(`{"title":"%s","type":"%s","status":"%s"}`, d.Title, d.Type, d.Status))
		}
		return mcp.NewToolResultText("[" + strings.Join(items, ",") + "]"), nil
	})

	srv.addTool("search_documents", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		limit := req.GetInt("limit", 5)
		var results []string
		for id, doc := range store.documents {
			if doc.Status != model.StatusActive {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(doc.Title), strings.ToLower(query)) {
				continue
			}
			results = append(results, fmt.Sprintf(`{"id":"%s","title":"%s","status":"%s"}`, id, doc.Title, doc.Status))
			if len(results) >= limit {
				break
			}
		}
		if len(results) == 0 {
			return mcp.NewToolResultText(`[]`), nil
		}
		return mcp.NewToolResultText("[" + strings.Join(results, ",") + "]"), nil
	})

	srv.addTool("get_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, _ := req.RequireString("id")
		doc, ok := store.documents[id]
		if !ok {
			return mcp.NewToolResultError("документ не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"id":"%s","title":"%s","status":"%s","category":"%s"}`, doc.ID, doc.Title, doc.Status, doc.Category)), nil
	})

	srv.addTool("list_active_documents", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		category := req.GetString("category", "")
		var items []string
		for id, doc := range store.documents {
			if doc.Status != model.StatusActive {
				continue
			}
			if category != "" && doc.Category != category {
				continue
			}
			items = append(items, fmt.Sprintf(`{"id":"%s","title":"%s","category":"%s"}`, id, doc.Title, doc.Category))
		}
		return mcp.NewToolResultText("[" + strings.Join(items, ",") + "]"), nil
	})

	srv.addTool("ask_consultant", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		question, _ := req.RequireString("question")
		return mcp.NewToolResultText(fmt.Sprintf(`{"answer":"Ответ: %s","confidence":0.85}`, question)), nil
	})

	srv.addTool("validate_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		docText, _ := req.RequireString("document_text")
		procType := req.GetString("procedure_type", "")
		if strings.TrimSpace(docText) == "" {
			return mcp.NewToolResultText(`{"issues":[{"type":"error","message":"Документ пуст","severity":10}],"score":0,"passed":false}`), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"issues":[],"score":95,"passed":true,"procedure":"%s"}`, procType)), nil
	})

	srv.addTool("draft_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, _ := req.RequireString("client_id")
		docType, _ := req.RequireString("document_type")
		c, ok := store.clients[cid]
		if !ok {
			return mcp.NewToolResultError("клиент не найден"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf(`{"document_type":"%s","client_id":"%s","filled_variables":{"company_name":"%s","inn":"%s"}}`, docType, cid, c.Name, c.INN)), nil
	})

	srv.addTool("generate_document", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tplID, _ := req.RequireString("template_id")
		cid, _ := req.RequireString("client_id")
		result := map[string]any{
			"path":     fmt.Sprintf("/generated/%s_%s.pdf", tplID, cid),
			"filename": fmt.Sprintf("%s_%s.pdf", tplID, cid),
		}
		if req.GetBool("inline", false) {
			result["content_base64"] = "JVBERi0xLjQK..."
			result["size_bytes"] = 45678
		}
		return mcp.NewToolResultText(toJSON(result)), nil
	})

	srv.addTool("get_source_health", func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`[{"name":"documents","state":"healthy"},{"name":"events","state":"healthy"},{"name":"contests","state":"healthy"}]`), nil
	})

	srv.addTool("get_recent_changes", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText(`[{"entity_type":"document","kind":"new","title":"Новый документ"},{"entity_type":"event","kind":"new","title":"Новое мероприятие"}]`), nil
	})

	srv.addTool("subscribe_to_changes", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cid, _ := req.RequireString("client_id")
		catsStr, _ := req.RequireString("categories")
		cats := strings.Split(catsStr, ",")
		for i := range cats {
			cats[i] = strings.TrimSpace(cats[i])
		}
		store.subscriptions[cid] = cats
		return mcp.NewToolResultText(fmt.Sprintf("Клиент %s подписан на: %s", cid, catsStr)), nil
	})
}

func callTool(t *testing.T, srv *testMCPServer, name string, args map[string]interface{}) string {
	t.Helper()
	return srv.callTool(t, name, args)
}

func assertWorkflowContains(t *testing.T, body, substr, context string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Fatalf("%s: expected %q in body, got: %s", context, substr, body)
	}
}

func extractField(jsonStr, field string) string {
	key := fmt.Sprintf(`"%s":"`, field)
	idx := strings.Index(jsonStr, key)
	if idx == -1 {
		return ""
	}
	start := idx + len(key)
	end := strings.Index(jsonStr[start:], `"`)
	if end == -1 {
		return ""
	}
	return jsonStr[start : start+end]
}

// ---------------------------------------------------------------------------
// Workflow 1: Client lifecycle
// ---------------------------------------------------------------------------

func TestWorkflow_ClientLifecycle(t *testing.T) {
	store := newWorkflowStore()
	srv := newTestMCPServer()
	registerWorkflowTools(srv, store)

	result := callTool(t, srv, "create_client", map[string]interface{}{
		"name":          "ООО Инновации Плюс",
		"inn":           "7701234567",
		"contact_email": "innovations@example.com",
		"tenant_id":     "tenant-1",
	})
	assertWorkflowContains(t, result, "ООО Инновации Плюс", "create_client: name")
	assertWorkflowContains(t, result, "7701234567", "create_client: inn")
	assertWorkflowContains(t, result, `"stage":"подача_заявки"`, "create_client: initial stage")

	clientID := extractField(result, "id")
	if clientID == "" {
		t.Fatal("could not extract client_id")
	}

	result2 := callTool(t, srv, "update_client_stage", map[string]interface{}{
		"client_id": clientID,
		"new_stage": string(model.StageExamination),
		"notes":     "Документы приняты к экспертизе",
	})
	assertWorkflowContains(t, result2, `"from":"подача_заявки"`, "update_stage: from")
	assertWorkflowContains(t, result2, `"to":"экспертиза"`, "update_stage: to")

	result3 := callTool(t, srv, "update_client_stage", map[string]interface{}{
		"client_id": clientID,
		"new_stage": string(model.StageDecision),
	})
	assertWorkflowContains(t, result3, `"to":"решение"`, "update_stage: decision")

	result4 := callTool(t, srv, "get_client_status", map[string]interface{}{"client_id": clientID})
	assertWorkflowContains(t, result4, `"stage":"решение"`, "get_status: stage")
	assertWorkflowContains(t, result4, "ООО Инновации Плюс", "get_status: name")

	callTool(t, srv, "get_checklist", map[string]interface{}{"client_id": clientID})
	callTool(t, srv, "get_deadlines", map[string]interface{}{"client_id": clientID})

	if len(store.transitions[clientID]) != 2 {
		t.Errorf("expected 2 transitions, got %d", len(store.transitions[clientID]))
	}

	t.Log("Client lifecycle workflow completed")
}

// ---------------------------------------------------------------------------
// Workflow 2: Document workflow
// ---------------------------------------------------------------------------

func TestWorkflow_DocumentFlow(t *testing.T) {
	store := newWorkflowStore()
	srv := newTestMCPServer()
	registerWorkflowTools(srv, store)

	doc1ID := "doc-001"
	store.documents[doc1ID] = &model.Document{ID: doc1ID, Title: "Положение о Фонде v1.0", Status: model.StatusActive, Category: "Регламенты", SourceURL: "https://sk.ru/doc1", FetchedAt: time.Now()}

	doc2ID := "doc-002"
	store.documents[doc2ID] = &model.Document{ID: doc2ID, Title: "Положение о Фонде v2.0", Status: model.StatusActive, Category: "Регламенты", SourceURL: "https://sk.ru/doc2", FetchedAt: time.Now(), Supersedes: doc1ID}

	doc3ID := "doc-003"
	store.documents[doc3ID] = &model.Document{ID: doc3ID, Title: "Архивный документ", Status: model.StatusOutdated, Category: "Архив"}

	result := callTool(t, srv, "search_documents", map[string]interface{}{"query": "Положение", "limit": 10})
	assertWorkflowContains(t, result, "Положение о Фонде", "search: should find")

	result2 := callTool(t, srv, "get_document", map[string]interface{}{"id": doc1ID})
	assertWorkflowContains(t, result2, `"title":"Положение о Фонде v1.0"`, "get_document: title")
	assertWorkflowContains(t, result2, `"status":"действует"`, "get_document: status")

	result3 := callTool(t, srv, "list_active_documents", map[string]interface{}{})
	assertWorkflowContains(t, result3, "Положение о Фонде v1.0", "list_active: doc1")
	assertWorkflowContains(t, result3, "Положение о Фонде v2.0", "list_active: doc2")
	if strings.Contains(result3, "Архивный документ") {
		t.Error("list_active should not include outdated")
	}

	result4 := callTool(t, srv, "list_active_documents", map[string]interface{}{"category": "Регламенты"})
	assertWorkflowContains(t, result4, "Положение", "list_active filtered")

	doc2 := store.documents[doc2ID]
	if doc2.Supersedes != doc1ID {
		t.Error("doc2 should supersedes doc1")
	}

	t.Log("Document workflow completed")
}

// ---------------------------------------------------------------------------
// Workflow 3: AI workflow
// ---------------------------------------------------------------------------

func TestWorkflow_AIWorkflow(t *testing.T) {
	store := newWorkflowStore()
	srv := newTestMCPServer()
	registerWorkflowTools(srv, store)

	result0 := callTool(t, srv, "create_client", map[string]interface{}{"name": "ООО ИИ Тест", "inn": "7709876543"})
	clientID := extractField(result0, "id")

	result := callTool(t, srv, "ask_consultant", map[string]interface{}{"question": "Какие документы нужны?", "client_id": clientID})
	assertWorkflowContains(t, result, "Ответ:", "ask_consultant: answer")
	assertWorkflowContains(t, result, `"confidence":0.85`, "ask_consultant: confidence")

	result2 := callTool(t, srv, "validate_document", map[string]interface{}{
		"document_text":  "Заявка на резидентство Сколково. Организация: ООО ИИ Тест. ИНН: 7709876543. Дата: 30.05.2026. Утверждено.",
		"procedure_type": "entry", "client_id": clientID,
	})
	assertWorkflowContains(t, result2, `"passed":true`, "validate: pass")
	assertWorkflowContains(t, result2, `"score":95`, "validate: score")

	result2b := callTool(t, srv, "validate_document", map[string]interface{}{"document_text": "", "procedure_type": "entry"})
	assertWorkflowContains(t, result2b, `"passed":false`, "validate: fail empty")
	assertWorkflowContains(t, result2b, `"score":0`, "validate: empty score")

	result3 := callTool(t, srv, "draft_document", map[string]interface{}{"client_id": clientID, "document_type": "application", "extra_context": "Ускорить"})
	assertWorkflowContains(t, result3, `"document_type":"application"`, "draft: type")
	assertWorkflowContains(t, result3, `"company_name":"ООО ИИ Тест"`, "draft: company")
	assertWorkflowContains(t, result3, `"inn":"7709876543"`, "draft: inn")

	result4 := callTool(t, srv, "generate_document", map[string]interface{}{"template_id": "Заявление_на_резидентство.go.tpl", "client_id": clientID})
	assertWorkflowContains(t, result4, `Заявление_на_резидентство.go.tpl`, "generate: filename")
	if strings.Contains(result4, "content_base64") {
		t.Error("should not contain base64 without inline")
	}

	result4b := callTool(t, srv, "generate_document", map[string]interface{}{"template_id": "Заявление_на_резидентство.go.tpl", "client_id": clientID, "inline": true})
	assertWorkflowContains(t, result4b, "content_base64", "generate inline: base64")
	assertWorkflowContains(t, result4b, "size_bytes", "generate inline: size")

	t.Log("AI workflow completed")
}

// ---------------------------------------------------------------------------
// Workflow 4: Monitoring workflow
// ---------------------------------------------------------------------------

func TestWorkflow_MonitoringWorkflow(t *testing.T) {
	store := newWorkflowStore()
	srv := newTestMCPServer()
	registerWorkflowTools(srv, store)

	result := callTool(t, srv, "get_source_health", map[string]interface{}{})
	assertWorkflowContains(t, result, `"name":"documents"`, "health: documents")
	assertWorkflowContains(t, result, `"state":"healthy"`, "health: healthy")
	assertWorkflowContains(t, result, `"name":"events"`, "health: events")
	assertWorkflowContains(t, result, `"name":"contests"`, "health: contests")

	result2 := callTool(t, srv, "get_recent_changes", map[string]interface{}{"since_days": 7, "entity_type": "document", "limit": 20})
	assertWorkflowContains(t, result2, `"kind":"new"`, "changes: new")
	assertWorkflowContains(t, result2, `"entity_type":"document"`, "changes: entity")

	result3 := callTool(t, srv, "subscribe_to_changes", map[string]interface{}{"client_id": "client-monitor-1", "categories": "regulations,events,contests,reporting"})
	assertWorkflowContains(t, result3, "подписан на", "subscribe: confirmation")

	subs := store.subscriptions["client-monitor-1"]
	if len(subs) != 4 {
		t.Errorf("expected 4 subscriptions, got %d", len(subs))
	}
	for _, cat := range []string{"regulations", "events", "contests", "reporting"} {
		found := false
		for _, s := range subs {
			if s == cat {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subscription: %s", cat)
		}
	}

	t.Log("Monitoring workflow completed")
}

// ---------------------------------------------------------------------------
// Workflow 5: Full residency lifecycle
// ---------------------------------------------------------------------------

func TestWorkflow_FullResidencyLifecycle(t *testing.T) {
	store := newWorkflowStore()
	srv := newTestMCPServer()
	registerWorkflowTools(srv, store)

	transitions := []model.ResidencyStage{
		model.StageApplication, model.StageExamination, model.StageDecision,
		model.StageContract, model.StageResident, model.StageReporting,
		model.StageExtension, model.StageExit,
	}

	result := callTool(t, srv, "create_client", map[string]interface{}{"name": "ООО Полный Цикл", "inn": "7705555555"})
	clientID := extractField(result, "id")

	for i := 1; i < len(transitions); i++ {
		from := transitions[i-1]
		to := transitions[i]
		result := callTool(t, srv, "update_client_stage", map[string]interface{}{
			"client_id": clientID, "new_stage": string(to),
			"notes": fmt.Sprintf("Переход %s → %s", from, to),
		})
		assertWorkflowContains(t, result, fmt.Sprintf(`"from":"%s"`, from), fmt.Sprintf("transition %d: from", i))
		assertWorkflowContains(t, result, fmt.Sprintf(`"to":"%s"`, to), fmt.Sprintf("transition %d: to", i))
	}

	finalStatus := callTool(t, srv, "get_client_status", map[string]interface{}{"client_id": clientID})
	assertWorkflowContains(t, finalStatus, `"stage":"выход"`, "final stage: exit")

	if len(store.transitions[clientID]) != 7 {
		t.Errorf("expected 7 transitions, got %d", len(store.transitions[clientID]))
	}

	t.Log("Full lifecycle completed")
}

// ---------------------------------------------------------------------------
// Workflow 6: Multi-client with document filtering
// ---------------------------------------------------------------------------

func TestWorkflow_MultiClientDocumentFlow(t *testing.T) {
	store := newWorkflowStore()
	srv := newTestMCPServer()
	registerWorkflowTools(srv, store)

	clients := []struct{ name, inn string }{
		{"ООО Альфа", "7701111111"},
		{"ООО Бета", "7702222222"},
		{"ООО Гамма", "7703333333"},
	}

	var clientIDs []string
	for _, c := range clients {
		result := callTool(t, srv, "create_client", map[string]interface{}{"name": c.name, "inn": c.inn})
		id := extractField(result, "id")
		clientIDs = append(clientIDs, id)
	}

	if len(clientIDs) != 3 {
		t.Fatalf("expected 3 clients, got %d", len(clientIDs))
	}

	categories := map[string][]string{
		"Регламенты": {"Положение о Фонде", "Правила внутреннего распорядка"},
		"НПА":        {"Федеральный закон 244-ФЗ", "Постановление Правительства"},
		"Льготы":     {"Налоговые льготы резидентов", "Таможенные льготы"},
	}

	var docCount int
	for cat, titles := range categories {
		for _, title := range titles {
			id := fmt.Sprintf("doc-%s-%d", cat, docCount)
			store.documents[id] = &model.Document{ID: id, Title: title, Status: model.StatusActive, Category: cat, FetchedAt: time.Now()}
			docCount++
		}
	}

	result := callTool(t, srv, "search_documents", map[string]interface{}{"query": "", "limit": 20})
	docCountInResult := strings.Count(result, `"id":"doc-`)
	if docCountInResult != 6 {
		t.Errorf("expected 6 documents, got %d", docCountInResult)
	}

	result2 := callTool(t, srv, "list_active_documents", map[string]interface{}{"category": "НПА"})
	assertWorkflowContains(t, result2, "244-ФЗ", "list_active НПА: 244-ФЗ")
	assertWorkflowContains(t, result2, "Постановление", "list_active НПА: Постановление")

	for i, cid := range clientIDs {
		status := callTool(t, srv, "get_client_status", map[string]interface{}{"client_id": cid})
		assertWorkflowContains(t, status, clients[i].name, fmt.Sprintf("client %d: name", i))
		assertWorkflowContains(t, status, clients[i].inn, fmt.Sprintf("client %d: inn", i))
	}

	t.Log("Multi-client workflow completed")
}
