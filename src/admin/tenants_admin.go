// tenants_admin.go — раздел «Тенанты и токены» ГЛАВНОЙ админки (:8090).
//
// Зачем здесь: единственная страница, где подключаются токены Telegram-ботов и
// MCP API-ключи, исторически жила в Резидентство-Админе (:8091), который nginx
// наружу не выпускает (location / → :8090, /mcp → :8080). Поэтому с публичного
// адреса к ней было не добраться. Тот же функционал поверх общего store.TenantStore
// смонтирован сюда, в видимое меню :8090; страница :8091 остаётся как есть.
//
// Хендлеры зеркалят tenant-логику residency_admin.go, но работают через
// s.tenantStore и используют каркас главной админки (sidebarMainDefine + сессии),
// а не Residency-сайдбар с BasicAuth.
package admin

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// handleTenantsPage — список тенантов с MCP-ключами и статусом Telegram-ботов.
func (s *Server) handleTenantsPage(w http.ResponseWriter, r *http.Request) {
	if s.tenantStore == nil {
		http.Error(w, "Хранилище тенантов недоступно (требуется Postgres-бэкенд)", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	tenants, err := s.tenantStore.ListTenants(ctx)
	if err != nil {
		http.Error(w, "Ошибка загрузки тенантов: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(tenants, func(i, j int) bool {
		return tenants[i].CreatedAt.After(tenants[j].CreatedAt)
	})

	data := tenantsPageData{
		Tenants:   tenants,
		Flash:     r.URL.Query().Get("msg"),
		FlashKind: orDefault(r.URL.Query().Get("kind"), "ok"),
	}
	// Одноразовый показ свежесгенерированного ключа по nonce.
	if key, ok := keyReveals.take(r.URL.Query().Get("reveal_nonce")); ok {
		data.RevealKey = key
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminTenantsTmpl.Execute(w, data); err != nil {
		log.Println("[admin] шаблон tenants:", err)
	}
}

// handleTenantCreate создаёт тенанта и показывает его MCP-ключ один раз.
func (s *Server) handleTenantCreate(w http.ResponseWriter, r *http.Request) {
	if s.tenantStore == nil {
		http.Error(w, "Хранилище тенантов недоступно", http.StatusServiceUnavailable)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		residencyRedirect(w, r, "/tenants", "Название обязательно", "err")
		return
	}

	// API-ключ можно задать вручную, но обычно генерируется автоматически.
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if apiKey == "" {
		apiKey = generateAPIKey()
	}

	ctx := r.Context()

	// Тенант «Default» идемпотентен — не плодим дубли.
	if strings.EqualFold(name, store.DefaultTenantName) {
		if existing, err := store.GetOrCreateDefaultTenant(ctx, s.tenantStore); err == nil {
			residencyRedirect(w, r, "/tenants", "Тенант по умолчанию: "+existing.Name, "ok")
			return
		}
	}

	tenant := &model.Tenant{
		ID:        generateUUID(),
		Name:      name,
		APIKey:    apiKey,
		CreatedAt: time.Now(),
		Active:    true,
	}

	if err := s.tenantStore.CreateTenant(ctx, tenant); err != nil {
		residencyRedirect(w, r, "/tenants", "Ошибка создания тенанта: "+err.Error(), "err")
		return
	}

	s.redirectTenantReveal(w, r, apiKey, "Тенант создан: "+name+". Скопируйте API-ключ — позже он будет скрыт.")
}

// handleTenantRegenerateKey ротирует MCP API-ключ тенанта.
func (s *Server) handleTenantRegenerateKey(w http.ResponseWriter, r *http.Request) {
	if s.tenantStore == nil {
		http.Error(w, "Хранилище тенантов недоступно", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	tenant, err := s.tenantStore.GetTenant(ctx, id)
	if err != nil {
		residencyRedirect(w, r, "/tenants", "Тенант не найден: "+err.Error(), "err")
		return
	}

	now := time.Now()
	newKey := generateAPIKey()
	tenant.APIKey = newKey
	tenant.KeyRotatedAt = &now

	if err := s.tenantStore.UpdateTenant(ctx, tenant); err != nil {
		residencyRedirect(w, r, "/tenants", "Ошибка ротации ключа: "+err.Error(), "err")
		return
	}

	s.redirectTenantReveal(w, r, newKey, "Ключ обновлён. Старый ключ больше не действует — скопируйте новый.")
}

// handleTenantTelegramToken сохраняет/сбрасывает токен Telegram-бота тенанта.
// Мультибот-менеджер (src/tgbot) подхватит изменение на ближайшем reconcile.
func (s *Server) handleTenantTelegramToken(w http.ResponseWriter, r *http.Request) {
	if s.tenantStore == nil {
		http.Error(w, "Хранилище тенантов недоступно", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	tenant, err := s.tenantStore.GetTenant(ctx, id)
	if err != nil {
		residencyRedirect(w, r, "/tenants", "Тенант не найден: "+err.Error(), "err")
		return
	}

	token := strings.TrimSpace(r.FormValue("telegram_token"))
	tenant.TelegramBotToken = token
	if token == "" {
		// Токен убран — сбрасываем и кэш @username (бот будет остановлен менеджером).
		tenant.TelegramBotUsername = ""
	}

	if err := s.tenantStore.UpdateTenant(ctx, tenant); err != nil {
		residencyRedirect(w, r, "/tenants", "Ошибка сохранения токена: "+err.Error(), "err")
		return
	}

	msg := "Токен Telegram-бота сохранён. Бот поднимется в течение минуты."
	if token == "" {
		msg = "Токен Telegram-бота убран. Бот будет остановлен."
	}
	residencyRedirect(w, r, "/tenants", msg, "ok")
}

// handleTenantToggleActive включает/выключает тенанта.
func (s *Server) handleTenantToggleActive(w http.ResponseWriter, r *http.Request) {
	if s.tenantStore == nil {
		http.Error(w, "Хранилище тенантов недоступно", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	tenant, err := s.tenantStore.GetTenant(ctx, id)
	if err != nil {
		residencyRedirect(w, r, "/tenants", "Тенант не найден: "+err.Error(), "err")
		return
	}

	tenant.Active = !tenant.Active
	if err := s.tenantStore.UpdateTenant(ctx, tenant); err != nil {
		residencyRedirect(w, r, "/tenants", "Ошибка обновления: "+err.Error(), "err")
		return
	}

	state := "активирован"
	if !tenant.Active {
		state = "деактивирован"
	}
	residencyRedirect(w, r, "/tenants", "Тенант "+state+": "+tenant.Name, "ok")
}

// redirectTenantReveal перенаправляет на /tenants с одноразовым показом ключа.
// Открытый ключ кладётся в keyReveals под nonce; в URL попадает только nonce.
func (s *Server) redirectTenantReveal(w http.ResponseWriter, r *http.Request, apiKey, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	nonce := keyReveals.put(apiKey)
	target := "/tenants?reveal_nonce=" + url.QueryEscape(nonce) +
		"&msg=" + url.QueryEscape(msg) + "&kind=ok"
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// adminTenantsTmpl — та же страница тенантов, что и в :8091, но с сайдбаром
// ГЛАВНОЙ админки (sidebarMainDefine) и сессионной авторизацией. Стили и функции
// (residencyCSS / residencyFuncs c maskAPI) переиспользуются из residency_admin.go.
var adminTenantsTmpl = template.Must(template.New("admin-tenants").Funcs(residencyFuncs).Parse(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Тенанты и токены — Админ-панель</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>` + residencyCSS + `</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
{{template "sidebar" .}}
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

<script>
function copyText(id){var el=document.getElementById(id);if(!el)return;navigator.clipboard.writeText(el.textContent).then(function(){var b=event.target;var t=b.textContent;b.textContent='✓';setTimeout(function(){b.textContent=t},1200);});}
function submitToken(form,hasToken){var v=form.telegram_token.value.trim();if(v===''&&hasToken){return confirm('Очистить токен и остановить бота тенанта?');}return true;}
</script>

{{if .RevealKey}}
<div class="card" style="border:2px solid var(--yellow);background:var(--yellow-bg)">
  <h3>🔑 Новый API-ключ — сохраните сейчас</h3>
  <p class="meta" style="margin-bottom:8px">Ключ показывается один раз. В базе хранится только его хэш — позже посмотреть нельзя, только перевыпустить.</p>
  <code id="reveal-key" style="font-size:14px;padding:6px 10px;background:var(--surface);border-radius:4px;user-select:all">{{.RevealKey}}</code>
  <button type="button" class="btn btn-primary btn-sm" onclick="copyText('reveal-key')" data-tooltip="Скопировать ключ">⧉ Копировать</button>
</div>
{{end}}

<div class="card">
  <h3>Тенанты и токены</h3>
  <p class="meta" style="margin-bottom:12px">Тенант — организация-заказчик. Каждый получает собственный <strong>MCP API-ключ</strong> (для подключения своей системы/агента к :8080/mcp) и может подключить свой <strong>Telegram-бот</strong> своим токеном от @BotFather. Глобальный бот — fallback на TELEGRAM_BOT_TOKEN для тенанта Default.</p>
  <form method="POST" action="/tenants">
    <div class="form-group">
      <label>Название</label>
      <input type="text" name="name" placeholder="Название организации" required data-tooltip="Название организации-тенанта">
    </div>
    <p class="meta" style="margin:-4px 0 12px">API-ключ будет сгенерирован автоматически и показан один раз.</p>
    <button type="submit" class="btn btn-primary" data-tooltip="Создать нового тенанта">Создать тенант</button>
  </form>
</div>

{{if .Tenants}}
<div class="table-wrap">
<table>
  <thead>
    <tr>
      <th>Название</th>
      <th>MCP API-ключ</th>
      <th>Telegram-бот</th>
      <th>Активен</th>
      <th>Создан</th>
      <th>Действия</th>
    </tr>
  </thead>
  <tbody>
  {{range .Tenants}}
  <tr>
    <td><strong>{{.Name}}</strong></td>
    <td>
      <code style="background:var(--gray-bg);padding:2px 6px;border-radius:3px;font-size:12px" data-tooltip="Идентификатор ключа (не секрет); полный ключ виден только при создании/ротации">{{if .APIKeyPrefix}}{{.APIKeyPrefix}}…{{else if .APIKey}}{{maskAPI .APIKey}}{{else}}—{{end}}</code>
    </td>
    <td>
      {{if .TelegramBotUsername}}<span class="badge" style="background:var(--green-bg);color:var(--green)" data-tooltip="Бот запущен">@{{.TelegramBotUsername}}</span>
      {{else if .TelegramBotToken}}<span class="badge" style="background:var(--yellow-bg);color:var(--yellow)" data-tooltip="Токен задан, бот запускается">токен задан</span>
      {{else}}<span class="meta">не задан</span>{{end}}
    </td>
    <td>{{if .Active}}<span class="badge" style="background:var(--green-bg);color:var(--green)" data-tooltip="Тенант активен">Да</span>{{else}}<span class="badge" style="background:var(--gray-bg);color:var(--gray)" data-tooltip="Тенант отключён">Нет</span>{{end}}</td>
    <td class="meta">{{.CreatedAt.Format "02.01.2006 15:04"}}</td>
    <td>
      <div style="display:flex;flex-wrap:wrap;gap:6px;align-items:center">
        <form method="POST" action="/tenants/{{.ID}}/regenerate-key" onsubmit="return confirm('Сгенерировать новый ключ? Старый перестанет работать.')" style="display:inline">
          <button type="submit" class="btn btn-ghost btn-sm" data-tooltip="Ротация MCP-ключа">↻ ключ</button>
        </form>
        <form method="POST" action="/tenants/{{.ID}}/toggle-active" style="display:inline">
          <button type="submit" class="btn btn-ghost btn-sm" data-tooltip="Включить/выключить тенанта">{{if .Active}}выкл.{{else}}вкл.{{end}}</button>
        </form>
        <form method="POST" action="/tenants/{{.ID}}/telegram-token" onsubmit="return submitToken(this, {{if .TelegramBotToken}}true{{else}}false{{end}})" style="display:flex;gap:4px;align-items:center">
          <input type="text" name="telegram_token" placeholder="{{if .TelegramBotToken}}задан · новый/пусто=стоп{{else}}токен @BotFather{{end}}" style="width:160px;font-size:12px;padding:4px 6px" data-tooltip="Введите токен, чтобы поднять бота тенанта; отправьте пустым — чтобы остановить">
          <button type="submit" class="btn btn-ghost btn-sm" data-tooltip="Сохранить токен бота">Бот</button>
        </form>
      </div>
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
</div>
{{else}}
<div class="empty">
  <div class="icon"><svg style="width:48px;height:48px;opacity:.4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="2" width="16" height="20" rx="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01M16 6h.01M12 6h.01M8 10h.01M16 10h.01M12 10h.01M8 14h.01M16 14h.01M12 14h.01"/></svg></div>
  <p><strong>Нет тенантов</strong></p>
  <p>Создайте первый тенант через форму выше</p>
</div>
{{end}}
</main>
</body>
</html>` + sidebarMainDefine))
