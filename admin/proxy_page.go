package admin

import (
	"fmt"
	"net/http"
)

// handleProxyPage отображает страницу управления прокси
func (s *Server) handleProxyPage(w http.ResponseWriter, r *http.Request) {
	s.proxyManager.mu.Lock()
	proxies := s.proxyManager.Proxies
	activeID := s.proxyManager.CurrentID
	s.proxyManager.mu.Unlock()

	data := map[string]interface{}{
		"Proxies":  proxies,
		"ActiveID": activeID,
		"Page":     "proxy",
		"Title":    "Управление прокси и VPN",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderProxyPage(data))
}

func renderProxyPage(data map[string]interface{}) string {
	proxies := data["Proxies"].([]ProxyConfig)
	activeID := data["ActiveID"].(string)

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Прокси</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root { --bg: #f0f2f5; --surface: #fff; --primary: #1e40af; --primary-hover: #1e3a8a; --primary-light: #eff6ff; --text: #1e293b; --text-secondary: #64748b; --border: #e2e8f0; --radius: 8px; --shadow: 0 1px 3px rgba(0,0,0,.08); --green: #16a34a; --green-bg: #f0fdf4; --red: #dc2626; --red-bg: #fef2f2; --yellow: #ca8a04; --yellow-bg: #fefce8; --blue: #2563eb; }
body { font-family: 'Inter', sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; }
header { background: linear-gradient(135deg, var(--primary), #3b82f6); color: #fff; padding: 16px 28px; display: flex; align-items: center; justify-content: space-between; }
header h1 { font-size: 18px; font-weight: 600; }
.nav-btn { color: #fff; text-decoration: none; padding: 8px 16px; border-radius: 6px; font-size: 13px; }
.nav-btn:hover { background: rgba(255,255,255,.15); }
main { max-width: 1200px; margin: 24px auto; padding: 0 28px; }
.card { background: var(--surface); border-radius: var(--radius); padding: 20px; margin-bottom: 16px; box-shadow: var(--shadow); }
h2 { font-size: 16px; font-weight: 600; margin-bottom: 12px; }
.proxy-list { display: flex; flex-direction: column; gap: 12px; }
.proxy-item { display: flex; align-items: center; gap: 16px; padding: 16px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); }
.proxy-item.active { border-color: var(--green); background: var(--green-bg); }
.proxy-info { flex: 1; }
.proxy-name { font-weight: 600; }
.proxy-type { font-size: 12px; color: var(--text-secondary); background: var(--primary-light); padding: 2px 8px; border-radius: 12px; }
.proxy-url { font-family: monospace; font-size: 11px; color: var(--text-secondary); margin-top: 4px; word-break: break-all; }
.proxy-status { font-size: 12px; margin-top: 4px; }
.status-ok { color: var(--green); }
.status-error { color: var(--red); }
.status-unknown { color: var(--text-secondary); }
.actions { display: flex; gap: 8px; }
.btn { padding: 8px 16px; border: none; border-radius: 6px; cursor: pointer; font-size: 13px; font-weight: 500; transition: all .15s; }
.btn-primary { background: var(--primary); color: #fff; }
.btn-primary:hover { background: var(--primary-hover); }
.btn-success { background: var(--green); color: #fff; }
.btn-danger { background: var(--red); color: #fff; }
.btn-outline { background: transparent; border: 1px solid var(--border); color: var(--text); }
.btn-outline:hover { background: var(--primary-light); }
.form-group { margin-bottom: 16px; }
.form-group label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 4px; }
.form-group input, .form-group select { width: 100%; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; font-size: 13px; }
</style>
</head>
<body>
<header>
  <h1> Управление прокси и VPN</h1>
  <div>
    <a href="/" class="nav-btn">📚 Документы</a>
    <a href="/ai/models" class="nav-btn">🤖 ИИ Модели</a>
  </div>
</header>
<main>
  <div class="card">
    <h2>➕ Добавить прокси/VPN</h2>
    <form id="addForm" onsubmit="addProxy(event)">
      <div class="form-group">
        <label>Тип</label>
        <select id="addType" onchange="toggleFields()">
          <option value="http">HTTP Proxy</option>
          <option value="socks5">SOCKS5 Proxy</option>
          <option value="ovpn_file">OVPN Файл (VPN)</option>
        </select>
      </div>
      <div class="form-group">
        <label>Название</label>
        <input type="text" id="addName" placeholder="Например: Webshare US" required>
      </div>
      <div class="form-group" id="urlGroup">
        <label>URL прокси (http://user:pass@host:port)</label>
        <input type="text" id="addUrl" placeholder="http://user:pass@proxy.example.com:8080">
      </div>
      <div class="form-group" id="fileGroup" style="display:none;">
        <label>OVPN файл</label>
        <input type="file" id="addFile" accept=".ovpn,.conf">
      </div>
      <button type="submit" class="btn btn-primary">Добавить</button>
    </form>
  </div>

  <div class="card">
    <h2>📋 Список прокси</h2>
    <div class="proxy-list" id="proxyList">
`
	for _, p := range proxies {
		activeClass := ""
		if p.ID == activeID {
			activeClass = "active"
		}
		statusClass := "status-unknown"
		if p.Status == "ok" {
			statusClass = "status-ok"
		} else if p.Status == "error" {
			statusClass = "status-error"
		}
		html += fmt.Sprintf(`
      <div class="proxy-item %s" id="proxy-%s">
        <div class="proxy-info">
          <div class="proxy-name">%s %s</div>
          <span class="proxy-type">%s</span>
          <div class="proxy-url">%s</div>
          <div class="proxy-status %s">Статус: %s</div>
          %s
        </div>
        <div class="actions">
          <button class="btn btn-outline" onclick="testProxy('%s')">🧪 Тест</button>
          %s
          <button class="btn btn-danger" onclick="removeProxy('%s')">🗑</button>
        </div>
      </div>
`, activeClass, p.ID, p.Name, func() string {
			if p.Active {
				return "✅ Активен"
			} else {
				return ""
			}
		}(), p.Type, p.URL, statusClass, p.Status, func() string {
			if p.LastCheck != "" {
				return fmt.Sprintf("<div>Проверен: %s</div>", p.LastCheck)
			}
			return ""
		}(), p.ID, func() string {
			if !p.Active {
				return fmt.Sprintf(`<button class="btn btn-success" onclick="activateProxy('%s')">▶ Активировать</button>`, p.ID)
			}
			return ""
		}(), p.ID)
	}

	html += `
    </div>
    <div style="margin-top: 16px;">
      <button class="btn btn-primary" onclick="autoSwitch()">🔄 Автопереключение на рабочий</button>
    </div>
  </div>
</main>

<script>
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
    if (!file) return alert('Выберите файл');
    const formData = new FormData();
    formData.append('ovpn', file);
    const res = await fetch('/api/proxy/upload-ovpn', { method: 'POST', body: formData });
    if (res.ok) {
      alert('OVPN файл загружен');
      location.reload();
    } else {
      alert('Ошибка загрузки');
    }
  } else {
    const url = document.getElementById('addUrl').value;
    if (!url) return alert('Введите URL');
    const res = await fetch('/api/proxy/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, type, url })
    });
    if (res.ok) {
      alert('Прокси добавлен');
      location.reload();
    } else {
      alert('Ошибка добавления');
    }
  }
}

async function activateProxy(id) {
  const res = await fetch('/api/proxy/activate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  });
  if (res.ok) location.reload();
}

async function removeProxy(id) {
  if (!confirm('Удалить этот прокси?')) return;
  const res = await fetch('/api/proxy/remove', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  });
  if (res.ok) location.reload();
}

async function testProxy(id) {
  const btn = event.target;
  btn.textContent = ' Тестирую...';
  btn.disabled = true;
  const res = await fetch('/api/proxy/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id })
  });
  const data = await res.json();
  if (data.ok) {
    alert('✅ Прокси работает! IP: ' + data.ip);
  } else {
    alert('❌ Ошибка: ' + (data.error || 'Неизвестная ошибка'));
  }
  location.reload();
}

async function autoSwitch() {
  const res = await fetch('/api/proxy/auto-switch', { method: 'POST' });
  const data = await res.json();
  if (data.active_id) {
    alert('🔄 Переключено на прокси: ' + data.active_id);
    location.reload();
  } else {
    alert('❌ Не найдено рабочих прокси');
  }
}
</script>
</body>
</html>`
	return html
}
