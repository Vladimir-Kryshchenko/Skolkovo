package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"baza-skolkovo/src/aimodels"
)

// WithAIStore подключает хранилище ИИ-моделей к административному серверу.
func (s *Server) WithAIStore(st *aimodels.Store) *Server {
	s.aiStore = st
	return s
}

// ─── шаблоны ─────────────────────────────────────────────────────────────────

var aiTmpl = template.Must(template.New("ai").Funcs(template.FuncMap{
	"maskKey": func(k string) string {
		if len(k) <= 8 {
			return strings.Repeat("*", len(k))
		}
		return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
	},
	"providerLabel": func(p string) string {
		return aimodels.Provider(p).Label()
	},
	"agentTypeLabel": func(t string) string {
		return aimodels.AgentType(t).Label()
	},
	"formatTime": func(t time.Time) string {
		return t.Format("02.01.2006 15:04")
	},
	"truncate": func(s string, n int) string {
		if len([]rune(s)) <= n {
			return s
		}
		return string([]rune(s)[:n]) + "…"
	},
}).Parse(`
{{define "ai-layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — ИИ Конфигурация</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f5f6f8; --surface: #ffffff; --surface-alt: #f0f4f8; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e6f0fa; --text: #1a1d2e; --text-secondary: #5f6577;
  --border: #d5d9e2; --radius: 8px; --shadow: 0 1px 3px rgba(0,0,0,.06), 0 1px 2px rgba(0,0,0,.04);
  --shadow-lg: 0 8px 24px rgba(0,0,0,.08);
  --green: #00875a; --green-bg: #e3fcef; --yellow: #ff991f; --yellow-bg: #fff7e6;
  --red: #de350b; --red-bg: #ffeae6; --blue: #0073ea; --purple: #6554c0; --purple-bg: #eae6ff;
  --gray: #6b778c; --gray-bg: #f4f5f7;
  --header-bg: #ffffff; --header-text: #1a1d2e; --card-border: 1px solid #d5d9e2;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #4c9aff; --primary-hover: #6db3f8;
    --primary-light: #1e3a5f; --text: #e8eaf0; --text-secondary: #a0a5b8;
    --border: #3a3f56; --shadow: 0 1px 3px rgba(0,0,0,.3); --shadow-lg: 0 8px 24px rgba(0,0,0,.4);
    --green: #36b37e; --green-bg: #1b3a2a; --yellow: #ff991f; --yellow-bg: #3a2a0a;
    --red: #ff5630; --red-bg: #3a1510; --blue: #4c9aff; --purple: #998dd9; --purple-bg: #2d2450;
    --gray: #a0a5b8; --gray-bg: #2a2f45;
    --header-bg: #23273a; --header-text: #e8eaf0; --card-border: 1px solid #3a3f56;
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #4c9aff; --primary-hover: #6db3f8;
  --primary-light: #1e3a5f; --text: #e8eaf0; --text-secondary: #a0a5b8;
  --border: #3a3f56; --shadow: 0 1px 3px rgba(0,0,0,.3); --shadow-lg: 0 8px 24px rgba(0,0,0,.4);
  --green: #36b37e; --green-bg: #1b3a2a; --yellow: #ff991f; --yellow-bg: #3a2a0a;
  --red: #ff5630; --red-bg: #3a1510; --blue: #4c9aff; --purple: #998dd9; --purple-bg: #2d2450;
  --gray: #a0a5b8; --gray-bg: #2a2f45;
  --header-bg: #23273a; --header-text: #e8eaf0; --card-border: 1px solid #3a3f56;
}
body { font-family: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; font-size: 14px; }

/* ── Header ── */
header { background: var(--header-bg); color: var(--header-text); padding: 12px 28px; display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; border-bottom: var(--card-border); position: sticky; top: 0; z-index: 100; box-shadow: var(--shadow); }
header h1 { font-size: 17px; font-weight: 700; display: flex; align-items: center; gap: 8px; color: var(--header-text); }

/* ── Tooltip ── */
[data-tooltip] { position: relative; }
[data-tooltip]::after { content: attr(data-tooltip); position: absolute; bottom: calc(100% + 6px); left: 50%; transform: translateX(-50%); background: #1a1d2e; color: #fff; font-size: 11px; font-weight: 400; padding: 4px 8px; border-radius: 4px; white-space: nowrap; pointer-events: none; opacity: 0; transition: opacity .15s; z-index: 200; }
[data-tooltip]:hover::after { opacity: 1; }
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) [data-tooltip]::after { background: #e8eaf0; color: #1a1d2e; } }
:root[data-theme="dark"] [data-tooltip]::after { background: #e8eaf0; color: #1a1d2e; }

/* ── Nav ── */
.nav-btn { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); border-radius: 6px; padding: 6px 14px; font-size: 13px; font-weight: 500; cursor: pointer; transition: all .15s; text-decoration: none; display: inline-flex; align-items: center; gap: 6px; font-family: inherit; }
.nav-btn:hover { background: var(--primary-light); color: var(--primary); border-color: var(--primary); }
.nav-btn.active { background: var(--primary); color: #fff; border-color: var(--primary); }

/* ── Theme toggle ── */
#themeBtn { font-size: 18px; min-width: 36px; cursor: pointer; border: 1px solid var(--border); background: var(--surface); border-radius: 6px; padding: 6px; display: inline-flex; align-items: center; justify-content: center; transition: all .15s; color: var(--text); }
#themeBtn:hover { border-color: var(--primary); color: var(--primary); }

/* ── Main ── */
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }
@media(max-width: 768px) { main { padding: 16px; } .form-row { grid-template-columns: 1fr; } }

/* ── Tabs ── */
.tabs { display: flex; gap: 4px; margin-bottom: 20px; background: var(--surface); border-radius: var(--radius); padding: 6px; box-shadow: var(--shadow); width: fit-content; border: var(--card-border); }
.tab { padding: 8px 20px; border-radius: 6px; font-size: 13px; font-weight: 500; cursor: pointer; text-decoration: none; color: var(--text-secondary); transition: all .15s; }
.tab:hover { background: var(--primary-light); color: var(--primary); }
.tab.active { background: var(--primary); color: #fff; }

/* ── Cards ── */
.card { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow); overflow: hidden; border: var(--card-border); }
.card-header { padding: 16px 20px; border-bottom: var(--card-border); display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.card-header h2 { font-size: 15px; font-weight: 600; }
.card-body { padding: 20px; }

/* ── Table ── */
table { width: 100%; border-collapse: collapse; table-layout: fixed; }
thead th { background: var(--surface-alt); padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; border-bottom: 2px solid var(--border); }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; vertical-align: middle; word-break: break-word; overflow-wrap: break-word; }
tbody tr:hover { background: var(--surface-alt); }
tbody tr:last-child td { border-bottom: none; }

/* ── Badge ── */
.badge { display: inline-block; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; }
.badge-green { background: var(--green-bg); color: var(--green); }
.badge-gray { background: var(--gray-bg); color: var(--gray); }
.badge-blue { background: var(--primary-light); color: var(--primary); }
.badge-purple { background: var(--purple-bg); color: var(--purple); }
.badge-yellow { background: var(--yellow-bg); color: var(--yellow); }

/* ── Buttons ── */
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 4px; padding: 6px 14px; border: none; border-radius: 6px; font-size: 12px; font-weight: 500; cursor: pointer; transition: all .15s; white-space: nowrap; font-family: inherit; text-decoration: none; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-success { background: var(--green); color: #fff; }
.btn-success:hover { background: #006644; }
.btn-danger { background: var(--red); color: #fff; }
.btn-danger:hover { background: #bf2600; }
.btn-secondary { background: var(--gray-bg); color: var(--text); border: 1px solid var(--border); }
.btn-secondary:hover { background: var(--border); }
.btn-sm { padding: 4px 10px; font-size: 11px; }
.btn-test { background: var(--purple-bg); color: var(--purple); border: 1px solid #c3bdf0; }
.btn-test:hover { background: #ddd6fe; }

/* ── Forms ── */
.form-group { margin-bottom: 16px; }
.form-group label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 6px; color: var(--text); }
.form-group input, .form-group select, .form-group textarea {
  width: 100%; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px;
  font-size: 13px; font-family: inherit; outline: none; transition: border-color .15s, box-shadow .15s;
  background: var(--surface); color: var(--text);
}
.form-group input:focus, .form-group select:focus, .form-group textarea:focus {
  border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.15);
}
.form-group textarea { resize: vertical; min-height: 120px; }
.form-group .hint { font-size: 11px; color: var(--text-secondary); margin-top: 4px; }
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.form-actions { display: flex; gap: 10px; margin-top: 20px; padding-top: 20px; border-top: 1px solid var(--border); }

/* ── Flash ── */
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; display: flex; align-items: center; gap: 8px; }
.flash-ok { background: var(--green-bg); color: var(--green); border: 1px solid #abf5d1; }
.flash-err { background: var(--red-bg); color: var(--red); border: 1px solid #ffbdad; }

/* ── Provider icon (text initials) ── */
.provider-icon { width: 28px; height: 28px; border-radius: 6px; display: inline-flex; align-items: center; justify-content: center; font-size: 10px; font-weight: 700; flex-shrink: 0; color: #fff; letter-spacing: .3px; }
.pi-alibaba { background: #ff6a00; }
.pi-openai { background: #1a1d2e; }
.pi-anthropic { background: #d97706; }
.pi-custom { background: #6554c0; }

/* ── Test area ── */
.test-area { margin-top: 24px; background: var(--surface-alt); border: var(--card-border); border-radius: var(--radius); padding: 16px; }
.test-area h3 { font-size: 13px; font-weight: 600; margin-bottom: 12px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }
.test-input { display: flex; gap: 8px; align-items: flex-start; }
.test-input textarea { flex: 1; min-height: 80px; }
.test-result { margin-top: 12px; background: var(--surface); border: var(--card-border); border-radius: 6px; padding: 14px; font-size: 13px; min-height: 60px; white-space: pre-wrap; line-height: 1.6; color: var(--text); }
.test-result.loading { color: var(--text-secondary); font-style: italic; }
.test-result.success { border-left: 3px solid var(--green); }
.test-result.error { border-left: 3px solid var(--red); color: var(--red); }
.test-meta { font-size: 11px; color: var(--text-secondary); margin-top: 8px; }

/* ── Stats ── */
.stats-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-bottom: 24px; }
.stat-card { background: var(--surface); border-radius: var(--radius); padding: 16px; box-shadow: var(--shadow); text-align: center; border: var(--card-border); }
.stat-card .n { font-size: 28px; font-weight: 700; line-height: 1.1; }
.stat-card .l { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-top: 4px; }
.stat-card.blue { border-left: 3px solid var(--blue); }
.stat-card.blue .n { color: var(--blue); }
.stat-card.green { border-left: 3px solid var(--green); }
.stat-card.green .n { color: var(--green); }
.stat-card.purple { border-left: 3px solid var(--purple); }
.stat-card.purple .n { color: var(--purple); }
.stat-card.yellow { border-left: 3px solid var(--yellow); }
.stat-card.yellow .n { color: var(--yellow); }

/* ── Misc ── */
.prompt-preview { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 12px; background: var(--surface-alt); border: 1px solid var(--border); border-radius: 4px; padding: 8px; max-height: 80px; overflow: hidden; position: relative; cursor: pointer; color: var(--text); }
.prompt-preview::after { content: ''; position: absolute; bottom: 0; left: 0; right: 0; height: 24px; background: linear-gradient(transparent, var(--surface-alt)); }
.key-display { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 12px; color: var(--text-secondary); }
.actions-cell { display: flex; gap: 4px; flex-wrap: wrap; }

/* ── Responsive ── */
@media(max-width: 1024px) {
  main { padding: 20px; }
  .stats-row { grid-template-columns: repeat(3, 1fr); }
}
@media(max-width: 768px) {
  .form-row { grid-template-columns: 1fr; }
  main { padding: 16px; }
  header { padding: 12px 16px; }
  .stats-row { grid-template-columns: repeat(2, 1fr); }
  table { font-size: 12px; }
  thead th, tbody td { padding: 8px 10px; }
}
@media(max-width: 480px) {
  header { flex-direction: column; align-items: flex-start; }
  header > div { width: 100%; flex-wrap: wrap; }
  .stats-row { grid-template-columns: 1fr; }
  .actions-cell { flex-direction: column; }
  .nav-btn { font-size: 12px; padding: 5px 10px; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<header>
  <h1>
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a4 4 0 0 1 4 4c0 1.1-.9 2-2 2h-4a2 2 0 0 1-2-2 4 4 0 0 1 4-4z"/><path d="M12 8v4"/><circle cx="12" cy="16" r="4"/><path d="M8 16h8"/><path d="M10 20h4"/></svg>
    База Сколково — ИИ Конфигурация
  </h1>
  <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center">
    <a href="/" class="nav-btn" data-tooltip="Вернуться к документам">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><polyline points="12 19 5 12 12 5"/></svg>
      Документы
    </a>
    <a href="/clients" class="nav-btn" data-tooltip="Управление клиентами резидентства">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
      Клиенты
    </a>
    <a href="/ai/models" class="nav-btn{{if eq .Tab "models"}} active{{end}}" data-tooltip="ИИ-модели: API-ключи, провайдеры, параметры">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
      Модели
    </a>
    <a href="/ai/agents" class="nav-btn{{if eq .Tab "agents"}} active{{end}}" data-tooltip="ИИ-агенты: промпты, типы, настройки">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83"/></svg>
      Агенты
    </a>
    <button id="themeBtn" onclick="toggleTheme()" data-tooltip="Переключить тему" class="nav-btn" style="font-size:16px;min-width:36px;cursor:pointer;border:1px solid var(--border);padding:6px">
      <!-- sun icon (shown in dark) -->
      <svg class="theme-icon-sun" style="display:none" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
      <!-- moon icon (shown in light) -->
      <svg class="theme-icon-moon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
    </button>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}
{{template "ai-content" .}}
</main>
<script>
function testModel(modelId) {
  var msg = document.getElementById('test-msg-'+modelId).value.trim();
  if (!msg) { alert('Введите сообщение для теста'); return; }
  var resultEl = document.getElementById('test-result-'+modelId);
  var metaEl = document.getElementById('test-meta-'+modelId);
  var btn = document.getElementById('test-btn-'+modelId);
  resultEl.textContent = 'Отправляю запрос\u2026';
  resultEl.className = 'test-result loading';
  btn.disabled = true;
  var t0 = Date.now();
  fetch('/api/ai/models/'+modelId+'/test', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(function(r){return r.json()}).then(function(d){
    var ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = 'Ошибка: '+d.error;
      resultEl.className = 'test-result error';
      metaEl.textContent = '';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result success';
      metaEl.textContent = 'Время: '+ms+'мс \u00B7 Токены: '+(d.tokens||'\u2014');
    }
  }).catch(function(e){
    resultEl.textContent = 'Ошибка: '+e.message;
    resultEl.className = 'test-result error';
  }).finally(function(){ btn.disabled=false; });
}
function testAgent(agentId) {
  var msg = document.getElementById('test-msg-agent-'+agentId).value.trim();
  if (!msg) { alert('Введите сообщение для теста'); return; }
  var resultEl = document.getElementById('test-result-agent-'+agentId);
  var metaEl = document.getElementById('test-meta-agent-'+agentId);
  var btn = document.getElementById('test-btn-agent-'+agentId);
  resultEl.textContent = 'Запускаю агента\u2026';
  resultEl.className = 'test-result loading';
  btn.disabled = true;
  var t0 = Date.now();
  fetch('/api/ai/agents/'+agentId+'/test', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(function(r){return r.json()}).then(function(d){
    var ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = 'Ошибка: '+d.error;
      resultEl.className = 'test-result error';
      metaEl.textContent = '';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result success';
      metaEl.textContent = 'Время: '+ms+'мс \u00B7 Токены: '+(d.tokens||'\u2014')+' \u00B7 Модель: '+(d.model||'\u2014');
    }
  }).catch(function(e){
    resultEl.textContent = 'Ошибка: '+e.message;
    resultEl.className = 'test-result error';
  }).finally(function(){ btn.disabled=false; });
}
function confirmDelete(url, name) {
  if (confirm('Удалить \u00AB'+name+'\u00BB?')) {
    fetch(url, {method:'POST'}).then(function(r){
      if (r.ok) location.reload(); else alert('Ошибка удаления');
    });
  }
}
function seedQwen() {
  var key = document.getElementById('qwen-key').value.trim();
  if (!key) { alert('Введите API-ключ'); return; }
  if (!confirm('Добавить все модели Qwen с этим ключом?')) return;
  fetch('/api/ai/models/seed-qwen', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({api_key:key})
  }).then(function(r){return r.json()}).then(function(d){
    if (d.error) alert('Ошибка: '+d.error);
    else { alert('Добавлено '+d.count+' моделей Qwen'); location.reload(); }
  });
}
function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  updateThemeIcons(next);
}
function updateThemeIcons(theme) {
  var sun = document.querySelector('.theme-icon-sun');
  var moon = document.querySelector('.theme-icon-moon');
  if (!sun || !moon) return;
  if (theme === 'dark') { sun.style.display=''; moon.style.display='none'; }
  else { sun.style.display='none'; moon.style.display=''; }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  updateThemeIcons(cur);
});
</script>
</body>
</html>
{{end}}

{{define "ai-models-list"}}
{{template "ai-layout" .}}
{{end}}

{{define "ai-content"}}
{{end}}
`))

