package generator

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/common/model"
)

// ---- Mock stores ----

type mockClientStore struct {
	clients   map[string]*model.Client
	documents map[string][]model.ClientDocument
}

func (m *mockClientStore) GetClient(_ context.Context, id string) (*model.Client, error) {
	c, ok := m.clients[id]
	if !ok {
		return nil, &json.UnmarshalTypeError{}
	}
	return c, nil
}

func (m *mockClientStore) ListClientDocuments(_ context.Context, id string) ([]model.ClientDocument, error) {
	return m.documents[id], nil
}

type mockTemplateStore struct {
	templates    map[string]*model.DocumentTemplate
	files        map[string][]byte
	templateList []model.DocumentTemplate
}

func (m *mockTemplateStore) GetTemplate(_ context.Context, id string) (*model.DocumentTemplate, error) {
	t, ok := m.templates[id]
	if !ok {
		return nil, &json.UnmarshalTypeError{}
	}
	return t, nil
}

func (m *mockTemplateStore) ListTemplates(_ context.Context) ([]model.DocumentTemplate, error) {
	return m.templateList, nil
}

func (m *mockTemplateStore) ReadTemplateFile(_ context.Context, file string) ([]byte, error) {
	return m.files[file], nil
}

// ---- Helper ----

func newTestClient() *model.Client {
	return &model.Client{
		ID:             "client-001",
		Name:           "ООО Тестовая Компания",
		INN:            "7707123456",
		ContactEmail:   "test@example.com",
		ContactPhone:   "+7 (495) 123-45-67",
		ResidencyStage: model.StageResident,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func newTestGenerator(t *testing.T) (*DocumentGenerator, *mockClientStore, *mockTemplateStore) {
	t.Helper()

	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "output")
	tplDir := filepath.Join(tmpDir, "templates")

	clientStore := &mockClientStore{
		clients: map[string]*model.Client{
			"client-001": newTestClient(),
		},
		documents: map[string][]model.ClientDocument{},
	}

	now := time.Now()
	tmpl := &model.DocumentTemplate{
		ID:           "template-001",
		Name:         "Тестовый шаблон",
		Type:         "go.tpl",
		TemplateFile: "test.go.tpl",
		Version:      "1.0",
		CreatedAt:    now,
	}

	templateStore := &mockTemplateStore{
		templates: map[string]*model.DocumentTemplate{
			"template-001": tmpl,
		},
		files: map[string][]byte{
			"test.go.tpl": []byte(`
<h1>Документ для {{.Client.Name}}</h1>
<p>ИНН: {{.Client.INN}}</p>
<p>Email: {{.Client.ContactEmail}}</p>
<p>Телефон: {{.Client.ContactPhone}}</p>
<p>Стадия: {{.Client.ResidencyStage}}</p>
<p>Дата: {{.Date}}</p>
<p>Кастомное поле: {{.CustomField}}</p>
`),
		},
		templateList: []model.DocumentTemplate{*tmpl},
	}

	config := GeneratorConfig{
		TemplateDir:   tplDir,
		OutputDir:     outDir,
		DefaultFormat: "pdf",
	}

	gen := NewDocumentGenerator(config, clientStore, templateStore)
	return gen, clientStore, templateStore
}

// ---- Tests ----

func TestRenderTemplate_GoTpl(t *testing.T) {
	gen, _, _ := newTestGenerator(t)

	ctx := context.Background()
	outputPath, err := gen.RenderTemplate(ctx, "template-001", "client-001", map[string]string{
		"CustomField": "Тестовое значение",
	})
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	if outputPath == "" {
		t.Fatal("expected non-empty outputPath")
	}

	// .go.tpl should produce .pdf file
	if !strings.HasSuffix(outputPath, ".pdf") {
		t.Fatalf("expected .pdf extension, got %s", outputPath)
	}

	// Check file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("output file does not exist: %s", outputPath)
	}

	// Check content contains rendered values
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)

	for _, expected := range []string{
		"ООО Тестовая Компания",
		"7707123456",
		"test@example.com",
		"+7 (495) 123-45-67",
		"резидент",
		"Тестовое значение",
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("expected content to contain %q, but it does not", expected)
		}
	}
}

func TestRenderTemplate_ClientNotFound(t *testing.T) {
	gen, _, _ := newTestGenerator(t)

	ctx := context.Background()
	_, err := gen.RenderTemplate(ctx, "template-001", "nonexistent-client", nil)
	if err == nil {
		t.Fatal("expected error for non-existent client")
	}
}

