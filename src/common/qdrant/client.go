// Package qdrant — тонкий REST-клиент к векторной БД Qdrant.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client обращается к Qdrant по HTTP REST API.
type Client struct {
	BaseURL    string
	Collection string
	HTTP       *http.Client
}

// New создаёт клиент Qdrant.
func New(baseURL, collection string) *Client {
	return &Client{
		BaseURL:    baseURL,
		Collection: collection,
		HTTP:       &http.Client{Timeout: 30 * time.Second},
	}
}

// EnsureCollection создаёт коллекцию с косинусной метрикой, если её ещё нет.
// Если коллекция уже существует, её размерность сверяется с dim: при несовпадении
// возвращается явная ошибка (молчаливый upsert/поиск в коллекции несовместимой
// размерности приводит к битому поиску — например, после смены модели эмбеддингов).
func (c *Client) EnsureCollection(ctx context.Context, dim int) error {
	exists, size, err := c.collectionVectorSize(ctx)
	if err != nil {
		return err
	}
	if exists {
		if size > 0 && size != dim {
			return fmt.Errorf("коллекция %q уже существует с размерностью вектора %d, "+
				"а текущая конфигурация ожидает %d — сменилась модель эмбеддингов? "+
				"Пересоздайте коллекцию под новую размерность или верните прежний EMBEDDING_DIM",
				c.Collection, size, dim)
		}
		return nil
	}
	body := map[string]any{
		"vectors": map[string]any{"size": dim, "distance": "Cosine"},
	}
	return c.do(ctx, http.MethodPut, "/collections/"+c.Collection, body, nil)
}

// collectionVectorSize сообщает, существует ли коллекция и какова размерность её
// (безымянного) вектора. size==0 означает «существует, но размер определить не
// удалось» (например, именованные векторы) — в этом случае сверка не выполняется.
func (c *Client) collectionVectorSize(ctx context.Context) (exists bool, size int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/collections/"+c.Collection, nil)
	if err != nil {
		return false, 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		io.Copy(io.Discard, resp.Body)
		return false, 0, nil
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return false, 0, fmt.Errorf("проверка коллекции %q: статус %d", c.Collection, resp.StatusCode)
	}
	var info struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size int `json:"size"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		// Коллекция есть, но ответ не распарсился — не блокируем (размер неизвестен).
		return true, 0, nil
	}
	return true, info.Result.Config.Params.Vectors.Size, nil
}

// Point — точка для upsert.
type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

// Upsert добавляет/обновляет точки в коллекции.
func (c *Client) Upsert(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	body := map[string]any{"points": points}
	return c.do(ctx, http.MethodPut, "/collections/"+c.Collection+"/points?wait=true", body, nil)
}

// SearchHit — результат поиска.
type SearchHit struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
}

type searchResponse struct {
	Result []SearchHit `json:"result"`
}

// Search ищет ближайшие векторы. filter — необязательный фильтр Qdrant по payload.
func (c *Client) Search(ctx context.Context, vector []float32, limit int, filter map[string]any) ([]SearchHit, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if filter != nil {
		body["filter"] = filter
	}
	var out searchResponse
	if err := c.do(ctx, http.MethodPost, "/collections/"+c.Collection+"/points/search", body, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

// DeleteByDocument удаляет все точки документа (payload.document_id == docID).
func (c *Client) DeleteByDocument(ctx context.Context, docID string) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []any{
				map[string]any{"key": "document_id", "match": map[string]any{"value": docID}},
			},
		},
	}
	return c.do(ctx, http.MethodPost, "/collections/"+c.Collection+"/points/delete?wait=true", body, nil)
}

// ScrollPoint — точка, возвращаемая методом Scroll.
type ScrollPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector,omitempty"`
	Payload map[string]any `json:"payload"`
}

type scrollResponse struct {
	Result struct {
		Points   []ScrollPoint `json:"points"`
		NextPage *string       `json:"next_page_offset,omitempty"`
	} `json:"result"`
}

// Scroll постранично читает точки из коллекции с фильтром.
// limit — сколько точек за раз (0 = по умолчанию 10), offset — с какой точки начать ("", или значение из NextPage).
// Возвращает точки и следующий offset (nil если страниц больше нет).
func (c *Client) Scroll(ctx context.Context, limit int, offset *string, filter map[string]any, withVector bool) ([]ScrollPoint, *string, error) {
	body := map[string]any{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  withVector,
	}
	if offset != nil && *offset != "" {
		body["offset"] = *offset
	}
	if filter != nil {
		body["filter"] = filter
	}
	var out scrollResponse
	if err := c.do(ctx, http.MethodPost, "/collections/"+c.Collection+"/points/scroll", body, &out); err != nil {
		return nil, nil, err
	}
	return out.Result.Points, out.Result.NextPage, nil
}

// ScrollByDocument получает все точки документа через Scroll API.
func (c *Client) ScrollByDocument(ctx context.Context, docID string) ([]ScrollPoint, error) {
	filter := map[string]any{
		"must": []any{
			map[string]any{"key": "document_id", "match": map[string]any{"value": docID}},
		},
	}

	var allPoints []ScrollPoint
	var offset *string
	for {
		points, nextPage, err := c.Scroll(ctx, 100, offset, filter, false)
		if err != nil {
			return nil, err
		}
		allPoints = append(allPoints, points...)
		if nextPage == nil {
			break
		}
		offset = nextPage
	}
	return allPoints, nil
}

// FilterActive — фильтр Qdrant: только действующие документы.
func FilterActive() map[string]any {
	return map[string]any{
		"must": []any{
			map[string]any{"key": "status", "match": map[string]any{"value": "действует"}},
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant: статус %d: %s", resp.StatusCode, string(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
