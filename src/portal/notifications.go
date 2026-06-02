package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"baza-skolkovo/src/common/model"
)

// handleNotifications рендерит inbox клиента: персональные уведомления о
// касающихся его изменениях документации. Самодостаточная страница (не зависит
// от общего layout портала).
func (ps *PortalServer) handleNotifications(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	if ps.stores.NotifStore == nil {
		http.Error(w, "уведомления недоступны", http.StatusServiceUnavailable)
		return
	}
	items, err := ps.stores.NotifStore.ListForClient(r.Context(), sess.ClientID, 100)
	if err != nil {
		http.Error(w, "ошибка загрузки уведомлений: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderNotificationsPage(items))
}

// handleNotificationRead помечает уведомление прочитанным и возвращает на inbox.
func (ps *PortalServer) handleNotificationRead(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" && ps.stores.NotifStore != nil {
		_ = ps.stores.NotifStore.MarkRead(r.Context(), id, sess.ClientID)
	}
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// apiNotifications отдаёт уведомления клиента в формате JSON.
func (ps *PortalServer) apiNotifications(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	if ps.stores.NotifStore == nil {
		jsonError(w, http.StatusServiceUnavailable, "уведомления недоступны")
		return
	}
	items, err := ps.stores.NotifStore.ListForClient(r.Context(), sess.ClientID, 100)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func renderNotificationsPage(items []*model.ClientNotification) string {
	var unread int
	for _, n := range items {
		if !n.Read {
			unread++
		}
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="ru"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Уведомления — База Сколково</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#f5f6f8;--surface:#fff;--surface-hover:#f0f1f3;--primary:#0073ea;
  --text:#1a1d29;--text-secondary:#676f83;--text-muted:#9498a8;--body-text:#444b5e;
  --border:#e0e2e8;--border-strong:#c8cbd4;--unread-bg:#fbfdff;
  --red:#de350b;--red-bg:#fde8e0;--yellow:#bf6900;--yellow-bg:#fff6e5;--info-bg:#e8f2fc;
}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){
  --bg:#181b2b;--surface:#23273a;--surface-hover:#2a2f45;--primary:#579dff;
  --text:#d0d1d8;--text-secondary:#9698a6;--text-muted:#7e8194;--body-text:#b8bac6;
  --border:#3b3f54;--border-strong:#4a4f66;--unread-bg:#1f2436;
  --red:#ff6b6b;--red-bg:#2e1a1a;--yellow:#fbbf24;--yellow-bg:#2e2408;--info-bg:#1e3050;
}}
:root[data-theme="dark"]{
  --bg:#181b2b;--surface:#23273a;--surface-hover:#2a2f45;--primary:#579dff;
  --text:#d0d1d8;--text-secondary:#9698a6;--text-muted:#7e8194;--body-text:#b8bac6;
  --border:#3b3f54;--border-strong:#4a4f66;--unread-bg:#1f2436;
  --red:#ff6b6b;--red-bg:#2e1a1a;--yellow:#fbbf24;--yellow-bg:#2e2408;--info-bg:#1e3050;
}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:var(--bg);color:var(--text);line-height:1.5}
header{background:var(--surface);border-bottom:1px solid var(--border);padding:14px 24px;display:flex;justify-content:space-between;align-items:center;gap:12px}
header h1{font-size:17px;font-weight:700;display:flex;align-items:center;gap:8px}
header h1 svg{width:20px;height:20px;color:var(--primary)}
.header-right{display:flex;align-items:center;gap:10px}
.back{color:var(--primary);text-decoration:none;font-size:13px;font-weight:500;display:inline-flex;align-items:center;gap:5px}
.back:hover{text-decoration:underline}
.back svg{width:15px;height:15px}
.theme-btn{background:var(--surface);color:var(--text-secondary);border:1px solid var(--border);border-radius:6px;width:34px;height:34px;padding:0;display:flex;align-items:center;justify-content:center;cursor:pointer;transition:all .15s}
.theme-btn:hover{background:var(--surface-hover);color:var(--text)}
.theme-btn svg{width:17px;height:17px}
main{max-width:760px;margin:0 auto;padding:24px}
.count{font-size:13px;color:var(--text-secondary);margin-bottom:16px}
.item{background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--border-strong);border-radius:8px;padding:14px 16px;margin-bottom:10px}
.item.unread{border-left-color:var(--primary);background:var(--unread-bg)}
.item.sev-critical{border-left-color:var(--red)}
.item.sev-warning{border-left-color:var(--yellow)}
.item-head{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:4px}
.badge{font-size:11px;font-weight:600;padding:2px 9px;border-radius:12px}
.b-critical{background:var(--red-bg);color:var(--red)}
.b-warning{background:var(--yellow-bg);color:var(--yellow)}
.b-info{background:var(--info-bg);color:var(--primary)}
.title{font-weight:600;font-size:14px}
.time{font-size:12px;color:var(--text-muted);margin-left:auto}
.body{font-size:13px;color:var(--body-text);margin-top:4px}
.src{font-size:12px;margin-top:6px;display:inline-flex;align-items:center;gap:4px;color:var(--primary);text-decoration:none}
.src:hover{text-decoration:underline}
.src svg{width:13px;height:13px}
.readbtn{margin-top:8px;background:transparent;border:1px solid var(--border);border-radius:6px;padding:5px 10px;font-size:12px;cursor:pointer;color:var(--text-secondary)}
.readbtn:hover{background:var(--surface-hover);color:var(--text)}
.empty{text-align:center;padding:48px;color:var(--text-muted)}
[data-tooltip]{position:relative}
[data-tooltip]:hover::after{content:attr(data-tooltip);position:absolute;bottom:calc(100% + 8px);left:50%;transform:translateX(-50%);background:#1a1a2e;color:#fff;padding:6px 10px;border-radius:6px;font-size:11px;white-space:nowrap;z-index:999;pointer-events:none;box-shadow:0 2px 8px rgba(0,0,0,.2)}
[data-tooltip]:hover::before{content:'';position:absolute;bottom:calc(100% + 2px);left:50%;transform:translateX(-50%);border:5px solid transparent;border-top-color:#1a1a2e;z-index:999;pointer-events:none}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head><body>
<header>
  <h1><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>Уведомления</h1>
  <div class="header-right">
    <a class="back" href="/dashboard" data-tooltip="Вернуться в личный кабинет">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>
      Личный кабинет
    </a>
    <button class="theme-btn" id="themeBtn" onclick="toggleTheme()" data-tooltip="Переключить тему">
      <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
      <svg class="icon-sun" style="display:none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
    </button>
  </div>
</header>
<main>`)

	b.WriteString(fmt.Sprintf(`<div class="count">Непрочитанных: %d из %d</div>`, unread, len(items)))

	if len(items) == 0 {
		b.WriteString(`<div class="empty">Пока нет уведомлений об изменениях.</div>`)
	}

	for _, n := range items {
		sevClass, badgeClass, badgeText := notifSeverity(n.Severity)
		unreadClass := ""
		if !n.Read {
			unreadClass = " unread"
		}
		b.WriteString(fmt.Sprintf(`<div class="item%s sev-%s">`, unreadClass, html.EscapeString(n.Severity)))
		b.WriteString(`<div class="item-head">`)
		b.WriteString(fmt.Sprintf(`<span class="badge %s">%s</span>`, badgeClass, badgeText))
		_ = sevClass
		b.WriteString(fmt.Sprintf(`<span class="title">%s</span>`, html.EscapeString(n.Title)))
		b.WriteString(fmt.Sprintf(`<span class="time">%s</span>`, n.CreatedAt.Format("02.01.2006 15:04")))
		b.WriteString(`</div>`)
		if n.Body != "" {
			b.WriteString(fmt.Sprintf(`<div class="body">%s</div>`, html.EscapeString(n.Body)))
		}
		if n.URL != "" {
			b.WriteString(fmt.Sprintf(`<a class="src" href="%s" target="_blank" rel="noopener" data-tooltip="Открыть источник в новой вкладке">Источник<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg></a>`,
				html.EscapeString(n.URL)))
		}
		if !n.Read {
			b.WriteString(fmt.Sprintf(`<form method="post" action="/notifications/read">
<input type="hidden" name="id" value="%s"><button class="readbtn" type="submit">Отметить прочитанным</button></form>`,
				html.EscapeString(n.ID)))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</main>
<script>
function toggleTheme(){var r=document.documentElement;var cur=r.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');var next=cur==='dark'?'light':'dark';r.setAttribute('data-theme',next);localStorage.setItem('theme',next);updateThemeIcons(next);}
function updateThemeIcons(theme){var moon=document.querySelector('.icon-moon');var sun=document.querySelector('.icon-sun');if(moon&&sun){moon.style.display=theme==='dark'?'none':'';sun.style.display=theme==='dark'?'':'none';}}
document.addEventListener('DOMContentLoaded',function(){var cur=document.documentElement.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light');updateThemeIcons(cur);});
</script>
</body></html>`)
	return b.String()
}

func notifSeverity(s string) (sevClass, badgeClass, text string) {
	switch s {
	case "critical":
		return "critical", "b-critical", "Критично"
	case "warning":
		return "warning", "b-warning", "Важно"
	default:
		return "info", "b-info", "Информация"
	}
}
