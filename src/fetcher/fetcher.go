// Package fetcher скачивает тела файлов документов через headless-браузер (chromedp).
//
// Обход WAF (Variti):
//   - Stealth-скрипты маскируют признаки автоматизации
//   - Симуляция человеческого поведения: случайные задержки, движения мыши, скролл
//   - TLS/HTTP-заголовки имитируют реальный Chrome
//   - Один браузер-сессия переиспользуется на весь прогон (WAF-куки сохраняются),
//     это и быстрее, и менее заметно, чем перезапуск браузера на каждый файл
//   - При WAF-бане сессия пересоздаётся, при наличии колбэка — со сменой прокси
//   - Для дата-центровых IP рекомендуется использовать резидентный прокси (PROXY_URL)
package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Типизированные ошибки скачивания — для умного retry.
var (
	// errWAFBlock — доступ заблокирован WAF (403/challenge/HTML вместо файла).
	// На него реагируем сменой прокси и пересозданием сессии.
	errWAFBlock = errors.New("WAF заблокировал доступ")
	// errNoLink — ссылка на файл на странице не найдена (часто — нет тела файла).
	// Агрессивно ретраить бессмысленно.
	errNoLink = errors.New("ссылка на файл не найдена")
)

// stealthJS — расширенная маска автоматизации (anti-detection для Variti).
const stealthJS = `
(function() {
  // 1. webdriver флаг
  Object.defineProperty(navigator, 'webdriver', { get: () => undefined });

  // 2. Chrome runtime (как у реального Chrome)
  window.chrome = { runtime: {
    connect: function() { return { onMessage: { addListener: function(){} }, onDisconnect: { addListener: function(){} }, postMessage: function(){} }; },
    sendMessage: function() {}
  }};

  // 3. Языки
  Object.defineProperty(navigator, 'languages', { get: () => ['ru-RU','ru','en-US','en'] });

  // 4. Плагины (как у обычного Chrome)
  Object.defineProperty(navigator, 'plugins', { get: () => [
    { name: 'Chrome PDF Plugin', description: 'Portable Document Format', filename: 'internal-pdf-viewer' },
    { name: 'Chrome PDF Viewer', description: '', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
    { name: 'Native Client', description: '', filename: 'internal-nacl-plugin' }
  ]});

  // 5. Permissions — всегда prompt
  const originalQuery = window.navigator.permissions.query;
  window.navigator.permissions.query = function(args) {
    if (args.name === 'notifications') {
      return Promise.resolve({ state: Notification.permission });
    }
    return originalQuery.apply(this, arguments);
  };

  // 6. Notification.permission
  Object.defineProperty(Notification, 'permission', { get: () => 'default' });

  // 7. Убираем cdc_ свойства (признак chromedriver)
  for (var key of Object.getOwnPropertyNames(navigator)) {
    if (key.startsWith('cdc_') || key.startsWith('domAutomation')) {
      delete navigator[key];
    }
  }

  // 8. Platform
  Object.defineProperty(navigator, 'platform', { get: () => 'Win32' });
  Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 });
  Object.defineProperty(navigator, 'deviceMemory', { get: () => 8 });

  // 9. Touch support (как на десктопе)
  Object.defineProperty(navigator, 'maxTouchPoints', { get: () => 0 });

  // 10. Connection
  Object.defineProperty(navigator, 'connection', { get: () => ({
    effectiveType: '4g', rtt: 50, downlink: 10, saveData: false
  })});

  // 11. Webdriver property in document
  delete document.__proto__.webdriver;

  // 12. Remove selenium markers
  for (var k of ['_selenium', 'callSelenium', '_Selenium_IDE_Recorder']) {
    if (window[k]) delete window[k];
  }

  // 13. Fake chrome.app
  window.chrome.app = {
    isInstalled: false,
    InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
    RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' }
  };

  // 14. Fake Intl API locale
  Object.defineProperty(Intl, 'DateTimeFormat', {
    value: new Proxy(Intl.DateTimeFormat, {
      construct: (target, args) => new target('ru-RU', args[1])
    })
  });
})();
`

