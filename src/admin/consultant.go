// Package admin — консультантский дашборд.
// Показывает всех клиентов консультанта с приоритизацией по срочности:
// просроченные дедлайны → дедлайны ≤3 дней → застрявшие клиенты → остальные.
package admin

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// ConsultantDashboardStores — хранилища для дашборда.
type ConsultantDashboardStores struct {
	ClientStore    store.ClientStore
	DeadlineStore  store.DeadlineStore
	ChecklistStore store.ChecklistStore
	ChangesStore   changes.Store // лента изменений (опц.): блок «Важные изменения»
}

// ClientRow — строка в таблице дашборда.
type ClientRow struct {
	Client       *model.Client
	Stage        string
	DaysInStage  int
	NextDeadline *model.Deadline
	DaysLeft     int    // дней до ближайшего дедлайна (отрицательное = просрочен)
	Progress     int    // % выполнения чек-листа (0-100)
	Urgency      string // "overdue" | "critical" | "warning" | "ok"
	StuckDays    int    // дней без изменений стадии
}

// ConsultantDashboardHandler обрабатывает GET /consultant/dashboard.
func ConsultantDashboardHandler(stores ConsultantDashboardStores) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		rows, err := buildDashboardRows(ctx, stores)
		if err != nil {
			http.Error(w, "ошибка загрузки данных: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Сортировка: сначала самые срочные.
		sort.Slice(rows, func(i, j int) bool {
			return urgencyOrder(rows[i].Urgency) < urgencyOrder(rows[j].Urgency)
		})

		// Важные изменения документации за 30 дней (warning + critical).
		var recentChanges []changes.Event
		if stores.ChangesStore != nil {
			recentChanges, _ = stores.ChangesStore.Recent(ctx, changes.Filter{
				Since:       time.Now().AddDate(0, 0, -30),
				MinSeverity: changes.SeverityWarning,
				Limit:       15,
			})
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderDashboard(rows, recentChanges))
	}
}

// buildDashboardRows собирает данные по всем клиентам.
func buildDashboardRows(ctx context.Context, stores ConsultantDashboardStores) ([]ClientRow, error) {
	clients, err := stores.ClientStore.ListClients(ctx, "", model.ResidencyStage(""))
	if err != nil {
		return nil, fmt.Errorf("список клиентов: %w", err)
	}

	now := time.Now()
	var rows []ClientRow

	for _, c := range clients {
		row := ClientRow{
			Client: c,
			Stage:  stageLabel(c.ResidencyStage),
		}

		// Дней в текущей стадии (по дате обновления).
		row.DaysInStage = int(now.Sub(c.UpdatedAt).Hours() / 24)
		row.StuckDays = row.DaysInStage

		// Ближайший дедлайн.
		if stores.DeadlineStore != nil {
			deadlines, _ := stores.DeadlineStore.ListDeadlines(ctx, c.ID, 90)
			if len(deadlines) > 0 {
				closest := deadlines[0]
				for _, d := range deadlines[1:] {
					if d.DueDate.Before(closest.DueDate) {
						closest = d
					}
				}
				row.NextDeadline = closest
				row.DaysLeft = int(closest.DueDate.Sub(now).Hours() / 24)
			}
		}

		// Прогресс чек-листа.
		if stores.ChecklistStore != nil {
			row.Progress = calcProgress(ctx, c.ID, stores.ChecklistStore)
		}

		// Срочность.
		row.Urgency = calcUrgency(row)
		rows = append(rows, row)
	}

	return rows, nil
}

// calcProgress считает % выполнения чек-листов клиента.
func calcProgress(ctx context.Context, clientID string, cs store.ChecklistStore) int {
	checklists, err := cs.GetClientChecklists(ctx, clientID)
	if err != nil || len(checklists) == 0 {
		return 0
	}
	totalSteps, doneSteps := 0, 0
	for _, ccl := range checklists {
		cl, err := cs.GetChecklist(ctx, ccl.ChecklistID)
		if err != nil {
			continue
		}
		steps, err := cl.ParseSteps()
		if err != nil {
			continue
		}
		totalSteps += len(steps)
		statuses, _ := cs.GetStepStatuses(ctx, ccl.ID)
		for _, s := range statuses {
			if s.Status == model.StepDone {
				doneSteps++
			}
		}
	}
	if totalSteps == 0 {
		return 0
	}
	return doneSteps * 100 / totalSteps
}

