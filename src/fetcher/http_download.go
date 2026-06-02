package fetcher

// HTTP-скачивание тел файлов dochub по сессионной куке — БЕЗ браузера и БЕЗ прокси.
//
// Предыстория (проверено эмпирически 2026-06-01): WAF dochub (ServicePipe)
// блокирует автоматизацию (curl без куки, headless И headful chromedp) на всех
// страницах документов — независимо от IP (датацентр, мобильный RU/US, чистый
// домашний). НО: с сессионной кукой реального браузера, прошедшего WAF
// (`spid` + `AuthorizationCookie`), ОБЫЧНЫЙ http.Client отдаёт и страницы
// категорий (200), и тела файлов — с любого IP, без браузера. Кука не привязана
// к IP. Реальная ссылка скачивания: `…/download.aspx` → 302 → `…/cfs-file.ashx/…`
// → файл (Content-Disposition: attachment). ServicePipe жёстко лимитит частоту
// (перебор ловит 418/403), поэтому качаем с «человеческими» паузами.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// CookieDoc — документ, найденный в категории и скачанный по куке.
type CookieDoc struct {
	Title     string
	URL       string // download-ссылка, по которой скачано тело
	Category  string
	LocalPath string
	Hash      string
}

// CollectViaCookie проходит категории dochub с сессионной кукой, достаёт реальные
// download-ссылки из HTML и качает тела файлов обычным HTTP — без браузера и без
// прокси. Между файлами «человеческие» паузы (ServicePipe лимитит частоту:
// перебор ловит 418/403). limit<=0 — без ограничения. Возвращает скачанные
// документы и ошибки (403 на «мёртвых» дублях /m/docs/ — это норма).
// onDoc (если задан) вызывается СРАЗУ после успешного скачивания каждого файла —
// чтобы регистрировать его в реестр инкрементально (прогресс не теряется при
// обрыве/перезапуске), а не одним пакетом в конце.
func (f *Fetcher) CollectViaCookie(ctx context.Context, baseURL string, cats []CategorySpec, outDir string, limit int, logf func(string), onDoc func(CookieDoc)) ([]CookieDoc, []string) {
	if logf == nil {
		logf = func(string) {}
	}
	if !f.HasCookie() {
		return nil, []string{"кука dochub не задана — HTTP-скачивание по куке выключено"}
	}
	base := strings.TrimSuffix(baseURL, "/")
	var out []CookieDoc
	var errs []string
	seen := map[string]bool{}      // уже обработанные ссылки
	okTitles := map[string]bool{}  // заголовки с уже скачанным файлом (дедуп дублей)
	done := 0

	for _, c := range cats {
		if limit > 0 && done >= limit {
			break
		}
		pageURL := base + "/p/" + c.Slug + ".aspx"
		for visited := 0; pageURL != "" && visited < maxPagesPerCategory; visited++ {
			docs, next, err := f.fetchCategoryPage(ctx, pageURL)
			if err != nil {
				errs = append(errs, fmt.Sprintf("категория %s: %v", c.Slug, err))
				break
			}
			for _, d := range docs {
				if limit > 0 && done >= limit {
					break
				}
				if d.Link == "" || seen[d.Link] {
					continue
				}
				seen[d.Link] = true

				// Возобновляемость: пропускаем уже скачанные ранее файлы.
				if f.SkipURL != nil && f.SkipURL(d.Link) {
					if tkey := strings.ToLower(cleanCatalogTitle(d.Title)); tkey != "" {
						okTitles[tkey] = true // и его дубль больше не трогаем
					}
					continue
				}

				// Дедуп по заголовку: если файл с таким названием уже скачан
				// (часто документ залит дважды — рабочая ссылка + «мёртвый» дубль),
				// второй не трогаем — не качаем повторно и не шумим 403-ошибкой.
				tkey := strings.ToLower(cleanCatalogTitle(d.Title))
				if tkey != "" && okTitles[tkey] {
					continue
				}

				if len(out)+len(errs) > 0 { // пауза перед каждым файлом, кроме первого
					select {
					// Умеренная пауза (~1.5–3 с): для авторизованной куки этого
					// достаточно против лимита ServicePipe, но в разы быстрее
					// «человеческого» профиля headless-обхода (там 7 с + перерывы).
					case <-time.After(f.humanDelay(1500, 3000)):
					case <-ctx.Done():
						return out, errs
					}
				}
				lp, hash, derr := f.DownloadViaCookie(ctx, d.Link, pageURL, outDir)
				if derr != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", d.Link, derr))
					continue
				}
				cd := CookieDoc{
					Title:     cleanCatalogTitle(d.Title),
					URL:       d.Link,
					Category:  c.Name,
					LocalPath: lp,
					Hash:      hash,
				}
				out = append(out, cd)
				if tkey != "" {
					okTitles[tkey] = true
				}
				done++
				logf(fmt.Sprintf("скачано %d: %s", done, filepath.Base(lp)))
				if onDoc != nil {
					onDoc(cd) // инкрементальная регистрация — прогресс сохраняется сразу
				}
			}
			pageURL = next
			if pageURL != "" {
				select {
				case <-time.After(f.humanDelay(800, 2000)):
				case <-ctx.Done():
					return out, errs
				}
			}
		}
	}
	return out, errs
}

