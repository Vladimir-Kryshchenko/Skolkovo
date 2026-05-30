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
	// Последние изменения базы знаний
	RecentChanges []recentChange
}

// recentChange — упрощённое представление изменения для портала.
type recentChange struct {
	Title      string
	Kind       string
	EntityType string
	DetectedAt string
	Summary    string
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
	Documents []*portalDocInfo
}

// portalDocInfo — расширенная информация о документе для портала (с источником).
type portalDocInfo struct {
	ID          string
	Name        string
	Role        string
	Status      string
	StatusClass string
	StatusLabel string
	Date        string
	SourceURL   string
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
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700;800&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f5f6f8; --surface: #ffffff; --surface-alt: #f9fafb; --surface-hover: #f0f1f3;
  --primary: #0073ea; --primary-hover: #005bb5; --primary-light: #e8f2fc; --primary-text: #0073ea;
  --text: #1a1d29; --text-secondary: #676f83; --text-muted: #9498a8;
  --border: #e0e2e8; --border-strong: #c8cbd4;
  --radius: 8px;
  --shadow: 0 1px 3px rgba(0,0,0,.06), 0 1px 2px rgba(0,0,0,.04);
  --shadow-lg: 0 4px 12px rgba(0,0,0,.08);
  --green: #00875a; --green-bg: #e6f7f0; --green-border: #b3e0ce; --green-text: #00875a;
  --yellow: #bf6900; --yellow-bg: #fff6e5; --yellow-border: #f0d6a8; --yellow-text: #bf6900;
  --red: #de350b; --red-bg: #fde8e0; --red-border: #f5b8a0; --red-text: #de350b;
  --blue: #0073ea; --blue-bg: #e8f2fc; --blue-border: #b3d4f5; --blue-text: #0073ea;
  --purple: #6b3fa0; --purple-bg: #f0e8f7; --purple-border: #d4b8e8; --purple-text: #6b3fa0;
  --gray: #676f83; --gray-bg: #f0f1f3; --gray-border: #d8dbe4; --gray-text: #676f83;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2e42; --surface-hover: #2e3348;
    --primary: #4da3ff; --primary-hover: #73b8ff; --primary-light: #1e3a5f; --primary-text: #4da3ff;
    --text: #e4e6ed; --text-secondary: #9ca0b0; --text-muted: #6b7084;
    --border: #363b50; --border-strong: #4a5068;
    --shadow: 0 1px 3px rgba(0,0,0,.3); --shadow-lg: 0 4px 12px rgba(0,0,0,.4);
    --green: #3ddc84; --green-bg: #1a3328; --green-border: #2a5a44; --green-text: #3ddc84;
    --yellow: #fbbf24; --yellow-bg: #332810; --yellow-border: #5a4a1e; --yellow-text: #fbbf24;
    --red: #f87171; --red-bg: #331818; --red-border: #5a2a2a; --red-text: #f87171;
    --blue: #60a5fa; --blue-bg: #1a2d4a; --blue-border: #2a4a6a; --blue-text: #60a5fa;
    --purple: #a78bfa; --purple-bg: #2a1e44; --purple-border: #4a3468; --purple-text: #a78bfa;
    --gray: #9ca0b0; --gray-bg: #2a2e42; --gray-border: #3e4358; --gray-text: #9ca0b0;
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2e42; --surface-hover: #2e3348;
  --primary: #4da3ff; --primary-hover: #73b8ff; --primary-light: #1e3a5f; --primary-text: #4da3ff;
  --text: #e4e6ed; --text-secondary: #9ca0b0; --text-muted: #6b7084;
  --border: #363b50; --border-strong: #4a5068;
  --shadow: 0 1px 3px rgba(0,0,0,.3); --shadow-lg: 0 4px 12px rgba(0,0,0,.4);
  --green: #3ddc84; --green-bg: #1a3328; --green-border: #2a5a44; --green-text: #3ddc84;
  --yellow: #fbbf24; --yellow-bg: #332810; --yellow-border: #5a4a1e; --yellow-text: #fbbf24;
  --red: #f87171; --red-bg: #331818; --red-border: #5a2a2a; --red-text: #f87171;
  --blue: #60a5fa; --blue-bg: #1a2d4a; --blue-border: #2a4a6a; --blue-text: #60a5fa;
  --purple: #a78bfa; --purple-bg: #2a1e44; --purple-border: #4a3468; --purple-text: #a78bfa;
  --gray: #9ca0b0; --gray-bg: #2a2e42; --gray-border: #3e4358; --gray-text: #9ca0b0;
}
body { font-family: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; font-size: 14px; }
a { color: var(--primary); text-decoration: none; }
a:hover { color: var(--primary-hover); }