func TestRenderTemplate_TemplateNotFound(t *testing.T) {
	gen, _, _ := newTestGenerator(t)

	ctx := context.Background()
	_, err := gen.RenderTemplate(ctx, "nonexistent-template", "client-001", nil)
	if err == nil {
		t.Fatal("expected error for non-existent template")
	}
}

func TestRenderTemplate_WithCustomVariables(t *testing.T) {
	gen, _, _ := newTestGenerator(t)

	ctx := context.Background()
	outputPath, err := gen.RenderTemplate(ctx, "template-001", "client-001", map[string]string{
		"CustomField": "Мое значение",
		"AnotherVar":  "Дополнительно",
	})
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "Мое значение") {
		t.Errorf("expected custom variable to be rendered")
	}
}

func TestCreateDefaultTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	clientStore := &mockClientStore{clients: map[string]*model.Client{}}
	templateStore := &mockTemplateStore{
		templates:    map[string]*model.DocumentTemplate{},
		files:        map[string][]byte{},
		templateList: []model.DocumentTemplate{},
	}

	config := GeneratorConfig{
		TemplateDir: tmpDir,
		OutputDir:   filepath.Join(tmpDir, "output"),
	}

	gen := NewDocumentGenerator(config, clientStore, templateStore)

	if err := gen.CreateDefaultTemplates(); err != nil {
		t.Fatalf("CreateDefaultTemplates failed: %v", err)
	}

	expectedFiles := []string{
		"Заявление_на_резидентство.go.tpl",
		"Квартальный_отчёт.go.tpl",
		"Годовой_отчёт.go.tpl",
		"Запрос_на_продление.go.tpl",
		"Уведомление_о_выходе.go.tpl",
	}

	for _, fname := range expectedFiles {
		path := filepath.Join(tmpDir, fname)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected template file %q to be created", fname)
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read %q: %v", fname, err)
			continue
		}

		// Verify it contains client variables
		content := string(data)
		if !strings.Contains(content, "{{.Client.Name}}") {
			t.Errorf("template %q should contain {{.Client.Name}}", fname)
		}
		if !strings.Contains(content, "{{.Date}}") {
			t.Errorf("template %q should contain {{.Date}}", fname)
		}
	}
}

func TestGeneratePDF(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.pdf")

	htmlContent := "<h1>Test Document</h1><p>Hello World</p>"

	if err := GeneratePDF(htmlContent, outputPath); err != nil {
		t.Fatalf("GeneratePDF failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("PDF file was not created: %s", outputPath)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)

	// Should contain HTML wrapper
	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE html in output")
	}
	if !strings.Contains(content, "Test Document") {
		t.Error("expected original content in output")
	}
}

func TestGenerateDOCX(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.docx")

	content := "Hello {{.Name}}, your INN is {{.INN}}."
	variables := map[string]string{
		"Name": "Test Corp",
		"INN":  "7707000000",
	}

	if err := GenerateDOCX(content, variables, outputPath); err != nil {
		t.Fatalf("GenerateDOCX failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("DOCX file was not created: %s", outputPath)
	}

	// Verify it's a valid ZIP (DOCX = ZIP)
	zr, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("expected valid ZIP file, got error: %v", err)
	}
	defer zr.Close()

	// Should contain required DOCX entries
	fileNames := make(map[string]bool)
	for _, f := range zr.File {
		fileNames[f.Name] = true
	}

	requiredFiles := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
	for _, rf := range requiredFiles {
		if !fileNames[rf] {
			t.Errorf("expected DOCX to contain %q", rf)
		}
	}

	// Check document.xml content
	docXML, err := zr.File[2].Open() // word/document.xml
	if err != nil {
		t.Fatalf("failed to open document.xml: %v", err)
	}
	defer docXML.Close()
	docData := make([]byte, 1024)
	n, _ := docXML.Read(docData)
	docContent := string(docData[:n])

	if !strings.Contains(docContent, "Test Corp") {
		t.Error("expected substituted variable in document.xml")
	}
	if !strings.Contains(docContent, "7707000000") {
		t.Error("expected substituted INN in document.xml")
	}
}

func TestGeneratePDF_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "empty.pdf")

	if err := GeneratePDF("", outputPath); err != nil {
		t.Fatalf("GeneratePDF with empty content failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("PDF file was not created for empty content")
	}
}

