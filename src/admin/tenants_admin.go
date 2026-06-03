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
	"strconv"
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

	// Период биллинга из URL-параметра (по умолчанию 30 дней).
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days != 7 && days != 30 && days != 90 {
		days = 30
	}

	// Счётчики для статус-бара.
	activeCount, withBotCount := 0, 0
	for _, t := range tenants {
		if t.Active {
			activeCount++
		}
		if t.TelegramBotToken != "" {
			withBotCount++
		}
	}

	data := tenantsPageData{
		Tenants:      tenants,
		Flash:        r.URL.Query().Get("msg"),
		FlashKind:    orDefault(r.URL.Query().Get("kind"), "ok"),
		SinceDays:    days,
		TotalCount:   len(tenants),
		ActiveCount:  activeCount,
		WithBotCount: withBotCount,
	}
	// Одноразовый показ свежесгенерированного ключа по nonce.
	if key, ok := keyReveals.take(r.URL.Query().Get("reveal_nonce")); ok {
		data.RevealKey = key
	}

	// Загружаем статистику расходов по тенантам за выбранный период.
	if s.jobStore != nil {
		stats, err := s.jobStore.UsageByTenant(ctx, days)
		if err == nil && len(stats) > 0 {
			data.Billing = make(map[string]TenantBilling, len(stats))
			for _, st := range stats {
				b := TenantBilling{
					Calls:            st.Calls,
					TotalTokens:      st.TotalTokens,
					EstimatedCostUSD: st.EstimatedCostUSD,
				}
				data.Billing[st.TenantID] = b
				data.GlobalBilling.Calls += b.Calls
				data.GlobalBilling.TotalTokens += b.TotalTokens
				data.GlobalBilling.EstimatedCostUSD += b.EstimatedCostUSD
			}
		}
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
<style>` + residencyCSS + `
.stat-bar{display:flex;gap:20px;flex-wrap:wrap;align-items:center;margin-bottom:16px}
.stat-chip{background:var(--surface-alt);border-radius:8px;padding:8px 16px;text-align:center;min-width:90px}
.stat-chip .val{font-size:22px;font-weight:700;line-height:1.1}
.stat-chip .lbl{font-size:11px;color:var(--text-secondary);margin-top:2px}
.period-tabs{display:flex;gap:4px;margin-left:auto}
.period-tab{padding:6px 14px;border-radius:6px;font-size:13px;font-weight:500;text-decoration:none;color:var(--text-secondary);border:1px solid var(--border)}
.period-tab.active,.period-tab:hover{background:var(--primary);color:#fff;border-color:var(--primary)}
.billing-row{display:flex;gap:24px;flex-wrap:wrap;margin:8px 0 4px}
.billing-cell{min-width:100px}
.billing-cell .bval{font-size:18px;font-weight:700}
.billing-cell .blbl{font-size:11px;color:var(--text-secondary)}
.filter-bar{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-bottom:12px}
.filter-bar input[type=search]{flex:1;min-width:180px;padding:7px 12px;border-radius:6px;border:1px solid var(--border);background:var(--surface);color:var(--text);font-size:14px}
.filter-btn{padding:5px 12px;border-radius:6px;border:1px solid var(--border);background:var(--surface);color:var(--text-secondary);font-size:13px;cursor:pointer;white-space:nowrap}
.filter-btn.active{background:var(--primary);color:#fff;border-color:var(--primary)}
.sort-th{cursor:pointer;user-select:none;white-space:nowrap}
.sort-th:hover{color:var(--primary)}
.sort-th .arr{opacity:.4;font-size:11px}
.sort-th.asc .arr::after{content:'▲'}
.sort-th.desc .arr::after{content:'▼'}
.sort-th:not(.asc):not(.desc) .arr::after{content:'⇅'}
.bot-form{display:none;margin-top:8px;padding:10px;background:var(--surface-alt);border-radius:6px;border:1px solid var(--border)}
.bot-form.open{display:block}
.no-results{text-align:center;padding:32px;color:var(--text-secondary);font-size:14px}
.badge-inactive{background:var(--gray-bg);color:var(--gray)}
.create-toggle{cursor:pointer;display:flex;align-items:center;gap:8px;font-weight:600;font-size:15px;user-select:none}
.create-toggle .arr{transition:transform .2s;display:inline-block}
.create-toggle.open .arr{transform:rotate(90deg)}
.create-body{display:none;margin-top:14px}
.create-body.open{display:block}
</style>
<script>(function(){var t=localStorage.getItem('theme');if(t)document.documentElement.setAttribute('data-theme',t)})();</script>
</head>
<body>
{{template "sidebar" .}}
<main>
{{if .Flash}}<div class="flash {{.FlashKind}}">{{.Flash}}</div>{{end}}

{{if .RevealKey}}
<div class="card" style="border:2px solid var(--yellow);background:var(--yellow-bg);margin-bottom:16px">
  <h3 style="margin:0 0 8px">🔑 Новый API-ключ — сохраните сейчас</h3>
  <p class="meta" style="margin-bottom:10px">Ключ показывается один раз. В базе хранится только его хэш — позже посмотреть нельзя, только перевыпустить.</p>
  <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">
    <code id="reveal-key" style="font-size:14px;padding:8px 12px;background:var(--surface);border-radius:4px;user-select:all;word-break:break-all">{{.RevealKey}}</code>
    <button type="button" class="btn btn-primary btn-sm" onclick="copyById('reveal-key',this)">⧉ Копировать</button>
  </div>
</div>
{{end}}

<!-- ─── Статус-бар ─────────────────────────────────── -->
<div class="stat-bar">
  <div class="stat-chip">
    <div class="val">{{.TotalCount}}</div>
    <div class="lbl">всего</div>
  </div>
  <div class="stat-chip">
    <div class="val" style="color:var(--green)">{{.ActiveCount}}</div>
    <div class="lbl">активных</div>
  </div>
  <div class="stat-chip">
    <div class="val" style="color:var(--text-secondary)">{{sub .TotalCount .ActiveCount}}</div>
    <div class="lbl">отключено</div>
  </div>
  <div class="stat-chip">
    <div class="val" style="color:var(--primary)">{{.WithBotCount}}</div>
    <div class="lbl">с ботом</div>
  </div>
  <div class="period-tabs" style="margin-left:auto">
    <a class="period-tab{{if eq .SinceDays 7}} active{{end}}" href="/tenants?days=7">7 дн.</a>
    <a class="period-tab{{if eq .SinceDays 30}} active{{end}}" href="/tenants?days=30">30 дн.</a>
    <a class="period-tab{{if eq .SinceDays 90}} active{{end}}" href="/tenants?days=90">90 дн.</a>
  </div>
</div>

<!-- ─── Биллинг суммарно ───────────────────────────── -->
{{if .GlobalBilling.Calls}}
<div class="card" style="margin-bottom:16px">
  <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:4px">
    <h4 style="margin:0">📊 Суммарный расход ИИ за {{.SinceDays}} дней</h4>
    <span class="meta" style="font-size:12px">все тенанты + системные вызовы</span>
  </div>
  <div class="billing-row">
    <div class="billing-cell">
      <div class="bval">{{.GlobalBilling.Calls}}</div>
      <div class="blbl">запросов к LLM</div>
    </div>
    <div class="billing-cell">
      <div class="bval">{{fmtNum .GlobalBilling.TotalTokens}}</div>
      <div class="blbl">токенов всего</div>
    </div>
    <div class="billing-cell">
      <div class="bval" style="color:var(--green)">${{printf "%.4f" .GlobalBilling.EstimatedCostUSD}}</div>
      <div class="blbl">~стоимость USD</div>
    </div>
  </div>
  <p class="meta" style="margin:4px 0 0;font-size:12px">Расчёт по ценам из <a href="/ai/models">ИИ-моделей</a>. Вызовы без tenant_id (бэкфилл, системные) учтены в общей сумме.</p>
</div>
{{end}}

<!-- ─── Создать тенанта ───────────────────────────── -->
<div class="card" style="margin-bottom:16px">
  <div class="create-toggle" id="create-toggle" onclick="toggleCreate()">
    <span class="arr">▶</span> Создать нового тенанта
  </div>
  <div class="create-body" id="create-body">
    <p class="meta" style="margin:0 0 12px">Тенант — организация-заказчик. Получает MCP API-ключ для подключения агентов к :8080/mcp и может подключить собственный Telegram-бот.</p>
    <form method="POST" action="/tenants" style="display:flex;gap:12px;flex-wrap:wrap;align-items:flex-end">
      <div class="form-group" style="flex:1;min-width:200px;margin:0">
        <label>Название организации</label>
        <input type="text" name="name" placeholder="ООО Инновации" required autocomplete="off">
      </div>
      <button type="submit" class="btn btn-primary">Создать тенанта</button>
    </form>
    <p class="meta" style="margin:8px 0 0;font-size:12px">API-ключ будет сгенерирован автоматически и показан один раз сразу после создания.</p>
  </div>
</div>

<!-- ─── Фильтры и поиск ───────────────────────────── -->
{{if .Tenants}}
<div class="filter-bar">
  <input type="search" id="tenant-search" placeholder="🔍 Поиск по названию или @боту…" oninput="applyFilters()">
  <button class="filter-btn active" id="f-all"      onclick="setFilter('all',this)">Все ({{.TotalCount}})</button>
  <button class="filter-btn"        id="f-active"   onclick="setFilter('active',this)">Активные ({{.ActiveCount}})</button>
  <button class="filter-btn"        id="f-inactive" onclick="setFilter('inactive',this)">Отключены ({{sub .TotalCount .ActiveCount}})</button>
  <button class="filter-btn"        id="f-bot"      onclick="setFilter('bot',this)">С ботом ({{.WithBotCount}})</button>
</div>

<!-- ─── Таблица тенантов ──────────────────────────── -->
<div class="table-wrap">
<table id="tenants-table">
  <thead>
    <tr>
      <th class="sort-th" onclick="sortTable(0,this)" data-type="str">Название <span class="arr"></span></th>
      <th>MCP API-ключ</th>
      <th>Telegram-бот</th>
      <th class="sort-th" onclick="sortTable(3,this)" data-type="num" data-tooltip="За {{.SinceDays}} дней: запросов / токенов / ~стоимость">Расход ИИ <span class="arr"></span></th>
      <th>Статус</th>
      <th class="sort-th" onclick="sortTable(5,this)" data-type="date">Создан <span class="arr"></span></th>
      <th>Действия</th>
    </tr>
  </thead>
  <tbody>
  {{range .Tenants}}
  {{$b := index $.Billing .ID}}
  <tr
    data-name="{{.Name | lower}}"
    data-bot="{{if .TelegramBotToken}}1{{else}}0{{end}}"
    data-active="{{if .Active}}1{{else}}0{{end}}"
    data-cost="{{printf "%.6f" $b.EstimatedCostUSD}}"
    data-date="{{.CreatedAt.Unix}}">
    <td><strong>{{.Name}}</strong><br><span class="meta" style="font-size:11px">{{.ID}}</span></td>
    <td>
      <div style="display:flex;align-items:center;gap:6px">
        <code id="key-{{.ID}}" style="background:var(--gray-bg);padding:2px 6px;border-radius:3px;font-size:12px">{{if .APIKeyPrefix}}{{.APIKeyPrefix}}…{{else if .APIKey}}{{maskAPI .APIKey}}{{else}}—{{end}}</code>
        <button type="button" class="btn btn-ghost btn-sm" onclick="copyById('key-{{.ID}}',this)" data-tooltip="Скопировать фрагмент ключа">⧉</button>
      </div>
    </td>
    <td>
      {{if .TelegramBotUsername}}<span class="badge" style="background:var(--green-bg);color:var(--green)">🤖 @{{.TelegramBotUsername}}</span>
      {{else if .TelegramBotToken}}<span class="badge" style="background:var(--yellow-bg);color:var(--yellow)">⏳ запускается</span>
      {{else}}<span class="meta" style="font-size:12px">не подключён</span>{{end}}
    </td>
    <td style="font-size:12px">
      {{if $b.Calls}}
      <div><strong>{{$b.Calls}}</strong> <span class="meta">запросов</span></div>
      <div><strong>{{fmtNum $b.TotalTokens}}</strong> <span class="meta">токенов</span></div>
      <div style="color:var(--green);font-weight:600">${{printf "%.4f" $b.EstimatedCostUSD}}</div>
      {{else}}<span class="meta" data-tooltip="Данные появятся после первых запросов через этот тенант">—</span>{{end}}
    </td>
    <td>
      {{if .Active}}<span class="badge" style="background:var(--green-bg);color:var(--green)">Активен</span>
      {{else}}<span class="badge badge-inactive">Отключён</span>{{end}}
    </td>
    <td class="meta" style="white-space:nowrap">{{.CreatedAt.Format "02.01.2006"}}<br><span style="font-size:11px">{{.CreatedAt.Format "15:04"}}</span></td>
    <td>
      <div style="display:flex;flex-direction:column;gap:4px;min-width:160px">
        <div style="display:flex;gap:4px;flex-wrap:wrap">
          <form method="POST" action="/tenants/{{.ID}}/regenerate-key" onsubmit="return confirm('Сгенерировать новый ключ? Старый перестанет работать немедленно.')" style="display:inline">
            <button type="submit" class="btn btn-ghost btn-sm" data-tooltip="Перевыпустить MCP-ключ">↻ Ключ</button>
          </form>
          <form method="POST" action="/tenants/{{.ID}}/toggle-active" style="display:inline">
            <button type="submit" class="btn btn-ghost btn-sm" data-tooltip="{{if .Active}}Деактивировать тенанта{{else}}Активировать тенанта{{end}}">{{if .Active}}⏸ Выкл.{{else}}▶ Вкл.{{end}}</button>
          </form>
          <button type="button" class="btn btn-ghost btn-sm" onclick="toggleBotForm('bot-{{.ID}}')" data-tooltip="Настроить Telegram-бот">🤖 Бот</button>
        </div>
        <div class="bot-form" id="bot-{{.ID}}">
          <form method="POST" action="/tenants/{{.ID}}/telegram-token" onsubmit="return confirmBotToken(this,{{if .TelegramBotToken}}true{{else}}false{{end}})">
            <div style="font-size:12px;color:var(--text-secondary);margin-bottom:6px">
              {{if .TelegramBotToken}}Токен задан. Введите новый или оставьте пустым для остановки бота.{{else}}Введите токен от @BotFather для запуска бота.{{end}}
            </div>
            <div style="display:flex;gap:6px">
              <input type="text" name="telegram_token" placeholder="123456:ABC-DEF..." style="flex:1;font-size:12px;padding:5px 8px">
              <button type="submit" class="btn btn-primary btn-sm">Сохранить</button>
            </div>
          </form>
        </div>
      </div>
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
<div class="no-results" id="no-results" style="display:none">Ничего не найдено. Попробуйте изменить фильтр или поисковый запрос.</div>
</div>
{{else}}
<div class="empty">
  <div class="icon"><svg style="width:48px;height:48px;opacity:.4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="2" width="16" height="20" rx="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01M16 6h.01M12 6h.01M8 10h.01M16 10h.01M12 10h.01M8 14h.01M16 14h.01M12 14h.01"/></svg></div>
  <p><strong>Нет тенантов</strong></p>
  <p>Разверните форму выше и создайте первого тенанта.</p>
</div>
{{end}}

<script>
// ─── Утилиты ───────────────────────────────────────
function copyById(id,btn){
  var el=document.getElementById(id);
  if(!el)return;
  navigator.clipboard.writeText(el.textContent.trim()).then(function(){
    var t=btn.textContent;btn.textContent='✓';
    setTimeout(function(){btn.textContent=t},1400);
  });
}
function toggleBotForm(id){
  var el=document.getElementById(id);
  if(el)el.classList.toggle('open');
}
function confirmBotToken(form,hasToken){
  var v=form.telegram_token.value.trim();
  if(v===''&&hasToken){return confirm('Очистить токен и остановить бота?');}
  return true;
}

// ─── Создать тенанта (сворачиваемая форма) ─────────
function toggleCreate(){
  document.getElementById('create-toggle').classList.toggle('open');
  document.getElementById('create-body').classList.toggle('open');
}

// ─── Фильтры ───────────────────────────────────────
var currentFilter='all';
function setFilter(f,btn){
  currentFilter=f;
  document.querySelectorAll('.filter-btn').forEach(function(b){b.classList.remove('active');});
  btn.classList.add('active');
  applyFilters();
}
function applyFilters(){
  var q=(document.getElementById('tenant-search').value||'').toLowerCase().trim();
  var rows=document.querySelectorAll('#tenants-table tbody tr');
  var shown=0;
  rows.forEach(function(r){
    var name=r.dataset.name||'';
    var active=r.dataset.active==='1';
    var bot=r.dataset.bot==='1';
    var matchSearch=!q||name.indexOf(q)>=0||r.textContent.toLowerCase().indexOf(q)>=0;
    var matchFilter=currentFilter==='all'||(currentFilter==='active'&&active)||(currentFilter==='inactive'&&!active)||(currentFilter==='bot'&&bot);
    var show=matchSearch&&matchFilter;
    r.style.display=show?'':'none';
    if(show)shown++;
  });
  var nr=document.getElementById('no-results');
  if(nr)nr.style.display=shown===0?'block':'none';
}

// ─── Сортировка ────────────────────────────────────
var sortCol=-1,sortDir=1;
function sortTable(col,th){
  var tbody=document.querySelector('#tenants-table tbody');
  if(!tbody)return;
  if(sortCol===col){sortDir*=-1;}else{sortDir=1;sortCol=col;}
  document.querySelectorAll('.sort-th').forEach(function(h){h.classList.remove('asc','desc');});
  th.classList.add(sortDir===1?'asc':'desc');
  var rows=Array.from(tbody.querySelectorAll('tr'));
  var type=th.dataset.type||'str';
  rows.sort(function(a,b){
    var av,bv;
    if(type==='num'){av=parseFloat(a.dataset.cost||0);bv=parseFloat(b.dataset.cost||0);}
    else if(type==='date'){av=parseInt(a.dataset.date||0);bv=parseInt(b.dataset.date||0);}
    else{av=(a.dataset.name||'').toLowerCase();bv=(b.dataset.name||'').toLowerCase();}
    return av<bv?-sortDir:av>bv?sortDir:0;
  });
  rows.forEach(function(r){tbody.appendChild(r);});
}
</script>
</main>
</body>
</html>` + sidebarMainDefine))
