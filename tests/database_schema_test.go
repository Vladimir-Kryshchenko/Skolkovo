// Package tests — интеграционные тесты схемы БД (5 миграций).
package tests

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func testDBURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://skolkovo:skolkovo@localhost:5432/skolkovo?sslmode=disable"
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", testDBURL())
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	return db
}

// ---------------------------------------------------------------------------
// Migration existence
// ---------------------------------------------------------------------------

func TestSchemaMigrationsTableExists(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'schema_migrations')`).Scan(&exists)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !exists {
		t.Fatal("schema_migrations table does not exist")
	}
}

func TestAllMigrationsApplied(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedVersions := []string{"001", "002", "003", "004", "005"}
	expectedFiles := []string{
		"001_residency_system.sql",
		"002_extended_sources.sql",
		"003_mcp_audit_preferences_npa.sql",
		"004_change_feed_source_health.sql",
		"005_ai_models_agents.sql",
	}

	rows, err := db.Query(`SELECT version, filename FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	defer rows.Close()

	applied := make(map[string]string)
	for rows.Next() {
		var v, f string
		if err := rows.Scan(&v, &f); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		applied[v] = f
	}

	for i, ver := range expectedVersions {
		if filename, ok := applied[ver]; !ok {
			t.Errorf("migration %s not applied", ver)
		} else if filename != expectedFiles[i] {
			t.Errorf("migration %s: expected filename %q, got %q", ver, expectedFiles[i], filename)
		}
	}
}

// ---------------------------------------------------------------------------
// Migration 001: Residency System
// ---------------------------------------------------------------------------

func TestMigration001_TablesExist(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedTables := []string{
		"tenants", "clients", "stage_transitions", "checklists",
		"client_checklists", "checklist_step_statuses", "deadlines",
		"document_templates", "client_documents",
	}

	for _, tbl := range expectedTables {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, tbl).Scan(&exists)
		if err != nil {
			t.Fatalf("query error for %s: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %s does not exist", tbl)
		}
	}
}

func TestMigration001_TenantsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := map[string]string{
		"id": "uuid", "name": "text", "api_key": "text",
		"settings": "jsonb", "created_at": "timestamp", "active": "boolean",
	}
	for col, expectedType := range expectedCols {
		var actualType string
		err := db.QueryRow(`SELECT data_type FROM information_schema.columns WHERE table_name = 'tenants' AND column_name = $1`, col).Scan(&actualType)
		if err != nil {
			t.Errorf("column %s not found in tenants: %v", col, err)
			continue
		}
		if !strings.HasPrefix(actualType, strings.Split(expectedType, " ")[0]) {
			t.Errorf("column %s: expected type containing %q, got %q", col, expectedType, actualType)
		}
	}
}

func TestMigration001_ClientsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "name", "inn", "contact_email", "contact_phone", "residency_stage", "tenant_id", "created_at", "updated_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'clients' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from clients", col)
		}
	}
}

func TestMigration001_ClientsINNUnique(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.table_constraints WHERE table_name = 'clients' AND constraint_type = 'UNIQUE' AND constraint_name LIKE '%inn%')`).Scan(&exists)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !exists {
		t.Error("clients.inn should have UNIQUE constraint")
	}
}

func TestMigration001_StageTransitionsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "client_id", "from_stage", "to_stage", "transitioned_at", "notes"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'stage_transitions' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from stage_transitions", col)
		}
	}
}

func TestMigration001_ChecklistsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "title", "procedure_type", "steps", "version", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'checklists' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from checklists", col)
		}
	}
}

func TestMigration001_ChecklistProcedureTypeCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var checkCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'checklists'::regclass AND contype = 'c'`).Scan(&checkCount)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if checkCount == 0 {
		t.Error("checklists should have CHECK constraint on procedure_type")
	}
}

func TestMigration001_DeadlinesColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "client_id", "title", "due_date", "type", "status", "notification_sent", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'deadlines' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from deadlines", col)
		}
	}
}

