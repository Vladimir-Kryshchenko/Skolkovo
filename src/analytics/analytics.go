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
	Total       int `json:"total"`
	Overdue     int `json:"overdue"`
	Upcoming30d int `json:"upcoming_30d"`
	Completed   int `json:"completed"`
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
	Total      int `json:"total"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
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
	if s == nil {
		return
	}

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
	if s == nil {
		return
	}

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
	if s == nil {
		return
	}
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
	if s == nil {
		return
	}
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
	if s == nil {
		return
	}
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
	if s == nil {
		return
	}
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
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.4/dist/chart.umd.min.js"></script>
<style>
:root {
  --bg: #f0f2f5;
  --surface: #ffffff;
  --text: #181b2b;
  --text-secondary: #6b7280;
  --border: #e0e3eb;
  --primary: #0073ea;
  --primary-hover: #005fbf;
  --stat-color: #0073ea;
  --danger: #de350b;
  --warning: #ff8b00;
  --success: #00875a;
  --shadow: 0 1px 3px rgba(0,0,0,0.08);
  --shadow-md: 0 2px 8px rgba(0,0,0,0.1);
  --radius: 8px;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b;
    --surface: #23273a;
    --text: #e8eaed;
    --text-secondary: #9ca3af;
    --border: #333850;
    --primary: #0073ea;
    --primary-hover: #339af0;
    --stat-color: #339af0;
    --danger: #ff6b6b;
    --warning: #ffa94d;
    --success: #51cf66;
    --shadow: 0 1px 3px rgba(0,0,0,0.3);
    --shadow-md: 0 2px 8px rgba(0,0,0,0.4);
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b;
  --surface: #23273a;
  --text: #e8eaed;
  --text-secondary: #9ca3af;
  --border: #333850;
  --primary: #0073ea;
  --primary-hover: #339af0;
  --stat-color: #339af0;
  --danger: #ff6b6b;
  --warning: #ffa94d;
  --success: #51cf66;
  --shadow: 0 1px 3px rgba(0,0,0,0.3);
  --shadow-md: 0 2px 8px rgba(0,0,0,0.4);
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: 'Figtree', -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  margin: 0;
  padding: 20px;
  background: var(--bg);
  color: var(--text);
}

/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]::after {
  content: attr(data-tooltip);
  position: absolute;
  bottom: calc(100% + 6px);
  left: 50%;
  transform: translateX(-50%);
  background: var(--text);
  color: var(--bg);
  font-size: 11px;
  font-weight: 500;
  padding: 4px 8px;
  border-radius: 4px;
  white-space: nowrap;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.15s ease;
  z-index: 10;
}
[data-tooltip]:hover::after { opacity: 1; }

/* Header */
.header-bar {
  max-width: 1400px;
  margin: 0 auto 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.header-bar h1 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--text);
  display: flex;
  align-items: center;
  gap: 10px;
}
.header-bar h1 svg { width: 24px; height: 24px; fill: var(--primary); }
.header-actions { display: flex; gap: 8px; align-items: center; }

.nav-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  background: var(--primary);
  color: #fff;
  border-radius: var(--radius);
  text-decoration: none;
  font-size: 13px;
  font-weight: 500;
  transition: background 0.15s;
}
.nav-back:hover { background: var(--primary-hover); }

.theme-toggle {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: var(--text);
  transition: background 0.15s, border-color 0.15s;
}
.theme-toggle:hover { background: var(--bg); border-color: var(--primary); }
.theme-toggle svg { width: 18px; height: 18px; fill: currentColor; }

/* Period label */
.period {
  text-align: center;
  color: var(--text-secondary);
  margin-bottom: 24px;
  font-size: 14px;
}

/* Grid */
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
  gap: 20px;
  max-width: 1400px;
  margin: 0 auto;
}

/* Card */
.card {
  background: var(--surface);
  border-radius: var(--radius);
  padding: 20px;
  box-shadow: var(--shadow);
  border: 1px solid var(--border);
}
.card h2 {
  margin-top: 0;
  font-size: 14px;
  color: var(--text-secondary);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.4px;
  margin-bottom: 16px;
}

/* Stat rows */
.stat-row {
  display: flex;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1px solid var(--border);
  align-items: center;
}
.stat-row:last-child { border-bottom: none; }
.stat-value {
  font-weight: 700;
  color: var(--stat-color);
  font-size: 15px;
}
.stat-value.danger { color: var(--danger); }
.stat-value.warning { color: var(--warning); }
.stat-value.success { color: var(--success); }

/* Chart canvas */
canvas { max-height: 280px; }

/* Responsive */
@media (max-width: 768px) {
  body { padding: 12px; }
  .grid { grid-template-columns: 1fr; gap: 16px; }
  .header-bar { margin-bottom: 16px; }
  .header-bar h1 { font-size: 17px; }
  .card { padding: 16px; }
}
@media (max-width: 480px) {
  .header-bar { flex-direction: column; align-items: flex-start; }
  .header-actions { width: 100%; justify-content: flex-start; }
}
@media (min-width: 1024px) {
  .grid { grid-template-columns: repeat(3, 1fr); }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>

<!-- SVG icons -->
<svg style="display:none" xmlns="http://www.w3.org/2000/svg">
  <symbol id="icon-chart" viewBox="0 0 24 24"><path d="M18 20V10M12 20V4M6 20v-6"/></svg>
  <symbol id="icon-moon" viewBox="0 0 24 24"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></symbol>
  <symbol id="icon-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3" stroke="currentColor" stroke-width="2"/><line x1="12" y1="21" x2="12" y2="23" stroke="currentColor" stroke-width="2"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64" stroke="currentColor" stroke-width="2"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78" stroke="currentColor" stroke-width="2"/><line x1="1" y1="12" x2="3" y2="12" stroke="currentColor" stroke-width="2"/><line x1="21" y1="12" x2="23" y2="12" stroke="currentColor" stroke-width="2"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36" stroke="currentColor" stroke-width="2"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" stroke="currentColor" stroke-width="2"/></symbol>
</svg>

<div class="header-bar">
  <h1>
    <svg><use href="#icon-chart"/></svg>
    Аналитика — База Сколково
  </h1>
  <div class="header-actions">
    <a href="/" class="nav-back" data-tooltip="Вернуться к документам">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>
      Документы
    </a>
    <button id="themeBtn" class="theme-toggle" onclick="toggleTheme()" data-tooltip="Переключить тему">
      <svg id="themeIcon"><use href="#icon-moon"/></svg>
    </button>
  </div>
</div>
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

  <!-- Deadlines -->
  <div class="card">
    <h2>Дедлайны</h2>
    <div class="stat-row"><span>Всего</span><span class="stat-value">%d</span></div>
    <div class="stat-row"><span>Просрочено</span><span class="stat-value danger">%d</span></div>
    <div class="stat-row"><span>Предстоящие (30 дн.)</span><span class="stat-value warning">%d</span></div>
    <div class="stat-row"><span>Выполнено</span><span class="stat-value success">%d</span></div>
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
    <div class="stat-row"><span>Завершено</span><span class="stat-value success">%d</span></div>
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
    backgroundColor: ['#0073ea','#00875a','#ffa94d','#ff6b6b','#7b1fa2'] }] },
  options: { responsive: true, maintainAspectRatio: true }
});

const cliLabels = %s;
const cliData   = %s;
new Chart(document.getElementById('chartClientStage'), {
  type: 'bar',
  data: { labels: cliLabels, datasets: [{ label: 'Клиенты', data: cliData,
    backgroundColor: '#0073ea' }] },
  options: {
    responsive: true,
    maintainAspectRatio: true,
    scales: { y: { beginAtZero: true, ticks: { stepSize: 1 } } },
    plugins: { legend: { display: false } }
  }
});

// SVG icons for theme
var moonUse = '<use href="#icon-moon"/>';
var sunUse = '<use href="#icon-sun"/>';

function setThemeIcon(isDark) {
  document.getElementById('themeIcon').innerHTML = isDark ? sunUse : moonUse;
}

function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  setThemeIcon(next === 'dark');
}

function applyThemeIcon() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  setThemeIcon(cur === 'dark');
}

document.addEventListener('DOMContentLoaded', applyThemeIcon);
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
