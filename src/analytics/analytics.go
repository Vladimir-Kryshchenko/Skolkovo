// Package analytics предоставляет модуль сбора и визуализации метрик системы
// «База Сколково»: документы, клиенты, дедлайны, мероприятия, конкурсы,
// чек-листы и MCP-запросы.
package analytics

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// ---------------------------------------------------------------------------
// Структуры отчёта
// ---------------------------------------------------------------------------

// DocumentStats — статистика по документам.
type DocumentStats struct {
	Total        int            `json:"total"`
	ByStatus     map[string]int `json:"by_status"`
	ByCategory   map[string]int `json:"by_category"`
	IndexedCount int            `json:"indexed_count"`
}

// ClientStats — статистика по клиентам.
type ClientStats struct {
	Total        int            `json:"total"`
	ByStage      map[string]int `json:"by_stage"`
	NewThisWeek  int            `json:"new_this_week"`
	NewThisMonth int            `json:"new_this_month"`
}

// DeadlineStats — статистика по дедлайнам.
type DeadlineStats struct {
	Total      int `json:"total"`
	Overdue    int `json:"overdue"`
	Upcoming30d int `json:"upcoming_30d"`
	Completed  int `json:"completed"`
}

// EventStats — статистика по мероприятиям.
type EventStats struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Past   int `json:"past"`
}

// ContestStats — статистика по конкурсам.
type ContestStats struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Closed int `json:"closed"`
}

// ChecklistStats — статистика по чек-листам.
type ChecklistStats struct {
	Total       int `json:"total"`
	InProgress  int `json:"in_progress"`
	Completed   int `json:"completed"`
}

