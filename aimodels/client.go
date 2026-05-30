package aimodels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client — HTTP-клиент для OpenAI-совместимого LLM API.
type Client struct {
	model      Model
	httpClient *http.Client
}

// NewClient создаёт клиент для конкретной модели.
func NewClient(m Model) *Client {
	return &Client{
		model: m,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// Chat отправляет запрос к LLM и возвращает текст ответа.
func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (string, int, error) {
	baseURL := strings.TrimRight(c.model.BaseURL, "/")
	endpoint := baseURL + "/chat/completions"

	req := ChatRequest{
		Model:       c.model.ModelID,
		Messages:    messages,
		MaxTokens:   c.model.MaxTokens,
		Temperature: c.model.Temperature,
		Stream:      false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", 0, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.model.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read response: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", 0, fmt.Errorf("unmarshal (status %d): %w\nbody: %s", resp.StatusCode, err, truncate(string(respBody), 300))
	}

	if chatResp.Error != nil {
		return "", 0, fmt.Errorf("API error %s: %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return "", 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	if len(chatResp.Choices) == 0 {
		return "", 0, fmt.Errorf("пустой ответ от модели")
	}

	total := chatResp.Usage.TotalTokens
	return chatResp.Choices[0].Message.Content, total, nil
}

// ChatWithAgent отправляет запрос к LLM с системным промптом агента.
func ChatWithAgent(ctx context.Context, m Model, a Agent, userMessage string) (string, int, error) {
	var messages []ChatMessage
	if a.SystemPrompt != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: a.SystemPrompt})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: userMessage})

	// Переопределяем параметры модели настройками агента.
	override := m
	if a.Temperature > 0 {
		override.Temperature = a.Temperature
	}
	if a.MaxTokens > 0 {
		override.MaxTokens = a.MaxTokens
	}

	cl := NewClient(override)
	return cl.Chat(ctx, messages)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
