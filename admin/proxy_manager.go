package admin

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ProxyConfig — конфигурация одного прокси/VPN
type ProxyConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`       // "http", "socks5", "ovpn_file", "api"
	URL       string `json:"url"`        // URL прокси или путь к файлу
	Active    bool   `json:"active"`     // Используется ли сейчас
	LastCheck string `json:"last_check"` // Последняя проверка
	Status    string `json:"status"`     // "ok", "error", "unknown"
	Error     string `json:"error"`
}

// ProxyManager — менеджер прокси/VPN
type ProxyManager struct {
	Path      string
	Proxies   []ProxyConfig // Exported
	mu        sync.Mutex
	CurrentID string
}

// NewProxyManager создает менеджер
func NewProxyManager(path string) *ProxyManager {
	pm := &ProxyManager{
		Path:    path,
		Proxies: []ProxyConfig{},
	}
	pm.load()
	return pm
}

func (pm *ProxyManager) load() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	data, err := os.ReadFile(pm.Path)
	if err != nil {
		pm.save()
		return
	}
	json.Unmarshal(data, &pm.Proxies)

	for _, p := range pm.Proxies {
		if p.Active {
			pm.CurrentID = p.ID
		}
	}
}

func (pm *ProxyManager) save() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	activeCount := 0
	for i := range pm.Proxies {
		if pm.Proxies[i].Active {
			activeCount++
			pm.CurrentID = pm.Proxies[i].ID
		}
	}
	if activeCount == 0 && len(pm.Proxies) > 0 {
		pm.Proxies[0].Active = true
		pm.CurrentID = pm.Proxies[0].ID
	}

	data, _ := json.MarshalIndent(pm.Proxies, "", "  ")
	os.MkdirAll(filepath.Dir(pm.Path), 0755)
	os.WriteFile(pm.Path, data, 0644)
}

// GetActiveURL возвращает URL активного прокси
func (pm *ProxyManager) GetActiveURL() string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, p := range pm.Proxies {
		if p.Active && p.Type != "ovpn_file" {
			return p.URL
		}
	}
	return ""
}

// AddProxy добавляет новый прокси
func (pm *ProxyManager) AddProxy(name, pType, urlStr string) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id := fmt.Sprintf("%x", sha256.Sum256([]byte(name+urlStr+time.Now().String())))[:8]

	newProxy := ProxyConfig{
		ID:     id,
		Name:   name,
		Type:   pType,
		URL:    urlStr,
		Status: "unknown",
	}

	if len(pm.Proxies) == 0 {
		newProxy.Active = true
		pm.CurrentID = id
	}

	pm.Proxies = append(pm.Proxies, newProxy)
	pm.save()
	return id
}

// ActivateProxy активирует прокси
func (pm *ProxyManager) ActivateProxy(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	found := false
	for i := range pm.Proxies {
		if pm.Proxies[i].ID == id {
			pm.Proxies[i].Active = true
			pm.CurrentID = id
			found = true
		} else {
			pm.Proxies[i].Active = false
		}
	}

	if !found {
		return fmt.Errorf("proxy not found")
	}

	pm.save()
	return nil
}

// RemoveProxy удаляет прокси
func (pm *ProxyManager) RemoveProxy(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i, p := range pm.Proxies {
		if p.ID == id {
			pm.Proxies = append(pm.Proxies[:i], pm.Proxies[i+1:]...)
			if pm.CurrentID == id {
				pm.CurrentID = ""
				if len(pm.Proxies) > 0 {
					pm.Proxies[0].Active = true
					pm.CurrentID = pm.Proxies[0].ID
				}
			}
			pm.save()
			return nil
		}
	}
	return fmt.Errorf("proxy not found")
}

// TestProxy тестирует прокси
func (pm *ProxyManager) TestProxy(id string) (bool, string, error) {
	pm.mu.Lock()
	var proxy ProxyConfig
	var proxyIdx int = -1
	for i, p := range pm.Proxies {
		if p.ID == id {
			proxy = p
			proxyIdx = i
			break
		}
	}
	pm.mu.Unlock()

	if proxyIdx == -1 {
		return false, "", fmt.Errorf("proxy not found")
	}

	var ok bool
	var ip string
	var err error

	if proxy.Type == "http" || proxy.Type == "socks5" {
		ok, ip, err = testHTTPProxy(proxy.URL)
	} else if proxy.Type == "ovpn_file" {
		_, err := os.Stat(proxy.URL)
		if err == nil {
			ok = true
			ip = "OVPN Config"
		} else {
			ok = false
			ip = ""
		}
	} else {
		ok = false
		ip = ""
		err = fmt.Errorf("unsupported type")
	}

	pm.mu.Lock()
	pm.Proxies[proxyIdx].LastCheck = time.Now().Format("2006-01-02 15:04:05")
	pm.Proxies[proxyIdx].Status = "ok"
	if err != nil {
		pm.Proxies[proxyIdx].Status = "error"
		pm.Proxies[proxyIdx].Error = err.Error()
	}
	pm.save()
	pm.mu.Unlock()

	return ok, ip, err
}

// AutoSwitch переключает на первый рабочий прокси
func (pm *ProxyManager) AutoSwitch() string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for i, p := range pm.Proxies {
		if p.Status == "ok" || p.Type == "ovpn_file" {
			pm.Proxies[i].Active = true
			pm.CurrentID = p.ID
			for j := range pm.Proxies {
				if i != j {
					pm.Proxies[j].Active = false
				}
			}
			pm.save()
			return p.ID
		}
	}
	return ""
}

func testHTTPProxy(proxyURL string) (bool, string, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return false, "", err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	resp, err := client.Get("https://api.ipify.org?format=json")
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	var result struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", err
	}

	return true, result.IP, nil
}

// UploadOVPNFile загружает OVPN файл и добавляет его как прокси
func (pm *ProxyManager) UploadOVPNFile(file io.Reader, filename string) (string, string, error) {
	saveDir := "/opt/vpn/configs"
	os.MkdirAll(saveDir, 0755)
	savePath := filepath.Join(saveDir, filename)

	dst, err := os.Create(savePath)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		return "", "", err
	}

	id := pm.AddProxy(filename, "ovpn_file", savePath)
	return id, savePath, nil
}
