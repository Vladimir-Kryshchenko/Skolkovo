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
