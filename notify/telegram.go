// Package notify — дополнение: прямые Telegram-уведомления консультанту.
// Отправляет алерты в личный чат консультанта (не клиентский бот)
// когда меняются важные документы, приближаются дедлайны или возникают эскалации.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TelegramAlert — уведомление в Telegram.
type TelegramAlert struct {
	Type     string    // "doc_changed" | "deadline" | "escalation" | "new_npa" | "new_contest"
	Severity string    // "info" | "warning" | "critical"
	Title    string
	Body     string
	ClientID string    // если относится к конкретному клиенту
	URL      string    // ссылка для перехода
	At       time.Time
}

// TelegramNotifier отправляет уведомления через Telegram Bot API
// в личный чат консультанта.
type TelegramNotifier struct {
	BotToken string // токен бота (TELEGRAM_BOT_TOKEN)
	ChatID   int64  // ID чата консультанта (CONSULTANT_TELEGRAM_CHAT_ID)
	http     *http.Client
	enabled  bool
}

// NewTelegramNotifier создаёт нотификатор для консультанта.
// Если botToken или chatID пусты — нотификатор работает в режиме no-op.
func NewTelegramNotifier(botToken string, chatID int64) *TelegramNotifier {
	enabled := botToken != "" && chatID != 0
	return &TelegramNotifier{
		BotToken: botToken,
		ChatID:   chatID,
		http:     &http.Client{Timeout: 15 * time.Second},
		enabled:  enabled,
	}
}

// Enabled возвращает true если нотификатор настроен.
func (t *TelegramNotifier) Enabled() bool { return t.enabled }

// Send отправляет алерт консультанту.
func (t *TelegramNotifier) Send(ctx context.Context, alert TelegramAlert) error {
	if !t.enabled {
		return nil
	}
	text := formatAlertMessage(alert)
	return t.sendMessage(ctx, text)
}

// SendDocumentChanged уведомляет об изменении документа.
func (t *TelegramNotifier) SendDocumentChanged(ctx context.Context, docTitle, category, changeType string) error {
	if !t.enabled {
		return nil
	}
	emoji := "📄"
	if changeType == "updated" {
		emoji = "✏️"
	} else if changeType == "outdated" {
		emoji = "⚠️"
	} else if changeType == "new" {
		emoji = "🆕"
	}

	text := fmt.Sprintf("%s *Изменение документа*\n\n"+
		"*Документ:* %s\n"+
		"*Категория:* %s\n"+
		"*Тип изменения:* %s\n"+
		"*Время:* %s",
		emoji,
		escapeMarkdown(docTitle),
		escapeMarkdown(category),
		changeType,
		time.Now().Format("02.01.2006 15:04"),
	)
	return t.sendMessage(ctx, text)
}

// SendDeadlineAlert уведомляет о приближающемся дедлайне клиента.
func (t *TelegramNotifier) SendDeadlineAlert(ctx context.Context, clientName, clientID, deadlineTitle string, dueDate time.Time, daysLeft int) error {
	if !t.enabled {
		return nil
	}

	emoji := "🟡"
	urgency := "приближается"
	if daysLeft <= 0 {
		emoji = "🔴"
		urgency = fmt.Sprintf("ПРОСРОЧЕН на %d дн.", -daysLeft)
	} else if daysLeft <= 3 {
		emoji = "🔴"
		urgency = fmt.Sprintf("через %d дн.", daysLeft)
	} else if daysLeft <= 7 {
		emoji = "🟠"
		urgency = fmt.Sprintf("через %d дн.", daysLeft)
	} else {
		urgency = fmt.Sprintf("через %d дн.", daysLeft)
	}

	text := fmt.Sprintf("%s *Дедлайн клиента*\n\n"+
		"*Клиент:* %s\n"+
		"*Дедлайн:* %s\n"+
		"*Статус:* %s\n"+
		"*Дата:* %s\n"+
		"*ID:* `%s`",
		emoji,
		escapeMarkdown(clientName),
		escapeMarkdown(deadlineTitle),
		urgency,
		dueDate.Format("02.01.2006"),
		clientID,
	)
	return t.sendMessage(ctx, text)
}

// SendEscalation уведомляет о застрявшем клиенте или критической ситуации.
func (t *TelegramNotifier) SendEscalation(ctx context.Context, clientName, clientID, message string, daysStuck int) error {
	if !t.enabled {
		return nil
	}

	emoji := "🚨"
	text := fmt.Sprintf("%s *Эскалация*\n\n"+
		"*Клиент:* %s\n"+
		"*ID:* `%s`\n"+
		"*Ситуация:* %s\n"+
		"*Без движения:* %d дней\n"+
		"*Время:* %s",
		emoji,
		escapeMarkdown(clientName),
		clientID,
		escapeMarkdown(message),
		daysStuck,
		time.Now().Format("02.01.2006 15:04"),
	)
	return t.sendMessage(ctx, text)
}

