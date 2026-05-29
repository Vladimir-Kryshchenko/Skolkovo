// Package portal — личный кабинет клиента (аутентификация через magic link).
package portal

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// MagicToken — одноразовый токен для входа по ссылке.
type MagicToken struct {
	Email     string
	ClientID  string
	ExpiresAt time.Time
	Token     string
}

// Session — активная сессия клиента.
type Session struct {
	ClientID  string
	Email     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// magicStore хранит токены и сессии в памяти (MVP).
type magicStore struct {
	mu       sync.RWMutex
	tokens   map[string]*MagicToken // token -> MagicToken
	sessions map[string]*Session    // sessionID -> Session
}

func newMagicStore() *magicStore {
	return &magicStore{
		tokens:   make(map[string]*MagicToken),
		sessions: make(map[string]*Session),
	}
}

// GenerateMagicLink создаёт одноразовую ссылку для входа.
func (s *magicStore) GenerateMagicLink(email, clientID, baseURL string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	s.mu.Lock()
	s.tokens[token] = &MagicToken{
		Email:     email,
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Token:     token,
	}
	s.mu.Unlock()

	return baseURL + "/login/verify?token=" + token, nil
}

// VerifyMagicLink проверяет токен и возвращает clientID.
// После успешной верификации токен удаляется (одноразовый).
func (s *magicStore) VerifyMagicLink(token string) (clientID, email string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mt, ok := s.tokens[token]
	if !ok {
		return "", "", ErrInvalidToken
	}
	if time.Now().After(mt.ExpiresAt) {
		delete(s.tokens, token)
		return "", "", ErrTokenExpired
	}

	delete(s.tokens, token) // одноразовый
	return mt.ClientID, mt.Email, nil
}

// CreateSession создаёт новую сессию и возвращает sessionID.
func (s *magicStore) CreateSession(clientID, email string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	sessionID := hex.EncodeToString(b)

	s.mu.Lock()
	s.sessions[sessionID] = &Session{
		ClientID:  clientID,
		Email:     email,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	s.mu.Unlock()

	return sessionID
}

// GetSession возвращает сессию по идентификатору.
func (s *magicStore) GetSession(sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return sess, nil
}

// DeleteSession удаляет сессию (выход).
func (s *magicStore) DeleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// Cleanup удаляет просроченные токены и сессии.
func (s *magicStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, t := range s.tokens {
		if now.After(t.ExpiresAt) {
			delete(s.tokens, k)
		}
	}
	for k, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, k)
		}
	}
}

var (
	ErrInvalidToken   = &authError{"invalid_token", "недействительный или уже использованный токен"}
	ErrTokenExpired   = &authError{"token_expired", "срок действия ссылки истёк"}
	ErrSessionNotFound = &authError{"session_not_found", "сессия не найдена"}
	ErrSessionExpired = &authError{"session_expired", "сессия истекла, войдите заново"}
)

type authError struct {
	code, msg string
}

func (e *authError) Error() string { return e.msg }