// MCPStats — статистика MCP-запросов (заглушка).
type MCPStats struct {
	TotalRequests int64   `json:"total_requests"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	ErrorRate     float64 `json:"error_rate"`
}

// Period — период отчёта.
type Period struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// AnalyticsReport — полный отчёт аналитики.
type AnalyticsReport struct {
	DocumentStats  DocumentStats  `json:"document_stats"`
	ClientStats    ClientStats    `json:"client_stats"`
	DeadlineStats  DeadlineStats  `json:"deadline_stats"`
	EventStats     EventStats     `json:"event_stats"`
	ContestStats   ContestStats   `json:"contest_stats"`
	ChecklistStats ChecklistStats `json:"checklist_stats"`
	MCPStats       MCPStats       `json:"mcp_stats"`
	Period         Period         `json:"period"`
}

// PopularQuery — популярный запрос (заглушка для MCP audit log).
type PopularQuery struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

// ---------------------------------------------------------------------------
// CollectReport
// ---------------------------------------------------------------------------

// CollectReport собирает статистику из всех хранилищ и возвращает полный отчёт.
func CollectReport(
	ctx context.Context,
	docStore store.Store,
	clientStore store.ClientStore,
	checklistStore store.ChecklistStore,
	deadlineStore store.DeadlineStore,
	eventStore store.EventStore,
	contestStore store.ContestStore,
) *AnalyticsReport {
	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)
	thirtyDaysAhead := now.Add(30 * 24 * time.Hour)

	report := &AnalyticsReport{
		Period: Period{From: now.Truncate(24 * time.Hour), To: now},
	}

	// --- Documents ---
	collectDocStats(ctx, docStore, &report.DocumentStats)

	// --- Clients ---
	collectClientStats(ctx, clientStore, weekAgo, monthAgo, &report.ClientStats)

	// --- Deadlines ---
	collectDeadlineStats(ctx, deadlineStore, thirtyDaysAhead, now, &report.DeadlineStats)

	// --- Events ---
	collectEventStats(ctx, eventStore, &report.EventStats)

	// --- Contests ---
	collectContestStats(ctx, contestStore, &report.ContestStats)

	// --- Checklists ---
	collectChecklistStats(ctx, checklistStore, &report.ChecklistStats)

	// --- MCP (заглушка) ---
	report.MCPStats = MCPStats{
		TotalRequests: 0,
		AvgLatencyMs:  0,
		ErrorRate:     0,
	}

	return report
}

func collectDocStats(ctx context.Context, s store.Store, ds *DocumentStats) {
	ds.ByStatus = make(map[string]int)
	ds.ByCategory = make(map[string]int)

	// Собираем все документы без фильтра
	docs, err := s.List(ctx, store.Filter{})
	if err != nil {
		return
	}

	ds.Total = len(docs)
	for _, d := range docs {
		ds.ByStatus[string(d.Status)]++
		if d.Category != "" {
			ds.ByCategory[d.Category]++
		}
		if d.Indexed {
			ds.IndexedCount++
		}
	}
}

func collectClientStats(ctx context.Context, s store.ClientStore, weekAgo, monthAgo time.Time, cs *ClientStats) {
	cs.ByStage = make(map[string]int)

	// Получаем всех клиентов (пустая стадия = без фильтра)
	// Нужно перебрать всех тенантов — пока берём без tenantID
	// Реализация ListClients требует tenantID, поэтому используем заглушку
	// В продакшене нужен метод ListAllClients или итерация по тенантам
	clients, err := s.ListClients(ctx, "", model.ResidencyStage(""))
	if err != nil {
		return
	}

	cs.Total = len(clients)
	for _, c := range clients {
		cs.ByStage[string(c.ResidencyStage)]++
		if c.CreatedAt.After(weekAgo) {
			cs.NewThisWeek++
		}
		if c.CreatedAt.After(monthAgo) {
			cs.NewThisMonth++
		}
	}
}

func collectDeadlineStats(ctx context.Context, s store.DeadlineStore, thirtyDaysAhead, now time.Time, dls *DeadlineStats) {
	// Просроченные
	overdue, err := s.ListOverdueDeadlines(ctx)
	if err == nil {
		dls.Overdue = len(overdue)
	}

	// Предстоящие 30 дней
	upcoming, err := s.ListDeadlines(ctx, "", 30)
	if err == nil {
		dls.Upcoming30d = len(upcoming)
	}

	// Считаем total и completed из upcoming + overdue
	seen := make(map[string]bool)
	for _, d := range upcoming {
		seen[d.ID] = true
		if d.Status == model.DeadlineCompleted {
			dls.Completed++
		}
	}
	for _, d := range overdue {
		if !seen[d.ID] {
			dls.Total++
			seen[d.ID] = true
		}
	}
	dls.Total += dls.Upcoming30d
}

func collectEventStats(ctx context.Context, s store.EventStore, es *EventStats) {
	total, err := s.CountEvents(ctx)
	if err == nil {
		es.Total = total
	}

	// Активные
	events, err := s.ListEvents(ctx, "", model.EventActive, nil, nil)
	if err == nil {
		es.Active = len(events)
	}

	// Прошедшие
	eventsPast, err := s.ListEvents(ctx, "", model.EventPast, nil, nil)
	if err == nil {
		es.Past = len(eventsPast)
	}
}

func collectContestStats(ctx context.Context, s store.ContestStore, cos *ContestStats) {
	// Активные
	active, err := s.CountActiveContests(ctx)
	if err == nil {
		cos.Active = active
	}

	// Все конкурсы для total и closed
	all, err := s.ListContests(ctx, "", model.ContestStatus(""))
	if err == nil {
		cos.Total = len(all)
		for _, c := range all {
			if c.Status == model.ContestClosed {
				cos.Closed++
			}
		}
	}
}

func collectChecklistStats(ctx context.Context, s store.ChecklistStore, cls *ChecklistStats) {
	// Все шаблоны чек-листов
	checklists, err := s.ListChecklists(ctx, model.ChecklistType(""))
	if err == nil {
		cls.Total = len(checklists)
	}

	// Привязки к клиентам — нужно перебрать всех клиентов
	// В заглушке считаем через total шаблонов
	// В продакшене нужен метод ListAllClientChecklists
}

// ---------------------------------------------------------------------------
// ToHTML
// ---------------------------------------------------------------------------

// ToHTML генерирует HTML-дашборд с графиками Chart.js.
func ToHTML(report *AnalyticsReport) string {
	const tpl = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Аналитика — База Сколково</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.4/dist/chart.umd.min.js"></script>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
         margin: 0; padding: 20px; background: #f5f7fa; color: #333; }
  h1 { text-align: center; margin-bottom: 30px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
          gap: 20px; max-width: 1400px; margin: 0 auto; }
  .card { background: #fff; border-radius: 12px; padding: 20px;
          box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
  .card h2 { margin-top: 0; font-size: 1.1rem; color: #555; }
  .stat-row { display: flex; justify-content: space-between; padding: 6px 0;
              border-bottom: 1px solid #eee; }
  .stat-row:last-child { border-bottom: none; }
  .stat-value { font-weight: 600; color: #1a73e8; }
  canvas { max-height: 280px; }
  .period { text-align: center; color: #888; margin-bottom: 20px; font-size: 0.9rem; }
</style>
</head>
<body>
<h1>📊 Аналитика «База Сколково»</h1>
<div class="period">Период: %s — %s</div>
<div class="grid">

  <!-- Documents pie -->
  <div class="card">
    <h2>Документы по статусам</h2>
    <canvas id="chartDocStatus"></canvas>
    <div>%s</div>
  </div>

  <!-- Clients bar -->
  <div class="card">
    <h2>Клиенты по стадиям</h2>
    <canvas id="chartClientStage"></canvas>
    <div>%s</div>
  </div>

  <!-- Deadlines timeline -->
  <div class="card">
    <h2>Дедлайны</h2>
    <div class="stat-row"><span>Всего</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Просрочено</span><span class="stat-value" style="color:#d32f2f">%d</span></div>
    <div class="stat-row"><span>Предстоящие (30 дн.)</span><span class="stat-value" style="color:#f9a825">%d</span></div>
    <div class="stat-row"><span>Выполнено</span><span class="stat-value" style="color:#2e7d32">%d</span></div>
  </div>

  <!-- Events & Contests -->
  <div class="card">
    <h2>Мероприятия и конкурсы</h2>
    <div class="stat-row"><span>Мероприятия (всего)</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Мероприятия (активные)</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Мероприятия (прошедшие)</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Конкурсы (всего)</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Конкурсы (активные)</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Конкурсы (закрытые)</span><span class="stat-value">%d</span></div>
  </div>

  <!-- Checklists -->
  <div class="card">
    <h2>Чек-листы</h2>
    <div class="stat-row"><span>Всего шаблонов</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>В работе</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Завершено</span><span class="stat-value">%d</span></div>
  </div>

  <!-- MCP -->
  <div class="card">
    <h2>MCP-запросы</h2>
    <div class="stat-row"><span>Всего запросов</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Средняя задержка</span><span class="stat-value">%.1f мс</span></div>
    <div class="stat-row"><span>Доля ошибок</span><span class="stat-value">%.2f%%</span></div>
  </div>

</div>

<script>
const docLabels = %s;
const docData  = %s;
new Chart(document.getElementById('chartDocStatus'), {
  type: 'pie',
  data: { labels: docLabels, datasets: [{ data: docData,
    backgroundColor: ['#1a73e8','#2e7d32','#f9a825','#d32f2f','#7b1fa2'] }] }
});

const cliLabels = %s;
const cliData   = %s;
new Chart(document.getElementById('chartClientStage'), {
  type: 'bar',
  data: { labels: cliLabels, datasets: [{ label: 'Клиенты', data: cliData,
    backgroundColor: '#1a73e8' }] },
  options: { scales: { y: { beginAtZero: true, ticks: { stepSize: 1 } } },
             plugins: { legend: { display: false } } }
});
</script>
</body>
</html>`

	fromStr := report.Period.From.Format("2006-01-02")
	toStr := report.Period.To.Format("2006-01-02")

	docStatusRows := mapToRows(report.DocumentStats.ByStatus)
	clientStageRows := mapToRows(report.ClientStats.ByStage)

	return fmt.Sprintf(tpl,
		// period
		fromStr, toStr,
		// doc detail rows
		rowsToHTML(docStatusRows),
		// client detail rows
		rowsToHTML(clientStageRows),
		// deadlines
		report.DeadlineStats.Total,
		report.DeadlineStats.Overdue,
		report.DeadlineStats.Upcoming30d,
		report.DeadlineStats.Completed,
		// events
		report.EventStats.Total,
		report.EventStats.Active,
		report.EventStats.Past,
		// contests
		report.ContestStats.Total,
		report.ContestStats.Active,
		report.ContestStats.Closed,
		// checklists
		report.ChecklistStats.Total,
		report.ChecklistStats.InProgress,
		report.ChecklistStats.Completed,
		// mcp
		report.MCPStats.TotalRequests,
		report.MCPStats.AvgLatencyMs,
		report.MCPStats.ErrorRate,
		// chart data
		jsonStrArr(docStatusRows.labels),
		jsonIntArr(docStatusRows.values),
		jsonStrArr(clientStageRows.labels),
		jsonIntArr(clientStageRows.values),
	)
}

