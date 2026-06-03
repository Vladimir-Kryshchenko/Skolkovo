// manager.go — мультибот-менеджер: поднимает по одному Telegram-боту на каждый
// активный тенант с заданным токеном и периодически сверяет состояние (reconcile).
//
// Логика выбора токена:
//   - у тенанта задан telegram_bot_token → поднимаем бота тенанта;
//   - токен пуст, но это тенант Default → используем глобальный TELEGRAM_BOT_TOKEN;
//   - если глобальный токен задан, но ни один тенант его не «забрал» → поднимаем
//     один глобальный бот без привязки к тенанту (обратная совместимость).
//
// Reconcile (по тикеру) обрабатывает изменения без рестарта процесса:
// новый токен → старт, изменился → рестарт, убран/деактивирован → стоп.
package tgbot

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"baza-skolkovo/src/agents"
	"baza-skolkovo/src/common/store"
)

// defaultPollInterval — как часто менеджер сверяет тенантов с запущенными ботами.
const defaultPollInterval = 45 * time.Second

// globalBotKey — ключ для глобального бота (без тенанта) в map running.
const globalBotKey = "__global__"

// botHandle — дескриптор запущенного бота.
type botHandle struct {
	cancel context.CancelFunc
	token  string
	done   chan struct{}
}

// botSpec — желаемое состояние одного бота.
type botSpec struct {
	key          string // tenantID или globalBotKey
	tenantID     string // "" для глобального бота
	token        string
	mcpKey       string
	prevUsername string // ранее сохранённый @username тенанта (для сравнения)
}

// BotManager управляет набором ботов (по одному на тенанта с токеном).
type BotManager struct {
	tenants       store.TenantStore
	stores        Stores
	consultant    *agents.ConsultantAgent
	mcpURL        string
	fallbackKey   string // глобальный MCP API-ключ (если у тенанта нет своего)
	fallbackToken string // глобальный TELEGRAM_BOT_TOKEN (fallback для Default)
	pollEvery     time.Duration

	mu      sync.Mutex
	running map[string]*botHandle // key → бот
}

// NewBotManager создаёт менеджер ботов.
func NewBotManager(tenants store.TenantStore, stores Stores, consultant *agents.ConsultantAgent,
	mcpURL, fallbackKey, fallbackToken string) *BotManager {
	return &BotManager{
		tenants:       tenants,
		stores:        stores,
		consultant:    consultant,
		mcpURL:        mcpURL,
		fallbackKey:   fallbackKey,
		fallbackToken: fallbackToken,
		pollEvery:     defaultPollInterval,
		running:       make(map[string]*botHandle),
	}
}

// Run запускает менеджер: первичный reconcile + периодическая сверка.
// Блокирует до отмены контекста.
func (m *BotManager) Run(ctx context.Context) error {
	if m.tenants == nil {
		log.Printf("[tgbot:manager] TenantStore не задан — мультибот недоступен")
		return nil
	}
	m.reconcile(ctx)
	t := time.NewTicker(m.pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return ctx.Err()
		case <-t.C:
			m.reconcile(ctx)
		}
	}
}

