// reveal_store.go — одноразовое хранилище свежесгенерированных API-ключей.
//
// Ключи хранятся как hash+prefix (открытый текст в БД не остаётся), поэтому
// показать полный ключ можно только один раз — сразу после генерации. Хендлер
// кладёт открытый ключ сюда под случайный nonce и редиректит на страницу с
// ?reveal_nonce=<nonce>; страница забирает ключ (одноразово) и показывает.
// Сам ключ в URL не попадает — только короткоживущий одноразовый nonce.
package admin

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// revealTTL — сколько живёт показанный ключ до автоудаления.
const revealTTL = 10 * time.Minute

type revealItem struct {
	key     string
	expires time.Time
}

type revealStore struct {
	mu sync.Mutex
	m  map[string]revealItem
}

func newRevealStore() *revealStore {
	return &revealStore{m: make(map[string]revealItem)}
}

// put сохраняет открытый ключ и возвращает одноразовый nonce.
func (s *revealStore) put(key string) string {
	nonce := randomNonce()
	s.mu.Lock()
	s.pruneLocked()
	s.m[nonce] = revealItem{key: key, expires: time.Now().Add(revealTTL)}
	s.mu.Unlock()
	return nonce
}

// take возвращает ключ по nonce и удаляет его (одноразово). ok=false, если
// nonce не найден или истёк.
func (s *revealStore) take(nonce string) (string, bool) {
	if nonce == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	it, found := s.m[nonce]
	if !found {
		return "", false
	}
	delete(s.m, nonce)
	if time.Now().After(it.expires) {
		return "", false
	}
	return it.key, true
}

// pruneLocked удаляет протухшие записи (вызывать под mu).
func (s *revealStore) pruneLocked() {
	now := time.Now()
	for k, v := range s.m {
		if now.After(v.expires) {
			delete(s.m, k)
		}
	}
}

func randomNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return generateUUID()
	}
	return hex.EncodeToString(buf)
}

// keyReveals — общий процессный стор показанных ключей (тенанты и клиенты).
var keyReveals = newRevealStore()
