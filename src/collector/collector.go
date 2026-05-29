// Package collector — автономный инструмент сбора, валидации и индексации
// документов с dochub.sk.ru с обходом WAF через симуляцию человека.
package collector

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
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

// stealthJS — расширенная маска автоматизации.
const stealthJS = `
(function() {
  Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
  window.chrome = { runtime: {} };
  Object.defineProperty(navigator, 'languages', { get: () => ['ru-RU','ru','en-US','en'] });
  Object.defineProperty(navigator, 'plugins', { get: () => [
    { name: 'Chrome PDF Plugin', description: 'Portable Document Format', filename: 'internal-pdf-viewer' },
    { name: 'Chrome PDF Viewer', description: '', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
    { name: 'Native Client', description: '', filename: 'internal-nacl-plugin' }
  ]});
  Object.defineProperty(navigator, 'platform', { get: () => 'Win32' });
  Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 });
  Object.defineProperty(navigator, 'deviceMemory', { get: () => 8 });
  Object.defineProperty(navigator, 'maxTouchPoints', { get: () => 0 });
  Object.defineProperty(navigator, 'connection', { get: () => ({
    effectiveType: '4g', rtt: 50, downlink: 10, saveData: false
  })});
  const originalQuery = window.navigator.permissions.query;
  window.navigator.permissions.query = function(args) {
    if (args.name === 'notifications') {
      return Promise.resolve({ state: Notification.permission });
    }
    return originalQuery.apply(this, arguments);
  };
  Object.defineProperty(Notification, 'permission', { get: () => 'default' });
  for (var key of Object.getOwnPropertyNames(navigator)) {
    if (key.startsWith('cdc_') || key.startsWith('domAutomation')) {
      delete navigator[key];
    }
  }
  delete document.__proto__.webdriver;
  for (var k of ['_selenium', 'callSelenium', '_Selenium_IDE_Recorder']) {
    if (window[k]) delete window[k];
  }
})();
`

// humanBehaviorJS — симуляция человеческого поведения.
const humanBehaviorJS = `(function(){
  function randomMouse() {
    var x = Math.floor(Math.random() * window.innerWidth);
    var y = Math.floor(Math.random() * window.innerHeight);
    var evt = new MouseEvent('mousemove', {clientX: x, clientY: y, bubbles: true});
    document.dispatchEvent(evt);
  }
  var moves = 2 + Math.floor(Math.random() * 4);
  for (var i = 0; i < moves; i++) {
    setTimeout(randomMouse, 300 + Math.random() * 800);
  }
  setTimeout(function(){
    window.scrollBy(0, Math.floor(window.innerHeight * 0.3 + Math.random() * 200));
  }, 1000 + Math.random() * 500);
  setTimeout(function(){
    window.scrollBy(0, -Math.floor(window.innerHeight * 0.15));
  }, 2500 + Math.random() * 500);
})()`

// jsFindFile — поиск ссылки на файл.
const jsFindFile = `(function(){
  var a=Array.from(document.querySelectorAll('a[href]')).map(function(x){return x.href;});
  var re=/cfs-file|cfs-filesystemfile|\.pdf|\.docx?|\.xlsx?|\.rtf|\.pptx?|download/i;
  for(var i=0;i<a.length;i++){if(re.test(a[i]))return a[i];}
  return "";
})()`

// CategorySpec — категория для перечисления.
type CategorySpec struct {
	Slug string
	Name string
}

// Collector — автономный сборщик данных.
type Collector struct {
	ChromePath  string
	ProxyURL    string
	Wait        time.Duration
	SourceURL   string
	DocsDir     string
	Store       store.Store
	IndexFn     func(ctx context.Context, id string) error
	AutoApprove bool
	Rng         *rand.Rand
	HTTP        *http.Client
}

