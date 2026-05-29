package mcpserver

import (
	"sync"
	"time"
)

// limiter — простой потокобезопасный token-bucket с пополнением rps токенов в секунду.
type limiter struct {
	mu     sync.Mutex
	tokens float64
	max    float64
	rps    float64
	last   time.Time
}

func newLimiter(rps int) *limiter {
	r := float64(rps)
	if r <= 0 {
		r = 1
	}
	return &limiter{tokens: r, max: r, rps: r, last: time.Now()}
}

// allow возвращает true, если запрос можно пропустить (есть токен).
func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.tokens += now.Sub(l.last).Seconds() * l.rps
	if l.tokens > l.max {
		l.tokens = l.max
	}
	l.last = now
	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// limiterSet — набор лимитеров с ключом (например, по IP клиента).
type limiterSet struct {
	mu  sync.Mutex
	rps int
	m   map[string]*limiter
}

func newLimiterSet(rps int) *limiterSet {
	return &limiterSet{rps: rps, m: map[string]*limiter{}}
}

// allow проверяет лимит для конкретного ключа.
func (s *limiterSet) allow(key string) bool {
	s.mu.Lock()
	l, ok := s.m[key]
	if !ok {
		l = newLimiter(s.rps)
		s.m[key] = l
	}
	s.mu.Unlock()
	return l.allow()
}