// humanBehaviorJS — симуляция человеческого поведения на странице.
const humanBehaviorJS = `(function(){
  // Случайные микро-движения мыши (имитация чтения)
  function randomMouse() {
    var x = Math.floor(Math.random() * window.innerWidth);
    var y = Math.floor(Math.random() * window.innerHeight);
    var evt = new MouseEvent('mousemove', {clientX: x, clientY: y, bubbles: true});
    document.dispatchEvent(evt);
  }
  // 2-5 случайных движений с паузами
  var moves = 2 + Math.floor(Math.random() * 4);
  for (var i = 0; i < moves; i++) {
    setTimeout(randomMouse, 300 + Math.random() * 800);
  }
  // Лёгкий скролл вниз-вверх (имитация просмотра)
  setTimeout(function(){
    window.scrollBy(0, Math.floor(window.innerHeight * 0.3 + Math.random() * 200));
  }, 1000 + Math.random() * 500);
  setTimeout(function(){
    window.scrollBy(0, -Math.floor(window.innerHeight * 0.15));
  }, 2500 + Math.random() * 500);
})()`

// jsFindFile — выражение, возвращающее первую ссылку на файл документа.
const jsFindFile = `(function(){
  var a=Array.from(document.querySelectorAll('a[href]')).map(function(x){return x.href;});
  var re=/cfs-file|cfs-filesystemfile|\.pdf|\.docx?|\.xlsx?|\.rtf|\.pptx?|download/i;
  for(var i=0;i<a.length;i++){if(re.test(a[i]))return a[i];}
  return "";
})()`

// wafDetector — проверяет, не заблокировал ли WAF доступ (Variti challenge page).
const wafDetector = `(function(){
  var body = document.body ? (document.body.innerText || '') : '';
  var title = document.title || '';
  // Variti challenge page markers
  if (body.toLowerCase().indexOf('variti') !== -1 ||
      body.toLowerCase().indexOf('ddos') !== -1 ||
      body.toLowerCase().indexOf('проверка') !== -1 ||
      title.toLowerCase().indexOf('checking') !== -1 ||
      body.toLowerCase().indexOf('cloudflare') !== -1 ||
      body.toLowerCase().indexOf('подтвердите') !== -1) {
    return 'challenge';
  }
  // 403/503 pages. ServicePipe (WAF dochub) отдаёт короткие текстовые блоки:
  //   "Forbidden\nTransaction ID: …"  и
  //   "Forbidden\n…IP: …\nIf you are not a bot, please copy the report…"
  // В них НЕТ числа "403", поэтому старая проверка их пропускала как ok →
  // jsFindFile ничего не находил → errNoLink (и смена прокси не срабатывала).
  var low = body.toLowerCase();
  if (low.indexOf('if you are not a bot') !== -1 ||
      low.indexOf('transaction id') !== -1 ||
      (low.indexOf('forbidden') !== -1 && body.length < 600) ||
      (body.indexOf('403') !== -1 && low.indexOf('forbidden') !== -1)) {
    return 'forbidden';
  }
  // Success
  return 'ok';
})()`

// jsCollectDocs — собирает ссылки на документы (/m/docs/) с заголовками.
const jsCollectDocs = `Array.from(document.querySelectorAll('a[href*="/m/docs/"]')).map(function(a){return {h:a.href,t:(a.textContent||'').replace(/\s+/g,' ').trim()};})`

// jsCollectLinks — собирает все ссылки на странице (для обхода сайта).
const jsCollectLinks = `Array.from(document.querySelectorAll('a[href]')).map(function(a){return a.href;})`

// locRe — извлекает URL из <loc>…</loc> в sitemap.xml.
var locRe = regexp.MustCompile(`<loc>\s*([^<\s]+)\s*</loc>`)

// catPageRe — выделяет слаг категории из URL вида …/p/{slug}.aspx.
var catPageRe = regexp.MustCompile(`/p/([a-z0-9_]+)\.aspx`)

// Fetcher скачивает файлы через headless-Chrome с симуляцией человека.
type Fetcher struct {
	ChromePath  string
	ProxyURL    string
	GetProxyURL func() string // получить активный прокси (динамическое переключение)
	// OnWAFBlocked вызывается при WAF-бане: должен переключить прокси и вернуть
	// новый URL прокси ("" — без прокси). nil — смена прокси отключена.
	OnWAFBlocked func() string

	Wait time.Duration // базовая пауза между файлами

	// Профиль «человеческого» темпа массового прогона.
	BatchSize    int           // файлов до длинного перерыва (0 — без перерывов)
	BreakMin     time.Duration // мин. длительность длинного перерыва
	BreakMax     time.Duration // макс. длительность длинного перерыва
	LongPausePct int           // вероятность (0-100) длинной паузы между файлами

	// Cookie — сессионная кука dochub (spid + AuthorizationCookie), скопированная
	// из реального браузера, прошедшего WAF. С ней простой HTTP-GET качает тела
	// файлов БЕЗ браузера и БЕЗ прокси (см. http_download.go). Периодически
	// протухает — обновляется в админке. "" — куки нет (HTTP-скачивание выкл.).
	Cookie string

	// SkipURL (опц.) — предикат для возобновляемого прогона: вернёт true, если
	// файл по этой download-ссылке уже скачан (есть в реестре с локальным файлом).
	// CollectViaCookie тогда не качает его повторно. Позволяет дробить большой
	// прогон на части и продолжать после обновления протухшей куки.
	SkipURL func(url string) bool

	Rng  *rand.Rand
	HTTP *http.Client
}

