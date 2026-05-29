package agents

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// ChangeNotification — уведомление об изменении в документах.
type ChangeNotification struct {
	// DocumentID — идентификатор изменённого документа.
	DocumentID string `json:"document_id"`
	// Title — заголовок документа.
	Title string `json:"title"`
	// ChangeType — тип изменения: "new", "updated", "status_changed", "outdated".
	ChangeType string `json:"change_type"`
	// Description — описание изменения.
	Description string `json:"description"`
	// AffectedClients — клиенты, на которых влияет изменение.
	AffectedClients []string `json:"affected_clients,omitempty"`
	// DetectedAt — время обнаружения изменения.
	DetectedAt time.Time `json:"detected_at"`
	// Category — категория документа.
	Category string `json:"category,omitempty"`
}

// DigestReport — сводный отчёт изменений за период.
type DigestReport struct {
	// ClientID — идентификатор клиента, для которого сформирован отчёт.
	ClientID string `json:"client_id"`
	// Period — период, за который сформирован отчёт.
	Period string `json:"period"`
	// TotalChanges — общее количество изменений.
	TotalChanges int `json:"total_changes"`
	// ImportantChanges — важные изменения, требующие внимания.
	ImportantChanges []ChangeNotification `json:"important_changes"`
	// Summary — текстовая сводка.
	Summary string `json:"summary"`
	// GeneratedAt — время генерации отчёта.
	GeneratedAt time.Time `json:"generated_at"`
}

// MonitorStores — набор хранилищ, необходимых агенту-монитору.
type MonitorStores struct {
	DocStore      store.Store
	EventStore    store.EventStore
	ContestStore  store.ContestStore
	ClientStore   store.ClientStore
	DeadlineStore store.DeadlineStore
}

// MonitorAgent — агент-монитор, отслеживающий изменения в документах и генерирующий уведомления.
type MonitorAgent struct {
	docStore      store.Store
	eventStore    store.EventStore
	contestStore  store.ContestStore
	clientStore   store.ClientStore
	deadlineStore store.DeadlineStore

	// Подписки клиентов на категории уведомлений (in-memory для MVP).
	mu            sync.RWMutex
	subscriptions map[string]map[string]bool // clientID -> category -> enabled
}

// NewMonitorAgent создаёт агента-монитор.
func NewMonitorAgent(stores MonitorStores) *MonitorAgent {
	return &MonitorAgent{
		docStore:      stores.DocStore,
		eventStore:    stores.EventStore,
		contestStore:  stores.ContestStore,
		clientStore:   stores.ClientStore,
		deadlineStore: stores.DeadlineStore,
		subscriptions: make(map[string]map[string]bool),
	}
}

// CheckForChanges проверяет новые/обновлённые документы с указанного времени.
//
// Для каждого изменения:
//  1. Генерирует уведомление.
//  2. Анализирует влияние на клиентов (через стадию и чек-листы).
func (a *MonitorAgent) CheckForChanges(ctx context.Context, since time.Time) ([]ChangeNotification, error) {
	// Получаем все действующие документы.
	docs, err := a.docStore.List(ctx, store.Filter{Status: model.StatusActive})
	if err != nil {
		return nil, fmt.Errorf("получение документов: %w", err)
	}

	var notifications []ChangeNotification

	for _, doc := range docs {
		// Определяем тип изменения.
		changeType := classifyChange(doc, since)
		if changeType == "" {
			continue // Документ не изменился с момента `since`.
		}

		// Генерируем уведомление.
		notif := ChangeNotification{
			DocumentID:  doc.ID,
			Title:       doc.Title,
			ChangeType:  changeType,
			Description: formatChangeDescription(changeType, doc),
			DetectedAt:  time.Now(),
			Category:    doc.Category,
		}

		// Анализируем влияние на клиентов.
		affectedClients, err := a.analyzeImpact(ctx, doc)
		if err != nil {
			// Не критично, продолжаем без анализа влияния.
		}
		notif.AffectedClients = affectedClients

		notifications = append(notifications, notif)
	}

	return notifications, nil
}

// GenerateDigest формирует сводку изменений за N дней, релевантных клиенту.
//
// Формат: "За неделю изменилось X, вам важно Y".
func (a *MonitorAgent) GenerateDigest(ctx context.Context, clientID string, days int) (DigestReport, error) {
	if days <= 0 {
		days = 7
	}

	since := time.Now().AddDate(0, 0, -days)
	changes, err := a.CheckForChanges(ctx, since)
	if err != nil {
		return DigestReport{}, fmt.Errorf("проверка изменений: %w", err)
	}

	// Фильтруем изменения по подпискам клиента.
	subs := a.GetSubscriptions(ctx, clientID)
	filtered := filterBySubscriptions(changes, subs)

	// Получаем стадию клиента для контекста.
	var stageContext string
	if a.clientStore != nil {
		client, err := a.clientStore.GetClient(ctx, clientID)
		if err == nil {
			stageContext = string(client.ResidencyStage)
		}
	}

	// Формируем сводку.
	summary := generateSummary(clientID, len(filtered), days, stageContext, filtered)

	return DigestReport{
		ClientID:         clientID,
		Period:           fmt.Sprintf("последние %d дней", days),
		TotalChanges:     len(changes),
		ImportantChanges: filtered,
		Summary:          summary,
		GeneratedAt:      time.Now(),
	}, nil
}

// GetSubscriptions возвращает список категорий, на которые подписан клиент.
func (a *MonitorAgent) GetSubscriptions(_ context.Context, clientID string) []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	subs := a.subscriptions[clientID]
	if len(subs) == 0 {
		// По умолчанию — подписка на все категории.
		return []string{"regulations", "events", "contests", "reporting"}
	}

	var result []string
	for cat, enabled := range subs {
		if enabled {
			result = append(result, cat)
		}
	}
	return result
}

