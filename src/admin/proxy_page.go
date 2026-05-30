// proxy_page.go — страница управления прокси в админке.
package admin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"time"
)

type proxyPageData struct {
	Proxies     []ProxyStatus
	CurrentType string
	Flash       string
	FlashKind   string
}

const proxyPageHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Управление прокси</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --primary: #0073ea;
  --primary-light: #e6f2fc;
  --bg: #ffffff;
  --surface: #f6f8fa;
  --surface-border: #d1d5da;
  --text: #24292e;
  --text-secondary: #586069;
  --ok-bg: #e6ffed;
  --ok-text: #22863a;
  --ok-border: #34d058;
  --warn-bg: #ffeef0;
  --warn-text: #b31d28;
  --warn-border: #f97583;
  --shadow: 0 1px 3px rgba(0,0,0,0.08);
  --radius: 8px;
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    --bg: #181b2b; --surface: #23273a; --surface-border: #3a3f52;
    --text: #e6e6e6; --text-secondary: #a0a6b8; --primary-light: #1a2d42;
    --ok-bg: #1a2e23; --ok-text: #6ecb7e; --ok-border: #2d8a3e;
    --warn-bg: #2e1a1e; --warn-text: #f08a94; --warn-border: #c04a54;
    --shadow: 0 1px 3px rgba(0,0,0,0.3);
  }
}
[data-theme="dark"] {
  --bg: #181b2b; --surface: #23273a; --surface-border: #3a3f52;
  --text: #e6e6e6; --text-secondary: #a0a6b8; --primary-light: #1a2d42;
  --ok-bg: #1a2e23; --ok-text: #6ecb7e; --ok-border: #2d8a3e;
  --warn-bg: #2e1a1e; --warn-text: #f08a94; --warn-border: #c04a54;
  --shadow: 0 1px 3px rgba(0,0,0,0.3);
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: 'Figtree', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: var(--bg); color: var(--text); line-height: 1.6; padding: 24px;
}
.container { max-width: 800px; margin: 0 auto; }
.theme-toggle {
  position: fixed; top: 16px; right: 16px; background: var(--surface);
  border: 1px solid var(--surface-border); border-radius: var(--radius);
  padding: 8px 14px; cursor: pointer; color: var(--text); font-family: inherit;
  font-size: 13px; font-weight: 500; box-shadow: var(--shadow); z-index: 10;
}
.theme-toggle:hover { border-color: var(--primary); }
.page-header { margin-bottom: 24px; }
.page-header h1 { font-size: 22px; font-weight: 700; color: var(--primary); margin-bottom: 4px; }
.page-header p { font-size: 14px; color: var(--text-secondary); }
.flash { padding: 12px 16px; border-radius: var(--radius); margin-bottom: 16px; font-size: 13px; font-weight: 500; }
.flash-ok { background: var(--ok-bg); color: var(--ok-text); border: 1px solid var(--ok-border); }
.flash-error { background: var(--warn-bg); color: var(--warn-text); border: 1px solid var(--warn-border); }
.proxy-list { display: flex; flex-direction: column; gap: 12px; }
.proxy-card {
  background: var(--surface); border: 1px solid var(--surface-border);
  border-radius: var(--radius); padding: 16px 18px; box-shadow: var(--shadow);
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  flex-wrap: wrap;
}
.proxy-card.current { border-color: var(--primary); border-left: 3px solid var(--primary); }
.proxy-info { flex: 1; min-width: 200px; }
.proxy-label { font-size: 15px; font-weight: 600; color: var(--text); }
.proxy-type { font-size: 12px; color: var(--text-secondary); font-family: monospace; }
.proxy-status { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
.status-badge {
  display: inline-flex; align-items: center; padding: 4px 12px;
  border-radius: 12px; font-size: 12px; font-weight: 600;
}
.status-ok { background: var(--ok-bg); color: var(--ok-text); }
.status-error { background: var(--warn-bg); color: var(--warn-text); }
.status-pending { background: var(--surface); color: var(--text-secondary); }
.proxy-ip { font-size: 12px; color: var(--text-secondary); font-family: monospace; }
.proxy-latency { font-size: 12px; color: var(--text-secondary); }
.btn {
  display: inline-flex; align-items: center; justify-content: center;
  padding: 6px 14px; border-radius: 6px; font-size: 13px; font-weight: 500;
  cursor: pointer; border: 1px solid transparent; transition: all 0.15s;
  font-family: inherit;
}
.btn-primary { background: var(--primary); color: #fff; border-color: var(--primary); }
.btn-primary:hover { opacity: 0.9; }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-outline { background: transparent; color: var(--primary); border-color: var(--primary); }
.btn-outline:hover { background: var(--primary-light); }
.btn-sm { padding: 4px 10px; font-size: 12px; }
.actions { display: flex; gap: 8px; margin-top: 24px; flex-wrap: wrap; }
.current-badge {
  font-size: 11px; font-weight: 600; color: var(--primary);
  background: var(--primary-light); padding: 2px 8px; border-radius: 8px;
}
</style>
</head>
<body>
<div class="container">
<button class="theme-toggle" onclick="toggleTheme()">Сменить тему</button>
<script>
(function(){var t=localStorage.getItem('proxy-theme');if(t){document.documentElement.setAttribute('data-theme',t)}else if(window.matchMedia('(prefers-color-scheme:dark)').matches){document.documentElement.setAttribute('data-theme','dark')}})();
function toggleTheme(){var c=document.documentElement.getAttribute('data-theme');var n=c==='dark'?'light':'dark';document.documentElement.setAttribute('data-theme',n);localStorage.setItem('proxy-theme',n)}
</script>
<div class="page-header">
<h1>Управление прокси</h1>
<p>Настройка и проверка прокси-серверов для обхода WAF</p>
</div>
{{if .Flash}}<div class="flash flash-{{.FlashKind}}">{{.Flash}}</div>{{end}}
<div class="actions">
<button class="btn btn-outline" onclick="testAll()">Тестировать все</button>
</div>
<div class="proxy-list" id="proxyList">
{{range .Proxies}}
<div class="proxy-card{{if eq .Config.Type $.CurrentType}} current{{end}}" data-type="{{.Config.Type}}">
  <div class="proxy-info">
    <div class="proxy-label">{{.Config.Label}}{{if eq .Config.Type $.CurrentType}} <span class="current-badge">Активен</span>{{end}}</div>
    <div class="proxy-type">{{.Config.Type}}</div>
  </div>
  <div class="proxy-status">
    <span class="status-badge {{if .OK}}status-ok{{else if .Error}}status-error{{else}}status-pending{{end}}" id="status-{{.Config.Type}}">
      {{if .OK}}OK{{else if .Error}}Ошибка{{else}}—{{end}}
    </span>
    <span class="proxy-ip" id="ip-{{.Config.Type}}">{{if .IP}}{{.IP}}{{else}}—{{end}}</span>
    <span class="proxy-latency" id="latency-{{.Config.Type}}">{{if .Latency}}{{.Latency}}{{else}}—{{end}}</span>
    <button class="btn btn-sm btn-outline" onclick="testProxy('{{.Config.Type}}')">Тест</button>
    {{if ne .Config.Type $.CurrentType}}
    <button class="btn btn-sm btn-primary" onclick="switchProxy('{{.Config.Type}}')">Применить</button>
    {{end}}
  </div>
  {{if .Error}}<div style="width:100%;font-size:12px;color:var(--warn-text);margin-top:4px;">{{.Error}}</div>{{end}}
</div>
{{end}}
</div>
</div>
<script>
function testProxy(type) {
  var btn = event.target;
  btn.disabled = true;
  btn.textContent = '...';
  fetch('/api/proxy/test', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({type: type})
  }).then(r => r.json()).then(data => {
    btn.disabled = false;
    btn.textContent = 'Тест';
    updateCard(data);
  });
}
function testAll() {
  var cards = document.querySelectorAll('.proxy-card');
  cards.forEach(function(card) {
    var type = card.dataset.type;
    testProxy(type);
  });
}
function switchProxy(type) {
  fetch('/api/proxy/switch', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({type: type})
  }).then(r => r.json()).then(data => {
    if (data.ok) {
      location.reload();
    } else {
      alert('Ошибка: ' + (data.error || 'неизвестная'));
    }
  });
}
function updateCard(data) {
  var card = document.querySelector('[data-type="' + data.config.type + '"]');
  if (!card) return;
  var statusEl = document.getElementById('status-' + data.config.type);
  var ipEl = document.getElementById('ip-' + data.config.type);
  var latEl = document.getElementById('latency-' + data.config.type);
  statusEl.className = 'status-badge ' + (data.ok ? 'status-ok' : 'status-error');
  statusEl.textContent = data.ok ? 'OK' : 'Ошибка';
  ipEl.textContent = data.ip || '—';
  latEl.textContent = data.latency ? formatDuration(data.latency) : '—';
  var errDiv = card.querySelector('.proxy-error');
  if (errDiv) errDiv.remove();
  if (data.error) {
    var div = document.createElement('div');
    div.className = 'proxy-error';
    div.style.cssText = 'width:100%;font-size:12px;color:var(--warn-text);margin-top:4px;';
    div.textContent = data.error;
    card.appendChild(div);
  }
}
function formatDuration(ns) {
  var ms = ns / 1000000;
  if (ms < 1000) return ms.toFixed(0) + 'ms';
  return (ms / 1000).toFixed(1) + 's';
}
</script>
</body>
</html>`

// handleProxyPage отображает страницу управления прокси.
func (s *Server) handleProxyPage(w http.ResponseWriter, r *http.Request) {
	var data proxyPageData
	data.CurrentType = s.proxyURL
	if data.CurrentType == "" {
		data.CurrentType = "none"
	}

	if s.proxyManager != nil {
		statuses := s.proxyManager.TestAll()
		data.Proxies = statuses
	} else {
		// Fallback: показываем только текущий
		data.Proxies = []ProxyStatus{
			{
				Config: ProxyConfig{Type: data.CurrentType, Label: "Текущий прокси"},
				OK:     true,
			},
		}
	}

	tmpl := template.Must(template.New("proxy").Parse(proxyPageHTML))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

type proxyTestRequest struct {
	Type string `json:"type"`
}

type proxyTestResponse struct {
	Config  ProxyConfig   `json:"config"`
	IP      string        `json:"ip"`
	Latency time.Duration `json:"latency"`
	OK      bool          `json:"ok"`
	Error   string        `json:"error,omitempty"`
}

type proxySwitchRequest struct {
	Type string `json:"type"`
}

type proxySwitchResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// handleAPITestProxy тестирует один прокси.
func (s *Server) handleAPITestProxy(w http.ResponseWriter, r *http.Request) {
	var req proxyTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if s.proxyManager == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy manager not configured"})
		return
	}

	status := s.proxyManager.Test(req.Type)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxyTestResponse{
		Config:  status.Config,
		IP:      status.IP,
		Latency: status.Latency,
		OK:      status.OK,
		Error:   status.Error,
	})
}

// handleAPISwitchProxy переключает текущий прокси.
func (s *Server) handleAPISwitchProxy(w http.ResponseWriter, r *http.Request) {
	var req proxySwitchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if s.proxyManager == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy manager not configured"})
		return
	}

	if err := s.proxyManager.Switch(req.Type); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(proxySwitchResponse{OK: false, Error: err.Error()})
		return
	}

	s.proxyURL = req.Type
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxySwitchResponse{OK: true})
}
