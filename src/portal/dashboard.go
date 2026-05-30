package portal

import (
	"baza-skolkovo/src/common/model"
	"html/template"
)

// ---------------------------------------------------------------------------
// Data types for templates
// ---------------------------------------------------------------------------

type loginData struct {
	Next      string
	Flash     string
	FlashKind string
	Link      string
}

type dashboardData struct {
	Client     *model.Client
	Deadlines  []*model.Deadline
	Checklists []*model.ClientChecklist
	Documents  []*model.ClientDocument
	Flash      string
}

type checklistsData struct {
	Client     *model.Client
	Checklists []*model.ClientChecklist
}

type deadlinesData struct {
	Client    *model.Client
	Deadlines []*model.Deadline
	Overdue   []*model.Deadline
}

type documentsData struct {
	Client    *model.Client
	Documents []*model.ClientDocument
}

type generateData struct {
	Client    *model.Client
	Templates []*model.DocumentTemplate
	Flash     string
	FlashKind string
}

// ---------------------------------------------------------------------------
// Portal templates
// ---------------------------------------------------------------------------

var portalTmpl = template.Must(template.New("portal").Funcs(template.FuncMap{
	"stageProgress":       stageProgress,
	"stageLabel":          stageLabel,
	"deadlineStatusClass": deadlineStatusClass,
	"docStatusLabel":      docStatusLabel,
	"docStatusClass":      docStatusClass,
	"humanTime":           humanTime,
	"filename":            filename,
	"sub":                 func(a, b int) int { return a - b },
	"add":                 func(a, b int) int { return a + b },
}).Parse(`
{{/* ===== LAYOUT ===== */}}
{{define "layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Личный кабинет</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f0f2f5; --surface: #fff; --surface-alt: #f8fafc; --primary: #6366f1; --primary-hover: #4f46e5;
  --primary-light: #eef2ff; --text: #1e293b; --text-secondary: #64748b;
  --border: #e2e8f0; --radius: 10px;
  --shadow: 0 1px 3px rgba(0,0,0,.06), 0 1px 2px rgba(0,0,0,.04);
  --shadow-lg: 0 10px 25px -5px rgba(0,0,0,.1), 0 4px 10px -2px rgba(0,0,0,.04);
  --green: #16a34a; --green-bg: #f0fdf4; --green-border: #bbf7d0;
  --yellow: #ca8a04; --yellow-bg: #fefce8; --yellow-border: #fef08a;
  --red: #dc2626; --red-bg: #fef2f2; --red-border: #fecaca;
  --blue: #2563eb; --blue-bg: #eff6ff; --blue-border: #bfdbfe;
  --purple: #7c3aed; --purple-bg: #f5f3ff; --purple-border: #ddd6fe;
  --gray: #6b7280; --gray-bg: #f3f4f6; --gray-border: #e5e7eb;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #0f172a; --surface: #1e293b; --surface-alt: #243357; --primary: #818cf8; --primary-hover: #a5b4fc;
    --primary-light: #1e2254; --text: #e2e8f0; --text-secondary: #94a3b8;
    --border: #334155; --shadow: 0 1px 3px rgba(0,0,0,.4); --shadow-lg: 0 10px 20px rgba(0,0,0,.6);
    --green: #4ade80; --green-bg: #052e16; --green-border: #14532d;
    --yellow: #fbbf24; --yellow-bg: #1c1202; --yellow-border: #78350f;
    --red: #f87171; --red-bg: #1c0707; --red-border: #7f1d1d;
    --blue: #60a5fa; --blue-bg: #0c1c3d; --blue-border: #1e3a5f;
    --purple: #a78bfa; --purple-bg: #200b3d; --purple-border: #4c1d95;
    --gray: #94a3b8; --gray-bg: #334155; --gray-border: #475569;
  }
}
:root[data-theme="dark"] {
  --bg: #0f172a; --surface: #1e293b; --surface-alt: #243357; --primary: #818cf8; --primary-hover: #a5b4fc;
  --primary-light: #1e2254; --text: #e2e8f0; --text-secondary: #94a3b8;
  --border: #334155; --shadow: 0 1px 3px rgba(0,0,0,.4); --shadow-lg: 0 10px 20px rgba(0,0,0,.6);
  --green: #4ade80; --green-bg: #052e16; --green-border: #14532d;
  --yellow: #fbbf24; --yellow-bg: #1c1202; --yellow-border: #78350f;
  --red: #f87171; --red-bg: #1c0707; --red-border: #7f1d1d;
  --blue: #60a5fa; --blue-bg: #0c1c3d; --blue-border: #1e3a5f;
  --purple: #a78bfa; --purple-bg: #200b3d; --purple-border: #4c1d95;
  --gray: #94a3b8; --gray-bg: #334155; --gray-border: #475569;
}
body { font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; }

/* Header */
header { background: linear-gradient(135deg, var(--primary) 0%, #818cf8 100%); color: #fff; padding: 14px 28px; display: flex; align-items: center; justify-content: space-between; box-shadow: 0 2px 8px rgba(99,102,241,.2); position: sticky; top: 0; z-index: 100; }
header h1 { font-size: 17px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
.header-right { display: flex; align-items: center; gap: 16px; }
.header-email { font-size: 13px; opacity: .9; }
.header-link { color: #fff; text-decoration: none; font-size: 13px; font-weight: 500; padding: 6px 14px; border: 1px solid rgba(255,255,255,.3); border-radius: 6px; transition: all .15s; }
.header-link:hover { background: rgba(255,255,255,.15); }

/* Nav */
nav { background: var(--surface); border-bottom: 1px solid var(--border); padding: 0 28px; display: flex; gap: 0; overflow-x: auto; }
nav a { padding: 12px 18px; font-size: 13px; font-weight: 500; color: var(--text-secondary); text-decoration: none; border-bottom: 2px solid transparent; transition: all .15s; white-space: nowrap; }
nav a:hover { color: var(--primary); background: var(--primary-light); }
nav a.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }

/* Main */
main { max-width: 1200px; margin: 0 auto; padding: 24px 28px; }

/* Flash */
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; animation: slideIn .3s ease; }
.flash.ok { background: var(--green-bg); color: #15803d; border: 1px solid var(--green-border); }
.flash.err { background: var(--red-bg); color: #b91c1c; border: 1px solid var(--red-border); }
@keyframes slideIn { from { opacity: 0; transform: translateY(-8px); } to { opacity: 1; transform: translateY(0); } }

/* Cards */
.card { background: var(--surface); border-radius: var(--radius); padding: 20px; box-shadow: var(--shadow); margin-bottom: 16px; }
.card h2 { font-size: 16px; font-weight: 600; margin-bottom: 12px; display: flex; align-items: center; gap: 8px; }
.card h3 { font-size: 14px; font-weight: 600; margin-bottom: 8px; }

/* Progress bar */
.progress { background: var(--gray-bg); border-radius: 20px; height: 10px; overflow: hidden; }
.progress-bar { height: 100%; border-radius: 20px; background: linear-gradient(90deg, var(--primary), #818cf8); transition: width .4s ease; }
.progress-label { font-size: 12px; color: var(--text-secondary); margin-top: 4px; display: flex; justify-content: space-between; }

/* Stage badge */
.stage-badge { display: inline-block; padding: 4px 14px; border-radius: 20px; font-size: 12px; font-weight: 600; background: var(--primary-light); color: var(--primary); }

/* Grid */
.grid { display: grid; gap: 16px; }
.grid-2 { grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); }
.grid-3 { grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); }

/* Stat card */
.stat-card { background: var(--surface); border-radius: var(--radius); padding: 18px; box-shadow: var(--shadow); border-left: 4px solid var(--primary); }
.stat-card .n { font-size: 28px; font-weight: 700; color: var(--primary); }
.stat-card .l { font-size: 12px; color: var(--text-secondary); margin-top: 4px; }

/* Table */
.table-wrap { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow); overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
thead th { background: var(--surface-alt); padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .4px; border-bottom: 2px solid var(--border); }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; }
tbody tr:hover { background: var(--surface-alt); }
tbody tr:last-child td { border-bottom: none; }

/* Badges */
.badge { display: inline-block; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; }
.badge-pending { background: var(--yellow-bg); color: var(--yellow); border: 1px solid var(--yellow-border); }
.badge-submitted { background: var(--blue-bg); color: var(--blue); border: 1px solid var(--blue-border); }
.badge-approved { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.badge-rejected { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.badge-completed { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.badge-overdue { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.badge-upcoming { background: var(--blue-bg); color: var(--blue); border: 1px solid var(--blue-border); }

/* Deadline row colors */
.row-overdue { background: var(--red-bg) !important; }
.row-completed { background: var(--green-bg) !important; }

/* Buttons */
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 18px; border: none; border-radius: 8px; font-size: 13px; font-weight: 500; cursor: pointer; transition: all .15s; font-family: inherit; text-decoration: none; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); box-shadow: 0 4px 12px rgba(99,102,241,.3); }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--gray-bg); }

/* Select */
select { padding: 8px 12px; border: 1px solid var(--border); border-radius: 8px; font-size: 13px; outline: none; font-family: inherit; background: var(--surface); color: var(--text); }
select:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(99,102,241,.1); }

/* Empty state */
.empty { text-align: center; padding: 40px 24px; color: var(--text-secondary); }
.empty .icon { font-size: 40px; margin-bottom: 8px; }
/* Кликабельные карточки */
.card[onclick] { cursor: pointer; }
.card[onclick]:hover { transform: translateY(-2px); box-shadow: var(--shadow-lg); }
/* Прогресс: тонкая анимация */
.progress-bar { transition: width .6s cubic-bezier(.4,0,.2,1); }

/* Responsive */
@media (max-width: 768px) {
  main { padding: 16px; }
  .grid-2, .grid-3 { grid-template-columns: 1fr; }
  nav { padding: 0 12px; overflow-x: auto; }
  nav a { padding: 12px 12px; font-size: 12px; }
  header { padding: 14px 16px; }
  .header-right { gap: 8px; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
{{template "header" .}}
{{template "nav" .}}
<main>
{{template "page" .}}
</main>
<script>
function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  var btn = document.getElementById('themeBtn');
  if (btn) btn.textContent = next === 'dark' ? '☀️' : '🌙';
}
document.addEventListener('DOMContentLoaded', function() {
  var btn = document.getElementById('themeBtn');
  if (!btn) return;
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  btn.textContent = cur === 'dark' ? '☀️' : '🌙';
});
</script>
</body>
</html>{{end}}

{{/* ===== HEADER ===== */}}
{{define "header"}}
<header>
  <h1>🏛 База Сколково</h1>
  <div class="header-right">
    {{if .Client}}<span class="header-email" title="Текущий аккаунт">{{.Client.Email}}</span>{{end}}
    <button id="themeBtn" onclick="toggleTheme()" title="Переключить тему: светлая / тёмная" style="background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.3);border-radius:6px;font-size:16px;padding:6px 10px;cursor:pointer;min-width:36px">🌙</button>
    <a href="/logout" class="header-link" title="Выйти из личного кабинета">Выйти</a>
  </div>
</header>
{{end}}

{{/* ===== NAV ===== */}}
{{define "nav"}}
<nav>
  <a href="/dashboard"{{if .ActiveTabDashboard}} class="active"{{end}} title="Обзор текущей стадии, дедлайнов и прогресса">📊 Дашборд</a>
  <a href="/checklists"{{if .ActiveTabChecklists}} class="active"{{end}} title="Список шагов для прохождения процедур резидентства">📋 Чек-листы</a>
  <a href="/deadlines"{{if .ActiveTabDeadlines}} class="active"{{end}} title="Сроки подачи документов и отчётности">⏰ Дедлайны</a>
  <a href="/documents"{{if .ActiveTabDocuments}} class="active"{{end}} title="Документы, связанные с вашим резидентством">📁 Документы</a>
  <a href="/generate"{{if .ActiveTabGenerate}} class="active"{{end}} title="Генерация документов из шаблонов на основе данных профиля">✨ Генерация</a>
</nav>
{{end}}

{{/* ===== LOGIN PAGE ===== */}}
{{define "login"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Вход — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f0f2f5; --surface: #fff; --primary: #6366f1; --primary-hover: #4f46e5;
  --primary-light: #eef2ff; --text: #1e293b; --text-secondary: #64748b;
  --border: #e2e8f0; --radius: 12px;
  --shadow: 0 1px 3px rgba(0,0,0,.06), 0 1px 2px rgba(0,0,0,.04);
  --shadow-lg: 0 20px 40px -10px rgba(0,0,0,.12);
  --green: #16a34a; --green-bg: #f0fdf4; --green-border: #bbf7d0;
  --red: #dc2626; --red-bg: #fef2f2; --red-border: #fecaca;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --surface: #1e293b; --primary: #818cf8; --primary-hover: #a5b4fc;
    --primary-light: #1e2254; --text: #e2e8f0; --text-secondary: #94a3b8;
    --border: #334155; --shadow-lg: 0 20px 40px -10px rgba(0,0,0,.5);
    --green: #4ade80; --green-bg: #052e16; --green-border: #14532d;
    --red: #f87171; --red-bg: #1c0707; --red-border: #7f1d1d;
  }
}
:root[data-theme="dark"] {
  --surface: #1e293b; --primary: #818cf8; --primary-hover: #a5b4fc;
  --primary-light: #1e2254; --text: #e2e8f0; --text-secondary: #94a3b8;
  --border: #334155; --shadow-lg: 0 20px 40px -10px rgba(0,0,0,.5);
  --green: #4ade80; --green-bg: #052e16; --green-border: #14532d;
  --red: #f87171; --red-bg: #1c0707; --red-border: #7f1d1d;
}
body { font-family: 'Inter', sans-serif; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px; }
.card { background: var(--surface); border-radius: 16px; padding: 36px; box-shadow: var(--shadow-lg); max-width: 420px; width: 100%; }
.logo { text-align: center; margin-bottom: 28px; }
.logo h1 { font-size: 22px; font-weight: 700; color: var(--text); }
.logo p { font-size: 13px; color: var(--text-secondary); margin-top: 4px; }
.form-group { margin-bottom: 16px; }
.form-group label { display: block; font-size: 13px; font-weight: 500; color: var(--text-secondary); margin-bottom: 6px; }
.form-group input { width: 100%; padding: 10px 14px; border: 1px solid var(--border); border-radius: 8px; font-size: 14px; outline: none; transition: all .15s; font-family: inherit; background: var(--surface); color: var(--text); }
.form-group input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(99,102,241,.12); }
.btn { width: 100%; padding: 12px; border: none; border-radius: 8px; font-size: 14px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: inherit; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); box-shadow: 0 4px 12px rgba(99,102,241,.3); }
.flash { padding: 10px 14px; border-radius: 8px; margin-bottom: 16px; font-size: 13px; font-weight: 500; }
.flash.ok { background: var(--green-bg); color: #15803d; border: 1px solid var(--green-border); }
.flash.err { background: var(--red-bg); color: #b91c1c; border: 1px solid var(--red-border); }
.link-box { background: var(--primary-light); border: 1px solid #c7d2fe; border-radius: 8px; padding: 10px 12px; margin-top: 12px; font-size: 12px; word-break: break-all; color: var(--primary); }
.link-box a { color: var(--primary); font-weight: 600; }
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<button id="themeBtn" onclick="toggleTheme()" title="Переключить тему: светлая / тёмная" style="position:fixed;top:16px;right:16px;background:rgba(255,255,255,.2);color:#fff;border:1px solid rgba(255,255,255,.3);border-radius:50%;width:40px;height:40px;font-size:18px;cursor:pointer;z-index:10;display:flex;align-items:center;justify-content:center">🌙</button>
<div class="card">
  <div class="logo">
    <h1>🏛 База Сколково</h1>
    <p>Личный кабинет резидента</p>
  </div>
  {{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
  {{if .Link}}
    <div class="link-box">
      🔗 <a href="{{.Link}}" target="_blank">Перейти по ссылке для входа</a>
    </div>
    <p style="font-size:11px;color:var(--text-secondary);margin-top:8px;text-align:center">В продакшене ссылка будет отправлена на email</p>
  {{else}}
    <form method="POST" action="/login">
      <div class="form-group">
        <label for="email">Электронная почта (Email)</label>
        <input type="email" id="email" name="email" placeholder="name@company.ru" required autocomplete="email" title="Введите email-адрес, на который придёт ссылка для входа">
      </div>
      <button type="submit" class="btn btn-primary" title="Отправить ссылку для входа без пароля на указанный email">Отправить ссылку для входа</button>
    </form>
  {{end}}
</div>
<script>
function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  var btn = document.getElementById('themeBtn');
  if (btn) btn.textContent = next === 'dark' ? '☀️' : '🌙';
}
document.addEventListener('DOMContentLoaded', function() {
  var btn = document.getElementById('themeBtn');
  if (!btn) return;
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  btn.textContent = cur === 'dark' ? '☀️' : '🌙';
});
</script>
</body>
</html>{{end}}

{{/* ===== DASHBOARD PAGE ===== */}}
{{define "dashboard"}}
{{.Flash}}
<div class="grid grid-2">
  <div>
    <div class="card">
      <h2>📊 Текущая стадия</h2>
      <div style="margin-bottom:12px">
        <span class="stage-badge">{{.Client.ResidencyStage}}</span>
      </div>
      <div class="progress">
        <div class="progress-bar" style="width: {{.Client.StageProgress}}%"></div>
      </div>
      <div class="progress-label">
        <span>{{.Client.ResidencyStage}}</span>
        <span>{{.Client.StageProgress}}%</span>
      </div>
    </div>
    <div class="card">
      <h2>📋 Прогресс чек-листов</h2>
      {{if .Checklists}}
        {{range .Checklists}}
          <div style="margin-bottom:12px">
            <div style="display:flex;justify-content:space-between;font-size:13px;margin-bottom:4px">
              <span>{{.Title}}</span>
              <span style="color:var(--text-secondary)">{{.Progress}}%</span>
            </div>
            <div class="progress">
              <div class="progress-bar" style="width:{{.Progress}}%"></div>
            </div>
          </div>
        {{end}}
      {{else}}
        <div class="empty"><div class="icon">📋</div><p>Чек-листы пока не назначены</p></div>
      {{end}}
    </div>
  </div>
  <div>
    <div class="card">
      <h2>⏰ Ближайшие дедлайны</h2>
      {{if .Deadlines}}
        <table>
          <thead><tr><th>Дедлайн</th><th>Дата</th><th>Статус</th></tr></thead>
          <tbody>
          {{range .Deadlines}}
            <tr class="row-{{.StatusClass}}">
              <td>{{.Title}}</td>
              <td>{{.DueDate.Format "02.01.2006"}}</td>
              <td><span class="badge badge-{{.StatusClass}}">{{.StatusLabel}}</span></td>
            </tr>
          {{end}}
          </tbody>
        </table>
      {{else}}
        <div class="empty"><div class="icon">✅</div><p>Нет ближайших дедлайнов</p></div>
      {{end}}
    </div>
    <div class="card">
      <h2>📁 Статус документов</h2>
      {{if .Documents}}
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          {{range .Documents}}
            <span class="badge badge-{{.StatusClass}}">{{.StatusLabel}}</span>
          {{end}}
        </div>
      {{else}}
        <div class="empty"><div class="icon">📁</div><p>Документы пока не назначены</p></div>
      {{end}}
    </div>
  </div>
</div>
{{end}}

{{/* ===== CHECKLISTS PAGE ===== */}}
{{define "checklists"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="card">
  <h2>📋 Мои чек-листы</h2>
  {{if .Checklists}}
    <div class="grid grid-2">
    {{range .Checklists}}
      <div class="card" style="margin:0">
        <h3>{{.Title}}</h3>
        <div style="font-size:12px;color:var(--text-secondary);margin-bottom:8px">
          Тип: {{.ProcedureType}} · Статус: {{.Status}}
        </div>
        <div class="progress">
          <div class="progress-bar" style="width:{{.Progress}}%"></div>
        </div>
        <div class="progress-label">
          <span>{{.CompletedSteps}}/{{.TotalSteps}} шагов</span>
          <span>{{.Progress}}%</span>
        </div>
        {{if .Steps}}
          <div style="margin-top:12px">
            {{range .Steps}}
              <div style="display:flex;align-items:center;gap:8px;padding:6px 0;font-size:12px;border-radius:4px;padding:6px 8px">
                <span style="font-size:14px" title="{{if eq .Status "done"}}Шаг выполнен{{else if eq .Status "in_progress"}}Шаг в процессе выполнения{{else}}Шаг ещё не начат{{end}}">{{if eq .Status "done"}}✅{{else if eq .Status "in_progress"}}🔄{{else}}⬜{{end}}</span>
                <span style="{{if eq .Status "done"}}text-decoration:line-through;color:var(--text-secondary){{else if eq .Status "in_progress"}}font-weight:500{{end}}">{{.Title}}</span>
              </div>
            {{end}}
          </div>
        {{end}}
      </div>
    {{end}}
    </div>
  {{else}}
    <div class="empty"><div class="icon">📋</div><p>Чек-листы пока не назначены</p></div>
  {{end}}
</div>
{{end}}

{{/* ===== DEADLINES PAGE ===== */}}
{{define "deadlines"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="grid grid-2">
  <div class="card">
    <h2>⏰ Ближайшие дедлайны</h2>
    {{if .Deadlines}}
      <div class="table-wrap" style="box-shadow:none">
        <table>
          <thead><tr><th>Название</th><th>Дата</th><th>Тип</th><th>Статус</th></tr></thead>
          <tbody>
          {{range .Deadlines}}
            <tr class="row-{{.StatusClass}}">
              <td style="font-weight:500">{{.Title}}</td>
              <td>{{.DueDate.Format "02.01.2006"}}</td>
              <td><span style="font-size:11px;color:var(--text-secondary)">{{.Type}}</span></td>
              <td><span class="badge badge-{{.StatusClass}}">{{.StatusLabel}}</span></td>
            </tr>
          {{end}}
          </tbody>
        </table>
      </div>
    {{else}}
      <div class="empty"><div class="icon">✅</div><p>Нет ближайших дедлайнов</p></div>
    {{end}}
  </div>
  <div class="card">
    <h2>🔴 Просроченные</h2>
    {{if .Overdue}}
      <div class="table-wrap" style="box-shadow:none">
        <table>
          <thead><tr><th>Название</th><th>Дата</th><th>Статус</th></tr></thead>
          <tbody>
          {{range .Overdue}}
            <tr class="row-overdue">
              <td style="font-weight:500">{{.Title}}</td>
              <td style="color:var(--red);font-weight:600">{{.DueDate.Format "02.01.2006"}}</td>
              <td><span class="badge badge-overdue">Просрочен</span></td>
            </tr>
          {{end}}
          </tbody>
        </table>
      </div>
    {{else}}
      <div class="empty"><div class="icon">✅</div><p>Нет просроченных дедлайнов</p></div>
    {{end}}
  </div>
</div>
{{end}}

{{/* ===== DOCUMENTS PAGE ===== */}}
{{define "documents"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="card">
  <h2>📁 Мои документы</h2>
  {{if .Documents}}
    <div class="table-wrap" style="box-shadow:none">
      <table>
        <thead><tr><th>Документ</th><th title="Роль клиента в работе с документом: владелец, подписант, ознакомлен и т.д.">Роль</th><th title="Текущий статус документа в процессе согласования">Статус</th><th title="Дата последнего изменения документа">Дата</th></tr></thead>
        <tbody>
        {{range .Documents}}
          <tr>
            <td style="font-weight:500">{{.Name}}</td>
            <td><span style="font-size:12px;color:var(--text-secondary)">{{.Role}}</span></td>
            <td><span class="badge badge-{{.StatusClass}}">{{.StatusLabel}}</span></td>
            <td style="font-size:12px;color:var(--text-secondary)">{{.Date}}</td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
  {{else}}
    <div class="empty"><div class="icon">📁</div><p>Документы пока не назначены</p></div>
  {{end}}
</div>
{{end}}

{{/* ===== GENERATE PAGE ===== */}}
{{define "generate"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="card">
  <h2>✨ Генерация документа из шаблона</h2>
  <p style="font-size:13px;color:var(--text-secondary);margin-bottom:20px">Выберите шаблон — документ будет создан автоматически на основе данных вашего профиля и сохранён в раздел «Документы».</p>
  <form method="POST" action="/generate" id="generateForm">
    <div style="display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap">
      <div style="flex:1;min-width:240px">
        <label for="template" style="display:block;font-size:13px;font-weight:600;color:var(--text-secondary);margin-bottom:6px;text-transform:uppercase;letter-spacing:.4px">Шаблон документа</label>
        <select id="template" name="template_id" style="width:100%" title="Выберите тип документа для генерации из списка доступных шаблонов" required>
          <option value="">— Выберите шаблон —</option>
          {{range .Templates}}
            <option value="{{.ID}}">{{.Name}} ({{.Type}})</option>
          {{end}}
        </select>
      </div>
      <button type="submit" class="btn btn-primary" title="Создать документ по выбранному шаблону" style="white-space:nowrap">✨ Сгенерировать</button>
    </div>
  </form>
</div>
{{if .Templates}}
<div class="card">
  <h3 style="font-size:14px;font-weight:600;margin-bottom:12px">📋 Доступные шаблоны</h3>
  <div class="grid grid-3">
  {{range .Templates}}
    <div class="card" style="margin:0;padding:16px;border-left:3px solid var(--primary);cursor:pointer;transition:transform .15s,box-shadow .15s"
         onclick="document.getElementById('template').value='{{.ID}}';document.getElementById('generateForm').requestSubmit()"
         title="Нажмите, чтобы сгенерировать документ по этому шаблону">
      <div style="font-size:13px;font-weight:600;color:var(--text)">{{.Name}}</div>
      <div style="font-size:11px;color:var(--text-secondary);margin-top:6px;display:flex;gap:10px;flex-wrap:wrap">
        <span title="Тип документа">📄 {{.Type}}</span>
        <span title="Версия шаблона">v{{.Version}}</span>
      </div>
    </div>
  {{end}}
  </div>
</div>
{{else}}
<div class="empty" style="margin-top:16px">
  <div class="icon">📋</div>
  <p><strong>Шаблоны пока не добавлены</strong></p>
  <p>Шаблоны документов создаются администратором в разделе резидентства</p>
</div>
{{end}}
{{end}}

`))
