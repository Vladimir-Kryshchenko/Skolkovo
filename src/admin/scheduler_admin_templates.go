package admin

// scheduler_admin_templates.go — HTML-шаблоны раздела «Расписание и журнал».
// Один компактный layout (с CSS-переменными темы и сайдбаром) + по одному
// content-блоку на страницу. Стиль и палитра согласованы с остальными админ-страницами.

import "html/template"

const jobsLayout = `
{{define "layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Расписание и журнал</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f5f6f8;--surface:#fff;--surface-alt:#f0f4f8;--primary:#0073ea;--primary-light:#e6f0fa;
--text:#1a1d2e;--text-secondary:#5f6577;--border:#d5d9e2;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.06);
--green:#00875a;--green-bg:#e3fcef;--yellow:#ff991f;--yellow-bg:#fff7e6;--red:#de350b;--red-bg:#ffeae6;
--gray:#6b778c;--gray-bg:#f4f5f7;--blue:#0073ea;--blue-bg:#e6f0fa}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#181b2b;--surface:#23273a;--surface-alt:#2a2f45;
--primary:#4c9aff;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;
--shadow:0 1px 3px rgba(0,0,0,.3);--green:#36b37e;--green-bg:#1b3a2a;--yellow:#ff991f;--yellow-bg:#3a2a0a;
--red:#ff5630;--red-bg:#3a1510;--gray:#a0a5b8;--gray-bg:#2a2f45;--blue:#4c9aff;--blue-bg:#1e3a5f}}
:root[data-theme="dark"]{--bg:#181b2b;--surface:#23273a;--surface-alt:#2a2f45;--primary:#4c9aff;--primary-light:#1e3a5f;
--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--shadow:0 1px 3px rgba(0,0,0,.3);
--green:#36b37e;--green-bg:#1b3a2a;--yellow:#ff991f;--yellow-bg:#3a2a0a;--red:#ff5630;--red-bg:#3a1510;
--gray:#a0a5b8;--gray-bg:#2a2f45;--blue:#4c9aff;--blue-bg:#1e3a5f}
body{font-family:'Figtree',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.5;font-size:14px}
main{max-width:1400px;margin:0 auto;padding:24px 28px}
@media(max-width:768px){main{padding:16px}}
h1{font-size:20px;font-weight:700;margin-bottom:4px}
.lead{color:var(--text-secondary);font-size:13px;margin-bottom:18px}
.tabs{display:flex;gap:4px;margin-bottom:20px;background:var(--surface);border-radius:var(--radius);padding:6px;box-shadow:var(--shadow);width:fit-content;border:1px solid var(--border);flex-wrap:wrap}
.tab{padding:8px 18px;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;text-decoration:none;color:var(--text-secondary);transition:all .15s}
.tab:hover{background:var(--primary-light);color:var(--primary)}
.tab.active{background:var(--primary);color:#fff}
.flash{padding:11px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500}
.flash-ok{background:var(--green-bg);color:var(--green)}
.flash-err{background:var(--red-bg);color:var(--red)}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);border:1px solid var(--border);overflow:hidden;margin-bottom:18px}
.cardhead{padding:14px 18px;border-bottom:1px solid var(--border);font-size:14px;font-weight:600;display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap}
.cardbody{padding:18px}
.muted{color:var(--text-secondary)}
table{width:100%;border-collapse:collapse}
thead th{background:var(--surface-alt);padding:10px 14px;text-align:left;font-size:11px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;border-bottom:2px solid var(--border)}
tbody td{padding:11px 14px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:middle}
tbody tr:hover{background:var(--surface-alt)}
tbody tr:last-child td{border-bottom:none}
.sub{font-size:11px;color:var(--text-secondary);margin-top:2px;font-family:ui-monospace,Menlo,Consolas,monospace}
.num{width:64px;padding:6px 8px;border:1px solid var(--border);border-radius:6px;background:var(--surface);color:var(--text);font-size:13px;font-family:inherit}
.sel{padding:6px 8px;border:1px solid var(--border);border-radius:6px;background:var(--surface);color:var(--text);font-size:13px;font-family:inherit}
.btn{padding:6px 12px;border-radius:6px;border:1px solid var(--primary);background:var(--primary);color:#fff;font-size:12.5px;font-weight:600;cursor:pointer;font-family:inherit;text-decoration:none;display:inline-block}
.btn:hover{opacity:.9}
.btn.ghost{background:transparent;color:var(--primary)}
.btn.ghost:hover{background:var(--primary-light)}
.badge{display:inline-block;padding:3px 9px;border-radius:20px;font-size:11px;font-weight:600;white-space:nowrap}
.badge.ok{background:var(--green-bg);color:var(--green)}
.badge.err{background:var(--red-bg);color:var(--red)}
.badge.warn{background:var(--yellow-bg);color:var(--yellow)}
.badge.run{background:var(--blue-bg);color:var(--blue)}
.badge.muted{background:var(--gray-bg);color:var(--gray)}
.err-txt{font-size:11px;color:var(--red);margin-top:3px;max-width:240px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.act{white-space:nowrap;display:flex;gap:6px;align-items:center}
.filters{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
.rt{text-align:right;font-variant-numeric:tabular-nums}
.mono{font-family:ui-monospace,Menlo,Consolas,monospace;font-size:12px}
.kpis{display:flex;gap:14px;flex-wrap:wrap;margin-bottom:8px}
.kpi{background:var(--surface-alt);border-radius:var(--radius);padding:12px 16px;min-width:130px}
.kpi b{display:block;font-size:20px;font-weight:700}
.kpi span{font-size:11px;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.4px}
pre{background:var(--surface-alt);border-radius:6px;padding:12px;overflow:auto;font-size:12px;font-family:ui-monospace,Menlo,Consolas,monospace}
a.link{color:var(--primary);text-decoration:none}
a.link:hover{text-decoration:underline}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
{{template "sidebar" .}}
<main>
<h1>Расписание и журнал</h1>
<div class="lead">Периодичность проверки изменений по каждому источнику, журнал запусков и учёт токенов ИИ-агентов.</div>
<nav class="tabs">
<a href="/jobs" class="tab {{if eq .Active "jobs"}}active{{end}}">Расписание</a>
<a href="/jobs/runs" class="tab {{if eq .Active "runs"}}active{{end}}">Журнал запусков</a>
<a href="/jobs/ai-usage" class="tab {{if eq .Active "ai-usage"}}active{{end}}">Токены ИИ</a>
</nav>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}
{{template "content" .}}
</main>
</body>
</html>{{end}}
`

