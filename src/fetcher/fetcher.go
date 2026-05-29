// Package fetcher скачивает тела файлов документов через headless-браузер (chromedp).
//
// Зачем браузер: dochub.sk.ru за анти-бот WAF (Variti) — страницы документов
// /m/docs/ отдают 403 обычным HTTP-клиентам. Реальный браузер исполняет
// JS-челлендж и получает доступ. ВАЖНО: WAF также блокирует трафик с
// дата-центровых IP, поэтому загрузчик следует запускать из разрешённой сети
// (рабочая машина) или через резидентный прокси (PROXY_URL).
package fetcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// stealthJS маскирует признаки автоматизации.
const stealthJS = `
Object.defineProperty(navigator,'webdriver',{get:()=>undefined});
Object.defineProperty(navigator,'languages',{get:()=>['ru-RU','ru','en-US','en']});
Object.defineProperty(navigator,'plugins',{get:()=>[1,2,3,4,5]});
window.chrome={runtime:{}};
`

// jsFindFile — выражение, возвращающее первую ссылку на файл документа.
const jsFindFile = `(function(){
  var a=Array.from(document.querySelectorAll('a[href]')).map(function(x){return x.href;});
  var re=/cfs-file|cfs-filesystemfile|\.pdf|\.docx?|\.xlsx?|\.rtf|\.pptx?|download/i;
  for(var i=0;i<a.length;i++){if(re.test(a[i]))return a[i];}
  return "";
})()`

// Fetcher скачивает файлы через headless-Chrome.
type Fetcher struct {
	ChromePath string
	ProxyURL   string
	Wait       time.Duration
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
		chromedp.UserAgent(userAgent),
	}
	if f.ProxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(f.ProxyURL))
	}
	return opts
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
	err := chromedp.Run(bctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
			return err
		}),
		chromedp.Navigate(viewerURL),
		chromedp.Sleep(f.Wait),
		chromedp.Evaluate(jsFindFile, &fileURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	)
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
// Это полное перечисление каталога (в отличие от RSS, отдающего ~20 последних).
// Требует разрешённой сети/прокси — WAF блокирует браузер с дата-центровых IP.
func (f *Fetcher) EnumerateCategories(ctx context.Context, baseURL string, cats []CategorySpec) ([]CatalogItem, error) {
	allocCtx, cancelA := chromedp.NewExecAllocator(ctx, f.execOpts()...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(allocCtx)
	defer cancelB()

	// «Прогрев» (первый Run стартует браузер) + stealth на все документы.
	if err := chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(stealthJS).Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("старт браузера: %w", err)
	}

	base := strings.TrimSuffix(baseURL, "/")
	var out []CatalogItem
	seen := map[string]bool{}

	for _, c := range cats {
		pageURL := base + "/p/" + c.Slug + ".aspx"
		var raw []struct {
			H string `json:"h"`
			T string `json:"t"`
		}
		err := chromedp.Run(bctx,
			chromedp.Navigate(pageURL),
			chromedp.Sleep(f.Wait),
			chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href*="/m/docs/"]')).map(function(a){return {h:a.href,t:(a.textContent||'').replace(/\s+/g,' ').trim()};})`, &raw),
		)
		if err != nil {
			return out, fmt.Errorf("категория %s: %w", c.Slug, err)
		}
		for _, r := range raw {
			if r.H == "" || seen[r.H] {
				continue
			}
			seen[r.H] = true
			out = append(out, CatalogItem{Title: r.T, Link: r.H, Category: c.Name})
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
