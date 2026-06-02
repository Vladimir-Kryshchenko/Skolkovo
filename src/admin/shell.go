// Package admin — общий «каркас» интерфейса: левое навигационное меню (сайдбар),
// единое для всех страниц обеих админок (:8090 главная и :8091 резидентство).
//
// Зачем отдельный файл: шаблоны страниц лежат в разных *template.Template
// (tmpl, aiTmpl, residencyTmpl и т.д.), поэтому общий партиал нельзя задать один раз —
// его подмешивают в каждый инстанс строкой `+ sidebarMainDefine` (или Residency)
// к вызову .Parse(...). Страница просто вызывает {{template "sidebar" .}} вместо
// прежней «шапки» <header>. Сайдбар полностью статичен (не зависит от данных страницы),
// поэтому одинаково безопасен для любого типа данных; активный пункт и тема — на JS.
package admin

// ── Иконки (Feather-стиль, stroke) ───────────────────────────────────────────
// Держим набор в одном месте, чтобы пункты меню выглядели единообразно.
const (
	icoDocs      = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>`
	icoChanges   = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>`
	icoGlobe     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>`
	icoDiff      = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M6 21V9a9 9 0 0 0 9 9"/></svg>`
	icoGraph     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg>`
	icoChart     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>`
	icoCpu       = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><line x1="9" y1="1" x2="9" y2="4"/><line x1="15" y1="1" x2="15" y2="4"/><line x1="9" y1="20" x2="9" y2="23"/><line x1="15" y1="20" x2="15" y2="23"/><line x1="20" y1="9" x2="23" y2="9"/><line x1="20" y1="14" x2="23" y2="14"/><line x1="1" y1="9" x2="4" y2="9"/><line x1="1" y1="14" x2="4" y2="14"/></svg>`
	icoBot       = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="10" rx="2"/><circle cx="12" cy="5" r="2"/><path d="M12 7v4"/><line x1="8" y1="16" x2="8" y2="16"/><line x1="16" y1="16" x2="16" y2="16"/></svg>`
	icoShield    = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>`
	icoBook      = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"/><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"/></svg>`
	icoUsers     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>`
	icoCheck     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>`
	icoClock     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`
	icoTemplate  = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>`
	icoBriefcase = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"/></svg>`
	icoCalendar  = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>`
	icoAward     = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="7"/><polyline points="8.21 13.89 7 23 12 20 17 23 15.79 13.88"/></svg>`
	icoLogout    = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>`
	icoMenu      = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="18" x2="21" y2="18"/></svg>`
	icoChevron   = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>`
	icoMoon      = `<svg class="sb-icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`
	icoSun       = `<svg class="sb-icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>`
	icoLogo      = `<svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg>`
)

