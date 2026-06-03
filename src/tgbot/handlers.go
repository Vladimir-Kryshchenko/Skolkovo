package tgbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/skru"
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

	ctx := context.Background()
	if b.tenantID != "" {
		ctx = aimodels.WithTenantID(ctx, b.tenantID)
	}

	resp, err := b.consultant.Ask(ctx, question, "")
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
// /news — актуальные новости Фонда (sk.ru, лента «Новости», свежие сверху).
// ---------------------------------------------------------------------------

func (b *Bot) cmdNews(chatID int64) {
	if b.stores.SkRu == nil {
		b.sendReply(chatID, "📰 Раздел новостей временно недоступен.")
		return
	}
	posts, err := b.stores.SkRu.Posts(context.Background(), "news", time.Now())
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить новости. Попробуйте позже.")
		b.logf("news: %v", err)
		return
	}
	b.renderPosts(chatID, "news", "📰 *Новости Фонда «Сколково»*", "📰", posts,
		"📰 Свежих новостей не нашлось. Спросите о теме — поищу по всей базе.")
}

// ---------------------------------------------------------------------------
// /events — актуальные мероприятия Фонда (sk.ru, лента «События»).
// ---------------------------------------------------------------------------

func (b *Bot) cmdEvents(chatID int64) {
	if b.stores.SkRu == nil {
		b.sendReply(chatID, "📅 Раздел мероприятий временно недоступен.")
		return
	}
	posts, err := b.stores.SkRu.Posts(context.Background(), "events", time.Now())
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить мероприятия. Попробуйте позже.")
		b.logf("events: %v", err)
		return
	}
	b.renderPosts(chatID, "events", "📅 *Мероприятия Сколково*", "📅", posts,
		"📅 Актуальных мероприятий не нашлось.")
}

// ---------------------------------------------------------------------------
// /contests — актуальные конкурсы и гранты (sk.ru, фильтр ленты по маркерам).
// ---------------------------------------------------------------------------

func (b *Bot) cmdContests(chatID int64) {
	if b.stores.SkRu == nil {
		b.sendReply(chatID, "🏆 Раздел конкурсов временно недоступен.")
		return
	}
	posts, err := b.stores.SkRu.Contests(context.Background(), time.Now())
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить конкурсы. Попробуйте позже.")
		b.logf("contests: %v", err)
		return
	}
	b.renderPosts(chatID, "contests", "🏆 *Конкурсы и гранты*", "🏆", posts,
		"🏆 Открытых конкурсов сейчас не нашлось. Спросите о конкретной программе.")
}

// renderPosts выводит ленту публикаций sk.ru: баннер, до listLimit записей
// (заголовок + дата + описание + ссылка), кнопка «← Меню».
func (b *Bot) renderPosts(chatID int64, banner, header, emoji string, posts []skru.Post, emptyMsg string) {
	if len(posts) == 0 {
		b.sendReplyWithKeyboard(chatID, emptyMsg, BackToMenuKeyboard())
		return
	}
	b.sendCommandBanner(chatID, banner, header)

	var sb strings.Builder
	for i, p := range posts {
		if i >= listLimit {
			break
		}
		sb.WriteString(fmt.Sprintf("%s *%s*\n", emoji, p.Title))
		if !p.PublishDate.IsZero() {
			sb.WriteString(fmt.Sprintf("   🗓 %s\n", p.PublishDate.Format("02.01.2006")))
		}
		if p.ShortDesc != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", truncate(p.ShortDesc, 160)))
		}
		sb.WriteString(fmt.Sprintf("   [Открыть](%s)\n\n", p.URL))
	}
	sb.WriteString("💡 Нужны детали? Напишите вопрос — отвечу через консультанта.")
	b.sendLongWithMenu(chatID, sb.String())
}

// ---------------------------------------------------------------------------
// /faq — частые вопросы (семантический поиск по страницам сайта).
// ---------------------------------------------------------------------------

func (b *Bot) cmdFAQ(chatID int64) {
	if b.stores.PageSearch == nil {
		b.sendReply(chatID, "❓ Раздел FAQ временно недоступен.")
		return
	}
	hits, err := b.stores.PageSearch.Search(context.Background(),
		"часто задаваемые вопросы как стать резидентом требования Сколково", listLimit)
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить раздел. Попробуйте позже.")
		b.logf("faq: %v", err)
		return
	}
	if len(hits) == 0 {
		b.sendReplyWithKeyboard(chatID,
			"❓ По частым вопросам ничего не нашлось. Просто напишите свой вопрос в чат.",
			BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "faq", "❓ *Частые вопросы*")
	var sb strings.Builder
	for _, h := range hits {
		title := h.Title
		if title == "" {
			title = h.URL
		}
		sb.WriteString(fmt.Sprintf("• *%s*\n", title))
		if h.Summary != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", truncate(h.Summary, 160)))
		}
		sb.WriteString(fmt.Sprintf("  [Открыть](%s)\n\n", h.URL))
	}
	sb.WriteString("💡 Нужны детали? Напишите вопрос — отвечу через консультанта.")
	b.sendLongWithMenu(chatID, sb.String())
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
