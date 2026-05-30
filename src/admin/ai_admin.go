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
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg: #f0f2f5; --surface: #fff; --primary: #1e40af; --primary-hover: #1e3a8a;
  --primary-light: #eff6ff; --text: #1e293b; --text-secondary: #64748b;
  --border: #e2e8f0; --radius: 8px; --shadow: 0 1px 3px rgba(0,0,0,.08),0 1px 2px rgba(0,0,0,.06);
  --shadow-lg: 0 10px 15px -3px rgba(0,0,0,.1),0 4px 6px -2px rgba(0,0,0,.05);
  --green: #16a34a; --green-bg: #f0fdf4; --yellow: #ca8a04; --yellow-bg: #fefce8;
  --red: #dc2626; --red-bg: #fef2f2; --blue: #2563eb; --purple: #7c3aed; --purple-bg: #f5f3ff;
  --gray: #6b7280; --gray-bg: #f3f4f6;
}
body { font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif; background:var(--bg); color:var(--text); line-height:1.5; }
header { background:linear-gradient(135deg,var(--primary) 0%,#3b82f6 100%); color:#fff; padding:16px 28px; display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:12px; box-shadow:0 2px 8px rgba(0,0,0,.15); position:sticky; top:0; z-index:100; }
header h1 { font-size:18px; font-weight:600; display:flex; align-items:center; gap:8px; }
.nav-btn { background:rgba(255,255,255,.15); color:#fff; border:1px solid rgba(255,255,255,.25); border-radius:6px; padding:7px 14px; font-size:13px; font-weight:500; cursor:pointer; transition:all .2s; text-decoration:none; display:inline-block; }
.nav-btn:hover { background:rgba(255,255,255,.25); }
.nav-btn.active { background:rgba(255,255,255,.35); border-color:rgba(255,255,255,.5); }
main { max-width:1400px; margin:0 auto; padding:24px 28px; }
.tabs { display:flex; gap:4px; margin-bottom:20px; background:var(--surface); border-radius:var(--radius); padding:6px; box-shadow:var(--shadow); width:fit-content; }
.tab { padding:8px 20px; border-radius:6px; font-size:13px; font-weight:500; cursor:pointer; text-decoration:none; color:var(--text-secondary); transition:all .15s; }
.tab:hover { background:var(--primary-light); color:var(--primary); }
.tab.active { background:var(--primary); color:#fff; }
.card { background:var(--surface); border-radius:var(--radius); box-shadow:var(--shadow); overflow:hidden; }
.card-header { padding:16px 20px; border-bottom:1px solid var(--border); display:flex; align-items:center; justify-content:space-between; gap:12px; }
.card-header h2 { font-size:15px; font-weight:600; }
.card-body { padding:20px; }
table { width:100%; border-collapse:collapse; }
thead th { background:#f8fafc; padding:10px 14px; text-align:left; font-size:12px; font-weight:600; color:var(--text-secondary); text-transform:uppercase; letter-spacing:.5px; border-bottom:2px solid var(--border); }
tbody td { padding:12px 14px; border-bottom:1px solid var(--border); font-size:13px; vertical-align:middle; }
tbody tr:hover { background:#f8fafc; }
tbody tr:last-child td { border-bottom:none; }
.badge { display:inline-block; padding:3px 10px; border-radius:20px; font-size:11px; font-weight:600; }
.badge-green { background:var(--green-bg); color:var(--green); }
.badge-gray { background:var(--gray-bg); color:var(--gray); }
.badge-blue { background:var(--primary-light); color:var(--primary); }
.badge-purple { background:var(--purple-bg); color:var(--purple); }
.badge-yellow { background:var(--yellow-bg); color:var(--yellow); }
.btn { display:inline-flex; align-items:center; justify-content:center; gap:4px; padding:6px 14px; border:none; border-radius:6px; font-size:12px; font-weight:500; cursor:pointer; transition:all .15s; white-space:nowrap; font-family:inherit; text-decoration:none; }
.btn-primary { background:var(--primary); color:#fff; }
.btn-primary:hover { background:var(--primary-hover); }
.btn-success { background:var(--green); color:#fff; }
.btn-success:hover { background:#15803d; }
.btn-danger { background:var(--red); color:#fff; }
.btn-danger:hover { background:#b91c1c; }
.btn-secondary { background:var(--gray-bg); color:var(--text); border:1px solid var(--border); }
.btn-secondary:hover { background:var(--border); }
.btn-sm { padding:4px 10px; font-size:11px; }
.btn-test { background:var(--purple-bg); color:var(--purple); border:1px solid #ddd6fe; }
.btn-test:hover { background:#ede9fe; }
.form-group { margin-bottom:16px; }
.form-group label { display:block; font-size:13px; font-weight:500; margin-bottom:6px; color:var(--text); }
.form-group input, .form-group select, .form-group textarea {
  width:100%; padding:8px 12px; border:1px solid var(--border); border-radius:6px;
  font-size:13px; font-family:inherit; outline:none; transition:border-color .15s,box-shadow .15s;
  background:#fff;
}
.form-group input:focus,.form-group select:focus,.form-group textarea:focus {
  border-color:var(--primary); box-shadow:0 0 0 3px rgba(30,64,175,.1);
}
.form-group textarea { resize:vertical; min-height:120px; }
.form-group .hint { font-size:11px; color:var(--text-secondary); margin-top:4px; }
.form-row { display:grid; grid-template-columns:1fr 1fr; gap:16px; }
.form-actions { display:flex; gap:10px; margin-top:20px; padding-top:20px; border-top:1px solid var(--border); }
.flash { padding:12px 16px; border-radius:var(--radius); margin-bottom:16px; font-size:13px; font-weight:500; display:flex; align-items:center; gap:8px; }
.flash-ok { background:var(--green-bg); color:#15803d; border:1px solid #bbf7d0; }
.flash-err { background:var(--red-bg); color:#b91c1c; border:1px solid #fecaca; }
.provider-icon { width:24px; height:24px; border-radius:4px; display:inline-flex; align-items:center; justify-content:center; font-size:10px; font-weight:700; flex-shrink:0; }
.pi-alibaba { background:#ff6a00; color:#fff; }
.pi-openai { background:#000; color:#fff; }
.pi-anthropic { background:#d97706; color:#fff; }
.pi-custom { background:#7c3aed; color:#fff; }
.test-area { margin-top:24px; background:#f8fafc; border:1px solid var(--border); border-radius:var(--radius); padding:16px; }
.test-area h3 { font-size:13px; font-weight:600; margin-bottom:12px; color:var(--text-secondary); text-transform:uppercase; letter-spacing:.5px; }
.test-input { display:flex; gap:8px; align-items:flex-start; }
.test-input textarea { flex:1; min-height:80px; }
.test-result { margin-top:12px; background:#fff; border:1px solid var(--border); border-radius:6px; padding:14px; font-size:13px; min-height:60px; white-space:pre-wrap; line-height:1.6; }
.test-result.loading { color:var(--text-secondary); font-style:italic; }
.test-result.success { border-left:3px solid var(--green); }
.test-result.error { border-left:3px solid var(--red); color:var(--red); }
.test-meta { font-size:11px; color:var(--text-secondary); margin-top:8px; }
.stats-row { display:grid; grid-template-columns:repeat(auto-fit,minmax(160px,1fr)); gap:12px; margin-bottom:24px; }
.stat-card { background:var(--surface); border-radius:var(--radius); padding:16px; box-shadow:var(--shadow); text-align:center; }
.stat-card .n { font-size:28px; font-weight:700; line-height:1.1; }
.stat-card .l { font-size:11px; color:var(--text-secondary); text-transform:uppercase; letter-spacing:.5px; margin-top:4px; }
.stat-card.blue { border-left:3px solid var(--blue); }
.stat-card.blue .n { color:var(--blue); }
.stat-card.green { border-left:3px solid var(--green); }
.stat-card.green .n { color:var(--green); }
.stat-card.purple { border-left:3px solid var(--purple); }
.stat-card.purple .n { color:var(--purple); }
.prompt-preview { font-family:'SF Mono','Fira Code',monospace; font-size:12px; background:#f8fafc; border:1px solid var(--border); border-radius:4px; padding:8px; max-height:80px; overflow:hidden; position:relative; cursor:pointer; }
.prompt-preview::after { content:''; position:absolute; bottom:0; left:0; right:0; height:24px; background:linear-gradient(transparent,#f8fafc); }
.key-display { font-family:'SF Mono','Fira Code',monospace; font-size:12px; color:var(--text-secondary); }
.actions-cell { display:flex; gap:4px; flex-wrap:wrap; }
@media(max-width:768px) { .form-row { grid-template-columns:1fr; } main { padding:16px; } }
</style>
</head>
<body>
<header>
  <h1>🤖 База Сколково — ИИ Конфигурация</h1>
  <div style="display:flex;gap:8px;flex-wrap:wrap;">
    <a href="/" class="nav-btn">← Документы</a>
    <a href="/ai/models" class="nav-btn{{if eq .Tab "models"}} active{{end}}">Модели</a>
    <a href="/ai/agents" class="nav-btn{{if eq .Tab "agents"}} active{{end}}">Агенты</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}
{{template "ai-content" .}}
</main>
<script>
function testModel(modelId) {
  const msg = document.getElementById('test-msg-'+modelId).value.trim();
  if (!msg) { alert('Введите сообщение для теста'); return; }
  const resultEl = document.getElementById('test-result-'+modelId);
  const metaEl = document.getElementById('test-meta-'+modelId);
  const btn = document.getElementById('test-btn-'+modelId);
  resultEl.textContent = 'Отправляю запрос…';
  resultEl.className = 'test-result loading';
  btn.disabled = true;
  const t0 = Date.now();
  fetch('/api/ai/models/'+modelId+'/test', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(r=>r.json()).then(d=>{
    const ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = '❌ Ошибка: '+d.error;
      resultEl.className = 'test-result error';
      metaEl.textContent = '';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result success';
      metaEl.textContent = 'Время: '+ms+'мс · Токены: '+(d.tokens||'—');
    }
  }).catch(e=>{
    resultEl.textContent = '❌ '+e.message;
    resultEl.className = 'test-result error';
  }).finally(()=>{ btn.disabled=false; });
}
function testAgent(agentId) {
  const msg = document.getElementById('test-msg-agent-'+agentId).value.trim();
  if (!msg) { alert('Введите сообщение для теста'); return; }
  const resultEl = document.getElementById('test-result-agent-'+agentId);
  const metaEl = document.getElementById('test-meta-agent-'+agentId);
  const btn = document.getElementById('test-btn-agent-'+agentId);
  resultEl.textContent = 'Запускаю агента…';
  resultEl.className = 'test-result loading';
  btn.disabled = true;
  const t0 = Date.now();
  fetch('/api/ai/agents/'+agentId+'/test', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(r=>r.json()).then(d=>{
    const ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = '❌ Ошибка: '+d.error;
      resultEl.className = 'test-result error';
      metaEl.textContent = '';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result success';
      metaEl.textContent = 'Время: '+ms+'мс · Токены: '+(d.tokens||'—')+' · Модель: '+(d.model||'—');
    }
  }).catch(e=>{
    resultEl.textContent = '❌ '+e.message;
    resultEl.className = 'test-result error';
  }).finally(()=>{ btn.disabled=false; });
}
function confirmDelete(url, name) {
  if (confirm('Удалить «'+name+'»?')) {
    fetch(url, {method:'POST'}).then(r=>{
      if (r.ok) location.reload(); else alert('Ошибка удаления');
    });
  }
}
function seedQwen() {
  const key = document.getElementById('qwen-key').value.trim();
  if (!key) { alert('Введите API-ключ'); return; }
  fetch('/api/ai/models/seed-qwen', {
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({api_key:key})
  }).then(r=>r.json()).then(d=>{
    if (d.error) alert('Ошибка: '+d.error);
    else { alert('Добавлено '+d.count+' моделей Qwen'); location.reload(); }
  });
}
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
	"providerLabel": func(p string) string { return aimodels.Provider(p).Label() },
	"agentTypeLabel": func(t string) string { return aimodels.AgentType(t).Label() },
	"formatTime": func(t time.Time) string { return t.Format("02.01.2006 15:04") },
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
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f0f2f5;--surface:#fff;--primary:#1e40af;--primary-hover:#1e3a8a;--primary-light:#eff6ff;--text:#1e293b;--text-secondary:#64748b;--border:#e2e8f0;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.08),0 1px 2px rgba(0,0,0,.06);--shadow-lg:0 10px 15px -3px rgba(0,0,0,.1),0 4px 6px -2px rgba(0,0,0,.05);--green:#16a34a;--green-bg:#f0fdf4;--yellow:#ca8a04;--yellow-bg:#fefce8;--red:#dc2626;--red-bg:#fef2f2;--blue:#2563eb;--purple:#7c3aed;--purple-bg:#f5f3ff;--gray:#6b7280;--gray-bg:#f3f4f6}
body{font-family:'Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:linear-gradient(135deg,var(--primary) 0%,#3b82f6 100%);color:#fff;padding:16px 28px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:12px;box-shadow:0 2px 8px rgba(0,0,0,.15);position:sticky;top:0;z-index:100}
header h1{font-size:18px;font-weight:600}
.nav-btn{background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.25);border-radius:6px;padding:7px 14px;font-size:13px;font-weight:500;cursor:pointer;transition:all .2s;text-decoration:none;display:inline-block}
.nav-btn:hover{background:rgba(255,255,255,.25)}
.nav-btn.active{background:rgba(255,255,255,.35);border-color:rgba(255,255,255,.5)}
main{max-width:1400px;margin:0 auto;padding:24px 28px}
.stats-row{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px;margin-bottom:24px}
.stat-card{background:var(--surface);border-radius:var(--radius);padding:16px;box-shadow:var(--shadow);text-align:center}
.stat-card .n{font-size:28px;font-weight:700;line-height:1.1}
.stat-card .l{font-size:11px;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-top:4px}
.stat-card.blue{border-left:3px solid var(--blue)}.stat-card.blue .n{color:var(--blue)}
.stat-card.green{border-left:3px solid var(--green)}.stat-card.green .n{color:var(--green)}
.stat-card.yellow{border-left:3px solid var(--yellow)}.stat-card.yellow .n{color:var(--yellow)}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden;margin-bottom:20px}
.card-header{padding:16px 20px;border-bottom:1px solid var(--border);display:flex;align-items:center;justify-content:space-between;gap:12px}
.card-header h2{font-size:15px;font-weight:600}
.card-body{padding:20px}
table{width:100%;border-collapse:collapse}
thead th{background:#f8fafc;padding:10px 14px;text-align:left;font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;border-bottom:2px solid var(--border)}
tbody td{padding:12px 14px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:middle}
tbody tr:hover{background:#f8fafc}
tbody tr:last-child td{border-bottom:none}
.badge{display:inline-block;padding:3px 10px;border-radius:20px;font-size:11px;font-weight:600}
.badge-green{background:var(--green-bg);color:var(--green)}
.badge-gray{background:var(--gray-bg);color:var(--gray)}
.badge-blue{background:var(--primary-light);color:var(--primary)}
.badge-purple{background:var(--purple-bg);color:var(--purple)}
.badge-yellow{background:var(--yellow-bg);color:var(--yellow)}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:4px;padding:6px 14px;border:none;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;transition:all .15s;white-space:nowrap;font-family:inherit;text-decoration:none}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-success{background:var(--green);color:#fff}.btn-success:hover{background:#15803d}
.btn-danger{background:var(--red);color:#fff}.btn-danger:hover{background:#b91c1c}
.btn-secondary{background:var(--gray-bg);color:var(--text);border:1px solid var(--border)}.btn-secondary:hover{background:var(--border)}
.btn-test{background:var(--purple-bg);color:var(--purple);border:1px solid #ddd6fe}.btn-test:hover{background:#ede9fe}
.btn-sm{padding:4px 10px;font-size:11px}
.actions-cell{display:flex;gap:4px;flex-wrap:wrap}
.provider-icon{width:26px;height:26px;border-radius:5px;display:inline-flex;align-items:center;justify-content:center;font-size:9px;font-weight:700;flex-shrink:0;margin-right:4px}
.pi-alibaba{background:#ff6a00;color:#fff}.pi-openai{background:#000;color:#fff}
.pi-anthropic{background:#d97706;color:#fff}.pi-custom{background:#7c3aed;color:#fff}
.key-mono{font-family:'SF Mono','Fira Code',monospace;font-size:12px;color:var(--text-secondary)}
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500;display:flex;align-items:center;gap:8px}
.flash-ok{background:var(--green-bg);color:#15803d;border:1px solid #bbf7d0}
.flash-err{background:var(--red-bg);color:#b91c1c;border:1px solid #fecaca}
.test-panel{background:#f8fafc;border:1px solid var(--border);border-radius:var(--radius);padding:16px;margin-top:12px}
.test-panel h4{font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:10px}
.test-input{display:flex;gap:8px}
.test-input input{flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none}
.test-input input:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(30,64,175,.1)}
.test-result{margin-top:10px;background:#fff;border:1px solid var(--border);border-radius:6px;padding:12px;font-size:13px;min-height:50px;white-space:pre-wrap;line-height:1.6;display:none}
.test-result.visible{display:block}
.test-result.loading{color:var(--text-secondary);font-style:italic}
.test-result.ok{border-left:3px solid var(--green)}
.test-result.err{border-left:3px solid var(--red);color:var(--red)}
.test-meta{font-size:11px;color:var(--text-secondary);margin-top:6px}
.seed-box{background:linear-gradient(135deg,#fff7ed 0%,#fff 100%);border:1px solid #fed7aa;border-radius:var(--radius);padding:20px;margin-bottom:20px}
.seed-box h3{font-size:14px;font-weight:600;color:#c2410c;margin-bottom:8px}
.seed-box p{font-size:13px;color:var(--text-secondary);margin-bottom:12px}
.seed-input{display:flex;gap:8px}
.seed-input input{flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none}
.model-name-cell{display:flex;align-items:center;gap:8px}
.model-desc{font-size:11px;color:var(--text-secondary);margin-top:2px}
tr.disabled-row td{opacity:.55}
</style>
</head>
<body>
<header>
  <h1>🤖 ИИ Конфигурация</h1>
  <div style="display:flex;gap:8px;flex-wrap:wrap">
    <a href="/" class="nav-btn">← Документы</a>
    <a href="/ai/models" class="nav-btn active">Модели</a>
    <a href="/ai/agents" class="nav-btn">Агенты</a>
    <a href="/ai/models/new" class="nav-btn" style="background:rgba(255,255,255,.3)">+ Добавить модель</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}

{{if not .Models}}
<div class="seed-box">
  <h3>🚀 Быстрое подключение моделей Qwen (Alibaba Cloud)</h3>
  <p>Добавить все доступные модели Qwen автоматически — вставьте API-ключ от Alibaba Cloud Model Studio:</p>
  <div class="seed-input">
    <input type="password" id="qwen-key" placeholder="sk-..." autocomplete="off">
    <button class="btn btn-success" onclick="seedQwen()">Импортировать Qwen модели</button>
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
    <a href="/ai/models/new" class="btn btn-primary">+ Добавить модель</a>
  </div>
  {{if .Models}}
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
      <td><code style="font-size:12px;background:#f3f4f6;padding:2px 6px;border-radius:4px">{{.ModelID}}</code></td>
      <td><span class="key-mono">{{maskKey .APIKey}}</span></td>
      <td style="font-size:12px">T={{.Temperature}} · {{.MaxTokens}}tok</td>
      <td>
        {{if .Enabled}}<span class="badge badge-green">Активна</span>{{else}}<span class="badge badge-gray">Отключена</span>{{end}}
      </td>
      <td>
        <div class="actions-cell">
          <a href="/ai/models/{{.ID}}/edit" class="btn btn-secondary btn-sm">Изменить</a>
          <button class="btn btn-test btn-sm" onclick="toggleTest('{{.ID}}')">Тест</button>
          <button class="btn btn-danger btn-sm" onclick="confirmDelete('/api/ai/models/{{.ID}}/delete','{{.Name}}')">Удалить</button>
        </div>
        <div id="test-{{.ID}}" style="display:none">
          <div class="test-panel">
            <h4>Тест модели</h4>
            <div class="test-input">
              <input type="text" id="test-msg-{{.ID}}" placeholder="Введите тестовое сообщение…" onkeydown="if(event.key==='Enter')testModel('{{.ID}}')">
              <button class="btn btn-test" id="test-btn-{{.ID}}" onclick="testModel('{{.ID}}')">Отправить</button>
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
  {{else}}
  <div style="padding:40px;text-align:center;color:var(--text-secondary)">
    <div style="font-size:48px;margin-bottom:12px">🤖</div>
    <div style="font-size:15px;font-weight:600;margin-bottom:8px">Модели не настроены</div>
    <div style="font-size:13px;margin-bottom:20px">Добавьте модели вручную или используйте быстрый импорт Qwen выше</div>
    <a href="/ai/models/new" class="btn btn-primary">+ Добавить первую модель</a>
  </div>
  {{end}}
</div>
</main>
<script>
function toggleTest(id) {
  const el = document.getElementById('test-'+id);
  el.style.display = el.style.display === 'none' ? 'block' : 'none';
  if (el.style.display === 'block') {
    document.getElementById('test-msg-'+id).focus();
  }
}
function testModel(id) {
  const msg = document.getElementById('test-msg-'+id).value.trim();
  if (!msg) { alert('Введите сообщение'); return; }
  const resultEl = document.getElementById('test-result-'+id);
  const metaEl = document.getElementById('test-meta-'+id);
  const btn = document.getElementById('test-btn-'+id);
  resultEl.textContent = 'Отправляю запрос…';
  resultEl.className = 'test-result visible loading';
  metaEl.textContent = '';
  btn.disabled = true;
  const t0 = Date.now();
  fetch('/api/ai/models/'+id+'/test', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(r=>r.json()).then(d=>{
    const ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = '❌ '+d.error;
      resultEl.className = 'test-result visible err';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result visible ok';
      metaEl.textContent = 'Время: '+ms+'мс  Токены: '+(d.tokens||'—');
    }
  }).catch(e=>{
    resultEl.textContent = '❌ '+e.message;
    resultEl.className = 'test-result visible err';
  }).finally(()=>{ btn.disabled=false; });
}
function confirmDelete(url, name) {
  if (!confirm('Удалить модель «'+name+'»?')) return;
  fetch(url, {method:'POST'}).then(r=>{
    if (r.redirected) { location.href = r.url; }
    else if (r.ok) { location.reload(); }
    else { r.text().then(t=>alert('Ошибка: '+t)); }
  });
}
function seedQwen() {
  const key = document.getElementById('qwen-key').value.trim();
  if (!key) { alert('Введите API-ключ'); return; }
  if (!confirm('Добавить все модели Qwen с этим ключом?')) return;
  fetch('/api/ai/models/seed-qwen', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body:JSON.stringify({api_key:key})
  }).then(r=>r.json()).then(d=>{
    if (d.error) alert('Ошибка: '+d.error);
    else { alert('✅ Добавлено '+d.count+' моделей Qwen'); location.reload(); }
  });
}
</script>
</body>
</html>
`))

var aiModelFormTmpl = template.Must(template.New("ai-model-form").Funcs(template.FuncMap{
	"selected": func(a, b string) template.HTMLAttr {
		if a == b {
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
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f0f2f5;--surface:#fff;--primary:#1e40af;--primary-hover:#1e3a8a;--primary-light:#eff6ff;--text:#1e293b;--text-secondary:#64748b;--border:#e2e8f0;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.08)}
body{font-family:'Inter',-apple-system,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:linear-gradient(135deg,var(--primary) 0%,#3b82f6 100%);color:#fff;padding:16px 28px;display:flex;align-items:center;justify-content:space-between;box-shadow:0 2px 8px rgba(0,0,0,.15)}
header h1{font-size:18px;font-weight:600}
.nav-btn{background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.25);border-radius:6px;padding:7px 14px;font-size:13px;text-decoration:none;display:inline-block}
.nav-btn:hover{background:rgba(255,255,255,.25)}
main{max-width:800px;margin:32px auto;padding:0 28px}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden}
.card-header{padding:20px 24px;border-bottom:1px solid var(--border)}.card-header h2{font-size:16px;font-weight:600}
.card-body{padding:24px}
.form-group{margin-bottom:18px}
.form-group label{display:block;font-size:13px;font-weight:500;margin-bottom:6px}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:9px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none;transition:border-color .15s}
.form-group input:focus,.form-group select:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(30,64,175,.1)}
.form-group .hint{font-size:11px;color:var(--text-secondary);margin-top:4px}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.form-actions{display:flex;gap:10px;margin-top:24px;padding-top:20px;border-top:1px solid var(--border)}
.btn{display:inline-flex;align-items:center;padding:8px 18px;border:none;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;transition:all .15s;font-family:inherit;text-decoration:none}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-secondary{background:#f3f4f6;color:var(--text);border:1px solid var(--border)}
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500}
.flash-err{background:#fef2f2;color:#b91c1c;border:1px solid #fecaca}
.toggle-wrap{display:flex;align-items:center;gap:10px;padding:10px 12px;border:1px solid var(--border);border-radius:6px;cursor:pointer}
.toggle-wrap label{cursor:pointer;font-size:13px;font-weight:500}
</style>
</head>
<body>
<header>
  <h1>{{if .Model.ID}}Редактировать модель{{else}}Добавить ИИ-модель{{end}}</h1>
  <a href="/ai/models" class="nav-btn">← Назад</a>
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
const defaultURLs = {
  alibabacloud: 'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
  openai: 'https://api.openai.com/v1',
  anthropic: '',
  custom: ''
};
function updateBaseURL(provider) {
  const urlInput = document.getElementById('base-url');
  if (!urlInput.value || Object.values(defaultURLs).includes(urlInput.value)) {
    urlInput.value = defaultURLs[provider] || '';
  }
}
</script>
</body>
</html>
`))

var aiAgentsTmpl = template.Must(template.New("ai-agents").Funcs(template.FuncMap{
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
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f0f2f5;--surface:#fff;--primary:#1e40af;--primary-hover:#1e3a8a;--primary-light:#eff6ff;--text:#1e293b;--text-secondary:#64748b;--border:#e2e8f0;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.08),0 1px 2px rgba(0,0,0,.06);--shadow-lg:0 10px 15px -3px rgba(0,0,0,.1);--green:#16a34a;--green-bg:#f0fdf4;--yellow:#ca8a04;--yellow-bg:#fefce8;--red:#dc2626;--red-bg:#fef2f2;--blue:#2563eb;--purple:#7c3aed;--purple-bg:#f5f3ff;--gray:#6b7280;--gray-bg:#f3f4f6}
body{font-family:'Inter',-apple-system,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:linear-gradient(135deg,var(--primary) 0%,#3b82f6 100%);color:#fff;padding:16px 28px;display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:12px;box-shadow:0 2px 8px rgba(0,0,0,.15);position:sticky;top:0;z-index:100}
header h1{font-size:18px;font-weight:600}
.nav-btn{background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.25);border-radius:6px;padding:7px 14px;font-size:13px;text-decoration:none;display:inline-block}.nav-btn:hover{background:rgba(255,255,255,.25)}.nav-btn.active{background:rgba(255,255,255,.35)}
main{max-width:1400px;margin:0 auto;padding:24px 28px}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden;margin-bottom:24px}
.card-header{padding:16px 20px;border-bottom:1px solid var(--border);display:flex;align-items:center;justify-content:space-between;gap:12px}
.card-header h2{font-size:15px;font-weight:600}
table{width:100%;border-collapse:collapse}
thead th{background:#f8fafc;padding:10px 14px;text-align:left;font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;border-bottom:2px solid var(--border)}
tbody td{padding:12px 14px;border-bottom:1px solid var(--border);font-size:13px;vertical-align:top}
tbody tr:hover{background:#f8fafc}
tbody tr:last-child td{border-bottom:none}
.badge{display:inline-block;padding:3px 10px;border-radius:20px;font-size:11px;font-weight:600}
.badge-green{background:var(--green-bg);color:var(--green)}.badge-gray{background:var(--gray-bg);color:var(--gray)}
.badge-blue{background:var(--primary-light);color:var(--primary)}.badge-purple{background:var(--purple-bg);color:var(--purple)}
.badge-yellow{background:var(--yellow-bg);color:var(--yellow)}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:4px;padding:6px 14px;border:none;border-radius:6px;font-size:12px;font-weight:500;cursor:pointer;transition:all .15s;white-space:nowrap;font-family:inherit;text-decoration:none}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-danger{background:var(--red);color:#fff}.btn-danger:hover{background:#b91c1c}
.btn-secondary{background:var(--gray-bg);color:var(--text);border:1px solid var(--border)}.btn-secondary:hover{background:var(--border)}
.btn-test{background:var(--purple-bg);color:var(--purple);border:1px solid #ddd6fe}.btn-test:hover{background:#ede9fe}
.btn-sm{padding:4px 10px;font-size:11px}
.actions-cell{display:flex;gap:4px;flex-wrap:wrap}
.prompt-box{font-family:'SF Mono','Fira Code',monospace;font-size:11px;background:#f8fafc;border:1px solid var(--border);border-radius:4px;padding:6px 8px;max-height:60px;overflow:hidden;position:relative;color:var(--text-secondary)}
.prompt-box::after{content:'';position:absolute;bottom:0;left:0;right:0;height:20px;background:linear-gradient(transparent,#f8fafc)}
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500;display:flex;align-items:center;gap:8px}
.flash-ok{background:var(--green-bg);color:#15803d;border:1px solid #bbf7d0}
.flash-err{background:var(--red-bg);color:#b91c1c;border:1px solid #fecaca}
.test-panel{background:#f8fafc;border:1px solid var(--border);border-radius:var(--radius);padding:14px;margin-top:10px}
.test-panel h4{font-size:11px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px}
.test-input{display:flex;gap:8px}
.test-input textarea{flex:1;padding:8px 12px;border:1px solid var(--border);border-radius:6px;font-size:12px;font-family:inherit;outline:none;min-height:60px;resize:vertical}
.test-input textarea:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(30,64,175,.1)}
.test-result{margin-top:10px;background:#fff;border:1px solid var(--border);border-radius:6px;padding:12px;font-size:13px;min-height:50px;white-space:pre-wrap;line-height:1.6;display:none}
.test-result.visible{display:block}
.test-result.loading{color:var(--text-secondary);font-style:italic}
.test-result.ok{border-left:3px solid var(--green)}
.test-result.err{border-left:3px solid var(--red);color:var(--red)}
.test-meta{font-size:11px;color:var(--text-secondary);margin-top:6px}
tr.disabled-row td{opacity:.55}
</style>
</head>
<body>
<header>
  <h1>🤖 ИИ Конфигурация</h1>
  <div style="display:flex;gap:8px;flex-wrap:wrap">
    <a href="/" class="nav-btn">← Документы</a>
    <a href="/ai/models" class="nav-btn">Модели</a>
    <a href="/ai/agents" class="nav-btn active">Агенты</a>
    <a href="/ai/agents/new" class="nav-btn" style="background:rgba(255,255,255,.3)">+ Добавить агента</a>
  </div>
</header>
<main>
{{if .Flash}}<div class="flash {{.FlashClass}}">{{.Flash}}</div>{{end}}
<div class="card">
  <div class="card-header">
    <h2>Настроенные агенты</h2>
    <a href="/ai/agents/new" class="btn btn-primary">+ Добавить агента</a>
  </div>
  {{if .Agents}}
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
          <a href="/ai/agents/{{.Agent.ID}}/edit" class="btn btn-secondary btn-sm">Изменить</a>
          <button class="btn btn-test btn-sm" onclick="toggleAgentTest('{{.Agent.ID}}')">Тест</button>
          <button class="btn btn-danger btn-sm" onclick="confirmDeleteAgent('/api/ai/agents/{{.Agent.ID}}/delete','{{.Agent.Name}}')">Удалить</button>
        </div>
        <div id="agent-test-{{.Agent.ID}}" style="display:none">
          <div class="test-panel">
            <h4>Тест агента</h4>
            <div class="test-input">
              <textarea id="test-msg-agent-{{.Agent.ID}}" placeholder="Введите тестовый запрос к агенту…"></textarea>
              <button class="btn btn-test" id="test-btn-agent-{{.Agent.ID}}" onclick="testAgent('{{.Agent.ID}}')">Запустить</button>
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
  {{else}}
  <div style="padding:40px;text-align:center;color:var(--text-secondary)">
    <div style="font-size:48px;margin-bottom:12px">🧑‍💼</div>
    <div style="font-size:15px;font-weight:600;margin-bottom:8px">Агенты не настроены</div>
    <div style="font-size:13px;margin-bottom:20px">Создайте первого агента — выберите модель и задайте системный промпт</div>
    <a href="/ai/agents/new" class="btn btn-primary">+ Создать агента</a>
  </div>
  {{end}}
</div>
</main>
<script>
function toggleAgentTest(id) {
  const el = document.getElementById('agent-test-'+id);
  el.style.display = el.style.display==='none' ? 'block' : 'none';
  if (el.style.display==='block') document.getElementById('test-msg-agent-'+id).focus();
}
function testAgent(id) {
  const msg = document.getElementById('test-msg-agent-'+id).value.trim();
  if (!msg) { alert('Введите запрос'); return; }
  const resultEl = document.getElementById('test-result-agent-'+id);
  const metaEl = document.getElementById('test-meta-agent-'+id);
  const btn = document.getElementById('test-btn-agent-'+id);
  resultEl.textContent = 'Запускаю агента…';
  resultEl.className = 'test-result visible loading';
  metaEl.textContent = '';
  btn.disabled = true;
  const t0 = Date.now();
  fetch('/api/ai/agents/'+id+'/test', {
    method:'POST', headers:{'Content-Type':'application/json'},
    body:JSON.stringify({message:msg})
  }).then(r=>r.json()).then(d=>{
    const ms = Date.now()-t0;
    if (d.error) {
      resultEl.textContent = '❌ '+d.error;
      resultEl.className = 'test-result visible err';
    } else {
      resultEl.textContent = d.answer;
      resultEl.className = 'test-result visible ok';
      metaEl.textContent = 'Время: '+ms+'мс  Токены: '+(d.tokens||'—')+'  Модель: '+(d.model||'—');
    }
  }).catch(e=>{
    resultEl.textContent = '❌ '+e.message;
    resultEl.className = 'test-result visible err';
  }).finally(()=>{ btn.disabled=false; });
}
function confirmDeleteAgent(url, name) {
  if (!confirm('Удалить агента «'+name+'»?')) return;
  fetch(url, {method:'POST'}).then(r=>{
    if (r.redirected) location.href=r.url;
    else if (r.ok) location.reload();
    else r.text().then(t=>alert('Ошибка: '+t));
  });
}
</script>
</body>
</html>
`))

var aiAgentFormTmpl = template.Must(template.New("ai-agent-form").Funcs(template.FuncMap{
	"selected": func(a, b string) template.HTMLAttr {
		if a == b {
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
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#f0f2f5;--surface:#fff;--primary:#1e40af;--primary-hover:#1e3a8a;--text:#1e293b;--text-secondary:#64748b;--border:#e2e8f0;--radius:8px;--shadow:0 1px 3px rgba(0,0,0,.08);--red:#dc2626;--red-bg:#fef2f2}
body{font-family:'Inter',-apple-system,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:linear-gradient(135deg,var(--primary) 0%,#3b82f6 100%);color:#fff;padding:16px 28px;display:flex;align-items:center;justify-content:space-between;box-shadow:0 2px 8px rgba(0,0,0,.15)}
header h1{font-size:18px;font-weight:600}
.nav-btn{background:rgba(255,255,255,.15);color:#fff;border:1px solid rgba(255,255,255,.25);border-radius:6px;padding:7px 14px;font-size:13px;text-decoration:none;display:inline-block}.nav-btn:hover{background:rgba(255,255,255,.25)}
main{max-width:900px;margin:32px auto;padding:0 28px}
.card{background:var(--surface);border-radius:var(--radius);box-shadow:var(--shadow);overflow:hidden}
.card-header{padding:20px 24px;border-bottom:1px solid var(--border)}.card-header h2{font-size:16px;font-weight:600}
.card-body{padding:24px}
.form-group{margin-bottom:18px}
.form-group label{display:block;font-size:13px;font-weight:500;margin-bottom:6px}
.form-group input,.form-group select,.form-group textarea{width:100%;padding:9px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;font-family:inherit;outline:none;transition:border-color .15s}
.form-group input:focus,.form-group select:focus,.form-group textarea:focus{border-color:var(--primary);box-shadow:0 0 0 3px rgba(30,64,175,.1)}
.form-group textarea{resize:vertical;min-height:200px}
.form-group .hint{font-size:11px;color:var(--text-secondary);margin-top:4px}
.form-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.form-actions{display:flex;gap:10px;margin-top:24px;padding-top:20px;border-top:1px solid var(--border)}
.btn{display:inline-flex;align-items:center;padding:8px 18px;border:none;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;font-family:inherit;text-decoration:none;transition:all .15s}
.btn-primary{background:var(--primary);color:#fff}.btn-primary:hover{background:var(--primary-hover)}
.btn-secondary{background:#f3f4f6;color:var(--text);border:1px solid var(--border)}
.flash{padding:12px 16px;border-radius:var(--radius);margin-bottom:16px;font-size:13px;font-weight:500}
.flash-err{background:var(--red-bg);color:#b91c1c;border:1px solid #fecaca}
.toggle-wrap{display:flex;align-items:center;gap:10px;padding:10px 12px;border:1px solid var(--border);border-radius:6px;cursor:pointer}
.prompt-defaults{background:#f8fafc;border:1px solid var(--border);border-radius:6px;padding:16px;margin-bottom:12px}
.prompt-defaults h4{font-size:12px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:10px}
.preset-btn{display:inline-block;padding:5px 12px;border:1px solid var(--border);border-radius:4px;font-size:12px;cursor:pointer;margin:4px;background:#fff;transition:all .15s}
.preset-btn:hover{background:var(--primary);color:#fff;border-color:var(--primary)}
</style>
</head>
<body>
<header>
  <h1>{{if .Agent.ID}}Редактировать агента{{else}}Добавить ИИ-агента{{end}}</h1>
  <a href="/ai/agents" class="nav-btn">← Назад</a>
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
        <div class="hint">Инструкции для агента. Определяет его поведение и специализацию.</div>
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
const defaultPrompts = {{.DefaultPromptsJSON}};
function loadDefaultPrompt(type) {
  const p = defaultPrompts[type];
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