// New создаёт загрузчик. chromePath="" — автоопределение Chrome/Edge.
// getProxyURL — функция для получения URL активного прокси (может быть nil).
//
// Отсутствие Chrome НЕ является ошибкой: HTTP-каталогизация
// (EnumerateCategoriesHTTP) работает без браузера. Chrome нужен только для
// headless-операций (скачивание тел файлов за WAF, fallback-обход) — они
// сами проверят его наличие через requireChrome().
func New(chromePath, proxyURL string, wait time.Duration, getProxyURL func() string) (*Fetcher, error) {
	if chromePath == "" {
		chromePath = detectChrome()
	}

	// Определяем начальный прокси
	currentProxyURL := proxyURL
	if getProxyURL != nil {
		if u := getProxyURL(); u != "" {
			currentProxyURL = u
		}
	}

	f := &Fetcher{
		ChromePath:  chromePath,
		ProxyURL:    currentProxyURL,
		GetProxyURL: getProxyURL,
		Wait:        wait,
		// Консервативные дефолты, чтобы без настройки прогон выглядел естественно.
		BatchSize:    30,
		BreakMin:     60 * time.Second,
		BreakMax:     180 * time.Second,
		LongPausePct: 15,
		Rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	f.applyProxy(currentProxyURL)
	return f, nil
}

// applyProxy перенастраивает HTTP-клиент (и запоминает URL для следующей сессии).
func (f *Fetcher) applyProxy(proxyURL string) {
	f.ProxyURL = proxyURL
	tr := &http.Transport{}
	if proxyURL != "" {
		if pu, err := url.Parse(proxyURL); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	f.HTTP = &http.Client{Timeout: 90 * time.Second, Transport: tr}
}

func detectChrome() string {
	for _, p := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// execOpts формирует опции запуска Chrome (stealth + опц. прокси).
func (f *Fetcher) execOpts() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(f.ChromePath),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("window-size", "1920,1080"),
		chromedp.Flag("lang", "ru-RU"),
		chromedp.Flag("accept-lang", "ru-RU,ru,en-US,en"),
		chromedp.UserAgent(userAgent),
		// Убираем автоматизацию на уровне протокола DevTools
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("enable-automation", false),
		// Реалистичные параметры Chrome
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		// Отключаем WebRTC (может раскрыть реальный IP)
		chromedp.Flag("disable-webrtc", true),
		// Убираем признаки автоматизации в TLS-рукопожатии
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-sync", true),
	}
	if f.ProxyURL != "" {
		// Chrome --proxy-server не принимает логин/пароль в URL — передаём чистый
		// scheme://host:port; авторизация резидентного прокси выполняется через
		// CDP Fetch.authRequired в OpenSession.
		clean, _, _ := parseProxy(f.ProxyURL)
		opts = append(opts, chromedp.ProxyServer(clean))
	}
	return opts
}

// parseProxy разбирает URL прокси на «чистый» URL (без userinfo) и креды.
func parseProxy(raw string) (clean, user, pass string) {
	if strings.TrimSpace(raw) == "" {
		return "", "", ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw, "", ""
	}
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
		u.User = nil
	}
	return u.String(), user, pass
}

// setupProxyAuth вешает CDP-обработчик аутентификации на прокси: на запрос
// учётных данных отвечает логином/паролем, остальные приостановленные запросы
// пропускает. Нужен для резидентных прокси с авторизацией (user:pass).
func setupProxyAuth(ctx context.Context, user, pass string) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
					Response: fetch.AuthChallengeResponseResponseProvideCredentials,
					Username: user,
					Password: pass,
				}))
			}()
		case *fetch.EventRequestPaused:
			go func() {
				_ = chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID))
			}()
		}
	})
}

// humanDelay — случайная задержка в диапазоне [minMs, maxMs].
func (f *Fetcher) humanDelay(minMs, maxMs int) time.Duration {
	if maxMs <= minMs {
		return time.Duration(minMs) * time.Millisecond
	}
	d := f.Rng.Intn(maxMs-minMs) + minMs
	return time.Duration(d) * time.Millisecond
}

