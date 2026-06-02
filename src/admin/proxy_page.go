package admin

import (
	"fmt"
	"net/http"
	"strings"
)

// handleProxyPage отображает страницу управления прокси
func (s *Server) handleProxyPage(w http.ResponseWriter, r *http.Request) {
	s.proxyManager.mu.Lock()
	proxies := s.proxyManager.Proxies
	activeID := s.proxyManager.CurrentID
	s.proxyManager.mu.Unlock()

	cookieMasked, cookieSavedAt := "", ""
	if s.cookieStore != nil {
		cookieMasked = s.cookieStore.Masked()
		cookieSavedAt = s.cookieStore.SavedAtStr()
	}

	data := map[string]interface{}{
		"Proxies":       proxies,
		"ActiveID":      activeID,
		"Page":          "proxy",
		"Title":         "Управление прокси и VPN",
		"CookieMasked":  cookieMasked,
		"CookieSavedAt": cookieSavedAt,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderProxyPage(data))
}

// proxyStatusBadge возвращает класс и подпись для статуса прокси.
func proxyStatusBadge(status string) (string, string) {
	switch status {
	case "ok":
		return "s-ok", "Работает"
	case "error":
		return "s-err", "Ошибка"
	default:
		return "s-unknown", "Не проверен"
	}
}

// proxyTypeLabel переводит технический тип прокси в человекочитаемую подпись.
func proxyTypeLabel(t string) string {
	switch t {
	case "http":
		return "HTTP-прокси"
	case "socks5":
		return "SOCKS5-прокси"
	case "ovpn_file":
		return "Файл OVPN (VPN)"
	case "api":
		return "API-прокси"
	default:
		return t
	}
}

func renderProxyPage(data map[string]interface{}) string {
	proxies := data["Proxies"].([]ProxyConfig)
	activeID := data["ActiveID"].(string)

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Прокси и VPN</title>
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
  --gray: #9698a6; --gray-bg: #2a2f45;
}
body { font-family: var(--font); background: var(--bg); color: var(--text); line-height: 1.5; }

/* Header */
header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 0 28px; display: flex; align-items: center; justify-content: space-between; height: 56px; box-shadow: var(--shadow-sm); position: sticky; top: 0; z-index: 100; }
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
.header-actions svg { width: 15px; height: 15px; }
.theme-btn {
  background: transparent; color: var(--text-secondary);
  border: 1px solid var(--border); border-radius: 6px;
  width: 36px; height: 36px; padding: 0;
  display: flex; align-items: center; justify-content: center; cursor: pointer; transition: all .15s;
}
.theme-btn:hover { background: var(--surface-alt); color: var(--text); }
.theme-btn svg { width: 18px; height: 18px; }

/* Main */
main { max-width: 1200px; margin: 0 auto; padding: 24px 28px; }
.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 20px; margin-bottom: 16px; box-shadow: var(--shadow-sm); }
.card-head { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; }
.card-head svg { width: 18px; height: 18px; color: var(--primary); flex-shrink: 0; }
h2 { font-size: 16px; font-weight: 700; color: var(--text); }

/* Form */
.form-group { margin-bottom: 14px; }
.form-group label { display: block; font-size: 13px; font-weight: 600; color: var(--text); margin-bottom: 6px; }
.form-group input, .form-group select { width: 100%; padding: 9px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; outline: none; transition: border-color .15s, box-shadow .15s; font-family: var(--font); background: var(--surface); color: var(--text); }
.form-group input:focus, .form-group select:focus { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(0,115,234,.12); }

