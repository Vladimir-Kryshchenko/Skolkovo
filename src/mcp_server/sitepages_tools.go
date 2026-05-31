// sitepages_tools.go — MCP-инструмент search_site_pages: поиск по страницам сайта.
//
// Отвечает на вопросы «на какой странице сайта про …», возвращая отдельные
// страницы публичного сайта Сколково (sk.ru/dochub.sk.ru): заголовок, краткое
// описание, раздел и URL. Это ОТДЕЛЬНЫЙ канал от:
//   - search_documents — полный текст файлов-документов;
//   - get_navigation — устройство внутренних интерфейсов системы.
package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/sitepages"
)

// SitePageSearcher — то, что умеет искать по страницам сайта (реализуется
// sitepages.Searcher).
type SitePageSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]sitepages.Hit, error)
}

// RegisterSitePageTools регистрирует search_site_pages на MCP-сервере.
func RegisterSitePageTools(srv *server.MCPServer, s SitePageSearcher) {
	if s == nil {
		return
	}
	srv.AddTool(
		mcp.NewTool("search_site_pages",
			mcp.WithDescription("Поиск по СТРАНИЦАМ публичного сайта Сколково (sk.ru, dochub.sk.ru): одна запись на страницу — заголовок, краткое описание, раздел и URL. Используйте, когда нужно найти НУЖНУЮ СТРАНИЦУ/РАЗДЕЛ сайта (например: «страница про льготы резидентам», «где раздел документов фонда»). Это НЕ текст файлов-документов (для него — search_documents) и НЕ устройство внутренних интерфейсов системы (для него — get_navigation)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Required(), mcp.Description("Что ищет пользователь на сайте (тема/раздел/страница)")),
			mcp.WithNumber("limit", mcp.Description("Максимум результатов (по умолчанию 5)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query := req.GetString("query", "")
			if query == "" {
				return mcp.NewToolResultError("параметр query обязателен"), nil
			}
			limit := req.GetInt("limit", 5)
			hits, err := s.Search(ctx, query, limit)
			if err != nil {
				return mcp.NewToolResultError("ошибка поиска по страницам: " + err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(hits)), nil
		},
	)
}
