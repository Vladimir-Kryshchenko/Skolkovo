package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/common/model"
)

const (
	// listLimit — сколько элементов показываем в разделах-листингах.
	listLimit = 8
	// askCooldown — минимальный интервал между вопросами консультанту (анти-флуд).
	askCooldown = 3 * time.Second
	// tgMaxLen — лимит длины сообщения Telegram.
	tgMaxLen = 4096
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
	switch msg.Command() {
	case "start":
		b.cmdStart(chatID)
	case "ask":
		b.cmdAsk(chatID, msg.CommandArguments())
	case "events":
		b.cmdEvents(chatID)
	case "contests":
		b.cmdContests(chatID)
	case "news":
		b.cmdNews(chatID)
	case "faq":
		b.cmdFAQ(chatID)
	case "company":
		b.cmdCompany(chatID, msg.CommandArguments())
	case "help":
		b.cmdHelp(chatID)
	default:
		b.sendReply(chatID, "❌ Неизвестная команда. Нажмите *ℹ️ Помощь* или введите /help.")
	}
}

// ---------------------------------------------------------------------------
// handleMessage — обработка обычных сообщений (кнопки меню + свободный текст).
// ---------------------------------------------------------------------------

func (b *Bot) handleMessage(update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// Кнопки постоянного reply-меню.
	switch text {
	case "🤖 Задать вопрос":
		b.sendReply(chatID, "❓ Напишите ваш вопрос — я найду ответ в базе знаний Сколково.\n\nНапример: _Какие льготы по налогу на прибыль у резидентов?_")
		return
	case "📅 Мероприятия":
		b.cmdEvents(chatID)
		return
	case "🏆 Конкурсы":
		b.cmdContests(chatID)
		return
	case "📰 Новости":
		b.cmdNews(chatID)
		return
	case "❓ FAQ":
		b.cmdFAQ(chatID)
		return
	case "ℹ️ Помощь":
		b.cmdHelp(chatID)
		return
	}

	// Любой другой текст — вопрос ИИ-консультанту.
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

	chatID := cb.Message.Chat.ID
	b.answerCallback(cb.ID, "")

	switch cb.Data {
	case "cmd:events":
		b.cmdEvents(chatID)
	case "cmd:contests":
		b.cmdContests(chatID)
	case "cmd:news":
		b.cmdNews(chatID)
	case "cmd:faq":
		b.cmdFAQ(chatID)
	case "cmd:ask":
		b.sendReply(chatID, "❓ Напишите ваш вопрос — я найду ответ в базе знаний Сколково.")
	case "cmd:help":
		b.cmdHelp(chatID)
	case "cmd:menu":
		b.sendReplyWithKeyboard(chatID, "Главное меню:", MainKeyboard())
	}
}

// ---------------------------------------------------------------------------
// /start — приветствие.
// ---------------------------------------------------------------------------

func (b *Bot) cmdStart(chatID int64) {
	b.sendCommandBanner(chatID, "start", "🚀 *База Сколково* — информационный помощник")
	b.sendReplyWithMenuKeyboard(chatID,
		"👋 *Здравствуйте!*\n\n"+
			"Я — информационный бот Фонда «Сколково». Помогу найти ответы по документам, "+
			"расскажу о мероприятиях, конкурсах и новостях.\n\n"+
			"Просто *напишите вопрос* в чат — например:\n"+
			"_«Какие документы нужны для статуса резидента?»_\n\n"+
			"Или выберите раздел в меню ниже 👇")
	b.sendReplyWithKeyboard(chatID, "Популярные разделы:", MainKeyboard())
}

// ---------------------------------------------------------------------------
// /ask — вопрос ИИ-консультанту (основная функция).
// ---------------------------------------------------------------------------