const jobsContent = `
{{define "content"}}
{{if not .Available}}
<div class="card"><div class="cardbody muted">Планировщик доступен только на Postgres-бэкенде. Запустите сервис с backend=postgres.</div></div>
{{else}}
<div class="card">
<div class="cardhead">Источники и периодичность{{if not .CanRun}}<span class="muted" style="font-weight:400;font-size:12px">Ручной запуск доступен в режиме serve</span>{{end}}</div>
<table>
<thead><tr><th>Источник</th><th>Периодичность</th><th>Вкл.</th><th>Последний запуск</th><th>Следующий</th><th>Статус</th><th></th></tr></thead>
<tbody>
{{range .Jobs}}
<tr>
<td><b>{{.Title}}</b><div class="sub">{{.Name}}</div></td>
<td>
<form method="post" action="/jobs/{{.Name}}" id="f-{{.Name}}"></form>
<input type="number" form="f-{{.Name}}" name="interval_value" min="1" value="{{.IntervalValue}}" class="num">
<select form="f-{{.Name}}" name="interval_unit" class="sel">
<option value="minutes" {{if eq .IntervalUnit "minutes"}}selected{{end}}>минут</option>
<option value="hours" {{if eq .IntervalUnit "hours"}}selected{{end}}>часов</option>
<option value="days" {{if eq .IntervalUnit "days"}}selected{{end}}>дней</option>
</select>
</td>
<td><input type="checkbox" form="f-{{.Name}}" name="enabled" {{if .Enabled}}checked{{end}}></td>
<td>{{fmtTime .LastRunAt}}</td>
<td>{{fmtTime .NextRunAt}}</td>
<td><span class="badge {{statusClass .LastStatus}}">{{statusLabel .LastStatus}}</span>{{if .LastError}}<div class="err-txt" title="{{.LastError}}">{{.LastError}}</div>{{end}}</td>
<td class="act">
<button type="submit" form="f-{{.Name}}" class="btn">Сохранить</button>
{{if $.CanRun}}<form method="post" action="/jobs/{{.Name}}/run" style="display:inline"><button class="btn ghost">▶ Сейчас</button></form>{{end}}
</td>
</tr>
{{end}}
</tbody>
</table>
</div>
{{end}}
{{end}}
`