// appSidebarCSS — стили каркаса. Используют ТОЛЬКО те CSS-переменные (--surface,
// --primary, --text, --border …), которые объявлены на каждой странице, поэтому
// сайдбар автоматически берёт палитру и тему конкретной страницы.
// Блок вставляется в <body> после head-стилей страницы, поэтому свои правила
// (например body{padding-left}) он переопределяет корректно.
const appSidebarCSS = `
:root{--app-sb-w:236px;--app-sb-wc:64px}
html.sb-collapsed{--app-sb-w:var(--app-sb-wc)}
body{padding-left:var(--app-sb-w);transition:padding-left .18s ease}
.app-sidebar{position:fixed;top:0;left:0;bottom:0;width:var(--app-sb-w);background:var(--surface);border-right:1px solid var(--border);display:flex;flex-direction:column;z-index:200;transition:width .18s ease,transform .2s ease;font-family:var(--font,'Figtree',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif)}
.app-sb-brand{display:flex;align-items:center;gap:10px;padding:0 14px;min-height:57px;border-bottom:1px solid var(--border);flex-shrink:0;text-decoration:none}
.app-sb-brand .logo-i{width:32px;height:32px;flex-shrink:0;background:var(--primary);border-radius:8px;display:flex;align-items:center;justify-content:center}
.app-sb-brand .logo-i svg{width:18px;height:18px;fill:#fff}
.app-sb-brand .b-txt{display:flex;flex-direction:column;line-height:1.18;overflow:hidden}
.app-sb-brand .b-txt b{font-size:14px;font-weight:700;color:var(--text);white-space:nowrap}
.app-sb-brand .b-txt span{font-size:10.5px;color:var(--text-secondary);white-space:nowrap;letter-spacing:.3px}
.app-sb-nav{flex:1;overflow-y:auto;overflow-x:hidden;padding:6px 0 10px}
.app-sb-group{padding:12px 18px 5px;font-size:10px;font-weight:700;letter-spacing:.7px;text-transform:uppercase;color:var(--text-secondary);opacity:.65;white-space:nowrap}
.app-sb-nav a{position:relative;display:flex;align-items:center;gap:12px;padding:9px 18px;color:var(--text-secondary);text-decoration:none;font-size:13.5px;font-weight:500;border-left:3px solid transparent;white-space:nowrap;transition:background .12s,color .12s}
.app-sb-nav a svg{width:18px;height:18px;flex-shrink:0}
.app-sb-nav a:hover{background:var(--primary-light, rgba(0,115,234,.12));color:var(--primary)}
.app-sb-nav a.active{background:var(--primary-light, rgba(0,115,234,.12));color:var(--primary);border-left-color:var(--primary);font-weight:600}
.app-sb-foot{border-top:1px solid var(--border);padding:8px;display:flex;gap:6px;align-items:stretch;flex-shrink:0}
.app-sb-foot button,.app-sb-foot a{display:inline-flex;align-items:center;justify-content:center;gap:7px;height:36px;border-radius:7px;border:1px solid var(--border);background:transparent;color:var(--text-secondary);font-size:12.5px;font-weight:600;cursor:pointer;text-decoration:none;font-family:inherit;transition:background .12s,color .12s,border-color .12s}
.app-sb-foot button:hover,.app-sb-foot a:hover{background:var(--primary-light, rgba(0,115,234,.12));color:var(--primary);border-color:var(--primary)}
.app-sb-foot .sb-theme{width:36px;flex-shrink:0;padding:0}
.app-sb-foot .sb-theme svg{width:17px;height:17px}
.app-sb-foot .sb-logout{flex:1}
.app-sb-foot .sb-logout svg{width:15px;height:15px}
.app-sb-toggle{position:absolute;top:62px;right:-12px;width:24px;height:24px;border-radius:50%;background:var(--surface);border:1px solid var(--border);color:var(--text-secondary);display:flex;align-items:center;justify-content:center;cursor:pointer;z-index:201;box-shadow:0 1px 5px rgba(0,0,0,.14);transition:color .12s,transform .18s}
.app-sb-toggle:hover{color:var(--primary)}
.app-sb-toggle svg{width:14px;height:14px}
html.sb-collapsed .app-sb-toggle svg{transform:rotate(180deg)}
/* свёрнутый режим — только иконки */
html.sb-collapsed .app-sidebar .b-txt,html.sb-collapsed .app-sidebar .nav-label,html.sb-collapsed .app-sidebar .app-sb-group,html.sb-collapsed .app-sidebar .sb-logout span{display:none}
html.sb-collapsed .app-sidebar .app-sb-brand{justify-content:center;padding:0}
html.sb-collapsed .app-sb-nav a{justify-content:center;gap:0;padding:11px 0;border-left-width:0}
html.sb-collapsed .app-sb-foot{flex-direction:column}
html.sb-collapsed .app-sb-foot .sb-logout{flex:none;width:100%}
/* подсказки сайдбара — всплывают справа, с переносом строк */
.app-sidebar [data-tip]{position:relative}
.app-sidebar [data-tip]:hover::after{content:attr(data-tip);position:absolute;left:calc(100% + 10px);top:50%;transform:translateY(-50%);background:#11131f;color:#fff;padding:7px 11px;border-radius:7px;font-size:12px;font-weight:500;line-height:1.35;white-space:normal;width:max-content;max-width:230px;z-index:320;pointer-events:none;box-shadow:0 4px 16px rgba(0,0,0,.28)}
.app-sidebar .app-sb-foot [data-tip]:hover::after{top:auto;bottom:50%;transform:translateY(50%)}
/* мобильный гамбургер + затемнение */
.app-sb-mobile{display:none;position:fixed;top:10px;left:10px;z-index:210;width:42px;height:42px;border-radius:9px;background:var(--surface);border:1px solid var(--border);color:var(--text);align-items:center;justify-content:center;cursor:pointer;box-shadow:0 2px 10px rgba(0,0,0,.18)}
.app-sb-mobile svg{width:20px;height:20px}
.app-sb-backdrop{display:none;position:fixed;inset:0;background:rgba(0,0,0,.4);z-index:195}
@media(max-width:860px){
  body{padding-left:0}
  .app-sidebar{transform:translateX(-100%);width:236px;box-shadow:0 8px 32px rgba(0,0,0,.3)}
  html.sb-open .app-sidebar{transform:translateX(0)}
  html.sb-collapsed .app-sidebar{width:236px}
  html.sb-collapsed .app-sidebar .b-txt,html.sb-collapsed .app-sidebar .nav-label,html.sb-collapsed .app-sidebar .app-sb-group,html.sb-collapsed .app-sidebar .sb-logout span{display:revert}
  html.sb-collapsed .app-sb-nav a{justify-content:flex-start;gap:12px;padding:9px 18px;border-left-width:3px}
  html.sb-collapsed .app-sb-foot{flex-direction:row}
  .app-sb-toggle{display:none}
  .app-sb-mobile{display:flex}
  html.sb-open .app-sb-backdrop{display:block}
  .app-sidebar [data-tip]:hover::after{display:none}
}
`

