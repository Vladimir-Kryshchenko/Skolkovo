// Package embed предоставляет абстракцию вычисления эмбеддингов и клиент к TEI.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Embedder — провайдер эмбеддингов. Реализация подменяема (TEI, внешний API).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// TEIClient обращается к HuggingFace Text Embeddings Inference по HTTP.
type TEIClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewTEIClient создаёт клиент TEI с базовым URL (например, http://localhost:8081).
func NewTEIClient(baseURL string) *TEIClient {
	return &TEIClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Embed возвращает эмбеддинги для каждого текста.
//
// Модели семейства e5 требуют префиксов: "query: " для запросов и
// "passage: " для индексируемых фрагментов. Префикс задаётся вызывающим кодом.
func (c *TEIClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"inputs": texts})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tei: запрос не выполнен: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tei: статус %d", resp.StatusCode)
	}

	var out [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("tei: разбор ответа: %w", err)
	}
	return out, nil
}

// Префиксы для моделей e5.
const (
	PrefixQuery   = "query: "
	PrefixPassage = "passage: "
)
