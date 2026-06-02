// files.go — открытый (без API-ключа) read-only эндпоинт отдачи скачанной копии
// документа: GET /files/{id}. Нужен для кликабельных ссылок на источники в
// ответах консультанта (чат-виджет, Telegram-бот): прямые ссылки на тело
// документа dochub (/m/docs/) блокирует WAF, поэтому при наличии локальной копии
// консультант ссылается сюда (см. agents/source_links.go, WithSourceLinks).
//
// Отдаются только действующие документы с наличным локальным файлом. Документы
// Сколково публичны, MCP-сервер по дизайну открытый — отдача нормативного файла
// безопасна. Путь к файлу берётся из нашего реестра (не из ввода пользователя),
// поэтому обхода каталога нет.
package mcpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"baza-skolkovo/src/common/model"
)

// handleDocumentFile отдаёт локальную копию документа по пути /files/{id}.
func (s *Server) handleDocumentFile(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/files/"), "/")
	if id == "" {
		http.Error(w, "не указан идентификатор документа", http.StatusBadRequest)
		return
	}

	doc, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if doc.Status != model.StatusActive {
		http.Error(w, "документ недоступен", http.StatusForbidden)
		return
	}
	if strings.TrimSpace(doc.LocalPath) == "" {
		http.Error(w, "файл документа недоступен", http.StatusNotFound)
		return
	}

	f, err := os.Open(doc.LocalPath)
	if err != nil {
		http.Error(w, "файл недоступен", http.StatusNotFound)
		return
	}
	defer f.Close()

	name := filepath.Base(doc.LocalPath)
	w.Header().Set("Content-Type", mimeByExt(name))
	w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// ServeContent добавит ETag/Range/кеширование; *os.File реализует io.ReadSeeker.
	http.ServeContent(w, r, name, doc.FetchedAt, f)
}
