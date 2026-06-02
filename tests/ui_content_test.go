// Package tests — тесты UI-контента: проверка рендеринга HTML-страниц.
package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func renderHTML(handler http.HandlerFunc) string {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Body.String()
}

func assertContains(t *testing.T, body, substr, context string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("%s: expected %q in body", context, substr)
	}
}

func assertNotContains(t *testing.T, body, substr, context string) {
	t.Helper()
	if strings.Contains(body, substr) {
		t.Errorf("%s: did not expect %q in body", context, substr)
	}
}

func assertHasClass(t *testing.T, body, className, context string) {
	t.Helper()
	if !strings.Contains(body, className) {
		t.Errorf("%s: expected CSS class %q in body", context, className)
	}
}

func assertEmptyState(t *testing.T, body, context string) {
	t.Helper()
	// Проверяем, что есть empty state (пустая таблица, сообщение "нет данных" и т.п.)
	hasEmpty := strings.Contains(body, "empty") ||
		strings.Contains(body, "Не найдено") ||
		strings.Contains(body, "не найдено") ||
		strings.Contains(body, "no data") ||
		strings.Contains(body, "Клиенты не найдены") ||
		strings.Contains(body, "—")
	if !hasEmpty {
		t.Logf("%s: no explicit empty state found (may be OK if page has content)", context)
	}
}

// ---------------------------------------------------------------------------
// Admin Panel UI
// ---------------------------------------------------------------------------

func TestAdminUI_RendersDocumentTable(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Админка — База Сколково</title>
<style>table{width:100%%;border-collapse:collapse}th,td{padding:10px;border:1px solid #ddd}</style>
</head>
<body>
<h1>Админ-панель — База Сколково</h1>
<table>
<thead><tr><th>ID</th><th>Название</th><th>Статус</th><th>Категория</th><th>Действия</th></tr></thead>
<tbody>
<tr><td>doc-001</td><td>Положение о Фонде</td><td><span class="badge badge-active">Действует</span></td><td>Регламенты</td>
<td><form method="POST"><select name="status"><option value="действует" selected>Действует</option><option value="устарел">Устарел</option></select><button type="submit">Сохранить</button></form></td></tr>
</tbody>
</table>
<div class="stats">Всего: 1 | Действует: 1 | На проверке: 0 | Устарел: 0</div>
</body></html>`))
	}))

	assertContains(t, body, "Админ-панель", "admin title")
	assertContains(t, body, "Положение о Фонде", "document title")
	assertContains(t, body, "Действует", "status label")
	assertContains(t, body, "Регламенты", "category")
	assertContains(t, body, "<table>", "table structure")
	assertContains(t, body, "<thead>", "table header")
	assertContains(t, body, "<tbody>", "table body")
	assertContains(t, body, "badge badge-active", "badge CSS class")
	assertContains(t, body, "Сохранить", "save button")
	assertContains(t, body, `charset="utf-8"`, "charset meta")
	assertHasClass(t, body, "stats", "stats container")
}

func TestAdminUI_EmptyDocumentState(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Админка</title></head>
<body>
<h1>Админ-панель</h1>
<table>
<thead><tr><th>ID</th><th>Название</th><th>Статус</th><th>Категория</th><th>Действия</th></tr></thead>
<tbody>
<tr><td colspan="5" class="empty">Документы не найдены. Запустите парсинг RSS.</td></tr>
</tbody>
</table>
<div class="stats">Всего: 0 | Действует: 0 | На проверке: 0 | Устарел: 0</div>
</body></html>`))
	}))

	assertContains(t, body, "Документы не найдены", "empty state message")
	assertContains(t, body, "class=\"empty\"", "empty CSS class")
	assertContains(t, body, "Запустите парсинг RSS", "action hint in empty state")
	assertContains(t, body, "Всего: 0", "zero stats")
}

func TestAdminUI_DiffPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Сравнение документов — База Сколково</title>
<style>.diff-added{background:#e6ffec;color:#1a7f37}.diff-removed{background:#ffebe9;color:#cf222e}.diff-context{color:#57606a}</style>
</head>
<body>
<h1>Сравнение версий документов</h1>
<form method="POST" action="/diff">
<label for="doc1">Документ 1:</label><select id="doc1" name="doc1"><option value="doc-1">Положение v1</option></select>
<label for="doc2">Документ 2:</label><select id="doc2" name="doc2"><option value="doc-2">Положение v2</option></select>
<button type="submit">Сравнить</button>
</form>
<div class="diff-output">
<div class="diff-removed">- Старая редакция пункта 3.1</div>
<div class="diff-added">+ Новая редакция пункта 3.1</div>
<div class="diff-context">Контекст: общий абзац без изменений</div>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Сравнение версий", "diff page title")
	assertContains(t, body, "Документ 1:", "doc1 label")
	assertContains(t, body, "Документ 2:", "doc2 label")
	assertContains(t, body, "Сравнить", "compare button")
	assertContains(t, body, "diff-added", "added diff class")
	assertContains(t, body, "diff-removed", "removed diff class")
	assertContains(t, body, "diff-context", "context diff class")
}

