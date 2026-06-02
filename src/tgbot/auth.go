// auth.go — авторизация: привязка Telegram chat ID к клиенту по email.
//
// Привязка хранится в памяти конкретного бота (b.chatIDToClientID), а не в
// пакетной глобальной map — это обязательно для мультибота: у каждого тенанта
// свой экземпляр Bot со своими авторизациями, иначе чаты разных ботов
// «перемешались» бы. Поиск клиента ограничен тенантом бота (b.tenantID).
//
// В дальнейшем in-memory map можно заменить на таблицу telegram_bindings.
package tgbot

import (
	"context"
	"fmt"
	"strings"

	"baza-skolkovo/src/common/model"
)

// clientByEmailFinder — опциональный быстрый поиск клиента по email в рамках тенанта.
// *store.PostgresClientStore его реализует; иначе используется перебор ListClients.
type clientByEmailFinder interface {
	GetClientByEmail(ctx context.Context, tenantID, email string) (*model.Client, error)
}

// authorizeUser привязывает Telegram chat ID к клиенту по email в рамках тенанта бота.
// Возвращает clientID или ошибку, если клиент с таким email в тенанте не найден.
func (b *Bot) authorizeUser(chatID int64, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("email не указан")
	}

	client, err := b.findClientByEmail(email)
	if err != nil {
		return "", fmt.Errorf("клиент с email %s не найден", email)
	}

	b.authMutex.Lock()
	b.chatIDToClientID[chatID] = client.ID
	b.authMutex.Unlock()

	b.logf("авторизация: chat=%d → client=%s (email=%s)", chatID, client.ID, email)
	return client.ID, nil
}

// clientByChatID возвращает клиента по chat ID (должен быть авторизован).
func (b *Bot) clientByChatID(chatID int64) (*model.Client, error) {
	b.authMutex.RLock()
	clientID, exists := b.chatIDToClientID[chatID]
	b.authMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("чат %d не авторизован — введите /start", chatID)
	}

	client, err := b.stores.Client.GetClient(context.Background(), clientID)
	if err != nil {
		return nil, fmt.Errorf("клиент %s не найден: %w", clientID, err)
	}
	return client, nil
}

// isAuthorized сообщает, привязан ли chat ID к клиенту.
func (b *Bot) isAuthorized(chatID int64) bool {
	b.authMutex.RLock()
	_, exists := b.chatIDToClientID[chatID]
	b.authMutex.RUnlock()
	return exists
}

// deauthorize отвязывает Telegram chat ID от клиента.
func (b *Bot) deauthorize(chatID int64) {
	b.authMutex.Lock()
	delete(b.chatIDToClientID, chatID)
	b.authMutex.Unlock()
	b.logf("деавторизация: chat=%d", chatID)
}

// findClientByEmail ищет клиента по email в рамках тенанта бота.
// Использует быстрый GetClientByEmail, если хранилище его поддерживает;
// иначе перебирает ListClients(tenantID) — список уже ограничен тенантом.
func (b *Bot) findClientByEmail(email string) (*model.Client, error) {
	ctx := context.Background()

	if f, ok := b.stores.Client.(clientByEmailFinder); ok {
		c, err := f.GetClientByEmail(ctx, b.tenantID, email)
		if err != nil {
			return nil, err
		}
		return c, nil
	}

	clients, err := b.stores.Client.ListClients(ctx, b.tenantID, model.ResidencyStage(""))
	if err != nil {
		return nil, fmt.Errorf("список клиентов: %w", err)
	}
	for _, c := range clients {
		if strings.EqualFold(c.ContactEmail, email) {
			return c, nil
		}
	}
	return nil, fmt.Errorf("не найден клиент с email %s (проверено %d записей)", email, len(clients))
}