// betweenFilesDelay — реалистичная пауза между скачиваниями файлов.
// В основном короткая (база ±50%), изредка — длинная (человек отвлёкся).
func (f *Fetcher) betweenFilesDelay() time.Duration {
	base := f.Wait
	if base <= 0 {
		base = 5 * time.Second
	}
	// джиттер ±50% от базы
	jitter := time.Duration(f.Rng.Int63n(int64(base))) - base/2
	d := base + jitter
	if f.LongPausePct > 0 && f.Rng.Intn(100) < f.LongPausePct {
		d += time.Duration(20+f.Rng.Intn(40)) * time.Second
	}
	if d < time.Second {
		d = time.Second
	}
	return d
}

// humanMouseMove — эмулирует плавное движение мыши к координатам.
func humanMouseMove(x, y int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// Несколько промежуточных точек для плавности
		steps := 3 + rand.Intn(3)
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			px := int(float64(x) * t)
			py := int(float64(y) * t)
			input.DispatchMouseEvent(input.MouseMoved, float64(px), float64(py)).Do(ctx)
			time.Sleep(time.Duration(20+rand.Intn(40)) * time.Millisecond)
		}
		return nil
	})
}

// humanClick — клик с предварительным наведением мыши.
func humanClick(sel string) chromedp.Tasks {
	return chromedp.Tasks{
		humanMouseMove(200+rand.Intn(800), 200+rand.Intn(400)),
		chromedp.Click(sel, chromedp.ByQuery),
		chromedp.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond),
	}
}

// humanScroll — имитация скролла страницы человеком.
func humanScroll() chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond),
		chromedp.Evaluate(`window.scrollBy(0, Math.floor(window.innerHeight * (0.3 + Math.random() * 0.4)));`, nil),
		chromedp.Sleep(time.Duration(300+rand.Intn(800)) * time.Millisecond),
		chromedp.Evaluate(`window.scrollBy(0, -Math.floor(window.innerHeight * (0.1 + Math.random() * 0.2)));`, nil),
		chromedp.Sleep(time.Duration(200+rand.Intn(400)) * time.Millisecond),
	}
}

// simulateHuman — полный набор действий, имитирующих реального пользователя.
func (f *Fetcher) simulateHuman() chromedp.Tasks {
	return chromedp.Tasks{
		// 1. Ждём загрузки страницы
		chromedp.Sleep(f.humanDelay(1500, 3000)),
		// 2. Скроллим как человек (читает)
		humanScroll(),
		// 3. JS-симуляция микро-движений мыши
		chromedp.Evaluate(humanBehaviorJS, nil),
		// 4. Дополнительная пауза (чтение контента)
		chromedp.Sleep(f.humanDelay(1000, 2500)),
		// 5. Ещё один скролл
		chromedp.Evaluate(`window.scrollBy(0, Math.floor(window.innerHeight * 0.5));`, nil),
		chromedp.Sleep(f.humanDelay(800, 1500)),
		// 6. Возврат наверх (как при поиске ссылки)
		chromedp.Evaluate(`window.scrollTo({top: 0, behavior: 'smooth'});`, nil),
		chromedp.Sleep(f.humanDelay(500, 1000)),
	}
}

// simulateHumanLight — облегчённая симуляция (сессия уже «прогрета»).
func (f *Fetcher) simulateHumanLight() chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Sleep(f.humanDelay(800, 1800)),
		humanScroll(),
		chromedp.Evaluate(humanBehaviorJS, nil),
		chromedp.Sleep(f.humanDelay(500, 1200)),
	}
}

// checkWAF — проверяет, не заблокировал ли WAF доступ.
func checkWAF(ctx context.Context) (string, error) {
	var status string
	err := chromedp.Evaluate(wafDetector, &status).Do(ctx)
	if err != nil {
		return "error", err
	}
	return status, nil
}

// waitForWAF — ждёт пока WAF разрешит доступ (max 30 секунд).
func (f *Fetcher) waitForWAF(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, err := checkWAF(ctx)
		if err != nil {
			return err
		}
		if status == "ok" {
			return nil
		}
		if status == "forbidden" {
			return fmt.Errorf("%w (403 Forbidden)", errWAFBlock)
		}
		// challenge — ждём
		time.Sleep(f.humanDelay(2000, 4000))
	}
	return fmt.Errorf("%w: challenge не пройден за 30 секунд", errWAFBlock)
}

// Session — переиспользуемая браузер-сессия (один Chrome на весь прогон).
// WAF-куки сохраняются в контексте chromedp, пока сессия открыта.
type Session struct {
	f       *Fetcher
	bctx    context.Context
	cancelA context.CancelFunc
	cancelB context.CancelFunc
}

