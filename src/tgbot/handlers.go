package tgbot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/aimodels"
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
	case "checklists":
		b.cmdChecklists(chatID)
	case "ask":
		b.cmdAsk(chatID, msg.CommandArguments())
	case "logout":
		b.cmdLogout(chatID)
	case "help":
		b.cmdHelp(chatID)
	default:
		b.sendReply(chatID, "❌ Неизвестная команда. Нажмите *ℹ️ Помощь* или введите /help.")
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
	exists := b.isAuthorized(chatID)

	if !exists && emailRe.MatchString(text) {
		b.handleEmailAuth(chatID, text)
		return
	}

	if !exists {
		b.sendReply(chatID, "👋 Добро пожаловать!\n\nДля начала работы введите ваш email, указанный при регистрации в Сколково.\nПример: `ivan@company.com`")
		return
	}

	// Обработка нажатий reply-кнопок постоянного меню.
	switch text {
	case "📊 Статус":
		b.cmdStatus(chatID)
		return
	case "⏰ Дедлайны":
		b.cmdDeadlines(chatID, 0)
		return
	case "📋 Чек-листы":
		b.cmdChecklists(chatID)
		return
	case "📄 Документы":
		b.cmdDocs(chatID, 0)
		return
	case "❓ Задать вопрос":
		b.sendReply(chatID, "❓ Введите ваш вопрос — я перешлю его консультанту по резидентству Сколково.\n\nПример: *Какие отчёты нужно сдавать?*")
		return
	case "ℹ️ Помощь":
		b.cmdHelp(chatID)
		return
	}

	// Авторизован и прислал произвольный текст — направляем консультанту.
	b.cmdAsk(chatID, text)
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
	if b.isAuthorized(chatID) {
		b.sendCommandBanner(chatID, "start",
			"👋 *Добро пожаловать!* Вы уже авторизованы.")
		b.sendReplyWithMenuKeyboard(chatID, "Используйте кнопки меню или выберите раздел ниже:")
		b.sendReplyWithKeyboard(chatID, "Быстрые действия:", MainKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "start",
		"🚀 *Система резидентства Сколково*\n\nВведите email для авторизации")
	b.sendReply(chatID,
		"Этот бот поможет вам:\n"+
			"• Отслеживать статус и стадию резидентства\n"+
			"• Контролировать дедлайны и чек-листы\n"+
			"• Получать ответы на вопросы по документам Сколково\n\n"+
			"Введите email, указанный при регистрации:\n"+
			"`ivan@company.com`",
	)
}