// Отдельные шаблоны контента
var aiModelsTmpl = template.Must(template.New("ai-models").Funcs(template.FuncMap{
	"maskKey": func(k string) string {
		if len(k) <= 8 {
			return strings.Repeat("*", len(k))
		}
		return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
	},
	"providerLabel":  func(p string) string { return aimodels.Provider(p).Label() },
	"agentTypeLabel": func(t string) string { return aimodels.AgentType(t).Label() },
	"formatTime":     func(t time.Time) string { return t.Format("02.01.2006 15:04") },
	"truncate": func(s string, n int) string {
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		return string(r[:n]) + "…"
	},
	"providerClass": func(p string) string {
		switch p {
		case "alibabacloud":
			return "pi-alibaba"
		case "openai":
			return "pi-openai"
		case "anthropic":
			return "pi-anthropic"
		default:
			return "pi-custom"
		}
	},
	"string": func(v interface{}) string { return fmt.Sprintf("%s", v) },
	"providerShort": func(p string) string {
		switch p {
		case "alibabacloud":
			return "AK"
		case "openai":
			return "OA"
		case "anthropic":
			return "AN"
		default:
			return "CU"
		}
	},
	"badgeClass": func(p string) string {
		switch p {
		case "alibabacloud":
			return "badge-yellow"
		case "openai":
			return "badge-blue"
		case "anthropic":
			return "badge-purple"
		default:
			return "badge-gray"
		}
	},
}).Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ИИ Модели — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f5f6f8;--surface:#fff;--primary:#0073ea;--primary-hover:#005bb5;--primary-light:#e6f0fa;--text:#1a1d2e;--text-secondary:#5f6577;--border:#d5d9e2;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.06),0 1px 2px rgba(0,0,0,.04);--shadow-lg:0 8px 24px rgba(0,0,0,.08);--green:#00875a;--green-bg:#e3fcef;--yellow:#ff991f;--yellow-bg:#fff7e6;--red:#de350b;--red-bg:#ffeae6;--blue:#0073ea;--purple:#6554c0;--purple-bg:#eae6ff;--gray:#6b778c;--gray-bg:#f4f5f7;--header-bg:#fff;--card-border:1px solid #d5d9e2}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#181b2b;--surface:#23273a;--surface-alt:#2a2f45;--primary:#4c9aff;--primary-hover:#6db3f8;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--shadow:0 1px 3px rgba(0,0,0,.3);--green:#36b37e;--green-bg:#1b3a2a;--yellow:#ff991f;--yellow-bg:#3a2a0a;--red:#ff5630;--red-bg:#3a1510;--blue:#4c9aff;--purple:#998dd9;--purple-bg:#2d2450;--gray:#a0a5b8;--gray-bg:#2a2f45;--card-border:1px solid #3a3f56}}
:root[data-theme="dark"]{--bg:#181b2b;--surface:#23273a;--surface-alt:#2a2f45;--primary:#4c9aff;--primary-hover:#6db3f8;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--shadow:0 1px 3px rgba(0,0,0,.3);--green:#36b37e;--green-bg:#1b3a2a;--yellow:#ff991f;--yellow-bg:#3a2a0a;--red:#ff5630;--red-bg:#3a1510;--blue:#4c9aff;--purple:#998dd9;--purple-bg:#2d2450;--gray:#a0a5b8;--gray-bg:#2a2f45;--card-border:1px solid #3a3f56}
body{font-family:'Figtree',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}

/* Header */
header{background:var(--header-bg);color:var(--text);padding:12px 28px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:12px;border-bottom:var(--card-border);box-shadow:var(--shadow);position:sticky;top:0;z-index:100}
header h1{font-size:17px;font-weight:700;display:flex;align-items:center;gap:8px}

/* Tooltip */
[data-tooltip]{position:relative}
[data-tooltip]::after{content:attr(data-tooltip);position:absolute;bottom:calc(100% + 6px);left:50%;transform:translateX(-50%);background:#1a1d2e;color:#fff;font-size:11px;font-weight:400;padding:4px 8px;border-radius:4px;white-space:nowrap;pointer-events:none;opacity:0;transition:opacity .15s;z-index:200}
[data-tooltip]:hover::after{opacity:1}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"])[data-tooltip]::after{background:#e8eaf0;color:#1a1d2e}}
:root[data-theme="dark"][data-tooltip]::after{background:#e8eaf0;color:#1a1d2e}

/* Nav */
.nav-btn{background:transparent;color:var(--text-secondary);border:1px solid var(--border);border-radius:6px;padding:6px 14px;font-size:13px;font-weight:500;cursor:pointer;transition:all .15s;text-decoration:none;display:inline-flex;align-items:center;gap:6px;font-family:inherit}
.nav-btn:hover{background:var(--primary-light);color:var(--primary);border-color:var(--primary)}
.nav-btn.active{background:var(--primary);color:#fff;border-color:var(--primary)}

main{max-width:1400px;margin:0 auto;padding:24px 28px}
@media(max-width:768px){main{padding:16px}}
@media(max-width:480px){header{flex-direction:column;align-items:flex-start}.nav-btn{font-size:12px;padding:5px 10px}}

/* Stats */
.stats-row{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px;margin-bottom:24px}
.stat-card{background:var(--surface);border-radius:var(--radius);padding:16px;box-shadow:var(--shadow);text-align:center;border:var(--card-border)}
.stat-card .n{font-size:28px;font-weight:700;line-height:1.1}
.stat-card .l{font-size:11px;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-top:4px}
.stat-card.blue{border-left:3px solid var(--blue)}.stat-card.blue .n{color:var(--blue)}
.stat-card.green{border-left:3px solid var(--green)}.stat-card.green .n{color:var(--green)}
.stat-card.yellow{border-left:3px solid var(--yellow)}.stat-card.yellow .n{color:var(--yellow)}

/* Cards */
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden;margin-bottom:20px;border:var(--card-border)}
.card-header{padding:16px 20px;border-bottom:var(--card-border);display:flex;align-items:center;justify-content:space-between;gap:12px}
.card-header h2{font-size:15px;font-weight:600}
.card-body{padding:20px}

/* Table */
table{width:100%;border-collapse:collapse;table-layout:fixed}
thead th{background:var(--surface-alt);padding:10px 14px;text-align:left;font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;border-bottom:2px solid var(--border)}
tbody td{padding:12px 14px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:middle;word-break:break-word;overflow-wrap:break-word}
tbody tr:hover{background:var(--surface-alt)}
tbody tr:last-child td{border-bottom:none}

/* Badge */
.badge{display:inline-block;padding:3px 10px;border-radius:20px;font-size:11px;font-weight:600}
.badge-green{background:var(--green-bg);color:var(--green)}
.badge-gray{background:var(--gray-bg);color:var(--gray)}
.badge-blue{background:var(--primary-light);color:var(--primary)}
.badge-purple{background:var(--purple-bg);color:var(--purple)}
.badge-yellow{background:var(--yellow-bg);color:var(--yellow)}

/* Buttons */
.btn{display:inline-flex;align-items:center;justify-content:center;gap:4px;padding:6px 14px;border:none;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;transition:all .15s;white-space:nowrap;font-family:inherit;text-decoration:none}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-success{background:var(--green);color:#fff}.btn-success:hover{background:#006644}
.btn-danger{background:var(--red);color:#fff}.btn-danger:hover{background:#bf2600}
.btn-secondary{background:var(--gray-bg);color:var(--text);border:1px solid var(--border)}.btn-secondary:hover{background:var(--border)}
.btn-test{background:var(--purple-bg);color:var(--purple);border:1px solid #c3bdf0}.btn-test:hover{background:#ddd6fe}
.btn-sm{padding:4px 10px;font-size:11px}
.actions-cell{display:flex;gap:4px;flex-wrap:wrap}

/* Provider icon */
.provider-icon{width:28px;height:28px;border-radius:6px;display:inline-flex;align-items:center;justify-content:center;font-size:10px;font-weight:700;flex-shrink:0;margin-right:4px;color:#fff;letter-spacing:.3px}
.pi-alibaba{background:#ff6a00}.pi-openai{background:#1a1d2e}
.pi-anthropic{background:#d97706}.pi-custom{background:#6554c0}

.key-mono{font-family:'SF Mono','Fira Code',monospace;font-size:12px;color:var(--text-secondary)}

/* Flash */
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500;display:flex;align-items:center;gap:8px}
.flash-ok{background:var(--green-bg);color:var(--green);border:1px solid #abf5d1}
.flash-err{background:var(--red-bg);color:var(--red);border:1px solid #ffbdad}

/* Test panel */
.test-panel{background:var(--surface-alt);border:var(--card-border);border-radius:var(--radius);padding:16px;margin-top:12px}
.test-panel h4{font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:10px}
.test-input{display:flex;gap:8px}
.test-input input{flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none;color:var(--text);background:var(--surface)}
.test-input input:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(0,115,234,.15)}
.test-result{margin-top:10px;background:var(--surface);border:var(--card-border);border-radius:6px;padding:12px;font-size:13px;min-height:50px;white-space:pre-wrap;line-height:1.6;color:var(--text);display:none}
.test-result.visible{display:block}
.test-result.loading{color:var(--text-secondary);font-style:italic}
.test-result.ok{border-left:3px solid var(--green)}
.test-result.err{border-left:3px solid var(--red);color:var(--red)}
.test-meta{font-size:11px;color:var(--text-secondary);margin-top:6px}

/* Seed box */
.seed-box{background:var(--surface-alt);border:var(--card-border);border-radius:var(--radius);padding:20px;margin-bottom:20px}
.seed-box h3{font-size:14px;font-weight:600;color:var(--text);margin-bottom:8px;display:flex;align-items:center;gap:8px}
.seed-box p{font-size:13px;color:var(--text-secondary);margin-bottom:12px}
.seed-input{display:flex;gap:8px}
.seed-input input{flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none;color:var(--text);background:var(--surface)}
.seed-input input:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(0,115,234,.15)}

.model-name-cell{display:flex;align-items:center;gap:8px}
.model-desc{font-size:11px;color:var(--text-secondary);margin-top:2px}
tr.disabled-row td{opacity:.55}

/* Responsive */
@media(max-width:768px){.form-row{grid-template-columns:1fr}table{font-size:12px}thead th,tbody td{padding:8px 10px}}
@media(max-width:480px){.stats-row{grid-template-columns:1fr}.actions-cell{flex-direction:column}}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})()</script>
</head>
<body>
<header>
  <h1>
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
    ИИ Конфигурация
  </h1>
  <div style="display:flex;gap:8px;flex-wrap:wrap">
    <a href="/" class="nav-btn" data-tooltip="Вернуться к документам">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><polyline points="12 19 5 12 12 5"/></svg>
      Документы
    </a>
    <a href="/ai/models" class="nav-btn active" data-tooltip="Управление ИИ-моделями">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
      Модели
    </a>
    <a href="/ai/agents" class="nav-btn" data-tooltip="Управление ИИ-агентами">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4"/></svg>
      Агенты
    </a>
    <a href="/ai/models/new" class="btn btn-primary" data-tooltip="Добавить новую модель">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      Добавить модель
    </a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}

{{if not .Models}}
<div class="seed-box">
  <h3>
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
    Быстрое подключение моделей Qwen (Alibaba Cloud)
  </h3>
  <p>Добавить все доступные модели Qwen автоматически — вставьте API-ключ от Alibaba Cloud Model Studio:</p>
  <div class="seed-input">
    <input type="password" id="qwen-key" placeholder="sk-..." autocomplete="off">
    <button class="btn btn-success" onclick="seedQwen()" data-tooltip="Импортировать модели Qwen">Импортировать Qwen модели</button>
  </div>
</div>
{{end}}

<div class="stats-row">
  <div class="stat-card blue"><div class="n">{{len .Models}}</div><div class="l">Всего моделей</div></div>
  <div class="stat-card green"><div class="n">{{.EnabledCount}}</div><div class="l">Активные</div></div>
  <div class="stat-card yellow"><div class="n">{{.ProviderCount}}</div><div class="l">Провайдеров</div></div>
</div>

<div class="card">
  <div class="card-header">
    <h2>Зарегистрированные модели</h2>
    <a href="/ai/models/new" class="btn btn-primary" data-tooltip="Добавить новую модель">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      Добавить модель
    </a>
  </div>
  {{if .Models}}
  <div style="overflow-x:auto">
  <table>
    <thead><tr>
      <th>Модель</th>
      <th>Провайдер</th>
      <th>API Model ID</th>
      <th>API-ключ</th>
      <th>Параметры</th>
      <th>Статус</th>
      <th>Действия</th>
    </tr></thead>
    <tbody>
    {{range .Models}}
    <tr{{if not .Enabled}} class="disabled-row"{{end}}>
      <td>
        <div class="model-name-cell">
          <span class="provider-icon {{providerClass (string .Provider)}}">{{providerShort (string .Provider)}}</span>
          <div>
            <div style="font-weight:600">{{.Name}}</div>
            {{if .Description}}<div class="model-desc">{{truncate .Description 60}}</div>{{end}}
          </div>
        </div>
      </td>
      <td><span class="badge {{badgeClass (string .Provider)}}">{{providerLabel (string .Provider)}}</span></td>
      <td><code style="font-size:12px;background:var(--surface-alt);padding:2px 6px;border-radius:4px">{{.ModelID}}</code></td>
      <td><span class="key-mono">{{maskKey .APIKey}}</span></td>
      <td style="font-size:12px">T={{.Temperature}} · {{.MaxTokens}}tok</td>
      <td>
        {{if .Enabled}}<span class="badge badge-green">Активна</span>{{else}}<span class="badge badge-gray">Отключена</span>{{end}}
      </td>
      <td>
        <div class="actions-cell">
          <a href="/ai/models/{{.ID}}/edit" class="btn btn-secondary btn-sm" data-tooltip="Редактировать модель">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            Изменить
          </a>
          <button class="btn btn-test btn-sm" onclick="toggleTest('{{.ID}}')" data-tooltip="Протестировать модель">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
            Тест
          </button>
          <button class="btn btn-danger btn-sm" onclick="confirmDelete('/api/ai/models/{{.ID}}/delete','{{.Name}}')" data-tooltip="Удалить модель">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            Удалить
          </button>
        </div>
        <div id="test-{{.ID}}" style="display:none">
          <div class="test-panel">
            <h4>Тест модели</h4>
            <div class="test-input">
              <input type="text" id="test-msg-{{.ID}}" placeholder="Введите тестовое сообщение…" onkeydown="if(event.key==='Enter')testModel('{{.ID}}')">
              <button class="btn btn-test" id="test-btn-{{.ID}}" onclick="testModel('{{.ID}}')" data-tooltip="Отправить запрос">Отправить</button>
            </div>
            <div id="test-result-{{.ID}}" class="test-result"></div>
            <div id="test-meta-{{.ID}}" class="test-meta"></div>
          </div>
        </div>
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
  </div>
  {{else}}
  <div style="padding:40px;text-align:center;color:var(--text-secondary)">
    <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="margin-bottom:12px;opacity:.5"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
    <div style="font-size:15px;font-weight:600;margin-bottom:8px">Модели не настроены</div>
    <div style="font-size:13px;margin-bottom:20px">Добавьте модели вручную или используйте быстрый импорт Qwen выше</div>
    <a href="/ai/models/new" class="btn btn-primary">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      Добавить первую модель
    </a>
  </div>
  {{end}}
</div>
</main>
<script>
function toggleTest(id) {
  var el = document.getElementById('test-'+id);
  el.style.display = el.style.display === 'none' ? 'block' : 'none';
  if (el.style.display === 'block') document.getElementById('test-msg-'+id).focus();
}
function testModel(id) {
  var msg = document.getElementById('test-msg-'+id).value.trim();
  if (!msg) { alert('Введите сообщение'); return; }
  var resultEl = document.getElementById('test-result-'+id);
  var metaEl = document.getElementById('test-meta-'+id);
  var btn = document.getElementById('test-btn-'+id);
  resultEl.textContent = 'Отправляю запрос\u2026';
  resultEl.className = 'test-result visible loading';
  metaEl.textContent = '';
  btn.disabled = true;
  var t0 = Date.now();
  fetch('/api/ai/models/'+id+'/test', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(function(r){return r.json()}).then(function(d){
    var ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = 'Ошибка: '+d.error;
      resultEl.className = 'test-result visible err';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result visible ok';
      metaEl.textContent = 'Время: '+ms+'мс  Токены: '+(d.tokens||'\u2014');
    }
  }).catch(function(e){
    resultEl.textContent = 'Ошибка: '+e.message;
    resultEl.className = 'test-result visible err';
  }).finally(function(){ btn.disabled=false; });
}
function confirmDelete(url, name) {
  if (!confirm('Удалить модель \u00AB'+name+'\u00BB?')) return;
  fetch(url, {method:'POST'}).then(function(r){
    if (r.redirected) location.href=r.url;
    else if (r.ok) location.reload();
    else r.text().then(function(t){alert('Ошибка: '+t)});
  });
}
function seedQwen() {
  var key = document.getElementById('qwen-key').value.trim();
  if (!key) { alert('Введите API-ключ'); return; }
  if (!confirm('Добавить все модели Qwen с этим ключом?')) return;
  fetch('/api/ai/models/seed-qwen', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body:JSON.stringify({api_key:key})
  }).then(function(r){return r.json()}).then(function(d){
    if (d.error) alert('Ошибка: '+d.error);
    else { alert('Добавлено '+d.count+' моделей Qwen'); location.reload(); }
  });
}
</script>
</body>
</html>
`))

var aiModelFormTmpl = template.Must(template.New("ai-model-form").Funcs(template.FuncMap{
	"string": func(v interface{}) string { return fmt.Sprintf("%s", v) },
	"selected": func(a, b interface{}) template.HTMLAttr {
		if fmt.Sprintf("%s", a) == fmt.Sprintf("%s", b) {
			return "selected"
		}
		return ""
	},
}).Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{if .Model.ID}}Редактировать{{else}}Добавить{{end}} модель — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f5f6f8;--surface:#fff;--primary:#0073ea;--primary-hover:#005bb5;--primary-light:#e6f0fa;--text:#1a1d2e;--text-secondary:#5f6577;--border:#d5d9e2;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.06);--card-border:1px solid #d5d9e2}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#181b2b;--surface:#23273a;--primary:#4c9aff;--primary-hover:#6db3f8;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--card-border:1px solid #3a3f56}}
:root[data-theme="dark"]{--bg:#181b2b;--surface:#23273a;--primary:#4c9aff;--primary-hover:#6db3f8;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--card-border:1px solid #3a3f56}
body{font-family:'Figtree',-apple-system,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:var(--surface);color:var(--text);padding:12px 28px;display:flex;align-items:center;justify-content:space-between;box-shadow:var(--shadow);border-bottom:var(--card-border)}
header h1{font-size:17px;font-weight:700}
.nav-btn{background:transparent;color:var(--text-secondary);border:1px solid var(--border);border-radius:6px;padding:6px 14px;font-size:13px;text-decoration:none;display:inline-flex;align-items:center;gap:6px;font-family:inherit;transition:all .15s}
.nav-btn:hover{background:var(--primary-light);color:var(--primary);border-color:var(--primary)}
main{max-width:800px;margin:32px auto;padding:0 28px}
@media(max-width:768px){main{margin:16px auto;padding:0 16px}}
@media(max-width:480px){header{flex-direction:column;align-items:flex-start}}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden;border:var(--card-border)}
.card-header{padding:20px 24px;border-bottom:var(--card-border)}.card-header h2{font-size:16px;font-weight:600}
.card-body{padding:24px}
.form-group{margin-bottom:18px}
.form-group label{display:block;font-size:13px;font-weight:500;margin-bottom:6px}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:9px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none;transition:border-color .15s;color:var(--text);background:var(--surface)}
.form-group input:focus,.form-group select:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(0,115,234,.15)}
.form-group .hint{font-size:11px;color:var(--text-secondary);margin-top:4px}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
@media(max-width:768px){.form-row{grid-template-columns:1fr}}
.form-actions{display:flex;gap:10px;margin-top:24px;padding-top:20px;border-top:1px solid var(--border)}
.btn{display:inline-flex;align-items:center;padding:8px 18px;border:none;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;transition:all .15s;font-family:inherit;text-decoration:none}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-secondary{background:var(--surface-alt,#f4f5f7);color:var(--text);border:1px solid var(--border)}
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500}
.flash-err{background:#ffeae6;color:#de350b;border:1px solid #ffbdad}
.toggle-wrap{display:flex;align-items:center;gap:10px;padding:10px 12px;border:1px solid var(--border);border-radius:6px;cursor:pointer}
.toggle-wrap label{cursor:pointer;font-size:13px;font-weight:500}
[data-tooltip]{position:relative}
[data-tooltip]::after{content:attr(data-tooltip);position:absolute;bottom:calc(100% + 6px);left:50%;transform:translateX(-50%);background:#1a1d2e;color:#fff;font-size:11px;font-weight:400;padding:4px 8px;border-radius:4px;white-space:nowrap;pointer-events:none;opacity:0;transition:opacity .15s;z-index:200}
[data-tooltip]:hover::after{opacity:1}
</style>
</head>
<body>
<header>
  <h1>
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
    {{if .Model.ID}}Редактировать модель{{else}}Добавить ИИ-модель{{end}}
  </h1>
  <a href="/ai/models" class="nav-btn" data-tooltip="Вернуться к списку моделей">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><polyline points="12 19 5 12 12 5"/></svg>
    Назад
  </a>
</header>
<main>
{{if .Error}}<div class="flash flash-err">{{.Error}}</div>{{end}}
<div class="card">
  <div class="card-header"><h2>Конфигурация модели</h2></div>
  <div class="card-body">
    <form method="POST" action="{{if .Model.ID}}/ai/models/{{.Model.ID}}/update{{else}}/ai/models/create{{end}}">
      <div class="form-group">
        <label>Название *</label>
        <input type="text" name="name" value="{{.Model.Name}}" placeholder="Qwen Max" required>
        <div class="hint">Человекочитаемое название для отображения в интерфейсе</div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Провайдер *</label>
          <select name="provider" id="provider-sel" onchange="updateBaseURL(this.value)">
            <option value="alibabacloud" {{selected .Model.Provider "alibabacloud"}}>Alibaba Cloud (Qwen)</option>
            <option value="openai" {{selected .Model.Provider "openai"}}>OpenAI</option>
            <option value="anthropic" {{selected .Model.Provider "anthropic"}}>Anthropic</option>
            <option value="custom" {{selected .Model.Provider "custom"}}>Custom / Self-hosted</option>
          </select>
        </div>
        <div class="form-group">
          <label>Model ID в API *</label>
          <input type="text" name="model_id" value="{{.Model.ModelID}}" placeholder="qwen-max" required>
          <div class="hint">Идентификатор, передаваемый в поле "model" запроса</div>
        </div>
      </div>
      <div class="form-group">
        <label>Base URL API *</label>
        <input type="text" name="base_url" id="base-url" value="{{.Model.BaseURL}}" placeholder="https://dashscope-intl.aliyuncs.com/compatible-mode/v1" required>
        <div class="hint">Базовый URL OpenAI-совместимого API (без /chat/completions)</div>
      </div>
      <div class="form-group">
        <label>API-ключ *</label>
        <input type="password" name="api_key" value="{{.Model.APIKey}}" placeholder="sk-..." autocomplete="off" required>
        <div class="hint">Ключ авторизации. Хранится в базе данных.</div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Max Tokens</label>
          <input type="number" name="max_tokens" value="{{if .Model.MaxTokens}}{{.Model.MaxTokens}}{{else}}4096{{end}}" min="256" max="131072">
        </div>
        <div class="form-group">
          <label>Temperature</label>
          <input type="number" name="temperature" value="{{if .Model.Temperature}}{{.Model.Temperature}}{{else}}0.7{{end}}" min="0" max="2" step="0.1">
        </div>
      </div>
      <div class="form-group">
        <label>Описание</label>
        <input type="text" name="description" value="{{.Model.Description}}" placeholder="Краткое описание модели">
      </div>
      <div class="toggle-wrap" onclick="document.getElementById('enabled-cb').click()">
        <input type="checkbox" name="enabled" id="enabled-cb" value="true"{{if .Model.Enabled}} checked{{end}}>
        <label for="enabled-cb">Модель активна (используется агентами)</label>
      </div>
      <div class="form-actions">
        <button type="submit" class="btn btn-primary">{{if .Model.ID}}Сохранить изменения{{else}}Добавить модель{{end}}</button>
        <a href="/ai/models" class="btn btn-secondary">Отмена</a>
      </div>
    </form>
  </div>
</div>
</main>
<script>
var defaultURLs = {
  alibabacloud: 'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
  openai: 'https://api.openai.com/v1',
  anthropic: '',
  custom: ''
};
function updateBaseURL(provider) {
  var urlInput = document.getElementById('base-url');
  if (!urlInput.value || Object.values(defaultURLs).indexOf(urlInput.value) !== -1) {
    urlInput.value = defaultURLs[provider] || '';
  }
}
</script>
</body>
</html>
`))

var aiAgentsTmpl = template.Must(template.New("ai-agents").Funcs(template.FuncMap{
	"string":         func(v interface{}) string { return fmt.Sprintf("%s", v) },
	"agentTypeLabel": func(t string) string { return aimodels.AgentType(t).Label() },
	"formatTime":     func(t time.Time) string { return t.Format("02.01.2006 15:04") },
	"truncate": func(s string, n int) string {
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		return string(r[:n]) + "…"
	},
	"agentBadgeClass": func(t string) string {
		switch t {
		case "consultant":
			return "badge-blue"
		case "validator":
			return "badge-green"
		case "monitor":
			return "badge-purple"
		case "coordinator":
			return "badge-yellow"
		default:
			return "badge-gray"
		}
	},
}).Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ИИ Агенты — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f5f6f8;--surface:#fff;--primary:#0073ea;--primary-hover:#005bb5;--primary-light:#e6f0fa;--text:#1a1d2e;--text-secondary:#5f6577;--border:#d5d9e2;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.06),0 1px 2px rgba(0,0,0,.04);--green:#00875a;--green-bg:#e3fcef;--yellow:#ff991f;--yellow-bg:#fff7e6;--red:#de350b;--red-bg:#ffeae6;--blue:#0073ea;--purple:#6554c0;--purple-bg:#eae6ff;--gray:#6b778c;--gray-bg:#f4f5f7;--card-border:1px solid #d5d9e2}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#181b2b;--surface:#23273a;--primary:#4c9aff;--primary-hover:#6db3f8;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--green:#36b37e;--green-bg:#1b3a2a;--yellow:#ff991f;--yellow-bg:#3a2a0a;--red:#ff5630;--red-bg:#3a1510;--blue:#4c9aff;--purple:#998dd9;--purple-bg:#2d2450;--gray:#a0a5b8;--gray-bg:#2a2f45;--card-border:1px solid #3a3f56}}
:root[data-theme="dark"]{--bg:#181b2b;--surface:#23273a;--primary:#4c9aff;--primary-hover:#6db3f8;--primary-light:#1e3a5f;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--green:#36b37e;--green-bg:#1b3a2a;--yellow:#ff991f;--yellow-bg:#3a2a0a;--red:#ff5630;--red-bg:#3a1510;--blue:#4c9aff;--purple:#998dd9;--purple-bg:#2d2450;--gray:#a0a5b8;--gray-bg:#2a2f45;--card-border:1px solid #3a3f56}
body{font-family:'Figtree',-apple-system,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}

/* Header */
header{background:var(--surface);color:var(--text);padding:12px 28px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:12px;border-bottom:var(--card-border);box-shadow:var(--shadow);position:sticky;top:0;z-index:100}
header h1{font-size:17px;font-weight:700;display:flex;align-items:center;gap:8px}

/* Tooltip */
[data-tooltip]{position:relative}
[data-tooltip]::after{content:attr(data-tooltip);position:absolute;bottom:calc(100% + 6px);left:50%;transform:translateX(-50%);background:#1a1d2e;color:#fff;font-size:11px;font-weight:400;padding:4px 8px;border-radius:4px;white-space:nowrap;pointer-events:none;opacity:0;transition:opacity .15s;z-index:200}
[data-tooltip]:hover::after{opacity:1}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"])[data-tooltip]::after{background:#e8eaf0;color:#1a1d2e}}
:root[data-theme="dark"][data-tooltip]::after{background:#e8eaf0;color:#1a1d2e}

/* Nav */
.nav-btn{background:transparent;color:var(--text-secondary);border:1px solid var(--border);border-radius:6px;padding:6px 14px;font-size:13px;text-decoration:none;display:inline-flex;align-items:center;gap:6px;font-family:inherit;transition:all .15s;cursor:pointer}
.nav-btn:hover{background:var(--primary-light);color:var(--primary);border-color:var(--primary)}
.nav-btn.active{background:var(--primary);color:#fff;border-color:var(--primary)}

main{max-width:1400px;margin:0 auto;padding:24px 28px}
@media(max-width:768px){main{padding:16px}}
@media(max-width:480px){header{flex-direction:column;align-items:flex-start}.nav-btn{font-size:12px;padding:5px 10px}}

/* Card */
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden;margin-bottom:24px;border:var(--card-border)}
.card-header{padding:16px 20px;border-bottom:var(--card-border);display:flex;align-items:center;justify-content:space-between;gap:12px}
.card-header h2{font-size:15px;font-weight:600}

/* Table */
table{width:100%;border-collapse:collapse;table-layout:fixed}
thead th{background:var(--surface-alt,#f4f5f7);padding:10px 14px;text-align:left;font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;border-bottom:2px solid var(--border)}
tbody td{padding:12px 14px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:top;word-break:break-word;overflow-wrap:break-word}
tbody tr:hover{background:var(--surface-alt,#f4f5f7)}
tbody tr:last-child td{border-bottom:none}

/* Badge */
.badge{display:inline-block;padding:3px 10px;border-radius:20px;font-size:11px;font-weight:600}
.badge-green{background:var(--green-bg);color:var(--green)}.badge-gray{background:var(--gray-bg);color:var(--gray)}
.badge-blue{background:var(--primary-light);color:var(--primary)}.badge-purple{background:var(--purple-bg);color:var(--purple)}
.badge-yellow{background:var(--yellow-bg);color:var(--yellow)}

/* Buttons */
.btn{display:inline-flex;align-items:center;justify-content:center;gap:4px;padding:6px 14px;border:none;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;transition:all .15s;white-space:nowrap;font-family:inherit;text-decoration:none}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-danger{background:var(--red);color:#fff}.btn-danger:hover{background:#bf2600}
.btn-secondary{background:var(--gray-bg);color:var(--text);border:1px solid var(--border)}.btn-secondary:hover{background:var(--border)}
.btn-test{background:var(--purple-bg);color:var(--purple);border:1px solid #c3bdf0}.btn-test:hover{background:#ddd6fe}
.btn-sm{padding:4px 10px;font-size:11px}
.actions-cell{display:flex;gap:4px;flex-wrap:wrap}

/* Prompt */
.prompt-box{font-family:'SF Mono','Fira Code',monospace;font-size:11px;background:var(--surface-alt,#f4f5f7);border:1px solid var(--border);border-radius:4px;padding:6px 8px;max-height:60px;overflow:hidden;position:relative;color:var(--text-secondary)}
.prompt-box::after{content:'';position:absolute;bottom:0;left:0;right:0;height:20px;background:linear-gradient(transparent,var(--surface-alt,#f4f5f7))}

/* Flash */
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500;display:flex;align-items:center;gap:8px}
.flash-ok{background:var(--green-bg);color:var(--green);border:1px solid #abf5d1}
.flash-err{background:var(--red-bg);color:var(--red);border:1px solid #ffbdad}

/* Test */
.test-panel{background:var(--surface-alt,#f4f5f7);border:var(--card-border);border-radius:var(--radius);padding:14px;margin-top:10px}
.test-panel h4{font-size:11px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px}
.test-input{display:flex;gap:8px}
.test-input textarea{flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:12px;font-family:inherit;outline:none;min-height:60px;resize:vertical;color:var(--text);background:var(--surface)}
.test-input textarea:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(0,115,234,.15)}
.test-result{margin-top:10px;background:var(--surface);border:var(--card-border);border-radius:6px;padding:12px;font-size:13px;min-height:50px;white-space:pre-wrap;line-height:1.6;color:var(--text);display:none}
.test-result.visible{display:block}
.test-result.loading{color:var(--text-secondary);font-style:italic}
.test-result.ok{border-left:3px solid var(--green)}
.test-result.err{border-left:3px solid var(--red);color:var(--red)}
.test-meta{font-size:11px;color:var(--text-secondary);margin-top:6px}
tr.disabled-row td{opacity:.55}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})()</script>
</head>
<body>
<header>
  <h1>
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4"/></svg>
    ИИ Конфигурация
  </h1>
  <div style="display:flex;gap:8px;flex-wrap:wrap">
    <a href="/" class="nav-btn" data-tooltip="Вернуться к документам">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><polyline points="12 19 5 12 12 5"/></svg>
      Документы
    </a>
    <a href="/ai/models" class="nav-btn" data-tooltip="Управление ИИ-моделями">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
      Модели
    </a>
    <a href="/ai/agents" class="nav-btn active" data-tooltip="Управление ИИ-агентами">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4"/></svg>
      Агенты
    </a>
    <a href="/ai/agents/new" class="btn btn-primary" data-tooltip="Создать нового агента">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      Добавить агента
    </a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}
<div class="card">
  <div class="card-header">
    <h2>Настроенные агенты</h2>
    <a href="/ai/agents/new" class="btn btn-primary" data-tooltip="Создать нового агента">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      Добавить агента
    </a>
  </div>
  {{if .Agents}}
  <div style="overflow-x:auto">
  <table>
    <thead><tr>
      <th>Агент</th>
      <th>Тип</th>
      <th>Модель</th>
      <th>Системный промпт</th>
      <th>Параметры</th>
      <th>Статус</th>
      <th style="min-width:220px">Действия и тест</th>
    </tr></thead>
    <tbody>
    {{range .Agents}}
    <tr{{if not .Agent.Enabled}} class="disabled-row"{{end}}>
      <td>
        <div style="font-weight:600">{{.Agent.Name}}</div>
        {{if .Agent.Description}}<div style="font-size:11px;color:var(--text-secondary);margin-top:2px">{{truncate .Agent.Description 50}}</div>{{end}}
      </td>
      <td><span class="badge {{agentBadgeClass (string .Agent.AgentType)}}">{{agentTypeLabel (string .Agent.AgentType)}}</span></td>
      <td>
        {{if .ModelName}}<span style="font-size:12px;font-weight:500">{{.ModelName}}</span>
        <div style="font-size:11px;color:var(--text-secondary)">{{.ModelID}}</div>
        {{else}}<span style="color:var(--text-secondary);font-size:12px">—</span>{{end}}
      </td>
      <td>
        {{if .Agent.SystemPrompt}}
        <div class="prompt-box">{{truncate .Agent.SystemPrompt 120}}</div>
        {{else}}<span style="color:var(--text-secondary);font-size:12px">не задан</span>{{end}}
      </td>
      <td style="font-size:12px">T={{.Agent.Temperature}}<br>{{.Agent.MaxTokens}}tok</td>
      <td>{{if .Agent.Enabled}}<span class="badge badge-green">Активен</span>{{else}}<span class="badge badge-gray">Отключён</span>{{end}}</td>
      <td>
        <div class="actions-cell">
          <a href="/ai/agents/{{.Agent.ID}}/edit" class="btn btn-secondary btn-sm" data-tooltip="Редактировать агента">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            Изменить
          </a>
          <button class="btn btn-test btn-sm" onclick="toggleAgentTest('{{.Agent.ID}}')" data-tooltip="Протестировать агента">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
            Тест
          </button>
          <button class="btn btn-danger btn-sm" onclick="confirmDeleteAgent('/api/ai/agents/{{.Agent.ID}}/delete','{{.Agent.Name}}')" data-tooltip="Удалить агента">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            Удалить
          </button>
        </div>
        <div id="agent-test-{{.Agent.ID}}" style="display:none">
          <div class="test-panel">
            <h4>Тест агента</h4>
            <div class="test-input">
              <textarea id="test-msg-agent-{{.Agent.ID}}" placeholder="Введите тестовый запрос к агенту…"></textarea>
              <button class="btn btn-test" id="test-btn-agent-{{.Agent.ID}}" onclick="testAgent('{{.Agent.ID}}')" data-tooltip="Запустить агента">Запустить</button>
            </div>
            <div id="test-result-agent-{{.Agent.ID}}" class="test-result"></div>
            <div id="test-meta-agent-{{.Agent.ID}}" class="test-meta"></div>
          </div>
        </div>
      </td>
    </tr>
    {{end}}
    </tbody>
  </table>
  </div>
  {{else}}
  <div style="padding:40px;text-align:center;color:var(--text-secondary)">
    <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="margin-bottom:12px;opacity:.5"><circle cx="12" cy="8" r="4"/><path d="M6 20v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2"/></svg>
    <div style="font-size:15px;font-weight:600;margin-bottom:8px">Агенты не настроены</div>
    <div style="font-size:13px;margin-bottom:20px">Создайте первого агента — выберите модель и задайте системный промпт</div>
    <a href="/ai/agents/new" class="btn btn-primary">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      Создать агента
    </a>
  </div>
  {{end}}
</div>
</main>
<script>
function toggleAgentTest(id) {
  var el = document.getElementById('agent-test-'+id);
  el.style.display = el.style.display==='none' ? 'block' : 'none';
  if (el.style.display==='block') document.getElementById('test-msg-agent-'+id).focus();
}
function testAgent(id) {
  var msg = document.getElementById('test-msg-agent-'+id).value.trim();
  if (!msg) { alert('Введите запрос'); return; }
  var resultEl = document.getElementById('test-result-agent-'+id);
  var metaEl = document.getElementById('test-meta-agent-'+id);
  var btn = document.getElementById('test-btn-agent-'+id);
  resultEl.textContent = 'Запускаю агента\u2026';
  resultEl.className = 'test-result visible loading';
  metaEl.textContent = '';
  btn.disabled = true;
  var t0 = Date.now();
  fetch('/api/ai/agents/'+id+'/test', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(function(r){return r.json()}).then(function(d){
    var ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = 'Ошибка: '+d.error;
      resultEl.className = 'test-result visible err';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result visible ok';
      metaEl.textContent = 'Время: '+ms+'мс  Токены: '+(d.tokens||'\u2014')+'  Модель: '+(d.model||'\u2014');
    }
  }).catch(function(e){
    resultEl.textContent = 'Ошибка: '+e.message;
    resultEl.className = 'test-result visible err';
  }).finally(function(){ btn.disabled=false; });
}
function confirmDeleteAgent(url, name) {
  if (!confirm('Удалить агента \u00AB'+name+'\u00BB?')) return;
  fetch(url, {method:'POST'}).then(function(r){
    if (r.redirected) location.href=r.url;
    else if (r.ok) location.reload();
    else r.text().then(function(t){alert('Ошибка: '+t)});
  });
}
</script>
</body>
</html>
`))

var aiAgentFormTmpl = template.Must(template.New("ai-agent-form").Funcs(template.FuncMap{
	"string": func(v interface{}) string { return fmt.Sprintf("%s", v) },
	"selected": func(a, b interface{}) template.HTMLAttr {
		if fmt.Sprintf("%s", a) == fmt.Sprintf("%s", b) {
			return "selected"
		}
		return ""
	},
}).Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{if .Agent.ID}}Редактировать{{else}}Добавить{{end}} агента — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f5f6f8;--surface:#fff;--primary:#0073ea;--primary-hover:#005bb5;--text:#1a1d2e;--text-secondary:#5f6577;--border:#d5d9e2;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.06);--red:#de350b;--red-bg:#ffeae6;--card-border:1px solid #d5d9e2;--surface-alt:#f4f5f7}
@media(prefers-color-scheme:dark){:root:not([data-theme="light"]){--bg:#181b2b;--surface:#23273a;--primary:#4c9aff;--primary-hover:#6db3f8;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--red:#ff5630;--red-bg:#3a1510;--card-border:1px solid #3a3f56;--surface-alt:#2a2f45}}
:root[data-theme="dark"]{--bg:#181b2b;--surface:#23273a;--primary:#4c9aff;--primary-hover:#6db3f8;--text:#e8eaf0;--text-secondary:#a0a5b8;--border:#3a3f56;--red:#ff5630;--red-bg:#3a1510;--card-border:1px solid #3a3f56;--surface-alt:#2a2f45}
body{font-family:'Figtree',-apple-system,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:var(--surface);color:var(--text);padding:12px 28px;display:flex;align-items:center;justify-content:space-between;box-shadow:var(--shadow);border-bottom:var(--card-border)}
header h1{font-size:17px;font-weight:700}
.nav-btn{background:transparent;color:var(--text-secondary);border:1px solid var(--border);border-radius:6px;padding:6px 14px;font-size:13px;text-decoration:none;display:inline-flex;align-items:center;gap:6px;font-family:inherit;transition:all .15s}
.nav-btn:hover{background:var(--primary-light,#e6f0fa);color:var(--primary);border-color:var(--primary)}
main{max-width:900px;margin:32px auto;padding:0 28px}
@media(max-width:768px){main{margin:16px auto;padding:0 16px}}
@media(max-width:480px){header{flex-direction:column;align-items:flex-start}}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden;border:var(--card-border)}
.card-header{padding:20px 24px;border-bottom:var(--card-border)}.card-header h2{font-size:16px;font-weight:600}
.card-body{padding:24px}
.form-group{margin-bottom:18px}
.form-group label{display:block;font-size:13px;font-weight:500;margin-bottom:6px}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:9px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none;transition:border-color .15s;color:var(--text);background:var(--surface)}
.form-group input:focus,.form-group select:focus,.form-group textarea:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(0,115,234,.15)}
.form-group textarea{resize:vertical;min-height:200px}
.form-group .hint{font-size:11px;color:var(--text-secondary);margin-top:4px}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
@media(max-width:768px){.form-row{grid-template-columns:1fr}}
.form-actions{display:flex;gap:10px;margin-top:24px;padding-top:20px;border-top:1px solid var(--border)}
.btn{display:inline-flex;align-items:center;padding:8px 18px;border:none;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;font-family:inherit;text-decoration:none;transition:all .15s}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-secondary{background:var(--surface-alt);color:var(--text);border:1px solid var(--border)}
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500}
.flash-err{background:var(--red-bg);color:var(--red);border:1px solid #ffbdad}
.toggle-wrap{display:flex;align-items:center;gap:10px;padding:10px 12px;border:1px solid var(--border);border-radius:6px;cursor:pointer}
.prompt-defaults{background:var(--surface-alt);border:1px solid var(--border);border-radius:6px;padding:16px;margin-bottom:12px}
.prompt-defaults h4{font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:10px}
.preset-btn{display:inline-block;padding:5px 12px;border:1px solid var(--border);border-radius:4px;font-size:12px;cursor:pointer;margin:4px;background:var(--surface);transition:all .15s;color:var(--text);font-family:inherit}
.preset-btn:hover{background:var(--primary);color:#fff;border-color:var(--primary)}
[data-tooltip]{position:relative}
[data-tooltip]::after{content:attr(data-tooltip);position:absolute;bottom:calc(100% + 6px);left:50%;transform:translateX(-50%);background:#1a1d2e;color:#fff;font-size:11px;font-weight:400;padding:4px 8px;border-radius:4px;white-space:nowrap;pointer-events:none;opacity:0;transition:opacity .15s;z-index:200}
[data-tooltip]:hover::after{opacity:1}
</style>
</head>
<body>
<header>
  <h1>
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4"/></svg>
    {{if .Agent.ID}}Редактировать агента{{else}}Добавить ИИ-агента{{end}}
  </h1>
  <a href="/ai/agents" class="nav-btn" data-tooltip="Вернуться к списку агентов">
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><polyline points="12 19 5 12 12 5"/></svg>
    Назад
  </a>
</header>
<main>
{{if .Error}}<div class="flash flash-err">{{.Error}}</div>{{end}}
<div class="card">
  <div class="card-header"><h2>Конфигурация агента</h2></div>
  <div class="card-body">
    <form method="POST" action="{{if .Agent.ID}}/ai/agents/{{.Agent.ID}}/update{{else}}/ai/agents/create{{end}}">
      <div class="form-group">
        <label>Название *</label>
        <input type="text" name="name" value="{{.Agent.Name}}" placeholder="Консультант Сколково" required>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Тип агента *</label>
          <select name="agent_type" id="agent-type-sel" onchange="loadDefaultPrompt(this.value)">
            <option value="consultant" {{selected (string .Agent.AgentType) "consultant"}}>Консультант</option>
            <option value="validator" {{selected (string .Agent.AgentType) "validator"}}>Валидатор документов</option>
            <option value="monitor" {{selected (string .Agent.AgentType) "monitor"}}>Монитор изменений</option>
            <option value="coordinator" {{selected (string .Agent.AgentType) "coordinator"}}>Координатор</option>
          </select>
        </div>
        <div class="form-group">
          <label>Модель</label>
          <select name="model_id">
            <option value="">— не привязана —</option>
            {{range .Models}}
            <option value="{{.ID}}" {{selected .ID $.Agent.ModelID}}>{{.Name}} ({{.ModelID}})</option>
            {{end}}
          </select>
          <div class="hint">Модель, используемая этим агентом по умолчанию</div>
        </div>
      </div>
      <div class="form-group">
        <label>Системный промпт</label>
        <div class="prompt-defaults">
          <h4>Загрузить промпт по умолчанию для типа агента:</h4>
          <button type="button" class="preset-btn" onclick="loadDefaultPrompt('consultant')">Консультант</button>
          <button type="button" class="preset-btn" onclick="loadDefaultPrompt('validator')">Валидатор</button>
          <button type="button" class="preset-btn" onclick="loadDefaultPrompt('monitor')">Монитор</button>
          <button type="button" class="preset-btn" onclick="loadDefaultPrompt('coordinator')">Координатор</button>
        </div>
        <textarea name="system_prompt" id="system-prompt" placeholder="Системный промпт агента…">{{.Agent.SystemPrompt}}</textarea>
        <div class="hint">Инструкции для агента. Определяют его поведение и специализацию.</div>
      </div>
      <div class="form-row">
        <div class="form-group">
          <label>Temperature</label>
          <input type="number" name="temperature" value="{{if .Agent.Temperature}}{{.Agent.Temperature}}{{else}}0.7{{end}}" min="0" max="2" step="0.1">
          <div class="hint">0 — детерминированный, 2 — максимально случайный</div>
        </div>
        <div class="form-group">
          <label>Max Tokens</label>
          <input type="number" name="max_tokens" value="{{if .Agent.MaxTokens}}{{.Agent.MaxTokens}}{{else}}4096{{end}}" min="256" max="131072">
        </div>
      </div>
      <div class="form-group">
        <label>Описание</label>
        <input type="text" name="description" value="{{.Agent.Description}}" placeholder="Краткое описание назначения агента">
      </div>
      <div class="toggle-wrap" onclick="document.getElementById('agent-enabled').click()">
        <input type="checkbox" name="enabled" id="agent-enabled" value="true"{{if .Agent.Enabled}} checked{{end}}>
        <label for="agent-enabled">Агент активен</label>
      </div>
      <div class="form-actions">
        <button type="submit" class="btn btn-primary">{{if .Agent.ID}}Сохранить{{else}}Создать агента{{end}}</button>
        <a href="/ai/agents" class="btn btn-secondary">Отмена</a>
      </div>
    </form>
  </div>
</div>
</main>
<script>
var defaultPrompts = {{.DefaultPromptsJSON}};
function loadDefaultPrompt(type) {
  var p = defaultPrompts[type];
  if (p) document.getElementById('system-prompt').value = p;
}
</script>
</body>
</html>
`))

// ─── views ────────────────────────────────────────────────────────────────────

type aiModelsPageData struct {
	Models        []aimodels.Model
	EnabledCount  int
	ProviderCount int
	Flash         string
	FlashClass    string
}

type aiAgentView struct {
	Agent     aimodels.Agent
	ModelName string
	ModelID   string
}

type aiAgentsPageData struct {
	Agents     []aiAgentView
	Flash      string
	FlashClass string
}

type aiModelFormData struct {
	Model aimodels.Model
	Error string
}

type aiAgentFormData struct {
	Agent              aimodels.Agent
	Models             []aimodels.Model
	Error              string
	DefaultPromptsJSON template.JS
}

// ─── handlers: models ─────────────────────────────────────────────────────────

func (s *Server) handleAIModelsPage(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен (требуется PostgreSQL-бэкенд)", http.StatusServiceUnavailable)
		return
	}
	models, err := s.aiStore.ListModels(r.Context())
	if err != nil {
		http.Error(w, "Ошибка загрузки моделей: "+err.Error(), http.StatusInternalServerError)
		return
	}

	enabled := 0
	providerSet := map[string]bool{}
	for _, m := range models {
		if m.Enabled {
			enabled++
		}
		providerSet[string(m.Provider)] = true
	}

	flash, flashClass := "", ""
	if msg := r.URL.Query().Get("msg"); msg != "" {
		flash = msg
		flashClass = orDefault(r.URL.Query().Get("kind"), "flash-ok")
	}

	data := aiModelsPageData{
		Models:        models,
		EnabledCount:  enabled,
		ProviderCount: len(providerSet),
		Flash:         flash,
		FlashClass:    flashClass,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := aiModelsTmpl.Execute(w, data); err != nil {
		log.Printf("[ai-admin] models template: %v", err)
	}
}

func (s *Server) handleAIModelNew(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	data := aiModelFormData{
		Model: aimodels.Model{
			Provider:    aimodels.ProviderAlibabaCloud,
			BaseURL:     aimodels.ProviderAlibabaCloud.DefaultBaseURL(),
			MaxTokens:   4096,
			Temperature: 0.7,
			Enabled:     true,
		},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := aiModelFormTmpl.Execute(w, data); err != nil {
		log.Printf("[ai-admin] model-form template: %v", err)
	}
}

func (s *Server) handleAIModelCreate(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m, errMsg := parseModelForm(r)
	if errMsg != "" {
		data := aiModelFormData{Model: m, Error: errMsg}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := aiModelFormTmpl.Execute(w, data); err != nil {
			log.Printf("[ai-admin] model-form template: %v", err)
		}
		return
	}

	if _, err := s.aiStore.CreateModel(r.Context(), m); err != nil {
		data := aiModelFormData{Model: m, Error: "Ошибка создания: " + err.Error()}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiModelFormTmpl.Execute(w, data)
		return
	}
	http.Redirect(w, r, "/ai/models?msg=Модель+добавлена&kind=flash-ok", http.StatusSeeOther)
}

func (s *Server) handleAIModelEdit(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	m, err := s.aiStore.GetModel(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data := aiModelFormData{Model: m}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := aiModelFormTmpl.Execute(w, data); err != nil {
		log.Printf("[ai-admin] model-form template: %v", err)
	}
}

func (s *Server) handleAIModelUpdate(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	m, errMsg := parseModelForm(r)
	m.ID = id
	if errMsg != "" {
		data := aiModelFormData{Model: m, Error: errMsg}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiModelFormTmpl.Execute(w, data)
		return
	}
	if err := s.aiStore.UpdateModel(r.Context(), m); err != nil {
		data := aiModelFormData{Model: m, Error: "Ошибка обновления: " + err.Error()}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiModelFormTmpl.Execute(w, data)
		return
	}
	http.Redirect(w, r, "/ai/models?msg=Модель+обновлена&kind=flash-ok", http.StatusSeeOther)
}

// ─── handlers: agents ─────────────────────────────────────────────────────────

func (s *Server) handleAIAgentsPage(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	agents, err := s.aiStore.ListAgents(r.Context())
	if err != nil {
		http.Error(w, "Ошибка загрузки агентов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	models, _ := s.aiStore.ListModels(r.Context())
	modelMap := map[string]aimodels.Model{}
	for _, m := range models {
		modelMap[m.ID] = m
	}

	views := make([]aiAgentView, 0, len(agents))
	for _, a := range agents {
		v := aiAgentView{Agent: a}
		if m, ok := modelMap[a.ModelID]; ok {
			v.ModelName = m.Name
			v.ModelID = m.ModelID
		}
		views = append(views, v)
	}

	flash, flashClass := "", ""
	if msg := r.URL.Query().Get("msg"); msg != "" {
		flash = msg
		flashClass = orDefault(r.URL.Query().Get("kind"), "flash-ok")
	}

	data := aiAgentsPageData{
		Agents:     views,
		Flash:      flash,
		FlashClass: flashClass,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := aiAgentsTmpl.Execute(w, data); err != nil {
		log.Printf("[ai-admin] agents template: %v", err)
	}
}

func (s *Server) handleAIAgentNew(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	models, _ := s.aiStore.ListModels(r.Context())
	promptsJSON, _ := json.Marshal(aimodels.DefaultSystemPrompts)
	data := aiAgentFormData{
		Agent: aimodels.Agent{
			AgentType:    aimodels.AgentConsultant,
			SystemPrompt: aimodels.DefaultSystemPrompts[aimodels.AgentConsultant],
			Temperature:  0.7,
			MaxTokens:    4096,
			Enabled:      true,
		},
		Models:             models,
		DefaultPromptsJSON: template.JS(promptsJSON),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := aiAgentFormTmpl.Execute(w, data); err != nil {
		log.Printf("[ai-admin] agent-form template: %v", err)
	}
}

func (s *Server) handleAIAgentCreate(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	models, _ := s.aiStore.ListModels(r.Context())
	a, errMsg := parseAgentForm(r)
	if errMsg != "" {
		promptsJSON, _ := json.Marshal(aimodels.DefaultSystemPrompts)
		data := aiAgentFormData{Agent: a, Models: models, Error: errMsg, DefaultPromptsJSON: template.JS(promptsJSON)}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiAgentFormTmpl.Execute(w, data)
		return
	}
	if _, err := s.aiStore.CreateAgent(r.Context(), a); err != nil {
		promptsJSON, _ := json.Marshal(aimodels.DefaultSystemPrompts)
		data := aiAgentFormData{Agent: a, Models: models, Error: "Ошибка создания: " + err.Error(), DefaultPromptsJSON: template.JS(promptsJSON)}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiAgentFormTmpl.Execute(w, data)
		return
	}
	http.Redirect(w, r, "/ai/agents?msg=Агент+создан&kind=flash-ok", http.StatusSeeOther)
}

func (s *Server) handleAIAgentEdit(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	a, err := s.aiStore.GetAgent(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	models, _ := s.aiStore.ListModels(r.Context())
	promptsJSON, _ := json.Marshal(aimodels.DefaultSystemPrompts)
	data := aiAgentFormData{Agent: a, Models: models, DefaultPromptsJSON: template.JS(promptsJSON)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := aiAgentFormTmpl.Execute(w, data); err != nil {
		log.Printf("[ai-admin] agent-form template: %v", err)
	}
}

func (s *Server) handleAIAgentUpdate(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	a, errMsg := parseAgentForm(r)
	a.ID = id
	if errMsg != "" {
		models, _ := s.aiStore.ListModels(r.Context())
		promptsJSON, _ := json.Marshal(aimodels.DefaultSystemPrompts)
		data := aiAgentFormData{Agent: a, Models: models, Error: errMsg, DefaultPromptsJSON: template.JS(promptsJSON)}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiAgentFormTmpl.Execute(w, data)
		return
	}
	if err := s.aiStore.UpdateAgent(r.Context(), a); err != nil {
		models, _ := s.aiStore.ListModels(r.Context())
		promptsJSON, _ := json.Marshal(aimodels.DefaultSystemPrompts)
		data := aiAgentFormData{Agent: a, Models: models, Error: "Ошибка: " + err.Error(), DefaultPromptsJSON: template.JS(promptsJSON)}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = aiAgentFormTmpl.Execute(w, data)
		return
	}
	http.Redirect(w, r, "/ai/agents?msg=Агент+обновлён&kind=flash-ok", http.StatusSeeOther)
}

// ─── API handlers ─────────────────────────────────────────────────────────────

type testRequest struct {
	Message string `json:"message"`
}

type testResponse struct {
	Answer string `json:"answer,omitempty"`
	Tokens int    `json:"tokens,omitempty"`
	Model  string `json:"model,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handleAIModelTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.aiStore == nil {
		writeJSON(w, testResponse{Error: "AI Store не настроен"})
		return
	}
	id := r.PathValue("id")
	m, err := s.aiStore.GetModel(r.Context(), id)
	if err != nil {
		writeJSON(w, testResponse{Error: err.Error()})
		return
	}

	var req testRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, testResponse{Error: "поле message обязательно"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cl := aimodels.NewClient(m)
	answer, tokens, err := cl.Chat(ctx, []aimodels.ChatMessage{
		{Role: "user", Content: req.Message},
	})
	if err != nil {
		writeJSON(w, testResponse{Error: err.Error()})
		return
	}
	writeJSON(w, testResponse{Answer: answer, Tokens: tokens, Model: m.ModelID})
}

func (s *Server) handleAIAgentTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.aiStore == nil {
		writeJSON(w, testResponse{Error: "AI Store не настроен"})
		return
	}
	id := r.PathValue("id")
	a, err := s.aiStore.GetAgent(r.Context(), id)
	if err != nil {
		writeJSON(w, testResponse{Error: err.Error()})
		return
	}
	if a.ModelID == "" {
		writeJSON(w, testResponse{Error: "агент не привязан к модели"})
		return
	}
	m, err := s.aiStore.GetModel(r.Context(), a.ModelID)
	if err != nil {
		writeJSON(w, testResponse{Error: "модель не найдена: " + err.Error()})
		return
	}

	var req testRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, testResponse{Error: "поле message обязательно"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	answer, tokens, err := aimodels.ChatWithAgent(ctx, m, a, req.Message)
	if err != nil {
		writeJSON(w, testResponse{Error: err.Error()})
		return
	}
	writeJSON(w, testResponse{Answer: answer, Tokens: tokens, Model: m.ModelID})
}

func (s *Server) handleAIModelDelete(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.aiStore.DeleteModel(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ai/models?msg=Модель+удалена&kind=flash-ok", http.StatusSeeOther)
}

func (s *Server) handleAIAgentDelete(w http.ResponseWriter, r *http.Request) {
	if s.aiStore == nil {
		http.Error(w, "AI Store не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.aiStore.DeleteAgent(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/ai/agents?msg=Агент+удалён&kind=flash-ok", http.StatusSeeOther)
}

func (s *Server) handleAISeedQwen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.aiStore == nil {
		writeJSON(w, map[string]string{"error": "AI Store не настроен"})
		return
	}
	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.APIKey == "" {
		writeJSON(w, map[string]string{"error": "api_key обязателен"})
		return
	}

	// Принудительно создаём новые модели даже если есть (пересев).
	models := []aimodels.Model{
		{Name: "Qwen Max", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen-max", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "Флагманская модель Qwen — наивысшее качество рассуждений"},
		{Name: "Qwen Plus", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen-plus", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "Баланс качества и скорости"},
		{Name: "Qwen Turbo", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen-turbo", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "Быстрая и экономичная модель"},
		{Name: "Qwen Long", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen-long", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 16384, Temperature: 0.7, Enabled: true, Description: "Длинный контекст до 1M токенов"},
		{Name: "Qwen 2.5 72B Instruct", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen2.5-72b-instruct", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "72B параметров, сильное следование инструкциям"},
		{Name: "Qwen 2.5 32B Instruct", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen2.5-32b-instruct", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "32B — хорошее качество при меньших затратах"},
		{Name: "Qwen 2.5 14B Instruct", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen2.5-14b-instruct", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "14B — эффективная модель среднего размера"},
		{Name: "Qwen 2.5 7B Instruct", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen2.5-7b-instruct", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "7B — лёгкая и быстрая"},
		{Name: "Qwen VL Plus", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen-vl-plus", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 4096, Temperature: 0.7, Enabled: true, Description: "Мультимодальная — изображения + текст"},
		{Name: "Qwen Max Latest", Provider: aimodels.ProviderAlibabaCloud, ModelID: "qwen-max-latest", BaseURL: aimodels.ProviderAlibabaCloud.DefaultBaseURL(), APIKey: body.APIKey, MaxTokens: 8192, Temperature: 0.7, Enabled: true, Description: "Последняя версия Qwen Max"},
	}

	count := 0
	for _, m := range models {
		if _, err := s.aiStore.CreateModel(r.Context(), m); err != nil {
			log.Printf("[ai-admin] seed qwen %s: %v", m.Name, err)
			continue
		}
		count++
	}
	writeJSON(w, map[string]int{"count": count})
}

// ─── form helpers ─────────────────────────────────────────────────────────────

func parseModelForm(r *http.Request) (aimodels.Model, string) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return aimodels.Model{}, "Название обязательно"
	}
	modelID := strings.TrimSpace(r.FormValue("model_id"))
	if modelID == "" {
		return aimodels.Model{}, "Model ID обязателен"
	}
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	if baseURL == "" {
		return aimodels.Model{}, "Base URL обязателен"
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if apiKey == "" {
		return aimodels.Model{}, "API-ключ обязателен"
	}
	maxTokens, _ := strconv.Atoi(r.FormValue("max_tokens"))
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	temp, _ := strconv.ParseFloat(r.FormValue("temperature"), 64)
	if temp <= 0 {
		temp = 0.7
	}
	return aimodels.Model{
		Name:        name,
		Provider:    aimodels.Provider(r.FormValue("provider")),
		ModelID:     modelID,
		BaseURL:     baseURL,
		APIKey:      apiKey,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Enabled:     r.FormValue("enabled") == "true",
		Description: strings.TrimSpace(r.FormValue("description")),
	}, ""
}

func parseAgentForm(r *http.Request) (aimodels.Agent, string) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return aimodels.Agent{}, "Название обязательно"
	}
	temp, _ := strconv.ParseFloat(r.FormValue("temperature"), 64)
	if temp <= 0 {
		temp = 0.7
	}
	maxTokens, _ := strconv.Atoi(r.FormValue("max_tokens"))
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return aimodels.Agent{
		Name:         name,
		AgentType:    aimodels.AgentType(r.FormValue("agent_type")),
		ModelID:      strings.TrimSpace(r.FormValue("model_id")),
		SystemPrompt: r.FormValue("system_prompt"),
		Temperature:  temp,
		MaxTokens:    maxTokens,
		Enabled:      r.FormValue("enabled") == "true",
		Description:  strings.TrimSpace(r.FormValue("description")),
	}, ""
}

func writeJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}
