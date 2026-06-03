// auth.go — авторизация: привязка Telegram chat ID к клиенту по email.
//
// Привязка хранится в памяти (b.chatIDToClientID) и дублируется в таблицу
// telegram_bindings (если хранилище реализует BindingStore). Это позволяет
// восстанавливать сессии после рестарта бота без повторной авторизации.
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

// BindingStore — опциональный интерфейс для персистентного хранения привязок.
// Реализуется *store.PostgresClientStore или отдельным репозиторием.
type BindingStore interface {
	SaveBinding(ctx context.Context, chatID int64, clientID, tenantID string) error
	DeleteBinding(ctx context.Context, chatID int64, tenantID string) error
	LoadBindings(ctx context.Context, tenantID string) (map[int64]string, error) // chatID → clientID
}

// loadBindings загружает сохранённые привязки из БД в память при старте бота.
// Вызывается из NewBot; если хранилище не поддерживает BindingStore — пропускается.
func (b *Bot) loadBindings() {
	bs, ok := b.stores.Client.(BindingStore)
	if !ok {
		return
	}
	bindings, err := bs.LoadBindings(context.Background(), b.tenantID)
	if err != nil {
		b.logf("не удалось загрузить привязки: %v", err)
		return
	}
	b.authMutex.Lock()
	for chatID, clientID := range bindings {
		b.chatIDToClientID[chatID] = clientID
	}
	b.authMutex.Unlock()
	b.logf("загружено %d привязок из БД", len(bindings))
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

	// Сохраняем в БД, чтобы пережить рестарт.
	if bs, ok := b.stores.Client.(BindingStore); ok {
		if err := bs.SaveBinding(context.Background(), chatID, client.ID, b.tenantID); err != nil {
			b.logf("не удалось сохранить привязку в БД: %v", err)
		}
	}

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

// deauthorize отвязывает Telegram chat ID от клиента (память + БД).
func (b *Bot) deauthorize(chatID int64) {
	b.authMutex.Lock()
	delete(b.chatIDToClientID, chatID)
	b.authMutex.Unlock()

	if bs, ok := b.stores.Client.(BindingStore); ok {
		if err := bs.DeleteBinding(context.Background(), chatID, b.tenantID); err != nil {
			b.logf("не удалось удалить привязку из БД: %v", err)
		}
	}
	b.logf("деавторизация: chat=%d", chatID)
}

// findClientByEmail ищет клиента по email в рамках тенанта бота.
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