/* Header */
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 0 28px; height: 56px; display: flex; align-items: center; justify-content: space-between; position: sticky; top: 0; z-index: 100; }
header h1 { font-size: 16px; font-weight: 700; color: var(--text); display: flex; align-items: center; gap: 8px; }
header h1 .logo-icon { width: 22px; height: 22px; color: var(--primary); }
.header-right { display: flex; align-items: center; gap: 12px; }
.header-email { font-size: 13px; color: var(--text-secondary); max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.header-btn { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); border-radius: 6px; font-size: 13px; padding: 6px 12px; cursor: pointer; transition: all .15s; font-family: inherit; display: inline-flex; align-items: center; gap: 6px; }
.header-btn:hover { background: var(--surface-alt); color: var(--text); border-color: var(--border-strong); }
.header-btn svg { width: 16px; height: 16px; }

/* Nav */
nav { background: var(--surface); border-bottom: 1px solid var(--border); padding: 0 28px; display: flex; gap: 0; overflow-x: auto; }
nav a { padding: 12px 16px; font-size: 13px; font-weight: 500; color: var(--text-secondary); text-decoration: none; border-bottom: 2px solid transparent; transition: all .15s; white-space: nowrap; display: flex; align-items: center; gap: 6px; }
nav a:hover { color: var(--primary); background: var(--primary-light); }
nav a.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }
nav a svg { width: 16px; height: 16px; flex-shrink: 0; }

/* Main */
main { max-width: 1200px; margin: 0 auto; padding: 24px 28px; }

/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]::after {
  content: attr(data-tooltip);
  position: absolute; bottom: calc(100% + 6px); left: 50%; transform: translateX(-50%);
  background: var(--text); color: var(--bg); font-size: 11px; font-weight: 500;
  padding: 4px 8px; border-radius: 4px; white-space: nowrap; pointer-events: none;
  opacity: 0; transition: opacity .15s; z-index: 200;
}
[data-tooltip]:hover::after { opacity: 1; }

/* Flash */
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; animation: slideIn .3s ease; }
.flash.ok { background: var(--green-bg); color: var(--green-text); border: 1px solid var(--green-border); }
.flash.err { background: var(--red-bg); color: var(--red-text); border: 1px solid var(--red-border); }
@keyframes slideIn { from { opacity: 0; transform: translateY(-8px); } to { opacity: 1; transform: translateY(0); } }

/* Cards */
.card { background: var(--surface); border-radius: var(--radius); padding: 20px; box-shadow: var(--shadow); margin-bottom: 16px; border: 1px solid var(--border); }
.card h2 { font-size: 16px; font-weight: 700; margin-bottom: 16px; display: flex; align-items: center; gap: 8px; color: var(--text); }
.card h2 svg { width: 18px; height: 18px; color: var(--primary); flex-shrink: 0; }
.card h3 { font-size: 14px; font-weight: 600; margin-bottom: 8px; color: var(--text); }

/* Progress bar */
.progress { background: var(--gray-bg); border-radius: 6px; height: 8px; overflow: hidden; }
.progress-bar { height: 100%; border-radius: 6px; background: var(--primary); transition: width .4s ease; }
.progress-label { font-size: 12px; color: var(--text-secondary); margin-top: 6px; display: flex; justify-content: space-between; }

/* Stage badge */
.stage-badge { display: inline-flex; align-items: center; gap: 4px; padding: 4px 12px; border-radius: 20px; font-size: 12px; font-weight: 600; background: var(--primary-light); color: var(--primary-text); }

/* Grid */
.grid { display: grid; gap: 16px; }
.grid-2 { grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); }
.grid-3 { grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); }