// Subscribe подписывает клиента на указанные категории уведомлений.
func (a *MonitorAgent) Subscribe(_ context.Context, clientID string, categories []string) error {
	if len(categories) == 0 {
		return fmt.Errorf("категории не указаны")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.subscriptions[clientID] == nil {
		a.subscriptions[clientID] = make(map[string]bool)
	}

	for _, cat := range categories {
		a.subscriptions[clientID][strings.TrimSpace(cat)] = true
	}

	return nil
}

// Unsubscribe отписывает клиента от указанных категорий уведомлений.
func (a *MonitorAgent) Unsubscribe(_ context.Context, clientID string, categories []string) error {
	if len(categories) == 0 {
		return fmt.Errorf("категории не указаны")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	subs := a.subscriptions[clientID]
	if subs == nil {
		return nil // Уже не подписан.
	}

	for _, cat := range categories {
		delete(subs, strings.TrimSpace(cat))
	}

	return nil
}

// classifyChange определяет тип изменения документа относительно времени `since`.
func classifyChange(doc model.Document, since time.Time) string {
	// Новый документ: загружен после `since`.
	if doc.FetchedAt.After(since) {
		return "new"
	}

	// Обновлённый документ: опубликован/обновлён на источнике после `since`.
	if doc.PublishedAt != nil && doc.PublishedAt.After(since) {
		return "updated"
	}

	// Сменился статус на "устарел".
	if doc.Status == model.StatusOutdated {
		return "outdated"
	}

	return ""
}

// formatChangeDescription формирует описание изменения.
func formatChangeDescription(changeType string, doc model.Document) string {
	switch changeType {
	case "new":
		return fmt.Sprintf("Новый документ в базе: %s", doc.Title)
	case "updated":
		return fmt.Sprintf("Обновлён документ: %s", doc.Title)
	case "status_changed":
		return fmt.Sprintf("Изменён статус документа %s на %s", doc.Title, doc.Status)
	case "outdated":
		return fmt.Sprintf("Документ утратил силу: %s", doc.Title)
	default:
		return fmt.Sprintf("Изменение в документе: %s", doc.Title)
	}
}

// analyzeImpact анализирует, на каких клиентов влияет изменение документа.
func (a *MonitorAgent) analyzeImpact(ctx context.Context, doc model.Document) ([]string, error) {
	if a.clientStore == nil {
		return nil, nil
	}

	// Получаем всех клиентов (пустой tenantID и stage — берём всех).
	clients, err := a.clientStore.ListClients(ctx, "", "")
	if err != nil {
		return nil, err
	}

	var affected []string
	for _, client := range clients {
		if isClientAffected(client, doc) {
			affected = append(affected, client.ID)
		}
	}

	return affected, nil
}

// isClientAffected проверяет, влияет ли документ на клиента.
func isClientAffected(client *model.Client, doc model.Document) bool {
	// На стадии "отчётность" важны документы категории "reporting".
	if client.ResidencyStage == model.StageReporting {
		if strings.Contains(strings.ToLower(doc.Category), "отчёт") ||
			strings.Contains(strings.ToLower(doc.Category), "reporting") {
			return true
		}
	}

	// На стадии "продление" важны документы категории "extension".
	if client.ResidencyStage == model.StageExtension {
		if strings.Contains(strings.ToLower(doc.Category), "продлен") ||
			strings.Contains(strings.ToLower(doc.Category), "extension") {
			return true
		}
	}

	// На стадии "подача_заявки" и "экспертиза" важны документы категории "entry".
	if client.ResidencyStage == model.StageApplication ||
		client.ResidencyStage == model.StageExamination {
		if strings.Contains(strings.ToLower(doc.Category), "заявк") ||
			strings.Contains(strings.ToLower(doc.Category), "entry") {
			return true
		}
	}

	// На стадии "выход" важны документы категории "exit".
	if client.ResidencyStage == model.StageExit {
		if strings.Contains(strings.ToLower(doc.Category), "выход") ||
			strings.Contains(strings.ToLower(doc.Category), "exit") {
			return true
		}
	}

	return false
}

// filterBySubscriptions фильтрует изменения по подпискам клиента.
func filterBySubscriptions(changes []ChangeNotification, subscriptions []string) []ChangeNotification {
	if len(subscriptions) == 0 {
		return changes
	}

	subSet := make(map[string]bool, len(subscriptions))
	for _, s := range subscriptions {
		subSet[strings.ToLower(s)] = true
	}

	var filtered []ChangeNotification
	for _, ch := range changes {
		if subSet[strings.ToLower(ch.Category)] || subSet["all"] {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}

// generateSummary формирует текстовую сводку.
func generateSummary(clientID string, changeCount, days int, stage string, changes []ChangeNotification) string {
	if changeCount == 0 {
		return fmt.Sprintf("За последние %d дней изменений не обнаружено.", days)
	}

	important := len(changes)
	var importantMsg string
	if important > 0 {
		importantMsg = fmt.Sprintf(", из них %d требуют вашего внимания", important)
	}

	stageMsg := ""
	if stage != "" {
		stageMsg = fmt.Sprintf(" (ваша текущая стадия: %s)", stage)
	}

	summary := fmt.Sprintf(
		"За последние %d дней обнаружено %d изменений%s%s. ",
		days, changeCount, importantMsg, stageMsg,
	)

	if important > 0 {
		summary += fmt.Sprintf("Вам важно ознакомиться с изменениями в следующих документах: ")
		for i, ch := range changes {
			if i >= 3 {
				break // Максимум 3 в сводке.
			}
			if i > 0 {
				summary += ", "
			}
			summary += fmt.Sprintf("«%s»", ch.Title)
		}
	}

	return summary
}