/* Proxy list */
.proxy-list { display: flex; flex-direction: column; gap: 12px; }
.proxy-item { display: flex; align-items: center; gap: 16px; padding: 16px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); transition: border-color .15s, box-shadow .15s; }
.proxy-item:hover { box-shadow: var(--shadow-sm); }
.proxy-item.active { border-color: var(--green); background: var(--green-bg); }
.proxy-info { flex: 1; min-width: 0; }
.proxy-name { font-weight: 600; color: var(--text); display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.proxy-name .active-mark { display: inline-flex; align-items: center; gap: 4px; font-size: 11px; font-weight: 600; color: var(--green); background: var(--green-bg); border: 1px solid var(--green-border); border-radius: 20px; padding: 1px 8px; }
.proxy-name .active-mark svg { width: 12px; height: 12px; }
.proxy-type { display: inline-block; font-size: 11px; font-weight: 600; color: var(--primary); background: var(--primary-light); padding: 2px 8px; border-radius: 12px; margin-top: 6px; }
.proxy-url { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 11px; color: var(--text-secondary); margin-top: 6px; word-break: break-all; }
.proxy-status { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; font-weight: 600; margin-top: 6px; padding: 2px 10px; border-radius: 20px; }
.proxy-status .dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
.proxy-status.s-ok { color: var(--green); background: var(--green-bg); border: 1px solid var(--green-border); }
.proxy-status.s-ok .dot { background: var(--green); }
.proxy-status.s-err { color: var(--red); background: var(--red-bg); border: 1px solid var(--red-border); }
.proxy-status.s-err .dot { background: var(--red); }
.proxy-status.s-unknown { color: var(--text-secondary); background: var(--gray-bg); border: 1px solid var(--border); }
.proxy-status.s-unknown .dot { background: var(--text-secondary); }
.proxy-checked { font-size: 11px; color: var(--text-secondary); margin-top: 4px; }
.proxy-actions { display: flex; gap: 8px; flex-shrink: 0; }

/* Buttons */
.btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 8px 14px; border: none; border-radius: 6px; cursor: pointer; font-size: 13px; font-weight: 500; transition: all .15s; font-family: var(--font); white-space: nowrap; }
.btn svg { width: 14px; height: 14px; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-success { background: var(--green); color: #fff; }
.btn-success:hover { opacity: .88; }
.btn-danger { background: var(--red); color: #fff; }
.btn-danger:hover { opacity: .88; }
.btn-outline { background: transparent; border: 1px solid var(--border); color: var(--text); }
.btn-outline:hover { background: var(--surface-alt); border-color: var(--text-secondary); }
.btn-icon { width: 36px; padding: 0; height: 36px; }

/* Empty */
.empty { text-align: center; padding: 40px 20px; color: var(--text-secondary); }

/* Toast */
#toast { position: fixed; bottom: 24px; right: 24px; padding: 12px 20px; border-radius: var(--radius); color: #fff; font-size: 13px; font-weight: 500; box-shadow: var(--shadow-lg); z-index: 1000; transform: translateY(100px); opacity: 0; transition: all .3s ease; max-width: 320px; word-wrap: break-word; }
#toast.show { transform: translateY(0); opacity: 1; }
#toast.ok { background: var(--green); }
#toast.err { background: var(--red); }
.spinner { display: inline-block; width: 14px; height: 14px; border: 2px solid rgba(255,255,255,.4); border-top-color: #fff; border-radius: 50%; animation: spin .6s linear infinite; }
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
@media (max-width: 768px) {
  header { padding: 0 16px; }
  main { padding: 16px; }
  .header-actions a span { display: none; }
  .proxy-item { flex-direction: column; align-items: stretch; }
  .proxy-actions { flex-wrap: wrap; }
}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
` + sidebarMainHTML + `
<main>
  <!-- Автопоиск российского прокси -->
  <div class="card" style="border-left:4px solid #e63946;margin-bottom:16px">
    <div class="card-head">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
      <h2>Российский прокси (для dochub.sk.ru)</h2>
    </div>
    <p style="font-size:13px;color:var(--text-secondary);margin-bottom:16px">
      Файлы на dochub.sk.ru доступны только с российских IP. Введите API-ключ
      <a href="https://proxy6.net" target="_blank" rel="noopener" style="color:var(--primary)">proxy6.net</a>
      (от ~0.5$/IP/мес) или нажмите «Найти бесплатный» — система протестирует
      открытые российские прокси против dochub.sk.ru.
    </p>
    <div style="display:flex;gap:12px;flex-wrap:wrap;align-items:flex-end">
      <div class="form-group" style="flex:1;min-width:260px;margin:0">
        <label for="proxy6key" style="font-size:12px;font-weight:600;color:var(--text-secondary)">API-ключ proxy6.net (необязательно)</label>
        <input type="text" id="proxy6key" placeholder="ВАШИ_КЛЮЧ_proxy6.net"
               style="width:100%;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;background:var(--surface);color:var(--text)"
               data-tooltip="Получить на proxy6.net → Профиль → API. Если пусто — используются бесплатные прокси.">
      </div>
      <button onclick="findRussianProxy(this)" class="btn btn-primary"
              style="background:#e63946;white-space:nowrap"
              data-tooltip="Автоматически найти и активировать рабочий российский прокси">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="16" height="16"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        Найти российский прокси
      </button>
    </div>
    <div id="findResult" style="margin-top:12px;font-size:13px;display:none"></div>
  </div>

  <div class="card">
    <div class="card-head">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      <h2>Добавить прокси или VPN вручную</h2>
    </div>
    <form id="addForm" onsubmit="addProxy(event)">
      <div class="form-group">
        <label for="addType">Тип подключения</label>
        <select id="addType" onchange="toggleFields()" data-tooltip="Выберите способ подключения к прокси или VPN">
          <option value="http">HTTP-прокси</option>
          <option value="socks5">SOCKS5-прокси</option>
          <option value="ovpn_file">Файл OVPN (VPN)</option>
        </select>
      </div>
      <div class="form-group">
        <label for="addName">Название</label>
        <input type="text" id="addName" placeholder="Например: Webshare США" required data-tooltip="Понятное имя для списка прокси">
      </div>
      <div class="form-group" id="urlGroup">
        <label for="addUrl">Адрес прокси</label>
        <input type="text" id="addUrl" placeholder="http://логин:пароль@proxy.example.com:8080" data-tooltip="Адрес в формате http://логин:пароль@хост:порт">
      </div>
      <div class="form-group" id="fileGroup" style="display:none;">
        <label for="addFile">Файл OVPN</label>
        <input type="file" id="addFile" accept=".ovpn,.conf" data-tooltip="Конфигурационный файл OpenVPN (.ovpn или .conf)">
      </div>
      <button type="submit" class="btn btn-primary" data-tooltip="Сохранить новое подключение в список">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        Добавить
      </button>
    </form>
  </div>

  <div class="card">
    <div class="card-head">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>
      <h2>Список прокси</h2>
    </div>
    <div class="proxy-list" id="proxyList">
`
	if len(proxies) == 0 {
		html += `      <div class="empty">Нет добавленных прокси. Добавьте первое подключение в форме выше.</div>` + "\n"
	}
	for _, p := range proxies {
		activeClass := ""
		if p.ID == activeID {
			activeClass = " active"
		}
		statusClass, statusLabel := proxyStatusBadge(p.Status)
		activeMark := ""
		if p.Active {
			activeMark = `<span class="active-mark"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>Активен</span>`
		}
		checked := ""
		if p.LastCheck != "" {
			checked = fmt.Sprintf(`<div class="proxy-checked">Проверен: %s</div>`, p.LastCheck)
		}
		activateBtn := ""
		if !p.Active {
			activateBtn = fmt.Sprintf(`<button class="btn btn-success" onclick="activateProxy('%s')" data-tooltip="Сделать этот прокси активным">`+
				`<svg viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>Активировать</button>`, p.ID)
		}
		html += fmt.Sprintf(`
      <div class="proxy-item%s" id="proxy-%s">
        <div class="proxy-info">
          <div class="proxy-name">%s %s</div>
          <span class="proxy-type">%s</span>
          <div class="proxy-url">%s</div>
          <div class="proxy-status %s"><span class="dot"></span>%s</div>
          %s
        </div>
        <div class="proxy-actions">
          <button class="btn btn-outline" onclick="testProxy('%s')" data-tooltip="Проверить доступность прокси и определить IP">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>Тест
          </button>
          %s
          <button class="btn btn-danger btn-icon" onclick="removeProxy('%s')" data-tooltip="Удалить прокси из списка">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
          </button>
        </div>
      </div>
`, activeClass, p.ID, p.Name, activeMark, proxyTypeLabel(p.Type), p.URL, statusClass, statusLabel, checked, p.ID, activateBtn, p.ID)
	}

	html += `
    </div>
    <div style="margin-top: 16px;">
      <button class="btn btn-primary" onclick="autoSwitch(this)" data-tooltip="Проверить все прокси и автоматически выбрать рабочий">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
        Автопереключение на рабочий
      </button>
    </div>
  </div>
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

function toggleFields() {
  const type = document.getElementById('addType').value;
  document.getElementById('urlGroup').style.display = type === 'ovpn_file' ? 'none' : 'block';
  document.getElementById('fileGroup').style.display = type === 'ovpn_file' ? 'block' : 'none';
}

async function addProxy(e) {
  e.preventDefault();
  const type = document.getElementById('addType').value;
  const name = document.getElementById('addName').value;

  if (type === 'ovpn_file') {
    const file = document.getElementById('addFile').files[0];
    if (!file) return toast('Выберите файл OVPN', 'err');
    const formData = new FormData();
    formData.append('ovpn', file);
    const res = await fetch('/api/proxy/upload-ovpn', { method: 'POST', body: formData });
    if (res.ok) {
      toast('Файл OVPN загружен', 'ok');
      setTimeout(function() { location.reload(); }, 800);
    } else {
      toast('Ошибка загрузки файла', 'err');
    }
  } else {
    const url = document.getElementById('addUrl').value;
    if (!url) return toast('Введите адрес прокси', 'err');
    const res = await fetch('/api/proxy/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, type, url })
    });
    if (res.ok) {
      toast('Прокси добавлен', 'ok');
      setTimeout(function() { location.reload(); }, 800);
    } else {
      toast('Ошибка добавления', 'err');
    }
  }
}

async function activateProxy(id) {
  const res = await fetch('/api/proxy/activate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  });
  if (res.ok) { toast('Прокси активирован', 'ok'); setTimeout(function() { location.reload(); }, 800); }
  else { toast('Ошибка активации', 'err'); }
}

async function removeProxy(id) {
  if (!confirm('Удалить этот прокси? Действие нельзя отменить.')) return;
  const res = await fetch('/api/proxy/remove', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  });
  if (res.ok) { toast('Прокси удалён', 'ok'); setTimeout(function() { location.reload(); }, 800); }
  else { toast('Ошибка удаления', 'err'); }
}

async function testProxy(id) {
  const btn = event.currentTarget;
  const orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span> Проверка…';
  btn.disabled = true;
  try {
    const res = await fetch('/api/proxy/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id })
    });
    const data = await res.json();
    if (data.ok) { toast('Прокси работает. IP: ' + data.ip, 'ok'); }
    else { toast('Ошибка: ' + (data.error || 'неизвестная ошибка'), 'err'); }
    setTimeout(function() { location.reload(); }, 1000);
  } catch (err) {
    toast('Ошибка сети: ' + err.message, 'err');
    btn.innerHTML = orig;
    btn.disabled = false;
  }
}

async function autoSwitch(btn) {
  const orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span> Подбор рабочего…';
  btn.disabled = true;
  try {
    const res = await fetch('/api/proxy/auto-switch', { method: 'POST' });
    const data = await res.json();
    if (data.active_id) { toast('Переключено на прокси: ' + data.active_id, 'ok'); setTimeout(function() { location.reload(); }, 1000); }
    else { toast('Не найдено рабочих прокси', 'err'); btn.innerHTML = orig; btn.disabled = false; }
  } catch (err) {
    toast('Ошибка сети: ' + err.message, 'err');
    btn.innerHTML = orig;
    btn.disabled = false;
  }
}

async function findRussianProxy(btn) {
  const key = document.getElementById('proxy6key').value.trim();
  const orig = btn.innerHTML;
  btn.innerHTML = '<span class="spinner"></span> Ищу российский прокси…';
  btn.disabled = true;
  const result = document.getElementById('findResult');
  result.style.display = 'block';
  result.style.color = 'var(--text-secondary)';
  result.innerHTML = '⏳ Проверяю прокси против dochub.sk.ru… (может занять 30–60 сек)';
  try {
    const body = key ? JSON.stringify({proxy6_key: key}) : '{}';
    const res = await fetch('/api/proxy/find-russian', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: body
    });
    const data = await res.json();
    if (data.ok) {
      result.style.color = 'var(--success, #00875a)';
      result.innerHTML = '✅ ' + (data.msg || 'Прокси найден и активирован!');
      setTimeout(function() { location.reload(); }, 1500);
    } else {
      result.style.color = 'var(--danger, #de350b)';
      result.innerHTML = '❌ ' + (data.error || 'Не удалось найти рабочий прокси');
      btn.innerHTML = orig;
      btn.disabled = false;
    }
  } catch (err) {
    result.style.color = 'var(--danger, #de350b)';
    result.innerHTML = '❌ Ошибка сети: ' + err.message;
    btn.innerHTML = orig;
    btn.disabled = false;
  }
}
</script>
</body>
</html>`

	// Карточка куки dochub — основной способ скачивания файлов (HTTP по куке).
	cookieMasked, _ := data["CookieMasked"].(string)
	cookieSavedAt, _ := data["CookieSavedAt"].(string)
	cookieStatus := `<span style="color:var(--text-secondary)">не задана</span>`
	if cookieMasked != "" {
		cookieStatus = `задана: <code>` + cookieMasked + `</code>`
		if cookieSavedAt != "" {
			cookieStatus += ` · обновлена ` + cookieSavedAt
		}
	}
	cookieCard := `
  <div class="card" style="border-left:4px solid #008653;margin-bottom:16px">
    <div class="card-head">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8.5 8.5v.01M16 15.5v.01M11 12v.01M12 17v.01M7 14v.01"/></svg>
      <h2>Кука dochub — скачивание файлов (рекомендуется)</h2>
    </div>
    <p style="font-size:13px;color:var(--text-secondary);margin-bottom:12px">
      Надёжный способ забрать тела файлов: вставьте сессионную куку из браузера, который уже открыл dochub.
      Качается обычным HTTP — без браузера и без прокси (кука работает с любого IP).
      Где взять: F12 → вкладка <b>Сеть</b> → обновите страницу dochub → клик по запросу страницы →
      <b>Заголовки запроса</b> → скопируйте всю строку <code>cookie</code>. Кука периодически протухает —
      обновите её, когда «Скачать файлы» начнёт давать 403.
    </p>
    <div style="font-size:13px;margin-bottom:10px">Текущая кука: ` + cookieStatus + `</div>
    <textarea id="cookieInput" rows="3" placeholder="spid=...; sk_lang=ru; AuthorizationCookie=..." style="width:100%;font-family:monospace;font-size:12px;padding:8px;border:1px solid var(--border);border-radius:6px;resize:vertical"></textarea>
    <div style="margin-top:10px;display:flex;gap:8px;flex-wrap:wrap">
      <button onclick="saveCookie(this)" style="padding:8px 16px;background:var(--primary);color:#fff;border:none;border-radius:6px;cursor:pointer;font-weight:600">Сохранить куку</button>
      <button onclick="fetchFiles(this)" style="padding:8px 16px;background:var(--green);color:#fff;border:none;border-radius:6px;cursor:pointer;font-weight:600">Скачать файлы</button>
    </div>
    <div id="cookieResult" style="margin-top:10px;font-size:13px"></div>
  </div>
`
	cookieJS := `
async function saveCookie(btn){
  var c=document.getElementById('cookieInput').value.trim();
  var r=document.getElementById('cookieResult');
  if(!c){ r.style.color='var(--danger,#de350b)'; r.innerHTML='Вставьте куку'; return; }
  var orig=btn.innerHTML; btn.disabled=true; btn.innerHTML='Сохраняю…';
  try{
    var res=await fetch('/api/dochub-cookie',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({cookie:c})});
    var d=await res.json();
    r.style.color=d.ok?'var(--green)':'var(--danger,#de350b)';
    r.innerHTML=(d.ok?'✅ ':'❌ ')+(d.msg||d.error||'');
    if(d.ok) setTimeout(function(){location.reload()},1200);
  }catch(e){ r.style.color='var(--danger,#de350b)'; r.innerHTML='❌ '+e.message; }
  btn.disabled=false; btn.innerHTML=orig;
}
async function fetchFiles(btn){
  var r=document.getElementById('cookieResult');
  var orig=btn.innerHTML; btn.disabled=true; btn.innerHTML='Запускаю…';
  try{
    var res=await fetch('/api/fetch',{method:'POST'});
    var d=await res.json();
    r.style.color=d.ok?'var(--green)':'var(--danger,#de350b)';
    r.innerHTML=(d.ok?'✅ ':'❌ ')+(d.msg||d.error||'');
  }catch(e){ r.style.color='var(--danger,#de350b)'; r.innerHTML='❌ '+e.message; }
  btn.disabled=false; btn.innerHTML=orig;
}
`
	html = strings.Replace(html, "<main>", "<main>"+cookieCard, 1)
	html = strings.Replace(html, "</script>\n</body>", cookieJS+"</script>\n</body>", 1)
	return html
}
