// source_tools.go — MCP-инструменты для расширенных источников:
// мероприятия, конкурсы, FAQ, резиденты, связи документов.
//
// Инструменты:
//   - search_events         — поиск мероприятий с фильтрацией по дате и категории;
//   - search_contests       — поиск конкурсов/грантов с фильтрацией по статусу и категории;
//   - search_faq            — поиск по FAQ с фильтрацией по категории;
//   - search_residents      — поиск резидентов по отрасли, статусу и текстовому запросу;
//   - get_document_links    — связи указанного документа (опц. по типу);
//   - get_event             — мероприятие по идентификатору;
//   - get_contest           — конкурс/грант по идентификатору.
package mcpserver

import (
	"context"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// RegisterSourceTools регистрирует инструменты расширенных источников на MCP-сервере.
func RegisterSourceTools(
	srv *server.MCPServer,
	eventStore store.EventStore,
	contestStore store.ContestStore,
	faqStore store.FAQStore,
	residentStore store.ResidentStore,
	linkStore store.DocumentLinkStore,
) {
	registerSourceTools(srv, eventStore, contestStore, faqStore, residentStore, linkStore)
}

// registerSourceTools регистрирует инструменты расширенных источников на MCP-сервере.
func registerSourceTools(
	srv *server.MCPServer,
	eventStore store.EventStore,
	contestStore store.ContestStore,
	faqStore store.FAQStore,
	residentStore store.ResidentStore,
	linkStore store.DocumentLinkStore,
) {
	// --- search_events ---
	srv.AddTool(
		mcp.NewTool("search_events",
			mcp.WithDescription("Поиск мероприятий с фильтрацией по текстовому запросу, диапазону дат и категории."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Поисковый запрос (опционально)")),
			mcp.WithString("date_from", mcp.Description("Начальная дата в формате YYYY-MM-DD (опционально)")),
			mcp.WithString("date_to", mcp.Description("Конечная дата в формате YYYY-MM-DD (опционально)")),
			mcp.WithString("category", mcp.Description("Категория мероприятия (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (по умолчанию 20)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSearchEvents(ctx, req, eventStore)
		},
	)

	// --- search_contests ---
	srv.AddTool(
		mcp.NewTool("search_contests",
			mcp.WithDescription("Поиск конкурсов/грантов с фильтрацией по статусу и категории."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Поисковый запрос (опционально)")),
			mcp.WithString("status", mcp.Description("Статус: active, closed, winner_selected (опционально)")),
			mcp.WithString("category", mcp.Description("Категория (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (опционально)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSearchContests(ctx, req, contestStore)
		},
	)

	// --- search_faq ---
	srv.AddTool(
		mcp.NewTool("search_faq",
			mcp.WithDescription("Поиск по FAQ с фильтрацией по категории."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Поисковый запрос (опционально)")),
			mcp.WithString("category", mcp.Description("Категория FAQ (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (опционально)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSearchFAQ(ctx, req, faqStore)
		},
	)

	// --- search_residents ---
	srv.AddTool(
		mcp.NewTool("search_residents",
			mcp.WithDescription("Поиск резидентов по отрасли, статусу и текстовому запросу (имя, ИНН)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Поисковый запрос по имени или ИНН (опционально)")),
			mcp.WithString("industry", mcp.Description("Отрасль (опционально)")),
			mcp.WithString("status", mcp.Description("Статус: active, inactive (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (опционально)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSearchResidents(ctx, req, residentStore)
		},
	)

	// --- get_document_links ---
	srv.AddTool(
		mcp.NewTool("get_document_links",
			mcp.WithDescription("Получить связи указанного документа. Можно отфильтровать по типу связи."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("document_id", mcp.Required(), mcp.Description("Идентификатор документа")),
			mcp.WithString("link_type", mcp.Description("Тип связи: references, supersedes, related (опционально)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetDocumentLinks(ctx, req, linkStore)
		},
	)

	// --- get_event ---
	srv.AddTool(
		mcp.NewTool("get_event",
			mcp.WithDescription("Получить мероприятие по идентификатору."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("event_id", mcp.Required(), mcp.Description("Идентификатор мероприятия")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetEvent(ctx, req, eventStore)
		},
	)

	// --- get_contest ---
	srv.AddTool(
		mcp.NewTool("get_contest",
			mcp.WithDescription("Получить конкурс/грант по идентификатору."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("contest_id", mcp.Required(), mcp.Description("Идентификатор конкурса/гранта")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetContest(ctx, req, contestStore)
		},
	)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleSearchEvents(ctx context.Context, req mcp.CallToolRequest, es store.EventStore) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	dateFromStr := req.GetString("date_from", "")
	dateToStr := req.GetString("date_to", "")
	category := req.GetString("category", "")
	limit := req.GetInt("limit", 20)

	var dateFrom, dateTo *time.Time
	const dateFmt = "2006-01-02"
	if dateFromStr != "" {
		t, err := time.Parse(dateFmt, dateFromStr)
		if err != nil {
			return mcp.NewToolResultError("неверный формат date_from, ожидается YYYY-MM-DD"), nil
		}
		dateFrom = &t
	}
	if dateToStr != "" {
		t, err := time.Parse(dateFmt, dateToStr)
		if err != nil {
			return mcp.NewToolResultError("неверный формат date_to, ожидается YYYY-MM-DD"), nil
		}
		dateTo = &t
	}

	events, err := es.ListEvents(ctx, category, "", dateFrom, dateTo)
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска мероприятий: " + err.Error()), nil
	}

	// Фильтрация по текстовому запросу (поиск по заголовку и описанию).
	if query != "" {
		q := strings.ToLower(query)
		filtered := make([]*model.Event, 0, len(events))
		for _, e := range events {
			if strings.Contains(strings.ToLower(e.Title), q) || strings.Contains(strings.ToLower(e.Description), q) {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	return mcp.NewToolResultText(toJSON(events)), nil
}

func handleSearchContests(ctx context.Context, req mcp.CallToolRequest, cs store.ContestStore) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	statusStr := req.GetString("status", "")
	category := req.GetString("category", "")
	limit := req.GetInt("limit", 0)

	var status model.ContestStatus
	if statusStr != "" {
		status = model.ContestStatus(statusStr)
	}

	contests, err := cs.ListContests(ctx, category, status)
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска конкурсов: " + err.Error()), nil
	}

	if query != "" {
		q := strings.ToLower(query)
		filtered := make([]*model.Contest, 0, len(contests))
		for _, c := range contests {
			if strings.Contains(strings.ToLower(c.Title), q) || strings.Contains(strings.ToLower(c.Description), q) {
				filtered = append(filtered, c)
			}
		}
		contests = filtered
	}

	if limit > 0 && len(contests) > limit {
		contests = contests[:limit]
	}

	return mcp.NewToolResultText(toJSON(contests)), nil
}

func handleSearchFAQ(ctx context.Context, req mcp.CallToolRequest, fs store.FAQStore) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	category := req.GetString("category", "")
	limit := req.GetInt("limit", 0)

	items, err := fs.ListFAQItems(ctx, category)
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска FAQ: " + err.Error()), nil
	}

	if query != "" {
		q := strings.ToLower(query)
		filtered := make([]*model.FAQItem, 0, len(items))
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Question), q) || strings.Contains(strings.ToLower(item.Answer), q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	return mcp.NewToolResultText(toJSON(items)), nil
}

func handleSearchResidents(ctx context.Context, req mcp.CallToolRequest, rs store.ResidentStore) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	industry := req.GetString("industry", "")
	statusStr := req.GetString("status", "")
	limit := req.GetInt("limit", 0)

	var status model.ResidentStatus
	if statusStr != "" {
		status = model.ResidentStatus(statusStr)
	}

	residents, err := rs.ListResidents(ctx, industry, status, query)
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска резидентов: " + err.Error()), nil
	}

	if limit > 0 && len(residents) > limit {
		residents = residents[:limit]
	}

	return mcp.NewToolResultText(toJSON(residents)), nil
}

func handleGetDocumentLinks(ctx context.Context, req mcp.CallToolRequest, ls store.DocumentLinkStore) (*mcp.CallToolResult, error) {
	documentID, err := req.RequireString("document_id")
	if err != nil {
		return mcp.NewToolResultError("параметр document_id обязателен"), nil
	}

	linkTypeStr := req.GetString("link_type", "")
	var linkType model.DocumentLinkType
	if linkTypeStr != "" {
		linkType = model.DocumentLinkType(linkTypeStr)
	}

	links, err := ls.GetDocumentLinks(ctx, documentID, linkType)
	if err != nil {
		return mcp.NewToolResultError("ошибка получения связей документа: " + err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(links)), nil
}

func handleGetEvent(ctx context.Context, req mcp.CallToolRequest, es store.EventStore) (*mcp.CallToolResult, error) {
	eventID, err := req.RequireString("event_id")
	if err != nil {
		return mcp.NewToolResultError("параметр event_id обязателен"), nil
	}

	event, err := es.GetEvent(ctx, eventID)
	if err != nil {
		return mcp.NewToolResultError("мероприятие не найдено: " + err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(event)), nil
}

func handleGetContest(ctx context.Context, req mcp.CallToolRequest, cs store.ContestStore) (*mcp.CallToolResult, error) {
	contestID, err := req.RequireString("contest_id")
	if err != nil {
		return mcp.NewToolResultError("параметр contest_id обязателен"), nil
	}

	contest, err := cs.GetContest(ctx, contestID)
	if err != nil {
		return mcp.NewToolResultError("конкурс не найден: " + err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(contest)), nil
}
