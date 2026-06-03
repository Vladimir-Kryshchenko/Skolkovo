// inn.go — опциональная привязка ИНН компании к чату (в памяти).
//
// Бот публичный и анонимный: ИНН не обязателен и не даёт доступа к личному кабинету
// резидента. Он используется только как контекст к вопросам ИИ-консультанту
// («компания пользователя — ИНН XXX»), чтобы ответы были чуть точнее. Привязка
// живёт в памяти процесса и сбрасывается при перезапуске — это нормально для
// информационного бота.
package tgbot

import "regexp"

// innRe — ИНН организации (10 цифр) или ИП (12 цифр).
var innRe = regexp.MustCompile(`^\d{10}$|^\d{12}$`)

// setINN сохраняет ИНН компании для чата.
func (b *Bot) setINN(chatID int64, inn string) {
	b.innMu.Lock()
	b.chatINN[chatID] = inn
	b.innMu.Unlock()
}

// getINN возвращает привязанный ИНН чата (пусто, если не задан).
func (b *Bot) getINN(chatID int64) string {
	b.innMu.RLock()
	defer b.innMu.RUnlock()
	return b.chatINN[chatID]
}

// clearINN сбрасывает привязку ИНН.
func (b *Bot) clearINN(chatID int64) {
	b.innMu.Lock()
	delete(b.chatINN, chatID)
	b.innMu.Unlock()
}