func (b *Bot) cmdAsk(chatID int64, question string) {
	question = strings.TrimSpace(question)
	if question == "" {
		b.sendReplyWithKeyboard(chatID,
			"🤖 *ИИ-консультант Сколково*\n\n"+
				"Напишите вопрос в чат — я найду ответ в базе знаний и приведу источники.\n\n"+
				"Примеры:\n"+
				"• _Какие льготы у резидентов Сколково?_\n"+
				"• _Как продлить статус участника?_\n"+
				"• _Требования к отчётности_",
			BackToMenuKeyboard())
		return
	}

	// Анти-флуд: не чаще одного вопроса в askCooldown.
	if !b.allowAsk(chatID) {
		b.sendReply(chatID, "⏳ Секунду — предыдущий вопрос ещё обрабатывается.")
		return
	}

	if b.consultant == nil {
		b.sendReply(chatID, "⚠️ Консультант временно недоступен. Попробуйте позже.")
		return
	}

	b.sendReply(chatID, "🔍 Ищу ответ в базе знаний Сколково…")

	// ИНН (если задан) добавляем как контекст для персонализации.
	q := question
	if inn := b.getINN(chatID); inn != "" {
		q = fmt.Sprintf("%s\n\n(Контекст: компания пользователя, ИНН %s)", question, inn)
	}

	ctx := context.Background()
	if b.tenantID != "" {
		ctx = aimodels.WithTenantID(ctx, b.tenantID)
	}

	resp, err := b.consultant.Ask(ctx, q, "")
	if err != nil {
		b.sendReply(chatID, fmt.Sprintf("❌ Не удалось получить ответ: %v", err))
		return
	}

	answer := resp.Answer
	if answer == "" {
		answer = "К сожалению, по вашему вопросу ничего не нашлось. Попробуйте переформулировать."
	}

	if len(resp.Sources) > 0 {
		var sb strings.Builder
		sb.WriteString("\n\n📚 *Источники:*")
		for i, s := range resp.Sources {
			if i >= 5 {
				break
			}
			if s.SourceURL != "" {
				sb.WriteString(fmt.Sprintf("\n• [%s](%s)", s.Title, s.SourceURL))
			} else {
				sb.WriteString(fmt.Sprintf("\n• %s", s.Title))
			}
		}
		answer += sb.String()
	}

	b.sendLongWithMenu(chatID, answer)
}

// allowAsk реализует мягкий анти-флуд: не чаще одного вопроса в askCooldown.
func (b *Bot) allowAsk(chatID int64) bool {
	b.askMu.Lock()
	defer b.askMu.Unlock()
	now := time.Now()
	if last, ok := b.askLastTime[chatID]; ok && now.Sub(last) < askCooldown {
		return false
	}
	b.askLastTime[chatID] = now
	return true
}

// ---------------------------------------------------------------------------
// /events — ближайшие мероприятия.
// ---------------------------------------------------------------------------

func (b *Bot) cmdEvents(chatID int64) {
	if b.stores.Event == nil {
		b.sendReply(chatID, "📅 Раздел мероприятий временно недоступен.")
		return
	}

	now := time.Now()
	events, err := b.stores.Event.ListEvents(context.Background(), "", model.EventActive, &now, nil)
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить мероприятия. Попробуйте позже.")
		b.logf("events: %v", err)
		return
	}
	if len(events) == 0 {
		b.sendReplyWithKeyboard(chatID, "📅 Ближайших мероприятий пока нет.", BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "events", fmt.Sprintf("📅 *Мероприятия* — %d ближайших", len(events)))

	var sb strings.Builder
	for i, e := range events {
		if i >= listLimit {
			break
		}
		sb.WriteString(fmt.Sprintf("📌 *%s*\n", e.Title))
		sb.WriteString(fmt.Sprintf("   🗓 %s", e.EventDate.Format("02.01.2006")))
		if e.Location != "" {
			sb.WriteString(fmt.Sprintf(" · 📍 %s", e.Location))
		}
		sb.WriteString("\n")
		if e.SourceURL != "" {
			sb.WriteString(fmt.Sprintf("   [Подробнее](%s)\n", e.SourceURL))
		}
		sb.WriteString("\n")
	}
	if len(events) > listLimit {
		sb.WriteString(fmt.Sprintf("…и ещё %d. Уточните интересующую тему в вопросе.", len(events)-listLimit))
	}
	b.sendReplyWithKeyboard(chatID, sb.String(), BackToMenuKeyboard())
}