// cleanCatalogTitle нормализует заголовок ссылки из HTML категории.
func cleanCatalogTitle(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimSpace(strings.TrimPrefix(t, "File:"))
	return t
}

// browserUA — UA реального браузера (совпадает с тем, под которым выдана кука).
// Переопределяется вместе с кукой при необходимости.

// applyDochubHeaders ставит браузерные заголовки и сессионную куку на запрос.
func (f *Fetcher) applyDochubHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru,en;q=0.9")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if f.Cookie != "" {
		req.Header.Set("Cookie", f.Cookie)
	}
}

// HasCookie сообщает, задана ли сессионная кука (доступно ли HTTP-скачивание).
func (f *Fetcher) HasCookie() bool {
	return strings.TrimSpace(f.Cookie) != ""
}

// DownloadViaCookie скачивает тело файла по download-ссылке dochub с сессионной
// кукой. Следует за 302 (download.aspx → cfs-file.ashx), проверяет, что пришёл
// файл (а не HTML/блок WAF), и сохраняет в outDir. Имя берёт из
// Content-Disposition, иначе из URL + типа. Возвращает путь и sha256.
func (f *Fetcher) DownloadViaCookie(ctx context.Context, fileURL, referer, outDir string) (string, string, error) {
	if !f.HasCookie() {
		return "", "", fmt.Errorf("кука dochub не задана — HTTP-скачивание по куке выключено")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return "", "", err
	}
	f.applyDochubHeaders(req, referer)

	resp, err := f.HTTP.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// ниже
	case http.StatusForbidden, http.StatusServiceUnavailable, 418:
		return "", "", fmt.Errorf("%w: HTTP %d (кука протухла либо сработал лимит WAF — обнови куку/сбавь темп)", errWAFBlock, resp.StatusCode)
	default:
		return "", "", fmt.Errorf("скачивание: статус %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", "", err
	}

	name := filenameFromResponse(resp, fileURL)
	// validateFileBytes извлекает расширение из переданного имени и проверяет
	// сигнатуру (PK для docx, %PDF и т.п.) + отсекает HTML/челлендж WAF.
	if err := validateFileBytes(data, name); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", "", err
	}
	localPath := filepath.Join(outDir, name)
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(data)
	return localPath, hex.EncodeToString(sum[:]), nil
}

// filenameFromResponse выбирает имя файла: сперва из Content-Disposition
// (в т.ч. RFC 5987 filename*=UTF-8''…), иначе из id в URL + расширение по типу.
func filenameFromResponse(resp *http.Response, fileURL string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn := strings.TrimSpace(params["filename"]); fn != "" {
				return sanitizeFilename(fn)
			}
		}
	}
	// Фоллбэк: последний осмысленный сегмент пути (id) + расширение по типу.
	base := idFromDochubURL(fileURL)
	ext := extFromContentType(resp.Header.Get("Content-Type"))
	if base == "" {
		base = "document"
	}
	return sanitizeFilename(base + ext)
}

// idFromDochubURL вытаскивает числовой id из …/m/(docs|wiki)/{id}/download.aspx.
func idFromDochubURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if isAllDigits(parts[i]) {
			return parts[i]
		}
	}
	return ""
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// extFromContentType сопоставляет MIME-тип расширению (для имён без C-D).
func extFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "application/pdf":
		return ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/msword":
		return ".doc"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	case "application/vnd.ms-powerpoint":
		return ".ppt"
	case "application/rtf", "text/rtf":
		return ".rtf"
	case "application/zip":
		return ".zip"
	default:
		return ".bin"
	}
}

// sanitizeFilename убирает путь и запрещённые символы из имени файла.
func sanitizeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 0x20 {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "document"
	}
	return name
}
