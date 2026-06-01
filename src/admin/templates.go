package admin

import "html/template"

var tmpl = template.Must(template.New("admin").Parse(`
{{/* ===== LOGIN PAGE ===== */}}
{{define "login"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Вход — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f6f7fb; --surface: #fff; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e5f0fc; --text: #323338; --text-secondary: #676879;
  --border: #c3c6d4; --radius: 8px;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.08);
  --shadow: 0 4px 16px rgba(0,0,0,.1);
  --shadow-lg: 0 8px 32px rgba(0,0,0,.12);
  --green: #008653; --green-bg: #f4f9f4; --green-border: #b7e4c7;
  --red: #7a0606; --red-bg: #fdf3f3; --red-border: #f5c6c6;
  --role-admin-bg: #e5f0fc; --role-admin-text: #0073ea;
  --role-user-bg: #f4f9f4; --role-user-text: #008653;
  --role-viewer-bg: #fdf8e8; --role-viewer-text: #7a5900;
  --font: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --primary: #579dff; --primary-hover: #7db3ff;
    --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
    --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3);
    --shadow: 0 4px 16px rgba(0,0,0,.4); --shadow-lg: 0 8px 32px rgba(0,0,0,.5);
    --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
    --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
    --role-admin-bg: #1e3050; --role-admin-text: #579dff;
    --role-user-bg: #1a2e1a; --role-user-text: #4ade80;
    --role-viewer-bg: #2e2408; --role-viewer-text: #fbbf24;
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
  --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3);
  --shadow: 0 4px 16px rgba(0,0,0,.4); --shadow-lg: 0 8px 32px rgba(0,0,0,.5);
  --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
  --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
  --role-admin-bg: #1e3050; --role-admin-text: #579dff;
  --role-user-bg: #1a2e1a; --role-user-text: #4ade80;
  --role-viewer-bg: #2e2408; --role-viewer-text: #fbbf24;
}
body { font-family: var(--font); background: var(--bg); min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px; color: var(--text); }
.card { background: var(--surface); border-radius: var(--radius); padding: 40px; box-shadow: var(--shadow-lg); max-width: 440px; width: 100%; border: 1px solid var(--border); }
.logo { text-align: center; margin-bottom: 28px; }
.logo-icon { width: 48px; height: 48px; margin: 0 auto 12px; background: var(--primary); border-radius: 12px; display: flex; align-items: center; justify-content: center; }
.logo-icon svg { width: 28px; height: 28px; fill: #fff; }
.logo h1 { font-size: 20px; font-weight: 700; color: var(--text); }
.logo p { font-size: 13px; color: var(--text-secondary); margin-top: 4px; }
.form-group { margin-bottom: 16px; }
.form-group label { display: block; font-size: 13px; font-weight: 600; color: var(--text); margin-bottom: 6px; }
.form-group input { width: 100%; padding: 10px 14px; border: 1px solid var(--border); border-radius: var(--radius); font-size: 14px; outline: none; transition: all .15s; font-family: var(--font); background: var(--surface); color: var(--text); }
.form-group input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.15); }
.btn { width: 100%; padding: 12px; border: none; border-radius: var(--radius); font-size: 14px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: var(--font); }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); box-shadow: var(--shadow-sm); }
.flash { padding: 10px 14px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; }
.flash.ok { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.flash.err { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.role-switch { display: flex; gap: 8px; margin-bottom: 20px; }
.role-option { flex: 1; padding: 14px 10px; border: 2px solid var(--border); border-radius: var(--radius); text-align: center; cursor: pointer; transition: all .15s; font-size: 13px; font-weight: 600; color: var(--text-secondary); background: var(--surface); }
.role-option:hover { border-color: var(--primary); }
.role-option.active { border-color: var(--primary); background: var(--primary-light); }
.role-option input { display: none; }
.role-badge { width: 32px; height: 32px; margin: 0 auto 8px; border-radius: 8px; display: flex; align-items: center; justify-content: center; font-size: 14px; font-weight: 700; color: #fff; }
.role-option[data-role="admin"] .role-badge { background: var(--primary); }
.role-option[data-role="user"] .role-badge { background: var(--green); }
.role-option[data-role="viewer"] .role-badge { background: #7a5900; }
.role-option.active[data-role="admin"] .role-badge { background: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.2); }
.role-option.active[data-role="user"] .role-badge { background: var(--green); box-shadow: 0 0 0 3px rgba(0,134,83,.2); }
.role-option.active[data-role="viewer"] .role-badge { background: #7a5900; box-shadow: 0 0 0 3px rgba(122,89,0,.2); }
.role-label { font-size: 12px; font-weight: 600; }
.role-option.active .role-label { color: var(--primary); }
.role-desc { font-size: 10px; color: var(--text-secondary); margin-top: 2px; line-height: 1.3; }
.footer { text-align: center; margin-top: 20px; font-size: 12px; color: var(--text-secondary); }
.footer a { color: var(--primary); text-decoration: none; }
.footer a:hover { text-decoration: underline; }
.theme-btn { position: fixed; top: 16px; right: 16px; background: var(--surface); color: var(--text); border: 1px solid var(--border); border-radius: 50%; width: 40px; height: 40px; cursor: pointer; z-index: 10; display: flex; align-items: center; justify-content: center; box-shadow: var(--shadow-sm); transition: all .15s; }
.theme-btn:hover { box-shadow: var(--shadow); }
.theme-btn svg { width: 18px; height: 18px; }
/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]:hover::after {
  content: attr(data-tooltip);
  position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%);
  background: #1a1a2e; color: #fff; padding: 6px 10px; border-radius: 6px;
  font-size: 11px; white-space: nowrap; z-index: 999; pointer-events: none;
  box-shadow: 0 2px 8px rgba(0,0,0,.2);
}
[data-tooltip]:hover::before {
  content: ''; position: absolute; bottom: calc(100% + 2px); left: 50%; transform: translateX(-50%);
  border: 5px solid transparent; border-top-color: #1a1a2e; z-index: 999; pointer-events: none;
}
@media (max-width: 480px) {
  .card { padding: 24px; }
  .role-switch { flex-direction: column; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<button class="theme-btn" id="themeBtn" onclick="toggleTheme()" title="Переключить тему">
  <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
  <svg class="icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
</button>
<div class="card">
  <div class="logo">
    <div class="logo-icon">
      <svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg>
    </div>
    <h1>База Сколково</h1>
    <p>Вход в административную панель</p>
  </div>
  {{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}
  <form method="POST" action="/login">
    <div class="form-group">
      <label for="role">Роль</label>
      <div class="role-switch">
        <label class="role-option active" data-role="admin" onclick="selectRole('admin', this)" title="Полный доступ: управление документами, настройка ИИ, индексация">
          <input type="radio" name="role" value="admin" checked>
          <div class="role-badge">А</div>
          <div class="role-label">Администратор</div>
          <div class="role-desc">Полный доступ</div>
        </label>
        <label class="role-option" data-role="user" onclick="selectRole('user', this)" title="Рабочий доступ: управление документами, без настроек системы">
          <input type="radio" name="role" value="user">
          <div class="role-badge">П</div>
          <div class="role-label">Пользователь</div>
          <div class="role-desc">Рабочий доступ</div>
        </label>
        <label class="role-option" data-role="viewer" onclick="selectRole('viewer', this)" title="Только просмотр: просмотр документов без возможности редактирования">
          <input type="radio" name="role" value="viewer">
          <div class="role-badge">Н</div>
          <div class="role-label">Наблюдатель</div>
          <div class="role-desc">Только просмотр</div>
        </label>
      </div>
    </div>
    <div class="form-group">
      <label for="username">Имя пользователя</label>
      <input type="text" id="username" name="username" placeholder="Введите логин" required autocomplete="username" title="Имя пользователя, выданное администратором">
    </div>
    <div class="form-group">
      <label for="password">Пароль</label>
      <input type="password" id="password" name="password" placeholder="Введите пароль" required autocomplete="current-password" title="Пароль для входа в систему">
    </div>
    <button type="submit" class="btn btn-primary" title="Войти в систему с выбранной ролью">Войти</button>
  </form>
  <div class="footer">
    <a href="/" title="Вернуться на главную страницу">Вернуться на главную</a>
  </div>
</div>
<script>
function selectRole(role, el) {
  document.querySelectorAll('.role-option').forEach(function(o) { o.classList.remove('active'); });
  el.classList.add('active');
  el.querySelector('input').checked = true;
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
  var moon = document.querySelector('.icon-moon');
  var sun = document.querySelector('.icon-sun');
  if (moon && sun) { moon.style.display = theme === 'dark' ? 'none' : ''; sun.style.display = theme === 'dark' ? '' : 'none'; }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  updateThemeIcons(cur);
});
</script>
</body>
</html>{{end}}

{{/* ===== MAIN LAYOUT ===== */}}
{{define "layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Административная панель</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f6f7fb; --surface: #fff; --surface-alt: #f0f2f5; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e5f0fc; --text: #323338; --text-secondary: #676879;
  --border: #c3c6d4; --radius: 8px;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.06); --shadow: 0 2px 8px rgba(0,0,0,.08); --shadow-lg: 0 8px 24px rgba(0,0,0,.1);
  --green: #008653; --green-bg: #f4f9f4; --green-border: #b7e4c7;
  --yellow: #7a5900; --yellow-bg: #fdf8e8; --yellow-border: #f5e0a0;
  --red: #7a0606; --red-bg: #fdf3f3; --red-border: #f5c6c6;
  --blue: #005cc7; --purple: #6544e0; --purple-bg: #f0ecfd; --purple-border: #d4b8f5;
  --gray: #676879; --gray-bg: #f0f2f5;
  --role-admin-bg: #e5f0fc; --role-admin-text: #0073ea;
  --role-user-bg: #f4f9f4; --role-user-text: #008653;
  --role-viewer-bg: #fdf8e8; --role-viewer-text: #7a5900;
  --font: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
    --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
    --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
    --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
    --yellow: #fbbf24; --yellow-bg: #2e2408; --yellow-border: #5a4510;
    --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
    --blue: #60a5fa; --purple: #a78bfa; --purple-bg: #2d1f5e; --purple-border: #4a3580;
    --gray: #9698a6; --gray-bg: #2a2f45;
    --role-admin-bg: #1e3050; --role-admin-text: #579dff;
    --role-user-bg: #1a2e1a; --role-user-text: #4ade80;
    --role-viewer-bg: #2e2408; --role-viewer-text: #fbbf24;
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
  --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
  --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
  --yellow: #fbbf24; --yellow-bg: #2e2408; --yellow-border: #5a4510;
  --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
  --blue: #60a5fa; --purple: #a78bfa; --purple-bg: #2d1f5e; --purple-border: #4a3580;
  --gray: #9698a6; --gray-bg: #2a2f45;
  --role-admin-bg: #1e3050; --role-admin-text: #579dff;
  --role-user-bg: #1a2e1a; --role-user-text: #4ade80;
  --role-viewer-bg: #2e2408; --role-viewer-text: #fbbf24;
}
body { font-family: var(--font); background: var(--bg); color: var(--text); line-height: 1.5; }

/* Header */
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 8px 28px; display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 56px; flex-wrap: wrap; box-shadow: var(--shadow-sm); position: sticky; top: 0; z-index: 100; }
.logo-wrap { display: flex; align-items: center; gap: 10px; }
.logo-icon { width: 32px; height: 32px; background: var(--primary); border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.logo-icon svg { width: 20px; height: 20px; fill: #fff; }
header h1 { font-size: 16px; font-weight: 700; color: var(--text); }
.header-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.header-actions a, .header-actions button {
  background: transparent; color: var(--text-secondary); border: 1px solid var(--border);
  border-radius: 6px; padding: 6px 12px; font-size: 13px; font-weight: 500;
  cursor: pointer; transition: all .15s; text-decoration: none; font-family: var(--font);
  display: inline-flex; align-items: center; gap: 5px;
}
.header-actions a:hover, .header-actions button:hover { background: var(--surface-alt); color: var(--text); border-color: var(--text-secondary); }
.header-divider { width: 1px; height: 24px; background: var(--border); margin: 0 4px; flex-shrink: 0; }
.header-user { font-size: 12px; color: var(--text-secondary); padding: 0 8px; }
.theme-btn {
  background: transparent !important; color: var(--text-secondary) !important;
  border: 1px solid var(--border) !important; border-radius: 6px !important;
  width: 36px !important; height: 36px !important; padding: 0 !important;
  display: flex !important; align-items: center; justify-content: center;
}
.theme-btn:hover { background: var(--surface-alt) !important; color: var(--text) !important; }
.theme-btn svg { width: 18px; height: 18px; }

/* Main */
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }

/* Stats */
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-bottom: 20px; }
.stat { background: var(--surface); border-radius: var(--radius); padding: 16px; box-shadow: var(--shadow-sm); text-align: center; border: 1px solid var(--border); transition: box-shadow .15s; cursor: default; }
.stat:hover { box-shadow: var(--shadow); }
.stat .n { font-size: 28px; font-weight: 700; line-height: 1.1; }
.stat .l { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-top: 4px; font-weight: 600; }
.stat.total { border-top: 3px solid var(--primary); }
.stat.active { border-top: 3px solid var(--green); }
.stat.active .n { color: var(--green); }
.stat.pending { border-top: 3px solid var(--yellow); }
.stat.pending .n { color: var(--yellow); }
.stat.outdated { border-top: 3px solid var(--red); }
.stat.outdated .n { color: var(--red); }
.stat.archived { border-top: 3px solid var(--gray); }
.stat.archived .n { color: var(--gray); }
.stat.indexed { border-top: 3px solid var(--purple); }
.stat.indexed .n { color: var(--purple); }

/* Toolbar */
.toolbar { background: var(--surface); border-radius: var(--radius); padding: 12px 16px; margin-bottom: 12px; box-shadow: var(--shadow-sm); display: flex; align-items: center; gap: 12px; flex-wrap: wrap; border: 1px solid var(--border); }
.toolbar label { font-size: 13px; color: var(--text); font-weight: 600; white-space: nowrap; }
.doc-filters { display: flex; gap: 12px; flex-wrap: wrap; align-items: flex-end; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 12px 16px; margin-bottom: 16px; box-shadow: var(--shadow-sm); }
.doc-filters .filter-field { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 150px; }
.doc-filters .filter-field label { font-size: 11px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .4px; }
.doc-filters input, .doc-filters select { padding: 8px 10px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; background: var(--surface); color: var(--text); font-family: inherit; outline: none; }
.doc-filters input:focus, .doc-filters select:focus { border-color: var(--primary); }
.filter-actions-inline { display: flex; gap: 8px; align-items: flex-end; }
.filter-tabs { display: flex; gap: 4px; flex-wrap: wrap; }
.filter-tab { padding: 5px 12px; border-radius: 20px; font-size: 12px; font-weight: 500; text-decoration: none; color: var(--text-secondary); transition: all .15s; border: 1px solid var(--border); cursor: pointer; background: var(--surface); }
.filter-tab:hover { background: var(--primary-light); color: var(--primary); border-color: var(--primary); }
.filter-tab.active { background: var(--primary); color: #fff; border-color: var(--primary); }
.search-box { flex: 1; min-width: 180px; max-width: 360px; position: relative; }
.search-box form { display: block; }
.search-box input { width: 100%; padding: 7px 12px 7px 34px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; transition: border-color .15s; background: var(--surface); color: var(--text); font-family: var(--font); }
.search-box input:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.1); }
.search-box svg { position: absolute; left: 10px; top: 50%; transform: translateY(-50%); color: var(--text-secondary); pointer-events: none; }

/* Flash */
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; animation: slideIn .3s ease; }
.flash.ok { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.flash.err { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
@keyframes slideIn { from { opacity: 0; transform: translateY(-8px); } to { opacity: 1; transform: translateY(0); } }

/* Table */
.table-wrap { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow-sm); overflow: hidden; border: 1px solid var(--border); }
.table-scroll { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; min-width: 900px; }
thead th { background: var(--surface-alt); padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; border-bottom: 2px solid var(--border); position: sticky; top: 0; }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; vertical-align: middle; word-break: break-word; }
tbody tr { transition: background .1s; }
tbody tr:hover { background: var(--surface-alt); }
tbody tr:last-child td { border-bottom: none; }

/* Document */
.doc-title { font-weight: 600; color: var(--text); line-height: 1.4; max-width: 400px; word-wrap: break-word; }
.doc-meta { font-size: 11px; color: var(--text-secondary); margin-top: 4px; display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
.doc-meta a { color: var(--primary); text-decoration: none; font-weight: 500; }
.doc-meta a:hover { text-decoration: underline; }
.doc-meta .id-code { font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace; color: var(--text-secondary); background: var(--gray-bg); padding: 1px 6px; border-radius: 3px; font-size: 10px; }

/* Badge */
.badge { display: inline-block; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; letter-spacing: .2px; }
.s-на_проверке { background: var(--yellow-bg); color: var(--yellow); border: 1px solid var(--yellow-border); }
.s-действует { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.s-устарел { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.s-архив { background: var(--gray-bg); color: var(--gray); border: 1px solid var(--border); }
.s-отклонён { background: var(--purple-bg); color: var(--purple); border: 1px solid var(--purple-border); }

/* Actions */
.actions { display: flex; flex-direction: column; gap: 6px; min-width: 220px; }
.action-row { display: flex; gap: 4px; align-items: center; }

/* Controls */
select, input[type=text] { padding: 6px 10px; border: 1px solid var(--border); border-radius: 6px; font-size: 12px; outline: none; transition: border-color .15s, box-shadow .15s; font-family: var(--font); background: var(--surface); color: var(--text); }
select:focus, input[type=text]:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.1); }
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 5px; padding: 6px 14px; border: none; border-radius: 6px; font-size: 12px; font-weight: 500; cursor: pointer; transition: all .15s; white-space: nowrap; font-family: var(--font); }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-success { background: var(--green); color: #fff; }
.btn-success:hover { opacity: .85; }
.btn-danger { background: var(--red); color: #fff; }
.btn-danger:hover { opacity: .85; }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--surface-alt); color: var(--text); }
.btn-sm { padding: 4px 10px; font-size: 11px; }

/* File upload */
.file-upload { display: flex; align-items: center; gap: 6px; }
.file-upload input[type=file] { font-size: 11px; max-width: 150px; }
.file-ok { display: flex; align-items: center; gap: 6px; color: var(--green); font-size: 12px; font-weight: 500; }
.file-ok-icon { width: 16px; height: 16px; background: var(--green); border-radius: 4px; display: inline-flex; align-items: center; justify-content: center; }
.file-ok-icon svg { width: 10px; height: 10px; stroke: #fff; stroke-width: 3; fill: none; }

/* Indexed */
.idx { font-size: 16px; }
.idx-yes { color: var(--green); }
.idx-no { color: var(--text-secondary); }

/* Empty */
.empty { text-align: center; padding: 48px 24px; color: var(--text-secondary); }
.empty-icon { width: 48px; height: 48px; margin: 0 auto 12px; background: var(--gray-bg); border-radius: 12px; display: flex; align-items: center; justify-content: center; }
.empty-icon svg { width: 24px; height: 24px; stroke: var(--text-secondary); fill: none; stroke-width: 2; }
.empty p { margin-bottom: 12px; }
.empty code { background: var(--gray-bg); padding: 3px 8px; border-radius: 4px; font-size: 12px; font-family: 'SF Mono', 'Fira Code', monospace; }

/* Toast */
#toast { position: fixed; bottom: 24px; right: 24px; padding: 12px 20px; border-radius: var(--radius); color: #fff; font-size: 13px; font-weight: 500; box-shadow: var(--shadow-lg); z-index: 1000; transform: translateY(100px); opacity: 0; transition: all .3s ease; max-width: 320px; word-wrap: break-word; }
#toast.show { transform: translateY(0); opacity: 1; }
#toast.ok { background: var(--green); }
#toast.err { background: var(--red); }

/* Spinner */
.spinner { display: inline-block; width: 14px; height: 14px; border: 2px solid rgba(255,255,255,.3); border-top-color: #fff; border-radius: 50%; animation: spin .6s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]:hover::after {
  content: attr(data-tooltip);
  position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%);
  background: #1a1a2e; color: #fff; padding: 6px 10px; border-radius: 6px;
  font-size: 11px; white-space: nowrap; z-index: 999; pointer-events: none;
  box-shadow: 0 2px 8px rgba(0,0,0,.2);
}
[data-tooltip]:hover::before {
  content: ''; position: absolute; bottom: calc(100% + 2px); left: 50%; transform: translateX(-50%);
  border: 5px solid transparent; border-top-color: #1a1a2e; z-index: 999; pointer-events: none;
}

/* Responsive */
@media (max-width: 1024px) {
  .actions { min-width: auto; }
  .doc-title { max-width: 280px; }
}
@media (max-width: 768px) {
  header { padding: 0 16px; }
  .header-actions { gap: 4px; }
  .header-actions a, .header-actions button { padding: 5px 8px; font-size: 12px; }
  .header-divider { display: none; }
  main { padding: 16px; }
  .stats { grid-template-columns: repeat(3, 1fr); gap: 8px; }
  .stat { padding: 12px 8px; }
  .stat .n { font-size: 22px; }
  .toolbar { flex-direction: column; align-items: stretch; }
  .filter-tabs { overflow-x: auto; flex-wrap: nowrap; padding-bottom: 4px; }
  .search-box { max-width: 100%; }
  table { font-size: 12px; }
  thead th, tbody td { padding: 8px 10px; }
  .actions { flex-direction: row; flex-wrap: wrap; }
  .doc-title { max-width: 200px; }
}
@media (max-width: 480px) {
  .stats { grid-template-columns: repeat(2, 1fr); }
  header h1 { font-size: 14px; }
  .logo-icon { width: 28px; height: 28px; }
  .logo-icon svg { width: 16px; height: 16px; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<header>
  <div class="logo-wrap">
    <div class="logo-icon">
      <svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg>
    </div>
    <h1>База Сколково</h1>
  </div>
  <div class="header-actions">
    <a href="/" title="Список всех документов базы знаний">Документы</a>
    <a href="/changes" title="История изменений: что нового появилось в базе">Изменения</a>
    <a href="/sitepages" title="Страницы публичного сайта Сколково: просмотр и переход на сайт">Страницы сайта</a>
    <a href="/diff" title="Сравнение версий документов">Сравнение</a>
    <a href="/analytics" title="Статистика и аналитика базы">Аналитика</a>
    <a href="/graph" title="Граф связей между документами">Граф</a>
    <a href="/clients" title="Управление клиентами резидентства">Клиенты</a>
    <a href="/ai/models" title="Настройка ИИ-моделей и агентов">ИИ</a>
    <div class="header-divider"></div>
    <button onclick="runAction('scrape', this)" title="Проверить сайт Сколково и добавить новые документы (RSS-каталог)">Обновить из источников</button>
    <button onclick="runAction('index', this)" title="Перестроить поисковый индекс по всем действующим документам">Переиндексировать поиск</button>
    <button onclick="runAction('sync', this)" title="Полное обновление: загрузка документов и новостей + переиндексация поиска">Полное обновление</button>
    <button onclick="runAction('fetch', this)" title="Скачать тела файлов (PDF/DOCX) для документов без файла на сервере. Идёт через активный прокси/обход WAF — настройте прокси на странице «Прокси», иначе WAF режет дата-центровый IP (403)">Скачать файлы</button>
    <button onclick="runAction('approve-all', this)" style="background:#00875a;color:#fff" title="Одобрить все документы «на проверке» и запустить их индексацию в RAG — документы с метаданными попадут в поиск даже без локального файла">✓ Одобрить все ({{.PendingCount}})</button>
    <button onclick="runAction('seed-local', this)" title="Добавить в поиск локальные .md-файлы из папки документов">Локальные файлы</button>
    <button onclick="runAction('navindex', this)" title="Пересобрать карту навигации сайта для чат-бота">Карта навигации</button>
    <a href="/proxy" class="btn" title="Настройка прокси для обхода WAF dochub (нужно, чтобы скачивались тела файлов)" style="padding:5px 10px">Прокси</a>
    <div class="header-divider"></div>
    <span class="header-user" title="Текущий пользователь">{{if .CurrentUser}}{{.CurrentUser.Username}}{{end}}</span>
    <a href="/logout" title="Выйти из системы" style="padding: 5px 10px">Выход</a>
    <button class="theme-btn" id="themeBtn" onclick="toggleTheme()" title="Переключить тему">
      <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
      <svg class="icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
    </button>
  </div>
</header>
<main>
{{template "content" .}}
</main>
<div id="toast"></div>
<script>
function toast(msg, type) {
  var t = document.getElementById('toast');
  t.textContent = msg;
  t.className = 'show ' + (type || 'ok');
  clearTimeout(t._timer);
  t._timer = setTimeout(function() { t.className = ''; }, 4000);
}
async function runAction(action, btn) {
  var orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span>';
  btn.disabled = true;
  try {
    var r = await fetch('/api/' + action, { method: 'POST' });
    var data = await r.json();
    if (data.ok) { toast(data.msg || 'Готово', 'ok'); }
    else { toast('Ошибка: ' + (data.error || 'неизвестно'), 'err'); }
    setTimeout(function() { location.reload(); }, 800);
  } catch(e) {
    toast('Ошибка сети: ' + e.message, 'err');
  } finally {
    btn.innerHTML = orig;
    btn.disabled = false;
  }
}
async function setStatus(id, status, sel) {
  try {
    var fd = new FormData();
    fd.append('status', status);
    var r = await fetch('/documents/' + id + '/status', { method: 'POST', body: fd });
    if (r.ok) {
      toast('Статус обновлён: ' + status, 'ok');
      var row = sel.closest('tr');
      var badge = row.querySelector('.badge');
      badge.className = 'badge s-' + status;
      badge.textContent = status;
    } else { toast('Ошибка при обновлении статуса', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}
async function saveCategory(id, val, inp) {
  try {
    var fd = new FormData(); fd.append('category', val);
    var r = await fetch('/documents/' + id + '/category', { method: 'POST', body: fd });
    if (r.ok) { toast('Категория обновлена', 'ok'); } else { toast('Ошибка', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}
async function saveSupersedes(id, val, inp) {
  try {
    var fd = new FormData(); fd.append('supersedes', val);
    var r = await fetch('/documents/' + id + '/supersedes', { method: 'POST', body: fd });
    if (r.ok) { toast('Связь замещения обновлена', 'ok'); } else { toast('Ошибка', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}
async function deleteDoc(id) {
  if (!confirm('Удалить документ? Это действие нельзя отменить.')) return;
  try {
    var r = await fetch('/documents/' + id + '/delete', { method: 'POST' });
    if (r.ok) {
      toast('Документ удалён', 'ok');
      var row = document.querySelector('[data-doc-id="' + id + '"]');
      if (row) { row.style.transition = 'opacity .3s'; row.style.opacity = '0'; setTimeout(function() { row.remove(); }, 300); }
    } else { toast('Ошибка удаления', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
}
async function uploadFile(id, input) {
  var file = input.files[0]; if (!file) return;
  var fd = new FormData(); fd.append('file', file); input.disabled = true;
  try {
    var r = await fetch('/documents/' + id + '/upload', { method: 'POST', body: fd });
    if (r.ok) {
      toast('Файл загружен', 'ok');
      var cell = input.closest('td');
      cell.innerHTML = '<div class="file-ok"><span class="file-ok-icon"><svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg></span>' + file.name + '</div>';
    } else { var t = await r.text(); toast('Ошибка: ' + t, 'err'); input.disabled = false; }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); input.disabled = false; }
}
async function deindexDoc(id) {
  if (!confirm('Удалить документ из индекса RAG (поиск по векторам)? Документ останется в реестре.')) return;
  try {
    var r = await fetch('/documents/' + id + '/deindex', { method: 'POST' });
    if (r.ok) {
      toast('Документ удалён из индекса', 'ok');
      var row = document.querySelector('[data-doc-id="' + id + '"]');
      if (row) { var ic = row.querySelector('.idx'); if (ic) ic.textContent = '—'; var db = row.querySelector('[onclick*="deindexDoc"]'); if (db) db.remove(); }
    } else { toast('Ошибка удаления из индекса', 'err'); }
  } catch(e) { toast('Ошибка: ' + e.message, 'err'); }
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
  var moon = document.querySelector('.icon-moon');
  var sun = document.querySelector('.icon-sun');
  if (moon && sun) { moon.style.display = theme === 'dark' ? 'none' : ''; sun.style.display = theme === 'dark' ? '' : 'none'; }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  updateThemeIcons(cur);
});
</script>
</body>
</html>{{end}}

{{/* ===== CONTENT ===== */}}
{{define "content"}}
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<div class="stats">
  <div class="stat total" title="Общее количество документов в реестре">
    <div class="n">{{.Stats.Total}}</div><div class="l">Всего</div>
  </div>
  <div class="stat active" title="Документы со статусом «действует» — участвуют в RAG (поиск по векторам)-поиске">
    <div class="n">{{.Stats.Active}}</div><div class="l">Действует</div>
  </div>
  <div class="stat pending" title="Документы ожидают проверки перед переводом в статус «действует»">
    <div class="n">{{.Stats.Pending}}</div><div class="l">На проверке</div>
  </div>
  <div class="stat outdated" title="Устаревшие документы — заменены новыми версиями">
    <div class="n">{{.Stats.Outdated}}</div><div class="l">Устарело</div>
  </div>
  <div class="stat archived" title="Документы в архиве — не участвуют в поиске">
    <div class="n">{{.Stats.Archived}}</div><div class="l">Архив</div>
  </div>
  <div class="stat indexed" title="Документы, проиндексированные в Qdrant (векторный поиск)">
    <div class="n">{{.Stats.Indexed}}</div><div class="l">В индексе (RAG (поиск по векторам))</div>
  </div>
</div>

<div class="toolbar">
  <label>Статус:</label>
  <div class="filter-tabs">
    <a class="filter-tab{{if eq .FilterStatus ""}} active{{end}}" href="/{{if .BaseQS}}?{{.BaseQS}}{{end}}" title="Все документы">Все ({{.Stats.Total}})</a>
    <a class="filter-tab{{if eq .FilterStatus "на_проверке"}} active{{end}}" href="/?status=на_проверке{{if .BaseQS}}&{{.BaseQS}}{{end}}" title="Документы, ожидающие проверки">На проверке ({{.Stats.Pending}})</a>
    <a class="filter-tab{{if eq .FilterStatus "действует"}} active{{end}}" href="/?status=действует{{if .BaseQS}}&{{.BaseQS}}{{end}}" title="Действующие документы (участвуют в поиске)">Действует ({{.Stats.Active}})</a>
    <a class="filter-tab{{if eq .FilterStatus "устарел"}} active{{end}}" href="/?status=устарел{{if .BaseQS}}&{{.BaseQS}}{{end}}" title="Устаревшие документы">Устарел ({{.Stats.Outdated}})</a>
    <a class="filter-tab{{if eq .FilterStatus "архив"}} active{{end}}" href="/?status=архив{{if .BaseQS}}&{{.BaseQS}}{{end}}" title="Архивные документы">Архив ({{.Stats.Archived}})</a>
    <a class="filter-tab{{if eq .FilterStatus "отклонён"}} active{{end}}" href="/?status=отклонён{{if .BaseQS}}&{{.BaseQS}}{{end}}" title="Отклонённые документы">Отклонён ({{.Stats.Rejected}})</a>
  </div>
</div>

<form class="doc-filters" method="get" action="/">
  <input type="hidden" name="status" value="{{.FilterStatus}}">
  <div class="filter-field" style="flex:2;min-width:200px">
    <label>Поиск по названию</label>
    <input type="text" name="q" value="{{.Query}}" placeholder="Название документа…">
  </div>
  <div class="filter-field">
    <label>Категория</label>
    <select name="category">
      <option value="">Все категории</option>
      {{range .Categories}}<option value="{{.}}"{{if eq . $.FilterCategory}} selected{{end}}>{{.}}</option>{{end}}
    </select>
  </div>
  <div class="filter-field">
    <label title="Когда запись обновлялась в базе">Обновлено от</label>
    <input type="date" name="updated_from" value="{{.UpdatedFrom}}">
  </div>
  <div class="filter-field">
    <label>Обновлено до</label>
    <input type="date" name="updated_to" value="{{.UpdatedTo}}">
  </div>
  <div class="filter-field">
    <label title="Дата публикации документа на сайте Сколково (есть не у всех)">Загружено на sk.ru от</label>
    <input type="date" name="published_from" value="{{.PublishedFrom}}">
  </div>
  <div class="filter-field">
    <label>Загружено на sk.ru до</label>
    <input type="date" name="published_to" value="{{.PublishedTo}}">
  </div>
  <div class="filter-actions-inline">
    <button type="submit" class="btn btn-primary btn-sm">Применить</button>
    <a href="/" class="btn btn-ghost btn-sm">Сбросить</a>
  </div>
</form>

{{if .Docs}}
<div class="table-wrap">
<div class="table-scroll">
<table>
  <thead>
    <tr>
      <th style="width:40%">Документ</th>
      <th>Категория</th>
      <th>Статус</th>
      <th>Файл</th>
      <th style="text-align:center;min-width:60px">Индекс</th>
      <th style="min-width:280px">Действия</th>
    </tr>
  </thead>
  <tbody>
  {{range .Docs}}
  <tr data-doc-id="{{.ID}}">
    <td>
      <div class="doc-title">{{.Title}}</div>
      <div class="doc-meta">
        {{if .SourceLinkURL}}<a href="{{.SourceLinkURL}}" target="_blank" rel="noopener" title="Открыть источник документа">{{.SourceLinkText}}</a>{{end}}
        <span class="id-code">{{.ID}}</span>
        {{if .PublishedAt}}<span>{{.PublishedAt.Format "02.01.2006"}}</span>{{end}}
        {{if .Supersedes}}<span>заменяет {{.Supersedes}}</span>{{end}}
      </div>
    </td>
    <td>
      <input type="text" value="{{.Category}}" placeholder="категория"
             onchange="saveCategory('{{.ID}}', this.value, this)"
             style="width:140px" title="Категория документа. Нажмите Enter или уйдите с поля для сохранения">
    </td>
    <td><span class="badge s-{{.Status}}">{{.Status}}</span></td>
    <td>
      {{if .LocalPath}}
        <div class="file-ok"><span class="file-ok-icon"><svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg></span>Файл на сервере</div>
        <div style="font-size:11px;color:var(--text-secondary);margin-top:2px">{{.FileSize}} · {{.FileAge}}</div>
        <div style="margin-top:4px;display:flex;gap:4px;flex-wrap:wrap">
          <a href="/documents/{{.ID}}/view-original" target="_blank" class="btn btn-ghost btn-sm" title="Открыть файл в просмотрщике">Просмотр</a>
          <a href="/documents/{{.ID}}/download" class="btn btn-ghost btn-sm" title="Скачать файл документа">Скачать</a>
          {{if .Indexed}}<a href="/documents/{{.ID}}/view-processed" target="_blank" class="btn btn-ghost btn-sm" title="Текст документа, как он проиндексирован для поиска">Текст для поиска</a>{{end}}
        </div>
      {{else}}
        {{if .WebURL}}<a href="{{.WebURL}}" target="_blank" rel="noopener" class="btn btn-primary btn-sm" title="Открыть оригинал на сайте Сколково (откроется или скачается в вашем браузере)">Открыть на сайте ↗</a>{{end}}
        <div style="font-size:11px;color:var(--text-secondary);margin-top:4px">файл не загружен на сервер</div>
        <div class="file-upload" style="margin-top:4px">
          <input type="file" onchange="uploadFile('{{.ID}}', this)" style="max-width:140px;font-size:11px" title="Прикрепить файл документа вручную">
        </div>
      {{end}}
    </td>
    <td style="text-align:center">
      <span class="idx {{if .Indexed}}idx-yes{{else}}idx-no{{end}}">{{if .Indexed}}
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
      {{else}}—{{end}}</span>
    </td>
    <td>
      <div class="actions">
        <div class="action-row">
          <select onchange="setStatus('{{.ID}}', this.value, this)" title="Изменить статус документа. Только «действует» попадает в RAG (поиск по векторам)-поиск">
            <option value="на_проверке" {{if eq .StatusStr "на_проверке"}}selected{{end}}>На проверке</option>
            <option value="действует" {{if eq .StatusStr "действует"}}selected{{end}}>Действует</option>
            <option value="устарел" {{if eq .StatusStr "устарел"}}selected{{end}}>Устарел</option>
            <option value="архив" {{if eq .StatusStr "архив"}}selected{{end}}>Архив</option>
            <option value="отклонён" {{if eq .StatusStr "отклонён"}}selected{{end}}>Отклонён</option>
          </select>
        </div>
        <div class="action-row">
          <input type="text" value="{{.Supersedes}}" placeholder="id заменяемого"
                 onchange="saveSupersedes('{{.ID}}', this.value, this)"
                 style="flex:1;min-width:100px" title="ID документа, который этот документ замещает">
          <button class="btn btn-ghost btn-sm" onclick="saveSupersedes('{{.ID}}', this.previousElementSibling.value, this)" title="Сохранить связь замещения">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>
          </button>
        </div>
        <div class="action-row">
          {{if .Indexed}}<button class="btn btn-ghost btn-sm" onclick="deindexDoc('{{.ID}}')" title="Удалить документ из RAG (поиск по векторам)-индекса (запись в реестре сохраняется)">Деиндекс</button>{{end}}
          <button class="btn btn-danger btn-sm" onclick="deleteDoc('{{.ID}}')" title="Безвозвратно удалить документ из реестра и индекса">Удалить</button>
        </div>
      </div>
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
</div>
{{else}}
<div class="empty">
  <div class="empty-icon">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
  </div>
  <p><strong>Нет документов</strong></p>
  <p>Запустите парсинг кнопкой <strong>«Парсинг RSS (каналы)»</strong> в шапке страницы<br>
  или выполните в терминале: <code>skolkovo scrape</code></p>
</div>
{{end}}
{{end}}

{{/* ======================== DIFF TEMPLATE ======================== */}}
{{define "diff-layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Сравнение версий — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f6f7fb; --surface: #fff; --surface-alt: #f0f2f5; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e5f0fc; --text: #323338; --text-secondary: #676879;
  --border: #c3c6d4; --radius: 8px;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.06); --shadow: 0 2px 8px rgba(0,0,0,.08); --shadow-lg: 0 8px 24px rgba(0,0,0,.1);
  --green: #008653; --green-bg: #f4f9f4; --green-border: #b7e4c7;
  --red: #7a0606; --red-bg: #fdf3f3; --red-border: #f5c6c6;
  --font: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
    --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
    --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
    --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
    --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
  --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
  --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
  --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
}
body { font-family: var(--font); background: var(--bg); color: var(--text); line-height: 1.5; }
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 8px 28px; display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 56px; flex-wrap: wrap; box-shadow: var(--shadow-sm); position: sticky; top: 0; z-index: 100; }
.logo-wrap { display: flex; align-items: center; gap: 10px; }
.logo-icon { width: 32px; height: 32px; background: var(--primary); border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.logo-icon svg { width: 20px; height: 20px; fill: #fff; }
header h1 { font-size: 16px; font-weight: 700; color: var(--text); }
.header-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.header-actions a, .header-actions button {
  background: transparent; color: var(--text-secondary); border: 1px solid var(--border);
  border-radius: 6px; padding: 6px 12px; font-size: 13px; font-weight: 500;
  cursor: pointer; transition: all .15s; text-decoration: none; font-family: var(--font);
  display: inline-flex; align-items: center; gap: 5px;
}
.header-actions a:hover, .header-actions button:hover { background: var(--surface-alt); color: var(--text); border-color: var(--text-secondary); }
.header-actions a.active-link { background: var(--primary-light); color: var(--primary); border-color: var(--primary); }
.header-divider { width: 1px; height: 24px; background: var(--border); margin: 0 4px; flex-shrink: 0; }
.theme-btn {
  background: transparent !important; color: var(--text-secondary) !important;
  border: 1px solid var(--border) !important; border-radius: 6px !important;
  width: 36px !important; height: 36px !important; padding: 0 !important;
  display: flex !important; align-items: center; justify-content: center;
}
.theme-btn:hover { background: var(--surface-alt) !important; color: var(--text) !important; }
.theme-btn svg { width: 18px; height: 18px; }
main { max-width: 1200px; margin: 0 auto; padding: 24px 28px; }
.card { background: var(--surface); border-radius: var(--radius); padding: 20px; box-shadow: var(--shadow-sm); margin-bottom: 20px; border: 1px solid var(--border); }
.card h2 { font-size: 16px; margin-bottom: 12px; color: var(--text); display: flex; align-items: center; gap: 8px; }
.card h2 svg { width: 20px; height: 20px; flex-shrink: 0; }
.form-row { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 16px; }
.form-group { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 250px; }
.form-group label { font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }
.form-group select { padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 14px; outline: none; font-family: var(--font); background: var(--surface); color: var(--text); }
.form-group select:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.1); }
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 8px 20px; border: none; border-radius: 6px; font-size: 14px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: var(--font); }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.error-box { background: var(--red-bg); color: var(--red); padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; border: 1px solid var(--red-border); font-size: 13px; display: flex; align-items: center; gap: 8px; }
.error-box svg { width: 18px; height: 18px; flex-shrink: 0; }
.diff-result { border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.diff-result iframe { width: 100%; height: 70vh; border: none; }
.diff-summary { display: flex; gap: 16px; margin-bottom: 12px; }
.diff-stat { padding: 4px 10px; border-radius: 6px; font-size: 12.5px; font-weight: 600; }
.diff-stat.added { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.diff-stat.removed { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
/* Автоматическая лента сравнения версий */
.intro { font-size: 13.5px; color: var(--text-secondary); margin-bottom: 10px; }
.hint { background: var(--primary-light); color: var(--primary); border: 1px solid var(--primary); border-radius: var(--radius); padding: 10px 14px; font-size: 13px; }
.hint a { color: var(--primary); font-weight: 600; }
.empty { color: var(--text-secondary); font-size: 14px; text-align: center; padding: 24px 12px; }
.diff-card { border-left: 4px solid var(--border); }
.diff-card.sev-info { border-left-color: var(--primary); }
.diff-card.sev-warning { border-left-color: #d68a00; }
.diff-card.sev-critical { border-left-color: var(--red); }
.diff-card-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 8px; }
.diff-card-title { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.doc-title { font-weight: 600; font-size: 15px; color: var(--text); }
.cat { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: var(--surface-alt); color: var(--text-secondary); }
.sev-badge { font-size: 11px; font-weight: 700; padding: 3px 10px; border-radius: 12px; white-space: nowrap; text-transform: uppercase; letter-spacing: .3px; }
.sev-badge.sev-info { background: var(--primary-light); color: var(--primary); }
.sev-badge.sev-warning { background: #fff4e0; color: #d68a00; }
.sev-badge.sev-critical { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.sev-badge.sev-none { background: var(--surface-alt); color: var(--text-secondary); }
.diff-meta { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; font-size: 12.5px; color: var(--text-secondary); margin-bottom: 10px; }
.diff-meta .vpair { font-weight: 500; color: var(--text); }
.ai-summary { font-size: 13.5px; color: var(--text); background: var(--surface-alt); border-radius: 6px; padding: 10px 14px; margin-bottom: 10px; }
.stages { font-size: 12.5px; color: var(--text-secondary); margin-bottom: 10px; }
.stage { display: inline-block; background: var(--primary-light); color: var(--primary); border-radius: 10px; padding: 2px 8px; font-size: 11px; margin: 0 2px; }
.diff-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--surface-alt); color: var(--text); }
.reanalyze-result { font-size: 12.5px; color: var(--text); }
.diff-frame-wrap { margin-top: 14px; border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.diff-frame-wrap iframe { width: 100%; height: 65vh; border: none; display: block; }
.spinner { display: inline-block; width: 16px; height: 16px; border: 2px solid rgba(255,255,255,.3); border-top-color: #fff; border-radius: 50%; animation: spin .6s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]:hover::after {
  content: attr(data-tooltip);
  position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%);
  background: #1a1a2e; color: #fff; padding: 6px 10px; border-radius: 6px;
  font-size: 11px; white-space: nowrap; z-index: 999; pointer-events: none;
  box-shadow: 0 2px 8px rgba(0,0,0,.2);
}
[data-tooltip]:hover::before {
  content: ''; position: absolute; bottom: calc(100% + 2px); left: 50%; transform: translateX(-50%);
  border: 5px solid transparent; border-top-color: #1a1a2e; z-index: 999; pointer-events: none;
}
@media (max-width: 768px) {
  header { padding: 0 16px; }
  .header-actions a, .header-actions button { padding: 5px 8px; font-size: 12px; }
  main { padding: 16px; }
  .form-row { flex-direction: column; }
  .form-group { min-width: auto; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<header>
  <div class="logo-wrap">
    <div class="logo-icon">
      <svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg>
    </div>
    <h1>База Сколково</h1>
  </div>
  <div class="header-actions">
    <a href="/" title="Список всех документов базы знаний">Документы</a>
    <a href="/diff" class="active-link" title="Сравнение версий документов">Сравнение</a>
    <a href="/analytics" title="Статистика и аналитика базы">Аналитика</a>
    <a href="/graph" title="Граф связей между документами">Граф</a>
    <a href="/clients" title="Управление клиентами резидентства">Клиенты</a>
    <a href="/ai/models" title="Настройка ИИ-моделей и агентов">ИИ</a>
    <div class="header-divider"></div>
    <button onclick="runAction('scrape', this)" title="Проверить сайт Сколково и добавить новые документы (RSS-каталог)">Обновить из источников</button>
    <button onclick="runAction('index', this)" title="Перестроить поисковый индекс по всем действующим документам">Переиндексировать поиск</button>
    <button onclick="runAction('sync', this)" title="Полное обновление: документы + новости + переиндексация">Полное обновление</button>
    <button onclick="runAction('seed-local', this)" title="Добавить в поиск локальные .md-файлы">Локальные файлы</button>
    <button onclick="runAction('navindex', this)" title="Пересобрать карту навигации сайта для чат-бота">Карта навигации</button>
    <button class="theme-btn" id="themeBtn" onclick="toggleTheme()" title="Переключить тему">
      <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
      <svg class="icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
    </button>
  </div>
</header>
<main>
<div class="card">
  <h2>
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 3h5v5"/><path d="M8 3H3v5"/><path d="M12 22v-8.3a4 4 0 0 0-1.172-2.872L3 3"/><path d="m15 9 6-6"/></svg>
    Автоматическое сравнение версий документов
  </h2>
  <p class="intro">Сравнение редакций выполняется <b>автоматически</b>: как только в источнике появляется новая версия документа, ИИ-агент «Монитор» сопоставляет её с предыдущей, оценивает важность изменений и описывает их суть. Ручной выбор документов не требуется.</p>
  {{if not .HasAI}}<div class="hint">Агент «Монитор» не настроен — пока показываются статистика правок и эвристическая оценка. Включить полноценный ИИ-анализ можно в разделе <a href="/ai/agents">ИИ → Агенты</a>.</div>{{end}}
  {{if .Error}}<div class="error-box">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
    {{.Error}}
  </div>{{end}}
</div>
{{if .Unavailable}}
<div class="card"><div class="empty">История версий недоступна: требуется Postgres-бэкенд с хранилищем версий документов.</div></div>
{{else if not .Cards}}
<div class="card"><div class="empty">Пока нет документов с несколькими редакциями. Как только в источнике появится новая версия документа, её автоматическое сравнение с предыдущей появится здесь.</div></div>
{{else}}
{{range .Cards}}
<div class="card diff-card sev-{{.Severity}}">
  <div class="diff-card-head">
    <div class="diff-card-title">
      <span class="doc-title">{{.Title}}</span>
      {{if .Category}}<span class="cat">{{.Category}}</span>{{end}}
    </div>
    {{if .Severity}}<span class="sev-badge sev-{{.Severity}}">{{.SeverityText}}</span>{{else}}<span class="sev-badge sev-none">Без ИИ-оценки</span>{{end}}
  </div>
  <div class="diff-meta">
    <span class="vpair">Версия {{.OldNo}} ({{.OldAt}}) → {{.NewNo}} ({{.NewAt}})</span>
    <span class="diff-stat added">+{{.Added}}</span>
    <span class="diff-stat removed">−{{.Removed}}</span>
    <span class="vcount">всего версий: {{.VersionCount}}</span>
    {{if .AnalyzedAt}}<span class="analyzed-at">анализ: {{.AnalyzedAt}}</span>{{end}}
  </div>
  {{if .Summary}}<div class="ai-summary"><b>Что изменилось:</b> {{.Summary}}</div>{{end}}
  {{if .Stages}}<div class="stages">Затронутые стадии:{{range .Stages}} <span class="stage">{{.}}</span>{{end}}</div>{{end}}
  <div class="diff-actions">
    <button class="btn btn-ghost" onclick="toggleDiff('{{.DocID}}', this)" title="Показать построчный дифф двух последних версий">Показать полный дифф</button>
    <button class="btn btn-primary" onclick="reanalyze('{{.DocID}}', this)" title="Запустить ИИ-агента «Монитор» заново на свежих версиях">Переанализировать (ИИ)</button>
    <span class="reanalyze-result" id="ra-{{.DocID}}"></span>
  </div>
  <div class="diff-frame-wrap" id="frame-{{.DocID}}" style="display:none">
    <iframe data-src="/diff/{{.DocID}}/full" title="Полный дифф: {{.Title}}"></iframe>
  </div>
</div>
{{end}}
{{end}}
</main>
<script>
function toggleDiff(id, btn) {
  var wrap = document.getElementById('frame-' + id);
  var ifr = wrap.querySelector('iframe');
  if (wrap.style.display === 'none') {
    if (!ifr.getAttribute('src')) { ifr.setAttribute('src', ifr.getAttribute('data-src')); }
    wrap.style.display = 'block'; btn.textContent = 'Скрыть дифф';
  } else { wrap.style.display = 'none'; btn.textContent = 'Показать полный дифф'; }
}
async function reanalyze(id, btn) {
  var out = document.getElementById('ra-' + id);
  var orig = btn.innerHTML; btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>';
  if (out) out.textContent = '';
  try {
    var r = await fetch('/api/diff/' + encodeURIComponent(id) + '/analyze', { method: 'POST' });
    var j = await r.json();
    if (j.ok && out) {
      var sev = j.severity || 'none';
      out.innerHTML = '<span class="sev-badge sev-' + sev + '">' + escapeHtml(j.severity_text || '—') + '</span> ' +
        escapeHtml(j.summary || '') + ' <em>(' + (j.used_llm ? 'ИИ' : 'эвристика') + ', +' + j.added + ' −' + j.removed + ')</em>';
    } else if (out) { out.textContent = 'Ошибка: ' + (j.error || 'неизвестно'); }
  } catch(e) { if (out) out.textContent = 'Ошибка сети: ' + e.message; }
  finally { btn.disabled = false; btn.innerHTML = orig; }
}
function escapeHtml(s) { var d = document.createElement('div'); d.textContent = s == null ? '' : s; return d.innerHTML; }
async function runAction(action, btn) {
  var orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span>';
  btn.disabled = true;
  try {
    var r = await fetch('/api/' + action, { method: 'POST' });
    var data = await r.json();
    if (data.ok) { alert(data.msg || 'Готово'); location.reload(); }
    else { alert('Ошибка: ' + (data.error || 'неизвестно')); }
  } catch(e) { alert('Ошибка сети: ' + e.message); }
  finally { btn.innerHTML = orig; btn.disabled = false; }
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
  var moon = document.querySelector('.icon-moon');
  var sun = document.querySelector('.icon-sun');
  if (moon && sun) { moon.style.display = theme === 'dark' ? 'none' : ''; sun.style.display = theme === 'dark' ? '' : 'none'; }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  updateThemeIcons(cur);
});
</script>
</body>
</html>{{end}}

{{/* ======================== GRAPH TEMPLATE ======================== */}}
{{define "graph-layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Граф связей — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f6f7fb; --surface: #fff; --surface-alt: #f0f2f5; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e5f0fc; --text: #323338; --text-secondary: #676879;
  --border: #c3c6d4; --radius: 8px;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.06); --shadow: 0 2px 8px rgba(0,0,0,.08); --shadow-lg: 0 8px 24px rgba(0,0,0,.1);
  --font: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
    --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
    --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
  --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
}
body { font-family: var(--font); background: var(--bg); color: var(--text); line-height: 1.5; }
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 8px 28px; display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 56px; flex-wrap: wrap; box-shadow: var(--shadow-sm); position: sticky; top: 0; z-index: 100; }
.logo-wrap { display: flex; align-items: center; gap: 10px; }
.logo-icon { width: 32px; height: 32px; background: var(--primary); border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.logo-icon svg { width: 20px; height: 20px; fill: #fff; }
header h1 { font-size: 16px; font-weight: 700; color: var(--text); }
.header-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.header-actions a, .header-actions button {
  background: transparent; color: var(--text-secondary); border: 1px solid var(--border);
  border-radius: 6px; padding: 6px 12px; font-size: 13px; font-weight: 500;
  cursor: pointer; transition: all .15s; text-decoration: none; font-family: var(--font);
  display: inline-flex; align-items: center; gap: 5px;
}
.header-actions a:hover, .header-actions button:hover { background: var(--surface-alt); color: var(--text); border-color: var(--text-secondary); }
.header-actions a.active-link { background: var(--primary-light); color: var(--primary); border-color: var(--primary); }
.header-divider { width: 1px; height: 24px; background: var(--border); margin: 0 4px; flex-shrink: 0; }
.theme-btn {
  background: transparent !important; color: var(--text-secondary) !important;
  border: 1px solid var(--border) !important; border-radius: 6px !important;
  width: 36px !important; height: 36px !important; padding: 0 !important;
  display: flex !important; align-items: center; justify-content: center;
}
.theme-btn:hover { background: var(--surface-alt) !important; color: var(--text) !important; }
.theme-btn svg { width: 18px; height: 18px; }
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }
.card { background: var(--surface); border-radius: var(--radius); padding: 20px; box-shadow: var(--shadow-sm); margin-bottom: 20px; border: 1px solid var(--border); }
.card h2 { font-size: 16px; margin-bottom: 12px; color: var(--text); display: flex; align-items: center; gap: 8px; }
.card h2 svg { width: 20px; height: 20px; flex-shrink: 0; }
#graph-container { width: 100%; height: 75vh; border: 1px solid var(--border); border-radius: var(--radius); overflow: hidden; }
.legend { display: flex; gap: 16px; flex-wrap: wrap; margin-bottom: 12px; font-size: 13px; }
.legend-item { display: flex; align-items: center; gap: 6px; }
.legend-color { width: 24px; height: 4px; border-radius: 2px; }
.spinner { display: inline-block; width: 16px; height: 16px; border: 2px solid rgba(255,255,255,.3); border-top-color: #fff; border-radius: 50%; animation: spin .6s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]:hover::after {
  content: attr(data-tooltip);
  position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%);
  background: #1a1a2e; color: #fff; padding: 6px 10px; border-radius: 6px;
  font-size: 11px; white-space: nowrap; z-index: 999; pointer-events: none;
  box-shadow: 0 2px 8px rgba(0,0,0,.2);
}
[data-tooltip]:hover::before {
  content: ''; position: absolute; bottom: calc(100% + 2px); left: 50%; transform: translateX(-50%);
  border: 5px solid transparent; border-top-color: #1a1a2e; z-index: 999; pointer-events: none;
}
@media (max-width: 768px) {
  header { padding: 0 16px; }
  .header-actions a, .header-actions button { padding: 5px 8px; font-size: 12px; }
  main { padding: 16px; }
  .legend { flex-direction: column; gap: 8px; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<header>
  <div class="logo-wrap">
    <div class="logo-icon">
      <svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg>
    </div>
    <h1>База Сколково</h1>
  </div>
  <div class="header-actions">
    <a href="/" title="Список всех документов базы знаний">Документы</a>
    <a href="/diff" title="Сравнение версий документов">Сравнение</a>
    <a href="/analytics" title="Статистика и аналитика базы">Аналитика</a>
    <a href="/graph" class="active-link" title="Граф связей между документами">Граф</a>
    <a href="/clients" title="Управление клиентами резидентства">Клиенты</a>
    <a href="/ai/models" title="Настройка ИИ-моделей и агентов">ИИ</a>
    <div class="header-divider"></div>
    <button onclick="runAction('scrape', this)" title="Проверить сайт Сколково и добавить новые документы (RSS-каталог)">Обновить из источников</button>
    <button onclick="runAction('index', this)" title="Перестроить поисковый индекс по всем действующим документам">Переиндексировать поиск</button>
    <button onclick="runAction('sync', this)" title="Полное обновление: документы + новости + переиндексация">Полное обновление</button>
    <button onclick="runAction('seed-local', this)" title="Добавить в поиск локальные .md-файлы">Локальные файлы</button>
    <button onclick="runAction('navindex', this)" title="Пересобрать карту навигации сайта для чат-бота">Карта навигации</button>
    <button class="theme-btn" id="themeBtn" onclick="toggleTheme()" title="Переключить тему">
      <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
      <svg class="icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
    </button>
  </div>
</header>
<main>
<div class="card">
  <h2>
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><circle cx="4" cy="4" r="2"/><circle cx="20" cy="4" r="2"/><circle cx="4" cy="20" r="2"/><circle cx="20" cy="20" r="2"/><line x1="6" y1="5" x2="10" y2="10"/><line x1="18" y1="5" x2="14" y2="10"/><line x1="6" y1="19" x2="10" y2="14"/><line x1="18" y1="19" x2="14" y2="14"/></svg>
    Граф связей документов
  </h2>
  <div class="legend">
    <div class="legend-item" title="Документ ссылается на другой"><div class="legend-color" style="background:#2563eb"></div><span>Ссылается (references)</span></div>
    <div class="legend-item" title="Документ замещает другой (более новая версия)"><div class="legend-color" style="background:#dc2626"></div><span>Замещает (supersedes)</span></div>
    <div class="legend-item" title="Документы связаны по теме"><div class="legend-color" style="background:#16a34a"></div><span>Связано (related)</span></div>
  </div>
  <div id="graph-container"></div>
</div>
</main>
<script src="https://unpkg.com/vis-network@9.1.6/standalone/umd/vis-network.min.js"></script>
<script>
var graphData = {{.GraphJSON}};
var nodes = new vis.DataSet(graphData.nodes.map(function(n) {
  return { id: n.id, label: n.label, group: n.group, title: n.title, shape: 'dot', size: 20, font: { size: 12, face: 'Figtree' } };
}));
var edges = new vis.DataSet(graphData.edges.map(function(e) {
  return { from: e.from, to: e.to, label: e.label, color: { color: e.color || '#6b7280' }, dashes: e.dashes || false, font: { size: 10, align: 'middle' }, smooth: { type: 'continuous' } };
}));
var container = document.getElementById('graph-container');
var data = { nodes: nodes, edges: edges };
var options = {
  physics: {
    enabled: true,
    stabilization: { iterations: 150 },
    solver: 'forceAtlas2Based',
    forceAtlas2Based: { gravitationalConstant: -80, springLength: 120, springConstant: 0.05 }
  },
  interaction: { hover: true, zoomView: true, zoomSpeed: 0.1 },
  groups: { default: { color: { background: '#3b82f6', border: '#1e40af' } } }
};
var network = new vis.Network(container, data, options);
network.on('click', function(params) {
  if (params.nodes.length > 0) {
    var nodeId = params.nodes[0];
    var node = nodes.get(nodeId);
    if (node && node.id) {
      window.open('/?q=' + encodeURIComponent(node.label), '_blank');
    }
  }
});
async function runAction(action, btn) {
  var orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span>';
  btn.disabled = true;
  try {
    var r = await fetch('/api/' + action, { method: 'POST' });
    var data = await r.json();
    if (data.ok) { alert(data.msg || 'Готово'); location.reload(); }
    else { alert('Ошибка: ' + (data.error || 'неизвестно')); }
  } catch(e) { alert('Ошибка сети: ' + e.message); }
  finally { btn.innerHTML = orig; btn.disabled = false; }
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
  var moon = document.querySelector('.icon-moon');
  var sun = document.querySelector('.icon-sun');
  if (moon && sun) { moon.style.display = theme === 'dark' ? 'none' : ''; sun.style.display = theme === 'dark' ? '' : 'none'; }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  updateThemeIcons(cur);
});
</script>
</body>
</html>{{end}}

{{/* ======================== CHANGES TEMPLATE ======================== */}}
{{define "changes-layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>История изменений — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f6f7fb; --surface: #fff; --surface-alt: #f0f2f5; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e5f0fc; --text: #323338; --text-secondary: #676879;
  --border: #c3c6d4; --radius: 8px;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.06); --shadow: 0 2px 8px rgba(0,0,0,.08); --shadow-lg: 0 8px 24px rgba(0,0,0,.1);
  --green: #008653; --green-bg: #f4f9f4; --green-border: #b7e4c7;
  --yellow: #7a5900; --yellow-bg: #fdf8e8; --yellow-border: #f5e0a0;
  --red: #7a0606; --red-bg: #fdf3f3; --red-border: #f5c6c6;
  --blue: #005cc7; --purple: #6544e0; --purple-bg: #f0ecfd; --purple-border: #d4b8f5;
  --gray: #676879; --gray-bg: #f0f2f5;
  --font: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
    --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
    --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
    --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
    --yellow: #fbbf24; --yellow-bg: #2e2408; --yellow-border: #5a4510;
    --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
    --blue: #60a5fa; --purple: #a78bfa; --purple-bg: #2d1f5e; --purple-border: #4a3580;
    --gray: #9698a6; --gray-bg: #2a2f45;
  }
}
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6;
  --border: #3b3f54; --shadow-sm: 0 1px 4px rgba(0,0,0,.3); --shadow: 0 2px 8px rgba(0,0,0,.4); --shadow-lg: 0 8px 24px rgba(0,0,0,.5);
  --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
  --yellow: #fbbf24; --yellow-bg: #2e2408; --yellow-border: #5a4510;
  --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d;
  --blue: #60a5fa; --purple: #a78bfa; --purple-bg: #2d1f5e; --purple-border: #4a3580;
  --gray: #9698a6; --gray-bg: #2a2f45;
}
body { font-family: var(--font); background: var(--bg); color: var(--text); line-height: 1.5; }

/* Header — reuse admin header */
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 8px 28px; display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 56px; flex-wrap: wrap; box-shadow: var(--shadow-sm); position: sticky; top: 0; z-index: 100; }
.logo-wrap { display: flex; align-items: center; gap: 10px; }
.logo-icon { width: 32px; height: 32px; background: var(--primary); border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.logo-icon svg { width: 20px; height: 20px; fill: #fff; }
header h1 { font-size: 16px; font-weight: 700; color: var(--text); }
.header-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.header-actions a, .header-actions button {
  background: transparent; color: var(--text-secondary); border: 1px solid var(--border);
  border-radius: 6px; padding: 6px 12px; font-size: 13px; font-weight: 500;
  cursor: pointer; transition: all .15s; text-decoration: none; font-family: var(--font);
  display: inline-flex; align-items: center; gap: 5px;
}
.header-actions a:hover, .header-actions button:hover { background: var(--surface-alt); color: var(--text); border-color: var(--text-secondary); }
.header-actions a.active-link { background: var(--primary-light); color: var(--primary); border-color: var(--primary); }
.header-divider { width: 1px; height: 24px; background: var(--border); margin: 0 4px; flex-shrink: 0; }
.theme-btn {
  background: transparent !important; color: var(--text-secondary) !important;
  border: 1px solid var(--border) !important; border-radius: 6px !important;
  width: 36px !important; height: 36px !important; padding: 0 !important;
  display: flex !important; align-items: center; justify-content: center;
}
.theme-btn:hover { background: var(--surface-alt) !important; color: var(--text) !important; }
.theme-btn svg { width: 18px; height: 18px; }
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }

/* Last parse info bar */
.parse-info { background: var(--primary-light); border: 1px solid var(--primary); border-radius: var(--radius); padding: 12px 16px; margin-bottom: 16px; display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; font-size: 13px; }
.parse-info .label { font-weight: 600; color: var(--primary); }
.parse-info .time { color: var(--text); font-weight: 500; }

/* Stats cards */
.stats-row { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-bottom: 16px; }
.stat-card { background: var(--surface); border-radius: var(--radius); padding: 16px; box-shadow: var(--shadow-sm); text-align: center; border: 1px solid var(--border); }
.stat-card .n { font-size: 28px; font-weight: 700; line-height: 1.1; }
.stat-card .l { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-top: 4px; font-weight: 600; }
.stat-card.new { border-top: 3px solid var(--green); }
.stat-card.new .n { color: var(--green); }
.stat-card.updated { border-top: 3px solid var(--blue); }
.stat-card.updated .n { color: var(--blue); }
.stat-card.outdated { border-top: 3px solid var(--red); }
.stat-card.outdated .n { color: var(--red); }
.stat-card.removed { border-top: 3px solid var(--gray); }
.stat-card.removed .n { color: var(--gray); }
.stat-card.total { border-top: 3px solid var(--primary); }

/* Filter bar */
.filter-bar { background: var(--surface); border-radius: var(--radius); padding: 16px; margin-bottom: 16px; box-shadow: var(--shadow-sm); border: 1px solid var(--border); }
.filter-row { display: flex; gap: 12px; flex-wrap: wrap; align-items: flex-end; }
.filter-group { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 140px; }
.filter-group label { font-size: 11px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }
.filter-group input, .filter-group select { padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; font-family: var(--font); background: var(--surface); color: var(--text); }
.filter-group input:focus, .filter-group select:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.1); }
.filter-actions { display: flex; gap: 8px; align-items: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 5px; padding: 8px 18px; border: none; border-radius: 6px; font-size: 13px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: var(--font); text-decoration: none; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--surface-alt); }

/* Timeline / events table */
.table-wrap { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow-sm); overflow: hidden; border: 1px solid var(--border); }
.table-scroll { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; min-width: 800px; }
thead th { background: var(--surface-alt); padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; border-bottom: 2px solid var(--border); }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; vertical-align: middle; }
tbody tr { transition: background .1s; }
tbody tr:hover { background: var(--surface-alt); }

/* Kind badge */
.kind-badge { display: inline-flex; align-items: center; gap: 6px; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; }
.kind-new { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.kind-updated { background: var(--blue-bg, #e5f0fc); color: var(--blue); border: 1px solid var(--blue-border, #b3d4fc); }
.kind-outdated { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.kind-removed { background: var(--gray-bg); color: var(--gray); border: 1px solid var(--border); }
.kind-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.kind-dot.kind-new { background: var(--green); }
.kind-dot.kind-updated { background: var(--blue); }
.kind-dot.kind-outdated { background: var(--red); }
.kind-dot.kind-removed { background: var(--gray); }

/* Entity type tag */
.entity-tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 10px; font-weight: 600; background: var(--purple-bg); color: var(--purple); border: 1px solid var(--purple-border); font-family: 'SF Mono', 'Fira Code', monospace; }

/* Tag cloud / chips */
.tag-bar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 12px 16px; margin-bottom: 16px; box-shadow: var(--shadow-sm); }
.tag-bar-label { font-size: 11px; font-weight: 700; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-right: 4px; }
.tag-chip { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 16px; font-size: 12px; font-weight: 500; text-decoration: none; background: var(--surface-alt); color: var(--text); border: 1px solid var(--border); transition: all .12s; }
.tag-chip:hover { border-color: var(--primary); color: var(--primary); }
.tag-chip.active { background: var(--primary); color: #fff; border-color: var(--primary); }
.tag-chip .tag-count { font-size: 10px; font-weight: 700; opacity: .7; }
.tag-chip.tag-reset { background: var(--red-bg); color: var(--red); border-color: var(--red-border); }
.tag-chip.mini { padding: 1px 8px; font-size: 11px; }
.row-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }

/* Source freshness panel */
.health-panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 14px 16px; margin-bottom: 16px; box-shadow: var(--shadow-sm); }
.health-title { font-size: 11px; font-weight: 700; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-bottom: 10px; }
.health-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 10px; }
.health-card { display: flex; align-items: center; gap: 10px; padding: 8px 12px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface-alt); }
.health-dot { width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0; background: var(--gray); }
.health-ok .health-dot { background: var(--green); }
.health-stale .health-dot { background: var(--yellow); }
.health-failing .health-dot { background: var(--red); }
.health-name { font-size: 13px; font-weight: 600; }
.health-meta { font-size: 11px; color: var(--text-secondary); }

/* Empty */
.empty { text-align: center; padding: 48px 24px; color: var(--text-secondary); }
.empty-icon { width: 48px; height: 48px; margin: 0 auto 12px; background: var(--gray-bg); border-radius: 12px; display: flex; align-items: center; justify-content: center; }
.empty-icon svg { width: 24px; height: 24px; stroke: var(--text-secondary); fill: none; stroke-width: 2; }

/* Tooltip */
[data-tooltip] { position: relative; }
[data-tooltip]:hover::after {
  content: attr(data-tooltip);
  position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%);
  background: #1a1a2e; color: #fff; padding: 6px 10px; border-radius: 6px;
  font-size: 11px; white-space: nowrap; z-index: 999; pointer-events: none;
  box-shadow: 0 2px 8px rgba(0,0,0,.2);
}

/* Responsive */
@media (max-width: 768px) {
  header { padding: 0 16px; }
  .header-actions { gap: 4px; }
  main { padding: 16px; }
  .stats-row { grid-template-columns: repeat(2, 1fr); }
  .filter-row { flex-direction: column; }
  .filter-group { min-width: 100%; }
  table { font-size: 12px; }
  thead th, tbody td { padding: 8px 10px; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
<header>
  <div class="logo-wrap">
    <div class="logo-icon">
      <svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg>
    </div>
    <h1>База Сколково</h1>
  </div>
  <div class="header-actions">
    <a href="/">Документы</a>
    <a href="/changes" class="active-link">Изменения</a>
    <a href="/sitepages">Страницы сайта</a>
    <a href="/diff">Сравнение</a>
    <a href="/analytics">Аналитика</a>
    <a href="/graph">Граф</a>
    <a href="/clients">Клиенты</a>
    <a href="/ai/models">ИИ</a>
    <div class="header-divider"></div>
    <a href="/logout" style="padding: 5px 10px">Выход</a>
    <button class="theme-btn" id="themeBtn" onclick="toggleTheme()">
      <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
      <svg class="icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
    </button>
  </div>
</header>
<main>
{{template "changes-content" .}}
</main>
<script>
function toggleTheme() {
  var r = document.documentElement;
  var cur = r.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  var next = cur === 'dark' ? 'light' : 'dark';
  r.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  updateThemeIcons(next);
}
function updateThemeIcons(theme) {
  var moon = document.querySelector('.icon-moon');
  var sun = document.querySelector('.icon-sun');
  if (moon && sun) { moon.style.display = theme === 'dark' ? 'none' : ''; sun.style.display = theme === 'dark' ? '' : 'none'; }
}
document.addEventListener('DOMContentLoaded', function() {
  var cur = document.documentElement.getAttribute('data-theme') || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
  updateThemeIcons(cur);
});
</script>
</body>
</html>{{end}}

{{define "changes-content"}}
{{/* Last parse info */}}
<div class="parse-info">
  <div>
    <span class="label"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:14px;height:14px;vertical-align:-2px;margin-right:6px"><path d="M4 11a9 9 0 0 1 9 9"/><path d="M4 4a16 16 0 0 1 16 16"/><circle cx="5" cy="19" r="1"/></svg>Последний парсинг:</span>
    {{if .Stats.LastParse.IsZero}}
      <span class="time">ещё не выполнялся</span>
    {{else}}
      <span class="time" title="Когда последний раз успешно работал сбор данных">{{.Stats.LastParse.Format "02.01.2006 15:04"}}</span>
    {{end}}
  </div>
  <div style="font-size:12px;color:var(--text-secondary)">
    Показано изменений: <strong>{{.Stats.Total}}</strong>
  </div>
</div>

{{/* Stats */}}
<div class="stats-row">
  <div class="stat-card total"><div class="n">{{.Stats.Total}}</div><div class="l">Всего</div></div>
  <div class="stat-card new"><div class="n">{{.Stats.New}}</div><div class="l">Новые</div></div>
  <div class="stat-card updated"><div class="n">{{.Stats.Updated}}</div><div class="l">Обновлены</div></div>
  <div class="stat-card outdated"><div class="n">{{.Stats.Outdated}}</div><div class="l">Устарели</div></div>
  <div class="stat-card removed"><div class="n">{{.Stats.Removed}}</div><div class="l">Удалены</div></div>
</div>

{{/* Filters */}}
<div class="filter-bar">
  <form method="get" action="/changes">
    <div class="filter-row">
      <div class="filter-group" style="min-width:200px">
        <label for="q">Поиск</label>
        <input type="text" id="q" name="q" value="{{.Query}}" placeholder="По названию или ID…">
      </div>
      <div class="filter-group">
        <label for="entity_type">Тип сущности</label>
        <select id="entity_type" name="entity_type">
          <option value="">Все</option>
          <option value="document"{{if eq .EntityType "document"}} selected{{end}}>Документ</option>
          <option value="news"{{if eq .EntityType "news"}} selected{{end}}>Новость</option>
          <option value="event"{{if eq .EntityType "event"}} selected{{end}}>Мероприятие</option>
          <option value="contest"{{if eq .EntityType "contest"}} selected{{end}}>Конкурс/грант</option>
          <option value="npa"{{if eq .EntityType "npa"}} selected{{end}}>НПА (нормативный акт)</option>
          <option value="preference"{{if eq .EntityType "preference"}} selected{{end}}>Льгота</option>
          <option value="faq"{{if eq .EntityType "faq"}} selected{{end}}>FAQ (частые вопросы)</option>
          <option value="telegram"{{if eq .EntityType "telegram"}} selected{{end}}>Telegram</option>
          <option value="sitepage"{{if eq .EntityType "sitepage"}} selected{{end}}>Страница сайта</option>
        </select>
      </div>
      <div class="filter-group">
        <label for="date_from">Дата от</label>
        <input type="date" id="date_from" name="date_from" value="{{.DateFrom}}">
      </div>
      <div class="filter-group">
        <label for="date_to">Дата до</label>
        <input type="date" id="date_to" name="date_to" value="{{.DateTo}}">
      </div>
      <div class="filter-actions">
        <button type="submit" class="btn btn-primary" title="Применить фильтры">Применить</button>
        <a href="/changes" class="btn btn-ghost" title="Сбросить все фильтры">Сбросить</a>
      </div>
    </div>
  </form>
</div>

{{/* Tag cloud (auto-derived) */}}
{{if .AllTags}}
<div class="tag-bar">
  <span class="tag-bar-label">Теги:</span>
  {{if .Tag}}<a class="tag-chip tag-reset" href="/changes{{if .BaseQS}}?{{.BaseQS}}{{end}}" title="Сбросить фильтр по тегу">× {{.Tag}}</a>{{end}}
  {{range .AllTags}}
    <a class="tag-chip{{if eq .Name $.Tag}} active{{end}}" href="/changes?tag={{.Enc}}{{if $.BaseQS}}&{{$.BaseQS}}{{end}}" title="Фильтровать по тегу «{{.Name}}»">{{.Name}}<span class="tag-count">{{.Count}}</span></a>
  {{end}}
</div>
{{end}}

{{/* Source freshness panel — когда какой источник обновлялся */}}
{{if .Health}}
<div class="health-panel">
  <div class="health-title">Свежесть источников</div>
  <div class="health-grid">
    {{range .Health}}
    <div class="health-card health-{{.State}}" title="Последнее успешное обновление: {{.LastSuccess}}">
      <div class="health-dot"></div>
      <div class="health-body">
        <div class="health-name">{{.Label}}</div>
        <div class="health-meta">{{.StateLabel}} · {{.LastSuccess}}</div>
      </div>
    </div>
    {{end}}
  </div>
</div>
{{end}}

{{/* Events table */}}
{{if .Rows}}
<div class="table-wrap">
<div class="table-scroll">
<table>
  <thead>
    <tr>
      <th style="width:120px">Время</th>
      <th style="width:100px">Тип</th>
      <th style="width:100px">Сущность</th>
      <th>Название</th>
      <th style="width:140px">Категория</th>
      <th style="width:100px">Изменение</th>
    </tr>
  </thead>
  <tbody>
  {{range .Rows}}
  <tr>
    <td style="white-space:nowrap;font-size:12px" title="{{.Event.DetectedAt.Format "02.01.2006 15:04:05"}}">
      {{.Event.DetectedAt.Format "02.01 15:04"}}
    </td>
    <td>
      <span class="kind-badge kind-{{.Event.Kind}}" title="
        {{if eq .Event.Kind "new"}}Сущность впервые появилась в базе
        {{else if eq .Event.Kind "updated"}}Содержимое или метаданные изменились
        {{else if eq .Event.Kind "outdated"}}Сущность переведена в статус «устарела»
        {{else if eq .Event.Kind "removed"}}Сущность удалена из источника{{end}}
      ">
        {{if eq .Event.Kind "new"}}<span class="kind-dot kind-new"></span> Новая{{else if eq .Event.Kind "updated"}}<span class="kind-dot kind-updated"></span> Обновлена{{else if eq .Event.Kind "outdated"}}<span class="kind-dot kind-outdated"></span> Устарела{{else if eq .Event.Kind "removed"}}<span class="kind-dot kind-removed"></span> Удалена{{end}}
      </span>
    </td>
    <td><span class="entity-tag">{{.Event.EntityType}}</span></td>
    <td>
      <div style="font-weight:600;font-size:13px">{{.Event.Title}}</div>
      {{if .Event.Summary}}<div style="font-size:11px;color:var(--text-secondary);margin-top:4px">{{.Event.Summary}}</div>{{end}}
      {{if .Tags}}<div class="row-tags">{{range .Tags}}<a class="tag-chip mini{{if eq . $.Tag}} active{{end}}" href="/changes?tag={{. | urlquery}}{{if $.BaseQS}}&{{$.BaseQS}}{{end}}" title="Фильтровать по тегу «{{.}}»">{{.}}</a>{{end}}</div>{{end}}
      <div style="font-size:10px;color:var(--text-secondary);margin-top:2px;font-family:monospace">{{.Event.EntityID}}</div>
    </td>
    <td style="font-size:12px">{{if .Event.Category}}{{.Event.Category}}{{else}}—{{end}}</td>
    <td>
      {{if eq .Event.EntityType "sitepage"}}<a href="/sitepages/{{.Event.EntityID}}" style="font-size:12px;color:var(--primary)" title="Открыть страницу в просмотрщике">просмотр</a><br>{{end}}
      {{if .Event.SourceURL}}<a href="{{.Event.SourceURL}}" target="_blank" rel="noopener" style="font-size:12px;color:var(--primary)" title="Открыть источник">источник ↗</a>{{else}}—{{end}}
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
</div>
{{else}}
<div class="empty">
  <div class="empty-icon">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
  </div>
  <p><strong>Нет изменений</strong></p>
  <p>За указанный период изменений не найдено.<br>
  Попробуйте расширить диапазон дат или сбросить фильтры.</p>
</div>
{{end}}
{{end}}

{{/* ===================== СТРАНИЦЫ ПУБЛИЧНОГО САЙТА ===================== */}}

{{/* Общий стиль для страниц раздела «Страницы сайта» */}}
{{define "sp-style"}}<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f6f7fb; --surface: #fff; --surface-alt: #f0f2f5; --primary: #0073ea; --primary-hover: #005bb5;
  --primary-light: #e5f0fc; --text: #323338; --text-secondary: #676879;
  --border: #c3c6d4; --radius: 8px;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.06);
  --green: #008653; --green-bg: #f4f9f4; --green-border: #b7e4c7;
  --red: #7a0606; --red-bg: #fdf3f3; --red-border: #f5c6c6;
  --gray: #676879; --gray-bg: #f0f2f5;
  --font: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6; --border: #3b3f54;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.3);
  --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
  --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d; --gray: #9698a6; --gray-bg: #2a2f45;
} }
:root[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-alt: #2a2f45; --primary: #579dff; --primary-hover: #7db3ff;
  --primary-light: #1e3050; --text: #d0d1d8; --text-secondary: #9698a6; --border: #3b3f54;
  --shadow-sm: 0 1px 4px rgba(0,0,0,.3);
  --green: #4ade80; --green-bg: #1a2e1a; --green-border: #2d5a2d;
  --red: #ff6b6b; --red-bg: #2e1a1a; --red-border: #5a2d2d; --gray: #9698a6; --gray-bg: #2a2f45;
}
body { font-family: var(--font); background: var(--bg); color: var(--text); line-height: 1.5; }
a { color: var(--primary); text-decoration: none; }
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 8px 28px; display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 56px; flex-wrap: wrap; box-shadow: var(--shadow-sm); position: sticky; top: 0; z-index: 100; }
.logo-wrap { display: flex; align-items: center; gap: 10px; }
.logo-icon { width: 32px; height: 32px; background: var(--primary); border-radius: 8px; display: flex; align-items: center; justify-content: center; }
.logo-icon svg { width: 20px; height: 20px; fill: #fff; }
header h1 { font-size: 16px; font-weight: 700; color: var(--text); }
.header-actions { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.header-actions a, .header-actions button { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); border-radius: 6px; padding: 6px 12px; font-size: 13px; font-weight: 500; cursor: pointer; transition: all .15s; text-decoration: none; font-family: var(--font); display: inline-flex; align-items: center; gap: 5px; }
.header-actions a:hover { background: var(--surface-alt); color: var(--text); }
.header-actions a.active-link { background: var(--primary-light); color: var(--primary); border-color: var(--primary); }
.header-divider { width: 1px; height: 24px; background: var(--border); margin: 0 4px; }
.theme-btn { width: 36px !important; height: 36px !important; padding: 0 !important; display: flex !important; align-items: center; justify-content: center; }
.theme-btn svg { width: 18px; height: 18px; }
main { max-width: 1400px; margin: 0 auto; padding: 24px 28px; }
.parse-info { background: var(--primary-light); border: 1px solid var(--primary); border-radius: var(--radius); padding: 12px 16px; margin-bottom: 16px; display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; font-size: 13px; }
.parse-info .label { font-weight: 600; color: var(--primary); }
.parse-info .time { color: var(--text); font-weight: 500; }
.filter-bar { background: var(--surface); border-radius: var(--radius); padding: 16px; margin-bottom: 16px; box-shadow: var(--shadow-sm); border: 1px solid var(--border); }
.filter-row { display: flex; gap: 12px; flex-wrap: wrap; align-items: flex-end; }
.filter-group { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 140px; }
.filter-group label { font-size: 11px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }
.filter-group input, .filter-group select { padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; font-family: var(--font); background: var(--surface); color: var(--text); }
.filter-actions { display: flex; gap: 8px; align-items: flex-end; }
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 8px 18px; border: none; border-radius: 6px; font-size: 13px; font-weight: 600; cursor: pointer; transition: all .15s; font-family: var(--font); text-decoration: none; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-ghost { background: transparent; color: var(--text-secondary); border: 1px solid var(--border); }
.btn-ghost:hover { background: var(--surface-alt); }
.table-wrap { background: var(--surface); border-radius: var(--radius); box-shadow: var(--shadow-sm); overflow: hidden; border: 1px solid var(--border); }
.table-scroll { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; min-width: 760px; }
thead th { background: var(--surface-alt); padding: 10px 14px; text-align: left; font-size: 12px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; border-bottom: 2px solid var(--border); }
tbody td { padding: 12px 14px; border-bottom: 1px solid var(--border); font-size: 13px; vertical-align: middle; }
tbody tr:hover { background: var(--surface-alt); }
.badge { display: inline-flex; align-items: center; gap: 5px; padding: 3px 10px; border-radius: 20px; font-size: 11px; font-weight: 600; }
.badge-active { background: var(--green-bg); color: var(--green); border: 1px solid var(--green-border); }
.badge-gone { background: var(--red-bg); color: var(--red); border: 1px solid var(--red-border); }
.empty { text-align: center; padding: 48px 24px; color: var(--text-secondary); }
.empty-icon { width: 48px; height: 48px; margin: 0 auto 12px; background: var(--gray-bg); border-radius: 12px; display: flex; align-items: center; justify-content: center; }
.empty-icon svg { width: 24px; height: 24px; stroke: var(--text-secondary); fill: none; stroke-width: 2; }
.act-link { font-size: 12px; color: var(--primary); margin-right: 12px; }
.breadcrumb { font-size: 13px; color: var(--text-secondary); margin-bottom: 6px; }
.page-meta { display: flex; flex-wrap: wrap; gap: 16px; font-size: 13px; color: var(--text-secondary); margin: 12px 0 20px; }
.page-meta b { color: var(--text); font-weight: 600; }
.open-site { display: inline-flex; align-items: center; gap: 8px; background: var(--primary); color: #fff; padding: 10px 20px; border-radius: 8px; font-size: 14px; font-weight: 600; text-decoration: none; }
.open-site:hover { background: var(--primary-hover); color: #fff; }
.text-pane { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 20px 24px; box-shadow: var(--shadow-sm); white-space: pre-wrap; word-break: break-word; font-size: 14px; line-height: 1.7; max-height: 70vh; overflow-y: auto; }
.text-empty { color: var(--text-secondary); font-style: italic; }
.card-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; flex-wrap: wrap; margin-bottom: 8px; }
.card-head h2 { font-size: 22px; font-weight: 700; color: var(--text); }
[data-tooltip] { position: relative; }
[data-tooltip]:hover::after { content: attr(data-tooltip); position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%); background: #1a1a2e; color: #fff; padding: 6px 10px; border-radius: 6px; font-size: 11px; white-space: nowrap; z-index: 999; pointer-events: none; }
/* ── Теги ── */
.sp-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.sp-tag { display: inline-flex; align-items: center; background: var(--primary-light); color: var(--primary); border: 1px solid var(--primary); border-radius: 12px; padding: 1px 9px; font-size: 11px; font-weight: 600; }
.sp-tag.muted { background: var(--gray-bg); color: var(--text-secondary); border-color: var(--border); }
/* ── Мультиселект тегов ── */
.ms { position: relative; }
.ms-toggle { display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); color: var(--text); font-size: 13px; font-family: var(--font); cursor: pointer; text-align: left; }
.ms-toggle .ms-count { background: var(--primary); color: #fff; border-radius: 10px; font-size: 11px; font-weight: 700; padding: 0 7px; }
.ms-panel { display: none; position: absolute; z-index: 200; top: calc(100% + 4px); left: 0; right: 0; max-height: 300px; overflow-y: auto; background: var(--surface); border: 1px solid var(--border); border-radius: 8px; box-shadow: 0 8px 24px rgba(0,0,0,.18); padding: 6px; min-width: 220px; }
.ms-panel.open { display: block; }
.ms-search { width: 100%; padding: 7px 10px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; margin-bottom: 6px; background: var(--surface); color: var(--text); font-family: var(--font); }
.ms-opt { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; font-size: 13px; cursor: pointer; }
.ms-opt:hover { background: var(--surface-alt); }
.ms-opt input { width: auto; margin: 0; }
.ms-empty { padding: 8px; font-size: 12px; color: var(--text-secondary); }
/* ── ИИ-аннотация ── */
.ai-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin: 16px 0; }
@media (max-width: 768px) { .ai-grid { grid-template-columns: 1fr; } }
.ai-card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 16px 18px; box-shadow: var(--shadow-sm); }
.ai-card.span2 { grid-column: 1 / -1; }
.ai-card h3 { font-size: 12px; font-weight: 700; text-transform: uppercase; letter-spacing: .5px; color: var(--text-secondary); margin-bottom: 8px; }
.ai-card p { font-size: 14px; line-height: 1.6; }
.ai-card ul { margin: 0; padding-left: 18px; }
.ai-card li { font-size: 14px; line-height: 1.6; margin-bottom: 4px; }
.ai-empty { background: var(--surface-alt); border: 1px dashed var(--border); border-radius: var(--radius); padding: 14px 18px; font-size: 13px; color: var(--text-secondary); margin: 16px 0; }
.rel-list { display: flex; flex-direction: column; gap: 6px; }
.rel-item { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 8px 10px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface-alt); font-size: 13px; }
.rel-item .rel-sec { color: var(--text-secondary); font-size: 11px; }
.rel-shared { background: var(--primary-light); color: var(--primary); border-radius: 10px; font-size: 11px; font-weight: 700; padding: 0 7px; white-space: nowrap; }
/* ── Ручные действия куратора ── */
.sp-actions { display: flex; gap: 8px; flex-wrap: wrap; margin: 4px 0 16px; }
.btn-success { background: var(--green); color: #fff; }
.btn-success:hover { filter: brightness(1.08); }
.sp-edit { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: var(--shadow-sm); margin-bottom: 16px; }
.sp-edit > summary { cursor: pointer; padding: 12px 16px; font-weight: 600; font-size: 13px; color: var(--text); list-style: none; }
.sp-edit > summary::-webkit-details-marker { display: none; }
.sp-edit > summary::before { content: '✎  '; }
.sp-edit-body { padding: 0 16px 16px; display: flex; flex-direction: column; gap: 10px; }
.sp-edit-body label { font-size: 11px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; margin-bottom: 2px; display: block; }
.sp-edit-body input, .sp-edit-body textarea { width: 100%; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; font-family: var(--font); background: var(--surface); color: var(--text); outline: none; }
.sp-edit-body input:focus, .sp-edit-body textarea:focus { border-color: var(--primary); }
.sp-edit-body textarea { resize: vertical; min-height: 70px; }
.sp-edit-hint { font-size: 11px; color: var(--text-secondary); }
@media (max-width: 768px) { header { padding: 0 16px; } main { padding: 16px; } .filter-row { flex-direction: column; } .filter-group { min-width: 100%; } table { font-size: 12px; } }
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>{{end}}

{{/* Шапка раздела «Страницы сайта» (общая для списка и просмотрщика) */}}
{{define "sp-header"}}<header>
  <div class="logo-wrap">
    <div class="logo-icon"><svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12zM12 5.5v6l3.5-1.75z"/></svg></div>
    <h1>База Сколково</h1>
  </div>
  <div class="header-actions">
    <a href="/">Документы</a>
    <a href="/changes">Изменения</a>
    <a href="/sitepages" class="active-link">Страницы сайта</a>
    <a href="/diff">Сравнение</a>
    <a href="/analytics">Аналитика</a>
    <a href="/graph">Граф</a>
    <a href="/clients">Клиенты</a>
    <div class="header-divider"></div>
    <a href="/logout" style="padding:5px 10px">Выход</a>
    <button class="theme-btn header-actions-btn" onclick="spToggleTheme()" title="Переключить тему">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
    </button>
  </div>
</header>{{end}}

{{define "sp-theme-script"}}<script>
function spToggleTheme(){var r=document.documentElement;var cur=r.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');var next=cur==='dark'?'light':'dark';r.setAttribute('data-theme',next);localStorage.setItem('theme',next);}
function spToggleMs(e){e.stopPropagation();var p=document.getElementById('ms-panel');if(p)p.classList.toggle('open');}
function spFilterMs(v){v=(v||'').toLowerCase();var opts=document.querySelectorAll('#ms-panel .ms-opt');for(var i=0;i<opts.length;i++){var t=opts[i].textContent.toLowerCase();opts[i].style.display=t.indexOf(v)>=0?'':'none';}}
document.addEventListener('click',function(e){var ms=document.getElementById('ms-tags');var p=document.getElementById('ms-panel');if(ms&&p&&!ms.contains(e.target))p.classList.remove('open');});
</script>{{end}}

{{/* ===== СПИСОК СТРАНИЦ САЙТА ===== */}}
{{define "sitepages-layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Страницы сайта — База Сколково</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
{{template "sp-style"}}
</head>
<body>
{{template "sp-header" .}}
<main>{{template "sitepages-content" .}}</main>
{{template "sp-theme-script"}}
</body>
</html>{{end}}

{{define "sitepages-content"}}
<div class="parse-info">
  <div>
    <span class="label">Последний обход сайта:</span>
    {{if .LastCrawl.IsZero}}<span class="time">ещё не выполнялся</span>{{else}}<span class="time" title="Когда краулер последний раз успешно прошёл по сайту">{{.LastCrawl.Format "02.01.2006 15:04"}}</span>{{end}}
  </div>
  <div style="font-size:12px;color:var(--text-secondary)">Показано страниц: <strong>{{.Total}}</strong></div>
</div>

{{if not .HasStore}}
<div class="empty">
  <p><strong>Хранилище страниц сайта не подключено</strong></p>
  <p>Запустите обход командой <code>sitepages crawl</code> или дождитесь планового обхода.</p>
</div>
{{else}}
<div class="filter-bar">
  <form method="get" action="/sitepages">
    <div class="filter-row">
      <div class="filter-group" style="min-width:220px">
        <label for="q">Поиск</label>
        <input type="text" id="q" name="q" value="{{.Query}}" placeholder="По заголовку, URL, разделу…">
      </div>
      <div class="filter-group">
        <label for="section">Раздел</label>
        <select id="section" name="section">
          <option value="">Все разделы</option>
          {{range .Sections}}<option value="{{.}}"{{if eq . $.Section}} selected{{end}}>{{.}}</option>{{end}}
        </select>
      </div>
      <div class="filter-group">
        <label for="status">Статус</label>
        <select id="status" name="status">
          <option value="">Все</option>
          <option value="active"{{if eq .Status "active"}} selected{{end}}>Доступна</option>
          <option value="gone"{{if eq .Status "gone"}} selected{{end}}>Недоступна</option>
        </select>
      </div>
      {{if .AllTags}}
      <div class="filter-group">
        <label>Теги</label>
        <div class="ms" id="ms-tags">
          <button type="button" class="ms-toggle" onclick="spToggleMs(event)" data-tooltip="Фильтр по тегам (страница содержит все выбранные)">
            <span class="ms-label">{{if .SelectedTags}}Выбрано тегов{{else}}Все теги{{end}}</span>
            {{if .SelectedTags}}<span class="ms-count">{{len .SelectedTags}}</span>{{else}}<span style="color:var(--text-secondary)">▾</span>{{end}}
          </button>
          <div class="ms-panel" id="ms-panel">
            <input type="text" class="ms-search" placeholder="Фильтр тегов…" oninput="spFilterMs(this.value)" onclick="event.stopPropagation()">
            {{range .AllTags}}
            <label class="ms-opt"><input type="checkbox" name="tags" value="{{.}}"{{if index $.SelectedSet .}} checked{{end}}><span>{{.}}</span></label>
            {{end}}
          </div>
        </div>
      </div>
      {{end}}
      <div class="filter-group">
        <label for="date_from">Изменено от</label>
        <input type="date" id="date_from" name="date_from" value="{{.DateFrom}}">
      </div>
      <div class="filter-group">
        <label for="date_to">Изменено до</label>
        <input type="date" id="date_to" name="date_to" value="{{.DateTo}}">
      </div>
      <div class="filter-actions">
        <button type="submit" class="btn btn-primary">Применить</button>
        <a href="/sitepages" class="btn btn-ghost">Сбросить</a>
      </div>
    </div>
  </form>
</div>

{{if .Rows}}
<div class="table-wrap"><div class="table-scroll">
<table>
  <thead><tr>
    <th style="width:180px">Раздел</th>
    <th>Заголовок</th>
    <th style="width:120px">Статус</th>
    <th style="width:130px">Обновлено</th>
    <th style="width:200px">Действия</th>
  </tr></thead>
  <tbody>
  {{range .Rows}}
  <tr>
    <td style="font-size:12px;color:var(--text-secondary)">{{if .Section}}{{.Section}}{{else}}—{{end}}</td>
    <td>
      <div style="font-weight:600"><a href="/sitepages/{{.ID}}" title="Открыть в просмотрщике">{{.Title}}</a></div>
      <div style="font-size:11px;color:var(--text-secondary);font-family:monospace;word-break:break-all">{{.URL}}</div>
      {{if .Tags}}<div class="sp-tags">{{range .Tags}}<span class="sp-tag muted">{{.}}</span>{{end}}</div>{{end}}
    </td>
    <td><span class="badge {{if eq .Status "active"}}badge-active{{else}}badge-gone{{end}}">{{.StatusLabel}}</span></td>
    <td style="font-size:12px;white-space:nowrap" title="{{.LastChanged.Format "02.01.2006 15:04:05"}}">{{.LastChanged.Format "02.01.2006"}}</td>
    <td>
      <a class="act-link" href="/sitepages/{{.ID}}" title="Прочитать сохранённую информацию">Просмотр</a>
      <a class="act-link" href="{{.URL}}" target="_blank" rel="noopener" title="Открыть страницу на сайте Сколково">На сайте ↗</a>
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
</div></div>
{{else}}
<div class="empty">
  <div class="empty-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg></div>
  <p><strong>Страниц не найдено</strong></p>
  <p>Измените фильтры или запустите обход сайта.</p>
</div>
{{end}}
{{end}}
{{end}}

{{/* ===== ПРОСМОТРЩИК ОДНОЙ СТРАНИЦЫ ===== */}}
{{define "sitepage-view-layout"}}<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — Страница сайта</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
{{template "sp-style"}}
</head>
<body>
{{template "sp-header" .}}
<main>{{template "sitepage-view-content" .}}</main>
{{template "sp-theme-script"}}
</body>
</html>{{end}}

{{define "sitepage-view-content"}}
<div style="margin-bottom:16px"><a href="/sitepages" class="btn btn-ghost" title="Вернуться к списку страниц">← К списку страниц</a></div>
<div class="breadcrumb">{{if .Section}}{{.Section}}{{else}}Главная{{end}}</div>
<div class="card-head">
  <h2>{{.Title}}</h2>
  <a class="open-site" href="{{.URL}}" target="_blank" rel="noopener" title="Открыть оригинал на сайте Сколково и прочитать всё там">Открыть на сайте Сколково ↗</a>
</div>
<div class="page-meta">
  <span>Статус: <b>{{.StatusLabel}}</b></span>
  <span>Изменено: <b>{{.LastChanged.Format "02.01.2006 15:04"}}</b></span>
  <span>Найдено: <b>{{.FirstSeen.Format "02.01.2006"}}</b></span>
  <span>URL: <a href="{{.URL}}" target="_blank" rel="noopener" style="font-family:monospace;word-break:break-all">{{.URL}}</a></span>
</div>

{{if .Tags}}<div class="sp-tags" style="margin-bottom:16px">{{range .Tags}}<span class="sp-tag">{{.}}</span>{{end}}</div>{{end}}

{{if .Enriched}}
<div class="ai-grid">
  {{if .AISummary}}<div class="ai-card"><h3>Краткое описание</h3><p>{{.AISummary}}</p></div>{{end}}
  {{if .Goals}}<div class="ai-card"><h3>Цели страницы</h3><p>{{.Goals}}</p></div>{{end}}
  {{if .Theses}}<div class="ai-card span2"><h3>Важные тезисы</h3><ul>{{range .Theses}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
  {{if .Conclusions}}<div class="ai-card span2"><h3>Выводы</h3><p>{{.Conclusions}}</p></div>{{end}}
</div>
{{else}}
<div class="ai-empty">ИИ-аннотация (теги, краткое описание, цели, тезисы, выводы) для этой страницы ещё не сформирована — она появится после ближайшего прогона агента «Аннотатор страниц».</div>
{{end}}

{{if .CanEdit}}
<div class="sp-actions">
  <form method="post" action="/sitepages/{{.ID}}/reannotate" style="margin:0">
    <button type="submit" class="btn btn-success" data-tooltip="Заново сгенерировать аннотацию через ИИ">↻ Переаннотировать через ИИ</button>
  </form>
</div>
<details class="sp-edit"{{if not .Enriched}} open{{end}}>
  <summary>Править аннотацию вручную</summary>
  <form method="post" action="/sitepages/{{.ID}}/annotation" class="sp-edit-body">
    <div>
      <label>Теги (через запятую)</label>
      <input type="text" name="tags" value="{{.TagsCSV}}" placeholder="льготы, резиденты, гранты">
    </div>
    <div>
      <label>Краткое описание</label>
      <textarea name="ai_summary" placeholder="1–3 предложения о странице">{{.AISummary}}</textarea>
    </div>
    <div>
      <label>Цели страницы</label>
      <textarea name="goals" placeholder="Зачем эта страница пользователю">{{.Goals}}</textarea>
    </div>
    <div>
      <label>Важные тезисы (по одному на строку)</label>
      <textarea name="theses" placeholder="Тезис 1&#10;Тезис 2">{{.ThesesText}}</textarea>
    </div>
    <div>
      <label>Выводы</label>
      <textarea name="conclusions" placeholder="Главный практический вывод">{{.Conclusions}}</textarea>
    </div>
    <div class="sp-actions" style="margin:4px 0 0">
      <button type="submit" class="btn btn-primary">Сохранить аннотацию</button>
      <span class="sp-edit-hint">Сохранённое переиндексируется в RAG и не перезапишется автоаннотатором, пока не изменится содержимое страницы.</span>
    </div>
  </form>
</details>
{{end}}

{{if .RelatedByTags}}
<div class="ai-card" style="margin-bottom:16px">
  <h3>Связанные страницы по общим тегам</h3>
  <div class="rel-list">
    {{range .RelatedByTags}}<div class="rel-item"><a href="/sitepages/{{.ID}}">{{.Title}}{{if .Section}} <span class="rel-sec">— {{.Section}}</span>{{end}}</a>{{if .Shared}}<span class="rel-shared">{{.Shared}} общих</span>{{end}}</div>{{end}}
  </div>
</div>
{{end}}

{{if .RelatedSemantic}}
<div class="ai-card" style="margin-bottom:16px">
  <h3>Похожие страницы по смыслу</h3>
  <div class="rel-list">
    {{range .RelatedSemantic}}<div class="rel-item"><a href="/sitepages/{{.ID}}">{{.Title}}{{if .Section}} <span class="rel-sec">— {{.Section}}</span>{{end}}</a></div>{{end}}
  </div>
</div>
{{end}}

{{if .HasText}}
<div class="text-pane">{{.Text}}</div>
{{else}}
<div class="text-pane text-empty">Текст страницы не сохранён. Откройте оригинал на сайте Сколково по кнопке выше.</div>
{{end}}
{{end}}
`))