// ---------------------------------------------------------------------------
// /contests — открытые конкурсы и гранты.
// ---------------------------------------------------------------------------

func (b *Bot) cmdContests(chatID int64) {
	if b.stores.Contest == nil {
		b.sendReply(chatID, "🏆 Раздел конкурсов временно недоступен.")
		return
	}

	contests, err := b.stores.Contest.ListContests(context.Background(), "", model.ContestActive)
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить конкурсы. Попробуйте позже.")
		b.logf("contests: %v", err)
		return
	}
	if len(contests) == 0 {
		b.sendReplyWithKeyboard(chatID, "🏆 Открытых конкурсов сейчас нет.", BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "contests", fmt.Sprintf("🏆 *Конкурсы и гранты* — %d открытых", len(contests)))

	var sb strings.Builder
	for i, c := range contests {
		if i >= listLimit {
			break
		}
		sb.WriteString(fmt.Sprintf("🏆 *%s*\n", c.Title))
		if !c.EndDate.IsZero() {
			sb.WriteString(fmt.Sprintf("   ⏳ приём до %s\n", c.EndDate.Format("02.01.2006")))
		}
		if c.Prize != "" {
			sb.WriteString(fmt.Sprintf("   💰 %s\n", c.Prize))
		}
		if c.SourceURL != "" {
			sb.WriteString(fmt.Sprintf("   [Подробнее](%s)\n", c.SourceURL))
		}
		sb.WriteString("\n")
	}
	if len(contests) > listLimit {
		sb.WriteString(fmt.Sprintf("…и ещё %d.", len(contests)-listLimit))
	}
	b.sendReplyWithKeyboard(chatID, sb.String(), BackToMenuKeyboard())
}

// ---------------------------------------------------------------------------
// /faq — частые вопросы.
// ---------------------------------------------------------------------------

func (b *Bot) cmdFAQ(chatID int64) {
	if b.stores.FAQ == nil {
		b.sendReply(chatID, "❓ Раздел FAQ временно недоступен.")
		return
	}

	items, err := b.stores.FAQ.ListFAQItems(context.Background(), "")
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить FAQ. Попробуйте позже.")
		b.logf("faq: %v", err)
		return
	}
	if len(items) == 0 {
		b.sendReplyWithKeyboard(chatID, "❓ Список частых вопросов пока пуст.", BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "faq", fmt.Sprintf("❓ *Частые вопросы* — %d", len(items)))

	var sb strings.Builder
	for i, it := range items {
		if i >= listLimit {
			break
		}
		sb.WriteString(fmt.Sprintf("*%s*\n%s\n", it.Question, truncate(it.Answer, 280)))
		if it.SourceURL != "" {
			sb.WriteString(fmt.Sprintf("[Источник](%s)\n", it.SourceURL))
		}
		sb.WriteString("\n")
	}
	if len(items) > listLimit {
		sb.WriteString("💡 Не нашли свой вопрос? Просто напишите его в чат.")
	}
	b.sendLongWithMenu(chatID, sb.String())
}

// ---------------------------------------------------------------------------
// /news — свежие новости (семантический поиск по базе, тип news).
// ---------------------------------------------------------------------------

func (b *Bot) cmdNews(chatID int64) {
	if b.consultant == nil {
		b.sendReply(chatID, "📰 Раздел новостей временно недоступен.")
		return
	}

	ctx := context.Background()
	results, err := b.consultant.Search(ctx, "новости события Фонда Сколково", 25)
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить новости. Попробуйте позже.")
		b.logf("news: %v", err)
		return
	}

	// Оставляем только новости, дедуплицируем по документу.
	seen := map[string]bool{}
	var news []string
	for _, r := range results {
		if r.EntityType != "news" || seen[r.DocumentID] {
			continue
		}
		seen[r.DocumentID] = true
		line := fmt.Sprintf("📰 *%s*", r.Title)
		url := b.consultant.BestSourceURL(ctx, r)
		if url != "" {
			line += fmt.Sprintf("\n[Читать](%s)", url)
		}
		news = append(news, line)
		if len(news) >= listLimit {
			break
		}
	}

	if len(news) == 0 {
		b.sendReplyWithKeyboard(chatID,
			"📰 Свежих новостей не нашлось. Спросите о конкретной теме — я поищу по всей базе.",
			BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "news", "📰 *Новости Фонда «Сколково»*")
	b.sendLongWithMenu(chatID, strings.Join(news, "\n\n"))
}

