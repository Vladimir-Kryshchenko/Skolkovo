// Package mailer отправляет письма по SMTP (например, ссылки для входа в портал).
// Если SMTP не настроен, работает в режиме no-op — вызывающий код использует
// fallback (показ ссылки на странице в dev-режиме).
package mailer

import (
	"context"
	"fmt"
	"mime"
	"net/smtp"
	"strings"
	"time"
)

// Config — параметры SMTP.
type Config struct {
	Host     string // хост SMTP-сервера
	Port     int    // порт (587 STARTTLS, 465 implicit TLS, 25)
	Username string // логин (часто = From)
	Password string // пароль/токен
	From     string // адрес отправителя
}

// Mailer отправляет письма по SMTP.
type Mailer struct {
	cfg     Config
	enabled bool
}

// New создаёт mailer. Пустой Host или From → режим no-op.
func New(cfg Config) *Mailer {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &Mailer{
		cfg:     cfg,
		enabled: cfg.Host != "" && cfg.From != "",
	}
}

// Enabled сообщает, настроена ли отправка.
func (m *Mailer) Enabled() bool { return m.enabled }

// Send отправляет текстовое письмо (UTF-8). No-op, если mailer не настроен.
func (m *Mailer) Send(ctx context.Context, to, subject, body string) error {
	if !m.enabled {
		return nil
	}
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("mailer: пустой адрес получателя")
	}

	msg := buildMessage(m.cfg.From, to, subject, body)
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}

	// Отправляем в отдельной горутине-обёртке с учётом контекста: net/smtp
	// не принимает context, поэтому ограничиваем общим дедлайном вызывающего.
	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, m.cfg.From, []string{to}, msg)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("mailer: отправка на %s: %w", to, err)
		}
		return nil
	}
}

// buildMessage формирует RFC 5322 сообщение с UTF-8 заголовками.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", encodeHeader(subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// encodeHeader кодирует заголовок Subject в MIME-encoded-word (Base64 UTF-8),
// если он содержит не-ASCII символы.
func encodeHeader(s string) string {
	ascii := true
	for _, r := range s {
		if r > 127 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	return mime.BEncoding.Encode("UTF-8", s)
}