// requireChrome возвращает ошибку, если путь к Chrome не задан/не найден.
// Вызывается headless-операциями (HTTP-методам Chrome не нужен).
func (f *Fetcher) requireChrome() error {
	if f.ChromePath == "" {
		return fmt.Errorf("не найден Chrome; задайте CHROME_PATH (нужен для headless-операций)")
	}
	return nil
}

// OpenSession запускает браузер, ставит stealth на все страницы и «прогревается»
// на главной dochub.sk.ru (как реальный пользователь перед навигацией).
func (f *Fetcher) OpenSession(ctx context.Context) (*Session, error) {
	if err := f.requireChrome(); err != nil {
		return nil, err
	}
	allocCtx, cancelA := chromedp.NewExecAllocator(ctx, f.execOpts()...)
	bctx, cancelB := chromedp.NewContext(allocCtx)
	s := &Session{f: f, bctx: bctx, cancelA: cancelA, cancelB: cancelB}

	// Аутентификация на резидентном прокси (если задан user:pass): вешаем
	// обработчик до Enable и до навигации, иначе прокси вернёт 407.
	if _, user, pass := parseProxy(f.ProxyURL); user != "" {
		setupProxyAuth(bctx, user, pass)
		if err := chromedp.Run(bctx, fetch.Enable().WithHandleAuthRequests(true)); err != nil {
			s.Close()
			return nil, fmt.Errorf("включение прокси-аутентификации: %w", err)
		}
	}

	// Stealth на каждую новую страницу.
	if err := chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
		return err
	})); err != nil {
		s.Close()
		return nil, fmt.Errorf("старт браузера: %w", err)
	}

	// Прогрев: заходим на главную как человек.
	_ = chromedp.Run(bctx,
		chromedp.Navigate("https://dochub.sk.ru/"),
		chromedp.Sleep(f.humanDelay(2000, 4000)),
		humanScroll(),
		chromedp.Sleep(f.humanDelay(1000, 2000)),
	)
	return s, nil
}

// Close завершает браузер-сессию.
func (s *Session) Close() {
	if s.cancelB != nil {
		s.cancelB()
	}
	if s.cancelA != nil {
		s.cancelA()
	}
}

// fetchOne открывает страницу-просмотрщик в текущей сессии, находит ссылку на файл
// и скачивает его (с куками, выставленными WAF) в outDir. Возвращает путь и хэш.
func (s *Session) fetchOne(ctx context.Context, viewerURL, outDir string) (string, string, error) {
	f := s.f
	var fileURL string
	var cookies []*network.Cookie

	tasks := chromedp.Tasks{
		chromedp.Navigate(viewerURL),
		chromedp.ActionFunc(func(ctx context.Context) error { return f.waitForWAF(ctx) }),
		f.simulateHumanLight(),
		chromedp.Evaluate(jsFindFile, &fileURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	}
	if err := chromedp.Run(s.bctx, tasks); err != nil {
		return "", "", classifyRunErr(err)
	}
	if strings.TrimSpace(fileURL) == "" {
		return "", "", errNoLink
	}

	// Скачиваем файл через HTTP с WAF-куками
	data, err := f.download(ctx, fileURL, viewerURL, cookies)
	if err != nil {
		return "", "", err
	}
	if err := validateFileBytes(data, fileURL); err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", "", err
	}
	name := safeName(fileURL)
	localPath := filepath.Join(outDir, name)
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	return localPath, hex.EncodeToString(sum[:]), nil
}

// classifyRunErr приводит ошибку chromedp.Run к нашим типам (WAF/нет ссылки/иное).
func classifyRunErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errWAFBlock) || errors.Is(err, errNoLink) {
		return err
	}
	// Сетевые/навигационные ошибки считаем временными — обернём как есть.
	return fmt.Errorf("браузер: %w", err)
}

