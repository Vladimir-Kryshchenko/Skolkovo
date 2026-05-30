package tgbot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ---------------------------------------------------------------------------
// MainKeyboard — главное меню бота.
// ---------------------------------------------------------------------------

// MainKeyboard возвращает inline-клавиатуру с основными командами.
func MainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Мой статус", "cmd:status"),
			tgbotapi.NewInlineKeyboardButtonData("⏰ Дедлайны", "cmd:deadlines"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📄 Документы", "cmd:docs"),
			tgbotapi.NewInlineKeyboardButtonData("❓ Задать вопрос", "cmd:ask"),
		),
	)
}

// ---------------------------------------------------------------------------
// DeadlineKeyboard — навигация по страницам дедлайнов.
// ---------------------------------------------------------------------------

// DeadlineKeyboard возвращает клавиатуру для пагинации дедлайнов.
func DeadlineKeyboard(page, totalPages int) tgbotapi.InlineKeyboardMarkup {
	row := []tgbotapi.InlineKeyboardButton{}

	if page > 0 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("dl:prev:%d", page)))
	}
	if page < totalPages-1 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("Вперёд ➡️", fmt.Sprintf("dl:next:%d", page)))
	}

	if len(row) == 0 {
		return tgbotapi.NewInlineKeyboardMarkup()
	}
	return tgbotapi.NewInlineKeyboardMarkup(row)
}

// ---------------------------------------------------------------------------
// StageKeyboard — информация о стадии резидентства.
// ---------------------------------------------------------------------------

// StageKeyboard возвращает клавиатуру с информацией о текущей стадии.
func StageKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Чек-листы", "cmd:checklists"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📖 Справка о стадии", "cmd:stage_info"),
		),
	)
}

// ---------------------------------------------------------------------------
// DocsKeyboard — клавиатура для навигации по документам.
// ---------------------------------------------------------------------------

// DocsKeyboard возвращает клавиатуру для пагинации документов.
func DocsKeyboard(page, totalPages int) tgbotapi.InlineKeyboardMarkup {
	row := []tgbotapi.InlineKeyboardButton{}

	if page > 0 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", fmt.Sprintf("docs:prev:%d", page)))
	}
	if page < totalPages-1 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("Вперёд ➡️", fmt.Sprintf("docs:next:%d", page)))
	}

	if len(row) == 0 {
		return tgbotapi.NewInlineKeyboardMarkup()
	}
	return tgbotapi.NewInlineKeyboardMarkup(row)
}
