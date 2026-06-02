package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// minimal PDF-заголовок достаточно, чтобы файл существовал и отдавался.
const tinyPDF = "%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n"

// newTestServerWithPDF поднимает Server с файловым JSON-реестром и одним
// документом, у которого есть локальный PDF-файл на диске.
func newTestServerWithPDF(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdfPath, []byte(tinyPDF), 0o644); err != nil {
		t.Fatalf("запись PDF: %v", err)
	}
	st, err := store.NewJSONStore(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatalf("создание JSONStore: %v", err)
	}
	doc := model.Document{ID: "doc1", Title: "Тестовый PDF", Status: model.StatusActive, LocalPath: pdfPath}
	if err := st.Upsert(context.Background(), doc); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return &Server{store: st}, pdfPath
}

func mux(s *Server) *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /documents/{id}/download", s.handleDownload)
	m.HandleFunc("GET /documents/{id}/view-original", s.handleViewOriginal)
	return m
}

// TestDownloadInlineDisposition: ?inline=1 отдаёт файл для встроенного просмотра
// (Content-Disposition: inline), а без параметра — как вложение (скачивание).
// Это и есть исправление «PDF нельзя просмотреть, только скачать».
func TestDownloadInlineDisposition(t *testing.T) {
	s, _ := newTestServerWithPDF(t)
	h := mux(s)

	// inline — для iframe-просмотрщика
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/documents/doc1/download?inline=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("inline: код %d, ожидался 200", rec.Code)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
		t.Errorf("inline: Content-Disposition = %q, ожидался inline", cd)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("inline: Content-Type = %q, ожидался application/pdf", ct)
	}

	// без inline — скачивание
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("GET", "/documents/doc1/download", nil))
	if cd := rec2.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("download: Content-Disposition = %q, ожидался attachment", cd)
	}
}

// TestViewOriginalPDFEmbedsInline: страница-просмотрщик PDF ссылается на
// inline-эндпоинт, иначе iframe снова покажет пустоту вместо документа.
func TestViewOriginalPDFEmbedsInline(t *testing.T) {
	s, _ := newTestServerWithPDF(t)
	h := mux(s)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/documents/doc1/view-original", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("view-original: код %d, ожидался 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/documents/doc1/download?inline=1") {
		t.Errorf("view-original: iframe не использует inline-эндпоинт; тело:\n%s", body)
	}
}

// newTestServerWithFile поднимает Server с одним документом, чей локальный файл
// имеет указанное имя и содержимое.
func newTestServerWithFile(t *testing.T, name string, content []byte) *Server {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("запись файла: %v", err)
	}
	st, err := store.NewJSONStore(filepath.Join(dir, "registry.json"))
	if err != nil {
		t.Fatalf("JSONStore: %v", err)
	}
	if err := st.Upsert(context.Background(), model.Document{ID: "doc1", Title: "T", Status: model.StatusActive, LocalPath: p}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return &Server{store: st}
}

// TestViewOriginalBinaryShowsNotice: бинарный формат (старый .doc с нулевыми
// байтами) не выводится «кашей», а показывает плашку с предложением скачать.
func TestViewOriginalBinaryShowsNotice(t *testing.T) {
	// \xD0\xCF... — сигнатура OLE2 (.doc), содержит нулевые байты.
	bin := []byte{0xD0, 0xCF, 0x11, 0xE0, 0x00, 0x00, 0x00, 0x00, 'A', 'B'}
	s := newTestServerWithFile(t, "old.doc", bin)
	rec := httptest.NewRecorder()
	mux(s).ServeHTTP(rec, httptest.NewRequest("GET", "/documents/doc1/view-original", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d, ожидался 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "нельзя показать как текст") {
		t.Errorf("нет плашки о бинарном формате; тело:\n%s", body)
	}
	if strings.Contains(body, `<div class="content">`) {
		t.Errorf("бинарный файл не должен выводиться в блоке content")
	}
}

// TestViewOriginalUnknownTextRenders: текстовый файл с неизвестным расширением
// (.csv) всё равно показывается как текст.
func TestViewOriginalUnknownTextRenders(t *testing.T) {
	s := newTestServerWithFile(t, "data.csv", []byte("a,b,c\n1,2,3\n"))
	rec := httptest.NewRecorder()
	mux(s).ServeHTTP(rec, httptest.NewRequest("GET", "/documents/doc1/view-original", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `<div class="content">`) || !strings.Contains(body, "a,b,c") {
		t.Errorf("текстовый .csv должен рендериться как текст; тело:\n%s", body)
	}
}
