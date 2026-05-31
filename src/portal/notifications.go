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
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f5f6f8;color:#1a1d29;line-height:1.5}
header{background:#fff;border-bottom:1px solid #e0e2e8;padding:14px 24px;display:flex;justify-content:space-between;align-items:center}
header h1{font-size:17px;font-weight:700}
.back{color:#0073ea;text-decoration:none;font-size:13px;font-weight:500}
main{max-width:760px;margin:0 auto;padding:24px}
.count{font-size:13px;color:#676f83;margin-bottom:16px}
.item{background:#fff;border:1px solid #e0e2e8;border-left:3px solid #c8cbd4;border-radius:8px;padding:14px 16px;margin-bottom:10px}
.item.unread{border-left-color:#0073ea;background:#fbfdff}
.item.sev-critical{border-left-color:#de350b}
.item.sev-warning{border-left-color:#bf6900}
.item-head{display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:4px}
.badge{font-size:11px;font-weight:600;padding:2px 9px;border-radius:12px}
.b-critical{background:#fde8e0;color:#de350b}
.b-warning{background:#fff6e5;color:#bf6900}
.b-info{background:#e8f2fc;color:#0073ea}
.title{font-weight:600;font-size:14px}
.time{font-size:12px;color:#9498a8;margin-left:auto}
.body{font-size:13px;color:#444b5e;margin-top:4px}
.src{font-size:12px;margin-top:6px;display:inline-block}
.readbtn{margin-top:8px;background:transparent;border:1px solid #e0e2e8;border-radius:6px;padding:5px 10px;font-size:12px;cursor:pointer;color:#676f83}
.readbtn:hover{background:#f0f1f3}
.empty{text-align:center;padding:48px;color:#9498a8}
</style></head><body>
<header><h1>🔔 Уведомления</h1><a class="back" href="/dashboard">← Личный кабинет</a></header>
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
			b.WriteString(fmt.Sprintf(`<a class="src" href="%s" target="_blank" rel="noopener">Источник →</a>`,
				html.EscapeString(n.URL)))
		}
		if !n.Read {
			b.WriteString(fmt.Sprintf(`<form method="post" action="/notifications/read">
<input type="hidden" name="id" value="%s"><button class="readbtn" type="submit">Отметить прочитанным</button></form>`,
				html.EscapeString(n.ID)))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`</main></body></html>`)
	return b.String()
}

func notifSeverity(s string) (sevClass, badgeClass, text string) {
	switch s {
	case "critical":
		return "critical", "b-critical", "Критично"
	case "warning":
		return "warning", "b-warning", "Важно"
	default:
		return "info", "b-info", "Инфо"
	}
}