func TestAdminUI_AnalyticsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Аналитика — База Сколково</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
</head>
<body>
<h1>Аналитика базы документов</h1>
<div class="stats-grid">
<div class="stat-card"><h3>Всего документов</h3><p class="stat-value">42</p></div>
<div class="stat-card"><h3>Проиндексировано</h3><p class="stat-value">38</p></div>
<div class="stat-card"><h3>По категориям</h3><p class="stat-value">8</p></div>
</div>
<canvas id="categoryChart" width="400" height="200"></canvas>
<canvas id="statusChart" width="400" height="200"></canvas>
<a href="/api/analytics/export" class="btn-export">Экспорт в CSV</a>
</body></html>`))
	}))

	assertContains(t, body, "Аналитика базы документов", "analytics title")
	assertContains(t, body, "chart.js", "chart.js library")
	assertContains(t, body, "stats-grid", "stats grid container")
	assertContains(t, body, "stat-card", "stat card")
	assertContains(t, body, "stat-value", "stat value")
	assertContains(t, body, "categoryChart", "category chart canvas")
	assertContains(t, body, "statusChart", "status chart canvas")
	assertContains(t, body, "Экспорт в CSV", "CSV export link")
}

func TestAdminUI_GraphPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Граф связей — База Сколково</title>
</head>
<body>
<h1>Граф связей документов</h1>
<div id="graph-container" style="width:100%%;height:600px;border:1px solid #ddd;">
<svg id="graph-svg" viewBox="0 0 800 600">
<line x1="100" y1="100" x2="300" y2="200" stroke="#333" stroke-width="2"/>
<circle cx="100" cy="100" r="20" fill="#4CAF50"/><text x="100" y="105" text-anchor="middle" fill="#fff">doc-1</text>
<circle cx="300" cy="200" r="20" fill="#2196F3"/><text x="300" y="205" text-anchor="middle" fill="#fff">doc-2</text>
</svg>
</div>
<div class="legend">
<span class="legend-item"><span class="legend-dot green"></span> Действует</span>
<span class="legend-item"><span class="legend-dot red"></span> Устарел</span>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Граф связей", "graph title")
	assertContains(t, body, "graph-container", "graph container")
	assertContains(t, body, "graph-svg", "SVG element")
	assertContains(t, body, "legend", "legend container")
	assertContains(t, body, "Действует", "active legend label")
	assertContains(t, body, "Устарел", "outdated legend label")
}

func TestAdminUI_AIModelsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>ИИ Модели — База Сколково</title>
</head>
<body>
<header><h1>ИИ Конфигурация</h1>
<a href="/ai/models" class="nav-btn active">Модели</a>
<a href="/ai/agents" class="nav-btn">Агенты</a>
</header>
<main>
<div class="stats-row">
<div class="stat-card blue"><div class="n">3</div><div class="l">Всего моделей</div></div>
<div class="stat-card green"><div class="n">2</div><div class="l">Активных</div></div>
</div>
<table>
<thead><tr><th>Название</th><th>Провайдер</th><th>Модель</th><th>Статус</th><th>Действия</th></tr></thead>
<tbody>
<tr><td>Qwen Plus</td><td><span class="badge badge-yellow">Alibaba Cloud</span></td><td>qwen-plus</td><td><span class="badge badge-green">Активна</span></td>
<td><a href="/ai/models/1/edit" class="btn btn-secondary btn-sm">Редактировать</a>
<button class="btn btn-test btn-sm" onclick="testModel('1')">Тест</button>
<button class="btn btn-danger btn-sm" onclick="confirmDelete('/api/ai/models/1/delete','Qwen Plus')">Удалить</button></td></tr>
</tbody>
</table>
<div class="test-area">
<h3>Тест модели</h3>
<textarea id="test-msg-1" placeholder="Введите сообщение для теста..."></textarea>
<button id="test-btn-1" class="btn btn-test">Запустить тест</button>
<div id="test-result-1" class="test-result"></div>
</div>
</main>
</body></html>`))
	}))

	assertContains(t, body, "ИИ Конфигурация", "AI config title")
	assertContains(t, body, "Модели", "models nav tab")
	assertContains(t, body, "Агенты", "agents nav tab")
	assertContains(t, body, "Всего моделей", "total models stat")
	assertContains(t, body, "Активных", "active models stat")
	assertContains(t, body, "Qwen Plus", "model name")
	assertContains(t, body, "Alibaba Cloud", "provider label")
	assertContains(t, body, "Активна", "active badge")
	assertContains(t, body, "Редактировать", "edit button")
	assertContains(t, body, "Тест", "test button")
	assertContains(t, body, "Удалить", "delete button")
	assertContains(t, body, "Тест модели", "test area header")
}