/* Stat card */
.stat-card { background: var(--surface); border-radius: var(--radius); padding: 18px; box-shadow: var(--shadow); border: 1px solid var(--border); border-left: 3px solid var(--primary); }
.stat-card .n { font-size: 28px; font-weight: 800; color: var(--primary); }
.stat-card .l { font-size: 12px; color: var(--text-secondary); margin-top: 4px; }

/* Table */
.table-wrap { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; }
thead th { background: var(--surface-alt); padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .4px; border-bottom: 2px solid var(--border); white-space: nowrap; }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; word-break: break-word; }
tbody tr:hover { background: var(--surface-hover); }
tbody tr:last-child td { border-bottom: none; }

/* Badges */
.badge { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; white-space: nowrap; }
.badge-pending { background: var(--yellow-bg); color: var(--yellow-text); border: 1px solid var(--yellow-border); }
.badge-submitted { background: var(--blue-bg); color: var(--blue-text); border: 1px solid var(--blue-border); }
.badge-approved { background: var(--green-bg); color: var(--green-text); border: 1px solid var(--green-border); }
.badge-rejected { background: var(--red-bg); color: var(--red-text); border: 1px solid var(--red-border); }
.badge-completed { background: var(--green-bg); color: var(--green-text); border: 1px solid var(--green-border); }
.badge-overdue { background: var(--red-bg); color: var(--red-text); border: 1px solid var(--red-border); }
.badge-upcoming { background: var(--blue-bg); color: var(--blue-text); border: 1px solid var(--blue-border); }

/* Deadline row colors */
.row-overdue { background: var(--red-bg) !important; }
.row-completed { background: var(--green-bg) !important; }

/* Buttons */
.btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 18px; border: none; border-radius: var(--radius); font-size: 13px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: inherit; text-decoration: none; }
.btn svg { width: 16px; height: 16px; flex-shrink: 0; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); box-shadow: 0 2px 8px rgba(0,115,234,.25); }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--gray-bg); }

/* Select */
select { padding: 8px 12px; border: 1px solid var(--border); border-radius: var(--radius); font-size: 13px; outline: none; font-family: inherit; background: var(--surface); color: var(--text); }
select:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.12); }

/* Empty state */
.empty { text-align: center; padding: 40px 24px; color: var(--text-secondary); }
.empty svg { width: 40px; height: 40px; margin-bottom: 12px; color: var(--text-muted); }

/* Clickable cards */
.card[onclick] { cursor: pointer; transition: transform .15s, box-shadow .15s; }
.card[onclick]:hover { transform: translateY(-1px); box-shadow: var(--shadow-lg); }

/* Responsive */
@media (max-width: 1024px) {
  main { padding: 20px 24px; }
  .grid-3 { grid-template-columns: repeat(2, 1fr); }
}
@media (max-width: 768px) {
  main { padding: 16px; }
  .grid-2, .grid-3 { grid-template-columns: 1fr; }
  header { padding: 0 16px; }
  nav { padding: 0 12px; }
  .header-email { display: none; }
}
@media (max-width: 480px) {
  header { padding: 0 12px; height: 50px; }
  header h1 { font-size: 14px; }
  header h1 .logo-icon { width: 18px; height: 18px; }
  nav a { padding: 10px 10px; font-size: 12px; }
  .card { padding: 14px; }
  thead th, tbody td { padding: 8px 10px; }
}