// SendNewNPA уведомляет о новом нормативном акте.
func (t *TelegramNotifier) SendNewNPA(ctx context.Context, npaTitle, sourceURL string) error {
	if !t.enabled {
		return nil
	}
	text := fmt.Sprintf("⚖️ *Новый НПА*\n\n"+
		"*Документ:* %s\n"+
		"*Время:* %s",
		escapeMarkdown(npaTitle),
		time.Now().Format("02.01.2006 15:04"),
	)
	if sourceURL != "" {
		text += fmt.Sprintf("\n[Открыть](%s)", sourceURL)
	}
	return t.sendMessage(ctx, text)
}

// SendNewContest уведомляет о новом конкурсе/гранте.
func (t *TelegramNotifier) SendNewContest(ctx context.Context, contestTitle, sourceURL string) error {
	if !t.enabled {
		return nil
	}
	text := fmt.Sprintf("🏆 *Новый конкурс/грант*\n\n"+
		"*Название:* %s\n"+
		"*Время:* %s",
		escapeMarkdown(contestTitle),
		time.Now().Format("02.01.2006 15:04"),
	)
	if sourceURL != "" {
		text += fmt.Sprintf("\n[Открыть](%s)", sourceURL)
	}
	return t.sendMessage(ctx, text)
}

// SendDailySummary отправляет ежедневную сводку по всем клиентам.
func (t *TelegramNotifier) SendDailySummary(ctx context.Context, summary DailySummary) error {
	if !t.enabled {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("📊 *Ежедневная сводка*\n\n")
	sb.WriteString(fmt.Sprintf("🗓 %s\n\n", time.Now().Format("02.01.2006")))

	if summary.OverdueCount > 0 {
		sb.WriteString(fmt.Sprintf("🔴 Просроченных дедлайнов: *%d*\n", summary.OverdueCount))
	}
	if summary.UrgentCount > 0 {
		sb.WriteString(fmt.Sprintf("🟠 Срочных дедлайнов (≤3 дн.): *%d*\n", summary.UrgentCount))
	}
	if summary.StuckCount > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ Клиентов без движения (>14 дн.): *%d*\n", summary.StuckCount))
	}
	if summary.ActiveClients > 0 {
		sb.WriteString(fmt.Sprintf("✅ Всего активных клиентов: *%d*\n", summary.ActiveClients))
	}
	if summary.ChangedDocs > 0 {
		sb.WriteString(fmt.Sprintf("📄 Изменений документов за сутки: *%d*\n", summary.ChangedDocs))
	}
	if summary.NewContests > 0 {
		sb.WriteString(fmt.Sprintf("🏆 Новых конкурсов/грантов: *%d*\n", summary.NewContests))
	}

	if len(summary.UrgentClients) > 0 {
		sb.WriteString("\n*Требуют внимания:*\n")
		for _, cl := range summary.UrgentClients {
			sb.WriteString(fmt.Sprintf("• %s — %s\n",
				escapeMarkdown(cl.Name), escapeMarkdown(cl.Reason)))
		}
	}

	return t.sendMessage(ctx, sb.String())
}

// DailySummary — данные для ежедневной сводки.
type DailySummary struct {
	ActiveClients int
	OverdueCount  int
	UrgentCount   int
	StuckCount    int
	ChangedDocs   int
	NewContests   int
	UrgentClients []UrgentClientInfo
}

// UrgentClientInfo — краткая информация о клиенте, требующем внимания.
type UrgentClientInfo struct {
	Name   string
	ID     string
	Reason string
}

// sendMessage отправляет текстовое сообщение через Telegram Bot API.
func (t *TelegramNotifier) sendMessage(ctx context.Context, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)

	payload := map[string]interface{}{
		"chat_id":    t.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
		// Отключаем предпросмотр ссылок чтобы не загромождать чат.
		"disable_web_page_preview": true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("Telegram API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result struct {
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("Telegram API: статус %d — %s", resp.StatusCode, result.Description)
	}

	return nil
}

// formatAlertMessage форматирует алерт в Markdown-сообщение.
func formatAlertMessage(a TelegramAlert) string {
	emoji := alertEmoji(a.Type, a.Severity)
	var sb strings.Builder

	sb.WriteString(emoji)
	sb.WriteString(" *")
	sb.WriteString(escapeMarkdown(a.Title))
	sb.WriteString("*\n\n")

	if a.Body != "" {
		sb.WriteString(escapeMarkdown(a.Body))
		sb.WriteString("\n")
	}
	if a.ClientID != "" {
		sb.WriteString(fmt.Sprintf("*Клиент:* `%s`\n", a.ClientID))
	}
	sb.WriteString(fmt.Sprintf("*Время:* %s", a.At.Format("02.01.2006 15:04")))
	if a.URL != "" {
		sb.WriteString(fmt.Sprintf("\n[Подробнее](%s)", a.URL))
	}

	return sb.String()
}

func alertEmoji(alertType, severity string) string {
	if severity == "critical" {
		return "🚨"
	}
	switch alertType {
	case "doc_changed":
		return "📄"
	case "deadline":
		return "⏰"
	case "escalation":
		return "🚨"
	case "new_npa":
		return "⚖️"
	case "new_contest":
		return "🏆"
	default:
		return "ℹ️"
	}
}

// escapeMarkdown экранирует спецсимволы Markdown v1 для Telegram.
func escapeMarkdown(s string) string {
	// В Markdown v1 экранируем только критичные символы.
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "`", "\\`")
	return s
}