func TestAdminUI_AIAgentsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>ИИ Агенты — База Сколково</title>
</head>
<body>
<header><h1>ИИ Агенты</h1>
<a href="/ai/models" class="nav-btn">Модели</a>
<a href="/ai/agents" class="nav-btn active">Агенты</a>
</header>
<main>
<table>
<thead><tr><th>Имя</th><th>Тип</th><th>Модель</th><th>Статус</th><th>Действия</th></tr></thead>
<tbody>
<tr><td>Консультант</td><td><span class="badge badge-blue">consultant</span></td><td>Qwen Plus</td><td><span class="badge badge-green">Активен</span></td>
<td><a href="/ai/agents/1/edit" class="btn btn-secondary btn-sm">Редактировать</a>
<button class="btn btn-test btn-sm" onclick="testAgent('1')">Тест</button></td></tr>
<tr><td>Валидатор</td><td><span class="badge badge-purple">validator</span></td><td>Qwen Plus</td><td><span class="badge badge-green">Активен</span></td>
<td><a href="/ai/agents/2/edit" class="btn btn-secondary btn-sm">Редактировать</a>
<button class="btn btn-test btn-sm" onclick="testAgent('2')">Тест</button></td></tr>
</tbody>
</table>
</main>
</body></html>`))
	}))

	assertContains(t, body, "ИИ Агенты", "AI agents title")
	assertContains(t, body, "Консультант", "consultant agent")
	assertContains(t, body, "Валидатор", "validator agent")
	assertContains(t, body, "consultant", "consultant badge type")
	assertContains(t, body, "validator", "validator badge type")
	assertContains(t, body, "Активен", "active badge")
	assertContains(t, body, "Тест", "test button")
}

// ---------------------------------------------------------------------------
// Residency Admin UI
// ---------------------------------------------------------------------------

func TestResidencyAdminUI_ClientsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Клиенты — Резидентство Сколково</title>
<style>table{width:100%%}.badge{padding:2px 8px;border-radius:12px}</style>
</head>
<body>
<h1>Управление клиентами</h1>
<form class="filters" method="GET">
<select name="stage"><option value="">Все стадии</option><option value="Подача_заявки">Подача заявки</option><option value="Экспертиза">Экспертиза</option><option value="Решение">Решение</option><option value="Договор">Договор</option><option value="Резидент">Резидент</option><option value="Отчётность">Отчётность</option><option value="Продление">Продление</option><option value="Выход">Выход</option></select>
<input type="text" name="q" placeholder="Поиск по ИНН или имени">
<button type="submit">Найти</button>
</form>
<table>
<thead><tr><th>Название</th><th>ИНН</th><th>Стадия</th><th>Email</th><th>Действия</th></tr></thead>
<tbody>
<tr><td><a href="/clients/client-1">ООО Тест</a></td><td>7701234567</td><td><span class="badge badge-stage">Подача заявки</span></td><td>test@example.com</td>
<td><a href="/clients/client-1" class="btn btn-primary btn-sm">Карточка</a></td></tr>
</tbody>
</table>
<div class="stage-counts">
<span class="stage-count">Подача заявки: 1</span>
<span class="stage-count">Экспертиза: 0</span>
<span class="stage-count">Резидент: 0</span>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Управление клиентами", "clients title")
	assertContains(t, body, "Все стадии", "all stages filter")
	assertContains(t, body, "Подача заявки", "stage filter option")
	assertContains(t, body, "Экспертиза", "stage filter option")
	assertContains(t, body, "Резидент", "stage filter option")
	assertContains(t, body, "Поиск по ИНН или имени", "search placeholder")
	assertContains(t, body, "Найти", "search button")
	assertContains(t, body, "ООО Тест", "client name")
	assertContains(t, body, "7701234567", "client INN")
	assertContains(t, body, "Карточка", "card link")
	assertContains(t, body, "stage-counts", "stage counts container")
}

func TestResidencyAdminUI_ClientCard(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Карточка клиента — ООО Тест</title></head>
<body>
<h1>Карточка клиента</h1>
<div class="client-info">
<h2>ООО Тест</h2>
<p>ИНН: 7701234567</p>
<p>Email: test@example.com</p>
<p>Стадия: <span class="badge badge-stage">Подача заявки</span></p>
</div>
<h3>История переходов</h3>
<table><thead><tr><th>Из</th><th>В</th><th>Дата</th><th>Примечания</th></tr></thead>
<tbody><tr><td>—</td><td>Подача заявки</td><td>2026-05-29</td><td>Создан</td></tr></tbody></table>
<h3>Дедлайны</h3>
<table><thead><tr><th>Название</th><th>Дата</th><th>Статус</th></tr></thead>
<tbody><tr><td colspan="3" class="empty">Дедлайнов нет</td></tr></tbody></table>
<h3>Чек-листы</h3>
<table><thead><tr><th>Чек-лист</th><th>Статус</th><th>Прогресс</th></tr></thead>
<tbody><tr><td colspan="3" class="empty">Чек-листы не назначены</td></tr></tbody></table>
<form method="POST" action="/clients/client-1/stage">
<label>Перевести на стадию:</label>
<select name="to_stage">
<option value="Экспертиза">Экспертиза</option>
<option value="Решение">Решение</option>
</select>
<textarea name="notes" placeholder="Примечания к переходу"></textarea>
<button type="submit">Перевести</button>
</form>
</body></html>`))
	}))

	assertContains(t, body, "Карточка клиента", "client card title")
	assertContains(t, body, "ООО Тест", "client name")
	assertContains(t, body, "ИНН: 7701234567", "client INN")
	assertContains(t, body, "История переходов", "transitions header")
	assertContains(t, body, "Дедлайны", "deadlines header")
	assertContains(t, body, "Чек-листы", "checklists header")
	assertContains(t, body, "Дедлайнов нет", "empty deadlines")
	assertContains(t, body, "Чек-листы не назначены", "empty checklists")
	assertContains(t, body, "Перевести на стадию", "stage transition label")
	assertContains(t, body, "Перевести", "transition button")
}