func TestMigration001_DeadlinesDueDateIndex(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE tablename = 'deadlines' AND indexname = 'idx_deadlines_due_date')`).Scan(&exists)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !exists {
		t.Error("idx_deadlines_due_date index should exist")
	}
}

func TestMigration001_DocumentTemplatesColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "name", "type", "template_file", "variables", "version", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'document_templates' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from document_templates", col)
		}
	}
}

func TestMigration001_ClientDocumentsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "client_id", "document_id", "role", "status", "submitted_at", "tenant_id"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'client_documents' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from client_documents", col)
		}
	}
}

func TestMigration001_ChecklistStepStatusesColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "client_checklist_id", "step_index", "status", "completed_at", "notes"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'checklist_step_statuses' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from checklist_step_statuses", col)
		}
	}
}

func TestMigration001_ClientChecklistsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "client_id", "checklist_id", "status", "started_at", "completed_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'client_checklists' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from client_checklists", col)
		}
	}
}

// ---------------------------------------------------------------------------
// Migration 002: Extended Sources
// ---------------------------------------------------------------------------

func TestMigration002_TablesExist(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedTables := []string{"events", "contests", "faq_items", "telegram_posts", "residents", "document_links"}
	for _, tbl := range expectedTables {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, tbl).Scan(&exists)
		if err != nil {
			t.Fatalf("query error for %s: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %s does not exist", tbl)
		}
	}
}

func TestMigration002_EventsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "title", "description", "event_date", "event_end_date", "location", "source_url", "status", "category", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'events' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from events", col)
		}
	}
}

func TestMigration002_ContestsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "title", "description", "start_date", "end_date", "requirements", "prize", "source_url", "status", "category", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'contests' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from contests", col)
		}
	}
}

func TestMigration002_FAQColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "question", "answer", "category", "source_url", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'faq_items' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from faq_items", col)
		}
	}
}

func TestMigration002_TelegramPostsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "channel", "text", "published_at", "source_url", "media_urls", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'telegram_posts' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from telegram_posts", col)
		}
	}
}

func TestMigration002_TelegramPostsMediaURLsJSONB(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var actualType string
	err := db.QueryRow(`SELECT data_type FROM information_schema.columns WHERE table_name = 'telegram_posts' AND column_name = 'media_urls'`).Scan(&actualType)
	if err != nil {
		t.Fatalf("column media_urls not found: %v", err)
	}
	if actualType != "jsonb" {
		t.Errorf("media_urls should be jsonb, got %s", actualType)
	}
}

func TestMigration002_ResidentsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "name", "inn", "industry", "join_date", "status", "source_url", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'residents' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from residents", col)
		}
	}
}

func TestMigration002_DocumentLinksColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "source_id", "target_id", "link_type", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'document_links' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from document_links", col)
		}
	}
}

func TestMigration002_DocumentLinksLinkTypeCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var checkCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'document_links'::regclass AND contype = 'c'`).Scan(&checkCount)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if checkCount == 0 {
		t.Error("document_links should have CHECK constraint on link_type")
	}
}

func TestMigration002_Indexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{
		"idx_events_event_date", "idx_events_status", "idx_events_category",
		"idx_contests_start_date", "idx_contests_end_date", "idx_contests_status", "idx_contests_category",
		"idx_faq_items_category", "idx_telegram_posts_channel", "idx_telegram_posts_published_at",
		"idx_residents_inn", "idx_residents_status", "idx_residents_industry",
		"idx_document_links_source_id", "idx_document_links_target_id",
	}

	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

// ---------------------------------------------------------------------------
// Migration 003: MCP Audit, Preferences, NPA, Eligibility
// ---------------------------------------------------------------------------

func TestMigration003_TablesExist(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedTables := []string{"mcp_audit_log", "preferences", "npa_documents", "eligibility_checks"}
	for _, tbl := range expectedTables {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, tbl).Scan(&exists)
		if err != nil {
			t.Fatalf("query error for %s: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %s does not exist", tbl)
		}
	}
}

func TestMigration003_MCPAuditColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "tenant_id", "api_key_pfx", "tool_name", "input_hash", "input_brief", "client_id", "remote_addr", "duration_ms", "success", "error_msg", "created_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'mcp_audit_log' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from mcp_audit_log", col)
		}
	}
}

