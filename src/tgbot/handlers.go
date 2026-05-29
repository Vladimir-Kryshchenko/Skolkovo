package tgbot

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/common/model"
)

const (
	// docsPerPage — количество документов на странице пагинации.
	docsPerPage = 5
	// deadlinesPerPage — количество дедлайнов на странице пагинации.
	deadlinesPerPage = 5
)

// ---------------------------------------------------------------------------
// handleCommand — маршрутизатор команд.
// ---------------------------------------------------------------------------

func (b *Bot) handleCommand(update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	chatID := msg.Chat.ID
	command := msg.Command()

	switch command {
	case "start":
		b.cmdStart(chatID)
	case "status":
		b.cmdStatus(chatID)
	case "deadlines":
		b.cmdDeadlines(chatID, 0)
	case "docs":
		b.cmdDocs(chatID, 0)
	case "ask":
		b.cmdAsk(chatID, msg.Text)
	case "help":
		b.cmdHelp(chatID)
	default:
		b.sendReply(chatID, "❌ Неизвестная команда. Введите /help для справки.")
	}
}

// ---------------------------------------------------------------------------
// handleMessage — обработка обычных сообщений (не команд).
// ---------------------------------------------------------------------------

func (b *Bot) handleMessage(update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	// Если пользователь не авторизован и прислал email — привязываем.
	b.authMutex.RLock()
	_, exists := b.chatIDToEmail[chatID]
	b.authMutex.RUnlock()

	if !exists && emailRe.MatchString(text) {
		b.handleEmailAuth(chatID, text)
		return
	}

	// Если пользователь уже авторизован и прислал текст — это вопрос.
	if exists {
		b.cmdAsk(chatID, text)
		return
	}

	b.sendReply(chatID, "👋 Добро пожаловать! Введите /start для начала работы.")
}

// ---------------------------------------------------------------------------
// handleCallback — обработка inline callback-запросов.
// ---------------------------------------------------------------------------

func (b *Bot) handleCallback(update tgbotapi.Update) {
	cb := update.CallbackQuery
	if cb == nil {
		return
	}

	data := cb.Data
	chatID := cb.Message.Chat.ID

	switch {
	case strings.HasPrefix(data, "dl:"):
		b.handleDeadlineCallback(chatID, cb.ID, data)
	case strings.HasPrefix(data, "docs:"):
		b.handleDocsCallback(chatID, cb.ID, data)
	case strings.HasPrefix(data, "cmd:"):
		b.handleCommandCallback(chatID, cb.ID, data)
	default:
		b.answerCallback(cb.ID, "")
	}
}

// ---------------------------------------------------------------------------
// /start — приветствие.
// ---------------------------------------------------------------------------

func (b *Bot) cmdStart(chatID int64) {
	b.authMutex.RLock()
	_, exists := b.chatIDToEmail[chatID]
	b.authMutex.RUnlock()

	if exists {
		b.sendReply(chatID,
			"👋 Здравствуйте! Вы уже авторизованы.\n\n"+
				"Выберите действие:",
		)
		b.sendReplyWithKeyboard(chatID, "Главное меню:", MainKeyboard())
		return
	}

	b.sendReply(chatID,
		"👋 Добро пожаловать в бота системы резидентства Сколково!\n\n"+
			"Для начала работы введите ваш email, указанный при регистрации.\n"+
			"Пример: `ivan@example.com`",
	)
}

// handleEmailAuth привязывает chat ID к клиенту по email.
func (b *Bot) handleEmailAuth(chatID int64, email string) {
	clientID, err := AuthorizeUser(b.stores.Client, chatID, email)
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf(
			"❌ Не удалось авторизоваться: %v\n\n"+
				"Проверьте email или обратитесь в поддержку.", err,
		))
		return
	}

	b.authMutex.Lock()
	b.chatIDToEmail[chatID] = email
	b.authMutex.Unlock()

	b.sendReply(chatID,
		fmt.Sprintf("✅ Вы успешно авторизованы!\n\n"+
			"Email: `%s`\nClient ID: `%s`", email, clientID),
	)
	b.sendReplyWithKeyboard(chatID, "Главное меню:", MainKeyboard())
}

// ---------------------------------------------------------------------------
// /status — текущая стадия резидентства.
// ---------------------------------------------------------------------------

func (b *Bot) cmdStatus(chatID int64) {
	client, err := b.getClientByChatID(chatID)
	if err != nil {
		b.sendReply(chatID, "❌ Вы не авторизованы. Введите /start и укажите ваш email.")
		return
	}

	stageName := stageDescription(client.ResidencyStage)

	text := fmt.Sprintf(
		"📊 *Статус резидента*\n\n"+
			"Клиент: `%s`\n"+
			"ИНН: `%s`\n"+
			"Стадия: *%s*\n\n"+
			"%s",
		client.Name,
		client.INN,
		client.ResidencyStage,
		stageName,
	)

	b.sendReplyWithKeyboard(chatID, text, StageKeyboard())
}