// New создаёт сборщик.
func New(chromePath, proxyURL, sourceURL, docsDir string, wait time.Duration,
	st store.Store, indexFn func(ctx context.Context, id string) error, autoApprove bool) *Collector {
	if chromePath == "" {
		chromePath = detectChrome()
	}
	tr := &http.Transport{}
	if proxyURL != "" {
		pu, err := url.Parse(proxyURL)
		if err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &Collector{
		ChromePath:  chromePath,
		ProxyURL:    proxyURL,
		SourceURL:   sourceURL,
		DocsDir:     docsDir,
		Wait:        wait,
		Store:       st,
		IndexFn:     indexFn,
		AutoApprove: autoApprove,
		Rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
		HTTP:        &http.Client{Timeout: 120 * time.Second, Transport: tr},
	}
}

func detectChrome() string {
	for _, p := range []string{
		"/usr/bin/chromium", "/usr/bin/google-chrome", "/usr/bin/google-chrome-stable",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (c *Collector) execOpts() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(c.ChromePath),
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
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("enable-automation", false),
	}
	if c.ProxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(c.ProxyURL))
	}
	return opts
}

func (c *Collector) humanDelay(minMs, maxMs int) time.Duration {
	d := c.Rng.Intn(maxMs-minMs) + minMs
	return time.Duration(d) * time.Millisecond
}

func humanMouseMove(x, y int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
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

func humanScroll() chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond),
		chromedp.Evaluate(`window.scrollBy(0, Math.floor(window.innerHeight * (0.3 + Math.random() * 0.4)));`, nil),
		chromedp.Sleep(time.Duration(300+rand.Intn(800)) * time.Millisecond),
		chromedp.Evaluate(`window.scrollBy(0, -Math.floor(window.innerHeight * (0.1 + Math.random() * 0.2)));`, nil),
		chromedp.Sleep(time.Duration(200+rand.Intn(400)) * time.Millisecond),
	}
}

func (c *Collector) simulateHuman() chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Sleep(c.humanDelay(1500, 3000)),
		humanScroll(),
		chromedp.Evaluate(humanBehaviorJS, nil),
		chromedp.Sleep(c.humanDelay(1000, 2500)),
		chromedp.Evaluate(`window.scrollBy(0, Math.floor(window.innerHeight * 0.5));`, nil),
		chromedp.Sleep(c.humanDelay(800, 1500)),
		chromedp.Evaluate(`window.scrollTo({top: 0, behavior: 'smooth'});`, nil),
		chromedp.Sleep(c.humanDelay(500, 1000)),
	}
}

// FullCycle выполняет полный цикл: сбор каталога → скачивание файлов → валидация → индексация.
func (c *Collector) FullCycle(ctx context.Context) (*model.CollectorReport, error) {
	rep := &model.CollectorReport{
		StartedAt: time.Now(),
		Status:    "running",
	}

	log.Println("[collector] ═══════════════════════════════════════")
	log.Println("[collector] ЗАПУСК ПОЛНОГО ЦИКЛА СБОРА ДАННЫХ")
	log.Println("[collector] ═══════════════════════════════════════")

	// Шаг 1: Сбор каталога
	log.Println("[collector] Шаг 1/4: Сбор каталога документов...")
	if err := c.collectCatalog(ctx, rep); err != nil {
		rep.Status = "error"
		rep.Error = "каталог: " + err.Error()
		rep.FinishedAt = time.Now()
		return rep, err
	}

	// Шаг 2: Скачивание файлов
	log.Println("[collector] Шаг 2/4: Скачивание файлов документов...")
	c.downloadFiles(ctx, rep)

	// Шаг 3: Валидация
	log.Println("[collector] Шаг 3/4: Валидация данных...")
	valRep := c.validate(ctx)
	rep.ValidationErrors = valRep.InvalidDocs

	// Шаг 4: Индексация
	log.Println("[collector] Шаг 4/4: Индексация в RAG...")
	c.indexDocuments(ctx, rep)

	rep.FinishedAt = time.Now()
	rep.Status = "done"
	log.Printf("[collector] ═══════════════════════════════════════")
	log.Printf("[collector] Готово за %v", rep.FinishedAt.Sub(rep.StartedAt).Round(time.Second))
	log.Printf("[collector] Новых: %d | Обновлено: %d | Файлов: %d | Индексировано: %d",
		rep.DocumentsNew, rep.DocumentsUpd, rep.FilesDownloaded, rep.Indexed)
	log.Printf("[collector] ═══════════════════════════════════════")

	return rep, nil
}

