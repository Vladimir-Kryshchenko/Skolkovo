// Package fetcher скачивает тела файлов документов через headless-браузер (chromedp).
//
// Обход WAF (Variti):
// - Stealth-скрипты маскируют признаки автоматизации
// - Симуляция человеческого поведения: случайные задержки, движения мыши, скролл
// - TLS/HTTP-заголовки имитируют реальный Chrome
// - Для дата-центровых IP рекомендуется использовать резидентный прокси (PROXY_URL)
package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// stealthJS — расширенная маска автоматизации (anti-detection для Variti).
const stealthJS = `
(function() {
  // 1. webdriver флаг
  Object.defineProperty(navigator, 'webdriver', { get: () => undefined });

  // 2. Chrome runtime
  window.chrome = { runtime: {} };

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

// Fetcher скачивает файлы через headless-Chrome с симуляцией человека.
type Fetcher struct {
	ChromePath string
	ProxyURL   string
	Wait       time.Duration
	Rng        *rand.Rand
	HTTP       *http.Client
}

// New создаёт загрузчик. chromePath="" — автоопределение Chrome/Edge.
func New(chromePath, proxyURL string, wait time.Duration) (*Fetcher, error) {
	if chromePath == "" {
		chromePath = detectChrome()
	}
	if chromePath == "" {
		return nil, fmt.Errorf("не найден Chrome; задайте CHROME_PATH")
	}
	tr := &http.Transport{}
	if proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("PROXY_URL: %w", err)
		}
		tr.Proxy = http.ProxyURL(pu)
	}
	return &Fetcher{
		ChromePath: chromePath,
		ProxyURL:   proxyURL,
		Wait:       wait,
		Rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		HTTP:       &http.Client{Timeout: 90 * time.Second, Transport: tr},
	}, nil
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
		// Эмуляция реального браузера — отключаем flags, которые выдают автоматизацию
		chromedp.Flag("enable-automation", false),
	}
	if f.ProxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(f.ProxyURL))
	}
	return opts
}

// humanDelay — случайная задержка в диапазоне [minMs, maxMs].
func (f *Fetcher) humanDelay(minMs, maxMs int) time.Duration {
	d := f.Rng.Intn(maxMs-minMs) + minMs
	return time.Duration(d) * time.Millisecond
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

// FetchToDir открывает страницу-просмотрщик в браузере, находит ссылку на файл
// и скачивает его (с куками, выставленными WAF) в outDir. Возвращает путь и хэш.
func (f *Fetcher) FetchToDir(ctx context.Context, viewerURL, outDir string) (string, string, error) {
	allocCtx, cancelA := chromedp.NewExecAllocator(ctx, f.execOpts()...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(allocCtx)
	defer cancelB()

	var fileURL string
	var cookies []*network.Cookie

	tasks := chromedp.Tasks{
		// Stealth на каждую страницу
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
			return err
		}),
		// Навигация
		chromedp.Navigate(viewerURL),
		// Симуляция человека
		f.simulateHuman(),
		// Поиск ссылки на файл
		chromedp.Evaluate(jsFindFile, &fileURL),
		// Забираем cookies от WAF
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	}

	err := chromedp.Run(bctx, tasks)
	if err != nil {
		return "", "", fmt.Errorf("браузер: %w", err)
	}
	if strings.TrimSpace(fileURL) == "" {
		return "", "", fmt.Errorf("ссылка на файл не найдена (возможно, страница заблокирована WAF)")
	}

	data, err := f.download(ctx, fileURL, viewerURL, cookies)
	if err != nil {
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

// CatalogItem — документ, найденный при перечислении категории.
type CatalogItem struct {
	Title    string
	Link     string
	Category string
}

// EnumerateCategories рендерит страницы категорий в браузере (виджет superlist
// подгружает полный список JS-ом) и возвращает все ссылки на документы.
//
// Полная симуляция человеческого поведения для обхода WAF.
func (f *Fetcher) EnumerateCategories(ctx context.Context, baseURL string, cats []CategorySpec) ([]CatalogItem, error) {
	allocCtx, cancelA := chromedp.NewExecAllocator(ctx, f.execOpts()...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(allocCtx)
	defer cancelB()

	// «Прогрев» браузера + stealth на все документы.
	if err := chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("старт браузера: %w", err)
	}

	// Прогревочная страница (заходим на главную dochub как человек)
	fmt.Println("  [browser] прогрев: заходим на dochub.sk.ru...")
	chromedp.Run(bctx,
		chromedp.Navigate("https://dochub.sk.ru/"),
		chromedp.Sleep(f.humanDelay(2000, 4000)),
		humanScroll(),
		chromedp.Sleep(f.humanDelay(1000, 2000)),
	)

	base := strings.TrimSuffix(baseURL, "/")
	var out []CatalogItem
	seen := map[string]bool{}

	for i, c := range cats {
		pageURL := base + "/p/" + c.Slug + ".aspx"
		fmt.Printf("  [browser] категория %d/%d: %s\n", i+1, len(cats), c.Name)

		var raw []struct {
			H string `json:"h"`
			T string `json:"t"`
		}

		tasks := chromedp.Tasks{
			chromedp.Navigate(pageURL),
			// Симуляция человека на странице категории
			f.simulateHuman(),
			// Ждём пока superlist подгрузит все элементы
			chromedp.Sleep(f.humanDelay(2000, 4000)),
			// Скроллим до конца списка (trigger lazy load)
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(f.humanDelay(2000, 3000)),
			// Скроллим обратно
			chromedp.Evaluate(`window.scrollTo(0, 0);`, nil),
			chromedp.Sleep(f.humanDelay(500, 1000)),
			// Собираем все ссылки
			chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href*="/m/docs/"]')).map(function(a){return {h:a.href,t:(a.textContent||'').replace(/\s+/g,' ').trim()};})`, &raw),
		}

		err := chromedp.Run(bctx, tasks)
		if err != nil {
			fmt.Printf("  ! категория %s: %v\n", c.Slug, err)
			continue
		}

		for _, r := range raw {
			if r.H == "" || seen[r.H] {
				continue
			}
			seen[r.H] = true
			out = append(out, CatalogItem{Title: r.T, Link: r.H, Category: c.Name})
		}
		fmt.Printf("    найдено: %d документов (всего: %d)\n", len(raw), len(out))

		// Пауза между категориями (как человек — отдохнул, перешёл дальше)
		if i < len(cats)-1 {
			pause := f.humanDelay(3000, 7000)
			fmt.Printf("    пауза %v...\n", pause)
			time.Sleep(pause)
		}
	}
	return out, nil
}