/* Timeline for recent changes */
.changes-timeline { position: relative; padding-left: 20px; }
.changes-timeline::before { content: ''; position: absolute; left: 6px; top: 8px; bottom: 8px; width: 2px; background: var(--border); }
.change-item { position: relative; padding: 10px 0 10px 20px; border-bottom: 1px solid var(--border); }
.change-item:last-child { border-bottom: none; }
.change-dot { position: absolute; left: -17px; top: 14px; width: 12px; height: 12px; border-radius: 50%; border: 2px solid var(--primary); background: var(--surface); }
.change-item.change-new .change-dot { background: var(--green); border-color: var(--green); }
.change-item.change-updated .change-dot { background: var(--blue); border-color: var(--blue); }
.change-item.change-outdated .change-dot { background: var(--red); border-color: var(--red); }
.change-item.change-removed .change-dot { background: var(--gray); border-color: var(--gray); }
.change-header { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-bottom: 4px; }
.change-kind { font-size: 11px; font-weight: 600; padding: 2px 8px; border-radius: 12px; }
.change-kind-new { background: var(--green-bg); color: var(--green); }
.change-kind-updated { background: var(--blue-bg, #e5f0fc); color: var(--blue); }
.change-kind-outdated { background: var(--red-bg); color: var(--red); }
.change-kind-removed { background: var(--gray-bg); color: var(--gray); }
.change-entity { font-size: 10px; color: var(--text-secondary); font-family: monospace; background: var(--gray-bg); padding: 2px 6px; border-radius: 3px; }
.change-time { font-size: 11px; color: var(--text-secondary); }
.change-title { font-size: 13px; font-weight: 500; margin-bottom: 2px; }
.change-summary { font-size: 12px; color: var(--text-secondary); margin-top: 4px; line-height: 1.4; }
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t);else if(matchMedia('(prefers-color-scheme: dark)').matches)document.documentElement.setAttribute('data-theme','dark')})();</script>
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
  var cur = r.getAttribute('data-theme');
  if (!cur) cur = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  updateThemeIcon(next);
}
function updateThemeIcon(theme) {
  var btns = document.querySelectorAll('[data-theme-toggle]');
  for (var i = 0; i < btns.length; i++) {
    btns[i].innerHTML = theme === 'dark'
      ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="19"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>'
      : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>';
  }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme');
  if (!cur) cur = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  updateThemeIcon(cur);
});
</script>
</body>
</html>{{end}}

{{/* ===== HEADER ===== */}}
{{define "header"}}
<header>
  <h1>
    <svg class="logo-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 21h18"/><path d="M5 21V7l7-4 7 4v14"/><path d="M9 21v-4h6v4"/><path d="M9 9h.01"/><path d="M15 9h.01"/><path d="M9 13h.01"/><path d="M15 13h.01"/></svg>
    База Сколково
  </h1>
  <div class="header-right">
    {{if .Client}}<span class="header-email" data-tooltip="Текущий аккаунт">{{.Client.Email}}</span>{{end}}
    <button id="themeBtn" onclick="toggleTheme()" data-tooltip="Переключить тему" data-theme-toggle class="header-btn" aria-label="Переключить тему"></button>
    <a href="/logout" class="header-btn" data-tooltip="Выйти из личного кабинета">Выйти</a>
  </div>
</header>
{{end}}

{{/* ===== NAV ===== */}}
{{define "nav"}}
<nav>
  <a href="/dashboard"{{if .ActiveTabDashboard}} class="active"{{end}} data-tooltip="Обзор текущей стадии, дедлайнов и прогресса">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
    Дашборд
  </a>
  <a href="/checklists"{{if .ActiveTabChecklists}} class="active"{{end}} data-tooltip="Список шагов для прохождения процедур резидентства">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
    Чек-листы
  </a>
  <a href="/deadlines"{{if .ActiveTabDeadlines}} class="active"{{end}} data-tooltip="Сроки подачи документов и отчётности">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
    Дедлайны
  </a>
  <a href="/documents"{{if .ActiveTabDocuments}} class="active"{{end}} data-tooltip="Документы, связанные с вашим резидентством">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
    Документы
  </a>
  <a href="/generate"{{if .ActiveTabGenerate}} class="active"{{end}} data-tooltip="Генерация документов из шаблонов на основе данных профиля">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
    Генерация
  </a>
</nav>
{{end}}

