package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/common/model"
)

// PostgresNotificationStore хранит персональные уведомления резидентов
// (client_notifications) — inbox портала клиента.
type PostgresNotificationStore struct {
	db *pgxpool.Pool
}

// NewPostgresNotificationStore создаёт хранилище уведомлений.
func NewPostgresNotificationStore(db *pgxpool.Pool) *PostgresNotificationStore {
	return &PostgresNotificationStore{db: db}
}

// Create добавляет уведомление. Дедуплицирует по (client_id, change_event_id):
// повторная попытка по тому же изменению ничего не создаёт. Возвращает ID нового
// уведомления и created=true, либо created=false, если это дубль.
func (s *PostgresNotificationStore) Create(ctx context.Context, n *model.ClientNotification) (id string, created bool, err error) {
	var changeEventID any
	if n.ChangeEventID != "" {
		changeEventID = n.ChangeEventID
	}
	err = s.db.QueryRow(ctx, `
INSERT INTO client_notifications (client_id, change_event_id, severity, title, body, url)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (client_id, change_event_id) DO NOTHING
RETURNING id`,
		n.ClientID, changeEventID, n.Severity, n.Title, n.Body, n.URL).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil // дубль — пропускаем
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// ListForClient возвращает уведомления клиента (свежие сверху).
func (s *PostgresNotificationStore) ListForClient(ctx context.Context, clientID string, limit int) ([]*model.ClientNotification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
SELECT id, client_id, COALESCE(change_event_id::text,''), severity, title, body, url,
       read, created_at, email_sent_at, tg_sent_at
FROM client_notifications WHERE client_id = $1 ORDER BY created_at DESC LIMIT $2`, clientID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.ClientNotification
	for rows.Next() {
		var n model.ClientNotification
		if err := rows.Scan(&n.ID, &n.ClientID, &n.ChangeEventID, &n.Severity, &n.Title,
			&n.Body, &n.URL, &n.Read, &n.CreatedAt, &n.EmailSentAt, &n.TGSentAt); err != nil {
			return nil, err
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

// CountUnread возвращает число непрочитанных уведомлений клиента.
func (s *PostgresNotificationStore) CountUnread(ctx context.Context, clientID string) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM client_notifications WHERE client_id = $1 AND read = FALSE`, clientID).Scan(&n)
	return n, err
}

// MarkRead помечает уведомление прочитанным (только для своего клиента).
func (s *PostgresNotificationStore) MarkRead(ctx context.Context, id, clientID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE client_notifications SET read = TRUE WHERE id = $1 AND client_id = $2`, id, clientID)
	return err
}

// MarkEmailSent фиксирует время отправки email по уведомлению.
func (s *PostgresNotificationStore) MarkEmailSent(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE client_notifications SET email_sent_at = now() WHERE id = $1`, id)
	return err
}
