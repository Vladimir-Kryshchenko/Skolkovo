// Package httpx — небольшие HTTP-утилиты, общие для парсеров источников.
//
// Главное здесь — GetWithRetry: источники Сколково (dochub.sk.ru на Telligent,
// частично sk.ru) отвечают НЕСТАБИЛЬНО — спорадически отдают 502/503 даже на
// валидные страницы (anti-bot/перегрузка). Краулер site_pages обходит это
// ретраями; парсеры events/contests/faq исторически делали один запрос и падали
// с первой же 502. Этот хелпер повторяет запрос на временных ошибках.
package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent — браузерный UA: часть страниц отдаётся только «браузерам».
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// maxBodyBytes — предел размера тела (страницы/ленты, не файлы).
const maxBodyBytes = 8 << 20 // 8 МБ

// GetWithRetry выполняет GET с повторами на временных сбоях (сеть, 5xx, 429).
// attempts — максимальное число попыток (<=0 → 4); между попытками — нарастающая
// задержка. Возвращает тело ответа при финальном 200 либо последнюю ошибку.
func GetWithRetry(ctx context.Context, hc *http.Client, rawURL, userAgent string, attempts int) ([]byte, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	if attempts <= 0 {
		attempts = 4
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// Нарастающая задержка: 1.5s, 3s, 4.5s… (источник «отдышится»).
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(i) * 1500 * time.Millisecond):
			}
		}

		body, retryable, err := getOnce(ctx, hc, rawURL, userAgent)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("после %d попыток: %w", attempts, lastErr)
}

// getOnce — один GET. retryable=true для временных сбоев (сеть, 5xx, 429).
func getOnce(ctx context.Context, hc *http.Client, rawURL, userAgent string) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en;q=0.8")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, true, err // сетевая ошибка/таймаут — повторяемо
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, fmt.Errorf("статус %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, true, err
	}
	return data, false, nil
}