const runsContent = `
{{define "content"}}
{{if not .Available}}
<div class="card"><div class="cardbody muted">Журнал доступен только на Postgres-бэкенде.</div></div>
{{else}}
<div class="card">
<div class="cardhead">
Журнал запусков
<form method="get" action="/jobs/runs" class="filters">
<select name="job" class="sel" onchange="this.form.submit()">
<option value="">Все задания</option>
{{range .JobNames}}<option value="{{.}}" {{if eq . $.JobFilter}}selected{{end}}>{{.}}</option>{{end}}
</select>
<select name="status" class="sel" onchange="this.form.submit()">
<option value="">Любой статус</option>
<option value="success" {{if eq .StatusFilt "success"}}selected{{end}}>Успех</option>
<option value="error" {{if eq .StatusFilt "error"}}selected{{end}}>Ошибка</option>
<option value="skipped" {{if eq .StatusFilt "skipped"}}selected{{end}}>Пропущено</option>
</select>
</form>
</div>
<table>
<thead><tr><th>Старт</th><th>Задание</th><th>Триггер</th><th>Длительность</th><th>Статус</th><th class="rt">Новых</th><th class="rt">Обнов.</th><th class="rt">Всего</th><th>Ошибка</th><th></th></tr></thead>
<tbody>
{{range .Runs}}
<tr>
<td>{{fmtTimeV .StartedAt}}</td>
<td>{{.JobName}}</td>
<td class="muted">{{triggerLabel .Trigger}}</td>
<td>{{fmtDurMs .DurationMs}}</td>
<td><span class="badge {{statusClass .Status}}">{{statusLabel .Status}}</span></td>
<td class="rt">{{.ItemsNew}}</td>
<td class="rt">{{.ItemsUpdated}}</td>
<td class="rt">{{.ItemsTotal}}</td>
<td>{{if .Error}}<span class="err-txt" title="{{.Error}}">{{.Error}}</span>{{else}}—{{end}}</td>
<td><a class="link" href="/jobs/runs/{{.ID}}">детали →</a></td>
</tr>
{{else}}
<tr><td colspan="10" class="muted" style="text-align:center;padding:24px">Запусков пока нет.</td></tr>
{{end}}
</tbody>
</table>
</div>
{{end}}
{{end}}
`

const runDetailContent = `
{{define "content"}}
<div class="card">
<div class="cardhead">Прогон «{{.Run.JobName}}» <a class="link" href="/jobs/runs">← к журналу</a></div>
<div class="cardbody">
<div class="kpis">
<div class="kpi"><b><span class="badge {{statusClass .Run.Status}}">{{statusLabel .Run.Status}}</span></b><span>Статус</span></div>
<div class="kpi"><b>{{fmtDurMs .Run.DurationMs}}</b><span>Длительность</span></div>
<div class="kpi"><b>{{.Run.ItemsNew}}</b><span>Новых</span></div>
<div class="kpi"><b>{{.Run.ItemsUpdated}}</b><span>Обновлено</span></div>
<div class="kpi"><b>{{.Run.ItemsTotal}}</b><span>Всего</span></div>
</div>
<p class="muted" style="margin:6px 0">Триггер: {{triggerLabel .Run.Trigger}} · старт {{fmtTimeV .Run.StartedAt}} · финиш {{fmtTime .Run.FinishedAt}} · id <span class="mono">{{.Run.ID}}</span></p>
{{if .Run.Error}}<div class="flash flash-err">{{.Run.Error}}</div>{{end}}
{{if .Run.Details}}<h3 style="font-size:13px;margin:12px 0 6px">Детали</h3><pre>{{range $k,$v := .Run.Details}}{{$k}}: {{$v}}
{{end}}</pre>{{end}}
</div>
</div>
<div class="card">
<div class="cardhead">Вызовы ИИ в этом прогоне ({{len .Usage}})</div>
<table>
<thead><tr><th>Время</th><th>Агент</th><th>Модель</th><th>Провайдер</th><th class="rt">Prompt</th><th class="rt">Completion</th><th class="rt">Всего</th><th>Длит.</th><th>Статус</th></tr></thead>
<tbody>
{{range .Usage}}
<tr>
<td>{{fmtTimeV .CreatedAt}}</td>
<td>{{agentLabel .AgentType}}</td>
<td class="mono">{{orDash .ModelID}}</td>
<td>{{providerLabel .Provider}}</td>
<td class="rt">{{.PromptTokens}}</td>
<td class="rt">{{.CompletionTokens}}</td>
<td class="rt"><b>{{.TotalTokens}}</b></td>
<td>{{fmtDurMs .DurationMs}}</td>
<td>{{if .Success}}<span class="badge ok">OK</span>{{else}}<span class="badge err" title="{{.Error}}">Ошибка</span>{{end}}</td>
</tr>
{{else}}
<tr><td colspan="9" class="muted" style="text-align:center;padding:20px">ИИ-вызовов в этом прогоне не было.</td></tr>
{{end}}
</tbody>
</table>
</div>
{{end}}
`