// handleEmailAuth привязывает chat ID к клиенту по email.
func (b *Bot) handleEmailAuth(chatID int64, email string) {
	clientID, err := b.authorizeUser(chatID, email)
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf(
			"❌ *Не удалось авторизоваться*\n\n"+
				"Причина: %v\n\n"+
				"Проверьте правильность email. Если проблема сохраняется — обратитесь к своему куратору в Сколково.", err,
		))
		return
	}

	b.sendReplyWithMenuKeyboard(chatID,
		fmt.Sprintf("✅ *Авторизация успешна!*\n\n"+
			"Email: `%s`\n"+
			"ID: `%s`\n\n"+
			"Теперь вы можете использовать все функции бота. "+
			"Используйте кнопки меню или просто напишите вопрос — отвечу через консультанта.", email, clientID))
	b.sendReplyWithKeyboard(chatID, "Что хотите сделать?", MainKeyboard())
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
	caption := fmt.Sprintf("📊 *%s* — стадия: _%s_", client.Name, client.ResidencyStage)
	b.sendCommandBanner(chatID, "status", caption)

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

	if page == 0 {
		b.sendCommandBanner(chatID, "deadlines",
			fmt.Sprintf("⏰ *Дедлайны* — %d задач на ближайшие 90 дней", len(deadlines)))
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

	if page == 0 {
		b.sendCommandBanner(chatID, "docs",
			fmt.Sprintf("📄 *Документы* — %d файлов в вашем деле", len(documents)))
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
// /ask — вопрос через ConsultantAgent.
// ---------------------------------------------------------------------------

const askCooldown = 60 // секунд между запросами к ИИ

func (b *Bot) cmdAsk(chatID int64, question string) {
	question = strings.TrimSpace(question)
	if question == "" {
		b.sendReplyWithKeyboard(chatID,
			"❓ *Задайте вопрос консультанту*\n\n"+
				"Просто напишите вопрос в чат, и я отвечу на него.\n\n"+
				"Или используйте команду:\n"+
				"`/ask Какие документы нужны для вступления?`",
			BackToMenuKeyboard())
		return
	}

	// Rate limiting: не чаще одного запроса в askCooldown секунд.
	b.askMu.Lock()
	last, exists := b.askLastTime[chatID]
	now := time.Now()
	if exists && now.Sub(last).Seconds() < askCooldown {
		remaining := askCooldown - int(now.Sub(last).Seconds())
		b.askMu.Unlock()
		b.sendReply(chatID, fmt.Sprintf("⏳ Подождите %d сек. перед следующим вопросом.", remaining))
		return
	}
	b.askLastTime[chatID] = now
	b.askMu.Unlock()

	// Получаем clientID по chatID
	clientID := ""
	if client, err := b.getClientByChatID(chatID); err == nil && client != nil {
		clientID = client.ID
	}

	b.sendCommandBanner(chatID, "ask", "🤖 *ИИ-консультант Сколково*")
	b.sendReply(chatID, "🔍 Ищу ответ на ваш вопрос...")

	if b.consultant == nil {
		b.sendReply(chatID,
			"⚠️ Консультант временно недоступен. Попробуйте позже.")
		return
	}

	// Проставляем tenant_id в контекст для учёта расхода токенов.
	ctx := context.Background()
	if b.tenantID != "" {
		ctx = aimodels.WithTenantID(ctx, b.tenantID)
	}

	resp, err := b.consultant.Ask(ctx, question, clientID)
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf("❌ Ошибка: %v", err))
		return
	}

	// Разбиваем ответ на части, если он длиннее 4096 символов (Telegram limit)
	answer := resp.Answer
	if answer == "" {
		answer = "Ответ не найден."
	}

	// Добавляем источники, если есть
	if len(resp.Sources) > 0 {
		var sourcesText string
		for _, s := range resp.Sources {
			sourcesText += fmt.Sprintf("\n• %s", s.Title)
			if s.SourceURL != "" {
				sourcesText += fmt.Sprintf(" (%s)", s.SourceURL)
			}
		}
		answer += "\n\n📚 Источники:" + sourcesText
	}

	// Разбиваем на сообщения по 4096 символов
	const maxLen = 4096
	for len(answer) > 0 {
		if len(answer) <= maxLen {
			b.sendReply(chatID, answer)
			break
		}
		// Ищем ближайший перенос строки перед maxLen
		cut := maxLen
		for i := maxLen - 1; i >= maxLen-200 && i >= 0; i-- {
			if answer[i] == '\n' {
				cut = i + 1
				break
			}
		}
		b.sendReply(chatID, answer[:cut])
		answer = answer[cut:]
	}
}

// ---------------------------------------------------------------------------
// /help — справка по командам.
// ---------------------------------------------------------------------------

func (b *Bot) cmdHelp(chatID int64) {
	b.sendCommandBanner(chatID, "help", "ℹ️ *Справка — База Сколково*")
	text := "📖 *Справка*\n\n" +
		"*Команды:*\n" +
		"/start — начать работу и авторизоваться\n" +
		"/status — текущая стадия резидентства\n" +
		"/deadlines — ближайшие дедлайны (90 дней)\n" +
		"/docs — мои документы\n" +
		"/checklists — чек-листы текущей процедуры\n" +
		"/ask <вопрос> — вопрос ИИ-консультанту\n" +
		"/help — эта справка\n\n" +
		"*Быстрый доступ:* используйте кнопки меню внизу экрана.\n\n" +
		"*Свободный чат:* просто напишите вопрос — он автоматически уйдёт консультанту.\n\n" +
		"💡 По вопросам с ботом обратитесь к своему куратору в Сколково."

	b.sendReplyWithKeyboard(chatID, text, MainKeyboard())
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
		b.sendReply(chatID, "❓ Введите ваш вопрос — я перешлю его консультанту по резидентству Сколково.")
	case "checklists":
		b.cmdChecklists(chatID)
	case "stage_info":
		b.cmdStageInfo(chatID)
	case "menu":
		b.sendReplyWithKeyboard(chatID, "Главное меню:", MainKeyboard())
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

	b.sendCommandBanner(chatID, "checklists",
		fmt.Sprintf("📋 *Чек-листы* — %d процедуры", len(checklists)))

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
	return b.clientByChatID(chatID)
}

// cmdLogout — сброс авторизации текущего чата.
func (b *Bot) cmdLogout(chatID int64) {
	if !b.isAuthorized(chatID) {
		b.sendReply(chatID, "ℹ️ Вы не авторизованы. Введите /start чтобы войти.")
		return
	}
	b.deauthorize(chatID)
	b.sendReply(chatID,
		"👋 *Вы вышли из аккаунта.*\n\n"+
			"Для повторного входа введите /start и укажите ваш email.")
}

// emailRe — упрощённая проверка email.
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