// appSidebarScript — поведение каркаса: тема, активный пункт, сворачивание, мобильное меню.
// Скрипт стоит сразу за разметкой сайдбара, поэтому DOM сайдбара уже доступен.
const appSidebarScript = `
function sbUpdateThemeIcons(t){var m=document.querySelectorAll('.sb-icon-moon'),s=document.querySelectorAll('.sb-icon-sun');for(var i=0;i<m.length;i++)m[i].style.display=t==='dark'?'none':'';for(var j=0;j<s.length;j++)s[j].style.display=t==='dark'?'':'none';}
function sbToggleTheme(){var r=document.documentElement,cur=r.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'),next=cur==='dark'?'light':'dark';r.setAttribute('data-theme',next);try{localStorage.setItem('theme',next)}catch(e){}sbUpdateThemeIcons(next);}
function sbToggleCollapse(){var c=document.documentElement.classList.toggle('sb-collapsed');try{localStorage.setItem('sbCollapsed',c?'1':'0')}catch(e){}}
function sbToggleMobile(){document.documentElement.classList.toggle('sb-open');}
function sbCloseMobile(){document.documentElement.classList.remove('sb-open');}
(function(){
  var path=(location.pathname||'/').replace(/\/+$/,'')||'/';
  var links=document.querySelectorAll('.app-sidebar a[data-path]'),best=null,bestLen=-1;
  for(var i=0;i<links.length;i++){var p=links[i].getAttribute('data-path'),m=false;if(p==='/')m=(path==='/');else m=(path===p||path.indexOf(p+'/')===0);if(m&&p.length>bestLen){best=links[i];bestLen=p.length;}}
  if(best)best.classList.add('active');
  var cur=document.documentElement.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');
  sbUpdateThemeIcons(cur);
})();
`

// appShellCollapseRestore — синхронно (до отрисовки) восстанавливает свёрнутое состояние,
// чтобы сайдбар не «прыгал» при загрузке.
const appShellCollapseRestore = `<script>(function(){try{if(localStorage.getItem('sbCollapsed')==='1')document.documentElement.classList.add('sb-collapsed');}catch(e){}})();</script>`

// navItem собирает один пункт меню: иконка + подпись + интерактивная подсказка.
func navItem(path, icon, label, tip string) string {
	return `<a href="` + path + `" data-path="` + path + `" data-tip="` + tip + `">` + icon + `<span class="nav-label">` + label + `</span></a>`
}

// navMain — пункты левого меню ГЛАВНОЙ админки (:8090).
var navMain = `<aside class="app-sidebar">
<a class="app-sb-brand" href="/" data-tip="На главную — список документов"><span class="logo-i">` + icoLogo + `</span><span class="b-txt"><b>База Сколково</b><span>Админ-панель</span></span></a>
<button class="app-sb-toggle" onclick="sbToggleCollapse()" data-tip="Свернуть / развернуть меню" aria-label="Свернуть меню">` + icoChevron + `</button>
<nav class="app-sb-nav">
<div class="app-sb-group">База знаний</div>
` + navItem("/", icoDocs, "Документы", "Реестр документов: статусы, файлы, индексация в поиск") +
	navItem("/changes", icoChanges, "Изменения", "Лента: что появилось, обновилось или устарело в базе") +
	navItem("/sitepages", icoGlobe, "Страницы сайта", "Публичные страницы sk.ru/dochub с ИИ-аннотациями и тегами") +
	navItem("/diff", icoDiff, "Сравнение версий", "Сравнить две версии документа и увидеть, что изменилось") +
	navItem("/graph", icoGraph, "Граф связей", "Связи между документами: замещения, ссылки, редакции") +
	navItem("/analytics", icoChart, "Аналитика", "Статистика базы и динамика наполнения, экспорт в CSV") +
	`<div class="app-sb-group">Источники и ИИ</div>
` + navItem("/ai/models", icoCpu, "ИИ-модели", "Провайдеры и ключи LLM (Qwen, OpenAI, Anthropic, Custom)") +
	navItem("/ai/agents", icoBot, "ИИ-агенты", "Агенты: Консультант, Валидатор, Монитор, Аннотатор и др.") +
	navItem("/proxy", icoShield, "Прокси / VPN", "Обход WAF dochub: без рабочего прокси файлы не качаются (403)") +
	navItem("/regulations", icoBook, "Льготы и НПА", "Реестр льгот и нормативных актов с источниками") +
	`</nav>
<div class="app-sb-foot">
<button class="sb-theme" onclick="sbToggleTheme()" data-tip="Светлая / тёмная тема" aria-label="Сменить тему">` + icoMoon + icoSun + `</button>
<a class="sb-logout" href="/logout" data-tip="Выйти из админ-панели">` + icoLogout + `<span>Выход</span></a>
</div>
</aside>
<button class="app-sb-mobile" onclick="sbToggleMobile()" aria-label="Открыть меню">` + icoMenu + `</button>
<div class="app-sb-backdrop" onclick="sbCloseMobile()"></div>`

