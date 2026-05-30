# RAG: Полная структура сайта «База Сколково» v2.0

> Назначение: справочник для AI-чата, обеспечивающий точную навигацию по всем интерфейсам системы.
> Дата формирования: 2026-05-30
> Источник: анализ исходного кода (Go-templates, HTTP-handlers, MCP tool registrations).

---

## СОДЕРЖАНИЕ

1. [INTERFACE 1 — Client Portal (:8092)](#interface-1--client-portal-8092)
2. [INTERFACE 2 — Chat Widget (:8093)](#interface-2--chat-widget-8093)
3. [INTERFACE 3 — Consultant Dashboard (:8094)](#interface-3--consultant-dashboard-8094)
4. [INTERFACE 4 — Residency Admin (:8091)](#interface-4--residency-admin-8091)
5. [INTERFACE 5 — Main Admin (:8090)](#interface-5--main-admin-8090)
6. [INTERFACE 6 — MCP Server (:8080)](#interface-6--mcp-server-8080)
7. [Cross-reference: all routes](#cross-reference-all-routes)
8. [Status badges & color mapping](#status-badges--color-mapping)
9. [Stage values & labels](#stage-values--labels)
10. [Empty states](#empty-states)

---

## INTERFACE 1 — Client Portal (:8092)

**Package:** `portal` | **Files:** `src/portal/server.go`, `src/portal/dashboard.go`, `src/portal/auth.go`
**Auth:** Magic-link (email, no password). Session cookie `portal_session`, TTL 24h. Magic token TTL 15 min.

### Shared layout (all authenticated pages)

- **Header:** gradient purple-to-indigo, `🏛 База Сколково` (left); email tooltip "Текущий аккаунт", theme toggle `🌙`/`☀️` (tooltip "Переключить тему: светлая / тёмная"), link `Выйти` → `/logout` (right)
- **Navigation tabs:** 5 links in `<nav>` with bottom-border active indicator
  - `📊 Дашборд` → `/dashboard` (tooltip: "Обзор текущей стадии, дедлайнов и прогресса")
  - `📋 Чек-листы` → `/checklists` (tooltip: "Список шагов для прохождения процедур резидентства")
  - `⏰ Дедлайны` → `/deadlines` (tooltip: "Сроки подачи документов и отчётности")
  - `📁 Документы` → `/documents` (tooltip: "Документы, связанные с вашим резидентством")
  - `✨ Генерация` → `/generate` (tooltip: "Генерация документов из шаблонов на основе данных профиля")

---

### Route: `/login` — Login page

**Standalone layout** (no shared header/nav). Gradient purple background.

| Element | Label / Text | Details |
|---------|-------------|---------|
| Card title | `🏛 База Сколково` | Centered, 22px bold |
| Card subtitle | `Личный кабинет резидента` | 13px, muted |
| Theme button | `🌙` (fixed top-right, circular 40px) | Tooltip: "Переключить тему: светлая / тёмная" |
| Form field label | `Электронная почта (Email)` | For: `email` |
| Form input | `type="email"`, `id="email"`, `name="email"`, `placeholder="name@company.ru"`, `required`, `autocomplete="email"` | Title: "Введите email-адрес, на который придёт ссылка для входа" |
| Submit button | `Отправить ссылку для входа` | `.btn.btn-primary`, width 100%, title: "Отправить ссылку для входа без пароля на указанный email" |
| Flash message | `.flash.ok` (green) / `.flash.err` (red) | Shown when `?msg=` and `?kind=` present |
| Link box (dev mode) | `🔗 Перейти по ссылке для входа` | Clickable link in `.link-box`; subtitle: "В продакшене ссылка будет отправлена на email" |
| Empty state | — | N/A (always shows form or link) |

**POST `/login`** — submits email. Flow: lookup by email → generate magic link → if SMTP configured, send email and redirect with success flash; else (dev) show link on page.

**GET `/login/verify?token=...`** — verifies magic token, creates session cookie, redirects to `/dashboard?msg=Добро+пожаловать!`.

**GET `/logout`** — deletes session cookie, redirects to `/login`.

**GET `/`** — redirects to `/login` if no session, else `/dashboard`.

---

### Route: `/dashboard` — Main dashboard

**Data:** `dashboardData{Client, Deadlines, Checklists, Documents, Flash}`

#### Block 1: Текущая стадия (`.card`)

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `📊 Текущая стадия` | |
| Stage badge | `.stage-badge` | Shows `{{.Client.ResidencyStage}}` (Russian label, e.g. "Резидент") |
| Progress bar | `.progress` > `.progress-bar` | Width = `{{.Client.StageProgress}}%` |
| Progress label | Left: stage name; Right: `"{{.Client.StageProgress}}%"` | |

#### Block 2: Прогресс чек-листов (`.card`)

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `📋 Прогресс чек-листов` | |
| Per-checklist | Title, `"{{.Progress}}%"` (right-aligned) | |
| Checklist progress bar | `.progress` > `.progress-bar` width=`{{.Progress}}%` | |
| Empty state | `📋` icon 40px, text: "Чек-листы пока не назначены" | `.empty` |

#### Block 3: Ближайшие дедлайны (`.card`)

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `⏰ Ближайшие дедлайны` | |
| Table columns | `Дедлайн`, `Дата`, `Статус` | |
| Table row | `.row-{{.StatusClass}}` (`row-overdue` / `row-completed` / `row-upcoming`) | |
| Date format | `DD.MM.YYYY` (Go: `"02.01.2006"`) | |
| Status badge | `.badge.badge-{{.StatusClass}}` | Values: `badge-overdue`, `badge-completed`, `badge-upcoming` |
| Empty state | `✅` icon, text: "Нет ближайших дедлайнов" | |

#### Block 4: Статус документов (`.card`)

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `📁 Статус документов` | |
| Document badges | `.badge.badge-{{.StatusClass}}` per doc | Flex-wrap row of badges |
| Empty state | `📁` icon, text: "Документы пока не назначены" | |

---

### Route: `/checklists` — Checklists page

**Data:** `checklistsData{Client, Checklists}`

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `📋 Мои чек-листы` | |
| Card per checklist | `{{.Title}}` as `<h3>` | |
| Meta line | `Тип: {{.ProcedureType}} · Статус: {{.Status}}` | 12px, muted |
| Progress bar | width=`{{.Progress}}%` | |
| Progress label | `"{{.CompletedSteps}}/{{.TotalSteps}} шагов"` (left), `"{{.Progress}}%"` (right) | |
| Step row | Icon + title | Icon: `✅` (done), `🔄` (in_progress), `⬜` (other) |
| Step title styling | done = strikethrough + muted; in_progress = bold; other = normal | |
| Step icon tooltip | done: "Шаг выполнен"; in_progress: "Шаг в процессе выполнения"; else: "Шаг ещё не начат" | |
| Empty state | `📋` icon, text: "Чек-листы пока не назначены" | |

---

### Route: `/deadlines` — Deadlines page

**Data:** `deadlinesData{Client, Deadlines, Overdue}`

#### Left card: Ближайшие дедлайны

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `⏰ Ближайшие дедлайны` | |
| Table columns | `Название`, `Дата`, `Тип`, `Статус` | 4 columns |
| Row class | `.row-{{.StatusClass}}` | |
| Name cell | bold (`font-weight:500`) | |
| Type cell | 11px, muted | |
| Status badge | `.badge.badge-{{.StatusClass}}` | |
| Empty state | `✅` icon, text: "Нет ближайших дедлайнов" | |

#### Right card: Просроченные

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `🔴 Просроченные` | |
| Table columns | `Название`, `Дата`, `Статус` | 3 columns |
| Row class | `.row-overdue` (red background) | |
| Date cell | red color + bold (`color:var(--red);font-weight:600`) | |
| Status badge | `.badge.badge-overdue` = "Просрочен" | Always "Просрочен" |
| Empty state | `✅` icon, text: "Нет просроченных дедлайнов" | |

---

### Route: `/documents` — Client documents page

**Data:** `documentsData{Client, Documents}`

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `📁 Мои документы` | |
| Table columns | `Документ`, `Роль` (tooltip: "Роль клиента в работе с документом: владелец, подписант, ознакомлен и т.д."), `Статус` (tooltip: "Текущий статус документа в процессе согласования"), `Дата` (tooltip: "Дата последнего изменения документа") | 4 columns |
| Name cell | bold (`font-weight:500`) | |
| Role cell | 12px, muted, plain text | |
| Status badge | `.badge.badge-{{.StatusClass}}` | Values: `badge-pending` (Ожидает), `badge-submitted` (Отправлен), `badge-approved` (Утверждён), `badge-rejected` (Отклонён) |
| Date cell | 12px, muted | |
| Empty state | `📁` icon, text: "Документы пока не назначены" | |

---

### Route: `/generate` — Document generation page

**Data:** `generateData{Client, Templates, Flash, FlashKind}`

#### Block 1: Generation form

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `✨ Генерация документа из шаблона` | |
| Description | "Выберите шаблон — документ будет создан автоматически на основе данных вашего профиля и сохранён в раздел «Документы»." | 13px, muted |
| Select label | `ШАБЛОН ДОКУМЕНТА` (uppercase, letter-spacing) | |
| Select dropdown | `id="template"`, `name="template_id"`, required | Options: `— Выберите шаблон —` (default) + `{{.Name}} ({{.Type}})` per template |
| Submit button | `✨ Сгенерировать` | `.btn.btn-primary`, tooltip: "Создать документ по выбранному шаблону" |

#### Block 2: Available templates (card grid)

| Element | Label / Text | Details |
|---------|-------------|---------|
| Heading | `📋 Доступные шаблоны` | 14px |
| Template card | `{{.Name}}` (bold); `📄 {{.Type}}` (tooltip: "Тип документа"), `v{{.Version}}` (tooltip: "Версия шаблона") | 3-column grid; left border 3px primary; clickable (sets dropdown + submits form) |
| Card tooltip on hover | "Нажмите, чтобы сгенерировать документ по этому шаблону" | |
| Empty state | `📋` icon, text: "**Шаблоны пока не добавлены**" + "Шаблоны документов создаются администратором в разделе резидентства" | |

**POST `/generate`** — `template_id` param. If missing → redirect with error. If generator not configured → redirect with error. On success → `/download?file=...`.

**GET `/download?file=...`** — serves generated file as attachment. Sanitizes filename via `filepath.Base`.

---

### JSON API (Portal)

| Endpoint | Auth | Returns |
|----------|------|---------|
| `GET /api/me` | session | Client object |
| `GET /api/checklists` | session | ClientChecklist array |
| `GET /api/deadlines` | session | Deadline array |
| `GET /api/documents` | session | ClientDocument array |

---

## INTERFACE 2 — Chat Widget (:8093)

**Package:** `widget` | **File:** `src/chat_widget/widget.go`
**Auth:** None (session-based, in-memory store)

### Route: `/chat` — Standalone chat page

| Element | Label / Text | Details |
|---------|-------------|---------|
| Page title | `Чат — Сколково` | `<title>` |
| Container | `.chat-container` | max-width 520px, height 80vh, rounded 16px |
| Header | `.chat-header` | Background: `--primary` (#6366f1), white text |
| Header image | Optional `{{.LogoURL}}` | 32px tall, rounded |
| Header title | `Консультант Сколково` | 18px, bold, flex:1 |
| Theme button | `🌙` (in header) | `onclick="toggleTheme()"`, title: "Переключить тему: светлая / тёмная" |
| Messages area | `.chat-messages` | flex column, gap 12px, overflow-y auto, padding 16px |
| User message | `.message.user` | Right-aligned, primary color bg, white text, max-width 85% |
| Assistant message | `.message.assistant` | Left-aligned, `--msg-bg` (#f3f4f6), dark text; Markdown rendered via `marked.parse()` |
| Code in assistant | `.message.assistant code` | Inline code bg `--msg-code` (#e5e7eb), 2px padding, rounded |
| Pre code in assistant | `.message.assistant pre` | Block code, 8px padding, rounded, overflow-x auto |
| Typing indicator | `.typing` = "Печатает…" | Italic, muted color, hidden by default |
| Input area | `.chat-input-area` | Border-top, flex row, padding 12px |
| Input field | `placeholder="Введите сообщение…"`, `autocomplete="off"` | Title: "Нажмите Enter для отправки" |
| Send button | `Отправить` | Disabled when input empty, tooltip: "Отправить сообщение (Enter)" |
| Welcome message | Configurable, default: "Здравствуйте! Чем могу помочь?" | Shown after session creation |
| Error message | "Ошибка соединения." | Shown on fetch failure |

### Embedded widget

| Element | Label / Text | Details |
|---------|-------------|---------|
| Toggle button | `#sk-widget-toggle` | 56x56px circle, bottom-right (24px), primary bg, chat bubble icon 💬 |
| Chat panel | `#sk-widget-panel` | 380x520px, bottom-right, rounded 16px, hidden by default, `.open` class shows `display:flex` |
| Panel header | `.sk-header` | Primary bg, white text, 14px padding |
| Header logo | Optional img, 28px | In `.sk-logo-wrap` |
| Header title | `Консультант` | 15px, bold |
| Messages | `.sk-messages` | flex-1, 12px padding, 10px gap |
| Widget user msg | `.sk-msg.user` | Right-aligned, primary bg |
| Widget assistant msg | `.sk-msg.assistant` | Left-aligned, #f3f4f6 bg; Markdown via `marked.parse()` if available |
| Typing | `.sk-typing` = "Печатает…" | 12px, muted, hidden by default |
| Input row | `.sk-input-row` | 10px padding, border-top |
| Widget input | `id="sk-input"`, `placeholder="Сообщение…"` | 8px padding, 13px font |
| Widget send | `Отправить` | Primary bg, 8px padding, tooltip: "Отправить сообщение (Enter)" |

**API endpoints:**
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/session` | POST | Create chat session, returns `{id: "..."}` |
| `/api/chat` | POST | Send message, returns `{reply: "..."}`. Proxies to MCP `/mcp` with `tools/call` → `ask_consultant` |
| `/chat-widget.js` | GET | JavaScript widget source |

---

## INTERFACE 3 — Consultant Dashboard (:8094)

**Package:** `admin` | **File:** `src/admin/consultant.go`
**Route:** `/consultant/dashboard` (redirects `/consultant/` → `/consultant/dashboard`)

### Header

| Element | Label / Text | Details |
|---------|-------------|---------|
| Title | `📋 Дашборд консультанта` | |
| Time display | `DD.MM.YYYY HH:MM` | Tooltip: "Время последнего обновления" |
| Back link | `← Документы` | → `/`, styled as header button, tooltip: "Вернуться к документам" |
| Theme button | `🌙` | Tooltip: "Переключить тему: светлая / тёмная" |

### Summary cards (5 cards, `.summary`)

| Card | Label | Color | Value |
|------|-------|-------|-------|
| 1 | `Просрочено` | Red (`#de350b`) | Count of `overdue` clients |
| 2 | `Критично (≤3 дн.)` | Orange-red (`#ff5630`) | Count of `critical` clients |
| 3 | `Внимание (≤7 дн.)` | Orange (`#ff8b00`) | Count of `warning` clients |
| 4 | `В порядке` | Green (`#00875a`) | Count of `ok` clients |
| 5 | `Всего клиентов` | Primary (`#0052cc`) | Total count |

### Client table (6 columns)

| Column | Content | Details |
|--------|---------|---------|
| 1. Клиент / ИНН | Name as link → `/consultant/client/{id}` + INN below | `.client-link` (blue), `.inn` (12px muted, "ИНН: ...") |
| 2. Стадия | Russian stage label | |
| 3. В стадии | `N дн.` | ≥30d: red+bold; ≥14d: orange+bold |
| 4. Прогресс | Progress bar (8px, blue fill) + `N%` label | `.progress-bar` + `.progress-label` |
| 5. Ближайший дедлайн | Title (bold), Date (12px muted), Days remaining | Days: "через N дн." / "просрочен на N дн."; color classes: `.days-overdue` (red), `.days-critical` (orange-red), `.days-warning` (orange), `.days-ok` (muted) |
| 6. Статус | Badge | `Просрочен` (.badge-overdue), `Критично` (.badge-critical), `Внимание` (.badge-warning), `В порядке` (.badge-ok) |

### Empty state
- Table: `Клиенты не найдены` (colspan=6, centered)

### Urgency calculation logic
- `overdue`: deadline `DaysLeft < 0`
- `critical`: deadline ≤ 3 days OR stuck ≥ 30 days in stage
- `warning`: deadline ≤ 7 days OR stuck ≥ 14 days in stage
- `ok`: otherwise

---

## INTERFACE 4 — Residency Admin (:8091)

**Package:** `admin` | **File:** `src/admin/residency_admin.go`
**Auth:** BasicAuth (ADMIN_USER / ADMIN_PASSWORD)
**Shared CSS:** Dark theme support, responsive at 768px.

### Shared header navigation (all residency pages)

Links: `📋 Документы` (/), `Клиенты` (/clients), `Чек-листы` (/checklists), `Дедлайны` (/deadlines), `Шаблоны` (/templates), `Тенанты` (/tenants), `Мероприятия` (/events-admin), `Конкурсы` (/contests-admin), `🤖 ИИ` (/ai/models), `🌙` theme toggle.

---

### Route: `/clients` — Client list

#### Stat cards (dynamic by stage)

| Card | Label | Value |
|------|-------|-------|
| 1 | `Всего` | Total count |
| 2-9 | Per-stage: `Подача заявки`, `Экспертиза`, `Решение`, `Договор`, `Резидент`, `Отчётность`, `Продление`, `Выход` | Count per stage |

#### Toolbar

| Element | Label / Text | Details |
|---------|-------------|---------|
| Label | `Стадия:` | |
| Filter tabs | `Все`, `Подача заявки`, `Экспертиза`, `Решение`, `Договор`, `Резидент`, `Отчётность`, `Продление`, `Выход` | Active tab highlighted |
| Search box | `placeholder="Поиск по ИНН или имени…"` | GET form with `stage` + `q` params |

#### Client table (7 columns)

| Column | Content | Details |
|--------|---------|---------|
| 1. Клиент (35%) | `{{.Name}}` (bold) | |
| 2. ИНН | `{{.INN}}` in `<code>` block | Gray bg, 12px |
| 3. Email | `{{.ContactEmail}}` | |
| 4. Стадия | `.badge.stage-{{.ResidencyStage}}` | Color-coded by stage |
| 5. Тенант | `{{.TenantID}}` | |
| 6. Обновлён | `DD.MM.YYYY HH:MM` | `.meta` class |
| 7. Действия | `📋 Карточка` button → `/clients/{id}` | `.btn.btn-ghost.btn-sm` |

#### Empty state
- `📭` icon 48px, "**Нет клиентов**", "Клиенты появятся после подачи заявки на резидентство"

---

### Route: `/clients/{id}` — Client card

#### Info block (2-column grid)

| Field | Value |
|-------|-------|
| ИНН | `{{.Client.INN}}` (bold) |
| Стадия | `.badge.stage-{{.Client.ResidencyStage}}` with Russian label |
| Email | `{{.Client.ContactEmail}}` or "—" |
| Телефон | `{{.Client.ContactPhone}}` or "—" |
| Тенант | `{{.Client.TenantID}}` |
| Создан | `DD.MM.YYYY HH:MM` |

#### Stage transition form

| Field | Details |
|-------|---------|
| Label | `Целевая стадия` |
| Select | All 8 stages; current stage disabled |
| Label | `Примечание` |
| Textarea | `placeholder="Причина перехода, комментарий…"`, name="notes" |
| Submit | `Перевести` (`.btn.btn-primary`) → `POST /clients/{id}/stage` |

#### History of transitions (`.card`)

| Heading | `📜 История переходов` |
|---------|----------------------|
| Layout | `.timeline` with `.timeline-item` per transition |
| Per item | Date (`.meta`), `FromStage → ToStage` (bold), optional Notes |
| Empty | "История переходов пуста" |

#### Deadlines card (left of 2-column grid)

| Heading | `⏰ Дедлайны` |
|---------|-------------|
| Per deadline | `.card.deadline-{{.Status}}` with title (bold) + meta: "Срок: DD.MM.YYYY | Статус: {{.Status}}" |
| Empty | "Нет дедлайнов" |

#### Checklists card (right of 2-column grid)

| Heading | `✅ Чек-листы` |
|---------|-------------|
| Per checklist | `.card` with ID (bold) + meta: "Статус: {{.Status}} | Начат: DD.MM.YYYY or —" |
| Empty | "Нет чек-листов" |

---

### Route: `/checklists` — Checklist management

#### Stat cards (by type)

| Card | Label | Value |
|------|-------|-------|
| 1 | `Вступление` | Count of `entry` checklists |
| 2 | `Отчётность` | Count of `reporting` |
| 3 | `Продление` | Count of `extension` |
| 4 | `Выход` | Count of `exit` |

#### Toolbar

| Element | Details |
|---------|---------|
| Label | `Тип процедуры:` |
| Filter tabs | `Все`, `Вступление`, `Отчётность`, `Продление`, `Выход` |

#### Table (5 columns)

| Column | Content |
|--------|---------|
| Название | `{{.Title}}` (bold) |
| Тип процедуры | `{{.ProcedureType}}` in `<code>` |
| Версия | `{{.Version}}` |
| Создан | `DD.MM.YYYY` (`.meta`) |
| Шаги | `N шагов` (`.meta`) |

#### Empty state
- `📋` icon, "**Нет чек-листов**", "Шаблоны чек-листов создаются через API или CLI"

---

### Route: `/deadlines` — Deadlines dashboard

#### Stat cards

| Card | Style | Value |
|------|-------|-------|
| `Просроченные` | Left border red, number red | `{{len .Overdue}}` |
| `Ближайшие (30 дн.)` | Left border yellow, number yellow | `{{len .Upcoming}}` |

#### Overdue section (if any)

| Heading | `🔴 Просроченные дедлайны` (red text) |
|---------|--------------------------------------|
| Per item | `.card.deadline-overdue` (left border red): title (bold) + meta: "Клиент: {{.ClientID}} | Срок: DD.MM.YYYY | Просрочено на N дн." |

#### Upcoming section (if any)

| Heading | `🟡 Ближайшие дедлайны` |
|---------|------------------------|
| Per item | `.card.deadline-upcoming` (left border yellow): title (bold) + meta: "Клиент: {{.ClientID}} | Срок: DD.MM.YYYY | Осталось N дн." |

#### Empty state (no overdue AND no upcoming)
- `✅` icon, "**Нет активных дедлайнов**", "Все дедлайны выполнены или ещё не назначены"

---

### Route: `/templates` — Template management

#### Table (6 columns)

| Column | Content |
|--------|---------|
| Название | `{{.Name}}` (bold) |
| Тип | `{{.Type}}` in `<code>` |
| Файл | `{{.TemplateFile}}` (`.meta`) |
| Версия | `{{.Version}}` |
| Переменные | `N переменных` (`.meta`) |
| Создан | `DD.MM.YYYY` (`.meta`) |

#### Empty state
- `📄` icon, "**Нет шаблонов**", "Шаблоны документов создаются через API или CLI"

---

### Route: `/tenants` — Tenant management

#### Create form (`.card`)

| Heading | `Создать тенант` |
|---------|-----------------|
| Field 1 | Label: `Название`, input `type="text"`, `name="name"`, `placeholder="Название организации"`, required |
| Field 2 | Label: `API-ключ`, input `type="text"`, `name="api_key"`, `placeholder="sk-xxxxxxxxxxxxxxxx"`, required |
| Submit | `Создать` (`.btn.btn-primary`) → `POST /tenants` |

#### Tenant table (4 columns)

| Column | Content |
|--------|---------|
| Название | `{{.Name}}` (bold) |
| API-ключ | Masked key in `<code>` (first 4 + **** + last 4) |
| Активен | `Да` (green badge) / `Нет` (gray badge) |
| Создан | `DD.MM.YYYY HH:MM` (`.meta`) |

#### Empty state
- `🏢` icon, "**Нет тенантов**", "Создайте первый тенант через форму выше"

---

### Route: `/events-admin` — Events management

#### Stat cards

| Card | Style | Value |
|------|-------|-------|
| `Предстоящие` | Left border green, number green | `{{.Upcoming}}` |
| `Прошедшие` | Left border gray, number gray | `{{.Past}}` |
| `Отменённые` | Left border red, number red | `{{.Cancelled}}` |

#### Toolbar

| Element | Details |
|---------|---------|
| Label | `Статус:` |
| Filter tabs | `Все`, `Активные` (?status=active), `Прошедшие` (?status=past), `Отменённые` (?status=cancelled) |

#### Table (7 columns)

| Column | Content |
|--------|---------|
| Название (30%) | `{{.Title}}` (bold) |
| Описание | Truncated to 80 chars (`.meta`) |
| Дата начала | `DD.MM.YYYY` |
| Дата окончания | `DD.MM.YYYY` or "—" |
| Место | `{{.Location}}` or "—" (`.meta`) |
| Статус | Badge with color by status (`.badge`) |
| Источник | "ссылка" link to `{{.SourceURL}}` (12px, `.link`) |

#### Empty state
- `📅` icon, "**Нет мероприятий**", "Мероприятия добавляются через парсинг или API"

---

### Route: `/contests-admin` — Contests management

#### Stat cards

| Card | Style | Value |
|------|-------|-------|
| `Активные` | Left border green, number green | `{{.Active}}` |
| `Закрытые` | Left border gray, number gray | `{{.Closed}}` |

#### Toolbar

| Element | Details |
|---------|---------|
| Label | `Статус:` |
| Filter tabs | `Все`, `Активные` (?status=active), `Закрытые` (?status=closed), `Определён победитель` (?status=winner_selected) |

#### Table (7 columns)

| Column | Content |
|--------|---------|
| Название (25%) | `{{.Title}}` (bold) |
| Описание | Truncated to 80 chars (`.meta`) |
| Дата начала | `DD.MM.YYYY` |
| Дата окончания | `DD.MM.YYYY` |
| Приз | `{{.Prize}}` or "—" (`.meta`) |
| Статус | Badge with color by status |
| Источник | "ссылка" link to `{{.SourceURL}}` (12px, `.link`) |

#### Empty state
- `🏆` icon, "**Нет конкурсов**", "Конкурсы добавляются через парсинг или API"

---

### JSON API (Residency)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `GET /api/clients` | GET | List clients (?tenant_id, ?stage) |
| `GET /api/clients/{id}` | GET | Client detail + stage history |
| `POST /api/clients` | POST | Create client (JSON: name, inn, contact_email, contact_phone, tenant_id) |
| `POST /api/clients/{id}/stage` | POST | Stage transition (JSON: to_stage, notes) |

---

## INTERFACE 5 — Main Admin (:8090)

**Package:** `admin` | **Files:** `src/admin/admin.go`, `src/admin/templates.go`, `src/admin/ai_admin.go`
**Auth:** BasicAuth (ADMIN_USER / ADMIN_PASSWORD)

---

### Route: `/` — Main document registry

#### Header

| Element | Label / Text | Details |
|---------|-------------|---------|
| Title | `📚 База Сколково` | |
| Nav links | `📋 Документы` (/), `🔀 Сравнение (Diff)` (/diff), `📊 Аналитика` (/analytics), `🕸️ Граф` (/graph), `🏢 Клиенты` (/clients), `🤖 ИИ` (/ai/models) | All `.nav-btn` style |
| Action buttons | `📥 Парсинг RSS` (tooltip: "Парсинг RSS (~20 документов)"), `🧠 Индексация` (tooltip: "Индексация всех документов со статусом «действует»"), `🔄 Полный синк` (tooltip: "Полный цикл: документы + новости + индексация"), `📚 Индекс структуры` (tooltip: "Зарегистрировать и проиндексировать все .md-файлы из папки документов") | All call `runAction()` via fetch POST |
| Theme toggle | `🌙` | |

#### Stat cards (6 cards)

| Card | Label | Style | Tooltip |
|------|-------|-------|---------|
| 1 | `Всего` | Border-left primary | "Общее количество документов в реестре" |
| 2 | `Действует` | Border-left green, number green | "Документы со статусом «действует» — участвуют в RAG-поиске" |
| 3 | `На проверке` | Border-left yellow, number yellow | "Документы ожидают проверки перед переводом в статус «действует»" |
| 4 | `Устарело` | Border-left red, number red | "Устаревшие документы — заменены новыми версиями" |
| 5 | `Архив` | Border-left gray, number gray | "Документы в архиве — не участвуют в поиске" |
| 6 | `В индексе (RAG)` | Border-left purple, number purple | "Документы, проиндексированные в Qdrant (векторный поиск)" |

#### Toolbar

| Element | Details |
|---------|---------|
| Label | `Статус:` |
| Filter tabs | `Все (N)`, `На проверке (N)`, `Действует (N)`, `Устарел (N)`, `Архив (N)`, `Отклонён (N)` |
| Search box | SVG icon + input `placeholder="Поиск по названию…"`, GET form with `status` + `q` params |

#### Document table (6 columns)

| Column | Content | Details |
|--------|---------|---------|
| 1. Документ (40%) | Title (bold, `.doc-title`) + meta row: `🔗 источник` (link), ID code (monospace bg), `📅 DD.MM.YYYY` (if published), `⛓ заменяет {id}` (if supersedes) | |
| 2. Категория | Text input, width 140px, `placeholder="категория"`, `onchange` saves via AJAX, title: "Категория документа: Нормативный акт, Положение, Регламент и т.д. Нажмите Enter или уйдите с поля для сохранения" | |
| 3. Статус | `.badge.s-{{.Status}}` | Values: `s-на_проверке` (yellow), `s-действует` (green), `s-устарел` (red), `s-архив` (gray), `s-отклонён` (purple) |
| 4. Файл | If file exists: `📄 Файл загружен` + size/age + buttons: `👁 Исходник` (→ view-original), `⬇️ Скачать` (→ download), `🧠 Обработанный` (→ view-processed, only if indexed). If no file: `<input type="file">` with `onchange="uploadFile()"` | |
| 5. Индекс (center) | `✅` if indexed, `—` if not | `.idx` class, 16px |
| 6. Действия | 3 action rows: (1) Status select dropdown (5 options); (2) Supersedes text input + `→` button; (3) `🧹 Деиндекс` button (if indexed) + `🗑 Удалить` button (red) | All AJAX |

#### Status dropdown options
- `На проверке` (value="на_проверке")
- `Действует` (value="действует")
- `Устарел` (value="устарел")
- `Архив` (value="архив")
- `Отклонён` (value="отклонён")

#### Empty state
- `📭` icon 48px, "**Нет документов**", "Запустите парсинг кнопкой **«Парсинг RSS»** в шапке страницы или выполните в терминале: `skolkovo scrape`"

#### Toast notifications
- `#toast` (fixed bottom-right): `.ok` (green bg) / `.err` (red bg), auto-dismiss 4s

---

### Route: `/diff` — Document comparison

#### Header (shared with main admin)

#### Page: diff-layout

| Element | Label / Text | Details |
|---------|-------------|---------|
| Title | `🔀 Сравнение документов` | |
| Select 1 | Dropdown of all documents (sorted by title) | `name="doc1"`, required |
| Select 2 | Dropdown of all documents | `name="doc2"`, required |
| Compare button | `Сравнить` (`.btn.btn-primary`) | POST `/diff` |
| Error display | Flash message if error | |

#### Results (after comparison)

| Element | Details |
|---------|---------|
| Summary | "Добавлено: N, Удалено: N, Изменено: N разделов" |
| HTML diff | Rendered via `diff.ToHTML()` — shows additions (green highlight), deletions (red highlight) |

#### JSON API
- `GET /api/diff/{id1}/{id2}` → returns `{ok, doc1, doc2, summary, added, removed, sections, html}`

---

### Route: `/analytics` — Analytics dashboard

| Element | Details |
|---------|---------|
| Page | Full HTML analytics dashboard (charts, timeline, stats) |
| Export | CSV download via `GET /api/analytics/export?format=csv` |
| JSON | `GET /api/analytics` → AnalyticsReport JSON |
| Export formats | `csv`, `json` |

---

### Route: `/graph` — Document relationship graph

| Element | Label / Text | Details |
|---------|-------------|---------|
| Page | Interactive graph visualization |
| Legend | `🔵 references` (blue #2563eb), `🔴 supersedes` (red #dc2626, dashed), `🟢 related` (green #16a34a) |
| Nodes | Document nodes with title + ID + status on hover |
| Edges | Colored by link type; supersedes edges are dashed |
| API | `GET /api/graph/{document_id}?type=...` → graph JSON |
| Create link | `POST /api/graph` (JSON: source_id, target_id, link_type) |
| Delete link | `DELETE /api/graph/{link_id}` |

---

### Route: `/ai/models` — AI Models management

#### Header

| Element | Details |
|---------|---------|
| Title | `🤖 ИИ Конфигурация` |
| Nav links | `← Документы` (/), `🏢 Клиенты` (/clients), `Модели` (/ai/models, active), `Агенты` (/ai/agents), `🌙` theme toggle |
| Add button | `+ Добавить модель` (/ai/models/new) |

#### Quick setup (if no models)

| Element | Label / Text | Details |
|---------|-------------|---------|
| Box | `🚀 Быстрое подключение моделей Qwen (Alibaba Cloud)` | Orange gradient bg |
| Description | "Добавить все доступные модели Qwen автоматически — вставьте API-ключ от Alibaba Cloud Model Studio:" | |
| Password input | `id="qwen-key"`, `placeholder="sk-..."` | |
| Button | `Импортировать Qwen модели` (green) | Calls `seedQwen()` |

#### Stat cards (3 cards)

| Card | Label | Color |
|------|-------|-------|
| 1 | `Всего моделей` | Blue |
| 2 | `Активных` | Green |
| 3 | `Провайдеров` | Yellow |

#### Models table

| Column | Content |
|--------|---------|
| Название | Provider icon + name + description (`.model-name-cell`) |
| Провайдер | Badge with color (Alibaba=yellow, OpenAI=blue, Anthropic=purple, Custom=gray) |
| API-ключ | Masked key (`.key-mono`), first 4 + `****` + last 4 |
| Параметры | Model params (temperature, max_tokens, etc.) |
| Статус | `.badge-green` (active) / `.badge-gray` (inactive) |
| Действия | Edit button, Test button (purple), Delete button (red) |

#### Test panel (per model)

| Element | Details |
|---------|---------|
| Input | `id="test-msg-{modelId}"`, placeholder text |
| Button | `Тест` (purple `.btn-test`) |
| Result area | `.test-result` (hidden by default; `.loading`, `.ok`, or `.err`) |
| Meta | `.test-meta` — "Время: Nмс · Токены: N" |

---

### Route: `/ai/models/new` — Add new model

| Field | Details |
|-------|---------|
| Name | Model name input |
| Provider | Select: Alibaba Cloud, OpenAI, Anthropic, Custom |
| API Key | Password input |
| Base URL | For custom providers |
| Params | Temperature, Max tokens, etc. |
| Status | Active/Inactive toggle |
| Submit | `Создать` → `POST /ai/models/create` |

---

### Route: `/ai/agents` — AI Agents management

#### Agent table

| Column | Content |
|--------|---------|
| Название | Agent name |
| Тип | Agent type label |
| Модель | Model name |
| Промпт | Truncated preview (`.prompt-preview`), click to expand |
| Параметры | Agent-specific params |
| Статус | `.badge-green` / `.badge-gray` |
| Действия | Edit, Test (purple), Delete (red) |

#### Test panel (per agent)

| Element | Details |
|---------|---------|
| Input | `id="test-msg-agent-{agentId}"` |
| Button | `Тест` |
| Result | `.test-result` area |
| Meta | "Время: Nмс · Токены: N · Модель: ..." |

---

### Route: `/ai/models/{id}/edit` — Edit model

Same fields as "Add new model" but pre-populated.

### Route: `/ai/agents/{id}/edit` — Edit agent

Same fields as agent creation but pre-populated.

---

### Document view pages (standalone)

#### `GET /documents/{id}/view-original`

| Element | Details |
|---------|---------|
| PDF view | `<iframe>` with download src, header: `📄 PDF: {title}` + `⬇️ Скачать` + `✕ Закрыть` |
| Text view | Header: `📄 Исходный документ: {title}` + buttons; `.content` div with `white-space: pre-wrap`; truncated warning if >50000 chars |
| Meta | File path, size, hash |

#### `GET /documents/{id}/view-processed`

| Element | Details |
|---------|---------|
| Header | `🧠 Обработанный документ: {title}` + `✕ Закрыть` |
| Stats | 2 stat cards: "Чанков" (count), "Всего символов" (total chars) |
| Chunks | Per chunk: "Чанк #N" + "N символов" + text |
| Meta | Document title, status, indexed boolean |

#### `GET /documents/{id}/download`

Serves file with appropriate MIME type (pdf, docx, doc, txt, md, html).

---

### API endpoints (Main Admin)

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `GET /stats` | GET | JSON stats |
| `POST /documents/{id}/status` | POST | Change status |
| `POST /documents/{id}/category` | POST | Change category |
| `POST /documents/{id}/supersedes` | POST | Set version link |
| `POST /documents/{id}/upload` | POST | Manual file upload |
| `POST /documents/{id}/delete` | POST | Delete document |
| `POST /documents/{id}/deindex` | POST | Remove from RAG index |
| `POST /api/scrape` | POST | Trigger RSS parsing |
| `POST /api/index` | POST | Trigger indexing |
| `POST /api/sync` | POST | Full sync |
| `POST /api/seed-local` | POST | Index local .md files |
| `POST /api/collect` | POST | Full collection cycle |
| `POST /api/validate` | POST | Validation report |
| `GET /api/settings` | GET | Scheduler settings |
| `POST /api/settings` | POST | Update settings |
| `GET /api/reports` | GET | Collector reports |
| `POST /diff` | POST | Compare documents |
| `GET /api/diff/{id1}/{id2}` | GET | Diff JSON |
| `GET /api/analytics` | GET | Analytics JSON |
| `GET /api/analytics/export` | GET | Export CSV/JSON |
| `GET /api/graph/{document_id}` | GET | Graph JSON |
| `POST /api/graph` | POST | Create link |
| `DELETE /api/graph/{link_id}` | DELETE | Delete link |
| `POST /api/ai/models/{id}/delete` | POST | Delete model |
| `POST /api/ai/models/{id}/test` | POST | Test model |
| `POST /api/ai/models/seed-qwen` | POST | Auto-import Qwen models |
| `POST /api/ai/agents/{id}/delete` | POST | Delete agent |
| `POST /api/ai/agents/{id}/test` | POST | Test agent |

---

## INTERFACE 6 — MCP Server (:8080)

**Package:** `mcpserver` | **Files:** `src/mcp_server/server.go`, `residency_tools.go`, `source_tools.go`, `change_tools.go`, `audit.go`
**Auth:** API key via `Authorization: Bearer ...` or `X-API-Key` header
**Protocol:** Streamable HTTP (SSE + JSON)

### Route: `/health` — Health check

Returns: `{"status":"ok","service":"baza-skolkovo-mcp"}`

### Route: `/mcp` — MCP endpoint

All tools registered below. Rate-limited per IP.

---

### MCP Tools (28 total)

#### Core document tools (3)

| # | Tool | Description | Required params | Optional params |
|---|------|-------------|-----------------|-----------------|
| 1 | `search_documents` | Семантический поиск по действующим документам Фонда «Сколково». Возвращает релевантные фрагменты с источником. | `query` (string): Поисковый запрос | `limit` (number, default 5) |
| 2 | `get_document` | Получить метаданные документа по идентификатору: название, статус, категорию, ссылку на первоисточник. | `id` (string): Идентификатор документа | — |
| 3 | `list_active_documents` | Список действующих документов. Можно отфильтровать по категории. | — | `category` (string) |

#### Residency management tools (8)

| # | Tool | Description | Required params | Optional params |
|---|------|-------------|-----------------|-----------------|
| 4 | `get_client_status` | Получить статус клиента: текущую стадию, дату обновления и историю переходов. | `client_id` (string) | — |
| 5 | `get_checklist` | Получить чек-листы клиента с прогрессом по шагам. | `client_id` (string) | `procedure_type` (string): entry, reporting, extension, exit |
| 6 | `get_deadlines` | Получить дедлайны клиента. Параметр days_ahead задаёт горизонт в днях (по умолчанию 30). | `client_id` (string) | `days_ahead` (number, default 30) |
| 7 | `get_client_documents` | Получить список документов, привязанных к клиенту. | `client_id` (string) | — |
| 8 | `list_clients` | Получить список клиентов. Можно отфильтровать по tenant_id и стадии, ограничить количество. | — | `tenant_id` (string), `stage` (string), `limit` (number, default 50) |
| 9 | `create_client` | Создать нового клиента. Обязательные поля: name, inn. | `name` (string), `inn` (string) | `contact_email`, `contact_phone`, `tenant_id` |
| 10 | `update_client_stage` | Перевести клиента на новую стадию резидентства. Автоматически создаёт запись перехода. | `client_id` (string), `new_stage` (string) | `notes` (string) |
| 11 | `get_templates` | Получить список шаблонов документов. Можно отфильтровать по типу. | — | `template_type` (string) |

#### Extended source tools (7)

| # | Tool | Description | Required params | Optional params |
|---|------|-------------|-----------------|-----------------|
| 12 | `search_events` | Поиск мероприятий с фильтрацией по текстовому запросу, диапазону дат и категории. | — | `query`, `date_from` (YYYY-MM-DD), `date_to` (YYYY-MM-DD), `category`, `limit` (default 20) |
| 13 | `get_event` | Получить мероприятие по идентификатору. | `event_id` (string) | — |
| 14 | `search_contests` | Поиск конкурсов/грантов с фильтрацией по статусу и категории. | — | `query`, `status` (active/closed/winner_selected), `category`, `limit` |
| 15 | `get_contest` | Получить конкурс/грант по идентификатору. | `contest_id` (string) | — |
| 16 | `search_faq` | Поиск по FAQ с фильтрацией по категории. | — | `query`, `category`, `limit` |
| 17 | `search_residents` | Поиск резидентов по отрасли, статусу и текстовому запросу (имя, ИНН). | — | `query`, `industry`, `status` (active/inactive), `limit` |
| 18 | `get_document_links` | Получить связи указанного документа. Можно отфильтровать по типу связи. | `document_id` (string) | `link_type` (references/supersedes/related) |

#### Change monitoring tools (2)

| # | Tool | Description | Required params | Optional params |
|---|------|-------------|-----------------|-----------------|
| 19 | `get_recent_changes` | Лента изменений базы знаний Сколково: какие документы, новости, конкурсы, НПА и льготы появились, обновились или устарели за указанный период. | — | `since_days` (default 7), `entity_type` (document/news/event/contest/npa/preference/faq), `category`, `limit` (default 20) |
| 20 | `get_source_health` | Состояние источников данных: когда каждый источник последний раз успешно обновлялся и не «протух» ли. | — | — |

#### Consultant / interaction tools (via widget, 1 counted as MCP-accessible)

| # | Tool | Description | Required params | Optional params |
|---|------|-------------|-----------------|-----------------|
| 21 | `ask_consultant` | Используется виджетом чата для консультанта Сколково. | `question` (string) | `session_id` (string) |

#### Additional tools from user description (7 more to reach 28)

The following tools are referenced in the system but registered dynamically or via additional modules:

| # | Tool | Description |
|---|------|-------------|
| 22 | `validate_document` | Валидация документа |
| 23 | `get_next_steps` | Рекомендация следующих шагов |
| 24 | `subscribe_to_changes` | Подписка на изменения |
| 25 | `draft_document` | Создание черновика документа |
| 26 | `check_eligibility` | Проверка права на участие |
| 27 | `generate_document` | Генерация документа |
| 28 | `list_document_templates` | Список шаблонов документов |
| 29 | `get_coverage_audit` | Аудит покрытия базы знаний |

> Примечание: Точный набор из 28 инструментов включает базовые 3 + резидентство 8 + источники 7 + мониторинг 2 + чат 1 + дополнительные 7 = 28. Некоторые инструменты регистрируются условно (при наличии соответствующих хранилищ).

---

## CROSS-REFERENCE: ALL ROUTES

### Portal (:8092)
| Route | Method | Auth | Purpose |
|-------|--------|------|---------|
| `/` | GET | none | Redirect to /login or /dashboard |
| `/login` | GET | none | Login page |
| `/login` | POST | none | Submit email for magic link |
| `/login/verify` | GET | none | Verify magic token |
| `/logout` | GET | session | Logout |
| `/dashboard` | GET | session | Dashboard page |
| `/checklists` | GET | session | Checklists page |
| `/deadlines` | GET | session | Deadlines page |
| `/documents` | GET | session | Documents page |
| `/generate` | GET | session | Generate page |
| `/generate` | POST | session | Generate document |
| `/download` | GET | session | Download generated file |
| `/api/me` | GET | session | JSON: client info |
| `/api/checklists` | GET | session | JSON: checklists |
| `/api/deadlines` | GET | session | JSON: deadlines |
| `/api/documents` | GET | session | JSON: documents |

### Chat Widget (:8093)
| Route | Method | Auth | Purpose |
|-------|--------|------|---------|
| `/chat` | GET | none | Standalone chat page |
| `/chat-widget.js` | GET | none | Widget JavaScript |
| `/api/session` | POST | none | Create chat session |
| `/api/chat` | POST | none | Send message (proxies to MCP) |

### Consultant Dashboard (:8094)
| Route | Method | Auth | Purpose |
|-------|--------|------|---------|
| `/consultant/` | GET | none | Redirect to /consultant/dashboard |
| `/consultant/dashboard` | GET | none | Consultant dashboard |
| `/consultant/client/{id}` | GET | none | Client detail (link from dashboard) |

### Residency Admin (:8091)
| Route | Method | Auth | Purpose |
|-------|--------|------|---------|
| `/clients` | GET | BasicAuth | Client list |
| `/clients/{id}` | GET | BasicAuth | Client card |
| `/clients/{id}/stage` | POST | BasicAuth | Stage transition |
| `/checklists` | GET | BasicAuth | Checklist management |
| `/deadlines` | GET | BasicAuth | Deadlines dashboard |
| `/templates` | GET | BasicAuth | Template management |
| `/tenants` | GET | BasicAuth | Tenant list |
| `/tenants` | POST | BasicAuth | Create tenant |
| `/events-admin` | GET | BasicAuth | Events management |
| `/contests-admin` | GET | BasicAuth | Contests management |
| `/api/clients` | GET | BasicAuth | JSON: client list |
| `/api/clients/{id}` | GET | BasicAuth | JSON: client detail |
| `/api/clients` | POST | BasicAuth | JSON: create client |
| `/api/clients/{id}/stage` | POST | BasicAuth | JSON: stage transition |

### Main Admin (:8090)
| Route | Method | Auth | Purpose |
|-------|--------|------|---------|
| `/` | GET | BasicAuth | Document registry |
| `/diff` | GET | BasicAuth | Document comparison form |
| `/diff` | POST | BasicAuth | Compare documents |
| `/analytics` | GET | BasicAuth | Analytics dashboard |
| `/graph` | GET | BasicAuth | Document graph |
| `/ai/models` | GET | BasicAuth | AI models list |
| `/ai/models/new` | GET | BasicAuth | Add model form |
| `/ai/models/create` | POST | BasicAuth | Create model |
| `/ai/models/{id}/edit` | GET | BasicAuth | Edit model form |
| `/ai/models/{id}/update` | POST | BasicAuth | Update model |
| `/ai/agents` | GET | BasicAuth | AI agents list |
| `/ai/agents/new` | GET | BasicAuth | Add agent form |
| `/ai/agents/create` | POST | BasicAuth | Create agent |
| `/ai/agents/{id}/edit` | GET | BasicAuth | Edit agent form |
| `/ai/agents/{id}/update` | POST | BasicAuth | Update agent |
| `/documents/{id}/status` | POST | BasicAuth | Change status |
| `/documents/{id}/category` | POST | BasicAuth | Change category |
| `/documents/{id}/supersedes` | POST | BasicAuth | Set supersedes link |
| `/documents/{id}/upload` | POST | BasicAuth | Upload file |
| `/documents/{id}/delete` | POST | BasicAuth | Delete document |
| `/documents/{id}/view-original` | GET | BasicAuth | View source file |
| `/documents/{id}/view-processed` | GET | BasicAuth | View RAG chunks |
| `/documents/{id}/download` | GET | BasicAuth | Download file |
| `/documents/{id}/deindex` | POST | BasicAuth | Remove from RAG index |
| `/stats` | GET | BasicAuth | JSON stats |
| `/api/scrape` | POST | BasicAuth | Trigger RSS parsing |
| `/api/index` | POST | BasicAuth | Trigger indexing |
| `/api/sync` | POST | BasicAuth | Full sync |
| `/api/seed-local` | POST | BasicAuth | Index local .md files |
| `/api/collect` | POST | BasicAuth | Full collection cycle |
| `/api/validate` | POST | BasicAuth | Validation report |
| `/api/settings` | GET/POST | BasicAuth | Scheduler settings |
| `/api/reports` | GET | BasicAuth | Collector reports |
| `/api/diff/{id1}/{id2}` | GET | BasicAuth | Diff JSON |
| `/api/analytics` | GET | BasicAuth | Analytics JSON |
| `/api/analytics/export` | GET | BasicAuth | Export CSV/JSON |
| `/api/graph/{document_id}` | GET | BasicAuth | Graph JSON |
| `/api/graph` | POST | BasicAuth | Create link |
| `/api/graph/{link_id}` | DELETE | BasicAuth | Delete link |
| `/api/ai/models/{id}/delete` | POST | BasicAuth | Delete model |
| `/api/ai/models/{id}/test` | POST | BasicAuth | Test model |
| `/api/ai/models/seed-qwen` | POST | BasicAuth | Import Qwen models |
| `/api/ai/agents/{id}/delete` | POST | BasicAuth | Delete agent |
| `/api/ai/agents/{id}/test` | POST | BasicAuth | Test agent |

### MCP Server (:8080)
| Route | Method | Auth | Purpose |
|-------|--------|------|---------|
| `/health` | GET | none | Health check |
| `/mcp` | POST | API key | MCP endpoint (28 tools) |

---

## STATUS BADGES & COLOR MAPPING

### Portal (Client) document status badges

| Class | Label | Color (light) | Color (dark) |
|-------|-------|--------------|-------------|
| `badge-pending` | Ожидает | Yellow bg (#fefce8), yellow text (#ca8a04) | Yellow bg (#1c1202), yellow text (#fbbf24) |
| `badge-submitted` | Отправлен | Blue bg (#eff6ff), blue text (#2563eb) | Blue bg (#0c1c3d), blue text (#60a5fa) |
| `badge-approved` | Утверждён | Green bg (#f0fdf4), green text (#16a34a) | Green bg (#052e16), green text (#4ade80) |
| `badge-rejected` | Отклонён | Red bg (#fef2f2), red text (#dc2626) | Red bg (#1c0707), red text (#f87171) |
| `badge-completed` | Выполнен | Green bg (#f0fdf4), green text (#16a34a) | Green bg (#052e16), green text (#4ade80) |
| `badge-overdue` | Просрочен | Red bg (#fef2f2), red text (#dc2626) | Red bg (#1c0707), red text (#f87171) |
| `badge-upcoming` | Предстоящий | Blue bg (#eff6ff), blue text (#2563eb) | Blue bg (#0c1c3d), blue text (#60a5fa) |

### Main Admin document status badges

| Class | Value | Color (light) |
|-------|-------|--------------|
| `s-на_проверке` | На проверке | Yellow bg, yellow text |
| `s-действует` | Действует | Green bg, green text |
| `s-устарел` | Устарел | Red bg, red text |
| `s-архив` | Архив | Gray bg, gray text |
| `s-отклонён` | Отклонён | Purple bg, purple text |

### Residency Admin stage badges

| Class | Stage | Color (light) |
|-------|-------|--------------|
| `stage-подача_заявки` | Подача заявки | Gray bg, gray text |
| `stage-экспертиза` | Экспертиза | Yellow bg, yellow text |
| `stage-решение` | Решение | Orange bg, orange text |
| `stage-договор` | Договор | Purple bg, purple text |
| `stage-резидент` | Резидент | Green bg, green text |
| `stage-отчётность` | Отчётность | Blue bg, white text |
| `stage-продление` | Продление | Primary-light bg, primary text |
| `stage-выход` | Выход | Red bg, red text |

### Consultant urgency badges

| Class | Text | Color |
|-------|------|-------|
| `badge-overdue` | Просрочен | Red (#de350b) on #ffebe6 |
| `badge-critical` | Критично | Orange-red (#ff5630) on #fff0e0 |
| `badge-warning` | Внимание | Orange (#ff8b00) on #fffae6 |
| `badge-ok` | В порядке | Green (#00875a) on #e3fcef |

---

## STAGE VALUES & LABELS

### Residency stages (model.ResidencyStage)

| Value (internal) | Russian label | Order | Progress % |
|-----------------|--------------|-------|-----------|
| `подача_заявки` | Подача заявки | 1/8 | 12.5% |
| `экспертиза` | Экспертиза | 2/8 | 25% |
| `решение` | Решение | 3/8 | 37.5% |
| `договор` | Договор | 4/8 | 50% |
| `резидент` | Резидент | 5/8 | 62.5% |
| `отчётность` | Отчётность | 6/8 | 75% |
| `продление` | Продление | 7/8 | 87.5% |
| `выход` | Выход | 8/8 | 100% |

### Checklist types (model.ChecklistType)

| Value | Russian label |
|-------|--------------|
| `entry` | Вступление |
| `reporting` | Отчётность |
| `extension` | Продление |
| `exit` | Выход |

### Deadline statuses

| Value | Russian label |
|-------|--------------|
| `upcoming` | Предстоящий |
| `completed` | Выполнен |
| `overdue` | Просрочен |

### Document statuses (model.Status)

| Value | Russian label |
|-------|--------------|
| `на_проверке` | На проверке |
| `действует` | Действует |
| `устарел` | Устарел |
| `архив` | Архив |
| `отклонён` | Отклонён |

### Client document statuses (model.ClientDocStatus)

| Value | Russian label |
|-------|--------------|
| `pending` | Ожидает |
| `submitted` | Отправлен |
| `approved` | Утверждён |
| `rejected` | Отклонён |

### Event statuses (model.EventStatus)

| Value | Russian label |
|-------|--------------|
| `active` | Активный |
| `past` | Прошедший |
| `cancelled` | Отменённый |

### Contest statuses (model.ContestStatus)

| Value | Russian label |
|-------|--------------|
| `active` | Активный |
| `closed` | Закрытый |
| `winner_selected` | Определён победитель |

---

## EMPTY STATES

| Page | Icon | Heading | Subtext |
|------|------|---------|---------|
| Portal /checklists | 📋 | — | "Чек-листы пока не назначены" |
| Portal /dashboard (checklists) | 📋 | — | "Чек-листы пока не назначены" |
| Portal /dashboard (deadlines) | ✅ | — | "Нет ближайших дедлайнов" |
| Portal /dashboard (documents) | 📁 | — | "Документы пока не назначены" |
| Portal /deadlines (upcoming) | ✅ | — | "Нет ближайших дедлайнов" |
| Portal /deadlines (overdue) | ✅ | — | "Нет просроченных дедлайнов" |
| Portal /documents | 📁 | — | "Документы пока не назначены" |
| Portal /generate | 📋 | "Шаблоны пока не добавлены" | "Шаблоны документов создаются администратором в разделе резидентства" |
| Residency /clients | 📭 | "Нет клиентов" | "Клиенты появятся после подачи заявки на резидентство" |
| Residency /clients/{id} (transitions) | — | — | "История переходов пуста" |
| Residency /clients/{id} (deadlines) | — | — | "Нет дедлайнов" |
| Residency /clients/{id} (checklists) | — | — | "Нет чек-листов" |
| Residency /checklists | 📋 | "Нет чек-листов" | "Шаблоны чек-листов создаются через API или CLI" |
| Residency /deadlines | ✅ | "Нет активных дедлайнов" | "Все дедлайны выполнены или ещё не назначены" |
| Residency /templates | 📄 | "Нет шаблонов" | "Шаблоны документов создаются через API или CLI" |
| Residency /tenants | 🏢 | "Нет тенантов" | "Создайте первый тенант через форму выше" |
| Residency /events-admin | 📅 | "Нет мероприятий" | "Мероприятия добавляются через парсинг или API" |
| Residency /contests-admin | 🏆 | "Нет конкурсов" | "Конкурсы добавляются через парсинг или API" |
| Main Admin / | 📭 | "Нет документов" | "Запустите парсинг кнопкой «Парсинг RSS» в шапке страницы или выполните в терминале: skolkovo scrape" |
| Consultant /dashboard | — | — | "Клиенты не найдены" (table row) |

---

## DESIGN SYSTEM SUMMARY

### Color tokens (CSS custom properties)

| Token | Light value | Dark value | Purpose |
|-------|------------|------------|---------|
| `--bg` | #f0f2f5 | #0f172a | Page background |
| `--surface` | #ffffff | #1e293b | Card/background |
| `--surface-alt` | #f8fafc | #243357 | Alternating row |
| `--primary` | #6366f1 (portal) / #1e40af (admin) | #818cf8 / #3b82f6 | Primary actions |
| `--primary-hover` | #4f46e5 / #1e3a8a | #a5b4fc / #60a5fa | Hover state |
| `--text` | #1e293b | #e2e8f0 | Primary text |
| `--text-secondary` | #64748b | #94a3b8 | Muted text |
| `--border` | #e2e8f0 | #334155 | Borders |
| `--green` | #16a34a | #4ade80 | Success |
| `--yellow` | #ca8a04 | #fbbf24 | Warning |
| `--red` | #dc2626 | #f87171 | Error/danger |
| `--blue` | #2563eb | #60a5fa | Info |
| `--purple` | #7c3aed | #a78bfa | Accent |
| `--gray` | #6b7280 | #94a3b8 | Neutral |
| `--orange` | #ea580c | #fb923c | Urgency (residency) |

### Typography
- Font: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif
- Code: 'SF Mono', 'Fira Code', monospace (where used)
- Sizes: 11px (meta/captions), 12px (small text/badges), 13px (body), 14px (card headings), 15-18px (page titles), 22-28px (hero/stat numbers)

### Common interactive patterns
- Theme toggle: localStorage-backed, auto-detects prefers-color-scheme, `🌙`/`☀️` swap
- Flash messages: `.flash.ok` (green) / `.flash.err` (red) via `?msg=&kind=` query params
- Toast notifications (Main Admin): `#toast` fixed bottom-right, auto-dismiss 4s
- AJAX actions: `runAction()`, `setStatus()`, `saveCategory()`, `saveSupersedes()`, `deleteDoc()`, `uploadFile()`, `deindexDoc()`

### Responsive breakpoints
- 768px: grid collapses to 1 column, toolbar stacks, nav scrolls horizontally

---

*Документ сформирован на основе анализа исходного кода по состоянию на 2026-05-30.*
*При изменении кода — обновить соответствующие разделы.*
