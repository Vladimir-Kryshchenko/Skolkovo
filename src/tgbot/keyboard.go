package tgbot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ---------------------------------------------------------------------------
// ReplyMenuKeyboard — постоянная reply-клавиатура (нижняя панель Telegram).
// Показывается всегда после авторизации; каждая кнопка отправляет команду.
// ---------------------------------------------------------------------------

// ReplyMenuKeyboard возвращает постоянную reply-клавиатуру главного меню.
func ReplyMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Статус"),
			tgbotapi.NewKeyboardButton("⏰ Дедлайны"),
			tgbotapi.NewKeyboardButton("📋 Чек-листы"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📄 Документы"),
			tgbotapi.NewKeyboardButton("❓ Задать вопрос"),
			tgbotapi.NewKeyboardButton("ℹ️ Помощь"),
		),
	)
}

// ---------------------------------------------------------------------------
// MainKeyboard — inline-меню (показывается как дополнение к reply-клавиатуре).
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
			tgbotapi.NewInlineKeyboardButtonData("📋 Чек-листы", "cmd:checklists"),
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

	navRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("← Меню", "cmd:menu"),
	)
	if len(row) == 0 {
		return tgbotapi.NewInlineKeyboardMarkup(navRow)
	}
	return tgbotapi.NewInlineKeyboardMarkup(row, navRow)
}

// ---------------------------------------------------------------------------
// StageKeyboard — информация о стадии резидентства.
// ---------------------------------------------------------------------------

// StageKeyboard возвращает клавиатуру с информацией о текущей стадии.
func StageKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Чек-листы", "cmd:checklists"),
			tgbotapi.NewInlineKeyboardButtonData("📖 О стадии", "cmd:stage_info"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Меню", "cmd:menu"),
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

	navRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("← Меню", "cmd:menu"),
	)
	if len(row) == 0 {
		return tgbotapi.NewInlineKeyboardMarkup(navRow)
	}
	return tgbotapi.NewInlineKeyboardMarkup(row, navRow)
}
