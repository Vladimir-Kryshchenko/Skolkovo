// Package notify отправляет уведомления об изменениях в базе (webhook).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Event — полезная нагрузка уведомления.
type Event struct {
	Type      string    `json:"type"` // например "parsing_cycle"
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Details   any       `json:"details,omitempty"`
}

// Notifier отправляет события на webhook-URL. Если URL пуст — no-op.
type Notifier struct {
	WebhookURL string
	HTTP       *http.Client
}

// New создаёт нотификатор. Пустой url отключает отправку.
func New(webhookURL string) *Notifier {
	return &Notifier{
		WebhookURL: webhookURL,
		HTTP:       &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled сообщает, настроена ли отправка.
func (n *Notifier) Enabled() bool { return n.WebhookURL != "" }

// Send отправляет событие. Ошибки возвращаются, но не критичны для конвейера.
func (n *Notifier) Send(ctx context.Context, ev Event) error {
	if !n.Enabled() {
		return nil
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