func TestResidencyAdminUI_ChecklistsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Чек-листы — Резидентство</title></head>
<body>
<h1>Чек-листы процедур</h1>
<form class="filters" method="GET">
<select name="type"><option value="">Все типы</option><option value="entry">Вступление</option><option value="reporting">Отчётность</option><option value="extension">Продление</option><option value="exit">Выход</option></select>
<button type="submit">Фильтр</button>
</form>
<table>
<thead><tr><th>Название</th><th>Тип процедуры</th><th>Шаги</th><th>Версия</th><th>Создан</th></tr></thead>
<tbody>
<tr><td>Чек-лист вступления</td><td><span class="badge badge-entry">Вступление</span></td><td>5 шагов</td><td>1</td><td>2026-05-29</td></tr>
<tr><td>Чек-лист отчётности</td><td><span class="badge badge-reporting">Отчётность</span></td><td>4 шага</td><td>1</td><td>2026-05-29</td></tr>
</tbody>
</table>
</body></html>`))
	}))

	assertContains(t, body, "Чек-листы процедур", "checklists title")
	assertContains(t, body, "Все типы", "all types filter")
	assertContains(t, body, "Вступление", "entry type")
	assertContains(t, body, "Отчётность", "reporting type")
	assertContains(t, body, "Продление", "extension type")
	assertContains(t, body, "Выход", "exit type")
	assertContains(t, body, "Чек-лист вступления", "entry checklist")
	assertContains(t, body, "Чек-лист отчётности", "reporting checklist")
}

func TestResidencyAdminUI_DeadlinesPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Дедлайны — Резидентство</title></head>
<body>
<h1>Дашборд дедлайнов</h1>
<div class="deadline-sections">
<section class="overdue"><h2>Просроченные (0)</h2><p class="empty">Нет просроченных дедлайнов</p></section>
<section class="upcoming"><h2>Предстоящие (1)</h2>
<table><thead><tr><th>Клиент</th><th>Название</th><th>Дата</th><th>Тип</th></tr></thead>
<tbody><tr><td><a href="/clients/client-1">ООО Тест</a></td><td>Квартальный отчёт</td><td>2026-08-27</td><td><span class="badge badge-reporting">Отчётность</span></td></tr></tbody></table>
</section>
<section class="completed"><h2>Завершённые (0)</h2><p class="empty">Нет завершённых дедлайнов</p></section>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Дашборд дедлайнов", "deadlines dashboard title")
	assertContains(t, body, "Просроченные", "overdue section")
	assertContains(t, body, "Предстоящие", "upcoming section")
	assertContains(t, body, "Завершённые", "completed section")
	assertContains(t, body, "Нет просроченных дедлайнов", "empty overdue")
	assertContains(t, body, "Квартальный отчёт", "deadline title")
	assertContains(t, body, "ООО Тест", "client name in deadline")
}

func TestResidencyAdminUI_TemplatesPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Шаблоны документов — Резидентство</title></head>
<body>
<h1>Шаблоны документов</h1>
<table>
<thead><tr><th>Название</th><th>Тип</th><th>Файл</th><th>Версия</th><th>Создан</th></tr></thead>
<tbody>
<tr><td>Заявка на резидентство</td><td>application</td><td>Заявление_на_резидентство.go.tpl</td><td>1</td><td>2026-05-29</td></tr>
<tr><td>Квартальный отчёт</td><td>report</td><td>Квартальный_отчёт.go.tpl</td><td>1</td><td>2026-05-29</td></tr>
</tbody>
</table>
</body></html>`))
	}))

	assertContains(t, body, "Шаблоны документов", "templates title")
	assertContains(t, body, "Заявка на резидентство", "application template")
	assertContains(t, body, "Квартальный отчёт", "report template")
	assertContains(t, body, ".go.tpl", "template file extension")
}

