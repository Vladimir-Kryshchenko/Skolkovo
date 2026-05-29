package admin

import "html/template"

var tmpl = template.Must(template.New("admin").Parse(`
{{define "layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Админка</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f0f2f5; --surface: #fff; --primary: #1e40af; --primary-hover: #1e3a8a;
  --primary-light: #eff6ff; --text: #1e293b; --text-secondary: #64748b;
  --border: #e2e8f0; --radius: 8px; --shadow: 0 1px 3px rgba(0,0,0,.08), 0 1px 2px rgba(0,0,0,.06);
  --shadow-lg: 0 10px 15px -3px rgba(0,0,0,.1), 0 4px 6px -2px rgba(0,0,0,.05);
  --green: #16a34a; --green-bg: #f0fdf4; --yellow: #ca8a04; --yellow-bg: #fefce8;
  --red: #dc2626; --red-bg: #fef2f2; --blue: #2563eb; --purple: #7c3aed; --purple-bg: #f5f3ff;
  --gray: #6b7280; --gray-bg: #f3f4f6;
}
body { font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; }
/* Header */
header { background: linear-gradient(135deg, var(--primary) 0%, #3b82f6 100%); color: #fff; padding: 16px 28px; display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; box-shadow: 0 2px 8px rgba(0,0,0,.15); position: sticky; top: 0; z-index: 100; }
header h1 { font-size: 18px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
.header-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.header-actions button { background: rgba(255,255,255,.15); color: #fff; border: 1px solid rgba(255,255,255,.25); border-radius: 6px; padding: 7px 14px; font-size: 13px; font-weight: 500; cursor: pointer; transition: all .2s; backdrop-filter: blur(4px); }
.header-actions button:hover { background: rgba(255,255,255,.25); }
/* Main */
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }
/* Stats */
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(120px, 1fr)); gap: 12px; margin-bottom: 20px; }
.stat { background: var(--surface); border-radius: var(--radius); padding: 14px 16px; box-shadow: var(--shadow); text-align: center; transition: transform .15s; cursor: pointer; }
.stat:hover { transform: translateY(-2px); box-shadow: var(--shadow-lg); }
.stat .n { font-size: 28px; font-weight: 700; line-height: 1.1; }
.stat .l { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-top: 4px; font-weight: 500; }
.stat.total { border-left: 3px solid var(--primary); }
.stat.active { border-left: 3px solid var(--green); }
.stat.active .n { color: var(--green); }
.stat.pending { border-left: 3px solid var(--yellow); }
.stat.pending .n { color: var(--yellow); }
.stat.outdated { border-left: 3px solid var(--red); }
.stat.outdated .n { color: var(--red); }
.stat.archived { border-left: 3px solid var(--gray); }
.stat.archived .n { color: var(--gray); }
.stat.indexed { border-left: 3px solid var(--purple); }
.stat.indexed .n { color: var(--purple); }
/* Toolbar */
.toolbar { background: var(--surface); border-radius: var(--radius); padding: 14px 18px; margin-bottom: 16px; box-shadow: var(--shadow); display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.toolbar label { font-size: 13px; color: var(--text-secondary); font-weight: 500; }
.filter-tabs { display: flex; gap: 4px; }
.filter-tab { padding: 5px 12px; border-radius: 20px; font-size: 12px; font-weight: 500; text-decoration: none; color: var(--text-secondary); transition: all .15s; border: 1px solid transparent; cursor: pointer; }
.filter-tab:hover { background: var(--primary-light); color: var(--primary); }
.filter-tab.active { background: var(--primary); color: #fff; border-color: var(--primary); }
.search-box { flex: 1; min-width: 180px; max-width: 360px; position: relative; }
.search-box input { width: 100%; padding: 7px 12px 7px 34px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; transition: border-color .15s; }
.search-box input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(30,64,175,.1); }
.search-box svg { position: absolute; left: 10px; top: 50%; transform: translateY(-50%); color: var(--text-secondary); }
/* Flash */
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; display: flex; align-items: center; gap: 8px; animation: slideIn .3s ease; }
.flash.ok { background: var(--green-bg); color: #15803d; border: 1px solid #bbf7d0; }
.flash.err { background: var(--red-bg); color: #b91c1c; border: 1px solid #fecaca; }
@keyframes slideIn { from { opacity: 0; transform: translateY(-8px); } to { opacity: 1; transform: translateY(0); } }
/* Table */
.table-wrap { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow); overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
thead th { background: #f8fafc; padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; border-bottom: 2px solid var(--border); position: sticky; top: 0; }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; vertical-align: middle; }
tbody tr { transition: background .1s; }
tbody tr:hover { background: #f8fafc; }
tbody tr:last-child td { border-bottom: none; }
/* Document title */
.doc-title { font-weight: 600; color: var(--text); line-height: 1.4; }
.doc-meta { font-size: 11px; color: var(--text-secondary); margin-top: 3px; display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
.doc-meta a { color: var(--blue); text-decoration: none; }
.doc-meta a:hover { text-decoration: underline; }
.doc-meta .id-code { font-family: 'SF Mono', 'Fira Code', monospace; color: var(--text-secondary); background: var(--gray-bg); padding: 1px 6px; border-radius: 3px; font-size: 10px; }
/* Badge */
.badge { display: inline-block; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; letter-spacing: .2px; }
.s-на_проверке { background: var(--yellow-bg); color: var(--yellow); }
.s-действует { background: var(--green-bg); color: var(--green); }
.s-устарел { background: var(--red-bg); color: var(--red); }
.s-архив { background: var(--gray-bg); color: var(--gray); }
.s-отклонён { background: var(--purple-bg); color: var(--purple); }
/* Actions */
.actions { display: flex; flex-direction: column; gap: 6px; min-width: 220px; }
.action-row { display: flex; gap: 4px; align-items: center; }
/* Controls */
select, input[type=text] { padding: 5px 8px; border: 1px solid var(--border); border-radius: 5px; font-size: 12px; outline: none; transition: border-color .15s, box-shadow .15s; font-family: inherit; }
select:focus, input[type=text]:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(30,64,175,.1); }
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 4px; padding: 5px 12px; border: none; border-radius: 5px; font-size: 12px; font-weight: 500; cursor: pointer; transition: all .15s; white-space: nowrap; font-family: inherit; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-success { background: var(--green); color: #fff; }
.btn-success:hover { background: #15803d; }
.btn-danger { background: var(--red); color: #fff; }
.btn-danger:hover { background: #b91c1c; }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--gray-bg); }
.btn-sm { padding: 3px 8px; font-size: 11px; }
/* File upload */
.file-upload { display: flex; align-items: center; gap: 6px; }
.file-upload input[type=file] { font-size: 11px; max-width: 150px; }
.file-ok { display: flex; align-items: center; gap: 4px; color: var(--green); font-size: 12px; }
/* Indexed */
.idx { font-size: 16px; }
/* Empty state */
.empty { text-align: center; padding: 48px 24px; color: var(--text-secondary); }
.empty .icon { font-size: 48px; margin-bottom: 12px; }
.empty p { margin-bottom: 16px; }
.empty code { background: var(--gray-bg); padding: 3px 8px; border-radius: 4px; font-size: 12px; }
/* Toast */
#toast { position: fixed; bottom: 24px; right: 24px; padding: 12px 20px; border-radius: var(--radius); color: #fff; font-size: 13px; font-weight: 500; box-shadow: var(--shadow-lg); z-index: 1000; transform: translateY(100px); opacity: 0; transition: all .3s ease; }
#toast.show { transform: translateY(0); opacity: 1; }
#toast.ok { background: var(--green); }
#toast.err { background: var(--red); }
/* Spinner */
.spinner { display: inline-block; width: 14px; height: 14px; border: 2px solid rgba(255,255,255,.3); border-top-color: #fff; border-radius: 50%; animation: spin .6s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* Modal */
.modal-overlay { display: none; position: fixed; inset: 0; background: rgba(0,0,0,.4); z-index: 200; align-items: center; justify-content: center; backdrop-filter: blur(2px); }
.modal-overlay.show { display: flex; }
.modal { background: var(--surface); border-radius: 12px; padding: 24px; max-width: 500px; width: 90%; box-shadow: var(--shadow-lg); }
.modal h3 { font-size: 16px; margin-bottom: 12px; }
.modal p { font-size: 13px; color: var(--text-secondary); margin-bottom: 16px; }
.modal-actions { display: flex; gap: 8px; justify-content: flex-end; }
/* Responsive */
@media (max-width: 768px) {
  main { padding: 16px; }
  .stats { grid-template-columns: repeat(3, 1fr); }
  .toolbar { flex-direction: column; align-items: stretch; }
  .search-box { max-width: 100%; }
  .actions { min-width: auto; }
  table { font-size: 12px; }
  thead th, tbody td { padding: 8px 10px; }
}
</style>
</head>
<body>
<header>
  <h1>📚 База Сколково</h1>
  <div class="header-actions">
    <button onclick="runAction('scrape', this)" title="Парсинг RSS (~20 документов)">📥 Парсинг RSS</button>
    <button onclick="runAction('index', this)" title="Индексация всех документов со статусом 'действует'">🧠 Индексация</button>
    <button onclick="runAction('sync', this)" title="Полный цикл: документы + новости + индексация">🔄 Полный синк</button>
  </div>
</header>
<main>
{{template "content" .}}
</main>
<div id="toast"></div>
<script>
// Toast notification
function toast(msg, type) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.className = 'show ' + (type || 'ok');
  clearTimeout(t._timer);
  t._timer = setTimeout(() => { t.className = ''; }, 4000);
}

// AJAX action for header buttons
async function runAction(action, btn) {
  const orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span>';
  btn.disabled = true;
  try {
    const r = await fetch('/api/' + action, { method: 'POST' });
    const data = await r.json();
    if (data.ok) { toast(data.msg || 'Готово', 'ok'); }
    else { toast('Ошибка: ' + (data.error || 'неизвестно'), 'err'); }
    // Refresh page to update stats
    setTimeout(() => location.reload(), 800);
  } catch(e) {
    toast('Ошибка сети: ' + e.message, 'err');
  } finally {
    btn.innerHTML = orig;
    btn.disabled = false;
  }
}

// AJAX status change
async function setStatus(id, status, sel) {
  try {
    const fd = new FormData();
    fd.append('status', status);
    const r = await fetch('/documents/' + id + '/status', { method: 'POST', body: fd });
    if (r.ok) {
      toast('Статус обновлён: ' + status, 'ok');
      sel.closest('tr').querySelector('.badge').className = 'badge s-' + status;
      sel.closest('tr').querySelector('.badge').textContent = status;
    } else {
      toast('Ошибка при обновлении статуса', 'err');
    }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}

// AJAX category save
async function saveCategory(id, val, inp) {
  try {
    const fd = new FormData();
    fd.append('category', val);
    const r = await fetch('/documents/' + id + '/category', { method: 'POST', body: fd });
    if (r.ok) { toast('Категория обновлена', 'ok'); }
    else { toast('Ошибка', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}

// AJAX supersedes save
async function saveSupersedes(id, val, inp) {
  try {
    const fd = new FormData();
    fd.append('supersedes', val);
    const r = await fetch('/documents/' + id + '/supersedes', { method: 'POST', body: fd });
    if (r.ok) { toast('Связь обновлена', 'ok'); }
    else { toast('Ошибка', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}

// AJAX delete
async function deleteDoc(id) {
  if (!confirm('Удалить документ? Это действие нельзя отменить.')) return;
  try {
    const r = await fetch('/documents/' + id + '/delete', { method: 'POST' });
    if (r.ok) {
      toast('Документ удалён', 'ok');
      const row = document.querySelector('[data-doc-id="' + id + '"]');
      if (row) { row.style.transition = 'opacity .3s'; row.style.opacity = '0'; setTimeout(() => row.remove(), 300); }
    } else { toast('Ошибка удаления', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}

// AJAX file upload
async function uploadFile(id, input) {
  const file = input.files[0];
  if (!file) return;
  const fd = new FormData();
  fd.append('file', file);
  input.disabled = true;
  try {
    const r = await fetch('/documents/' + id + '/upload', { method: 'POST', body: fd });
    if (r.ok) {
      toast('Файл загружен', 'ok');
      // Replace upload form with file-ok indicator
      const cell = input.closest('td');
      cell.innerHTML = '<div class="file-ok">📄 ' + file.name + '</div>';
    } else {
      const t = await r.text();
      toast('Ошибка: ' + t, 'err');
      input.disabled = false;
    }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); input.disabled = false; }
}

// AJAX deindex
async function deindexDoc(id) {
  if (!confirm('Удалить документ из индекса RAG? Документ останется в реестре.')) return;
  try {
    const r = await fetch('/documents/' + id + '/deindex', { method: 'POST' });
    if (r.ok) {
      toast('Документ удалён из индекса', 'ok');
      // Update indexed cell
      const row = document.querySelector('[data-doc-id="' + id + '"]');
      if (row) {
        const idxCell = row.querySelector('.idx');
        if (idxCell) idxCell.textContent = '—';
        // Remove deindex button
        const deindexBtn = row.querySelector('[onclick*="deindexDoc"]');
        if (deindexBtn) deindexBtn.remove();
      }
    } else { toast('Ошибка удаления из индекса', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}
</script>
</body>
</html>{{end}}

{{define "content"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat total"><div class="n">{{.Stats.Total}}</div><div class="l">Всего</div></div>
  <div class="stat active"><div class="n">{{.Stats.Active}}</div><div class="l">Действует</div></div>
  <div class="stat pending"><div class="n">{{.Stats.Pending}}</div><div class="l">На проверке</div></div>
  <div class="stat outdated"><div class="n">{{.Stats.Outdated}}</div><div class="l">Устарело</div></div>
  <div class="stat archived"><div class="n">{{.Stats.Archived}}</div><div class="l">Архив</div></div>
  <div class="stat indexed"><div class="n">{{.Stats.Indexed}}</div><div class="l">В индексе</div></div>
</div>

<div class="toolbar">
  <label>Статус:</label>
  <div class="filter-tabs">
    <a class="filter-tab{{if eq .FilterStatus ""}} active{{end}}" href="/">Все ({{.Stats.Total}})</a>
    <a class="filter-tab{{if eq .FilterStatus "на_проверке"}} active{{end}}" href="/?status=на_проверке">На проверке ({{.Stats.Pending}})</a>
    <a class="filter-tab{{if eq .FilterStatus "действует"}} active{{end}}" href="/?status=действует">Действует ({{.Stats.Active}})</a>
    <a class="filter-tab{{if eq .FilterStatus "устарел"}} active{{end}}" href="/?status=устарел">Устарел ({{.Stats.Outdated}})</a>
    <a class="filter-tab{{if eq .FilterStatus "архив"}} active{{end}}" href="/?status=архив">Архив ({{.Stats.Archived}})</a>
    <a class="filter-tab{{if eq .FilterStatus "отклонён"}} active{{end}}" href="/?status=отклонён">Отклонён ({{.Stats.Rejected}})</a>
  </div>
  <div class="search-box">
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/></svg>
    <form method="get" action="/">
      <input type="hidden" name="status" value="{{.FilterStatus}}">
      <input type="text" name="q" value="{{.Query}}" placeholder="Поиск по названию…">
    </form>
  </div>
</div>

{{if .Docs}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th style="width:40%">Документ</th>
      <th>Категория</th>
      <th>Статус</th>
      <th>Файл</th>
      <th style="text-align:center">Индекс</th>
      <th>Действия</th>
    </tr>
  </thead>
  <tbody>
  {{range .Docs}}
  <tr data-doc-id="{{.ID}}">
    <td>
      <div class="doc-title">{{.Title}}</div>
      <div class="doc-meta">
        <a href="{{.SourceURL}}" target="_blank" rel="noopener">🔗 источник</a>
        <span class="id-code">{{.ID}}</span>
        {{if .PublishedAt}}<span>📅 {{.PublishedAt.Format "02.01.2006"}}</span>{{end}}
        {{if .Supersedes}}<span>⛓ заменяет {{.Supersedes}}</span>{{end}}
      </div>
    </td>
    <td>
      <input type="text" value="{{.Category}}" placeholder="категория"
             onchange="saveCategory('{{.ID}}', this.value, this)"
             style="width:140px">
    </td>
    <td><span class="badge s-{{.Status}}">{{.Status}}</span></td>
    <td>
      {{if .LocalPath}}
        <div class="file-ok">📄 Файл загружен</div>
        <div style="font-size:11px;color:var(--text-secondary);margin-top:2px">{{.FileSize}} · {{.FileAge}}</div>
        <div style="margin-top:4px;display:flex;gap:4px;flex-wrap:wrap">
          <a href="/documents/{{.ID}}/view-original" target="_blank" class="btn btn-ghost btn-sm">👁 Исходник</a>
          <a href="/documents/{{.ID}}/download" class="btn btn-ghost btn-sm">⬇️ Скачать</a>
          {{if .Indexed}}<a href="/documents/{{.ID}}/view-processed" target="_blank" class="btn btn-ghost btn-sm">🧠 Обработанный</a>{{end}}
        </div>
      {{else}}
        <div class="file-upload">
          <input type="file" onchange="uploadFile('{{.ID}}', this)" style="max-width:140px;font-size:11px">
        </div>
      {{end}}
    </td>
    <td style="text-align:center"><span class="idx">{{if .Indexed}}✅{{else}}—{{end}}</span></td>
    <td>
      <div class="actions">
        <div class="action-row">
          <select onchange="setStatus('{{.ID}}', this.value, this)">
            <option value="на_проверке" {{if eq .StatusStr "на_проверке"}}selected{{end}}>на проверке</option>
            <option value="действует" {{if eq .StatusStr "действует"}}selected{{end}}>действует</option>
            <option value="устарел" {{if eq .StatusStr "устарел"}}selected{{end}}>устарел</option>
            <option value="архив" {{if eq .StatusStr "архив"}}selected{{end}}>архив</option>
            <option value="отклонён" {{if eq .StatusStr "отклонён"}}selected{{end}}>отклонён</option>
          </select>
        </div>
        <div class="action-row">
          <input type="text" value="{{.Supersedes}}" placeholder="id заменяемого"
                 onchange="saveSupersedes('{{.ID}}', this.value, this)"
                 style="flex:1;min-width:100px">
          <button class="btn btn-ghost btn-sm" onclick="saveSupersedes('{{.ID}}', this.previousElementSibling.value, this)"></button>
        </div>
        <div class="action-row">
          {{if .Indexed}}<button class="btn btn-ghost btn-sm" onclick="deindexDoc('{{.ID}}')" title="Удалить из индекса RAG">🧹 Деиндекс</button>{{end}}
          <button class="btn btn-danger btn-sm" onclick="deleteDoc('{{.ID}}')">🗑 Удалить</button>
        </div>
      </div>
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon">📭</div>
  <p><strong>Нет документов</strong></p>
  <p>Запустите парсинг кнопкой <strong>«Парсинг RSS»</strong> в шапке страницы<br>
  или выполните в терминале: <code>skolkovo scrape</code></p>
</div>
{{end}}
{{end}}
`))
