// Package skru — клиент публичного сайта sk.ru (новый портал на Next.js).
//
// Актуальные новости и мероприятия Фонда лежат в JSON-острове __NEXT_DATA__ на
// страницах sk.ru/news/type/{news|events}/. Старый портал dochub за anti-bot и
// содержит только архив, поэтому свежий контент берём отсюда. Сайт иногда отдаёт
// 502 — запросы идут через httpx.GetWithRetry.
package skru

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"baza-skolkovo/src/common/httpx"
)

// Post — публикация sk.ru (новость или материал о мероприятии).
type Post struct {
	ID          string
	Title       string
	ShortDesc   string
	URL         string
	PublishDate time.Time
	TypeSlug    string // news | events
	TypeName    string // «Новости» | «События»
	Tags        []string
}

// nextDataRe вырезает JSON из <script id="__NEXT_DATA__" type="application/json">…</script>.
var nextDataRe = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)

// rawPost — сырое представление публикации в __NEXT_DATA__.
// id приходит числом (74254) — берём json.Number, чтобы не зависеть от типа.
type rawPost struct {
	ID          json.Number     `json:"id"`
	PublishDate string          `json:"publish_date"`
	Title       string          `json:"title"`
	ShortDesc   string          `json:"short_desc"`
	Slug        string          `json:"slug"`
	Type        json.RawMessage `json:"type"`
	Tags        json.RawMessage `json:"tags"`
}

// nextData — корень JSON-острова (нужен только listStore.news).
type nextData struct {
	Props struct {
		PageProps struct {
			InitialProps struct {
				ListStore struct {
					News []rawPost `json:"news"`
				} `json:"listStore"`
			} `json:"initialProps"`
		} `json:"pageProps"`
	} `json:"props"`
}

// Client тянет публикации sk.ru с кэшем по типу.
type Client struct {
	http *http.Client
	ttl  time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry // typeSlug → результат
}

type cacheEntry struct {
	posts   []Post
	fetched time.Time
}

// NewClient создаёт клиента sk.ru с кэшем на ttl (0 → 15 минут).
func NewClient(ttl time.Duration) *Client {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &Client{
		http:  &http.Client{Timeout: 30 * time.Second},
		ttl:   ttl,
		cache: make(map[string]cacheEntry),
	}
}

// Posts возвращает публикации указанного типа (news|events), свежие сверху.
// Результат кэшируется на ttl; now передаётся снаружи (в проде time.Now()).
func (c *Client) Posts(ctx context.Context, typeSlug string, now time.Time) ([]Post, error) {
	c.mu.Lock()
	if e, ok := c.cache[typeSlug]; ok && now.Sub(e.fetched) < c.ttl {
		c.mu.Unlock()
		return e.posts, nil
	}
	c.mu.Unlock()

	url := "https://sk.ru/news/type/" + typeSlug + "/"
	data, err := httpx.GetWithRetry(ctx, c.http, url, httpx.DefaultUserAgent, 5)
	if err != nil {
		return nil, fmt.Errorf("sk.ru %s: %w", typeSlug, err)
	}

	posts, err := parsePosts(data)
	if err != nil {
		return nil, err
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].PublishDate.After(posts[j].PublishDate) })

	c.mu.Lock()
	c.cache[typeSlug] = cacheEntry{posts: posts, fetched: now}
	c.mu.Unlock()
	return posts, nil
}

// contestKeywords — маркеры конкурсов/грантов в заголовке/описании.
var contestKeywords = []string{
	"конкурс", "грант", "отбор", "акселерат", "challenge", "преми",
	"startup tour", "стартап-тур", "питч", "заявк",
}

// Contests возвращает актуальные конкурсы/гранты — публикации news+events,
// в заголовке/описании которых есть маркеры конкурса. Отдельного типа на sk.ru нет.
func (c *Client) Contests(ctx context.Context, now time.Time) ([]Post, error) {
	var all []Post
	for _, t := range []string{"news", "events"} {
		ps, err := c.Posts(ctx, t, now)
		if err != nil {
			continue
		}
		all = append(all, ps...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("sk.ru: не удалось получить ленту")
	}

	var out []Post
	seen := map[string]bool{}
	for _, p := range all {
		hay := strings.ToLower(p.Title + " " + p.ShortDesc)
		if !containsAny(hay, contestKeywords) || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishDate.After(out[j].PublishDate) })
	return out, nil
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// parsePosts извлекает __NEXT_DATA__ и маппит listStore.news → []Post.
func parsePosts(htmlBytes []byte) ([]Post, error) {
	m := nextDataRe.FindSubmatch(htmlBytes)
	if m == nil {
		return nil, fmt.Errorf("sk.ru: __NEXT_DATA__ не найден (изменилась разметка?)")
	}
	var nd nextData
	if err := json.Unmarshal(m[1], &nd); err != nil {
		return nil, fmt.Errorf("sk.ru: разбор __NEXT_DATA__: %w", err)
	}

	raws := nd.Props.PageProps.InitialProps.ListStore.News
	posts := make([]Post, 0, len(raws))
	for _, r := range raws {
		if r.Slug == "" || r.Title == "" {
			continue
		}
		p := Post{
			ID:        r.ID.String(),
			Title:     strings.TrimSpace(r.Title),
			ShortDesc: strings.TrimSpace(r.ShortDesc),
			URL:       "https://sk.ru/news/" + r.Slug + "/",
		}
		if t, err := time.Parse(time.RFC3339, r.PublishDate); err == nil {
			p.PublishDate = t
		}
		p.TypeSlug, p.TypeName = parseType(r.Type)
		p.Tags = parseTags(r.Tags)
		posts = append(posts, p)
	}
	return posts, nil
}

// parseType разбирает поле type: {"name":"События","slug":"events"}.
func parseType(raw json.RawMessage) (slug, name string) {
	if len(raw) == 0 {
		return "", ""
	}
	var t struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	_ = json.Unmarshal(raw, &t)
	return t.Slug, t.Name
}

// parseTags разбирает поле tags: [{"slug":"...","name":"..."}].
func parseTags(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t.Name != "" {
			out = append(out, t.Name)
		}
	}
	return out
}