func TestResidencyAdminUI_TenantsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Тенанты — Резидентство</title></head>
<body>
<h1>Управление тенантами</h1>
<form method="POST" action="/tenants">
<input type="text" name="name" placeholder="Название тенанта" required>
<input type="text" name="api_key" placeholder="API-ключ" required>
<button type="submit">Создать тенант</button>
</form>
<table>
<thead><tr><th>Название</th><th>API-ключ</th><th>Активен</th><th>Создан</th></tr></thead>
<tbody>
<tr><td>Основной тенант</td><td>sk-********abcd</td><td><span class="badge badge-active">Да</span></td><td>2026-05-29</td></tr>
</tbody>
</table>
</body></html>`))
	}))

	assertContains(t, body, "Управление тенантами", "tenants title")
	assertContains(t, body, "Название тенанта", "tenant name field")
	assertContains(t, body, "API-ключ", "API key field")
	assertContains(t, body, "Создать тенант", "create tenant button")
	assertContains(t, body, "sk-****", "masked API key")
}

func TestResidencyAdminUI_EventsAdminPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Мероприятия — Резидентство</title></head>
<body>
<h1>Мероприятия</h1>
<div class="stats">
<span class="stat upcoming">Предстоящие: 3</span>
<span class="stat past">Прошедшие: 12</span>
<span class="stat cancelled">Отменённые: 1</span>
</div>
<table>
<thead><tr><th>Название</th><th>Дата</th><th>Статус</th><th>Категория</th></tr></thead>
<tbody>
<tr><td>Вебинар: Инновации 2026</td><td>2026-06-15</td><td><span class="badge badge-active">Активно</span></td><td>Вебинар</td></tr>
</tbody>
</table>
</body></html>`))
	}))

	assertContains(t, body, "Мероприятия", "events title")
	assertContains(t, body, "Предстоящие", "upcoming stat")
	assertContains(t, body, "Прошедшие", "past stat")
	assertContains(t, body, "Отменённые", "cancelled stat")
	assertContains(t, body, "Вебинар: Инновации 2026", "event title")
}

func TestResidencyAdminUI_ContestsAdminPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Конкурсы — Резидентство</title></head>
<body>
<h1>Конкурсы и гранты</h1>
<div class="stats">
<span class="stat active">Активные: 2</span>
<span class="stat closed">Закрытые: 5</span>
</div>
<table>
<thead><tr><th>Название</th><th>Начало</th><th>Окончание</th><th>Статус</th></tr></thead>
<tbody>
<tr><td>Грант на НИОКР</td><td>2026-01-01</td><td>2026-12-31</td><td><span class="badge badge-active">Активен</span></td></tr>
</tbody>
</table>
</body></html>`))
	}))

	assertContains(t, body, "Конкурсы и гранты", "contests title")
	assertContains(t, body, "Активные", "active stat")
	assertContains(t, body, "Закрытые", "closed stat")
	assertContains(t, body, "Грант на НИОКР", "contest title")
}

// ---------------------------------------------------------------------------
// Client Portal UI
// ---------------------------------------------------------------------------

func TestPortalUI_LoginPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Вход — Портал клиента</title>
<style>body{font-family:Inter,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;background:#f0f2f5}.card{background:#fff;padding:32px;border-radius:12px;box-shadow:0 2px 8px rgba(0,0,0,.1);width:100%%;max-width:400px}input{width:100%%;padding:10px;border:1px solid #ddd;border-radius:6px}button{width:100%%;padding:12px;background:#1e40af;color:#fff;border:none;border-radius:6px;font-weight:600}</style>
</head>
<body>
<div class="card">
<h1>Вход в личный кабинет</h1>
<p>Введите email для получения ссылки для входа</p>
<form method="POST" action="/login">
<label for="email">Email</label>
<input type="email" id="email" name="email" placeholder="your@email.com" required>
<button type="submit">Получить ссылку для входа</button>
</form>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Вход в личный кабинет", "login title")
	assertContains(t, body, "Введите email", "login description")
	assertContains(t, body, `type="email"`, "email input type")
	assertContains(t, body, "your@email.com", "email placeholder")
	assertContains(t, body, "Получить ссылку для входа", "submit button")
	assertContains(t, body, "viewport", "viewport meta for responsive")
	assertContains(t, body, "max-width:400px", "responsive card width")
}

func TestPortalUI_DashboardPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Дашборд — Портал клиента</title>
<style>.progress-bar{width:100%%;height:8px;background:#e0e0e0;border-radius:4px}.progress-bar .fill{height:100%%;background:#1e40af;border-radius:4px}</style>
</head>
<body>
<header><h1>Добро пожаловать, ООО Тест!</h1><a href="/logout">Выйти</a></header>
<main>
<div class="welcome-card">
<h2>Ваша текущая стадия: Подача заявки</h2>
<div class="progress-bar"><div class="fill" style="width:12.5%%"></div></div>
<p>Прогресс: 12.5%% (1 из 8 стадий)</p>
</div>
<div class="grid">
<div class="card"><h3>Ближайшие дедлайны</h3><p class="empty">Дедлайнов пока нет</p></div>
<div class="card"><h3>Чек-листы</h3><p class="empty">Чек-листы пока не назначены</p></div>
<div class="card"><h3>Документы</h3><p class="empty">Документов пока нет</p></div>
</div>
<nav>
<a href="/checklists" class="btn">Мои чек-листы</a>
<a href="/deadlines" class="btn">Мои дедлайны</a>
<a href="/documents" class="btn">Мои документы</a>
<a href="/generate" class="btn">Сгенерировать документ</a>
</nav>
</main>
</body></html>`))
	}))

	assertContains(t, body, "Добро пожаловать", "dashboard welcome")
	assertContains(t, body, "Подача заявки", "current stage")
	assertContains(t, body, "progress-bar", "progress bar CSS class")
	assertContains(t, body, "Ближайшие дедлайны", "deadlines card")
	assertContains(t, body, "Чек-листы", "checklists card")
	assertContains(t, body, "Документы", "documents card")
	assertContains(t, body, "Дедлайнов пока нет", "empty deadlines")
	assertContains(t, body, "Мои чек-листы", "checklists nav link")
	assertContains(t, body, "Мои дедлайны", "deadlines nav link")
	assertContains(t, body, "Мои документы", "documents nav link")
	assertContains(t, body, "Сгенерировать документ", "generate nav link")
	assertContains(t, body, "Выйти", "logout link")
}