// calcUrgency определяет уровень срочности для клиента.
func calcUrgency(row ClientRow) string {
	if row.NextDeadline != nil {
		if row.DaysLeft < 0 {
			return "overdue"
		}
		if row.DaysLeft <= 3 {
			return "critical"
		}
		if row.DaysLeft <= 7 {
			return "warning"
		}
	}
	if row.StuckDays >= 30 {
		return "critical"
	}
	if row.StuckDays >= 14 {
		return "warning"
	}
	return "ok"
}

// urgencyOrder — числовой порядок срочности (меньше = важнее).
func urgencyOrder(u string) int {
	switch u {
	case "overdue":
		return 1
	case "critical":
		return 2
	case "warning":
		return 3
	default:
		return 4
	}
}

// stageLabel возвращает читаемое название стадии.
func stageLabel(s model.ResidencyStage) string {
	switch s {
	case model.StageApplication:
		return "Подача заявки"
	case model.StageExamination:
		return "Экспертиза"
	case model.StageDecision:
		return "Решение"
	case model.StageContract:
		return "Договор"
	case model.StageResident:
		return "Резидент"
	case model.StageReporting:
		return "Отчётность"
	case model.StageExtension:
		return "Продление"
	case model.StageExit:
		return "Выход"
	default:
		return string(s)
	}
}

// RegisterConsultantRoutes регистрирует маршруты консультантского дашборда.
func RegisterConsultantRoutes(mux *http.ServeMux, stores ConsultantDashboardStores) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}
	mux.HandleFunc("/consultant/dashboard", ConsultantDashboardHandler(stores))
	mux.HandleFunc("/consultant/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/consultant/dashboard", http.StatusFound)
	})
	log.Printf("[consultant] дашборд зарегистрирован на /consultant/dashboard")
	return mux
}

// ---------------------------------------------------------------------------
// HTML-рендеринг
// ---------------------------------------------------------------------------

