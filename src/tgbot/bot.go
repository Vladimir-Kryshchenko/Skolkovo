// Package tgbot — публичный информационный Telegram-бот «База Сколково».
//
// Бот анонимный: не требует входа в аккаунт и email. Он даёт доступ к публичной
// базе знаний Фонда «Сколково» через ИИ-консультанта и тематические разделы.
//
// Команды:
//
//	/start    — приветствие и меню
//	/ask      — вопрос ИИ-консультанту (или просто напишите вопрос текстом)
//	/events   — ближайшие мероприятия
//	/contests — открытые конкурсы и гранты
//	/news     — свежие новости Фонда
//	/faq      — частые вопросы
//	/company  — опционально указать ИНН для персонализации ответов
//	/help     — справка
package tgbot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/agents"
	"baza-skolkovo/src/sitepages"
	"baza-skolkovo/src/skru"
)

// BotConfig — конфигурация Telegram-бота.
type BotConfig struct {
	// Token — токен Telegram-бота (получается у @BotFather).
	Token string
	// TenantID — тенант, которому принадлежит бот (для учёта расхода ИИ-токенов).
	// Пусто — глобальный бот.
	TenantID string
	// MCPURL — URL MCP-сервера (зарезервировано для будущих интеграций).
	MCPURL string
	// MCPAPIKey — API-ключ для MCP-сервера.
	MCPAPIKey string
}

// Stores — набор хранилищ, необходимых информационному боту.
// Разделы /news, /events, /contests, /faq наполняются из site_pages — отдельного
// слоя страниц публичного сайта sk.ru/dochub (структурные парсеры sk.ru блокируются
// anti-bot защитой, а краулер site_pages успешно собирает контент).
// Все опциональны: если стор nil, раздел сообщает о недоступности.
type Stores struct {
	// Pages — листинг страниц сайта по разделу/дате (Postgres, без эмбеддингов).
	Pages *sitepages.PostgresStore
	// PageSearch — семантический поиск по страницам сайта (Qdrant + TEI).
	PageSearch *sitepages.Searcher
	// SkRu — клиент актуальных публикаций sk.ru (новости/мероприятия/конкурсы).
	SkRu *skru.Client
}

// Bot — публичный информационный Telegram-бот.
type Bot struct {
	api        *tgbotapi.BotAPI
	stores     Stores
	config     BotConfig
	consultant *agents.ConsultantAgent
	// tenantID — тенант бота (для атрибуции расхода ИИ-токенов).
	tenantID string

	// askMu защищает askLastTime.
	askMu sync.Mutex
	// askLastTime хранит время последнего вопроса по chat ID (анти-флуд).
	askLastTime map[int64]time.Time
}

// NewBot создаёт новый экземпляр информационного бота.
func NewBot(config BotConfig, stores Stores, consultant *agents.ConsultantAgent) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return nil, fmt.Errorf("инициализация Telegram Bot API: %w", err)
	}

	b := &Bot{
		api:         api,
		stores:      stores,
		config:      config,
		consultant:  consultant,
		tenantID:    config.TenantID,
		askLastTime: make(map[int64]time.Time),
	}

	b.registerCommands()
	log.Printf("[tgbot] авторизован бот: %s (ID: %d, tenant=%s)", api.Self.UserName, api.Self.ID, orNone(config.TenantID))
	return b, nil
}

// registerCommands регистрирует список команд в Telegram (отображается в меню «/»).
// Emoji в Description показываются прямо в списке команд — это максимум возможного
// в Bot API (изображения на кнопки команд платформа не поддерживает).
func (b *Bot) registerCommands() {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "🚀 Начать работу и открыть меню"},
		{Command: "ask", Description: "🤖 Спросить ИИ-консультанта"},
		{Command: "events", Description: "📅 Ближайшие мероприятия"},
		{Command: "contests", Description: "🏆 Конкурсы и гранты"},
		{Command: "news", Description: "📰 Свежие новости Фонда"},
		{Command: "faq", Description: "❓ Частые вопросы"},
		{Command: "help", Description: "ℹ️ Справка по командам"},
	}
	cfg := tgbotapi.NewSetMyCommands(commands...)
	if _, err := b.api.Request(cfg); err != nil {
		log.Printf("[tgbot] не удалось зарегистрировать команды: %v", err)
	}
}

// logf пишет в лог с префиксом тенанта бота.
func (b *Bot) logf(format string, args ...any) {
	log.Printf("[tgbot tenant=%s] "+format, append([]any{orNone(b.tenantID)}, args...)...)
}

// orNone возвращает "—" для пустой строки (для читабельности логов).
func orNone(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// Username возвращает @username бота (как сообщил Telegram при авторизации).
func (b *Bot) Username() string {
	return b.api.Self.UserName
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
	msg.DisableWebPagePreview = true
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[tgbot] ошибка отправки сообщения chat=%d: %v", chatID, err)
	}
}

// sendReplyWithMenuKeyboard отправляет сообщение с постоянной reply-клавиатурой (нижнее меню).
func (b *Bot) sendReplyWithMenuKeyboard(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
	kb := ReplyMenuKeyboard()
	kb.ResizeKeyboard = true
	msg.ReplyMarkup = kb
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[tgbot] ошибка отправки с menu-клавиатурой chat=%d: %v", chatID, err)
	}
}

// sendReplyWithKeyboard отправляет сообщение с inline-клавиатурой.
func (b *Bot) sendReplyWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true
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
