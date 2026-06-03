package tgbot

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/sitepages"
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
// /news — свежие новости (страницы сайта раздела «новости», по дате публикации).
// ---------------------------------------------------------------------------

func (b *Bot) cmdNews(chatID int64) {
	if b.stores.Pages == nil {
		b.sendReply(chatID, "📰 Раздел новостей временно недоступен.")
		return
	}

	// Берём активные страницы, у которых раздел/URL содержит «news», свежие сверху.
	pages, err := b.stores.Pages.List(context.Background(), sitepages.PageFilter{
		Query:  "news",
		Status: "active",
		Limit:  500,
	})
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить новости. Попробуйте позже.")
		b.logf("news: %v", err)
		return
	}

	news := filterNewsPages(pages, listLimit)
	if len(news) == 0 {
		b.sendReplyWithKeyboard(chatID,
			"📰 Свежих новостей не нашлось. Спросите о конкретной теме — я поищу по всей базе.",
			BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, "news", "📰 *Новости Фонда «Сколково»*")
	var sb strings.Builder
	for _, p := range news {
		sb.WriteString(fmt.Sprintf("📰 *%s*\n", pageTitle(p)))
		if p.PublishedAt != nil {
			sb.WriteString(fmt.Sprintf("   🗓 %s\n", p.PublishedAt.Format("02.01.2006")))
		}
		sb.WriteString(fmt.Sprintf("   [Читать](%s)\n\n", p.URL))
	}
	b.sendLongWithMenu(chatID, sb.String())
}

// ---------------------------------------------------------------------------
// /events — материалы о мероприятиях (семантический поиск по страницам сайта).
// ---------------------------------------------------------------------------

func (b *Bot) cmdEvents(chatID int64) {
	b.searchSection(chatID, "events", "📅 *Мероприятия Сколково*",
		"мероприятия события форумы конференции Сколково",
		"📅 Материалов о мероприятиях не нашлось.")
}

// ---------------------------------------------------------------------------
// /contests — конкурсы и гранты (семантический поиск по страницам сайта).
// ---------------------------------------------------------------------------

func (b *Bot) cmdContests(chatID int64) {
	b.searchSection(chatID, "contests", "🏆 *Конкурсы и гранты*",
		"конкурсы гранты отбор финансирование стартапов Сколково",
		"🏆 Материалов о конкурсах не нашлось.")
}

// ---------------------------------------------------------------------------
// /faq — частые вопросы (семантический поиск по страницам сайта).
// ---------------------------------------------------------------------------

func (b *Bot) cmdFAQ(chatID int64) {
	b.searchSection(chatID, "faq", "❓ *Частые вопросы*",
		"часто задаваемые вопросы как стать резидентом требования Сколково",
		"❓ По частым вопросам ничего не нашлось. Просто напишите свой вопрос в чат.")
}

// searchSection — общий обработчик тематических разделов: семантический поиск по
// страницам публичного сайта (site_pages) с баннером и кнопкой «← Меню».
func (b *Bot) searchSection(chatID int64, banner, header, query, emptyMsg string) {
	if b.stores.PageSearch == nil {
		b.sendReply(chatID, header+"\n\nРаздел временно недоступен.")
		return
	}

	hits, err := b.stores.PageSearch.Search(context.Background(), query, listLimit)
	if err != nil {
		b.sendReply(chatID, "❌ Не удалось загрузить раздел. Попробуйте позже.")
		b.logf("section %s: %v", banner, err)
		return
	}
	if len(hits) == 0 {
		b.sendReplyWithKeyboard(chatID, emptyMsg, BackToMenuKeyboard())
		return
	}

	b.sendCommandBanner(chatID, banner, header)
	var sb strings.Builder
	for _, h := range hits {
		title := h.Title
		if title == "" {
			title = h.URL
		}
		sb.WriteString(fmt.Sprintf("• *%s*\n", title))
		summary := h.Summary
		if summary != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", truncate(summary, 160)))
		}
		sb.WriteString(fmt.Sprintf("  [Открыть](%s)\n\n", h.URL))
	}
	sb.WriteString("💡 Нужны детали? Напишите вопрос — отвечу через консультанта.")
	b.sendLongWithMenu(chatID, sb.String())
}