// navResidency — пункты левого меню админки РЕЗИДЕНТСТВА (:8091).
// HTTP Basic Auth, поэтому «Выхода» нет; ссылка «ИИ-модели» ведёт в общую ИИ-конфигурацию.
var navResidency = `<aside class="app-sidebar">
<a class="app-sb-brand" href="/clients" data-tip="К списку клиентов"><span class="logo-i">` + icoUsers + `</span><span class="b-txt"><b>Резидентство</b><span>Админ менеджера</span></span></a>
<button class="app-sb-toggle" onclick="sbToggleCollapse()" data-tip="Свернуть / развернуть меню" aria-label="Свернуть меню">` + icoChevron + `</button>
<nav class="app-sb-nav">
<div class="app-sb-group">Резиденты</div>
` + navItem("/clients", icoUsers, "Клиенты", "Карточки клиентов: стадия, контакты, история переходов") +
	navItem("/checklists", icoCheck, "Чек-листы", "Чек-листы процедур: Вступление, Отчётность, Продление, Выход") +
	navItem("/deadlines", icoClock, "Дедлайны", "Дашборд дедлайнов: просроченные, критичные, предстоящие") +
	`<div class="app-sb-group">Каталоги</div>
` + navItem("/templates", icoTemplate, "Шаблоны", "Шаблоны документов для генерации (PDF / DOCX)") +
	navItem("/tenants", icoBriefcase, "Тенанты", "Организации-партнёры и их API-ключи") +
	navItem("/events-admin", icoCalendar, "Мероприятия", "Мероприятия Сколково из парсинга") +
	navItem("/contests-admin", icoAward, "Конкурсы", "Конкурсы и гранты из парсинга") +
	`<div class="app-sb-group">Настройки</div>
` + navItem("/ai/models", icoCpu, "ИИ-модели", "Общая настройка LLM-провайдеров и агентов") +
	`</nav>
<div class="app-sb-foot">
<button class="sb-theme" onclick="sbToggleTheme()" data-tip="Светлая / тёмная тема" aria-label="Сменить тему">` + icoMoon + icoSun + `</button>
</div>
</aside>
<button class="app-sb-mobile" onclick="sbToggleMobile()" aria-label="Открыть меню">` + icoMenu + `</button>
<div class="app-sb-backdrop" onclick="sbCloseMobile()"></div>`

// sidebarDefine оборачивает разметку меню в именованный шаблон "sidebar"
// (вместе со стилями и скриптом каркаса), который страница вызывает как
// {{template "sidebar" .}}. Меню статично, поэтому переданные данные не используются.
func sidebarDefine(nav string) string {
	return `{{define "sidebar"}}` + appShellCollapseRestore +
		"<style>" + appSidebarCSS + "</style>" +
		nav +
		"<script>" + appSidebarScript + "</script>" +
		`{{end}}`
}

// sidebarMainHTML / sidebarResidencyHTML — готовая разметка каркаса для страниц,
// которые рендерятся без html/template (например, страница прокси собирается строкой).
var sidebarMainHTML = appShellCollapseRestore + "<style>" + appSidebarCSS + "</style>" + navMain + "<script>" + appSidebarScript + "</script>"

// sidebarMainDefine / sidebarResidencyDefine подмешиваются в .Parse(...) каждого
// инстанса шаблонов соответствующей админки.
var sidebarMainDefine = sidebarDefine(navMain)
var sidebarResidencyDefine = sidebarDefine(navResidency)
