// notifier.go — ежедневный дайджест дедлайнов для всех авторизованных клиентов.
//
// Горутина запускается вместе с ботом и каждый день в 09:00 (UTC+3) рассылает
// каждому авторизованному пользователю список дедлайнов на ближайшие 7 дней.
// Если дедлайнов нет — сообщение не отправляется.
package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
)

const (
	notifyHour     = 9  // час отправки (UTC+3 = UTC+3; сервер должен учитывать TZ)
	notifyDaysAhead = 7 // горизонт дедлайнов для дайджеста
	notifyTZ        = 3 // UTC+3 (Москва)
)

// RunNotifier запускает фоновую горутину ежедневного дайджеста дедлайнов.
// Блокирует до отмены ctx.
func (b *Bot) RunNotifier(ctx context.Context) {
	for {
		next := nextNotifyTime()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
			b.sendDailyDigest(ctx)
		}
	}
}

// nextNotifyTime вычисляет следующее время отправки дайджеста (09:00 МСК).
func nextNotifyTime() time.Time {
	loc := time.FixedZone("MSK", notifyTZ*3600)
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), notifyHour, 0, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// sendDailyDigest рассылает дайджест всем авторизованным пользователям бота.
func (b *Bot) sendDailyDigest(ctx context.Context) {
	b.authMutex.RLock()
	snapshot := make(map[int64]string, len(b.chatIDToClientID))
	for chatID, clientID := range b.chatIDToClientID {
		snapshot[chatID] = clientID
	}
	b.authMutex.RUnlock()

	for chatID, clientID := range snapshot {
		select {
		case <-ctx.Done():
			return
		default:
		}
		b.sendDeadlineDigest(ctx, chatID, clientID)
	}
}

// sendDeadlineDigest формирует и отправляет сообщение с дедлайнами клиента.
func (b *Bot) sendDeadlineDigest(ctx context.Context, chatID int64, clientID string) {
	deadlines, err := b.stores.Deadline.ListDeadlines(ctx, clientID, notifyDaysAhead)
	if err != nil || len(deadlines) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 *Дайджест дедлайнов на %d дней*\n\n", notifyDaysAhead))

	for _, d := range deadlines {
		emoji := "🟡"
		if d.Status == model.DeadlineOverdue {
			emoji = "🔴"
		} else if daysLeft(d.DueDate) <= 2 {
			emoji = "🟠"
		}
		sb.WriteString(fmt.Sprintf(
			"%s *%s*\n   Срок: %s\n\n",
			emoji,
			d.Title,
			d.DueDate.Format("02.01.2006"),
		))
	}

	sb.WriteString("_Используйте /deadlines для полного списка._")
	b.sendReply(chatID, sb.String())
}

// daysLeft возвращает количество дней до даты (отрицательное — просрочено).
func daysLeft(t time.Time) int {
	return int(time.Until(t).Hours() / 24)
}
