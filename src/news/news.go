// Package news отслеживает новости/события Сколково через RSS и заводит их
// в ту же RAG-базу как документы категории «Новости» (статус «действует»).
package news

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baza-skolkovo/src/common/feed"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// Monitor загружает RSS и синхронизирует новости в реестр и RAG.
type Monitor struct {
	RSSURL string
	OutDir string // корень Документы_Сколково
	Store  store.Store
	Rag    *rag.Service // может быть nil — тогда без индексации
	HTTP   *http.Client
}

// New создаёт монитор новостей.
func New(rssURL, outDir string, st store.Store, ragSvc *rag.Service) *Monitor {
	return &Monitor{
		RSSURL: rssURL,
		OutDir: outDir,
		Store:  st,
		Rag:    ragSvc,
		HTTP:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Result — итог синхронизации новостей.
type Result struct {
	Fetched   int
	New       int
	Updated   int
	Unchanged int
	Errors    []string
}

// Sync загружает ленту и заводит новые/изменённые новости в RAG.
func (m *Monitor) Sync(ctx context.Context) (*Result, error) {
	items, err := m.fetch(ctx)
	if err != nil {
		return nil, err
	}
	res := &Result{Fetched: len(items)}
	for _, it := range items {
		if it.Link == "" || strings.TrimSpace(it.Title) == "" {
			continue
		}
		if err := m.process(ctx, it, res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.Link, err))
		}
	}
	return res, nil
}

func (m *Monitor) process(ctx context.Context, it feed.Item, res *Result) error {
	id := "news-" + shortHash(it.Link)
	body := it.Title + "\n\n" + feed.StripTags(it.Summary) + "\n\nИсточник: " + it.Link
	sum := sha256.Sum256([]byte(body))
	hash := hex.EncodeToString(sum[:])

	if existing, err := m.Store.Get(ctx, id); err == nil {
		if existing.FileHash == hash {
			res.Unchanged++
			return nil
		}
		res.Updated++
	} else {
		res.New++
	}

	dir := filepath.Join(m.OutDir, "Действующие", "Новости")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	localPath := filepath.Join(dir, id+".txt")
	if err := os.WriteFile(localPath, []byte(body), 0o644); err != nil {
		return err
	}

	doc := model.Document{
		ID:          id,
		Title:       strings.TrimSpace(it.Title),
		SourceURL:   it.Link,
		LocalPath:   localPath,
		PublishedAt: it.Published,
		FetchedAt:   time.Now(),
		Status:      model.StatusActive, // новости информативны — публикуются без ручной валидации
		Category:    "Новости",
		FileHash:    hash,
	}
	if err := m.Store.Upsert(ctx, doc); err != nil {
		return err
	}
	if m.Rag != nil {
		if _, err := m.Rag.IndexDocument(ctx, id); err != nil {
			return fmt.Errorf("индексация: %w", err)
		}
	}
	return nil
}

func (m *Monitor) fetch(ctx context.Context) ([]feed.Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.RSSURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("статус %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return feed.Parse(data), nil
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}