func (c *Collector) collectCatalog(ctx context.Context, rep *model.CollectorReport) error {
	allocCtx, cancelA := chromedp.NewExecAllocator(ctx, c.execOpts()...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(allocCtx)
	defer cancelB()

	if err := chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
		return err
	})); err != nil {
		return fmt.Errorf("старт браузера: %w", err)
	}

	// Прогрев
	log.Println("  [browser] прогрев: dochub.sk.ru...")
	chromedp.Run(bctx,
		chromedp.Navigate("https://dochub.sk.ru/"),
		chromedp.Sleep(c.humanDelay(2000, 4000)),
		humanScroll(),
	)

	cats := []CategorySpec{
		{"cybersec_and_persdata", "Кибербезопасность и перс. данные"},
		{"legislative_acts", "Законодательные акты"},
		{"design_rules", "Правила проектирования"},
		{"other", "Иные нормативные документы"},
		{"development", "Развитие территории"},
		{"tenders", "Закупки и тендеры"},
		{"unactual_documents", "Утратившие силу"},
		{"anti_corruption", "Антикоррупция"},
	}

	base := strings.TrimSuffix(c.SourceURL, "/")
	seenIDs := map[string]bool{}
	oldIDs := map[string]bool{}

	allDocs, _ := c.Store.List(ctx, store.Filter{})
	for _, d := range allDocs {
		oldIDs[d.ID] = true
	}

	for i, cat := range cats {
		pageURL := base + "/p/" + cat.Slug + ".aspx"
		log.Printf("  [browser] категория %d/%d: %s", i+1, len(cats), cat.Name)

		var raw []struct {
			H string `json:"h"`
			T string `json:"t"`
		}

		tasks := chromedp.Tasks{
			chromedp.Navigate(pageURL),
			c.simulateHuman(),
			chromedp.Sleep(c.humanDelay(2000, 4000)),
			chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil),
			chromedp.Sleep(c.humanDelay(2000, 3000)),
			chromedp.Evaluate(`window.scrollTo(0, 0);`, nil),
			chromedp.Sleep(c.humanDelay(500, 1000)),
			chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href*="/m/docs/"]')).map(function(a){return {h:a.href,t:(a.textContent||'').replace(/\s+/g,' ').trim()};})`, &raw),
		}

		if err := chromedp.Run(bctx, tasks); err != nil {
			log.Printf("  ! категория %s: %v", cat.Slug, err)
			continue
		}

		for _, r := range raw {
			if r.H == "" {
				continue
			}
			sum := sha1.Sum([]byte(r.H))
			id := hex.EncodeToString(sum[:])
			seenIDs[id] = true

			title := strings.TrimSpace(strings.TrimPrefix(r.T, "File:"))
			if title == "" {
				continue
			}

			status := model.StatusPending
			if cat.Name == "Утратившие силу" || strings.Contains(strings.ToUpper(title), "УТРАТИЛ") {
				status = model.StatusOutdated
			}
			if c.AutoApprove && status == model.StatusPending {
				status = model.StatusActive
			}

			if existing, err := c.Store.Get(ctx, id); err == nil {
				marker := title + "|" + r.H
				if existing.PublishedAt != nil {
					marker += "|" + existing.PublishedAt.Format(time.RFC3339)
				}
				sum2 := sha256.Sum256([]byte(marker))
				hash := hex.EncodeToString(sum2[:])

				changed := false
				if existing.Title != title {
					existing.Title = title
					changed = true
				}
				if existing.Category != cat.Name {
					existing.Category = cat.Name
					changed = true
				}
				if existing.Status != status {
					existing.Status = status
					changed = true
				}
				if existing.FileHash != hash {
					existing.FileHash = hash
					changed = true
				}
				if changed {
					existing.FetchedAt = time.Now()
					if err := c.Store.Upsert(ctx, existing); err != nil {
						log.Printf("  ! сохранение %s: %v", id, err)
					} else {
						rep.DocumentsUpd++
					}
				} else {
					rep.DocumentsSame++
				}
			} else {
				doc := model.Document{
					ID:        id,
					Title:     title,
					SourceURL: r.H,
					FetchedAt: time.Now(),
					Status:    status,
					Category:  cat.Name,
				}
				if err := c.Store.Upsert(ctx, doc); err != nil {
					log.Printf("  ! новый %s: %v", id, err)
				} else {
					rep.DocumentsNew++
					log.Printf("  + новый: %s", title)
				}
			}
			delete(oldIDs, id)
		}

		log.Printf("    найдено: %d документов", len(raw))

		if i < len(cats)-1 {
			pause := c.humanDelay(3000, 7000)
			time.Sleep(pause)
		}
	}

	// Удалённые документы
	for id := range oldIDs {
		rep.DocumentsRemoved++
		log.Printf("  - удалён на источнике: %s", id)
	}

	return nil
}