const aiUsageContent = `
{{define "content"}}
{{if not .Available}}
<div class="card"><div class="cardbody muted">Учёт токенов доступен только на Postgres-бэкенде.</div></div>
{{else}}
<div class="card">
<div class="cardhead">Расход токенов по моделям<span class="muted" style="font-weight:400;font-size:12px">за последние {{.SinceDays}} дн.</span></div>
<table>
<thead><tr><th>Модель</th><th class="rt">Вызовов</th><th class="rt">Prompt</th><th class="rt">Completion</th><th class="rt">Всего токенов</th><th class="rt">Ошибок</th><th class="rt">Ср. длит.</th></tr></thead>
<tbody>
{{range .ByModel}}
<tr><td class="mono">{{orDash .Key}}</td><td class="rt">{{.Calls}}</td><td class="rt">{{.PromptTokens}}</td><td class="rt">{{.CompletionTokens}}</td><td class="rt"><b>{{.TotalTokens}}</b></td><td class="rt">{{.Errors}}</td><td class="rt">{{fmtDurMs .AvgDurationMs}}</td></tr>
{{else}}
<tr><td colspan="7" class="muted" style="text-align:center;padding:20px">Данных пока нет.</td></tr>
{{end}}
</tbody>
</table>
</div>
<div class="card">
<div class="cardhead">Расход токенов по агентам</div>
<table>
<thead><tr><th>Агент</th><th class="rt">Вызовов</th><th class="rt">Prompt</th><th class="rt">Completion</th><th class="rt">Всего токенов</th><th class="rt">Ошибок</th><th class="rt">Ср. длит.</th></tr></thead>
<tbody>
{{range .ByAgent}}
<tr><td>{{agentLabel .Key}}</td><td class="rt">{{.Calls}}</td><td class="rt">{{.PromptTokens}}</td><td class="rt">{{.CompletionTokens}}</td><td class="rt"><b>{{.TotalTokens}}</b></td><td class="rt">{{.Errors}}</td><td class="rt">{{fmtDurMs .AvgDurationMs}}</td></tr>
{{else}}
<tr><td colspan="7" class="muted" style="text-align:center;padding:20px">Данных пока нет.</td></tr>
{{end}}
</tbody>
</table>
</div>
<div class="card">
<div class="cardhead">Последние вызовы ИИ</div>
<table>
<thead><tr><th>Время</th><th>Агент</th><th>Модель</th><th>Провайдер</th><th class="rt">Prompt</th><th class="rt">Completion</th><th class="rt">Всего</th><th>Длит.</th><th>Статус</th></tr></thead>
<tbody>
{{range .Recent}}
<tr>
<td>{{fmtTimeV .CreatedAt}}</td>
<td>{{agentLabel .AgentType}}</td>
<td class="mono">{{orDash .ModelID}}</td>
<td>{{providerLabel .Provider}}</td>
<td class="rt">{{.PromptTokens}}</td>
<td class="rt">{{.CompletionTokens}}</td>
<td class="rt"><b>{{.TotalTokens}}</b></td>
<td>{{fmtDurMs .DurationMs}}</td>
<td>{{if .Success}}<span class="badge ok">OK</span>{{else}}<span class="badge err" title="{{.Error}}">Ошибка</span>{{end}}</td>
</tr>
{{else}}
<tr><td colspan="9" class="muted" style="text-align:center;padding:20px">Вызовов пока нет.</td></tr>
{{end}}
</tbody>
</table>
</div>
{{end}}
{{end}}
`

// Каждая страница = layout + свой content + общий сайдбар, скомпилированные отдельно.
var jobsPageTmpl = template.Must(template.New("jobs").Funcs(jobsFuncs).Parse(jobsLayout + jobsContent + sidebarMainDefine))
var runsPageTmpl = template.Must(template.New("runs").Funcs(jobsFuncs).Parse(jobsLayout + runsContent + sidebarMainDefine))
var runDetailTmpl = template.Must(template.New("run-detail").Funcs(jobsFuncs).Parse(jobsLayout + runDetailContent + sidebarMainDefine))
var aiUsageTmpl = template.Must(template.New("ai-usage").Funcs(jobsFuncs).Parse(jobsLayout + aiUsageContent + sidebarMainDefine))