func renderDashboard(rows []ClientRow, recentChanges []changes.Event) string {
	now := time.Now()

	// Счётчики для summary.
	var overdue, critical, warning, okCount int
	for _, r := range rows {
		switch r.Urgency {
		case "overdue":
			overdue++
		case "critical":
			critical++
		case "warning":
			warning++
		default:
			okCount++
		}
	}

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Дашборд консультанта — База Сколково</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #f0f2f5;
  --surface: #ffffff;
  --text: #181b2b;
  --text-muted: #6b7280;
  --border: #e0e3eb;
  --header-bg: #0073ea;
  --header-bg-hover: #005fbf;
  --tr-hover: #f7f8fa;
  --progress-bg: #e0e3eb;
  --primary: #0073ea;
  --danger: #de350b;
  --danger-bg: #ffebe6;
  --critical: #ff5630;
  --critical-bg: #fff0e0;
  --warning: #ff8b00;
  --warning-bg: #fffae6;
  --success: #00875a;
  --success-bg: #e3fcef;
  --shadow: 0 1px 3px rgba(0,0,0,0.08);
  --radius: 8px;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b;
    --surface: #23273a;
    --text: #e8eaed;
    --text-muted: #9ca3af;
    --border: #333850;
    --header-bg: #0073ea;
    --header-bg-hover: #339af0;
    --tr-hover: #2c3044;
    --progress-bg: #333850;
    --primary: #0073ea;
    --danger: #ff6b6b;
    --danger-bg: #3d2020;
    --critical: #ff8a65;
    --critical-bg: #3d2820;
    --warning: #ffa94d;
    --warning-bg: #3d3020;
    --success: #51cf66;
    --success-bg: #1e3a28;
    --shadow: 0 1px 3px rgba(0,0,0,0.3);
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b;
  --surface: #23273a;
  --text: #e8eaed;
  --text-muted: #9ca3af;
  --border: #333850;
  --header-bg: #0073ea;
  --header-bg-hover: #339af0;
  --tr-hover: #2c3044;
  --progress-bg: #333850;
  --primary: #0073ea;
  --danger: #ff6b6b;
  --danger-bg: #3d2020;
  --critical: #ff8a65;
  --critical-bg: #3d2820;
  --warning: #ffa94d;
  --warning-bg: #3d3020;
  --success: #51cf66;
  --success-bg: #1e3a28;
  --shadow: 0 1px 3px rgba(0,0,0,0.3);
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
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
.header {
  background: var(--header-bg);
  color: #fff;
  padding: 14px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.header h1 {
  font-size: 18px;
  font-weight: 700;
  display: flex;
  align-items: center;
  gap: 8px;
}
.header h1 svg { width: 22px; height: 22px; fill: rgba(255,255,255,0.9); }
.header .time { font-size: 13px; opacity: 0.85; }
.header-right { display: flex; align-items: center; gap: 10px; }

.header-link {
  background: rgba(255,255,255,0.15);
  color: #fff;
  border: 1px solid rgba(255,255,255,0.25);
  border-radius: 6px;
  padding: 7px 12px;
  font-size: 13px;
  text-decoration: none;
  font-weight: 500;
  transition: background 0.15s;
}
.header-link:hover { background: rgba(255,255,255,0.25); }

.theme-btn {
  background: rgba(255,255,255,0.15);
  color: #fff;
  border: 1px solid rgba(255,255,255,0.25);
  border-radius: 6px;
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background 0.15s;
}
.theme-btn:hover { background: rgba(255,255,255,0.25); }
.theme-btn svg { width: 18px; height: 18px; fill: currentColor; }

/* Summary */
.summary {
  display: flex;
  gap: 16px;
  padding: 20px 24px;
  flex-wrap: wrap;
}
.summary-card {
  background: var(--surface);
  border-radius: var(--radius);
  padding: 16px 20px;
  min-width: 140px;
  box-shadow: var(--shadow);
  border: 1px solid var(--border);
}
.summary-card .label {
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}
.summary-card .value {
  font-size: 28px;
  font-weight: 700;
  margin-top: 4px;
}
.summary-card.overdue .value { color: var(--danger); }
.summary-card.critical .value { color: var(--critical); }
.summary-card.warning .value { color: var(--warning); }
.summary-card.ok .value { color: var(--success); }

/* Table */
.table-wrap {
  padding: 0 24px 24px;
  overflow-x: auto;
}
table {
  width: 100%;
  border-collapse: collapse;
  background: var(--surface);
  border-radius: var(--radius);
  overflow: hidden;
  box-shadow: var(--shadow);
  border: 1px solid var(--border);
  min-width: 700px;
}
th {
  background: var(--bg);
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  padding: 12px 14px;
  text-align: left;
  border-bottom: 2px solid var(--border);
  white-space: nowrap;
}
td {
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  font-size: 14px;
  vertical-align: middle;
}
tr:last-child td { border-bottom: none; }
tr:hover td { background: var(--tr-hover); }

/* Badge */
.badge {
  display: inline-block;
  padding: 3px 10px;
  border-radius: 12px;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}
.badge-overdue { background: var(--danger-bg); color: var(--danger); }
.badge-critical { background: var(--critical-bg); color: var(--critical); }
.badge-warning { background: var(--warning-bg); color: var(--warning); }
.badge-ok { background: var(--success-bg); color: var(--success); }

/* Progress bar */
.progress-bar {
  width: 100%;
  height: 8px;
  background: var(--progress-bg);
  border-radius: 4px;
  overflow: hidden;
}
.progress-bar .fill {
  height: 100%;
  background: var(--primary);
  border-radius: 4px;
  transition: width 0.3s;
}
.progress-label {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
}

/* Deadline cell */
.deadline-cell { font-size: 13px; }
.deadline-cell .title { font-weight: 500; }
.deadline-cell .date { color: var(--text-muted); font-size: 12px; margin-top: 2px; }
.deadline-cell .days { font-weight: 600; }
.days-overdue { color: var(--danger); }
.days-critical { color: var(--critical); }
.days-warning { color: var(--warning); }
.days-ok { color: var(--text-muted); }

/* Client link */
a.client-link {
  color: var(--primary);
  text-decoration: none;
  font-weight: 500;
  transition: color 0.15s;
}
a.client-link:hover { text-decoration: underline; }
.inn { font-size: 12px; color: var(--text-muted); }

/* Stage badge (letter) */
.stage-letter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: var(--primary);
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  flex-shrink: 0;
}
.stage-cell { display: flex; align-items: center; gap: 8px; }

/* Empty state */
.empty { text-align: center; padding: 40px; color: var(--text-muted); }

/* Responsive */
@media (max-width: 768px) {
  .header { padding: 12px 16px; }
  .header h1 { font-size: 15px; }
  .summary { gap: 10px; padding: 16px; }
  .summary-card { min-width: 110px; padding: 12px 14px; }
  .summary-card .value { font-size: 22px; }
  .table-wrap { padding: 0 16px 16px; }
}
@media (max-width: 480px) {
  .header { flex-direction: column; align-items: flex-start; gap: 8px; }
  .header-right { width: 100%; justify-content: flex-start; }
  .summary { flex-direction: column; }
  .summary-card { min-width: 100%; }
}
@media (min-width: 1024px) {
  .summary { max-width: 1400px; margin-left: auto; margin-right: auto; padding-left: 24px; padding-right: 24px; }
  .table-wrap { max-width: 1400px; margin-left: auto; margin-right: auto; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>

<!-- SVG icons -->
<svg style="display:none" xmlns="http://www.w3.org/2000/svg">
  <symbol id="icon-dashboard" viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></symbol>
  <symbol id="icon-moon" viewBox="0 0 24 24"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></symbol>
  <symbol id="icon-sun" viewBox="0 0 24 24"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3" stroke="currentColor" stroke-width="2"/><line x1="12" y1="21" x2="12" y2="23" stroke="currentColor" stroke-width="2"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64" stroke="currentColor" stroke-width="2"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78" stroke="currentColor" stroke-width="2"/><line x1="1" y1="12" x2="3" y2="12" stroke="currentColor" stroke-width="2"/><line x1="21" y1="12" x2="23" y2="12" stroke="currentColor" stroke-width="2"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36" stroke="currentColor" stroke-width="2"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" stroke="currentColor" stroke-width="2"/></symbol>
</svg>
`)

	sb.WriteString(fmt.Sprintf(`<div class="header">
  <h1>
    <svg><use href="#icon-dashboard"/></svg>
    Дашборд консультанта
  </h1>
  <div class="header-right">
    <div class="time" data-tooltip="Время последнего обновления">%s</div>
    <a href="/" class="header-link" data-tooltip="Вернуться к документам">
      <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:-2px;margin-right:2px"><polyline points="15 18 9 12 15 6"/></svg>
      Документы
    </a>
    <button id="themeBtn" class="theme-btn" onclick="toggleTheme()" data-tooltip="Переключить тему">
      <svg id="themeIcon"><use href="#icon-moon"/></svg>
    </button>
  </div>
</div>
`, now.Format("02.01.2006 15:04")))

	sb.WriteString(fmt.Sprintf(`<div class="summary">
  <div class="summary-card overdue" data-tooltip="Клиенты с просроченными дедлайнами"><div class="label">Просрочено</div><div class="value">%d</div></div>
  <div class="summary-card critical" data-tooltip="Дедлайн менее 3 дней"><div class="label">Критично (≤3 дн.)</div><div class="value">%d</div></div>
  <div class="summary-card warning" data-tooltip="Дедлайн менее 7 дней"><div class="label">Внимание (≤7 дн.)</div><div class="value">%d</div></div>
  <div class="summary-card ok" data-tooltip="Клиенты без срочных задач"><div class="label">В порядке</div><div class="value">%d</div></div>
  <div class="summary-card" data-tooltip="Общее количество клиентов"><div class="label">Всего клиентов</div><div class="value">%d</div></div>
</div>
`, overdue, critical, warning, okCount, len(rows)))

	// Блок «Важные изменения документации».
	if len(recentChanges) > 0 {
		sb.WriteString(`<div class="table-wrap"><div style="background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);box-shadow:var(--shadow);padding:16px 20px">
<div style="font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px;color:var(--text-muted);margin-bottom:12px">⚠️ Важные изменения документации (30 дней)</div>`)
		for _, ev := range recentChanges {
			badgeClass, badgeText := severityBadge(ev.Severity)
			summary := ev.AnalysisSummary
			if summary == "" {
				summary = ev.Summary
			}
			stages := ""
			if len(ev.AffectedStages) > 0 {
				stages = fmt.Sprintf(`<div style="font-size:12px;color:var(--text-muted);margin-top:2px">Стадии: %s</div>`,
					html.EscapeString(strings.Join(ev.AffectedStages, ", ")))
			}
			title := html.EscapeString(ev.Title)
			if ev.SourceURL != "" {
				title = fmt.Sprintf(`<a class="client-link" href="%s" target="_blank" rel="noopener">%s</a>`,
					html.EscapeString(ev.SourceURL), title)
			}
			sb.WriteString(fmt.Sprintf(`<div style="padding:10px 0;border-bottom:1px solid var(--border)">
  <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap"><span class="badge %s">%s</span><span style="font-weight:500">%s</span><span style="font-size:12px;color:var(--text-muted)">%s</span></div>
  <div style="font-size:13px;margin-top:4px">%s</div>%s
</div>`, badgeClass, badgeText, title,
				ev.DetectedAt.Format("02.01.2006"),
				html.EscapeString(summary), stages))
		}
		sb.WriteString(`</div></div>`)
	}

	sb.WriteString(`<div class="table-wrap"><table>
<thead><tr>
  <th data-tooltip="Название компании и ИНН клиента">Клиент / ИНН</th>
  <th data-tooltip="Текущая стадия резидентства">Стадия</th>
  <th data-tooltip="Количество дней в текущей стадии">В стадии</th>
  <th data-tooltip="Процент выполнения чек-листа">Прогресс</th>
  <th data-tooltip="Ближайший срок дедлайна">Ближайший дедлайн</th>
  <th data-tooltip="Статус срочности задачи">Статус</th>
</tr></thead>
<tbody>
`)

	if len(rows) == 0 {
		sb.WriteString(`<tr><td colspan="6" class="empty">Клиенты не найдены</td></tr>`)
	}

	for _, row := range rows {
		sb.WriteString("<tr>")

		// Клиент + ИНН.
		name := html.EscapeString(row.Client.Name)
		inn := html.EscapeString(row.Client.INN)
		sb.WriteString(fmt.Sprintf(`<td><a class="client-link" href="/consultant/client/%s">%s</a>`,
			row.Client.ID, name))
		if inn != "" {
			sb.WriteString(fmt.Sprintf(`<br><span class="inn">ИНН: %s</span>`, inn))
		}
		sb.WriteString("</td>")

		// Стадия с буквенным бейджем.
		initial := stageInitial(row.Stage)
		sb.WriteString(fmt.Sprintf(`<td><div class="stage-cell"><span class="stage-letter">%s</span><span>%s</span></div></td>`,
			initial, html.EscapeString(row.Stage)))

		// Дней в стадии.
		stuckClass := ""
		if row.StuckDays >= 30 {
			stuckClass = `style="color:var(--danger);font-weight:600"`
		} else if row.StuckDays >= 14 {
			stuckClass = `style="color:var(--warning);font-weight:600"`
		}
		sb.WriteString(fmt.Sprintf(`<td %s data-tooltip="Дней без изменения стадии">%d дн.</td>`, stuckClass, row.DaysInStage))

		// Прогресс.
		sb.WriteString(fmt.Sprintf(`<td data-tooltip="Выполнено %d процентов">
  <div class="progress-bar"><div class="fill" style="width:%d%%"></div></div>
  <div class="progress-label">%d%%</div>
</td>`, row.Progress, row.Progress, row.Progress))

		// Ближайший дедлайн.
		if row.NextDeadline != nil {
			daysClass := "days-ok"
			daysText := fmt.Sprintf("через %d дн.", row.DaysLeft)
			if row.DaysLeft < 0 {
				daysClass = "days-overdue"
				daysText = fmt.Sprintf("просрочен на %d дн.", -row.DaysLeft)
			} else if row.DaysLeft <= 3 {
				daysClass = "days-critical"
			} else if row.DaysLeft <= 7 {
				daysClass = "days-warning"
			}
			sb.WriteString(fmt.Sprintf(`<td class="deadline-cell">
  <div class="title">%s</div>
  <div class="date">%s</div>
  <div class="days %s">%s</div>
</td>`, html.EscapeString(row.NextDeadline.Title),
				row.NextDeadline.DueDate.Format("02.01.2006"),
				daysClass, daysText))
		} else {
			sb.WriteString(`<td style="color:var(--text-muted)" data-tooltip="Нет активных дедлайнов">—</td>`)
		}

		// Статус-бейдж.
		badgeClass, badgeText := urgencyBadge(row.Urgency)
		sb.WriteString(fmt.Sprintf(`<td><span class="badge %s">%s</span></td>`, badgeClass, badgeText))

		sb.WriteString("</tr>\n")
	}

	sb.WriteString(`</tbody></table></div><script>
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
</script></body></html>`)
	return sb.String()
}

// stageInitial возвращает первую букву названия стадии для бейджа.
func stageInitial(stage string) string {
	if len(stage) == 0 {
		return "?"
	}
	// Возвращаем первый символ (кириллица поддерживается).
	return strings.ToUpper(string([]rune(stage)[0]))
}

// severityBadge возвращает CSS-класс и подпись для важности изменения.
func severityBadge(s changes.Severity) (class, text string) {
	switch s {
	case changes.SeverityCritical:
		return "badge-critical", "Критично"
	case changes.SeverityWarning:
		return "badge-warning", "Важно"
	default:
		return "badge-ok", "Инфо"
	}
}

func urgencyBadge(u string) (class, text string) {
	switch u {
	case "overdue":
		return "badge-overdue", "Просрочен"
	case "critical":
		return "badge-critical", "Критично"
	case "warning":
		return "badge-warning", "Внимание"
	default:
		return "badge-ok", "В порядке"
	}
}