// newsTitleBlacklist — служебные заголовки страниц-листингов, не являющихся статьями.
var newsTitleBlacklist = map[string]bool{
	"фонд сколково":             true,
	"news - skolkovo community": true,
	"":                          true,
}

// filterNewsPages оставляет настоящие новостные статьи и сортирует свежие сверху.
// Отсеивает служебные страницы: листинги по тегам (section …/tag(s)/…), архивы и
// страницы с пустым/служебным заголовком — оставляет только статьи (news / <slug>).
func filterNewsPages(pages []*sitepages.Page, limit int) []*sitepages.Page {
	var news []*sitepages.Page
	for _, p := range pages {
		sec := strings.ToLower(p.Section)
		url := strings.ToLower(p.URL)
		isNews := strings.Contains(sec, "news") || strings.Contains(url, "/news")
		if !isNews {
			continue
		}
		// Отсекаем страницы тегов и служебные заголовки.
		if strings.Contains(sec, "tag") {
			continue
		}
		title := strings.ToLower(strings.TrimSpace(p.Title))
		if newsTitleBlacklist[title] || strings.HasPrefix(title, "информация по тегу") {
			continue
		}
		news = append(news, p)
	}
	sort.Slice(news, func(i, j int) bool {
		return newsTime(news[i]).After(newsTime(news[j]))
	})
	if len(news) > limit {
		news = news[:limit]
	}
	return news
}

// newsTime возвращает дату для сортировки новостей: published_at либо last_changed.
func newsTime(p *sitepages.Page) time.Time {
	if p.PublishedAt != nil {
		return *p.PublishedAt
	}
	return p.LastChanged
}

// pageTitle возвращает заголовок страницы либо URL как фолбэк.
func pageTitle(p *sitepages.Page) string {
	if strings.TrimSpace(p.Title) != "" {
		return p.Title
	}
	return p.URL
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

	// Если подключён модуль проверки — сразу проверяем компанию по ИНН.
	if b.eligibility != nil {
		b.sendReply(chatID, "🔍 Проверяю компанию по ИНН…")
		b.checkEligibility(chatID, arg)
		return
	}

	b.sendReplyWithKeyboard(chatID,
		fmt.Sprintf("✅ ИНН `%s` сохранён. Теперь ответы будут учитывать вашу компанию.\n\nЗадайте вопрос — например: _Какие документы нужны для статуса резидента?_", arg),
		BackToMenuKeyboard())
}

// checkEligibility проверяет компанию по ИНН через модуль eligibility и выводит отчёт.
func (b *Bot) checkEligibility(chatID int64, inn string) {
	rep, err := b.eligibility.CheckByINN(context.Background(), inn)
	if err != nil {
		b.sendReplyWithKeyboard(chatID,
			fmt.Sprintf("✅ ИНН `%s` сохранён.\n\n⚠️ Автоматическую проверку компании выполнить не удалось (%v). ИНН учтётся при ответах консультанта.", inn, err),
			BackToMenuKeyboard())
		return
	}

	var sb strings.Builder
	if rep.Company != nil && rep.Company.ShortName != "" {
		sb.WriteString(fmt.Sprintf("🏢 *%s*\n", rep.Company.ShortName))
		if rep.Company.Status != "" {
			sb.WriteString(fmt.Sprintf("Статус: %s\n", rep.Company.Status))
		}
		if rep.Company.IsMSP {
			sb.WriteString("✅ В реестре МСП\n")
		}
		sb.WriteString("\n")
	}

	if rep.Eligible {
		sb.WriteString(fmt.Sprintf("✅ *Компания подходит под критерии Сколково* (оценка %d/100)\n", rep.Score))
	} else {
		sb.WriteString(fmt.Sprintf("⚠️ *Есть ограничения для резидентства* (оценка %d/100)\n", rep.Score))
	}
	for _, is := range rep.Issues {
		sb.WriteString(fmt.Sprintf("\n🔴 %s", is))
	}
	for _, w := range rep.Warnings {
		sb.WriteString(fmt.Sprintf("\n🟡 %s", w))
	}
	for _, rec := range rep.Recommendations {
		sb.WriteString(fmt.Sprintf("\n💡 %s", rec))
	}
	sb.WriteString("\n\n_Предварительная оценка по открытым данным. Точный статус определяет Фонд «Сколково»._")
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
		"/company — проверить компанию по ИНН и учитывать её в ответах\n" +
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