func TestMigration003_MCPAuditIndexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{"idx_mcp_audit_tenant_id", "idx_mcp_audit_tool_name", "idx_mcp_audit_created_at", "idx_mcp_audit_client_id"}
	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

func TestMigration003_PreferencesColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "ext_id", "title", "pref_type", "benefit_desc", "legal_ref", "source_url", "content", "status", "fetched_at", "updated_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'preferences' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from preferences", col)
		}
	}
}

func TestMigration003_PreferencesPrefTypeCheck(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var checkCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'preferences'::regclass AND contype = 'c'`).Scan(&checkCount)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if checkCount == 0 {
		t.Error("preferences should have CHECK constraint on pref_type")
	}
}

func TestMigration003_NPADocumentsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "ext_id", "title", "npa_number", "npa_type", "issued_by", "issued_at", "effective_at", "source_url", "summary", "status", "fetched_at", "updated_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'npa_documents' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from npa_documents", col)
		}
	}
}

func TestMigration003_NPAIndexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{"idx_npa_status", "idx_npa_issued_at", "idx_npa_npa_type"}
	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

func TestMigration003_EligibilityChecksColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "inn", "company_name", "status", "is_msp", "msp_category", "score", "eligible", "issues", "warnings", "checked_at", "checked_by"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'eligibility_checks' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from eligibility_checks", col)
		}
	}
}

func TestMigration003_EligibilityIssuesIsArray(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var actualType string
	err := db.QueryRow(`SELECT data_type FROM information_schema.columns WHERE table_name = 'eligibility_checks' AND column_name = 'issues'`).Scan(&actualType)
	if err != nil {
		t.Fatalf("column issues not found: %v", err)
	}
	if !strings.Contains(actualType, "array") {
		t.Errorf("issues should be array type, got %s", actualType)
	}
}

// ---------------------------------------------------------------------------
// Migration 004: Change Feed + Source Health
// ---------------------------------------------------------------------------

func TestMigration004_TablesExist(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedTables := []string{"change_events", "source_health"}
	for _, tbl := range expectedTables {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, tbl).Scan(&exists)
		if err != nil {
			t.Fatalf("query error for %s: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %s does not exist", tbl)
		}
	}
}

func TestMigration004_ChangeEventsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "entity_type", "entity_id", "title", "category", "kind", "source_url", "summary", "detected_at", "notified"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'change_events' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from change_events", col)
		}
	}
}

func TestMigration004_ChangeEventsIndexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{"idx_change_events_detected", "idx_change_events_unnotified", "idx_change_events_type"}
	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

func TestMigration004_SourceHealthColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"name", "last_run_at", "last_success_at", "items_last_run", "consecutive_failures", "last_error", "updated_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'source_health' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from source_health", col)
		}
	}
}

func TestMigration004_SourceHealthPrimaryKey(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var constraintName string
	err := db.QueryRow(`SELECT conname FROM pg_constraint WHERE conrelid = 'source_health'::regclass AND contype = 'p'`).Scan(&constraintName)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	var pkCol string
	err = db.QueryRow(`SELECT column_name FROM information_schema.key_column_usage WHERE table_name = 'source_health' AND constraint_name = $1`, constraintName).Scan(&pkCol)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if pkCol != "name" {
		t.Errorf("source_health PK should be 'name', got %s", pkCol)
	}
}

// ---------------------------------------------------------------------------
// Migration 005: AI Models + Agents
// ---------------------------------------------------------------------------

func TestMigration005_TablesExist(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedTables := []string{"ai_models", "ai_agents"}
	for _, tbl := range expectedTables {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, tbl).Scan(&exists)
		if err != nil {
			t.Fatalf("query error for %s: %v", tbl, err)
		}
		if !exists {
			t.Errorf("table %s does not exist", tbl)
		}
	}
}

func TestMigration005_AIModelsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "name", "provider", "model_id", "base_url", "api_key", "max_tokens", "temperature", "enabled", "description", "created_at", "updated_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'ai_models' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from ai_models", col)
		}
	}
}

func TestMigration005_AIAgentsColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedCols := []string{"id", "name", "agent_type", "model_id", "system_prompt", "temperature", "max_tokens", "enabled", "description", "created_at", "updated_at"}
	for _, col := range expectedCols {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'ai_agents' AND column_name = $1)`, col).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", col, err)
			continue
		}
		if !exists {
			t.Errorf("column %s missing from ai_agents", col)
		}
	}
}

