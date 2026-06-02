// status_labels.go — канонические русские подписи статусов клиентских документов.
// Единый источник истины для всех каналов (Портал, Telegram-бот, MCP), чтобы
// подписи не расходились между интерфейсами.
package model

// docStatusLabels — русские подписи статусов клиентского документа.
var docStatusLabels = map[ClientDocStatus]string{
	DocPending:   "Ожидает",
	DocSubmitted: "Отправлен",
	DocApproved:  "Утверждён",
	DocRejected:  "Отклонён",
}

// DocStatusLabel возвращает русскую подпись статуса клиентского документа
// (или саму строку статуса, если она неизвестна).
func DocStatusLabel(s ClientDocStatus) string {
	if l, ok := docStatusLabels[s]; ok {
		return l
	}
	return string(s)
}