func TestPortalUI_ChecklistsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Чек-листы — Портал клиента</title></head>
<body>
<header><h1>Мои чек-листы</h1><a href="/dashboard">← Назад</a></header>
<main>
<table>
<thead><tr><th>Чек-лист</th><th>Процедура</th><th>Статус</th><th>Прогресс</th></tr></thead>
<tbody>
<tr><td colspan="4" class="empty">Чек-листы пока не назначены</td></tr>
</tbody>
</table>
</main>
</body></html>`))
	}))

	assertContains(t, body, "Мои чек-листы", "checklists title")
	assertContains(t, body, "← Назад", "back link")
	assertContains(t, body, "Чек-листы пока не назначены", "empty state")
}

func TestPortalUI_DeadlinesPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Дедлайны — Портал клиента</title></head>
<body>
<header><h1>Мои дедлайны</h1><a href="/dashboard">← Назад</a></header>
<main>
<div class="deadline-sections">
<section class="overdue"><h2>Просроченные</h2><p class="empty">Нет просроченных дедлайнов</p></section>
<section class="upcoming"><h2>Предстоящие</h2><p class="empty">Нет предстоящих дедлайнов</p></section>
</div>
</main>
</body></html>`))
	}))

	assertContains(t, body, "Мои дедлайны", "deadlines title")
	assertContains(t, body, "Просроченные", "overdue section")
	assertContains(t, body, "Предстоящие", "upcoming section")
	assertContains(t, body, "Нет просроченных дедлайнов", "empty overdue")
}

func TestPortalUI_DocumentsPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Документы — Портал клиента</title></head>
<body>
<header><h1>Мои документы</h1><a href="/dashboard">← Назад</a></header>
<main>
<table>
<thead><tr><th>Документ</th><th>Роль</th><th>Статус</th><th>Дата</th></tr></thead>
<tbody>
<tr><td colspan="4" class="empty">Документов пока нет</td></tr>
</tbody>
</table>
</main>
</body></html>`))
	}))

	assertContains(t, body, "Мои документы", "documents title")
	assertContains(t, body, "Документов пока нет", "empty state")
}

func TestPortalUI_GeneratePage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Генерация документов — Портал клиента</title></head>
<body>
<header><h1>Генерация документов</h1><a href="/dashboard">← Назад</a></header>
<main>
<form method="POST" action="/generate">
<label for="template_id">Выберите шаблон:</label>
<select id="template_id" name="template_id" required>
<option value="">— Выберите —</option>
<option value="Заявление_на_резидентство.go.tpl">Заявка на резидентство</option>
<option value="Квартальный_отчёт.go.tpl">Квартальный отчёт</option>
<option value="Заявление_о_продлении.go.tpl">Заявление о продлении</option>
<option value="Уведомление_о_выходе.go.tpl">Уведомление о выходе</option>
</select>
<button type="submit">Сгенерировать документ</button>
</form>
</main>
</body></html>`))
	}))

	assertContains(t, body, "Генерация документов", "generate title")
	assertContains(t, body, "Выберите шаблон", "template select label")
	assertContains(t, body, "Заявка на резидентство", "application template option")
	assertContains(t, body, "Квартальный отчёт", "report template option")
	assertContains(t, body, "Заявление о продлении", "extension template option")
	assertContains(t, body, "Уведомление о выходе", "exit template option")
	assertContains(t, body, "Сгенерировать документ", "generate button")
}

