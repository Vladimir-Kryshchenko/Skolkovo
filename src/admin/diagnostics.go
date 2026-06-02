// Раздел «Диагностика» (:8090, /health): единый экран «что сломано и как починить».
//
// Зачем: подсистемы (Postgres, Qdrant, TEI-эмбеддинги, ИИ-модели и агенты, источники,
// прокси/WAF, кука dochub, сессии) раньше проверялись врозь и только косвенно. Здесь —
// одно место, где администратор видит статус каждого узла и КОНКРЕТНЫЙ способ решения
// проблемы (текст + ссылка на нужную страницу), а не просто «что-то не работает».
//
// Все проверки «живые»: выполняются при каждом открытии страницы (кнопка
// «Перепроверить» = перезагрузка). Внешние вызовы идут параллельно, каждый со своим
// таймаутом, чтобы один зависший узел не тормозил весь экран.
package admin

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/common/embed"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/health"
)

// diagStatus — итог одной проверки.
type diagStatus string

const (
	diagOK   diagStatus = "ok"   // работает
	diagWarn diagStatus = "warn" // работает, но есть риск/ограничение
	diagFail diagStatus = "fail" // не работает — требует вмешательства
	diagSkip diagStatus = "skip" // компонент не подключён в этой сборке
)

// diagCheck — результат проверки одного узла.
type diagCheck struct {
	Name     string     // что проверяли
	Status   diagStatus // итог
	Detail   string     // что обнаружено (человекочитаемо)
	Fix      string     // как починить (пусто для OK/Skip)
	FixURL   string     // ссылка на страницу-решение (опционально)
	FixLabel string     // подпись ссылки
}

// diagGroup — группа проверок одной подсистемы.
type diagGroup struct {
	Title  string
	Checks []diagCheck
}

// pendingCheck — отложенная проверка с привязкой к группе (для параллельного запуска).
type pendingCheck struct {
	group string
	fn    func(context.Context) diagCheck
}

// runDiagnostics выполняет все проверки параллельно и собирает их в группы,
// сохраняя порядок объявления.
func (s *Server) runDiagnostics(ctx context.Context, r *http.Request) []diagGroup {
	const (
		gInfra    = "Хранилища и инфраструктура"
		gAI       = "Искусственный интеллект"
		gSources  = "Источники данных"
		gDochub   = "Доступ к dochub (обход WAF)"
		gSessions = "Сессии и доступ"
	)

	pend := []pendingCheck{
		{gInfra, s.checkPostgres},
		{gInfra, s.checkQdrant},
		{gInfra, s.checkEmbeddings},
		{gAI, s.checkAIModel},
		{gAI, s.checkAIAgents},
		{gSources, s.checkSources},
		{gDochub, s.checkProxy},
		{gDochub, s.checkDochubCookie},
		{gSessions, func(ctx context.Context) diagCheck { return s.checkSession(r) }},
	}

	results := make([]diagCheck, len(pend))
	var wg sync.WaitGroup
	for i, p := range pend {
		wg.Add(1)
		go func(i int, fn func(context.Context) diagCheck) {
			defer wg.Done()
			defer func() {
				// Проверка не должна ронять всю страницу из-за паники в узле.
				if rec := recover(); rec != nil {
					results[i] = diagCheck{Name: "Проверка", Status: diagFail,
						Detail: fmt.Sprintf("внутренняя ошибка проверки: %v", rec)}
				}
			}()
			results[i] = fn(ctx)
		}(i, p.fn)
	}
	wg.Wait()

	// Собираем по группам в порядке первого появления.
	order := []string{}
	byGroup := map[string][]diagCheck{}
	for i, p := range pend {
		if _, ok := byGroup[p.group]; !ok {
			order = append(order, p.group)
		}
		byGroup[p.group] = append(byGroup[p.group], results[i])
	}
	groups := make([]diagGroup, 0, len(order))
	for _, g := range order {
		groups = append(groups, diagGroup{Title: g, Checks: byGroup[g]})
	}
	return groups
}

// ── Отдельные проверки ───────────────────────────────────────────────────────