// stageDescription возвращает текстовое описание стадии.
func stageDescription(stage model.ResidencyStage) string {
	descriptions := map[model.ResidencyStage]string{
		model.StageApplication: "📝 Ваша заявка подана и ожидает рассмотрения.",
		model.StageExamination: "🔍 Заявка находится на экспертизе.",
		model.StageDecision:    "⚖️ Ожидается решение по заявке.",
		model.StageContract:    "📄 Подготовка и подписание договора.",
		model.StageResident:    "🏆 Поздравляем! Вы — резидент Сколково.",
		model.StageReporting:   "📊 Период отчётности. Не забудьте сдать отчёты в срок.",
		model.StageExtension:   "🔄 Заявление на продление в обработке.",
		model.StageExit:        "🚪 Процедура выхода из резидентства.",
	}
	if desc, ok := descriptions[stage]; ok {
		return desc
	}
	return "ℹ️ Стадия: " + string(stage)
}

// ---------------------------------------------------------------------------
// /deadlines — ближайшие дедлайны.
// ---------------------------------------------------------------------------

func (b *Bot) cmdDeadlines(chatID int64, page int) {
	client, err := b.getClientByChatID(chatID)
	if err != nil {
		b.sendReply(chatID, "❌ Вы не авторизованы. Введите /start и укажите ваш email.")
		return
	}

	deadlines, err := b.stores.Deadline.ListDeadlines(context.Background(), client.ID, 90)
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf("❌ Ошибка получения дедлайнов: %v", err))
		return
	}

	if len(deadlines) == 0 {
		b.sendReply(chatID, "✅ У вас нет ближайших дедлайнов!")
		return
	}

	totalPages := (len(deadlines)-1)/deadlinesPerPage + 1
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * deadlinesPerPage
	end := start + deadlinesPerPage
	if end > len(deadlines) {
		end = len(deadlines)
	}

	var sb strings.Builder
	sb.WriteString("⏰ *Дедлайны*")
	if totalPages > 1 {
		sb.WriteString(fmt.Sprintf(" (стр. %d/%d)", page+1, totalPages))
	}
	sb.WriteString("\n\n")

	for i, d := range deadlines[start:end] {
		statusEmoji := "🟡"
		if d.Status == model.DeadlineOverdue {
			statusEmoji = "🔴"
		}
		sb.WriteString(fmt.Sprintf(
			"%s *%s*\n   Срок: %s\n   Статус: %s\n\n",
			statusEmoji,
			d.Title,
			d.DueDate.Format("02.01.2006"),
			d.Status,
		))
		_ = i
	}

	b.sendReplyWithKeyboard(chatID, sb.String(), DeadlineKeyboard(page, totalPages))
}

// ---------------------------------------------------------------------------
// /docs — мои документы.
// ---------------------------------------------------------------------------

func (b *Bot) cmdDocs(chatID int64, page int) {
	client, err := b.getClientByChatID(chatID)
	if err != nil {
		b.sendReply(chatID, "❌ Вы не авторизованы. Введите /start и укажите ваш email.")
		return
	}

	documents, err := b.stores.DocLink.ListClientDocuments(context.Background(), client.ID)
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf("❌ Ошибка получения документов: %v", err))
		return
	}

	if len(documents) == 0 {
		b.sendReply(chatID, "📄 У вас пока нет привязанных документов.")
		return
	}

	totalPages := (len(documents)-1)/docsPerPage + 1
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * docsPerPage
	end := start + docsPerPage
	if end > len(documents) {
		end = len(documents)
	}

	var sb strings.Builder
	sb.WriteString("📄 *Мои документы*")
	if totalPages > 1 {
		sb.WriteString(fmt.Sprintf(" (стр. %d/%d)", page+1, totalPages))
	}
	sb.WriteString("\n\n")

	for _, doc := range documents[start:end] {
		statusEmoji := "⏳"
		switch doc.Status {
		case model.DocSubmitted:
			statusEmoji = "📤"
		case model.DocApproved:
			statusEmoji = "✅"
		case model.DocRejected:
			statusEmoji = "❌"
		}
		sb.WriteString(fmt.Sprintf(
			"%s *%s* (ID: `%s`)\n   Статус: %s\n\n",
			statusEmoji,
			doc.DocumentID,
			doc.ID,
			doc.Status,
		))
	}

	b.sendReplyWithKeyboard(chatID, sb.String(), DocsKeyboard(page, totalPages))
}

// ---------------------------------------------------------------------------
// /ask — вопрос через MCP.
// ---------------------------------------------------------------------------

func (b *Bot) cmdAsk(chatID int64, fullText string) {
	// Извлекаем вопрос после команды /ask
	question := strings.TrimSpace(strings.TrimPrefix(fullText, "/ask"))
	if question == "" {
		b.sendReply(chatID,
			"❓ Задайте вопрос после команды `/ask`.\n\n"+
				"Пример: `/ask Какие документы нужны для подачи заявки?`",
		)
		return
	}

	b.sendReply(chatID, "🔍 Ищу ответ на ваш вопрос...")

	// Для MVP: отвечаем заглушкой, пока MCP ask_consultant не реализован.
	// TODO: подключить MCP ask_consultant при готовности.
	answer := fmt.Sprintf(
		"💬 *Ваш вопрос:* %s\n\n"+
			"⚠️ Функция `ask_consultant` ещё не подключена.\n"+
			"Ваш вопрос сохранён и будет обработан в ближайшее время.",
		question,
	)

	b.sendReply(chatID, answer)
}