{{/* ===== LOGIN PAGE ===== */}}
{{define "login"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Вход — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700;800&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f5f6f8; --surface: #ffffff; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e8f2fc; --text: #1a1d29; --text-secondary: #676f83;
  --border: #e0e2e8; --radius: 8px;
  --shadow-lg: 0 8px 30px rgba(0,0,0,.1);
  --green: #00875a; --green-bg: #e6f7f0; --green-border: #b3e0ce; --green-text: #00875a;
  --red: #de350b; --red-bg: #fde8e0; --red-border: #f5b8a0; --red-text: #de350b;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --surface: #23273a; --primary: #4da3ff; --primary-hover: #73b8ff;
    --primary-light: #1e3a5f; --text: #e4e6ed; --text-secondary: #9ca0b0;
    --border: #363b50; --shadow-lg: 0 8px 30px rgba(0,0,0,.4);
    --green: #3ddc84; --green-bg: #1a3328; --green-border: #2a5a44; --green-text: #3ddc84;
    --red: #f87171; --red-bg: #331818; --red-border: #5a2a2a; --red-text: #f87171;
  }
}
:root[data-theme="dark"] {
  --surface: #23273a; --primary: #4da3ff; --primary-hover: #73b8ff;
  --primary-light: #1e3a5f; --text: #e4e6ed; --text-secondary: #9ca0b0;
  --border: #363b50; --shadow-lg: 0 8px 30px rgba(0,0,0,.4);
  --green: #3ddc84; --green-bg: #1a3328; --green-border: #2a5a44; --green-text: #3ddc84;
  --red: #f87171; --red-bg: #331818; --red-border: #5a2a2a; --red-text: #f87171;
}
body { font-family: 'Figtree', sans-serif; background: var(--bg); min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px; }
.card { background: var(--surface); border-radius: 12px; padding: 36px; box-shadow: var(--shadow-lg); max-width: 420px; width: 100%; border: 1px solid var(--border); }
.logo { text-align: center; margin-bottom: 28px; }
.logo h1 { font-size: 22px; font-weight: 800; color: var(--text); display: flex; align-items: center; justify-content: center; gap: 10px; }
.logo h1 svg { width: 28px; height: 28px; color: var(--primary); }
.logo p { font-size: 13px; color: var(--text-secondary); margin-top: 6px; }
.form-group { margin-bottom: 16px; }
.form-group label { display: block; font-size: 13px; font-weight: 600; color: var(--text-secondary); margin-bottom: 6px; }
.form-group input { width: 100%; padding: 10px 14px; border: 1px solid var(--border); border-radius: var(--radius); font-size: 14px; outline: none; transition: all .15s; font-family: inherit; background: var(--surface); color: var(--text); }
.form-group input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.12); }
.btn { width: 100%; padding: 12px; border: none; border-radius: var(--radius); font-size: 14px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: inherit; display: inline-flex; align-items: center; justify-content: center; gap: 8px; }
.btn svg { width: 16px; height: 16px; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); box-shadow: 0 2px 8px rgba(0,115,234,.25); }
.flash { padding: 10px 14px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; }
.flash.ok { background: var(--green-bg); color: var(--green-text); border: 1px solid var(--green-border); }
.flash.err { background: var(--red-bg); color: var(--red-text); border: 1px solid var(--red-border); }
.link-box { background: var(--primary-light); border: 1px solid var(--primary); border-radius: var(--radius); padding: 10px 12px; margin-top: 12px; font-size: 12px; word-break: break-all; color: var(--primary); display: flex; align-items: center; gap: 8px; }
.link-box svg { width: 16px; height: 16px; flex-shrink: 0; }
.link-box a { color: var(--primary); font-weight: 600; }
.theme-toggle { position: fixed; top: 16px; right: 16px; background: var(--surface); border: 1px solid var(--border); border-radius: 50%; width: 40px; height: 40px; display: flex; align-items: center; justify-content: center; cursor: pointer; color: var(--text-secondary); transition: all .15s; }
.theme-toggle:hover { color: var(--text); border-color: var(--border-strong); }
.theme-toggle svg { width: 18px; height: 18px; }
@media (max-width: 480px) { .card { padding: 24px; } }
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t);else if(matchMedia('(prefers-color-scheme: dark)').matches)document.documentElement.setAttribute('data-theme','dark')})();</script>
</head>
<body>
<button onclick="toggleTheme()" data-tooltip="Переключить тему" class="theme-toggle" aria-label="Переключить тему" data-theme-toggle></button>
<div class="card">
  <div class="logo">
    <h1>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 21h18"/><path d="M5 21V7l7-4 7 4v14"/><path d="M9 21v-4h6v4"/><path d="M9 9h.01"/><path d="M15 9h.01"/><path d="M9 13h.01"/><path d="M15 13h.01"/></svg>
      База Сколково
    </h1>
    <p>Личный кабинет резидента</p>
  </div>
  {{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
  {{if .Link}}
    <div class="link-box">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
      <a href="{{.Link}}" target="_blank">Перейти по ссылке для входа</a>
    </div>
    <p style="font-size:11px;color:var(--text-secondary);margin-top:8px;text-align:center">В продакшене ссылка будет отправлена на email</p>
  {{else}}
    <form method="POST" action="/login">
      <div class="form-group">
        <label for="email">Электронная почта (Email)</label>
        <input type="email" id="email" name="email" placeholder="name@company.ru" required autocomplete="email" data-tooltip="Введите email-адрес, на который придёт ссылка для входа">
      </div>
      <button type="submit" class="btn btn-primary" data-tooltip="Отправить ссылку для входа без пароля на указанный email">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
        Отправить ссылку для входа
      </button>
    </form>
  {{end}}
</div>
<script>
function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme');
  if (!cur) cur = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  updateThemeIcon(next);
}
function updateThemeIcon(theme) {
  var btns = document.querySelectorAll('[data-theme-toggle]');
  for (var i = 0; i < btns.length; i++) {
    btns[i].innerHTML = theme === 'dark'
      ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="18" height="18"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="19"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>'
      : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="18" height="18"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>';
  }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme');
  if (!cur) cur = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  updateThemeIcon(cur);
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
      <h2>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
        Текущая стадия
      </h2>
      <div style="margin-bottom:12px">
        <span class="stage-badge" data-tooltip="Стадия: {{.Client.ResidencyStage}}">{{.Client.ResidencyStage}}</span>
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
      <h2>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
        Прогресс чек-листов
      </h2>
      {{if .Checklists}}
        {{range .Checklists}}
          <div style="margin-bottom:14px">
            <div style="display:flex;justify-content:space-between;font-size:13px;margin-bottom:4px">
              <span data-tooltip="Чек-лист: {{.Title}}">{{.Title}}</span>
              <span style="color:var(--text-secondary)">{{.Progress}}%</span>
            </div>
            <div class="progress">
              <div class="progress-bar" style="width:{{.Progress}}%"></div>
            </div>
          </div>
        {{end}}
      {{else}}
        <div class="empty">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
          <p>Чек-листы пока не назначены</p>
        </div>
      {{end}}
    </div>
  </div>
  <div>
    <div class="card">
      <h2>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
        Ближайшие дедлайны
      </h2>
      {{if .Deadlines}}
        <div class="table-wrap">
          <table>
            <thead><tr><th>Дедлайн</th><th>Дата</th><th>Статус</th></tr></thead>
            <tbody>
            {{range .Deadlines}}
              <tr class="row-{{.StatusClass}}">
                <td data-tooltip="{{.Title}}">{{.Title}}</td>
                <td>{{.DueDate.Format "02.01.2006"}}</td>
                <td><span class="badge badge-{{.StatusClass}}" data-tooltip="{{.StatusLabel}}">{{.StatusLabel}}</span></td>
              </tr>
            {{end}}
            </tbody>
          </table>
        </div>
      {{else}}
        <div class="empty">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
          <p>Нет ближайших дедлайнов</p>
        </div>
      {{end}}
    </div>
    <div class="card">
      <h2>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
        Статус документов
      </h2>
      {{if .Documents}}
        <div style="display:flex;gap:8px;flex-wrap:wrap">
          {{range .Documents}}
            <span class="badge badge-{{.StatusClass}}" data-tooltip="{{.StatusLabel}}">{{.StatusLabel}}</span>
          {{end}}
        </div>
      {{else}}
        <div class="empty">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          <p>Документы пока не назначены</p>
        </div>
      {{end}}
    </div>
  </div>
</div>

{{/* Что нового — блок последних изменений */}}
{{if .RecentChanges}}
<div class="card" style="margin-top:16px">
  <h2>
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 6 13.5 15.5 8.5 10.5 1 18"/><polyline points="17 6 23 6 23 12"/></svg>
    Что нового в базе Сколково
  </h2>
  <div class="changes-timeline">
    {{range .RecentChanges}}
    <div class="change-item change-{{.Kind}}">
      <div class="change-dot"></div>
      <div class="change-content">
        <div class="change-header">
          <span class="change-kind change-kind-{{.Kind}}">
            {{if eq .Kind "new"}}🆕 Новая{{else if eq .Kind "updated"}}🔄 Обновлена{{else if eq .Kind "outdated"}}⛔ Устарела{{else if eq .Kind "removed"}}🗑 Удалена{{end}}
          </span>
          <span class="change-entity">{{.EntityType}}</span>
          <span class="change-time">{{.DetectedAt}}</span>
        </div>
        <div class="change-title">{{.Title}}</div>
        {{if .Summary}}<div class="change-summary">{{.Summary}}</div>{{end}}
      </div>
    </div>
    {{end}}
  </div>
</div>
{{end}}

{{end}}

{{/* ===== CHECKLISTS PAGE ===== */}}
{{define "checklists"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="card">
  <h2>
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
    Мои чек-листы
  </h2>
  {{if .Checklists}}
    <div class="grid grid-2">
    {{range .Checklists}}
      <div class="card" style="margin:0">
        <h3>{{.Title}}</h3>
        <div style="font-size:12px;color:var(--text-secondary);margin-bottom:8px">
          Тип: {{.ProcedureType}} / Статус: {{.Status}}
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
              <div style="display:flex;align-items:center;gap:8px;padding:6px 8px;font-size:12px;border-radius:4px">
                {{if eq .Status "done"}}
                  <svg viewBox="0 0 24 24" fill="none" stroke="var(--green)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16" data-tooltip="Шаг выполнен"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
                {{else if eq .Status "in_progress"}}
                  <svg viewBox="0 0 24 24" fill="none" stroke="var(--blue)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16" data-tooltip="Шаг в процессе выполнения"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
                {{else}}
                  <svg viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16" data-tooltip="Шаг ещё не начат"><circle cx="12" cy="12" r="10"/></svg>
                {{end}}
                <span style="{{if eq .Status "done"}}text-decoration:line-through;color:var(--text-secondary){{else if eq .Status "in_progress"}}font-weight:500{{end}}">{{.Title}}</span>
              </div>
            {{end}}
          </div>
        {{end}}
      </div>
    {{end}}
    </div>
  {{else}}
    <div class="empty">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
      <p>Чек-листы пока не назначены</p>
    </div>
  {{end}}
</div>
{{end}}

{{/* ===== DEADLINES PAGE ===== */}}
{{define "deadlines"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="grid grid-2">
  <div class="card">
    <h2>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
      Ближайшие дедлайны
    </h2>
    {{if .Deadlines}}
      <div class="table-wrap">
        <table>
          <thead><tr><th>Название</th><th>Дата</th><th>Тип</th><th>Статус</th></tr></thead>
          <tbody>
          {{range .Deadlines}}
            <tr class="row-{{.StatusClass}}">
              <td style="font-weight:500" data-tooltip="{{.Title}}">{{.Title}}</td>
              <td>{{.DueDate.Format "02.01.2006"}}</td>
              <td><span style="font-size:11px;color:var(--text-secondary)">{{.Type}}</span></td>
              <td><span class="badge badge-{{.StatusClass}}" data-tooltip="{{.StatusLabel}}">{{.StatusLabel}}</span></td>
            </tr>
          {{end}}
          </tbody>
        </table>
      </div>
    {{else}}
      <div class="empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
        <p>Нет ближайших дедлайнов</p>
      </div>
    {{end}}
  </div>
  <div class="card">
    <h2>
      <svg viewBox="0 0 24 24" fill="none" stroke="var(--red)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
      Просроченные
    </h2>
    {{if .Overdue}}
      <div class="table-wrap">
        <table>
          <thead><tr><th>Название</th><th>Дата</th><th>Статус</th></tr></thead>
          <tbody>
          {{range .Overdue}}
            <tr class="row-overdue">
              <td style="font-weight:500" data-tooltip="{{.Title}}">{{.Title}}</td>
              <td style="color:var(--red);font-weight:600">{{.DueDate.Format "02.01.2006"}}</td>
              <td><span class="badge badge-overdue">Просрочен</span></td>
            </tr>
          {{end}}
          </tbody>
        </table>
      </div>
    {{else}}
      <div class="empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
        <p>Нет просроченных дедлайнов</p>
      </div>
    {{end}}
  </div>
</div>
{{end}}

{{/* ===== DOCUMENTS PAGE ===== */}}
{{define "documents"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="card">
  <h2>
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
    Мои документы
  </h2>
  {{if .Documents}}
    <div class="table-wrap">
      <table>
        <thead><tr><th>Документ</th><th>Роль</th><th>Статус</th><th>Дата</th><th>Источник</th></tr></thead>
        <tbody>
        {{range .Documents}}
          <tr>
            <td style="font-weight:500" data-tooltip="{{.Name}}">{{.Name}}</td>
            <td><span style="font-size:12px;color:var(--text-secondary)">{{.Role}}</span></td>
            <td><span class="badge badge-{{.StatusClass}}" data-tooltip="{{.StatusLabel}}">{{.StatusLabel}}</span></td>
            <td style="font-size:12px;color:var(--text-secondary)">{{.Date}}</td>
            <td>{{if .SourceURL}}<a href="{{.SourceURL}}" target="_blank" rel="noopener" style="font-size:12px;color:var(--primary);text-decoration:none" data-tooltip="Открыть источник в новой вкладке">источник ↗</a>{{else}}<span style="font-size:12px;color:var(--text-muted)">—</span>{{end}}</td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
  {{else}}
    <div class="empty">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
      <p>Документы пока не назначены</p>
    </div>
  {{end}}
</div>
{{end}}

{{/* ===== GENERATE PAGE ===== */}}
{{define "generate"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="card">
  <h2>
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
    Генерация документа из шаблона
  </h2>
  <p style="font-size:13px;color:var(--text-secondary);margin-bottom:20px">Выберите шаблон — документ будет создан автоматически на основе данных вашего профиля и сохранён в раздел «Документы».</p>
  <form method="POST" action="/generate" id="generateForm">
    <div style="display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap">
      <div style="flex:1;min-width:240px">
        <label for="template" style="display:block;font-size:13px;font-weight:600;color:var(--text-secondary);margin-bottom:6px;text-transform:uppercase;letter-spacing:.4px">Шаблон документа</label>
        <select id="template" name="template_id" style="width:100%" data-tooltip="Выберите тип документа для генерации из списка доступных шаблонов" required>
          <option value="">— Выберите шаблон —</option>
          {{range .Templates}}
            <option value="{{.ID}}">{{.Name}} ({{.Type}})</option>
          {{end}}
        </select>
      </div>
      <button type="submit" class="btn btn-primary" data-tooltip="Создать документ по выбранному шаблону" style="white-space:nowrap">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
        Сгенерировать
      </button>
    </div>
  </form>
</div>
{{if .Templates}}
<div class="card">
  <h3 style="font-size:14px;font-weight:600;margin-bottom:12px">Доступные шаблоны</h3>
  <div class="grid grid-3">
  {{range .Templates}}
    <div class="card" style="margin:0;padding:16px;border-left:3px solid var(--primary);cursor:pointer;transition:transform .15s,box-shadow .15s"
         onclick="document.getElementById('template').value='{{.ID}}';document.getElementById('generateForm').requestSubmit()"
         data-tooltip="Нажмите, чтобы сгенерировать документ по этому шаблону">
      <div style="font-size:13px;font-weight:600;color:var(--text)">{{.Name}}</div>
      <div style="font-size:11px;color:var(--text-secondary);margin-top:6px;display:flex;gap:10px;flex-wrap:wrap">
        <span data-tooltip="Тип документа">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="12" height="12" style="display:inline;vertical-align:middle"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          {{.Type}}
        </span>
        <span data-tooltip="Версия шаблона">v{{.Version}}</span>
      </div>
    </div>
  {{end}}
  </div>
</div>
{{else}}
<div class="empty" style="margin-top:16px">
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
  <p><strong>Шаблоны пока не добавлены</strong></p>
  <p>Шаблоны документов создаются администратором в разделе резидентства</p>
</div>
{{end}}
{{end}}

`))