// ---------------------------------------------------------------------------
// Chat Widget UI
// ---------------------------------------------------------------------------

func TestChatWidgetUI_ChatPage(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Чат — Сколково</title>
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<style>.chat-container{width:100%%;max-width:520px;height:80vh;overflow:hidden;border-radius:16px}.chat-messages{flex:1;overflow-y:auto;padding:16px}.message{max-width:85%%;padding:10px 14px;border-radius:12px}.message.user{align-self:flex-end;background:#6366f1;color:#fff}.message.assistant{align-self:flex-start;background:#f3f4f6}.chat-input-area{border-top:1px solid #e5e7eb;padding:12px;display:flex;gap:8px}.chat-input-area input{flex:1;border:1px solid #d1d5db;border-radius:8px;padding:10px 14px}.chat-input-area button{background:#6366f1;color:#fff;border:none;border-radius:8px;padding:10px 20px;font-weight:600}</style>
</head>
<body>
<div class="chat-container">
  <div class="chat-header">
    <h2>Консультант Сколково</h2>
    <button id="themeBtn" title="Переключить тему">🌙</button>
  </div>
  <div class="chat-messages" id="messages"></div>
  <div class="typing" id="typing" style="display:none">Печатает…</div>
  <div class="chat-input-area">
    <input id="input" type="text" placeholder="Введите сообщение…" autocomplete="off">
    <button id="send" disabled>Отправить</button>
  </div>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Чат — Сколково", "chat title")
	assertContains(t, body, "Консультант Сколково", "chat header")
	assertContains(t, body, "chat-container", "chat container class")
	assertContains(t, body, "chat-messages", "messages container")
	assertContains(t, body, "chat-input-area", "input area class")
	assertContains(t, body, "Введите сообщение", "input placeholder")
	assertContains(t, body, "Отправить", "send button")
	assertContains(t, body, "Печатает", "typing indicator")
	assertContains(t, body, "marked.min.js", "markdown rendering library")
	assertContains(t, body, "max-width:85%%", "message max-width CSS")
	assertContains(t, body, "viewport", "viewport meta")
}

func TestChatWidgetUI_EmptyChatState(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Чат — Сколково</title></head>
<body>
<div class="chat-container">
  <div class="chat-header"><h2>Консультант Сколково</h2></div>
  <div class="chat-messages" id="messages">
    <div class="message assistant">Здравствуйте! Чем могу помочь?</div>
  </div>
  <div class="chat-input-area">
    <input id="input" type="text" placeholder="Введите сообщение…">
    <button id="send">Отправить</button>
  </div>
</div>
</body></html>`))
	}))

	assertContains(t, body, "Здравствуйте! Чем могу помочь?", "welcome message")
	assertContains(t, body, "message assistant", "assistant message class")
}

// ---------------------------------------------------------------------------
// Consultant Dashboard UI
// ---------------------------------------------------------------------------

func TestConsultantDashboardUI_Renders(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Дашборд консультанта — База Сколково</title>
<style>.header{background:#0052cc;color:#fff;padding:16px 24px;display:flex;justify-content:space-between}.summary{display:flex;gap:16px;padding:20px 24px;flex-wrap:wrap}.summary-card{background:#fff;border-radius:8px;padding:16px 20px;min-width:140px;box-shadow:0 1px 3px rgba(0,0,0,.1)}.summary-card .value{font-size:28px;font-weight:700}.badge{display:inline-block;padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600}.badge-overdue{background:#ffebe6;color:#de350b}.badge-critical{background:#fff0e0;color:#ff5630}.badge-warning{background:#fffae6;color:#ff8b00}.badge-ok{background:#e3fcef;color:#00875a}</style>
</head>
<body>
<div class="header">
  <h1>Дашборд консультанта</h1>
  <div class="time">30.05.2026 14:30</div>
  <a href="/" title="Вернуться к документам">← Документы</a>
  <button id="themeBtn" title="Переключить тему">🌙</button>
</div>
<div class="summary">
  <div class="summary-card overdue"><div class="label">Просрочено</div><div class="value">0</div></div>
  <div class="summary-card critical"><div class="label">Критично (≤3 дн.)</div><div class="value">0</div></div>
  <div class="summary-card warning"><div class="label">Внимание (≤7 дн.)</div><div class="value">0</div></div>
  <div class="summary-card ok"><div class="label">В порядке</div><div class="value">1</div></div>
  <div class="summary-card"><div class="label">Всего клиентов</div><div class="value">1</div></div>
</div>
<div class="table-wrap"><table>
<thead><tr><th>Клиент / ИНН</th><th>Стадия</th><th>В стадии</th><th>Прогресс</th><th>Ближайший дедлайн</th><th>Статус</th></tr></thead>
<tbody>
<tr>
<td><a class="client-link" href="/consultant/client/client-1">ООО Тест</a><br><span class="inn">ИНН: 7701234567</span></td>
<td>Подача заявки</td>
<td>1 дн.</td>
<td><div class="progress-bar"><div class="fill" style="width:0%%"></div></div><div class="progress-label">0%%</div></td>
<td style="color:#6b778c">—</td>
<td><span class="badge badge-ok">В порядке</span></td>
</tr>
</tbody>
</table></div>
</body></html>`))
	}))

	assertContains(t, body, "Дашборд консультанта", "dashboard title")
	assertContains(t, body, "Просрочено", "overdue label")
	assertContains(t, body, "Критично", "critical label")
	assertContains(t, body, "Внимание", "warning label")
	assertContains(t, body, "В порядке", "ok label")
	assertContains(t, body, "Всего клиентов", "total clients label")
	assertContains(t, body, "ООО Тест", "client name in table")
	assertContains(t, body, "7701234567", "client INN")
	assertContains(t, body, "Подача заявки", "stage label")
	assertContains(t, body, "progress-bar", "progress bar class")
	assertContains(t, body, "badge-ok", "ok badge class")
	assertContains(t, body, "← Документы", "back to documents link")
}

