// Package tgbot — Telegram-бот для клиентов системы резидентства.
//
// Команды:
//
//	/start     — приветствие и привязка к клиенту по email
//	/status    — текущая стадия резидентства
//	/deadlines — ближайшие дедлайны
//	/docs      — мои документы
//	/ask       — вопрос через MCP ask_consultant
//	/help      — справка по командам
package tgbot

import (
	"context"
	"fmt"
	"log"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/agents"
	"baza-skolkovo/src/common/store"
)

// BotConfig — конфигурация Telegram-бота.
type BotConfig struct {
	// Token — токен Telegram-бота (получается у @BotFather).
	Token string
	// MCPURL — URL MCP-сервера для вызова инструментов (опционально, для MVP не требуется).
	MCPURL string
	// MCPAPIKey — API-ключ для MCP-сервера.
	MCPAPIKey string
}

// Stores — набор хранилищ, необходимых боту.
type Stores struct {
	Client    store.ClientStore
	Deadline  store.DeadlineStore
	DocLink   store.ClientDocumentStore
	Template  store.TemplateStore
	Checklist store.ChecklistStore
}

// Bot — Telegram-бот для системы резидентства.
type Bot struct {
	api        *tgbotapi.BotAPI
	stores     Stores
	config     BotConfig
	consultant *agents.ConsultantAgent

	// authMutex защищает map авторизаций.
	authMutex sync.RWMutex
	// chatIDToEmail map: Telegram chat ID → email клиента.
	chatIDToEmail map[int64]string
}

// NewBot создаёт новый экземпляр бота.
func NewBot(config BotConfig, stores Stores, consultant *agents.ConsultantAgent) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return nil, fmt.Errorf("инициализация Telegram Bot API: %w", err)
	}

	b := &Bot{
		api:           api,
		stores:        stores,
		config:        config,
		consultant:    consultant,
		chatIDToEmail: make(map[int64]string),
	}

	log.Printf("[tgbot] авторизован бот: %s (ID: %d)", api.Self.UserName, api.Self.ID)
	return b, nil
}

// Start запускает бота в режиме polling (блокирует вызывающую горутину).
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)
	b.runLoop(updates)
}

// Run запускает бота с поддержкой отмены через контекст.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	done := make(chan struct{})
	go func() {
		b.runLoop(updates)
		close(done)
	}()

	select {
	case <-ctx.Done():
		b.api.StopReceivingUpdates()
		return ctx.Err()
	case <-done:
		return nil
	}
}

// runLoop — основной цикл обработки обновлений.
func (b *Bot) runLoop(updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		b.handleUpdate(update)
	}
}

// handleUpdate маршрутизирует обновление к нужному обработчику.
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil && update.Message.IsCommand() {
		b.handleCommand(update)
		return
	}

	if update.Message != nil {
		b.handleMessage(update)
		return
	}

	if update.CallbackQuery != nil {
		b.handleCallback(update)
		return
	}
}

// sendReply отправляет текстовый ответ в чат.
func (b *Bot) sendReply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[tgbot] ошибка отправки сообщения chat=%d: %v", chatID, err)
	}
}

// sendReplyWithKeyboard отправляет сообщение с inline-клавиатурой.
func (b *Bot) sendReplyWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[tgbot] ошибка отправки сообщения с клавиатурой chat=%d: %v", chatID, err)
	}
}

// answerCallback отвечает на callback-запрос.
func (b *Bot) answerCallback(callbackID, text string) {
	cb := tgbotapi.NewCallback(callbackID, text)
	if _, err := b.api.Request(cb); err != nil {
		log.Printf("[tgbot] ошибка ответа на callback %s: %v", callbackID, err)
	}
}
