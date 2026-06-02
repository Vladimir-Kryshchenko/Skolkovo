// Integration tests for HTTP routes — test the running Docker Compose system.
// Run with: go test -v -tags=integration ./tests/
// Requires Docker Compose running: docker compose -f deploy/docker-compose.yml up -d
//go:build integration
// +build integration

package tests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	mcpBase        = "http://localhost:8080"
	adminBase      = "http://localhost:8090"
	residencyBase  = "http://localhost:8091"
	consultantBase = "http://localhost:8094"
	prometheusBase = "http://localhost:9090"
	apiKey         = "517a4b18d8701532ce5e9d50671395b8602a9f9e68691f1d"
	adminUser      = "admin"
	adminPass      = "change-me-please"
)

func authHeader() string {
	cred := base64.StdEncoding.EncodeToString([]byte(adminUser + ":" + adminPass))
	return "Basic " + cred
}

// adminLogin выполняет вход в сессионную админку и возвращает cookie сессии.
func adminLogin(t *testing.T) *http.Cookie {
	t.Helper()
	noRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest("POST", adminBase+"/login",
		strings.NewReader("username="+adminUser+"&password="+adminPass))
	if err != nil {
		t.Fatalf("adminLogin: build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("adminLogin: %v", err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "admin_session" {
			return c
		}
	}
	t.Fatal("adminLogin: no session cookie returned")
	return nil
}

// getWithCookie выполняет GET-запрос с cookie сессии.
func getWithCookie(t *testing.T, url string, cookie *http.Cookie) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(cookie)
	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

func get(t *testing.T, url string, headers map[string]string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

func post(t *testing.T, url string, contentType string, body io.Reader, headers map[string]string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// ===========================================================================
// MCP Server (:8080)
// ===========================================================================

func TestMCP_Health(t *testing.T) {
	resp, err := get(t, mcpBase+"/health", nil)
	if err != nil {
		t.Fatalf("MCP /health request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("MCP /health: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("MCP /health: expected 'ok', got %s", string(body))
	}
}

func TestMCP_Initialize(t *testing.T) {
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	}
	body, _ := json.Marshal(initReq)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), map[string]string{
		"X-API-Key": apiKey,
		"Accept":    "application/json, text/event-stream",
	})
	if err != nil {
		t.Fatalf("MCP initialize failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("MCP initialize: expected 200, got %d", resp.StatusCode)
	}
}

func TestMCP_SearchDocuments(t *testing.T) {
	callReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "search_documents",
			"arguments": map[string]interface{}{
				"query": "Сколково",
				"limit": 3,
			},
		},
	}
	body, _ := json.Marshal(callReq)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), map[string]string{
		"X-API-Key": apiKey,
		"Accept":    "application/json, text/event-stream",
	})
	if err != nil {
		t.Fatalf("MCP search_documents failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("MCP search_documents: expected 200, got %d", resp.StatusCode)
	}
}