// validateFileBytes проверяет, что скачано тело файла, а не HTML-челлендж WAF.
func validateFileBytes(data []byte, fileURL string) error {
	if len(data) < 100 {
		return fmt.Errorf("%w: подозрительно маленький ответ (%d байт)", errWAFBlock, len(data))
	}
	head := strings.ToLower(string(data[:min(512, len(data))]))
	if strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html") ||
		strings.Contains(head, "variti") || strings.Contains(head, "ddos") ||
		strings.Contains(head, "проверка") {
		return fmt.Errorf("%w: получен HTML вместо файла", errWAFBlock)
	}

	ext := strings.ToLower(path.Ext(safeName(fileURL)))
	hasPrefix := func(sig string) bool { return strings.HasPrefix(string(data), sig) }
	switch ext {
	case ".pdf":
		if !hasPrefix("%PDF") {
			return fmt.Errorf("%w: не похоже на PDF", errWAFBlock)
		}
	case ".docx", ".xlsx", ".pptx", ".zip":
		if !hasPrefix("PK\x03\x04") && !hasPrefix("PK\x05\x06") {
			return fmt.Errorf("%w: не похоже на ZIP-контейнер (%s)", errWAFBlock, ext)
		}
	case ".doc", ".xls", ".ppt":
		if !hasPrefix("\xD0\xCF\x11\xE0") {
			return fmt.Errorf("%w: не похоже на OLE-документ (%s)", errWAFBlock, ext)
		}
	case ".rtf":
		if !hasPrefix("{\\rtf") {
			return fmt.Errorf("%w: не похоже на RTF", errWAFBlock)
		}
	}
	// Неизвестное/прочее расширение — HTML-проверка выше уже отсекла мусор.
	return nil
}

