// regulation_tools.go — MCP-инструменты для льгот (Preferences) и нормативно-правовых актов (НПА).
//
// Инструменты:
//   - search_preferences — поиск льгот с фильтрацией по типу и статусу;
//   - get_preference     — получить льготу по идентификатору;
//   - search_npa         — поиск НПА с фильтрацией по типу и статусу;
//   - get_npa            — получить НПА по идентификатору.
package mcpserver

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// RegisterRegulationTools регистрирует инструменты льгот и НПА на MCP-сервере.
func RegisterRegulationTools(
	srv *server.MCPServer,
	prefStore store.PreferenceStore,
	npaStore store.NPAStore,
) {
	registerRegulationTools(srv, prefStore, npaStore)
}

// registerRegulationTools регистрирует инструменты льгот и НПА на MCP-сервере.
func registerRegulationTools(
	srv *server.MCPServer,
	prefStore store.PreferenceStore,
	npaStore store.NPAStore,
) {
	// --- search_preferences ---
	srv.AddTool(
		mcp.NewTool("search_preferences",
			mcp.WithDescription("Поиск льгот с фильтрацией по типу и статусу."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Поисковый запрос (опционально)")),
			mcp.WithString("pref_type", mcp.Description("Тип льготы: tax_profit, insurance, vat, customs, other (опционально)")),
			mcp.WithString("status", mcp.Description("Статус: active, outdated (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (по умолчанию 20)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSearchPreferences(ctx, req, prefStore)
		},
	)

	// --- get_preference ---
	srv.AddTool(
		mcp.NewTool("get_preference",
			mcp.WithDescription("Получить льготу по идентификатору."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("preference_id", mcp.Required(), mcp.Description("Идентификатор льготы")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetPreference(ctx, req, prefStore)
		},
	)

	// --- search_npa ---
	srv.AddTool(
		mcp.NewTool("search_npa",
			mcp.WithDescription("Поиск нормативно-правовых актов с фильтрацией по типу и статусу."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Поисковый запрос (опционально)")),
			mcp.WithString("npa_type", mcp.Description("Тип НПА: law, decree, order, decision (опционально)")),
			mcp.WithString("status", mcp.Description("Статус: active, amended, revoked (опционально)")),
			mcp.WithNumber("limit", mcp.Description("Максимум записей (по умолчанию 20)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSearchNPA(ctx, req, npaStore)
		},
	)

	// --- get_npa ---
	srv.AddTool(
		mcp.NewTool("get_npa",
			mcp.WithDescription("Получить нормативно-правовой акт по идентификатору."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("npa_id", mcp.Required(), mcp.Description("Идентификатор НПА")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleGetNPA(ctx, req, npaStore)
		},
	)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleSearchPreferences(ctx context.Context, req mcp.CallToolRequest, ps store.PreferenceStore) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	prefTypeStr := req.GetString("pref_type", "")
	statusStr := req.GetString("status", "")
	limit := req.GetInt("limit", 20)

	var prefType model.PreferenceType
	if prefTypeStr != "" {
		prefType = model.PreferenceType(prefTypeStr)
	}
	var status model.PreferenceStatus
	if statusStr != "" {
		status = model.PreferenceStatus(statusStr)
	}

	prefs, err := ps.ListPreferences(ctx, string(prefType), string(status))
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска льгот: "+err.Error()), nil
	}

	// Фильтрация по текстовому запросу (поиск по названию и описанию льготы).
	if query != "" {
		q := strings.ToLower(query)
		filtered := make([]*model.Preference, 0, len(prefs))
		for _, p := range prefs {
			if strings.Contains(strings.ToLower(p.Title), q) || strings.Contains(strings.ToLower(p.BenefitDesc), q) {
				filtered = append(filtered, p)
			}
		}
		prefs = filtered
	}

	if limit > 0 && len(prefs) > limit {
		prefs = prefs[:limit]
	}

	return mcp.NewToolResultText(toJSON(prefs)), nil
}

func handleGetPreference(ctx context.Context, req mcp.CallToolRequest, ps store.PreferenceStore) (*mcp.CallToolResult, error) {
	preferenceID, err := req.RequireString("preference_id")
	if err != nil {
		return mcp.NewToolResultError("параметр preference_id обязателен"), nil
	}

	pref, err := ps.GetPreference(ctx, preferenceID)
	if err != nil {
		return mcp.NewToolResultError("льгота не найдена: "+err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(pref)), nil
}

func handleSearchNPA(ctx context.Context, req mcp.CallToolRequest, ns store.NPAStore) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	npaTypeStr := req.GetString("npa_type", "")
	statusStr := req.GetString("status", "")
	limit := req.GetInt("limit", 20)

	var npaType model.NPAType
	if npaTypeStr != "" {
		npaType = model.NPAType(npaTypeStr)
	}
	var status model.NPAStatus
	if statusStr != "" {
		status = model.NPAStatus(statusStr)
	}

	npas, err := ns.ListNPA(ctx, string(npaType), string(status))
	if err != nil {
		return mcp.NewToolResultError("ошибка поиска НПА: "+err.Error()), nil
	}

	// Фильтрация по текстовому запросу (поиск по названию и краткому описанию).
	if query != "" {
		q := strings.ToLower(query)
		filtered := make([]*model.NPADocument, 0, len(npas))
		for _, n := range npas {
			if strings.Contains(strings.ToLower(n.Title), q) || strings.Contains(strings.ToLower(n.Summary), q) {
				filtered = append(filtered, n)
			}
		}
		npas = filtered
	}

	if limit > 0 && len(npas) > limit {
		npas = npas[:limit]
	}

	return mcp.NewToolResultText(toJSON(npas)), nil
}

func handleGetNPA(ctx context.Context, req mcp.CallToolRequest, ns store.NPAStore) (*mcp.CallToolResult, error) {
	npaID, err := req.RequireString("npa_id")
	if err != nil {
		return mcp.NewToolResultError("параметр npa_id обязателен"), nil
	}

	npa, err := ns.GetNPA(ctx, npaID)
	if err != nil {
		return mcp.NewToolResultError("НПА не найден: "+err.Error()), nil
	}

	return mcp.NewToolResultText(toJSON(npa)), nil
}