// ---------------------------------------------------------------------------
// /help — справка по командам.
// ---------------------------------------------------------------------------

func (b *Bot) cmdHelp(chatID int64) {
	text := "📖 *Справка по командам*\n\n" +
		"/start — начать работу и привязать аккаунт\n" +
		"/status — узнать текущую стадию резидентства\n" +
		"/deadlines — посмотреть ближайшие дедлайны\n" +
		"/docs — список ваших документов\n" +
		"/ask <вопрос> — задать вопрос консультанту\n" +
		"/help — эта справка\n\n" +
		"💡 Вы также можете использовать кнопки под сообщениями для навигации."

	b.sendReply(chatID, text)
}

// ---------------------------------------------------------------------------
// Callback-обработчики.
// ---------------------------------------------------------------------------

func (b *Bot) handleDeadlineCallback(chatID int64, callbackID, data string) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		b.answerCallback(callbackID, "")
		return
	}

	var page int
	if _, err := fmt.Sscanf(parts[2], "%d", &page); err != nil {
		b.answerCallback(callbackID, "Ошибка пагинации")
		return
	}

	var newPage int
	switch parts[1] {
	case "prev":
		newPage = page - 1
		if newPage < 0 {
			newPage = 0
		}
	case "next":
		newPage = page + 1
	default:
		b.answerCallback(callbackID, "")
		return
	}

	b.answerCallback(callbackID, "")
	b.cmdDeadlines(chatID, newPage)
}

func (b *Bot) handleDocsCallback(chatID int64, callbackID, data string) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		b.answerCallback(callbackID, "")
		return
	}

	var page int
	if _, err := fmt.Sscanf(parts[2], "%d", &page); err != nil {
		b.answerCallback(callbackID, "Ошибка пагинации")
		return
	}

	var newPage int
	switch parts[1] {
	case "prev":
		newPage = page - 1
		if newPage < 0 {
			newPage = 0
		}
	case "next":
		newPage = page + 1
	default:
		b.answerCallback(callbackID, "")
		return
	}

	b.answerCallback(callbackID, "")
	b.cmdDocs(chatID, newPage)
}

func (b *Bot) handleCommandCallback(chatID int64, callbackID, data string) {
	parts := strings.Split(data, ":")
	if len(parts) != 2 {
		b.answerCallback(callbackID, "")
		return
	}

	b.answerCallback(callbackID, "")

	switch parts[1] {
	case "status":
		b.cmdStatus(chatID)
	case "deadlines":
		b.cmdDeadlines(chatID, 0)
	case "docs":
		b.cmdDocs(chatID, 0)
	case "ask":
		b.cmdAsk(chatID, "")
	case "checklists":
		b.cmdChecklists(chatID)
	case "stage_info":
		b.cmdStageInfo(chatID)
	default:
		b.answerCallback(callbackID, "Неизвестное действие")
	}
}

// cmdChecklists — показывает чек-листы клиента.
func (b *Bot) cmdChecklists(chatID int64) {
	client, err := b.getClientByChatID(chatID)
	if err != nil {
		b.sendReply(chatID, "❌ Вы не авторизованы. Введите /start.")
		return
	}

	checklists, err := b.stores.Checklist.GetClientChecklists(context.Background(), client.ID)
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf("❌ Ошибка получения чек-листов: %v", err))
		return
	}

	if len(checklists) == 0 {
		b.sendReply(chatID, "📋 У вас пока нет чек-листов.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 *Мои чек-листы*\n\n")

	for _, cc := range checklists {
		tpl, err := b.stores.Checklist.GetChecklist(context.Background(), cc.ChecklistID)
		title := cc.ChecklistID
		if err == nil && tpl != nil {
			title = tpl.Title
		}
		sb.WriteString(fmt.Sprintf("• *%s* — %s\n", title, cc.Status))
	}

	b.sendReply(chatID, sb.String())
}

// cmdStageInfo — информация о текущей стадии.
func (b *Bot) cmdStageInfo(chatID int64) {
	client, err := b.getClientByChatID(chatID)
	if err != nil {
		b.sendReply(chatID, "❌ Вы не авторизованы. Введите /start.")
		return
	}

	text := fmt.Sprintf(
		"📖 *Информация о стадии*\n\n"+
			"Текущая стадия: *%s*\n\n"+
			"%s",
		client.ResidencyStage,
		stageDescription(client.ResidencyStage),
	)

	b.sendReply(chatID, text)
}

// getClientByChatID получает клиента по chat ID через авторизацию.
func (b *Bot) getClientByChatID(chatID int64) (*model.Client, error) {
	return GetClientByChatID(b.stores.Client, chatID)
}

// emailRe — упрощённая проверка email.
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
