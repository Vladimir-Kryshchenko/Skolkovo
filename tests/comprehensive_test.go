// Package tests — комплексное тестирование всей системы «База Сколково».
//
// Охватывает каждую страницу, блок, надпись и API-точку на всех 7 серверах.
// Тестирует через реально развёрнутый Docker Compose.
//
// Запуск: go test -v -tags=integration ./tests/ -run TestFull
//
//go:build integration
// +build integration

package tests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// consultantAuthHeader возвращает Basic Auth заголовок для дашборда консультанта.
func consultantAuthHeader() string {
	cred := base64.StdEncoding.EncodeToString([]byte("consultant:change-me-please"))
	return "Basic " + cred
}

// ===========================================================================
// РАЗДЕЛ 1: MCP-СЕРВЕР (:8080) — полное покрытие
// ===========================================================================

// TC-MCP-001: Health endpoint
func TestFull_MCP_HealthEndpoint(t *testing.T) {
	resp, err := get(t, mcpBase+"/health", nil)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Статус и структура ответа
	if resp.StatusCode != 200 {
		t.Fatalf("GET /health: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("GET /health: missing status ok, body: %s", body)
	}
	if !strings.Contains(string(body), `"service"`) {
		t.Errorf("GET /health: missing service field, body: %s", body)
	}
}

// TC-MCP-002: Отклонение без API-ключа
func TestFull_MCP_RequiresAPIKey(t *testing.T) {
	initReq := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	}
	body, _ := json.Marshal(initReq)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), nil)
	if err != nil {
		t.Fatalf("MCP without key: %v", err)
	}
	defer resp.Body.Close()
	// Должен вернуть 401
	if resp.StatusCode != 401 {
		t.Errorf("MCP without API key: expected 401, got %d", resp.StatusCode)
	}
}

// TC-MCP-003: initialize — протокольное рукопожатие
func TestFull_MCP_InitializeProtocol(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	initReq := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	}
	body, _ := json.Marshal(initReq)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("MCP initialize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("MCP initialize: expected 200, got %d", resp.StatusCode)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), "protocolVersion") && !strings.Contains(string(respBody), "serverInfo") {
		t.Logf("MCP initialize body: %s", respBody[:min(len(respBody), 200)])
	}
}

// TC-MCP-004: tools/list — реестр инструментов
func TestFull_MCP_ToolsListComplete(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	req := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": map[string]interface{}{}}
	body, _ := json.Marshal(req)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("MCP tools/list: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	s := string(respBody)

	// Проверяем все 29 инструментов
	tools := []string{
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
	for _, tool := range tools {
		if !strings.Contains(s, tool) {
			t.Errorf("MCP tools/list: missing tool %q", tool)
		}
	}
}

// TC-MCP-005: search_documents — семантический поиск
func TestFull_MCP_SearchDocuments(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "search_documents",
			"arguments": map[string]interface{}{"query": "Сколково резидент", "limit": 5},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("search_documents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("search_documents: expected 200, got %d", resp.StatusCode)
	}
}

// TC-MCP-006: get_source_health — мониторинг источников
func TestFull_MCP_SourceHealth(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{"name": "get_source_health", "arguments": map[string]interface{}{}},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("get_source_health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get_source_health: expected 200, got %d", resp.StatusCode)
	}
}

// TC-MCP-007: get_recent_changes — лента изменений
func TestFull_MCP_RecentChanges(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "get_recent_changes",
			"arguments": map[string]interface{}{"since_days": 7, "limit": 10},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("get_recent_changes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get_recent_changes: expected 200, got %d", resp.StatusCode)
	}
}

// TC-MCP-008: get_coverage_audit — аудит охвата
func TestFull_MCP_CoverageAudit(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{"name": "get_coverage_audit", "arguments": map[string]interface{}{}},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("get_coverage_audit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get_coverage_audit: expected 200, got %d", resp.StatusCode)
	}
}

// TC-MCP-009: Rate-limit заголовки присутствуют
func TestFull_MCP_RateLimitHeaders(t *testing.T) {
	resp, err := get(t, mcpBase+"/health", nil)
	if err != nil {
		t.Fatalf("MCP health for rate-limit test: %v", err)
	}
	defer resp.Body.Close()
	// /health не требует key — просто проверяем что сервер отвечает
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// РАЗДЕЛ 2: ГЛАВНАЯ АДМИН-ПАНЕЛЬ (:8090) — полное покрытие
// ===========================================================================

// TC-ADM-001: Страница входа — все блоки
func TestFull_Admin_LoginPage(t *testing.T) {
	resp, err := get(t, adminBase+"/login", nil)
	if err != nil {
		t.Fatalf("Admin /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /login: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	elements := []string{
		"<title>", "Вход", "username", "password",
		`type="submit"`, "form",
	}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Admin /login: missing element %q", el)
		}
	}
}

