package admin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DochubCookieStore хранит сессионную куку dochub (spid + AuthorizationCookie),
// скопированную из браузера, прошедшего WAF. С этой кукой HTTP-фетчер качает
// тела файлов без браузера и без прокси (см. fetcher.DownloadViaCookie).
// Кука периодически протухает — её обновляют в админке (страница «Прокси»).
// Персистится в JSON-файл рядом с прокси (доступ только владельцу — это
// секрет-сессия).
type DochubCookieStore struct {
	mu      sync.RWMutex
	path    string
	Cookie  string `json:"cookie"`
	SavedAt string `json:"saved_at,omitempty"`
}

// NewDochubCookieStore загружает куку из файла (если есть).
func NewDochubCookieStore(path string) *DochubCookieStore {
	s := &DochubCookieStore{path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	return s
}

// dochubCookieFile — стандартный путь файла куки внутри каталога документов.
func dochubCookieFile(docsDir string) string {
	return filepath.Join(docsDir, ".admin", "dochub_cookie.json")
}

// LoadDochubCookie читает сохранённую в админке куку (для планировщика serve,
// у которого нет ссылки на Server). "" — куки нет.
func LoadDochubCookie(docsDir string) string {
	return NewDochubCookieStore(dochubCookieFile(docsDir)).Get()
}

// Get возвращает текущую куку ("" — не задана).
func (s *DochubCookieStore) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Cookie
}

// Set сохраняет куку (с отметкой времени) и персистит на диск.
func (s *DochubCookieStore) Set(cookie string) error {
	cookie = strings.TrimSpace(cookie)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cookie = cookie
	s.SavedAt = time.Now().Format("2006-01-02 15:04")
	return s.persist()
}

// Masked возвращает безопасное для показа представление куки.
func (s *DochubCookieStore) Masked() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.Cookie
	if c == "" {
		return ""
	}
	if len(c) <= 24 {
		return "********"
	}
	return c[:12] + "…" + c[len(c)-8:]
}

// SavedAtStr — когда куку сохранили (для подсказки «не пора ли обновить»).
func (s *DochubCookieStore) SavedAtStr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SavedAt
}

func (s *DochubCookieStore) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