func TestGenerateDOCX_EmptyContent(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "empty.docx")

	if err := GenerateDOCX("", nil, outputPath); err != nil {
		t.Fatalf("GenerateDOCX with empty content failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("DOCX file was not created for empty content")
	}
}

func TestListAvailableTemplates(t *testing.T) {
	_, _, templateStore := newTestGenerator(t)

	ctx := context.Background()
	names, err := templateStore.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}

	if len(names) != 1 {
		t.Fatalf("expected 1 template, got %d", len(names))
	}
	if names[0].ID != "template-001" {
		t.Errorf("expected template-001, got %s", names[0].ID)
	}
}

func TestGeneratorConfig_ApplyDefaults(t *testing.T) {
	config := GeneratorConfig{}
	config.ApplyDefaults()

	if config.TemplateDir != "./templates" {
		t.Errorf("expected TemplateDir='./templates', got %q", config.TemplateDir)
	}
	expected := filepath.Join("Документы_Сколково", "Сгенерированные")
	if config.OutputDir != expected {
		t.Errorf("expected OutputDir=%q, got %q", expected, config.OutputDir)
	}
	if config.DefaultFormat != "pdf" {
		t.Errorf("expected DefaultFormat='pdf', got %q", config.DefaultFormat)
	}
}

func TestGeneratorConfig_NoOverride(t *testing.T) {
	config := GeneratorConfig{
		TemplateDir:   "/my/templates",
		OutputDir:     "/my/output",
		DefaultFormat: "docx",
	}
	config.ApplyDefaults()

	if config.TemplateDir != "/my/templates" {
		t.Errorf("TemplateDir was overridden: got %q", config.TemplateDir)
	}
	if config.OutputDir != "/my/output" {
		t.Errorf("OutputDir was overridden: got %q", config.OutputDir)
	}
	if config.DefaultFormat != "docx" {
		t.Errorf("DefaultFormat was overridden: got %q", config.DefaultFormat)
	}
}

func TestTemplateExtension(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"file.go.tpl", ".go.tpl"},
		{"template.docx.tpl", ".docx.tpl"},
		{"page.html.tpl", ".html.tpl"},
		{"document.pdf", ".pdf"},
		{"FILE.GO.TPL", ".go.tpl"},
		{"mixed.Docx.Tpl", ".docx.tpl"},
	}

	for _, tc := range tests {
		got := templateExtension(tc.input)
		if got != tc.expected {
			t.Errorf("templateExtension(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal_name", "normal_name"},
		{"name/with/slashes", "name_with_slashes"},
		{"name\\with\\backslashes", "name_with_backslashes"},
		{"name:with:colons", "name_with_colons"},
		{"  spaces  ", "spaces"},
		{"complex/name:with*chars?", "complex_name_with_chars_"},
	}

	for _, tc := range tests {
		got := sanitizeFilename(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"<tag>", "&lt;tag&gt;"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"'single'", "&apos;single&apos;"},
		{"all: <>&\"'", "all: &lt;&gt;&amp;&quot;&apos;"},
	}

	for _, tc := range tests {
		got := escapeXML(tc.input)
		if got != tc.expected {
			t.Errorf("escapeXML(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestRenderTemplate_DefaultVariables(t *testing.T) {
	gen, _, _ := newTestGenerator(t)

	ctx := context.Background()
	outputPath, err := gen.RenderTemplate(ctx, "template-001", "client-001", nil)
	if err != nil {
		t.Fatalf("RenderTemplate failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)

	// Verify all built-in variables are present
	for _, expected := range []string{
		"{{.Client.Name}}", // Should NOT appear (should be rendered)
		"{{.Client.INN}}",  // Should NOT appear
	} {
		if strings.Contains(content, expected) {
			t.Errorf("template variable %q was not rendered", expected)
		}
	}
}

func TestNewDocumentGenerator(t *testing.T) {
	config := GeneratorConfig{}
	cs := &mockClientStore{}
	ts := &mockTemplateStore{}

	gen := NewDocumentGenerator(config, cs, ts)

	if gen == nil {
		t.Fatal("NewDocumentGenerator returned nil")
	}
	if gen.config.TemplateDir != "./templates" {
		t.Errorf("expected default TemplateDir, got %q", gen.config.TemplateDir)
	}
}