// ---------------------------------------------------------------------------
// /company — опциональная привязка ИНН для персонализации.
// ---------------------------------------------------------------------------

func (b *Bot) cmdCompany(chatID int64, arg string) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		current := b.getINN(chatID)
		if current != "" {
			b.sendReplyWithKeyboard(chatID,
				fmt.Sprintf("🏢 Сейчас указан ИНН: `%s`\n\nЧтобы изменить — отправьте `/company НОВЫЙ_ИНН`, чтобы убрать — `/company -`.", current),
				BackToMenuKeyboard())
			return
		}
		b.sendReplyWithKeyboard(chatID,
			"🏢 *Указать компанию (необязательно)*\n\n"+
				"Если укажете ИНН своей компании, я буду учитывать его при ответах.\n"+
				"Это *не* вход в личный кабинет — только контекст для консультанта.\n\n"+
				"Отправьте: `/company 7710000000`",
			BackToMenuKeyboard())
		return
	}

	if arg == "-" {
		b.clearINN(chatID)
		b.sendReply(chatID, "✅ ИНН удалён.")
		return
	}

	if !innRe.MatchString(arg) {
		b.sendReply(chatID, "❌ Это не похоже на ИНН. Нужно 10 цифр (организация) или 12 (ИП).")
		return
	}

	b.setINN(chatID, arg)
	b.sendReplyWithKeyboard(chatID,
		fmt.Sprintf("✅ ИНН `%s` сохранён. Теперь ответы будут учитывать вашу компанию.\n\nЗадайте вопрос — например: _Подходит ли моя компания под критерии Сколково?_", arg),
		BackToMenuKeyboard())
}

// ---------------------------------------------------------------------------
// /help — справка.
// ---------------------------------------------------------------------------

func (b *Bot) cmdHelp(chatID int64) {
	b.sendCommandBanner(chatID, "help", "ℹ️ *Справка — База Сколково*")
	text := "📖 *Что умеет бот*\n\n" +
		"Это публичный помощник по базе знаний Фонда «Сколково». " +
		"Вход и регистрация не нужны.\n\n" +
		"*Главное:* просто напишите любой вопрос в чат — я найду ответ в документах и приведу источники.\n\n" +
		"*Команды:*\n" +
		"/ask — спросить ИИ-консультанта\n" +
		"/events — ближайшие мероприятия\n" +
		"/contests — конкурсы и гранты\n" +
		"/news — свежие новости\n" +
		"/faq — частые вопросы\n" +
		"/company — указать ИНН для персонализации (необязательно)\n" +
		"/help — эта справка\n\n" +
		"💡 Бот отвечает по открытым материалам Сколково и не имеет доступа к личным кабинетам резидентов."
	b.sendReplyWithKeyboard(chatID, text, MainKeyboard())
}

// ---------------------------------------------------------------------------
// Вспомогательные функции.
// ---------------------------------------------------------------------------

// truncate обрезает строку до n символов (по рунам), добавляя «…».
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// sendLongWithMenu отправляет (при необходимости — разбивая) длинный текст,
// прикрепляя кнопку «← Меню» к последнему сообщению.
func (b *Bot) sendLongWithMenu(chatID int64, text string) {
	for len(text) > tgMaxLen {
		cut := tgMaxLen
		// Пытаемся резать по переносу строки в последних 300 байтах.
		for i := tgMaxLen - 1; i >= tgMaxLen-300 && i >= 0; i-- {
			if text[i] == '\n' {
				cut = i + 1
				break
			}
		}
		b.sendReply(chatID, text[:cut])
		text = text[cut:]
	}
	b.sendReplyWithKeyboard(chatID, text, BackToMenuKeyboard())
}
