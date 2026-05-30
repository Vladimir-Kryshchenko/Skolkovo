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

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// ConsultantDashboardStores — хранилища для дашборда.
type ConsultantDashboardStores struct {
	ClientStore    store.ClientStore
	DeadlineStore  store.DeadlineStore
	ChecklistStore store.ChecklistStore
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

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderDashboard(rows))
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

func renderDashboard(rows []ClientRow) string {
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
<style>
:root{--bg:#f4f5f7;--surface:#fff;--text:#172b4d;--text-muted:#6b778c;--border:#e0e0e0;--header-bg:#0052cc;--tr-hover:#fafbfc;--progress-bg:#e0e0e0}
@media(prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#0f172a;--surface:#1e293b;--text:#e2e8f0;--text-muted:#94a3b8;--border:#334155;--header-bg:#1e40af;--tr-hover:#243357;--progress-bg:#334155}}
:root[data-theme=dark]{--bg:#0f172a;--surface:#1e293b;--text:#e2e8f0;--text-muted:#94a3b8;--border:#334155;--header-bg:#1e40af;--tr-hover:#243357;--progress-bg:#334155}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:var(--bg);color:var(--text)}
.header{background:var(--header-bg);color:#fff;padding:16px 24px;display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap}
.header h1{font-size:20px;font-weight:600}
.header .time{font-size:13px;opacity:0.8}
.header-right{display:flex;align-items:center;gap:10px}
.theme-btn{background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.3);border-radius:6px;padding:6px 10px;font-size:16px;cursor:pointer;min-width:36px}
.summary{display:flex;gap:16px;padding:20px 24px;flex-wrap:wrap}
.summary-card{background:var(--surface);border-radius:8px;padding:16px 20px;min-width:140px;box-shadow:0 1px 3px rgba(0,0,0,.1)}
.summary-card .label{font-size:12px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px}
.summary-card .value{font-size:28px;font-weight:700;margin-top:4px}
.summary-card.overdue .value{color:#de350b}
.summary-card.critical .value{color:#ff5630}
.summary-card.warning .value{color:#ff8b00}
.summary-card.ok .value{color:#00875a}
.table-wrap{padding:0 24px 24px}
table{width:100%;border-collapse:collapse;background:var(--surface);border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,.1)}
th{background:var(--bg);font-size:12px;font-weight:600;text-transform:uppercase;letter-spacing:.5px;color:var(--text-muted);padding:10px 14px;text-align:left;border-bottom:2px solid var(--border)}
td{padding:12px 14px;border-bottom:1px solid var(--border);font-size:14px;vertical-align:middle}
tr:last-child td{border-bottom:none}
tr:hover td{background:var(--tr-hover)}
.badge{display:inline-block;padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600;white-space:nowrap}
.badge-overdue{background:#ffebe6;color:#de350b}
.badge-critical{background:#fff0e0;color:#ff5630}
.badge-warning{background:#fffae6;color:#ff8b00}
.badge-ok{background:#e3fcef;color:#00875a}
.progress-bar{width:100%;height:8px;background:var(--progress-bg);border-radius:4px;overflow:hidden}
.progress-bar .fill{height:100%;background:#0052cc;border-radius:4px;transition:width .3s}
.progress-label{font-size:11px;color:var(--text-muted);margin-top:2px}
.deadline-cell{font-size:13px}
.deadline-cell .title{font-weight:500}
.deadline-cell .date{color:var(--text-muted);font-size:12px;margin-top:2px}
.deadline-cell .days{font-weight:600}
.days-overdue{color:#de350b}
.days-critical{color:#ff5630}
.days-warning{color:#ff8b00}
.days-ok{color:var(--text-muted)}
a.client-link{color:#0052cc;text-decoration:none;font-weight:500}
a.client-link:hover{text-decoration:underline}
.inn{font-size:12px;color:var(--text-muted)}
.empty{text-align:center;padding:40px;color:var(--text-muted)}
@media(max-width:768px){.summary{gap:10px;padding:16px}.table-wrap{padding:0 16px 16px}table{font-size:12px}th,td{padding:8px 10px}}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
`)

	sb.WriteString(fmt.Sprintf(`<div class="header">
  <h1>📋 Дашборд консультанта</h1>
  <div class="header-right">
    <div class="time" title="Время последнего обновления">%s</div>
    <a href="/" style="background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.3);border-radius:6px;padding:7px 12px;font-size:13px;text-decoration:none" title="Вернуться к документам">← Документы</a>
    <button id="themeBtn" class="theme-btn" onclick="toggleTheme()" title="Переключить тему: светлая / тёмная">🌙</button>
  </div>
</div>
`, now.Format("02.01.2006 15:04")))

	sb.WriteString(fmt.Sprintf(`<div class="summary">
  <div class="summary-card overdue"><div class="label">Просрочено</div><div class="value">%d</div></div>
  <div class="summary-card critical"><div class="label">Критично (≤3 дн.)</div><div class="value">%d</div></div>
  <div class="summary-card warning"><div class="label">Внимание (≤7 дн.)</div><div class="value">%d</div></div>
  <div class="summary-card ok"><div class="label">В порядке</div><div class="value">%d</div></div>
  <div class="summary-card"><div class="label">Всего клиентов</div><div class="value">%d</div></div>
</div>
`, overdue, critical, warning, okCount, len(rows)))

	sb.WriteString(`<div class="table-wrap"><table>
<thead><tr>
  <th>Клиент / ИНН</th>
  <th>Стадия</th>
  <th>В стадии</th>
  <th>Прогресс</th>
  <th>Ближайший дедлайн</th>
  <th>Статус</th>
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

		// Стадия.
		sb.WriteString(fmt.Sprintf("<td>%s</td>", html.EscapeString(row.Stage)))

		// Дней в стадии.
		stuckClass := ""
		if row.StuckDays >= 30 {
			stuckClass = `style="color:#de350b;font-weight:600"`
		} else if row.StuckDays >= 14 {
			stuckClass = `style="color:#ff8b00;font-weight:600"`
		}
		sb.WriteString(fmt.Sprintf(`<td %s>%d дн.</td>`, stuckClass, row.DaysInStage))

		// Прогресс.
		sb.WriteString(fmt.Sprintf(`<td>
  <div class="progress-bar"><div class="fill" style="width:%d%%"></div></div>
  <div class="progress-label">%d%%</div>
</td>`, row.Progress, row.Progress))

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
			sb.WriteString(`<td style="color:#6b778c">—</td>`)
		}

		// Статус-бейдж.
		badgeClass, badgeText := urgencyBadge(row.Urgency)
		sb.WriteString(fmt.Sprintf(`<td><span class="badge %s">%s</span></td>`, badgeClass, badgeText))

		sb.WriteString("</tr>\n")
	}

	sb.WriteString(`</tbody></table></div><script>
function toggleTheme(){var r=document.documentElement;var cur=r.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');var next=cur==='dark'?'light':'dark';r.setAttribute('data-theme',next);localStorage.setItem('theme',next);var btn=document.getElementById('themeBtn');if(btn)btn.textContent=next==='dark'?'☀️':'🌙';}
document.addEventListener('DOMContentLoaded',function(){var btn=document.getElementById('themeBtn');if(!btn)return;var cur=document.documentElement.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');btn.textContent=cur==='dark'?'☀️':'🌙';});
</script></body></html>`)
	return sb.String()
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
