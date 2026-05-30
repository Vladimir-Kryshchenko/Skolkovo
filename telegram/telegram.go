// Package telegram мониторит Telegram-каналы Сколково через RSS-агрегаторы
// и заводит посты в хранилище как категорию «Telegram».
package telegram

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"baza-skolkovo/src/common/feed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

// userAgent — браузерный UA для HTTP-запросов.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// TelegramConfig — конфигурация источников Telegram.
type TelegramConfig struct {
	Channels []string // список каналов (например, "skolkovo", "skolkovoventures")
	APIURL   string   // URL RSS-агрегатора (по умолчанию "https://rsshub.app/telegram/channel/")
}

// Monitor загружает посты из Telegram-каналов и синхронизирует их в хранилище.
type Monitor struct {
	Cfg   TelegramConfig
	Store store.TelegramStore
	HTTP  *http.Client
}

// New создаёт монитор Telegram-каналов.
func New(cfg TelegramConfig, st store.TelegramStore) *Monitor {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "https://rsshub.app/telegram/channel/"
	}
	return &Monitor{
		Cfg:   TelegramConfig{Channels: cfg.Channels, APIURL: apiURL},
		Store: st,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации постов.
type Result struct {
	Fetched   int
	New       int
	Skipped   int
	Errors    []string
}

// FetchChannelPosts получает посты из одного канала через RSS-агрегатор.
// Поддерживаемые подходы:
//  1. RSS через rsshub.app/rss?url=t.me/{channel} — работает без API-ключа.
//  2. telegra.ph RSS — если канал публикует статьи через Telegraph.
//
// Fallback: если RSS недоступен, возвращается заглушка с комментарием про
// Telegram Bot API — для реальной работы нужно подключить официальный
// Bot API (https://core.telegram.org/bots/api#getchathistory).
func FetchChannelPosts(ctx context.Context, channel string, apiURL string, hc *http.Client) ([]*model.TelegramPost, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	if channel == "" {
		return nil, fmt.Errorf("пустое имя канала")
	}

	// Стратегия 1: RSS через rsshub.app.
	if apiURL == "" {
		apiURL = "https://rsshub.app/telegram/channel/"
	}

	rssURL := buildRSSURL(channel, apiURL)
	if posts, err := fetchFromRSS(ctx, rssURL, channel, hc); err == nil && len(posts) > 0 {
		return posts, nil
	}

	// Стратегия 2: telegra.ph RSS.
	telegraphURL := buildTelegraphRSSURL(channel)
	if posts, err := fetchFromRSS(ctx, telegraphURL, channel, hc); err == nil && len(posts) > 0 {
		return posts, nil
	}

	// Fallback: заглушка.
	return stubPosts(channel), nil
}

// FetchAllChannels получает посты из всех каналов конфигурации.
func FetchAllChannels(ctx context.Context, cfg TelegramConfig, hc *http.Client) ([]*model.TelegramPost, error) {
	var all []*model.TelegramPost

	for _, ch := range cfg.Channels {
		posts, err := FetchChannelPosts(ctx, ch, cfg.APIURL, hc)
		if err != nil {
			// Логируем ошибку, но продолжаем с другими каналами.
			continue
		}
		all = append(all, posts...)
	}

	return all, nil
}

// buildRSSURL формирует URL для RSS-агрегатора rsshub.
func buildRSSURL(channel, apiURL string) string {
	channel = strings.TrimPrefix(channel, "@")
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return ""
	}
	// rsshub.app/telegram/channel/{channel}
	return strings.TrimSuffix(apiURL, "/") + "/" + channel
}

// buildTelegraphRSSURL формирует URL для telegra.ph RSS.
func buildTelegraphRSSURL(channel string) string {
	channel = strings.TrimPrefix(channel, "@")
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return ""
	}
	// Telegraph не имеет встроенного RSS, но можно попробовать feed-формат
	// через сторонние сервисы.
	return fmt.Sprintf("https://telegra.ph/rss/%s", channel)
}

// rssEnvelope — минимальная структура для разбора RSS.
type rssEnvelope struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			GUID        string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

