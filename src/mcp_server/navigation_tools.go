// navigation_tools.go — MCP-инструмент get_navigation: навигация по сайту.
//
// Отвечает на вопросы «где это находится на сайте» и «как туда попасть»,
// используя специализированный навигационный индекс (пакет navindex,
// отдельная Qdrant-коллекция). Не смешивается с семантическим поиском по
// документам (search_documents) — это разные каналы знаний для чат-бота.
package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/navindex"
)

// NavSearcher — то, что умеет искать по навигационной карте (реализуется
// navindex.Searcher). Интерфейс позволяет не тянуть зависимость на конкретный тип.
type NavSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]navindex.Hit, error)
}

// RegisterNavigationTools регистрирует get_navigation на MCP-сервере.
func RegisterNavigationTools(srv *server.MCPServer, ns NavSearcher) {
	if ns == nil {
		return
	}
	srv.AddTool(
		mcp.NewTool("get_navigation",
			mcp.WithDescription("Навигация по интерфейсам системы «База Сколково»: где на сайте находится нужная функция, страница, вкладка или блок, и как туда попасть. Используйте, когда пользователь спрашивает «где посмотреть…», «как открыть…», «где подать заявку/отчёт», «куда нажать», «на какой странице…». Возвращает интерфейс, порт, маршрут, заголовок страницы, блок и инструкцию «как попасть» (howto). Это про устройство САЙТА, а не про содержание документов (для документов — search_documents)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Required(), mcp.Description("Что ищет пользователь на сайте (например: «где сменить стадию клиента», «как подать заявку», «где уведомления об изменениях»)")),
			mcp.WithNumber("limit", mcp.Description("Максимум результатов (по умолчанию 5)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query := req.GetString("query", "")
			if query == "" {
				return mcp.NewToolResultError("параметр query обязателен"), nil
			}
			limit := req.GetInt("limit", 5)
			hits, err := ns.Search(ctx, query, limit)
			if err != nil {
				return mcp.NewToolResultError("ошибка навигации: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(hits)), nil
		},
	)
}