func TestMigration005_AIAgentsModelIDFK(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	var fkCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = 'ai_agents'::regclass AND contype = 'f'`).Scan(&fkCount)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if fkCount == 0 {
		t.Error("ai_agents should have FK constraint on model_id referencing ai_models")
	}
}

func TestMigration005_AIModelsIndexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{"idx_ai_models_provider", "idx_ai_models_enabled"}
	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

func TestMigration005_AIAgentsIndexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{"idx_ai_agents_type", "idx_ai_agents_enabled"}
	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist", idx)
		}
	}
}

// ---------------------------------------------------------------------------
// Cross-migration: all expected indexes
// ---------------------------------------------------------------------------

func TestAllExpectedIndexes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedIndexes := []string{
		// Migration 001
		"idx_clients_tenant_id", "idx_stage_transitions_client_id",
		"idx_client_checklists_client_id", "idx_client_checklists_checklist_id",
		"idx_checklist_step_statuses_client_checklist_id",
		"idx_deadlines_client_id", "idx_deadlines_due_date",
		"idx_client_documents_client_id", "idx_client_documents_tenant_id",
		// Migration 002
		"idx_events_event_date", "idx_events_status", "idx_events_category",
		"idx_contests_start_date", "idx_contests_end_date", "idx_contests_status", "idx_contests_category",
		"idx_faq_items_category", "idx_telegram_posts_channel", "idx_telegram_posts_published_at",
		"idx_residents_inn", "idx_residents_status", "idx_residents_industry",
		"idx_document_links_source_id", "idx_document_links_target_id",
		// Migration 003
		"idx_mcp_audit_tenant_id", "idx_mcp_audit_tool_name", "idx_mcp_audit_created_at", "idx_mcp_audit_client_id",
		"idx_preferences_pref_type", "idx_preferences_status",
		"idx_npa_status", "idx_npa_issued_at", "idx_npa_npa_type",
		"idx_eligibility_inn", "idx_eligibility_checked_at",
		// Migration 004
		"idx_change_events_detected", "idx_change_events_unnotified", "idx_change_events_type",
		// Migration 005
		"idx_ai_models_provider", "idx_ai_models_enabled",
		"idx_ai_agents_type", "idx_ai_agents_enabled",
	}

	var actualCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_indexes`).Scan(&actualCount)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}

	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Errorf("query error for %s: %v", idx, err)
			continue
		}
		if !exists {
			t.Errorf("index %s does not exist (total indexes in DB: %d)", idx, actualCount)
		}
	}

	t.Logf("Verified %d expected indexes out of %d total indexes", len(expectedIndexes), actualCount)
}

// ---------------------------------------------------------------------------
// Foreign key constraints
// ---------------------------------------------------------------------------

