// proxy_manager.go — управление конфигурацией прокси в runtime.
package admin

import (
	"context"
	"os"
	"sync"
	"time"

	"baza-skolkovo/src/fetcher"
)

// ProxyConfig — конфигурация одного прокси.
type ProxyConfig struct {
	Type    string `json:"type"`    // none, cloudflare, webshare, iproyal
	BaseURL string `json:"baseURL"` // URL прокси-сервера
	Label   string `json:"label"`   // Человекочитаемое название
}

// ProxyStatus — статус прокси после проверки.
type ProxyStatus struct {
	Config  ProxyConfig `json:"config"`
	IP      string      `json:"ip"`
	Latency time.Duration `json:"latency"`
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
}

// ProxyManager управляет списком прокси и текущей конфигурацией.
type ProxyManager struct {
	mu          sync.RWMutex
	proxies     []ProxyConfig
	currentType string
	testCtx     context.Context
}

// NewProxyManager создаёт менеджер из переменных окружения.
func NewProxyManager(ctx context.Context) *ProxyManager {
	proxies := []ProxyConfig{
		{Type: "none", BaseURL: "", Label: "Без прокси (прямое подключение)"},
	}

	if url := os.Getenv("PROXY_CLOUDFLARE_URL"); url != "" {
		proxies = append(proxies, ProxyConfig{
			Type: "cloudflare", BaseURL: url, Label: "Cloudflare Worker",
		})
	}
	if url := os.Getenv("PROXY_WEBSHARE_URL"); url != "" {
		proxies = append(proxies, ProxyConfig{
			Type: "webshare", BaseURL: url, Label: "WebShare",
		})
	}
	if url := os.Getenv("PROXY_IPROYAL_URL"); url != "" {
		proxies = append(proxies, ProxyConfig{
			Type: "iproyal", BaseURL: url, Label: "IPRoyal",
		})
	}

	currentType := os.Getenv("PROXY_TYPE")
	if currentType == "" {
		currentType = "none"
	}

	return &ProxyManager{
		proxies:     proxies,
		currentType: currentType,
		testCtx:     ctx,
	}
}

// List возвращает список доступных прокси.
func (pm *ProxyManager) List() []ProxyConfig {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]ProxyConfig, len(pm.proxies))
	copy(result, pm.proxies)
	return result
}

// Current возвращит текущий тип прокси.
func (pm *ProxyManager) Current() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.currentType
}

// Switch переключает текущий прокси.
func (pm *ProxyManager) Switch(proxyType string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, p := range pm.proxies {
		if p.Type == proxyType {
			pm.currentType = proxyType
			os.Setenv("PROXY_TYPE", proxyType)
			return nil
		}
	}
	return nil
}

// Test проверяет конкретный прокси и возвращает его статус.
func (pm *ProxyManager) Test(proxyType string) ProxyStatus {
	var cfg ProxyConfig
	pm.mu.RLock()
	for _, p := range pm.proxies {
		if p.Type == proxyType {
			cfg = p
			break
		}
	}
	pm.mu.RUnlock()

	client := &fetcher.ProxyClient{
		Type:    cfg.Type,
		BaseURL: cfg.BaseURL,
		Timeout: 10 * time.Second,
	}

	start := time.Now()
	ip, err := client.TestProxy()
	latency := time.Since(start)

	status := ProxyStatus{
		Config:  cfg,
		IP:      ip,
		Latency: latency,
		OK:      err == nil,
	}
	if err != nil {
		status.Error = err.Error()
	}
	return status
}

// TestAll проверяет все прокси параллельно.
func (pm *ProxyManager) TestAll() []ProxyStatus {
	proxies := pm.List()
	results := make([]ProxyStatus, len(proxies))

	var wg sync.WaitGroup
	for i, p := range proxies {
		wg.Add(1)
		go func(idx int, cfg ProxyConfig) {
			defer wg.Done()
			results[idx] = pm.Test(cfg.Type)
		}(i, p)
	}
	wg.Wait()

	return results
}
