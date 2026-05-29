// auth.go — авторизация: привязка Telegram chat ID к клиенту по email.
//
// Для MVP хранится в памяти (map[int64]string: chatID → clientID).
// В дальнейшем можно заменить на БД (таблица telegram_bindings).
package tgbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

var (
	// chatIDToClientID хранит привязку Telegram chat ID к clientID.
	chatIDToClientID = make(map[int64]string)
	authMu           sync.RWMutex
)

// AuthorizeUser привязывает Telegram chat ID к клиенту по email.
//
// Алгоритм:
//  1. Ищем клиента по email (ContactEmail) через перебор ListClients.
//  2. Если найден — сохраняем привязку chatID → clientID.
//  3. Возвращаем clientID или ошибку.
//
// Для MVP используется in-memory map. В продакшене:
//
//	— добавить метод GetClientByEmail в store.ClientStore;
//	— хранить привязку в БД.
func AuthorizeUser(clientStore store.ClientStore, chatID int64, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("email не указан")
	}

	clientID, err := findClientIDByEmail(clientStore, email)
	if err != nil {
		return "", fmt.Errorf("клиент с email %s не найден", email)
	}

	authMu.Lock()
	chatIDToClientID[chatID] = clientID
	authMu.Unlock()

	log.Printf("[tgbot] авторизация: chat=%d → client=%s (email=%s)", chatID, clientID, email)
	return clientID, nil
}

// GetClientByChatID возвращает клиента по chat ID.
func GetClientByChatID(clientStore store.ClientStore, chatID int64) (*model.Client, error) {
	authMu.RLock()
	clientID, exists := chatIDToClientID[chatID]
	authMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("чат %d не авторизован — введите /start", chatID)
	}

	ctx := context.Background()
	client, err := clientStore.GetClient(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("клиент %s не найден: %w", clientID, err)
	}
	return client, nil
}

// DeauthorizeUser отвязывает Telegram chat ID от клиента.
func DeauthorizeUser(chatID int64) {
	authMu.Lock()
	delete(chatIDToClientID, chatID)
	authMu.Unlock()

	log.Printf("[tgbot] деавторизация: chat=%d", chatID)
}

// findClientIDByEmail ищет clientID по email через ListClients.
//
// Для MVP перебирает всех клиентов — в продакшене заменить на
// GetClientByEmail в ClientStore или индекс по ContactEmail.
func findClientIDByEmail(clientStore store.ClientStore, email string) (string, error) {
	ctx := context.Background()

	// Перебираем все тенанты (пустой tenantID вернёт всех клиентов).
	// Для MVP это допустимо — в реальной системе будет GetClientByEmail.
	clients, err := clientStore.ListClients(ctx, "", model.ResidencyStage(""))
	if err != nil {
		return "", fmt.Errorf("список клиентов: %w", err)
	}

	for _, c := range clients {
		if strings.EqualFold(c.ContactEmail, email) {
			return c.ID, nil
		}
	}

	return "", fmt.Errorf("не найден клиент с email %s (проверено %d записей)", email, len(clients))
}