// reconcile сверяет желаемое состояние (активные тенанты с токеном) с запущенными
// ботами: запускает новых, перезапускает при смене токена, останавливает лишних.
func (m *BotManager) reconcile(ctx context.Context) {
	desired := m.desiredBots(ctx)
	if desired == nil {
		return // ошибка чтения тенантов — оставляем текущее состояние
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Остановить боты, которых больше нет в желаемом состоянии.
	for key, h := range m.running {
		if _, ok := desired[key]; !ok {
			log.Printf("[tgbot:manager] остановка бота key=%s (тенант убран/деактивирован/без токена)", key)
			h.cancel()
			delete(m.running, key)
		}
	}

	// Запустить новых / перезапустить при смене токена.
	for key, spec := range desired {
		if h, ok := m.running[key]; ok {
			if h.token == spec.token {
				continue // уже работает с тем же токеном
			}
			log.Printf("[tgbot:manager] токен изменился key=%s — перезапуск", key)
			h.cancel()
			delete(m.running, key)
		}
		m.startLocked(ctx, spec)
	}
}

// desiredBots строит желаемый набор ботов из активных тенантов.
// Возвращает nil при ошибке чтения (чтобы reconcile не трогал текущее состояние).
func (m *BotManager) desiredBots(ctx context.Context) map[string]botSpec {
	tenants, err := m.tenants.ListTenants(ctx)
	if err != nil {
		log.Printf("[tgbot:manager] ошибка чтения тенантов: %v", err)
		return nil
	}

	desired := make(map[string]botSpec)
	fallbackConsumed := false

	for _, t := range tenants {
		if !t.Active {
			continue
		}
		token := strings.TrimSpace(t.TelegramBotToken)
		if token == "" {
			// Тенант Default без своего токена наследует глобальный.
			if strings.EqualFold(t.Name, store.DefaultTenantName) && m.fallbackToken != "" {
				token = m.fallbackToken
				fallbackConsumed = true
			} else {
				continue
			}
		}
		desired[t.ID] = botSpec{
			key:          t.ID,
			tenantID:     t.ID,
			token:        token,
			mcpKey:       m.mcpKeyFor(t.APIKey),
			prevUsername: t.TelegramBotUsername,
		}
	}

	// Глобальный токен задан, но ни один тенант его не забрал → один глобальный бот.
	if m.fallbackToken != "" && !fallbackConsumed {
		desired[globalBotKey] = botSpec{
			key:    globalBotKey,
			token:  m.fallbackToken,
			mcpKey: m.fallbackKey,
		}
	}

	return desired
}

// startLocked запускает бота по спецификации (вызывать под m.mu).
func (m *BotManager) startLocked(ctx context.Context, spec botSpec) {
	cfg := BotConfig{
		Token:     spec.token,
		TenantID:  spec.tenantID,
		MCPURL:    m.mcpURL,
		MCPAPIKey: spec.mcpKey,
	}

	bot, err := NewBot(cfg, m.stores, m.consultant)
	if err != nil {
		log.Printf("[tgbot:manager] ошибка создания бота key=%s: %v", spec.key, err)
		return // не добавляем в running → повтор на следующем reconcile
	}

	// Сохраняем @username бота для отображения статуса в админке.
	m.persistUsername(ctx, spec, bot.Username())

	botCtx, cancel := context.WithCancel(ctx)
	h := &botHandle{cancel: cancel, token: spec.token, done: make(chan struct{})}
	m.running[spec.key] = h

	go func() {
		defer close(h.done)
		go bot.RunNotifier(botCtx)
		if err := bot.Run(botCtx); err != nil && err != context.Canceled {
			log.Printf("[tgbot:manager] бот key=%s остановлен: %v", spec.key, err)
		}
	}()
	log.Printf("[tgbot:manager] запущен бот @%s key=%s", bot.Username(), spec.key)
}

// persistUsername сохраняет @username бота в тенанте, если он изменился.
func (m *BotManager) persistUsername(ctx context.Context, spec botSpec, username string) {
	if spec.tenantID == "" || username == "" {
		return
	}
	if spec.prevUsername == username {
		return
	}
	t, err := m.tenants.GetTenant(ctx, spec.tenantID)
	if err != nil || t == nil {
		return
	}
	t.TelegramBotUsername = username
	if err := m.tenants.UpdateTenant(ctx, t); err != nil {
		log.Printf("[tgbot:manager] не удалось сохранить username тенанта %s: %v", spec.tenantID, err)
	}
}

// mcpKeyFor возвращает MCP-ключ для бота: собственный ключ тенанта или глобальный.
func (m *BotManager) mcpKeyFor(tenantKey string) string {
	if strings.TrimSpace(tenantKey) != "" {
		return tenantKey
	}
	return m.fallbackKey
}

// stopAll останавливает все боты.
func (m *BotManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, h := range m.running {
		h.cancel()
		delete(m.running, key)
	}
}