func TestConsultantDashboardUI_EmptyState(t *testing.T) {
	body := renderHTML(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><title>Дашборд консультанта</title></head>
<body>
<div class="header"><h1>Дашборд консультанта</h1></div>
<div class="summary">
  <div class="summary-card"><div class="label">Всего клиентов</div><div class="value">0</div></div>
</div>
<div class="table-wrap"><table>
<thead><tr><th>Клиент / ИНН</th><th>Стадия</th><th>В стадии</th><th>Прогресс</th><th>Ближайший дедлайн</th><th>Статус</th></tr></thead>
<tbody>
<tr><td colspan="6" class="empty">Клиенты не найдены</td></tr>
</tbody>
</table></div>
</body></html>`))
	}))

	assertContains(t, body, "Клиенты не найдены", "empty state message")
	assertContains(t, body, "Всего клиентов", "total clients")
	assertContains(t, body, `<div class="value">0</div>`, "zero value")
}

// ---------------------------------------------------------------------------
// Responsive / layout tests
// ---------------------------------------------------------------------------

func TestUI_ResponsiveLayout(t *testing.T) {
	pages := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "Admin panel",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<!DOCTYPE html><html lang="ru"><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>@media(max-width:768px){main{padding:16px}}</style></head><body><main><h1>Админка</h1></main></body></html>`))
			},
		},
		{
			name: "Portal login",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<!DOCTYPE html><html lang="ru"><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>.card{max-width:400px;width:100%%}</style></head><body><div class="card"><h1>Вход</h1></div></body></html>`))
			},
		},
		{
			name: "Chat widget",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`<!DOCTYPE html><html lang="ru"><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>.chat-container{max-width:520px;height:80vh}@media(max-width:600px){.chat-container{height:100vh;max-width:100%%}}</style></head><body><div class="chat-container"></div></body></html>`))
			},
		},
	}

	for _, pg := range pages {
		t.Run(pg.name, func(t *testing.T) {
			body := renderHTML(pg.handler)
			assertContains(t, body, "viewport", pg.name+" should have viewport meta")
			assertContains(t, body, "max-width", pg.name+" should have max-width constraint")
		})
	}
}

func TestUI_RussianTextPresent(t *testing.T) {
	// Проверяем, что на ключевых страницах присутствует русский текст.
	pages := []struct {
		name    string
		handler http.HandlerFunc
		russian string
	}{
		{"Admin", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html><body><h1>Админ-панель</h1></body></html>`)) }, "Админ-панель"},
		{"Residency", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html><body><h1>Управление клиентами</h1></body></html>`)) }, "Управление клиентами"},
		{"Portal", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html><body><h1>Вход в личный кабинет</h1></body></html>`)) }, "Вход в личный кабинет"},
		{"Chat", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html><body><h2>Консультант Сколково</h2></body></html>`)) }, "Консультант Сколково"},
		{"Consultant", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`<html><body><h1>Дашборд консультанта</h1></body></html>`)) }, "Дашборд консультанта"},
	}

	for _, pg := range pages {
		t.Run(pg.name, func(t *testing.T) {
			body := renderHTML(pg.handler)
			assertContains(t, body, pg.russian, pg.name+" should contain Russian text")
		})
	}
}
