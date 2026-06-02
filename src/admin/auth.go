// Package admin — система аутентификации админки (сессии + роли).
package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// Role — роль пользователя. В админ-панель допускается только администратор;
// прочие пользователи работают с базой через MCP, Telegram-бот и ИИ-агентов.
type Role string

const (
	RoleAdmin Role = "admin"
)

// AdminUser — пользователь админки.
type AdminUser struct {
	Username     string
	PasswordHash string // SHA-256 хеш пароля
	DisplayName  string
	Role         Role
}

// Session — активная сессия администратора.
type AdminSession struct {
	Username  string
	Role      Role
	CreatedAt time.Time
	ExpiresAt time.Time
}

// adminAuthStore хранит пользователей и сессии в памяти.
type adminAuthStore struct {
	mu       sync.RWMutex
	users    map[string]*AdminUser    // username -> AdminUser
	sessions map[string]*AdminSession // sessionID -> AdminSession
}

func newAdminAuthStore() *adminAuthStore {
	return &adminAuthStore{
		users:    make(map[string]*AdminUser),
		sessions: make(map[string]*AdminSession),
	}
}

// AddUser добавляет пользователя (для инициализации).
func (s *adminAuthStore) AddUser(username, password, displayName string, role Role) {
	// Вычисляем SHA-256 хеш пароля
	hash := sha256Hash(password)

	s.mu.Lock()
	s.users[username] = &AdminUser{
		Username:     username,
		PasswordHash: hash,
		DisplayName:  displayName,
		Role:         role,
	}
	s.mu.Unlock()
}

// Authenticate проверяет логин/пароль и возвращает true если совпадают.
func (s *adminAuthStore) Authenticate(username, password string) (*AdminUser, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok {
		return nil, false
	}

	hash := sha256Hash(password)
	if subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(hash)) != 1 {
		return nil, false
	}

	return user, true
}

// CreateSession создаёт новую сессию и возвращает sessionID.
func (s *adminAuthStore) CreateSession(username string, role Role) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	sessionID := hex.EncodeToString(b)

	s.mu.Lock()
	s.sessions[sessionID] = &AdminSession{
		Username:  username,
		Role:      role,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	s.mu.Unlock()

	return sessionID
}

// GetSession возвращает сессию по идентификатору.
func (s *adminAuthStore) GetSession(sessionID string) (*AdminSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, errSessionNotFound
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, errSessionExpired
	}
	return sess, nil
}

// DeleteSession удаляет сессию (выход).
func (s *adminAuthStore) DeleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// Cleanup удаляет просроченные сессии.
func (s *adminAuthStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, k)
		}
	}
}

// sha256Hash вычисляет SHA-256 хеш строки.
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

var (
	errSessionNotFound = &adminAuthError{"session_not_found", "сессия не найдена"}
	errSessionExpired  = &adminAuthError{"session_expired", "сессия истекла, войдите заново"}
)

type adminAuthError struct {
	code, msg string
}

func (e *adminAuthError) Error() string { return e.msg }

// BasicAuth — простой HTTP Basic Auth middleware.
func BasicAuth(user, pass, realm string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