func (f *Fetcher) download(ctx context.Context, fileURL, referer string, cookies []*network.Cookie) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/pdf,application/msword,application/vnd.openxmlformats-officedocument.*,*/*")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	if c := cookieHeader(cookies); c != "" {
		req.Header.Set("Cookie", c)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("%w: HTTP %d", errWAFBlock, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("скачивание файла: статус %d", resp.StatusCode)
	}
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	return buf, nil
}

// CategorySpec — категория для перечисления (слаг страницы + читаемое имя).
type CategorySpec struct {
	Slug string
	Name string
}

// CatalogItem — документ, найденный при перечислении категории/обходе сайта.
type CatalogItem struct {
	Title    string
	Link     string
	Category string
}

// rawLink — ссылка с заголовком, собранная со страницы.
type rawLink struct {
	H string `json:"h"`
	T string `json:"t"`
}

// crawlPage навигирует на страницу, проходит WAF, прокручивает (lazy-load) и
// собирает ссылки на документы (/m/docs/) и все внутренние ссылки для очереди.
func (s *Session) crawlPage(ctx context.Context, pageURL string) ([]rawLink, []string, error) {
	f := s.f
	var docs []rawLink
	var navs []string
	tasks := chromedp.Tasks{
		chromedp.Navigate(pageURL),
		chromedp.ActionFunc(func(ctx context.Context) error { return f.waitForWAF(ctx) }),
		f.simulateHuman(),
		// Ждём пока superlist подгрузит элементы
		chromedp.Sleep(f.humanDelay(2000, 4000)),
		// Скроллим до конца (trigger lazy load)
		chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
		chromedp.Sleep(f.humanDelay(2000, 3000)),
		chromedp.Evaluate(`window.scrollTo(0, 0);`, nil),
		chromedp.Sleep(f.humanDelay(500, 1000)),
		chromedp.Evaluate(jsCollectDocs, &docs),
		chromedp.Evaluate(jsCollectLinks, &navs),
	}
	if err := chromedp.Run(s.bctx, tasks); err != nil {
		return nil, nil, classifyRunErr(err)
	}
	return docs, navs, nil
}

// EnumerateCategories обходит страницы 8 категорий и возвращает ссылки на документы.
// Полная симуляция человека, один браузер на весь обход.
func (f *Fetcher) EnumerateCategories(ctx context.Context, baseURL string, cats []CategorySpec) ([]CatalogItem, error) {
	sess, err := f.OpenSession(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	base := strings.TrimSuffix(baseURL, "/")
	var out []CatalogItem
	seen := map[string]bool{}

	for i, c := range cats {
		pageURL := base + "/p/" + c.Slug + ".aspx"
		fmt.Printf("  [browser] категория %d/%d: %s\n", i+1, len(cats), c.Name)

		docs, _, err := sess.crawlPage(ctx, pageURL)
		if err != nil {
			fmt.Printf("  ! категория %s: %v\n", c.Slug, err)
			continue
		}
		for _, r := range docs {
			if r.H == "" || seen[r.H] {
				continue
			}
			seen[r.H] = true
			out = append(out, CatalogItem{Title: r.T, Link: r.H, Category: c.Name})
		}
		fmt.Printf("    найдено: %d документов (всего: %d)\n", len(docs), len(out))

		if i < len(cats)-1 {
			pause := f.humanDelay(3000, 7000)
			fmt.Printf("    пауза %v...\n", pause)
			time.Sleep(pause)
		}
	}
	return out, nil
}

// EnumerateSite рекурсивно обходит весь раздел документов сайта (BFS через браузер,
// чтобы не ловить 403 на WAF). Стартует с базовой страницы, страниц категорий и
// URL из sitemap.xml; идёт по внутренним ссылкам в пределах /foundation/documents/.
// maxPages<=0 — взять разумный дефолт.
func (f *Fetcher) EnumerateSite(ctx context.Context, baseURL string, cats []CategorySpec, maxPages int) ([]CatalogItem, error) {
	if maxPages <= 0 {
		maxPages = 800
	}
	sess, err := f.OpenSession(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("базовый URL: %w", err)
	}
	slugName := map[string]string{}
	for _, c := range cats {
		slugName[c.Slug] = c.Name
	}

	seenPage := map[string]bool{}
	seenDoc := map[string]bool{}
	var out []CatalogItem

	// Сид-очередь: база + страницы категорий + sitemap.
	root := strings.TrimSuffix(baseURL, "/")
	queue := []string{root}
	for _, c := range cats {
		queue = append(queue, root+"/p/"+c.Slug+".aspx")
	}
	queue = append(queue, sess.fetchSitemapURLs(ctx, base)...)

	for len(queue) > 0 && len(seenPage) < maxPages {
		pageURL := queue[0]
		queue = queue[1:]
		if seenPage[pageURL] {
			continue
		}
		seenPage[pageURL] = true

		pageCat := categoryFromURL(pageURL, slugName)
		docs, navs, err := sess.crawlPage(ctx, pageURL)
		if err != nil {
			fmt.Printf("  ! %s: %v\n", pageURL, err)
			continue
		}
		for _, r := range docs {
			if r.H == "" || seenDoc[r.H] {
				continue
			}
			seenDoc[r.H] = true
			out = append(out, CatalogItem{Title: r.T, Link: r.H, Category: pageCat})
		}
		// Внутренние ссылки раздела документов → в очередь.
		for _, l := range navs {
			u, err := url.Parse(l)
			if err != nil || u.Host != base.Host {
				continue
			}
			if !strings.Contains(u.Path, "/foundation/documents/") {
				continue
			}
			clean := u.Scheme + "://" + u.Host + u.Path
			if !seenPage[clean] {
				queue = append(queue, clean)
			}
		}
		fmt.Printf("  [crawl] страниц %d/%d, документов: %d\n", len(seenPage), maxPages, len(out))
		time.Sleep(f.humanDelay(2000, 5000))
	}
	return out, nil
}

// fetchSitemapURLs скачивает sitemap.xml через браузер (same-origin fetch несёт
// WAF-куки) и возвращает URL страниц раздела документов.
func (s *Session) fetchSitemapURLs(ctx context.Context, base *url.URL) []string {
	sitemapURL := base.Scheme + "://" + base.Host + "/sitemap.xml"
	js := `(async()=>{try{const r=await fetch(` + jsString(sitemapURL) + `);if(!r.ok)return "";return await r.text();}catch(e){return "";}})()`
	var body string
	err := chromedp.Run(s.bctx,
		chromedp.Evaluate(js, &body, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil || body == "" {
		return nil
	}
	var urls []string
	for _, m := range locRe.FindAllStringSubmatch(body, -1) {
		u, err := url.Parse(strings.TrimSpace(m[1]))
		if err != nil || u.Host != base.Host {
			continue
		}
		if strings.Contains(u.Path, "/foundation/documents/") {
			urls = append(urls, u.Scheme+"://"+u.Host+u.Path)
		}
	}
	return urls
}

// categoryFromURL определяет читаемое имя категории по URL вида …/p/{slug}.aspx.
func categoryFromURL(pageURL string, slugName map[string]string) string {
	m := catPageRe.FindStringSubmatch(pageURL)
	if m == nil {
		return ""
	}
	return slugName[m[1]]
}

// jsString безопасно оборачивает строку в JS-литерал.
func jsString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// EnrichMissing скачивает файлы для документов без локального файла в ОДНОЙ
// браузер-сессии (переиспользуя WAF-куки). Если документ «действует» и задан
// index — сразу индексирует. При WAF-бане пытается сменить прокси и пересоздать
// сессию. Между файлами — «человеческие» паузы, периодически длинный перерыв.
func (f *Fetcher) EnrichMissing(ctx context.Context, st store.Store, outRoot string, limit int, index func(ctx context.Context, id string) error) (int, []string) {
	docs, err := st.List(ctx, store.Filter{})
	if err != nil {
		return 0, []string{err.Error()}
	}

	// Отбираем документы без локального файла.
	var pending []model.Document
	for _, d := range docs {
		if d.LocalPath != "" || d.Status == model.StatusArchived || d.Status == model.StatusRejected {
			continue
		}
		pending = append(pending, d)
	}
	// Перемешиваем — порядок не должен быть предсказуемым паттерном.
	f.Rng.Shuffle(len(pending), func(i, j int) { pending[i], pending[j] = pending[j], pending[i] })

	sess, err := f.OpenSession(ctx)
	if err != nil {
		return 0, []string{fmt.Sprintf("старт браузера: %v", err)}
	}
	defer func() { sess.Close() }()

	var done int
	var errs []string
	sinceBreak := 0

	for _, d := range pending {
		if limit > 0 && done >= limit {
			break
		}
		outDir := filepath.Join(outRoot, "На_проверке", "Загружено")

		localPath, hash, ferr := f.fetchWithRetry(ctx, &sess, d.SourceURL, outDir)
		if ferr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", d.ID, ferr))
			continue
		}
		d.LocalPath = localPath
		d.FileHash = hash
		if err := st.Upsert(ctx, d); err != nil {
			errs = append(errs, fmt.Sprintf("%s: сохранение: %v", d.ID, err))
			continue
		}
		done++
		sinceBreak++
		if d.Status == model.StatusActive && index != nil {
			if err := index(ctx, d.ID); err != nil {
				errs = append(errs, fmt.Sprintf("%s: индексация: %v", d.ID, err))
			}
		}

		// Темп: длинный перерыв каждые BatchSize файлов, иначе обычная пауза.
		if f.BatchSize > 0 && sinceBreak >= f.BatchSize {
			sinceBreak = 0
			f.longBreak(ctx, sess)
		} else {
			select {
			case <-time.After(f.betweenFilesDelay()):
			case <-ctx.Done():
				return done, errs
			}
		}
	}
	return done, errs
}

// fetchWithRetry скачивает один файл с умным retry: временные ошибки — backoff;
// WAF-бан — смена прокси и пересоздание сессии; «нет ссылки» — 1 ретрай и выход.
// sess может быть пересоздана, поэтому передаётся по указателю.
func (f *Fetcher) fetchWithRetry(ctx context.Context, sess **Session, viewerURL, outDir string) (string, string, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		localPath, hash, err := (*sess).fetchOne(ctx, viewerURL, outDir)
		if err == nil {
			return localPath, hash, nil
		}
		lastErr = err

		switch {
		case errors.Is(err, errNoLink):
			if attempt >= 2 {
				return "", "", err
			}
			time.Sleep(f.humanDelay(1000, 2500))

		case errors.Is(err, errWAFBlock):
			// Пытаемся сменить прокси и пересоздать сессию.
			if f.OnWAFBlocked != nil {
				newURL := f.OnWAFBlocked()
				f.applyProxy(newURL)
				fmt.Printf("  [fetch] WAF-бан, смена прокси → %q\n", maskProxy(newURL))
			} else {
				fmt.Printf("  [fetch] WAF-бан (без смены прокси), пересоздаю сессию\n")
			}
			(*sess).Close()
			ns, oerr := f.OpenSession(ctx)
			if oerr != nil {
				return "", "", fmt.Errorf("пересоздание сессии: %w", oerr)
			}
			*sess = ns
			time.Sleep(time.Duration(attempt*attempt) * time.Second)

		default:
			// Временная ошибка — backoff и повтор в той же сессии.
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}
	}
	return "", "", lastErr
}

// longBreak — длинный «человеческий» перерыв с лёгкой активностью на главной.
func (f *Fetcher) longBreak(ctx context.Context, sess *Session) {
	minD, maxD := f.BreakMin, f.BreakMax
	if maxD <= minD {
		maxD = minD + 60*time.Second
	}
	d := minD + time.Duration(f.Rng.Int63n(int64(maxD-minD)))
	fmt.Printf("  [fetch] длинный перерыв %v...\n", d.Round(time.Second))

	// Имитируем, что пользователь «жив»: зашёл на главную, поскроллил.
	_ = chromedp.Run(sess.bctx,
		chromedp.Navigate("https://dochub.sk.ru/"),
		chromedp.Sleep(f.humanDelay(1500, 3000)),
		humanScroll(),
	)
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// maskProxy скрывает учётные данные прокси в логах.
func maskProxy(proxyURL string) string {
	if proxyURL == "" {
		return "(прямое соединение)"
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return "(proxy)"
	}
	if u.User != nil {
		u.User = url.User("***")
	}
	return u.String()
}

func cookieHeader(cookies []*network.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func safeName(fileURL string) string {
	u, err := url.Parse(fileURL)
	if err != nil {
		return "document"
	}
	name, _ := url.QueryUnescape(path.Base(u.Path))
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." {
		return "document"
	}
	return name
}
