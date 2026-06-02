// proxy_finder.go — автоматический поиск работающего российского прокси.
//
// Два источника (в порядке приоритета):
//  1. proxy6.net API — платные российские прокси (требует API-ключ).
//  2. Публичные списки бесплатных прокси — менее надёжны, проверяются перед использованием.
//
// Найденный прокси проверяется запросом к dochub.sk.ru — если 200/XML то прокси работает.
package fetcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// dochubProbeURL — URL для проверки прокси: должен вернуть 200 OK с XML.
	dochubProbeURL = "https://dochub.sk.ru/foundation/documents/rss.aspx"
	proxyTestTimeout = 12 * time.Second
)

// RussianProxyFinder находит работающий российский прокси.
type RussianProxyFinder struct {
	// Proxy6APIKey — ключ API proxy6.net (необязательно; без него только бесплатные прокси).
	Proxy6APIKey string
}

// Find возвращает URL первого работающего российского прокси.
// Формат: http://user:pass@host:port или http://host:port.
func (pf *RussianProxyFinder) Find(ctx context.Context) (string, error) {
	// 1. Пробуем proxy6.net (платный, надёжный).
	if pf.Proxy6APIKey != "" {
		if proxies, err := pf.fetchProxy6Net(ctx); err != nil {
			log.Printf("[proxy-finder] proxy6.net: %v", err)
		} else {
			for _, p := range proxies {
				if ok := pf.testProxy(ctx, p); ok {
					log.Printf("[proxy-finder] proxy6.net нашёл рабочий прокси: %s", maskProxyURL(p))
					return p, nil
				}
			}
		}
	}

	// 2. Бесплатные публичные прокси из нескольких источников.
	candidates, err := pf.fetchFreeRussianProxies(ctx)
	if err != nil {
		return "", fmt.Errorf("proxy-finder: не удалось получить список прокси: %w", err)
	}
	// Тестируем в случайном порядке до первого рабочего.
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	for _, p := range candidates {
		if ok := pf.testProxy(ctx, p); ok {
			log.Printf("[proxy-finder] бесплатный прокси работает: %s", p)
			return p, nil
		}
	}
	return "", fmt.Errorf("proxy-finder: ни один из %d кандидатов не прошёл проверку dochub.sk.ru", len(candidates))
}

// ---------------------------------------------------------------------------
// proxy6.net API
// ---------------------------------------------------------------------------

type proxy6Response struct {
	Status string                     `json:"status"`
	List   map[string]proxy6ProxyItem `json:"list"`
}

type proxy6ProxyItem struct {
	Host   string `json:"host"`
	Port   string `json:"port"`
	User   string `json:"user"`
	Pass   string `json:"pass"`
	Type   string `json:"type"`
	Active string `json:"active"`
}

func (pf *RussianProxyFinder) fetchProxy6Net(ctx context.Context) ([]string, error) {
	apiURL := fmt.Sprintf("https://proxy6.net/api/%s/getproxy?count=5&type=http&country=ru&state=5",
		pf.Proxy6APIKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result proxy6Response
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("proxy6 JSON: %w", err)
	}
	if result.Status != "yes" {
		return nil, fmt.Errorf("proxy6 status=%s", result.Status)
	}

	var out []string
	for _, item := range result.List {
		if item.Active != "1" {
			continue
		}
		// proxy6 отдаёт type как "auto"/"http"/"https" (HTTP-совместимый прокси) или
		// "socks"/"socks5". http.Transport.Proxy понимает только http/https/socks5,
		// поэтому всё, кроме socks, нормализуем в http (иначе получится auto:// и
		// "unsupported proxy scheme").
		scheme := "http"
		switch strings.ToLower(item.Type) {
		case "socks", "socks5":
			scheme = "socks5"
		}
		var proxyURL string
		if item.User != "" {
			proxyURL = fmt.Sprintf("%s://%s:%s@%s:%s", scheme, item.User, item.Pass, item.Host, item.Port)
		} else {
			proxyURL = fmt.Sprintf("%s://%s:%s", scheme, item.Host, item.Port)
		}
		out = append(out, proxyURL)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Публичные бесплатные прокси
// ---------------------------------------------------------------------------

func (pf *RussianProxyFinder) fetchFreeRussianProxies(ctx context.Context) ([]string, error) {
	sources := []struct {
		url    string
		parser func([]byte) []string
	}{
		{
			// ProxyScrape — только российские, HTTP, elite
			"https://api.proxyscrape.com/v2/?request=getproxies&protocol=http&timeout=4000&country=RU&anonymity=elite&simplified=true",
			parsePlainLines,
		},
		{
			// GeoNode — российские HTTP
			"https://proxylist.geonode.com/api/proxy-list?limit=50&page=1&sort_by=lastChecked&sort_type=desc&country=RU&protocols=http",
			parseGeoNodeJSON,
		},
		{
			// ProxyScrape SOCKS5 — российские
			"https://api.proxyscrape.com/v2/?request=getproxies&protocol=socks5&timeout=4000&country=RU&simplified=true",
			parsePlainLinesSocks5,
		},
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	var all []string
	for _, src := range sources {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, src.url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("[proxy-finder] источник %s недоступен: %v", src.url, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		items := src.parser(body)
		log.Printf("[proxy-finder] %s → %d кандидатов", src.url, len(items))
		all = append(all, items...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("все источники недоступны или пусты")
	}
	return all, nil
}

func parsePlainLines(body []byte) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, "http://"+line)
	}
	return out
}

func parsePlainLinesSocks5(body []byte) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, "socks5://"+line)
	}
	return out
}

type geoNodeResponse struct {
	Data []struct {
		IP       string   `json:"ip"`
		Port     string   `json:"port"`
		Protocols []string `json:"protocols"`
	} `json:"data"`
}

func parseGeoNodeJSON(body []byte) []string {
	var resp geoNodeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	var out []string
	for _, item := range resp.Data {
		scheme := "http"
		if len(item.Protocols) > 0 {
			scheme = item.Protocols[0]
		}
		out = append(out, fmt.Sprintf("%s://%s:%s", scheme, item.IP, item.Port))
	}
	return out
}

// ---------------------------------------------------------------------------
// Тест прокси против dochub.sk.ru
// ---------------------------------------------------------------------------

// testProxy проверяет прокси запросом к dochub.sk.ru/rss.
// Возвращает true если ответ 200 и содержит XML.
func (pf *RussianProxyFinder) testProxy(ctx context.Context, proxyURL string) bool {
	pURL, err := url.Parse(proxyURL)
	if err != nil {
		return false
	}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(pURL),
		TLSHandshakeTimeout: 8 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   proxyTestTimeout,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dochubProbeURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	// Проверяем что это XML (не страница-заглушка)
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	return strings.Contains(string(body[:n]), "<?xml") || strings.Contains(string(body[:n]), "<rss")
}

// maskProxyURL скрывает пароль в URL прокси для безопасного логирования.
func maskProxyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return u.String()
}
