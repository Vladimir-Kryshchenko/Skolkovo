package sitepages

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
)

// DateBackfiller дозаполняет дату публикации для уже сохранённых страниц, у
// которых её ещё нет (published_at IS NULL). Повторно загружает страницу и
// извлекает только дату (текст/хэш не трогаются). Нужен один раз для исторической
// базы; новые и изменённые страницы получают дату прямо на обходе (crawler.parse).
type DateBackfiller struct {
	Crawler     *Crawler       // источник загрузки/разбора (FetchPublished)
	Store       *PostgresStore // куда писать дату (SetPublished)
	Concurrency int            // число параллельных загрузчиков (>=1)
}

// Run обрабатывает переданные страницы и возвращает счётчики:
// filled — проставлено дат, notFound — дату определить не удалось, failed — ошибки загрузки.
func (b *DateBackfiller) Run(ctx context.Context, pages []*Page) (filled, notFound, failed int) {
	if len(pages) == 0 || b.Crawler == nil || b.Store == nil {
		return 0, 0, 0
	}
	conc := b.Concurrency
	if conc < 1 {
		conc = 1
	}
	var (
		nFilled, nNotFound, nFailed, nDone int64
		total                              = int64(len(pages))
		wg                                 sync.WaitGroup
		sem                                = make(chan struct{}, conc)
	)
	for _, p := range pages {
		select {
		case <-ctx.Done():
			wg.Wait()
			return int(nFilled), int(nNotFound), int(nFailed)
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p *Page) {
			defer wg.Done()
			defer func() { <-sem }()

			published, err := b.Crawler.FetchPublished(ctx, p.URL)
			switch {
			case err != nil:
				atomic.AddInt64(&nFailed, 1)
				log.Printf("[sitepages/dates] %s: %v", p.URL, err)
			case published == nil:
				atomic.AddInt64(&nNotFound, 1)
			default:
				if _, serr := b.Store.SetPublished(ctx, p.ID, *published); serr != nil {
					atomic.AddInt64(&nFailed, 1)
					log.Printf("[sitepages/dates] сохранение %s: %v", p.URL, serr)
				} else {
					atomic.AddInt64(&nFilled, 1)
				}
			}
			if d := atomic.AddInt64(&nDone, 1); d%200 == 0 || d == total {
				log.Printf("[sitepages/dates] обработано %d/%d (дат проставлено %d, без даты %d, ошибок %d)",
					d, total, atomic.LoadInt64(&nFilled), atomic.LoadInt64(&nNotFound), atomic.LoadInt64(&nFailed))
			}
		}(p)
	}
	wg.Wait()
	return int(nFilled), int(nNotFound), int(nFailed)
}

// NewDateBackfiller собирает бэкфиллер дат.
func NewDateBackfiller(cr *Crawler, store *PostgresStore, concurrency int) *DateBackfiller {
	return &DateBackfiller{Crawler: cr, Store: store, Concurrency: concurrency}
}
