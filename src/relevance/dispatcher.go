package relevance

import (
	"context"
	"fmt"
	"log"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/common/model"
)

// ClientLister отдаёт клиентов по стадии (реализуется store.PostgresClientStore).
// tenantID="" означает «по всем тенантам».
type ClientLister interface {
	ListClients(ctx context.Context, tenantID string, stage model.ResidencyStage) ([]*model.Client, error)
}

// NotificationSink создаёт персональные уведомления клиентов
// (реализуется store.PostgresNotificationStore).
type NotificationSink interface {
	Create(ctx context.Context, n *model.ClientNotification) (id string, created bool, err error)
	MarkEmailSent(ctx context.Context, id string) error
}

// MailSender отправляет email (реализуется *mailer.Mailer). Опционален.
type MailSender interface {
	Enabled() bool
	Send(ctx context.Context, to, subject, body string) error
}

// ConsultantAlerter уведомляет консультанта об изменении (реализуется
// *notify.TelegramNotifier). Опционален.
type ConsultantAlerter interface {
	Enabled() bool
	SendDocumentChangeAlert(ctx context.Context, title, category, severity string, affected int, summary, url string) error
}

// Dispatcher рассылает уведомления о значимом изменении: персональные клиентам
// затронутых стадий (inbox + email при critical) и алерт консультанту.
type Dispatcher struct {
	Clients ClientLister
	Notifs  NotificationSink
	Mail    MailSender        // опционален
	TG      ConsultantAlerter // опционален
}

// NewDispatcher создаёт диспетчер уведомлений.
func NewDispatcher(clients ClientLister, notifs NotificationSink, mail MailSender, tg ConsultantAlerter) *Dispatcher {
	return &Dispatcher{Clients: clients, Notifs: notifs, Mail: mail, TG: tg}
}

// Dispatch рассылает уведомления по результату анализа. Информационные изменения
// (severity=info) не порождают персональных уведомлений и алертов — они остаются
// в ленте изменений (get_recent_changes) и на дашборде. Возвращает число
// затронутых клиентов, которым создано уведомление.
func (d *Dispatcher) Dispatch(ctx context.Context, ev changes.Event, res Result) (int, error) {
	if changes.SeverityRank(res.Severity) < changes.SeverityRank(changes.SeverityWarning) {
		return 0, nil
	}

	affected := 0
	if d.Clients != nil && d.Notifs != nil {
		seen := make(map[string]bool)
		for _, st := range res.Stages {
			clients, err := d.Clients.ListClients(ctx, "", st)
			if err != nil {
				log.Printf("[relevance] clients[%s]: %v", st, err)
				continue
			}
			for _, c := range clients {
				if seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				if d.notifyClient(ctx, ev, res, c) {
					affected++
				}
			}
		}
	}

	if d.TG != nil && d.TG.Enabled() {
		if err := d.TG.SendDocumentChangeAlert(ctx, ev.Title, ev.Category,
			string(res.Severity), affected, res.Summary, ev.SourceURL); err != nil {
			log.Printf("[relevance] consultant alert: %v", err)
		}
	}
	return affected, nil
}

// notifyClient создаёт inbox-уведомление и (для critical) шлёт email.
// Возвращает true, если уведомление было создано (а не дубль).
func (d *Dispatcher) notifyClient(ctx context.Context, ev changes.Event, res Result, c *model.Client) bool {
	id, created, err := d.Notifs.Create(ctx, &model.ClientNotification{
		ClientID:      c.ID,
		ChangeEventID: ev.ID,
		Severity:      string(res.Severity),
		Title:         ev.Title,
		Body:          res.Summary,
		URL:           ev.SourceURL,
	})
	if err != nil {
		log.Printf("[relevance] notify client %s: %v", c.ID, err)
		return false
	}
	if !created {
		return false
	}

	if res.Severity == changes.SeverityCritical && d.Mail != nil && d.Mail.Enabled() && c.ContactEmail != "" {
		subject := "[Сколково] Важное изменение: " + ev.Title
		body := emailBody(ev, res)
		if err := d.Mail.Send(ctx, c.ContactEmail, subject, body); err != nil {
			log.Printf("[relevance] email %s: %v", c.ContactEmail, err)
		} else if err := d.Notifs.MarkEmailSent(ctx, id); err != nil {
			log.Printf("[relevance] mark email sent %s: %v", id, err)
		}
	}
	return true
}

func emailBody(ev changes.Event, res Result) string {
	body := fmt.Sprintf("Произошло важное изменение в документации Сколково.\n\nДокумент: %s\n", ev.Title)
	if ev.Category != "" {
		body += "Категория: " + ev.Category + "\n"
	}
	body += "\n" + res.Summary + "\n"
	if ev.SourceURL != "" {
		body += "\nИсточник: " + ev.SourceURL + "\n"
	}
	body += "\nПодробности — в вашем личном кабинете резидента."
	return body
}
