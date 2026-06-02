// key_cache.go — потокобезопасный кэш результатов проверки per-tenant API-ключей.
//
// middleware вызывается на каждый HTTP-запрос; без кэша каждый per-tenant ключ
// бил бы в БД (GetTenantByAPIKey). Кэшируем и положительные (ключ → tenantID),
// и отрицательные (ключ неизвестен/неактивен) результаты на короткий TTL —
// этого достаточно, чтобы отзыв/ротация ключа применялись быстро.
package mcpserver

import (
	"sync"
	"time"
)

type keyCacheEntry struct {
	tenantID string
	ok       bool
	expires  time.Time
}

// keyCache — кэш с TTL для результатов проверки API-ключей.
type keyCache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]keyCacheEntry
	// now подменяется в тестах; в проде — time.Now.
	now func() time.Time
}

func newKeyCache(ttl time.Duration) *keyCache {
	return &keyCache{
		ttl: ttl,
		m:   make(map[string]keyCacheEntry),
		now: time.Now,
	}
}

// get возвращает (tenantID, ok, hit). hit=false означает промах кэша
// (нет записи или истёк TTL) — вызывающий должен сходить в БД.
func (c *keyCache) get(key string) (tenantID string, ok bool, hit bool) {
	c.mu.RLock()
	e, found := c.m[key]
	c.mu.RUnlock()
	if !found || c.now().After(e.expires) {
		return "", false, false
	}
	return e.tenantID, e.ok, true
}

// set кладёт результат проверки ключа в кэш.
func (c *keyCache) set(key, tenantID string, ok bool) {
	c.mu.Lock()
	c.m[key] = keyCacheEntry{tenantID: tenantID, ok: ok, expires: c.now().Add(c.ttl)}
	c.mu.Unlock()
}

// invalidate удаляет ключ из кэша (например, после ротации/отзыва).
func (c *keyCache) invalidate(key string) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}