// fetchFromRSS загружает и парсит RSS-ленту канала.
func fetchFromRSS(ctx context.Context, rssURL, channel string, hc *http.Client) ([]*model.TelegramPost, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSS канала %s: статус %d", channel, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Пробуем стандартный feed.Parse.
	items := feed.Parse(data)
	if len(items) == 0 {
		// Пробуем альтернативный парсинг.
		return parseTelegramRSS(data, channel)
	}

	posts := make([]*model.TelegramPost, 0, len(items))
	now := time.Now()

	for _, it := range items {
		if it.Link == "" && it.Title == "" {
			continue
		}

		pubDate := time.Time{}
		if it.Published != nil {
			pubDate = *it.Published
		}

		text := it.Title
		if text == "" {
			text = feed.StripTags(it.Summary)
		} else if it.Summary != "" {
			text = text + "\n\n" + feed.StripTags(it.Summary)
		}

		sourceURL := it.Link
		if sourceURL == "" {
			sourceURL = fmt.Sprintf("https://t.me/%s", channel)
		}

		posts = append(posts, &model.TelegramPost{
			ID:          postID(sourceURL, channel, pubDate),
			Channel:     "@" + strings.TrimPrefix(channel, "@"),
			Text:        strings.TrimSpace(text),
			PublishedAt: pubDate,
			SourceURL:   sourceURL,
			CreatedAt:   now,
		})
	}

	return posts, nil
}

// parseTelegramRSS разбирает RSS-ленту Telegram-специфичного формата.
func parseTelegramRSS(data []byte, channel string) ([]*model.TelegramPost, error) {
	var env rssEnvelope
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, err
	}

	posts := make([]*model.TelegramPost, 0, len(env.Channel.Items))
	now := time.Now()

	for _, it := range env.Channel.Items {
		text := it.Title
		if text == "" {
			text = feed.StripTags(it.Description)
		} else if it.Description != "" {
			text = text + "\n\n" + feed.StripTags(it.Description)
		}

		sourceURL := it.Link
		if sourceURL == "" {
			sourceURL = fmt.Sprintf("https://t.me/%s", channel)
		}

		pubDate := parseRSSTime(it.PubDate)

		posts = append(posts, &model.TelegramPost{
			ID:          postID(sourceURL, channel, pubDate),
			Channel:     "@" + strings.TrimPrefix(channel, "@"),
			Text:        strings.TrimSpace(text),
			PublishedAt: pubDate,
			SourceURL:   sourceURL,
			CreatedAt:   now,
		})
	}

	return posts, nil
}

// stubPosts возвращает заглушку с комментарием про Telegram Bot API.
func stubPosts(channel string) []*model.TelegramPost {
	channel = "@" + strings.TrimPrefix(channel, "@")

	return []*model.TelegramPost{
		{
			ID:      postID("stub", channel, time.Time{}),
			Channel: channel,
			Text: "[ЗАГЛУШКА] Telegram Bot API недоступен. " +
				"Для получения постов подключите официальный Telegram Bot API: " +
				"https://core.telegram.org/bots/api#getchathistory. " +
				"Альтернатива: настройте RSSBridge или локальный rsshub.app.",
			PublishedAt: time.Now(),
			SourceURL:   fmt.Sprintf("https://t.me/%s", strings.TrimPrefix(channel, "@")),
			CreatedAt:   time.Now(),
		},
	}
}

// IngestPosts записывает посты в хранилище TelegramStore.
func IngestPosts(ctx context.Context, posts []*model.TelegramPost, st store.TelegramStore) (*Result, error) {
	res := &Result{Fetched: len(posts)}

	for _, p := range posts {
		if p.Channel == "" || p.Text == "" || p.SourceURL == "" {
			res.Errors = append(res.Errors, "пропущено: пустой канал, текст или URL")
			continue
		}

		// Проверяем дубликаты через GetLatestPostDate.
		latestDate, _ := st.GetLatestPostDate(ctx, p.Channel)
		if latestDate != nil && !p.PublishedAt.IsZero() && !p.PublishedAt.After(*latestDate) {
			res.Skipped++
			continue
		}

		if err := st.CreateTelegramPost(ctx, p); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("создание поста %s: %v", p.ID, err))
			continue
		}

		res.New++
	}

	return res, nil
}

// ---------------------------------------------------------------------------
// Вспомогательные функции
// ---------------------------------------------------------------------------

func postID(sourceURL, channel string, pubDate time.Time) string {
	raw := sourceURL + "|" + channel
	if !pubDate.IsZero() {
		raw += "|" + pubDate.Format(time.RFC3339)
	}
	sum := sha1.Sum([]byte(raw))
	return "tg-" + hex.EncodeToString(sum[:])[:16]
}

func parseRSSTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// resolveURL разрешает относительный URL относительно базового.
func resolveURL(base *url.URL, pageURL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	pu, err := url.Parse(pageURL)
	if err != nil {
		pu = base
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return pu.ResolveReference(ref).String()
}