func TestForeignKeyConstraints(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedFKs := []struct {
		table, column, refTable, refColumn string
	}{
		{"clients", "tenant_id", "tenants", "id"},
		{"stage_transitions", "client_id", "clients", "id"},
		{"client_checklists", "client_id", "clients", "id"},
		{"client_checklists", "checklist_id", "checklists", "id"},
		{"checklist_step_statuses", "client_checklist_id", "client_checklists", "id"},
		{"deadlines", "client_id", "clients", "id"},
		{"client_documents", "client_id", "clients", "id"},
		{"document_links", "source_id", "client_documents", "id"},
		{"document_links", "target_id", "client_documents", "id"},
		{"ai_agents", "model_id", "ai_models", "id"},
	}

	rows, err := db.Query(`
		SELECT tc.table_name, kcu.column_name, ccu.table_name AS foreign_table_name, ccu.column_name AS foreign_column_name
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu ON tc.constraint_name = kcu.constraint_name
		JOIN information_schema.constraint_column_usage AS ccu ON ccu.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
	`)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	defer rows.Close()

	fkMap := make(map[string]bool)
	for rows.Next() {
		var tbl, col, refTbl, refCol string
		if err := rows.Scan(&tbl, &col, &refTbl, &refCol); err != nil {
			t.Fatalf("scan error: %v", err)
		}
		key := fmt.Sprintf("%s.%s -> %s.%s", tbl, col, refTbl, refCol)
		fkMap[key] = true
	}

	for _, expected := range expectedFKs {
		key := fmt.Sprintf("%s.%s -> %s.%s", expected.table, expected.column, expected.refTable, expected.refColumn)
		if !fkMap[key] {
			t.Errorf("missing FK constraint: %s", key)
		}
	}

	t.Logf("Verified %d FK constraints", len(fkMap))
}

// ---------------------------------------------------------------------------
// CHECK constraints
// ---------------------------------------------------------------------------

func TestCheckConstraints(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedChecks := []struct {
		table  string
		column string
	}{
		{"checklists", "procedure_type"},
		{"client_checklists", "status"},
		{"checklist_step_statuses", "status"},
		{"deadlines", "type"},
		{"deadlines", "status"},
		{"client_documents", "role"},
		{"client_documents", "status"},
		{"events", "status"},
		{"contests", "status"},
		{"residents", "status"},
		{"document_links", "link_type"},
		{"preferences", "pref_type"},
		{"preferences", "status"},
		{"npa_documents", "status"},
	}

	for _, expected := range expectedChecks {
		var checkCount int
		err := db.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = $1::regclass AND contype = 'c'`, expected.table).Scan(&checkCount)
		if err != nil {
			t.Errorf("query error for %s.%s: %v", expected.table, expected.column, err)
			continue
		}
		if checkCount == 0 {
			t.Errorf("table %s should have CHECK constraint on %s", expected.table, expected.column)
		}
	}

	t.Logf("Verified CHECK constraints on %d tables", len(expectedChecks))
}

// ---------------------------------------------------------------------------
// Primary key constraints
// ---------------------------------------------------------------------------

func TestPrimaryKeyConstraints(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedPKs := []string{
		"tenants", "clients", "stage_transitions", "checklists",
		"client_checklists", "checklist_step_statuses", "deadlines",
		"document_templates", "client_documents",
		"events", "contests", "faq_items", "telegram_posts",
		"residents", "document_links",
		"mcp_audit_log", "preferences", "npa_documents", "eligibility_checks",
		"change_events", "source_health",
		"ai_models", "ai_agents",
	}

	for _, tbl := range expectedPKs {
		var pkCount int
		err := db.QueryRow(`SELECT COUNT(*) FROM pg_constraint WHERE conrelid = $1::regclass AND contype = 'p'`, tbl).Scan(&pkCount)
		if err != nil {
			t.Errorf("query error for %s: %v", tbl, err)
			continue
		}
		if pkCount == 0 {
			t.Errorf("table %s should have PRIMARY KEY", tbl)
		}
	}

	t.Logf("Verified PRIMARY KEYs on %d tables", len(expectedPKs))
}

// ---------------------------------------------------------------------------
// Total table count
// ---------------------------------------------------------------------------

func TestTotalTableCount(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	expectedTables := []string{
		"schema_migrations",
		"tenants", "clients", "stage_transitions", "checklists",
		"client_checklists", "checklist_step_statuses", "deadlines",
		"document_templates", "client_documents",
		"events", "contests", "faq_items", "telegram_posts",
		"residents", "document_links",
		"mcp_audit_log", "preferences", "npa_documents", "eligibility_checks",
		"change_events", "source_health",
		"ai_models", "ai_agents",
	}

	var actualCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&actualCount)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}

	if actualCount < len(expectedTables) {
		t.Errorf("expected at least %d tables, got %d", len(expectedTables), actualCount)
	}

	t.Logf("Total tables in public schema: %d (expected at least %d)", actualCount, len(expectedTables))
}