// checkPostgres пингует пул соединений Postgres. В dev-сборке с файловым
// хранилищем (не *store.PostgresStore) проверка неприменима.
func (s *Server) checkPostgres(ctx context.Context) diagCheck {
	c := diagCheck{Name: "PostgreSQL — реестр и метаданные"}
	ps, ok := s.store.(*store.PostgresStore)
	if !ok {
		c.Status = diagSkip
		c.Detail = "используется файловое хранилище (dev-режим), Postgres не задействован"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := ps.Pool().Ping(ctx); err != nil {
		c.Status = diagFail
		c.Detail = "нет соединения с базой: " + err.Error()
		c.Fix = "Проверьте, что контейнер postgres запущен (docker compose ps), и что DATABASE_URL в .env указывает на него. Перезапуск: docker compose up -d postgres."
		return c
	}
	c.Status = diagOK
	c.Detail = "соединение установлено, пул отвечает"
	return c
}

// checkQdrant делает лёгкий read-only запрос к Qdrant (scroll на 1 точку).
func (s *Server) checkQdrant(ctx context.Context) diagCheck {
	c := diagCheck{Name: "Qdrant — векторная база поиска"}
	if s.rag == nil || s.rag.Qdr == nil {
		c.Status = diagSkip
		c.Detail = "RAG-сервис не подключён к этой админке"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, _, err := s.rag.Qdr.Scroll(ctx, 1, nil, nil, false); err != nil {
		c.Status = diagFail
		c.Detail = "коллекция недоступна: " + err.Error()
		c.Fix = "Проверьте контейнер qdrant и переменную QDRANT_URL. Если база только что развёрнута — коллекция создаётся при старте serve (Init). Перезапуск: docker compose up -d qdrant."
		return c
	}
	c.Status = diagOK
	c.Detail = "коллекция доступна, поиск отвечает"
	return c
}

// checkEmbeddings прогоняет пробный эмбеддинг через TEI — это самый частый узкий
// узел: на CPU TEI медленный и капризный к размеру батча.
func (s *Server) checkEmbeddings(ctx context.Context) diagCheck {
	c := diagCheck{Name: "TEI — сервис эмбеддингов"}
	if s.rag == nil || s.rag.Emb == nil {
		c.Status = diagSkip
		c.Detail = "эмбеддер не подключён к этой админке"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	vecs, err := s.rag.Emb.Embed(ctx, []string{embed.PrefixQuery + "проверка работоспособности"})
	if err != nil {
		c.Status = diagFail
		c.Detail = "не удалось получить эмбеддинг: " + err.Error()
		c.Fix = "Проверьте контейнер tei и переменную EMBEDDINGS_URL. На холодном старте модель multilingual-e5 загружается до минуты — повторите проверку. Перезапуск: docker compose up -d tei."
		return c
	}
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	c.Status = diagOK
	c.Detail = fmt.Sprintf("эмбеддинг получен, размерность вектора: %d", dim)
	return c
}

// checkAIModel находит включённую LLM-модель с ключом и делает к ней пробный
// запрос — это и есть проверка «ИИ не работает».
func (s *Server) checkAIModel(ctx context.Context) diagCheck {
	c := diagCheck{Name: "LLM-модель — генеративный ИИ", FixURL: "/ai/models", FixLabel: "Открыть ИИ-модели"}
	if s.aiStore == nil {
		c.Status = diagSkip
		c.Detail = "конфигурация ИИ-моделей требует Postgres и здесь не подключена"
		c.FixURL, c.FixLabel = "", ""
		return c
	}
	models, err := s.aiStore.ListModels(ctx)
	if err != nil {
		c.Status = diagFail
		c.Detail = "не удалось прочитать список моделей: " + err.Error()
		c.Fix = "Проверьте Postgres (см. проверку PostgreSQL выше) — конфигурация моделей хранится в нём."
		return c
	}
	var active *aimodels.Model
	for i := range models {
		if models[i].Enabled && strings.TrimSpace(models[i].APIKey) != "" {
			active = &models[i]
			break
		}
	}
	if active == nil {
		c.Status = diagWarn
		c.Detail = "нет ни одной включённой модели с заданным API-ключом — чат-консультант, валидатор и аннотатор страниц работать не будут"
		c.Fix = "Добавьте модель (Qwen / OpenAI / Anthropic / Custom), укажите API-ключ и включите её."
		return c
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cl := aimodels.NewClient(*active)
	if _, _, err := cl.Chat(cctx, []aimodels.ChatMessage{{Role: "user", Content: "Ответь одним словом: тест"}}); err != nil {
		c.Status = diagFail
		c.Detail = fmt.Sprintf("модель «%s» (%s) не отвечает: %s", active.Name, active.ModelID, err.Error())
		c.Fix = "Проверьте корректность API-ключа и BaseURL, баланс/лимиты у провайдера и доступность его API из сети сервера (для зарубежных провайдеров может потребоваться прокси)."
		return c
	}
	c.Status = diagOK
	c.Detail = fmt.Sprintf("активная модель «%s» (%s) отвечает", active.Name, active.ModelID)
	return c
}

// checkAIAgents проверяет ключевых агентов, без которых ломаются конкретные сценарии:
// Консультант (чат-виджет, Telegram-бот) и Аннотатор страниц (обогащение sitepages).
func (s *Server) checkAIAgents(ctx context.Context) diagCheck {
	c := diagCheck{Name: "ИИ-агенты — Консультант и Аннотатор", FixURL: "/ai/agents", FixLabel: "Открыть ИИ-агентов"}
	if s.aiStore == nil {
		c.Status = diagSkip
		c.Detail = "конфигурация агентов требует Postgres и здесь не подключена"
		c.FixURL, c.FixLabel = "", ""
		return c
	}
	var problems []string
	if _, _, err := s.aiStore.EnabledAgentWithModel(ctx, aimodels.AgentConsultant); err != nil {
		problems = append(problems, "Консультант не настроен (нет включённого агента с рабочей моделью) — не будут работать чат-виджет и Telegram-бот")
	}
	if _, _, err := s.aiStore.EnabledAgentWithModel(ctx, aimodels.AgentPageAnnotator); err != nil {
		problems = append(problems, "Аннотатор страниц не настроен — ИИ-обогащение страниц сайта (теги, описания) выполняться не будет")
	}
	if len(problems) > 0 {
		c.Status = diagWarn
		c.Detail = strings.Join(problems, "; ")
		c.Fix = "Включите нужного агента и привяжите его к включённой модели с API-ключом."
		return c
	}
	c.Status = diagOK
	c.Detail = "Консультант и Аннотатор страниц включены и привязаны к рабочим моделям"
	return c
}

// checkSources сводит SLA-мониторинг источников: есть ли падающие или протухшие.
func (s *Server) checkSources(ctx context.Context) diagCheck {
	c := diagCheck{Name: "Свежесть источников", FixURL: "/changes", FixLabel: "Открыть «Изменения»"}
	if s.healthStore == nil {
		c.Status = diagSkip
		c.Detail = "мониторинг свежести источников не подключён"
		c.FixURL, c.FixLabel = "", ""
		return c
	}
	sources, err := s.healthStore.List(ctx)
	if err != nil {
		c.Status = diagFail
		c.Detail = "не удалось получить состояние источников: " + err.Error()
		return c
	}
	if len(sources) == 0 {
		c.Status = diagWarn
		c.Detail = "ни один источник ещё не запускался — данные не собираются"
		c.Fix = "Запустите сбор данных (команда serve с включёнными парсерами) и дождитесь первого прогона."
		return c
	}
	now := time.Now()
	var failing, stale []string
	for _, src := range sources {
		switch src.State(24*time.Hour, now) {
		case health.StatusFailing:
			failing = append(failing, sourceLabel(src.Name))
		case health.StatusStale:
			stale = append(stale, sourceLabel(src.Name))
		}
	}
	sort.Strings(failing)
	sort.Strings(stale)
	switch {
	case len(failing) > 0:
		c.Status = diagFail
		c.Detail = "источники со стабильными ошибками: " + strings.Join(failing, ", ")
		if len(stale) > 0 {
			c.Detail += "; давно не обновлялись: " + strings.Join(stale, ", ")
		}
		c.Fix = "Откройте «Изменения» → панель свежести: там показан текст последней ошибки по каждому источнику. Частые причины — недоступность внешнего сайта, нужен прокси для dochub, изменилась структура страницы."
	case len(stale) > 0:
		c.Status = diagWarn
		c.Detail = "давно (>24 ч) не обновлялись: " + strings.Join(stale, ", ")
		c.Fix = "Проверьте расписание конвейера и доступность источников. Возможно, прогон просто ещё не наступил."
	default:
		c.Status = diagOK
		c.Detail = fmt.Sprintf("все источники свежие (всего отслеживается: %d)", len(sources))
	}
	return c
}

// checkProxy проверяет, выбран ли активный прокси для обхода WAF dochub.
func (s *Server) checkProxy(ctx context.Context) diagCheck {
	c := diagCheck{Name: "Прокси для обхода WAF", FixURL: "/proxy", FixLabel: "Открыть «Прокси / VPN»"}
	if s.proxyManager == nil {
		c.Status = diagSkip
		c.Detail = "менеджер прокси не подключён"
		c.FixURL, c.FixLabel = "", ""
		return c
	}
	if s.proxyManager.GetActiveURL() == "" {
		c.Status = diagWarn
		c.Detail = "активный прокси не выбран — тела файлов dochub.sk.ru скачиваться не будут (WAF отдаёт 403)"
		c.Fix = "Добавьте рабочий (желательно российский) прокси и активируйте его. Каталог документов при этом наполняется и без прокси — затрагивается только автоскачивание файлов."
		return c
	}
	c.Status = diagOK
	c.Detail = "активный прокси выбран"
	return c
}

// checkDochubCookie проверяет наличие сессионной куки dochub для HTTP-скачивания.
func (s *Server) checkDochubCookie(ctx context.Context) diagCheck {
	c := diagCheck{Name: "Кука dochub для скачивания файлов", FixURL: "/proxy", FixLabel: "Открыть «Прокси / VPN»"}
	if s.cookieStore == nil {
		c.Status = diagSkip
		c.Detail = "хранилище куки dochub не подключено"
		c.FixURL, c.FixLabel = "", ""
		return c
	}
	if s.cookieStore.Get() == "" {
		c.Status = diagWarn
		c.Detail = "кука dochub не задана — альтернативный способ скачивания тел файлов по сессии недоступен"
		c.Fix = "Авторизуйтесь на dochub.sk.ru в браузере, скопируйте cookie и вставьте его в поле на странице «Прокси / VPN». Это резервный путь к файлам, если прокси не справляется."
		return c
	}
	c.Status = diagOK
	c.Detail = "кука задана, сохранена: " + s.cookieStore.SavedAtStr()
	return c
}

// checkSession отчитывается о сессии текущего администратора. Раз страница
// открылась через requireAuth — куки доходят; даём диагностику типовых проблем входа.
func (s *Server) checkSession(r *http.Request) diagCheck {
	c := diagCheck{Name: "Сессия и cookie входа"}
	https := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	scheme := "HTTP"
	if https {
		scheme = "HTTPS"
	}
	c.Status = diagOK
	c.Detail = fmt.Sprintf("сессионная cookie «%s» принимается, соединение: %s", sessionCookieName, scheme)
	// Сессии хранятся в памяти процесса — это главная причина «куки вдруг перестали
	// работать»: после перезапуска сервиса все входы сбрасываются.
	c.Fix = "Если вход «слетает» — учтите: сессии живут в памяти и сбрасываются при перезапуске сервиса (нужно войти заново; срок куки — 24 ч). Если cookie вовсе не сохраняется — проверьте, что заходите по одному и тому же адресу/домену, в браузере не заблокированы cookie, а за nginx корректно проксируется заголовок Cookie."
	return c
}

// ── HTTP-обработчик и рендер ──────────────────────────────────────────────────

// handleDiagnostics рендерит страницу диагностики со свежими проверками.
func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	groups := s.runDiagnostics(r.Context(), r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, renderDiagnosticsPage(groups, time.Now()))
}

// diagBadge возвращает CSS-класс и подпись для статуса проверки.
func diagBadge(st diagStatus) (cls, label string) {
	switch st {
	case diagOK:
		return "d-ok", "Работает"
	case diagWarn:
		return "d-warn", "Внимание"
	case diagFail:
		return "d-fail", "Ошибка"
	default:
		return "d-skip", "Не подключено"
	}
}

func renderDiagnosticsPage(groups []diagGroup, at time.Time) string {
	var fails, warns int
	for _, g := range groups {
		for _, c := range g.Checks {
			switch c.Status {
			case diagFail:
				fails++
			case diagWarn:
				warns++
			}
		}
	}

	// Сводный баннер.
	bannerCls, bannerText := "d-ok", "Все системы в норме — проблем не обнаружено"
	if warns > 0 {
		bannerCls, bannerText = "d-warn", fmt.Sprintf("Требует внимания: %d", warns)
	}
	if fails > 0 {
		bannerCls = "d-fail"
		bannerText = fmt.Sprintf("Критических ошибок: %d", fails)
		if warns > 0 {
			bannerText += fmt.Sprintf(", предупреждений: %d", warns)
		}
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>База Сколково — Диагностика</title>
<link href="https://fonts.googleapis.com/css2?family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
:root {
  --bg:#f6f7fb; --surface:#fff; --surface-alt:#f0f2f5; --primary:#0073ea; --primary-hover:#005bb5;
  --primary-light:#e5f0fc; --text:#323338; --text-secondary:#676879; --border:#c3c6d4; --radius:8px;
  --shadow-sm:0 1px 4px rgba(0,0,0,.06); --shadow:0 2px 8px rgba(0,0,0,.08); --shadow-lg:0 8px 24px rgba(0,0,0,.1);
  --green:#008653; --green-bg:#f4f9f4; --green-border:#b7e4c7;
  --yellow:#7a5900; --yellow-bg:#fdf8e8; --yellow-border:#f5e0a0;
  --red:#7a0606; --red-bg:#fdf3f3; --red-border:#f5c6c6;
  --gray:#676879; --gray-bg:#f0f2f5;
  --font:'Figtree',-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;
}
@media (prefers-color-scheme: dark){ :root:not([data-theme="light"]){
  --bg:#181b2b; --surface:#23273a; --surface-alt:#2a2f45; --primary:#579dff; --primary-hover:#7db3ff;
  --primary-light:#1e3050; --text:#d0d1d8; --text-secondary:#9698a6; --border:#3b3f54;
  --shadow-sm:0 1px 4px rgba(0,0,0,.3); --shadow:0 2px 8px rgba(0,0,0,.4); --shadow-lg:0 8px 24px rgba(0,0,0,.5);
  --green:#4ade80; --green-bg:#1a2e1a; --green-border:#2d5a2d;
  --yellow:#fbbf24; --yellow-bg:#2e2408; --yellow-border:#5a4510;
  --red:#ff6b6b; --red-bg:#2e1a1a; --red-border:#5a2d2d; --gray:#9698a6; --gray-bg:#2a2f45;
}}
:root[data-theme="dark"]{
  --bg:#181b2b; --surface:#23273a; --surface-alt:#2a2f45; --primary:#579dff; --primary-hover:#7db3ff;
  --primary-light:#1e3050; --text:#d0d1d8; --text-secondary:#9698a6; --border:#3b3f54;
  --shadow-sm:0 1px 4px rgba(0,0,0,.3); --shadow:0 2px 8px rgba(0,0,0,.4); --shadow-lg:0 8px 24px rgba(0,0,0,.5);
  --green:#4ade80; --green-bg:#1a2e1a; --green-border:#2d5a2d;
  --yellow:#fbbf24; --yellow-bg:#2e2408; --yellow-border:#5a4510;
  --red:#ff6b6b; --red-bg:#2e1a1a; --red-border:#5a2d2d; --gray:#9698a6; --gray-bg:#2a2f45;
}
body{ font-family:var(--font); background:var(--bg); color:var(--text); line-height:1.5; }
main{ max-width:1000px; margin:0 auto; padding:24px 28px; }
.page-head{ display:flex; align-items:center; justify-content:space-between; gap:16px; flex-wrap:wrap; margin-bottom:18px; }
.page-head h1{ font-size:22px; font-weight:700; }
.page-head p{ font-size:13px; color:var(--text-secondary); margin-top:2px; }
.recheck{ display:inline-flex; align-items:center; gap:7px; background:var(--primary); color:#fff; border:none;
  border-radius:7px; padding:9px 16px; font-size:13.5px; font-weight:600; cursor:pointer; font-family:inherit; text-decoration:none; }
.recheck:hover{ background:var(--primary-hover); }
.banner{ display:flex; align-items:center; gap:11px; padding:14px 18px; border-radius:var(--radius); margin-bottom:22px;
  font-size:15px; font-weight:600; border:1px solid; }
.banner .dot{ width:11px; height:11px; border-radius:50%; flex-shrink:0; }
.banner.d-ok{ background:var(--green-bg); border-color:var(--green-border); color:var(--green); }
.banner.d-ok .dot{ background:var(--green); }
.banner.d-warn{ background:var(--yellow-bg); border-color:var(--yellow-border); color:var(--yellow); }
.banner.d-warn .dot{ background:var(--yellow); }
.banner.d-fail{ background:var(--red-bg); border-color:var(--red-border); color:var(--red); }
.banner.d-fail .dot{ background:var(--red); }
.group{ margin-bottom:26px; }
.group h2{ font-size:13px; font-weight:700; letter-spacing:.5px; text-transform:uppercase; color:var(--text-secondary);
  margin-bottom:10px; }
.card{ background:var(--surface); border:1px solid var(--border); border-left-width:4px; border-radius:var(--radius);
  padding:14px 18px; margin-bottom:10px; box-shadow:var(--shadow-sm); }
.card.d-ok{ border-left-color:var(--green); }
.card.d-warn{ border-left-color:var(--yellow); }
.card.d-fail{ border-left-color:var(--red); }
.card.d-skip{ border-left-color:var(--gray); }
.card-top{ display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
.card-top .name{ font-size:15px; font-weight:600; flex:1; min-width:200px; }
.badge{ font-size:11.5px; font-weight:700; padding:3px 10px; border-radius:20px; white-space:nowrap; }
.badge.d-ok{ background:var(--green-bg); color:var(--green); border:1px solid var(--green-border); }
.badge.d-warn{ background:var(--yellow-bg); color:var(--yellow); border:1px solid var(--yellow-border); }
.badge.d-fail{ background:var(--red-bg); color:var(--red); border:1px solid var(--red-border); }
.badge.d-skip{ background:var(--gray-bg); color:var(--gray); border:1px solid var(--border); }
.detail{ font-size:13.5px; color:var(--text); margin-top:7px; }
.fix{ font-size:13px; color:var(--text-secondary); margin-top:8px; padding:9px 12px; background:var(--surface-alt);
  border-radius:6px; }
.fix b{ color:var(--text); }
.fix a{ display:inline-block; margin-top:7px; color:var(--primary); font-weight:600; text-decoration:none; font-size:13px; }
.fix a:hover{ text-decoration:underline; }
.foot-note{ font-size:12px; color:var(--text-secondary); margin-top:18px; text-align:center; }
@media (max-width:860px){ main{ padding:18px 16px; } }
</style>
</head>
<body>
`)
	b.WriteString(sidebarMainHTML)
	b.WriteString(`<main>
<div class="page-head">
  <div>
    <h1>Диагностика системы</h1>
    <p>Проверено: ` + html.EscapeString(at.Format("02.01.2006 15:04:05")) + `</p>
  </div>
  <a class="recheck" href="/health">↻ Перепроверить</a>
</div>
`)
	b.WriteString(`<div class="banner ` + bannerCls + `"><span class="dot"></span>` + html.EscapeString(bannerText) + `</div>`)

	for _, g := range groups {
		b.WriteString(`<section class="group"><h2>` + html.EscapeString(g.Title) + `</h2>`)
		for _, c := range g.Checks {
			cls, label := diagBadge(c.Status)
			b.WriteString(`<div class="card ` + cls + `"><div class="card-top">`)
			b.WriteString(`<span class="name">` + html.EscapeString(c.Name) + `</span>`)
			b.WriteString(`<span class="badge ` + cls + `">` + html.EscapeString(label) + `</span></div>`)
			if c.Detail != "" {
				b.WriteString(`<div class="detail">` + html.EscapeString(c.Detail) + `</div>`)
			}
			if c.Fix != "" {
				b.WriteString(`<div class="fix"><b>Как решить:</b> ` + html.EscapeString(c.Fix))
				if c.FixURL != "" {
					b.WriteString(`<br><a href="` + html.EscapeString(c.FixURL) + `">` + html.EscapeString(c.FixLabel) + ` →</a>`)
				}
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</section>`)
	}

	b.WriteString(`<p class="foot-note">Проверки выполняются заново при каждом открытии страницы. Узлы со статусом «Не подключено» в этой сборке не используются.</p>
</main>
</body>
</html>`)
	return b.String()
}