type rowSet struct {
	labels []string
	values []int
}

func mapToRows(m map[string]int) rowSet {
	rs := rowSet{}
	for k := range m {
		rs.labels = append(rs.labels, k)
	}
	sort.Strings(rs.labels)
	for _, k := range rs.labels {
		rs.values = append(rs.values, m[k])
	}
	return rs
}

func rowsToHTML(rs rowSet) string {
	var b strings.Builder
	for i, l := range rs.labels {
		b.WriteString(fmt.Sprintf("<div class=\"stat-row\"><span>%s</span><span class=\"stat-value\">%d</span></div>",
			escapeHTML(l), rs.values[i]))
	}
	return b.String()
}

func jsonStrArr(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = fmt.Sprintf("%q", escapeHTML(s))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func jsonIntArr(is []int) string {
	parts := make([]string, len(is))
	for i, v := range is {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// ---------------------------------------------------------------------------
// ToCSV
// ---------------------------------------------------------------------------

// ToCSV экспортирует отчёт в CSV-формат.
func ToCSV(report *AnalyticsReport) string {
	var b strings.Builder
	w := csv.NewWriter(&b)

	// Header
	w.Write([]string{"Section", "Metric", "Value"})

	// Documents
	w.Write([]string{"Documents", "total", fmt.Sprintf("%d", report.DocumentStats.Total)})
	for k, v := range report.DocumentStats.ByStatus {
		w.Write([]string{"Documents", "by_status:" + k, fmt.Sprintf("%d", v)})
	}
	for k, v := range report.DocumentStats.ByCategory {
		w.Write([]string{"Documents", "by_category:" + k, fmt.Sprintf("%d", v)})
	}
	w.Write([]string{"Documents", "indexed_count", fmt.Sprintf("%d", report.DocumentStats.IndexedCount)})

	// Clients
	w.Write([]string{"Clients", "total", fmt.Sprintf("%d", report.ClientStats.Total)})
	for k, v := range report.ClientStats.ByStage {
		w.Write([]string{"Clients", "by_stage:" + k, fmt.Sprintf("%d", v)})
	}
	w.Write([]string{"Clients", "new_this_week", fmt.Sprintf("%d", report.ClientStats.NewThisWeek)})
	w.Write([]string{"Clients", "new_this_month", fmt.Sprintf("%d", report.ClientStats.NewThisMonth)})

	// Deadlines
	w.Write([]string{"Deadlines", "total", fmt.Sprintf("%d", report.DeadlineStats.Total)})
	w.Write([]string{"Deadlines", "overdue", fmt.Sprintf("%d", report.DeadlineStats.Overdue)})
	w.Write([]string{"Deadlines", "upcoming_30d", fmt.Sprintf("%d", report.DeadlineStats.Upcoming30d)})
	w.Write([]string{"Deadlines", "completed", fmt.Sprintf("%d", report.DeadlineStats.Completed)})

	// Events
	w.Write([]string{"Events", "total", fmt.Sprintf("%d", report.EventStats.Total)})
	w.Write([]string{"Events", "active", fmt.Sprintf("%d", report.EventStats.Active)})
	w.Write([]string{"Events", "past", fmt.Sprintf("%d", report.EventStats.Past)})

	// Contests
	w.Write([]string{"Contests", "total", fmt.Sprintf("%d", report.ContestStats.Total)})
	w.Write([]string{"Contests", "active", fmt.Sprintf("%d", report.ContestStats.Active)})
	w.Write([]string{"Contests", "closed", fmt.Sprintf("%d", report.ContestStats.Closed)})

	// Checklists
	w.Write([]string{"Checklists", "total", fmt.Sprintf("%d", report.ChecklistStats.Total)})
	w.Write([]string{"Checklists", "in_progress", fmt.Sprintf("%d", report.ChecklistStats.InProgress)})
	w.Write([]string{"Checklists", "completed", fmt.Sprintf("%d", report.ChecklistStats.Completed)})

	// MCP
	w.Write([]string{"MCP", "total_requests", fmt.Sprintf("%d", report.MCPStats.TotalRequests)})
	w.Write([]string{"MCP", "avg_latency_ms", fmt.Sprintf("%.2f", report.MCPStats.AvgLatencyMs)})
	w.Write([]string{"MCP", "error_rate", fmt.Sprintf("%.4f", report.MCPStats.ErrorRate)})

	// Period
	w.Write([]string{"Period", "from", report.Period.From.Format("2006-01-02")})
	w.Write([]string{"Period", "to", report.Period.To.Format("2006-01-02")})

	w.Flush()
	return b.String()
}

// ---------------------------------------------------------------------------
// GetPopularQueries (заглушка)
// ---------------------------------------------------------------------------

// GetPopularQueries возвращает заглушку — в будущем будет подключаться к MCP audit log.
func GetPopularQueries() []PopularQuery {
	return []PopularQuery{
		{Query: "какие документы нужны для подачи заявки", Count: 42},
		{Query: "сроки рассмотрения заявки", Count: 31},
		{Query: "требования к отчётности резидента", Count: 27},
		{Query: "как продлить договор", Count: 19},
		{Query: "гранты для IT-компаний 2025", Count: 15},
	}
}