// EnrichMissing скачивает файлы для документов без локального файла.
// Если документ «действует» и задан ragSvc — сразу индексирует.
func (f *Fetcher) EnrichMissing(ctx context.Context, st store.Store, outRoot string, limit int, index func(ctx context.Context, id string) error) (int, []string) {
	docs, err := st.List(ctx, store.Filter{})
	if err != nil {
		return 0, []string{err.Error()}
	}
	var done int
	var errs []string
	for _, d := range docs {
		if d.LocalPath != "" || d.Status == model.StatusArchived || d.Status == model.StatusRejected {
			continue
		}
		if limit > 0 && done >= limit {
			break
		}
		outDir := filepath.Join(outRoot, "На_проверке", "Загружено")
		localPath, hash, err := f.FetchToDir(ctx, d.SourceURL, outDir)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", d.ID, err))
			continue
		}
		d.LocalPath = localPath
		d.FileHash = hash
		if err := st.Upsert(ctx, d); err != nil {
			errs = append(errs, fmt.Sprintf("%s: сохранение: %v", d.ID, err))
			continue
		}
		done++
		if d.Status == model.StatusActive && index != nil {
			if err := index(ctx, d.ID); err != nil {
				errs = append(errs, fmt.Sprintf("%s: индексация: %v", d.ID, err))
			}
		}
		// Пауза между скачиваниями (человек не качает пачками)
		time.Sleep(f.humanDelay(2000, 5000))
	}
	return done, errs
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