func (c *Collector) downloadFiles(ctx context.Context, rep *model.CollectorReport) {
	docs, err := c.Store.List(ctx, store.Filter{})
	if err != nil {
		log.Printf("  ! скачивание: %v", err)
		return
	}

	for _, d := range docs {
		if d.LocalPath != "" || d.Status == model.StatusArchived || d.Status == model.StatusRejected {
			continue
		}

		log.Printf("  ↓ скачивание: %s", d.Title)
		localPath, hash, err := c.fetchFile(ctx, d.SourceURL, d.Category)
		if err != nil {
			log.Printf("  ! файл %s: %v", d.ID, err)
			rep.FilesErrors++
			continue
		}

		d.LocalPath = localPath
		d.FileHash = hash
		if err := c.Store.Upsert(ctx, d); err != nil {
			log.Printf("  ! сохранение %s: %v", d.ID, err)
			continue
		}
		rep.FilesDownloaded++
		log.Printf("  ✓ файл сохранён: %s", filepath.Base(localPath))

		time.Sleep(c.humanDelay(2000, 5000))
	}
}

func (c *Collector) fetchFile(ctx context.Context, viewerURL, category string) (string, string, error) {
	allocCtx, cancelA := chromedp.NewExecAllocator(ctx, c.execOpts()...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(allocCtx)
	defer cancelB()

	var fileURL string
	var cookies []*network.Cookie

	tasks := chromedp.Tasks{
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
			return err
		}),
		chromedp.Navigate(viewerURL),
		c.simulateHuman(),
		chromedp.Evaluate(jsFindFile, &fileURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	}

	if err := chromedp.Run(bctx, tasks); err != nil {
		return "", "", fmt.Errorf("браузер: %w", err)
	}
	if strings.TrimSpace(fileURL) == "" {
		return "", "", fmt.Errorf("ссылка на файл не найдена")
	}

	data, err := c.downloadHTTP(ctx, fileURL, viewerURL, cookies)
	if err != nil {
		return "", "", err
	}

	statusDir := "На_проверке"
	if category == "Утратившие силу" {
		statusDir = "Архив"
	}
	catSlug := strings.ReplaceAll(category, " ", "_")
	if catSlug == "" {
		catSlug = "Без_категории"
	}
	outDir := filepath.Join(c.DocsDir, statusDir, catSlug)
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

func (c *Collector) downloadHTTP(ctx context.Context, fileURL, referer string, cookies []*network.Cookie) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/pdf,application/msword,application/vnd.openxmlformats-officedocument.*,*/*")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	if ch := cookieHeader(cookies); ch != "" {
		req.Header.Set("Cookie", ch)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("статус %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *Collector) validate(ctx context.Context) *model.ValidationReport {
	docs, err := c.Store.List(ctx, store.Filter{})
	if err != nil {
		return &model.ValidationReport{TotalDocs: 0, InvalidDocs: 0}
	}

	rep := &model.ValidationReport{TotalDocs: len(docs)}
	for _, d := range docs {
		valid := true
		if d.Title == "" {
			valid = false
		}
		if d.SourceURL == "" {
			valid = false
		}
		if d.Status == model.StatusActive && d.LocalPath == "" {
			rep.MissingFiles++
		}
		if valid {
			rep.ValidDocs++
		} else {
			rep.InvalidDocs++
		}
	}
	return rep
}

func (c *Collector) indexDocuments(ctx context.Context, rep *model.CollectorReport) {
	if c.IndexFn == nil {
		return
	}
	docs, err := c.Store.List(ctx, store.Filter{Status: model.StatusActive})
	if err != nil {
		return
	}
	for _, d := range docs {
		if d.Indexed {
			continue
		}
		if err := c.IndexFn(ctx, d.ID); err != nil {
			log.Printf("  ! индексация %s: %v", d.ID, err)
		} else {
			rep.Indexed++
			log.Printf("  ✓ индексирован: %s", d.Title)
		}
	}
}

func cookieHeader(cookies []*network.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, co := range cookies {
		parts = append(parts, co.Name+"="+co.Value)
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
