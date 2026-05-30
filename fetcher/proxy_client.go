package fetcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ProxyClient — клиент для скачивания через прокси
type ProxyClient struct {
	Type    string
	BaseURL string
	Timeout time.Duration
}

// NewProxyClient создаёт клиент из env
func NewProxyClient() *ProxyClient {
	proxyType := os.Getenv("PROXY_TYPE")
	if proxyType == "" {
		proxyType = "none"
	}

	baseURL := ""
	switch proxyType {
	case "cloudflare":
		baseURL = os.Getenv("PROXY_CLOUDFLARE_URL")
	case "webshare":
		baseURL = os.Getenv("PROXY_WEBSHARE_URL")
	case "iproyal":
		baseURL = os.Getenv("PROXY_IPROYAL_URL")
	}

	return &ProxyClient{
		Type:    proxyType,
		BaseURL: baseURL,
		Timeout: 90 * time.Second,
	}
}

// Fetch скачивает документ через прокси
func (pc *ProxyClient) Fetch(documentURL string) ([]byte, error) {
	if pc.Type == "none" || pc.BaseURL == "" {
		return pc.directFetch(documentURL)
	}

	switch pc.Type {
	case "cloudflare":
		return pc.cloudflareFetch(documentURL)
	case "webshare", "iproyal":
		return pc.authProxyFetch(documentURL)
	default:
		return pc.directFetch(documentURL)
	}
}

func (pc *ProxyClient) directFetch(docURL string) ([]byte, error) {
	client := &http.Client{Timeout: pc.Timeout}
	req, _ := http.NewRequest("GET", docURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/pdf,application/msword,application/vnd.openxmlformats-officedocument.*,*/*")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("direct fetch: status %d", resp.StatusCode)
	}

	return readAll(resp.Body)
}

func (pc *ProxyClient) cloudflareFetch(docURL string) ([]byte, error) {
	proxyURL := pc.BaseURL + url.QueryEscape(docURL)
	client := &http.Client{Timeout: pc.Timeout}
	resp, err := client.Get(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("cloudflare proxy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cloudflare proxy: status %d", resp.StatusCode)
	}

	return readAll(resp.Body)
}

func (pc *ProxyClient) authProxyFetch(docURL string) ([]byte, error) {
	client := &http.Client{Timeout: pc.Timeout}
	req, _ := http.NewRequest("GET", docURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/pdf,application/msword,application/vnd.openxmlformats-officedocument.*,*/*")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

	if pc.BaseURL != "" {
		proxyURL, err := url.Parse(pc.BaseURL)
		if err == nil && proxyURL.User != nil {
			user := proxyURL.User.Username()
			pass, _ := proxyURL.User.Password()
			req.SetBasicAuth(user, pass)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth proxy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("auth proxy: status %d", resp.StatusCode)
	}

	return readAll(resp.Body)
}

// TestProxy тестирует прокси и возвращает IP
func (pc *ProxyClient) TestProxy() (string, error) {
	if pc.Type == "none" {
		return "direct", nil
	}

	testURL := "https://api.ipify.org?format=json"
	if pc.Type == "cloudflare" {
		testURL = pc.BaseURL + url.QueryEscape(testURL)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("test failed: status %d", resp.StatusCode)
	}

	var result struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.IP, nil
}

func readAll(body io.ReadCloser) ([]byte, error) {
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 32*1024)
	for {
		n, rerr := body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	return buf, nil
}
