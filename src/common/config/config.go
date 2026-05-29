// Package config загружает конфигурацию сервиса из переменных окружения.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config — настройки всех подсистем «База Сколково».
type Config struct {
	PostgresDSN     string
	QdrantURL       string
	QdrantColl      string
	TEIURL          string
	EmbeddingDim    int
	SourceURL       string
	DocsDir         string
	StoreBackend    string // "json" | "postgres"
	RegistryPath    string // путь к JSON-реестру (для backend=json)
	MCPAddr         string
	MCPAPIKey       string
	MCPRateLimitRPS int
	AdminAddr       string
	AdminUser       string
	AdminPassword   string
	ReportDir       string
	ScrapeInterval  time.Duration
	ScrapeMaxPages  int
	ScrapeDelay     time.Duration
	NewsRSSURL      string
	NotifyWebhook   string
	ChromePath      string // путь к Chrome для headless-загрузчика ("" — автоопределение)
	ProxyURL        string // прокси для загрузчика (например, резидентный) — обход WAF
	FetchLimit      int    // сколько документов скачивать за прогон (0 — все)
	FetchWait       time.Duration

	// Мероприятия
	EventsRSSURL    string
	EventsSourceURL string

	// Конкурсы и гранты
	ContestsURL string
	GrantsURL   string

	// FAQ
	FAQURL string

	// Telegram-каналы
	TelegramChannels  string // comma-separated список каналов
	TelegramRssHubURL string // URL RSSHub-инстанса для Telegram

	// Портал клиента
	PortalEnabled bool
	PortalAddr    string

	// Чат-виджет
	ChatWidgetEnabled bool
	ChatWidgetAddr    string

	// Классификатор документов
	ClassifierEnabled bool

	// Льготы резидентов (preferences)
	PreferencesEnabled bool
	PreferencesURL     string // основной URL страницы льгот

	// НПА (regulations)
	RegulationsEnabled    bool
	RegulationsSearchURL  string // URL поиска на regulation.gov.ru
	RegulationsExtraURLs  string // дополнительные URL через запятую

	// Проверка eligibility по ИНН
	EligibilityEnabled bool
	DadataAPIKey       string // API-ключ DaData для ЕГРЮЛ

	// Telegram-уведомления консультанту
	ConsultantTelegramChatID int64 // chat_id консультанта (не клиентский бот)

	// Консультантский дашборд
	ConsultantAddr    string // адрес для консультантского дашборда
	ConsultantEnabled bool

	// Аудит-лог MCP
	MCPAuditEnabled bool

	// Ежедневная сводка консультанту
	DailySummaryHour int // час отправки ежедневной сводки (0-23, по умолчанию 9)
}

// Load читает конфигурацию из окружения, подставляя разумные значения по умолчанию.
func Load() Config {
	return Config{
		PostgresDSN:     env("POSTGRES_DSN", "postgres://skolkovo:skolkovo@localhost:5432/skolkovo?sslmode=disable"),
		QdrantURL:       env("QDRANT_URL", "http://localhost:6333"),
		QdrantColl:      env("QDRANT_COLLECTION", "skolkovo_docs"),
		TEIURL:          env("TEI_URL", "http://localhost:8081"),
		EmbeddingDim:    envInt("EMBEDDING_DIM", 768),
		SourceURL:       env("SOURCE_URL", "https://dochub.sk.ru/foundation/documents/"),
		DocsDir:         env("DOCS_DIR", "./Документы_Сколково"),
		StoreBackend:    env("STORE_BACKEND", "json"),
		RegistryPath:    env("REGISTRY_PATH", "./Документы_Сколково/Метаданные/реестр_документов.json"),
		MCPAddr:         env("MCP_ADDR", ":8080"),
		MCPAPIKey:       env("MCP_API_KEY", ""),
		MCPRateLimitRPS: envInt("MCP_RATE_LIMIT_RPS", 5),
		AdminAddr:       env("ADMIN_ADDR", ":8090"),
		AdminUser:       env("ADMIN_USER", ""),
		AdminPassword:   env("ADMIN_PASSWORD", ""),
		ReportDir:       env("REPORT_DIR", "./Аналитика/Отчеты/Отчеты_парсинга"),
		ScrapeInterval:  envDuration("SCRAPE_INTERVAL", 6*time.Hour),
		ScrapeMaxPages:  envInt("SCRAPE_MAX_PAGES", 200),
		ScrapeDelay:     envDuration("SCRAPE_DELAY", 3*time.Second),
		NewsRSSURL:      env("NEWS_RSS_URL", "https://sk.ru/news/rss/"),
		NotifyWebhook:   env("NOTIFY_WEBHOOK", ""),
		ChromePath:      env("CHROME_PATH", ""),
		ProxyURL:        env("PROXY_URL", ""),
		FetchLimit:      envInt("FETCH_LIMIT", 0),
		FetchWait:       envDuration("FETCH_WAIT", 7*time.Second),

		// Мероприятия
		EventsRSSURL:    env("EVENTS_RSS_URL", ""),
		EventsSourceURL: env("EVENTS_SOURCE_URL", "https://sk.ru/events/"),

		// Конкурсы и гранты
		ContestsURL: env("CONTESTS_URL", "https://sk.ru/foundation/contests/"),
		GrantsURL:   env("GRANTS_URL", "https://sk.ru/foundation/grants/"),

		// FAQ
		FAQURL: env("FAQ_URL", "https://sk.ru/foundation/faq/"),

		// Telegram-каналы
		TelegramChannels:  env("TELEGRAM_CHANNELS", ""),
		TelegramRssHubURL: env("TELEGRAM_RSS_HUB_URL", "https://rsshub.rssforever.com/telegram/channel/"),

		// Портал клиента
		PortalEnabled: envBool("PORTAL_ENABLED", false),
		PortalAddr:    env("PORTAL_ADDR", ":8092"),

		// Чат-виджет
		ChatWidgetEnabled: envBool("CHAT_WIDGET_ENABLED", false),
		ChatWidgetAddr:    env("CHAT_WIDGET_ADDR", ":8093"),

		// Классификатор документов
		ClassifierEnabled: envBool("CLASSIFIER_ENABLED", false),

		// Льготы резидентов
		PreferencesEnabled: envBool("PREFERENCES_ENABLED", true),
		PreferencesURL:     env("PREFERENCES_URL", "https://sk.ru/residents/preferences/"),

		// НПА
		RegulationsEnabled:   envBool("REGULATIONS_ENABLED", true),
		RegulationsSearchURL: env("REGULATIONS_SEARCH_URL", "https://regulation.gov.ru/Regulation/Npa/Search"),
		RegulationsExtraURLs: env("REGULATIONS_EXTRA_URLS", ""),

		// Eligibility
		EligibilityEnabled: envBool("ELIGIBILITY_ENABLED", true),
		DadataAPIKey:       env("DADATA_API_KEY", ""),

		// Telegram-уведомления консультанту
		ConsultantTelegramChatID: envInt64("CONSULTANT_TELEGRAM_CHAT_ID", 0),

		// Консультантский дашборд
		ConsultantAddr:    env("CONSULTANT_ADDR", ":8094"),
		ConsultantEnabled: envBool("CONSULTANT_ENABLED", true),

		// Аудит-лог MCP
		MCPAuditEnabled: envBool("MCP_AUDIT_ENABLED", true),

		// Ежедневная сводка
		DailySummaryHour: envInt("DAILY_SUMMARY_HOUR", 9),
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