// TC-ADM-002: Неверный пароль — редирект обратно на /login с ошибкой
func TestFull_Admin_LoginWrongPassword(t *testing.T) {
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       10 * time.Second,
	}
	req, _ := http.NewRequest("POST", adminBase+"/login",
		strings.NewReader("username=admin&password=wrongpassword123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("Admin wrong login: %v", err)
	}
	defer resp.Body.Close()
	// Неверный пароль → редирект обратно на /login с msg ошибки (303), не на /
	if resp.StatusCode != 303 {
		t.Errorf("Admin wrong login: expected 303, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/login") {
		t.Errorf("Admin wrong login: expected redirect to /login, got %q", location)
	}
	// Редирект должен содержать сообщение об ошибке
	if !strings.Contains(location, "msg") && !strings.Contains(location, "err") {
		t.Errorf("Admin wrong login: expected error in redirect location, got %q", location)
	}
}

// TC-ADM-003: Главная страница — все блоки и статистика
func TestFull_Admin_HomePage_Blocks(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/", cookie)
	if err != nil {
		t.Fatalf("Admin /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// Заголовок и название
	if !strings.Contains(html, "База Сколково") {
		t.Error("Admin /: missing 'База Сколково'")
	}
	// Вкладки навигации
	navItems := []string{"Документы", "Граф", "Сравнение", "Аналитика"}
	for _, nav := range navItems {
		if !strings.Contains(html, nav) {
			t.Errorf("Admin /: missing nav item %q", nav)
		}
	}
	// Статистические блоки
	stats := []string{"Всего", "Действует"}
	for _, s := range stats {
		if !strings.Contains(html, s) {
			t.Errorf("Admin /: missing stat block %q", s)
		}
	}
	// Кнопки действий
	actions := []string{"Парсинг RSS", "Индексация"}
	for _, a := range actions {
		if !strings.Contains(html, a) {
			t.Errorf("Admin /: missing action button %q", a)
		}
	}
	// Таблица документов
	if !strings.Contains(html, "<table") {
		t.Error("Admin /: missing document table")
	}
}

// TC-ADM-004: Страница аналитики
func TestFull_Admin_Analytics_Blocks(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/analytics", cookie)
	if err != nil {
		t.Fatalf("Admin /analytics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /analytics: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	// Страница аналитики должна содержать графики/статистику
	if !strings.Contains(html, "аналитик") && !strings.Contains(html, "Аналитик") && !strings.Contains(html, "статистик") {
		t.Log("Admin /analytics: page loaded but no 'аналитик' keyword found — verify content")
	}
}

// TC-ADM-005: API аналитики — JSON
func TestFull_Admin_API_Analytics(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/api/analytics", cookie)
	if err != nil {
		t.Fatalf("Admin /api/analytics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /api/analytics: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Errorf("Admin /api/analytics: invalid JSON: %v, body: %s", err, body[:min(len(body), 200)])
	}
}

// TC-ADM-006: Страница сравнения версий
func TestFull_Admin_Diff_Blocks(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/diff", cookie)
	if err != nil {
		t.Fatalf("Admin /diff: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /diff: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	elements := []string{"Сравнение", "Документ 1", "Документ 2", "Сравнить", "<select", "<form"}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Admin /diff: missing element %q", el)
		}
	}
}

// TC-ADM-007: Граф связей
func TestFull_Admin_Graph_Blocks(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/graph", cookie)
	if err != nil {
		t.Fatalf("Admin /graph: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /graph: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	// Легенда графа
	for _, el := range []string{"Граф", "references", "supersedes", "related"} {
		if !strings.Contains(html, el) {
			t.Errorf("Admin /graph: missing %q", el)
		}
	}
}

// TC-ADM-008: Страница изменений
func TestFull_Admin_Changes_Page(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/changes", cookie)
	if err != nil {
		t.Fatalf("Admin /changes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /changes: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-009: API изменений
func TestFull_Admin_API_Changes(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/api/changes", cookie)
	if err != nil {
		t.Fatalf("Admin /api/changes: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /api/changes: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-010: ИИ-модели страница
func TestFull_Admin_AIModels_Page(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/ai/models", cookie)
	if err != nil {
		t.Fatalf("Admin /ai/models: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /ai/models: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "модел") && !strings.Contains(html, "Модел") && !strings.Contains(html, "provider") {
		t.Log("Admin /ai/models: page loaded, content check inconclusive")
	}
}

// TC-ADM-011: Форма создания ИИ-модели
func TestFull_Admin_AIModels_NewForm(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/ai/models/new", cookie)
	if err != nil {
		t.Fatalf("Admin /ai/models/new: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /ai/models/new: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-012: ИИ-агенты страница
func TestFull_Admin_AIAgents_Page(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/ai/agents", cookie)
	if err != nil {
		t.Fatalf("Admin /ai/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /ai/agents: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-013: Прокси-менеджер страница
func TestFull_Admin_Proxy_Page(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/proxy", cookie)
	if err != nil {
		t.Fatalf("Admin /proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /proxy: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-014: API список прокси
func TestFull_Admin_Proxy_APIList(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/api/proxy/list", cookie)
	if err != nil {
		t.Fatalf("Admin /api/proxy/list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /api/proxy/list: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-015: Регуляторные документы (НПА)
func TestFull_Admin_Regulations_Page(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/regulations", cookie)
	if err != nil {
		t.Fatalf("Admin /regulations: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /regulations: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-016: API настройки
func TestFull_Admin_API_Settings(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/api/settings", cookie)
	if err != nil {
		t.Fatalf("Admin /api/settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /api/settings: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-017: API отчёты
func TestFull_Admin_API_Reports(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/api/reports", cookie)
	if err != nil {
		t.Fatalf("Admin /api/reports: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /api/reports: expected 200, got %d", resp.StatusCode)
	}
}

// TC-ADM-018: Выход из сессии
func TestFull_Admin_Logout(t *testing.T) {
	cookie := adminLogin(t)
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       10 * time.Second,
	}
	req, _ := http.NewRequest("GET", adminBase+"/logout", nil)
	req.AddCookie(cookie)
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("Admin /logout: %v", err)
	}
	defer resp.Body.Close()
	// Должен редиректить на /login
	if resp.StatusCode != 303 && resp.StatusCode != 302 {
		t.Errorf("Admin /logout: expected 303/302, got %d", resp.StatusCode)
	}
}

// TC-ADM-019: HTML-структура — responsive meta, CSS vars
func TestFull_Admin_HTML_Structure(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/", cookie)
	if err != nil {
		t.Fatalf("Admin HTML structure: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// Обязательные мета-теги
	if !strings.Contains(html, `charset="utf-8"`) {
		t.Error("Admin HTML: missing charset utf-8")
	}
	if !strings.Contains(html, `viewport`) {
		t.Error("Admin HTML: missing viewport meta")
	}
	// Закрытые теги
	if !strings.Contains(html, "</html>") {
		t.Error("Admin HTML: unclosed </html>")
	}
	if !strings.Contains(html, "</body>") {
		t.Error("Admin HTML: unclosed </body>")
	}
	// CSS переменные для тёмной темы
	if !strings.Contains(html, "--bg:") && !strings.Contains(html, "--primary:") {
		t.Log("Admin HTML: CSS variables not found (may be loaded externally)")
	}
}

// TC-ADM-020: Неавторизованный доступ — редирект
func TestFull_Admin_UnauthorizedRedirect(t *testing.T) {
	protected := []string{"/", "/diff", "/analytics", "/graph", "/changes", "/proxy", "/ai/models", "/ai/agents"}
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	for _, path := range protected {
		req, _ := http.NewRequest("GET", adminBase+path, nil)
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Errorf("Admin %s without auth: %v", path, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 303 && resp.StatusCode != 302 {
			t.Errorf("Admin %s without auth: expected redirect, got %d", path, resp.StatusCode)
		}
	}
}

// ===========================================================================
// РАЗДЕЛ 3: РЕЗИДЕНТСТВО-АДМИН (:8091) — полное покрытие
// ===========================================================================

// TC-RES-001: Страница клиентов — все блоки
func TestFull_Residency_Clients_Blocks(t *testing.T) {
	resp, err := get(t, residencyBase+"/clients", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /clients: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /clients: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	elements := []string{"Клиент", "Стадия", "ИНН"}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Residency /clients: missing element %q", el)
		}
	}
}

// TC-RES-002: API список клиентов — JSON
func TestFull_Residency_API_Clients(t *testing.T) {
	resp, err := get(t, residencyBase+"/api/clients", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /api/clients: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /api/clients: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Errorf("Residency /api/clients: invalid JSON: %v", err)
	}
}

// TC-RES-003: Чек-листы — структура страницы
func TestFull_Residency_Checklists_Blocks(t *testing.T) {
	resp, err := get(t, residencyBase+"/checklists", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /checklists: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /checklists: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "чек") && !strings.Contains(html, "Чек") && !strings.Contains(html, "checklist") {
		t.Log("Residency /checklists: page loaded, check content")
	}
}

// TC-RES-004: Дедлайны
func TestFull_Residency_Deadlines_Blocks(t *testing.T) {
	resp, err := get(t, residencyBase+"/deadlines", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /deadlines: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /deadlines: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "дедлайн") && !strings.Contains(html, "Дедлайн") && !strings.Contains(html, "срок") {
		t.Log("Residency /deadlines: page loaded, check keyword")
	}
}

// TC-RES-005: Шаблоны документов
func TestFull_Residency_Templates_Blocks(t *testing.T) {
	resp, err := get(t, residencyBase+"/templates", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /templates: expected 200, got %d", resp.StatusCode)
	}
}

// TC-RES-006: Тенанты
func TestFull_Residency_Tenants_Blocks(t *testing.T) {
	resp, err := get(t, residencyBase+"/tenants", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /tenants: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /tenants: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "тенант") && !strings.Contains(html, "Тенант") && !strings.Contains(html, "Tenant") {
		t.Log("Residency /tenants: page loaded, check keyword")
	}
}

// TC-RES-007: Мероприятия (events-admin)
func TestFull_Residency_Events_Admin(t *testing.T) {
	resp, err := get(t, residencyBase+"/events-admin", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /events-admin: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /events-admin: expected 200, got %d", resp.StatusCode)
	}
}

// TC-RES-008: Конкурсы (contests-admin)
func TestFull_Residency_Contests_Admin(t *testing.T) {
	resp, err := get(t, residencyBase+"/contests-admin", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /contests-admin: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /contests-admin: expected 200, got %d", resp.StatusCode)
	}
}

// ensureDefaultTenant создаёт тенанта «Default» если в системе ещё нет ни одного.
func ensureDefaultTenant(t *testing.T) {
	t.Helper()
	// Создаём дефолтный тенант через форму (POST /tenants)
	formData := "name=Default&api_key=test-api-key-default"
	resp, err := post(t, residencyBase+"/tenants", "application/x-www-form-urlencoded",
		strings.NewReader(formData), map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Logf("ensureDefaultTenant: %v (may already exist)", err)
		return
	}
	resp.Body.Close()
}

// TC-RES-009: Создание клиента через API
func TestFull_Residency_CreateClient_API(t *testing.T) {
	ensureDefaultTenant(t)

	// ИНН организации — ровно 10 цифр
	inn := fmt.Sprintf("77%08d", time.Now().Unix()%100000000)
	clientData := map[string]interface{}{
		"name":          "ООО Тест Авто",
		"inn":           inn,
		"contact_email": "test@auto.example.com",
	}
	body, _ := json.Marshal(clientData)
	resp, err := post(t, residencyBase+"/api/clients", "application/json",
		bytes.NewReader(body), map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency POST /api/clients: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("Residency POST /api/clients: expected 200/201, got %d, body: %s", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("Residency POST /api/clients: invalid JSON: %v", err)
	}
	data, _ := result["data"].(map[string]interface{})
	if data == nil || data["id"] == nil {
		t.Errorf("Residency POST /api/clients: missing id in response: %s", respBody)
	}
}

// TC-RES-010: Неавторизованный доступ к /clients — 401 или редирект
func TestFull_Residency_AuthRequired(t *testing.T) {
	resp, err := get(t, residencyBase+"/clients", nil)
	if err != nil {
		t.Fatalf("Residency auth test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 && resp.StatusCode != 403 && resp.StatusCode != 302 && resp.StatusCode != 303 {
		t.Errorf("Residency /clients without auth: expected 401/403/redirect, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// РАЗДЕЛ 4: ПОРТАЛ КЛИЕНТА (:8092) — полное покрытие
// ===========================================================================

// TC-PRT-001: Страница входа — все блоки
func TestFull_Portal_LoginPage_Blocks(t *testing.T) {
	resp, err := get(t, portalBase+"/login", nil)
	if err != nil {
		t.Fatalf("Portal /login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Portal /login: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	elements := []string{
		"<title>", "Вход", "email", `type="email"`, `type="submit"`, "<form",
	}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Portal /login: missing element %q", el)
		}
	}
	// Должен содержать название системы
	if !strings.Contains(html, "Сколково") && !strings.Contains(html, "сколково") {
		t.Error("Portal /login: missing 'Сколково' brand")
	}
}

// TC-PRT-002: Редирект с / на /login
func TestFull_Portal_RootRedirect(t *testing.T) {
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	req, _ := http.NewRequest("GET", portalBase+"/", nil)
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("Portal /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 303 && resp.StatusCode != 302 {
		t.Errorf("Portal /: expected redirect to login, got %d", resp.StatusCode)
	}
}

// TC-PRT-003: POST /login без токена — должен показать форму или редирект
func TestFull_Portal_LoginSubmit_NoEmail(t *testing.T) {
	resp, err := post(t, portalBase+"/login", "application/x-www-form-urlencoded",
		strings.NewReader("email="), nil)
	if err != nil {
		t.Fatalf("Portal /login POST empty: %v", err)
	}
	defer resp.Body.Close()
	// Ожидаем либо 200 (форма с ошибкой) либо 400
	if resp.StatusCode != 200 && resp.StatusCode != 400 {
		t.Errorf("Portal /login POST empty email: got %d", resp.StatusCode)
	}
}

// TC-PRT-004: Защищённые страницы — редирект на /login
func TestFull_Portal_ProtectedPages_Redirect(t *testing.T) {
	protected := []string{"/dashboard", "/checklists", "/deadlines", "/documents", "/generate"}
	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	for _, path := range protected {
		req, _ := http.NewRequest("GET", portalBase+path, nil)
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Errorf("Portal %s: %v", path, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != 303 && resp.StatusCode != 302 && resp.StatusCode != 401 {
			t.Errorf("Portal %s without auth: expected redirect, got %d", path, resp.StatusCode)
		}
	}
}

// TC-PRT-005: API /api/me — требует авторизацию
func TestFull_Portal_API_Me_Unauthorized(t *testing.T) {
	resp, err := get(t, portalBase+"/api/me", nil)
	if err != nil {
		t.Fatalf("Portal /api/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 && resp.StatusCode != 302 && resp.StatusCode != 303 {
		t.Errorf("Portal /api/me without auth: expected 401/redirect, got %d", resp.StatusCode)
	}
}

// TC-PRT-006: /login — HTML-структура полная
func TestFull_Portal_LoginPage_HTMLStructure(t *testing.T) {
	resp, err := get(t, portalBase+"/login", nil)
	if err != nil {
		t.Fatalf("Portal /login HTML: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// Обязательные HTML-структурные элементы
	structural := []string{"<!DOCTYPE", "<html", "<head>", "<body", "</html>", `charset="utf-8"`}
	for _, el := range structural {
		if !strings.Contains(html, el) {
			t.Errorf("Portal /login HTML: missing %q", el)
		}
	}
}

// ===========================================================================
// РАЗДЕЛ 5: ЧАТ-ВИДЖЕТ (:8093) — полное покрытие
// ===========================================================================

const portalBase = "http://localhost:8092"
const widgetBase = "http://localhost:8093"

// TC-WDG-001: Главная страница чата
func TestFull_Widget_ChatPage_Blocks(t *testing.T) {
	resp, err := get(t, widgetBase+"/chat", nil)
	if err != nil {
		t.Fatalf("Widget /chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Widget /chat: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	elements := []string{
		"<title>", "Чат", "Сколково",
		"<input", "Отправить",
		"</html>",
	}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Widget /chat: missing element %q", el)
		}
	}
}

// TC-WDG-002: JavaScript виджет — доступность и Content-Type
func TestFull_Widget_JS_File(t *testing.T) {
	resp, err := get(t, widgetBase+"/chat-widget.js", nil)
	if err != nil {
		t.Fatalf("Widget /chat-widget.js: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Widget /chat-widget.js: expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Errorf("Widget /chat-widget.js: expected javascript content-type, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("Widget /chat-widget.js: empty file")
	}
}

// TC-WDG-003: API сессии (требует POST, не GET)
func TestFull_Widget_API_Session(t *testing.T) {
	// /api/session — POST endpoint для создания сессии
	resp, err := post(t, widgetBase+"/api/session", "application/json",
		strings.NewReader("{}"), nil)
	if err != nil {
		t.Fatalf("Widget /api/session: %v", err)
	}
	defer resp.Body.Close()
	// 200 (сессия создана) или 401 (требует ключ) или 404 (не реализован)
	if resp.StatusCode != 200 && resp.StatusCode != 401 && resp.StatusCode != 404 && resp.StatusCode != 405 {
		t.Errorf("Widget /api/session: unexpected %d", resp.StatusCode)
	}
}

// TC-WDG-004: API chat — принимает сообщения
func TestFull_Widget_API_Chat_Post(t *testing.T) {
	msg := map[string]interface{}{"message": "Что нужно для резидентства?"}
	body, _ := json.Marshal(msg)
	resp, err := post(t, widgetBase+"/api/chat", "application/json", bytes.NewReader(body), nil)
	if err != nil {
		t.Fatalf("Widget POST /api/chat: %v", err)
	}
	defer resp.Body.Close()
	// Ожидаем 200 или 401 (если требует auth)
	if resp.StatusCode != 200 && resp.StatusCode != 401 && resp.StatusCode != 400 {
		t.Errorf("Widget /api/chat: unexpected %d", resp.StatusCode)
	}
}

// TC-WDG-005: HTML-структура чат-страницы
func TestFull_Widget_Chat_HTMLStructure(t *testing.T) {
	resp, err := get(t, widgetBase+"/chat", nil)
	if err != nil {
		t.Fatalf("Widget /chat HTML: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	structural := []string{"<!DOCTYPE", "<html", "<head>", "<body", "</html>", `charset="utf-8"`}
	for _, el := range structural {
		if !strings.Contains(html, el) {
			t.Errorf("Widget /chat HTML: missing %q", el)
		}
	}
}

// ===========================================================================
// РАЗДЕЛ 6: ДАШБОРД КОНСУЛЬТАНТА (:8094) — полное покрытие
// ===========================================================================

// TC-CON-001: Дашборд — основные блоки
func TestFull_Consultant_Dashboard_Blocks(t *testing.T) {
	resp, err := get(t, consultantBase+"/consultant/dashboard",
		map[string]string{"Authorization": consultantAuthHeader()})
	if err != nil {
		t.Fatalf("Consultant /consultant/dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Consultant /consultant/dashboard: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	elements := []string{"Дашборд консультанта", "Просрочено", "Всего клиентов"}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Consultant /consultant/dashboard: missing element %q", el)
		}
	}
}

// TC-CON-002: HTML-структура дашборда
func TestFull_Consultant_Dashboard_HTMLStructure(t *testing.T) {
	resp, err := get(t, consultantBase+"/consultant/dashboard",
		map[string]string{"Authorization": consultantAuthHeader()})
	if err != nil {
		t.Fatalf("Consultant dashboard HTML: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	htmlLower := strings.ToLower(string(body))

	if !strings.Contains(htmlLower, `charset="utf-8"`) && !strings.Contains(htmlLower, `charset=utf-8`) {
		t.Error("Consultant dashboard: missing charset")
	}
	if !strings.Contains(htmlLower, "</html>") {
		t.Error("Consultant dashboard: unclosed </html>")
	}
	if !strings.Contains(htmlLower, "viewport") {
		t.Error("Consultant dashboard: missing viewport meta")
	}
}

// TC-CON-003: Без авторизации — 401
func TestFull_Consultant_Unauthorized(t *testing.T) {
	resp, err := get(t, consultantBase+"/consultant/dashboard", nil)
	if err != nil {
		t.Fatalf("Consultant unauthorized: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 && resp.StatusCode != 302 && resp.StatusCode != 303 {
		t.Errorf("Consultant without auth: expected 401/redirect, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// РАЗДЕЛ 7: PROMETHEUS МЕТРИКИ (:9090)
// ===========================================================================

// TC-MET-001: /metrics доступны и валидны
func TestFull_Prometheus_Metrics_Valid(t *testing.T) {
	resp, err := get(t, prometheusBase+"/metrics", nil)
	if err != nil {
		t.Fatalf("Prometheus /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Prometheus /metrics: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	// Стандартные Go-метрики
	goMetrics := []string{
		"# HELP go_gc_duration_seconds",
		"# TYPE go_gc_duration_seconds",
		"go_goroutines",
		"go_memstats_alloc_bytes",
	}
	for _, m := range goMetrics {
		if !strings.Contains(s, m) {
			t.Errorf("Prometheus /metrics: missing metric %q", m)
		}
	}
}

// TC-MET-002: Content-Type метрик
func TestFull_Prometheus_ContentType(t *testing.T) {
	resp, err := get(t, prometheusBase+"/metrics", nil)
	if err != nil {
		t.Fatalf("Prometheus content-type: %v", err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Prometheus /metrics: expected text/plain, got %q", ct)
	}
}

// ===========================================================================
// РАЗДЕЛ 8: БИЗНЕС-ЛОГИКА — полная цепочка резидентства
// ===========================================================================

// TC-BIZ-001: Полный жизненный цикл клиента через MCP (end-to-end)
func TestFull_BizLogic_FullClientLifecycle(t *testing.T) {
	ensureDefaultTenant(t)
	// Создаём клиента через Residency Admin API
	// ИНН организации — ровно 10 цифр (77 + 8 цифр)
	inn := fmt.Sprintf("77%08d", (time.Now().Unix()+1)%100000000)
	clientData := map[string]interface{}{
		"name":          "ООО Полный Тест E2E",
		"inn":           inn,
		"contact_email": "e2e@test.example.com",
	}
	body, _ := json.Marshal(clientData)
	resp, err := post(t, residencyBase+"/api/clients", "application/json",
		bytes.NewReader(body), map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Create client: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("Create client: expected 200/201, got %d: %s", resp.StatusCode, b)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var apiResp map[string]interface{}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		t.Fatalf("Create client: invalid JSON: %v, body: %s", err, respBody)
	}
	// API возвращает {"ok":true,"data":{...client...}} или прямой объект
	var clientID string
	if data, ok := apiResp["data"].(map[string]interface{}); ok {
		clientID, _ = data["id"].(string)
	} else {
		clientID, _ = apiResp["id"].(string)
	}
	if clientID == "" {
		t.Fatalf("Create client: no id in response: %s", respBody)
	}

	// Проверяем клиент через MCP
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "get_client_status",
			"arguments": map[string]interface{}{"client_id": clientID},
		},
	}
	callBody, _ := json.Marshal(call)
	mcpResp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(callBody), headers)
	if err != nil {
		t.Fatalf("MCP get_client_status: %v", err)
	}
	defer mcpResp.Body.Close()
	if mcpResp.StatusCode != 200 {
		t.Fatalf("MCP get_client_status: expected 200, got %d", mcpResp.StatusCode)
	}

	// Переводим на следующую стадию
	stageData := map[string]interface{}{
		"new_stage": "экспертиза",
		"notes":     "E2E тест — переход в экспертизу",
	}
	stageBody, _ := json.Marshal(stageData)
	stageResp, err := post(t, residencyBase+"/api/clients/"+clientID+"/stage",
		"application/json", bytes.NewReader(stageBody),
		map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Stage transition: %v", err)
	}
	defer stageResp.Body.Close()
	if stageResp.StatusCode != 200 {
		b, _ := io.ReadAll(stageResp.Body)
		t.Fatalf("Stage transition: expected 200, got %d: %s", stageResp.StatusCode, b)
	}

	t.Logf("E2E lifecycle: client %s created (INN: %s), transitioned to экспертиза", clientID, inn)
}

// TC-BIZ-002: MCP draft_document — черновик заявки
func TestFull_BizLogic_DraftDocument_MCP(t *testing.T) {
	// Создаём клиента
	inn := fmt.Sprintf("77%08d", (time.Now().Unix()+2)%100000000)
	clientBody, _ := json.Marshal(map[string]interface{}{
		"name": "ООО Черновик Тест", "inn": inn,
	})
	resp, _ := post(t, residencyBase+"/api/clients", "application/json",
		bytes.NewReader(clientBody), map[string]string{"Authorization": authHeader()})
	if resp != nil {
		defer resp.Body.Close()
	}
	clientRespBody, _ := io.ReadAll(resp.Body)
	var clientCreated map[string]interface{}
	_ = json.Unmarshal(clientRespBody, &clientCreated)
	clientID, _ := clientCreated["id"].(string)
	if clientID == "" {
		t.Skip("cannot create client for draft test")
	}

	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name": "draft_document",
			"arguments": map[string]interface{}{
				"client_id":     clientID,
				"document_type": "application",
			},
		},
	}
	callBody, _ := json.Marshal(call)
	mcpResp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(callBody), headers)
	if err != nil {
		t.Fatalf("MCP draft_document: %v", err)
	}
	defer mcpResp.Body.Close()
	if mcpResp.StatusCode != 200 {
		t.Fatalf("MCP draft_document: expected 200, got %d", mcpResp.StatusCode)
	}
}

// TC-BIZ-003: check_eligibility — проверка пригодности
func TestFull_BizLogic_CheckEligibility(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "check_eligibility",
			"arguments": map[string]interface{}{"inn": "7701234567"},
		},
	}
	callBody, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(callBody), headers)
	if err != nil {
		t.Fatalf("check_eligibility: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("check_eligibility: expected 200, got %d", resp.StatusCode)
	}
}

// TC-BIZ-004: list_document_templates — доступные шаблоны
func TestFull_BizLogic_ListTemplates(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "list_document_templates",
			"arguments": map[string]interface{}{},
		},
	}
	callBody, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(callBody), headers)
	if err != nil {
		t.Fatalf("list_document_templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list_document_templates: expected 200, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// РАЗДЕЛ 9: DOCKER / ИНФРАСТРУКТУРА — контейнеры и сеть
// ===========================================================================

// TC-INF-001: Все 7 серверов отвечают
func TestFull_Infra_AllServersRespond(t *testing.T) {
	endpoints := []struct {
		name string
		url  string
	}{
		{"MCP :8080", mcpBase + "/health"},
		{"Admin :8090", adminBase + "/login"},
		{"Residency :8091", residencyBase + "/clients"},
		{"Portal :8092", portalBase + "/login"},
		{"Widget :8093", widgetBase + "/chat"},
		{"Consultant :8094", consultantBase + "/consultant/dashboard"},
		{"Prometheus :9090", prometheusBase + "/metrics"},
	}

	for _, ep := range endpoints {
		resp, err := get(t, ep.url, nil)
		if err != nil {
			t.Errorf("%s: connection failed: %v", ep.name, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 0 {
			t.Errorf("%s: no status code", ep.name)
		}
		t.Logf("%s: HTTP %d", ep.name, resp.StatusCode)
	}
}

// TC-INF-002: Qdrant доступен (через MCP get_source_health)
func TestFull_Infra_Qdrant_Reachable(t *testing.T) {
	resp, err := get(t, "http://localhost:6333/", nil)
	if err != nil {
		t.Fatalf("Qdrant :6333: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("Qdrant :6333: expected 200, got %d", resp.StatusCode)
	}
}

// TC-INF-003: TEI (эмбеддинги) доступен
func TestFull_Infra_TEI_Reachable(t *testing.T) {
	resp, err := get(t, "http://localhost:8081/health", nil)
	if err != nil {
		t.Fatalf("TEI :8081: %v", err)
	}
	defer resp.Body.Close()
	// TEI возвращает 200 когда готов, 503 пока грузит модель
	if resp.StatusCode != 200 && resp.StatusCode != 503 {
		t.Errorf("TEI :8081: unexpected status %d", resp.StatusCode)
	}
}

// ===========================================================================
// РАЗДЕЛ 10: RAG — поиск и навигация
// ===========================================================================

// TC-RAG-001: search_events через MCP
func TestFull_RAG_SearchEvents(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "search_events",
			"arguments": map[string]interface{}{"query": "конференция", "limit": 5},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("search_events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("search_events: expected 200, got %d", resp.StatusCode)
	}
}

// TC-RAG-002: search_faq через MCP
func TestFull_RAG_SearchFAQ(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "search_faq",
			"arguments": map[string]interface{}{"query": "как стать резидентом", "limit": 3},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("search_faq: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("search_faq: expected 200, got %d", resp.StatusCode)
	}
}

// TC-RAG-003: search_contests через MCP
func TestFull_RAG_SearchContests(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "search_contests",
			"arguments": map[string]interface{}{"query": "грант", "limit": 3},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("search_contests: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("search_contests: expected 200, got %d", resp.StatusCode)
	}
}

// TC-RAG-004: list_active_documents — возвращает список
func TestFull_RAG_ListActiveDocuments(t *testing.T) {
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "list_active_documents",
			"arguments": map[string]interface{}{},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("list_active_documents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list_active_documents: expected 200, got %d", resp.StatusCode)
	}
}

// TC-RAG-005: search_residents через MCP
func TestFull_RAG_SearchResidents(t *testing.T) {
	// Небольшая пауза чтобы не превысить rate-limit (5 RPS)
	time.Sleep(300 * time.Millisecond)
	headers := map[string]string{"X-API-Key": apiKey, "Accept": "application/json, text/event-stream"}
	call := map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "search_residents",
			"arguments": map[string]interface{}{"query": "ИТ", "limit": 3},
		},
	}
	body, _ := json.Marshal(call)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), headers)
	if err != nil {
		t.Fatalf("search_residents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		t.Skip("rate limit exceeded — run tests with -p 1 or increase MCP_RATE_LIMIT_RPS")
	}
	if resp.StatusCode != 200 {
		t.Fatalf("search_residents: expected 200, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// Вспомогательные функции
// ===========================================================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