func TestMCP_GetSourceHealth(t *testing.T) {
	callReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "get_source_health",
			"arguments": map[string]interface{}{},
		},
	}
	body, _ := json.Marshal(callReq)
	resp, err := post(t, mcpBase+"/mcp", "application/json", bytes.NewReader(body), map[string]string{
		"X-API-Key": apiKey,
		"Accept":    "application/json, text/event-stream",
	})
	if err != nil {
		t.Fatalf("MCP get_source_health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("MCP get_source_health: expected 200, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// Admin Panel (:8090)
// ===========================================================================

func TestAdmin_MainPage(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/", cookie)
	if err != nil {
		t.Fatalf("Admin / request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	elements := []string{"База Сколково", "Документы", "Парсинг RSS", "Индексация", "Всего", "Действует"}
	for _, el := range elements {
		if !strings.Contains(html, el) {
			t.Errorf("Admin /: missing element '%s'", el)
		}
	}
}

func TestAdmin_DiffPage(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/diff", cookie)
	if err != nil {
		t.Fatalf("Admin /diff failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /diff: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	for _, el := range []string{"Сравнение", "Документ 1", "Документ 2", "Сравнить"} {
		if !strings.Contains(html, el) {
			t.Errorf("Admin /diff: missing element '%s'", el)
		}
	}
}

func TestAdmin_AnalyticsPage(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/analytics", cookie)
	if err != nil {
		t.Fatalf("Admin /analytics failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /analytics: expected 200, got %d", resp.StatusCode)
	}
}

func TestAdmin_GraphPage(t *testing.T) {
	cookie := adminLogin(t)
	resp, err := getWithCookie(t, adminBase+"/graph", cookie)
	if err != nil {
		t.Fatalf("Admin /graph failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Admin /graph: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	for _, el := range []string{"Граф", "references", "supersedes", "related"} {
		if !strings.Contains(html, el) {
			t.Errorf("Admin /graph: missing element '%s'", el)
		}
	}
}

// ===========================================================================
// Residency Admin (:8091)
// ===========================================================================

func TestResidency_ClientsPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/clients", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /clients failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /clients: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	for _, el := range []string{"Клиенты", "Стадия", "ИНН"} {
		if !strings.Contains(html, el) {
			t.Errorf("Residency /clients: missing element '%s'", el)
		}
	}
}

func TestResidency_ChecklistsPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/checklists", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /checklists failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /checklists: expected 200, got %d", resp.StatusCode)
	}
}

func TestResidency_DeadlinesPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/deadlines", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /deadlines failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /deadlines: expected 200, got %d", resp.StatusCode)
	}
}

func TestResidency_TemplatesPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/templates", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /templates failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /templates: expected 200, got %d", resp.StatusCode)
	}
}

func TestResidency_TenantsPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/tenants", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /tenants failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /tenants: expected 200, got %d", resp.StatusCode)
	}
}

func TestResidency_EventsPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/events-admin", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /events-admin failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /events-admin: expected 200, got %d", resp.StatusCode)
	}
}

func TestResidency_ContestsPage(t *testing.T) {
	resp, err := get(t, residencyBase+"/contests-admin", map[string]string{"Authorization": authHeader()})
	if err != nil {
		t.Fatalf("Residency /contests-admin failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Residency /contests-admin: expected 200, got %d", resp.StatusCode)
	}
}

// ===========================================================================
// Consultant Dashboard (:8094)
// ===========================================================================

func consultantHeader() string {
	cred := base64.StdEncoding.EncodeToString([]byte("consultant:change-me-please"))
	return "Basic " + cred
}

func TestConsultant_Dashboard(t *testing.T) {
	resp, err := get(t, consultantBase+"/consultant/dashboard", map[string]string{"Authorization": consultantHeader()})
	if err != nil {
		t.Fatalf("Consultant /dashboard failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Consultant /dashboard: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	for _, el := range []string{"Дашборд консультанта", "Просрочено", "Всего клиентов"} {
		if !strings.Contains(html, el) {
			t.Errorf("Consultant /dashboard: missing element '%s'", el)
		}
	}
}

// ===========================================================================
// Prometheus Metrics (:9090)
// ===========================================================================

func TestPrometheus_Metrics(t *testing.T) {
	resp, err := get(t, prometheusBase+"/metrics", nil)
	if err != nil {
		t.Fatalf("Prometheus /metrics failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Prometheus /metrics: expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	metrics := string(body)
	if !strings.Contains(metrics, "# HELP") {
		t.Error("Prometheus /metrics: no HELP lines found")
	}
}

// ===========================================================================
// Auth rejection test
// ===========================================================================

func TestAdmin_AuthRejected(t *testing.T) {
	// Админка использует сессионную авторизацию (форма входа).
	// Без сессии должен быть редирект на /login (303).
	noRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}
	req, _ := http.NewRequest("GET", adminBase+"/", nil)
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("Admin auth test failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("Admin /: expected 303 redirect for unauthenticated access, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/login") {
		t.Errorf("Admin /: expected redirect to /login, got %s", location)
	}
}
