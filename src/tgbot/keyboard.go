package tgbot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ---------------------------------------------------------------------------
// ReplyMenuKeyboard — постоянная reply-клавиатура (нижняя панель Telegram).
// Показывается всегда; каждая кнопка отправляет текст, который ловит handleMessage.
// ---------------------------------------------------------------------------

// ReplyMenuKeyboard возвращает постоянную reply-клавиатуру главного меню.
func ReplyMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🤖 Задать вопрос"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📅 Мероприятия"),
			tgbotapi.NewKeyboardButton("🏆 Конкурсы"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📰 Новости"),
			tgbotapi.NewKeyboardButton("❓ FAQ"),
			tgbotapi.NewKeyboardButton("ℹ️ Помощь"),
		),
	)
}

// ---------------------------------------------------------------------------
// MainKeyboard — inline-меню разделов (дублирует reply-меню для удобства).
// ---------------------------------------------------------------------------

// MainKeyboard возвращает inline-клавиатуру с разделами.
func MainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🤖 Задать вопрос", "cmd:ask"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📅 Мероприятия", "cmd:events"),
			tgbotapi.NewInlineKeyboardButtonData("🏆 Конкурсы", "cmd:contests"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📰 Новости", "cmd:news"),
			tgbotapi.NewInlineKeyboardButtonData("❓ FAQ", "cmd:faq"),
		),
	)
}

// BackToMenuKeyboard возвращает inline-кнопку «← Главное меню».
func BackToMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Главное меню", "cmd:menu"),
		),
	)
}
