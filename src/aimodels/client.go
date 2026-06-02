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

// Usage — расход токенов одного вызова LLM (из поля usage ответа API).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat отправляет запрос к LLM и возвращает текст ответа и суммарное число токенов.
// Тонкая обёртка над ChatUsage — сохраняет историческую сигнатуру для всех вызовов.
func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (string, int, error) {
	answer, usage, _, err := c.ChatUsage(ctx, messages)
	return answer, usage.TotalTokens, err
}

// ChatUsage отправляет запрос к LLM и возвращает текст ответа, детальный расход
// токенов (prompt/completion/total) и длительность запроса.
func (c *Client) ChatUsage(ctx context.Context, messages []ChatMessage) (string, Usage, time.Duration, error) {
	start := time.Now()
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
		return "", Usage{}, time.Since(start), fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, time.Since(start), fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.model.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", Usage{}, time.Since(start), fmt.Errorf("HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", Usage{}, time.Since(start), fmt.Errorf("read response: %w", err)
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", Usage{}, time.Since(start), fmt.Errorf("unmarshal (status %d): %w\nbody: %s", resp.StatusCode, err, truncate(string(respBody), 300))
	}

	usage := Usage{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}

	if chatResp.Error != nil {
		return "", usage, time.Since(start), fmt.Errorf("API error %s: %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return "", usage, time.Since(start), fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	if len(chatResp.Choices) == 0 {
		return "", usage, time.Since(start), fmt.Errorf("пустой ответ от модели")
	}

	return chatResp.Choices[0].Message.Content, usage, time.Since(start), nil
}

// ChatWithAgent отправляет запрос к LLM с системным промптом агента. Это единая
// точка всех агентских вызовов — здесь же фиксируется расход токенов через
// usageRecorder (см. usage.go).
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
	answer, usage, dur, err := cl.ChatUsage(ctx, messages)
	recordUsage(ctx, m, a, usage, dur, err)
	return answer, usage.TotalTokens, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
