package sitepages

import (
	"context"
	"fmt"
	"net/url"
)

// PruneResult — итог прунинга: что просмотрено, что подпадает под удаление и
// сколько фактически удалено из БД и Qdrant (0, если dryRun).
type PruneResult struct {
	Scanned       int      // всего страниц просмотрено
	Matched       int      // подпадает под денилист обхода
	DeletedRows   int64    // удалено строк из site_pages
	DeletedPoints int      // удалено точек из Qdrant
	Samples       []string // примеры URL под удаление (до 10)
}

// Prune удаляет из site_pages и из Qdrant все страницы, которые теперь отсекает
// денилист обхода (isExcludedURL): тег-ловушки и служебные страницы Telligent,
// раздувшие базу. Гарантирует согласованность с краулером — критерий ровно тот же
// (isExcludedURL), поэтому почищенное больше не появится при следующем обходе.
//
// dryRun=true — только подсчёт (ничего не удаляется). Безопасно запускать
// повторно (идемпотентно): уже удалённые страницы просто не находятся.
func Prune(ctx context.Context, store *PostgresStore, ix *Indexer, dryRun bool) (PruneResult, error) {
	refs, err := store.ListRefs(ctx)
	if err != nil {
		return PruneResult{}, fmt.Errorf("список страниц: %w", err)
	}
	res := PruneResult{Scanned: len(refs)}

	var ids, points []string
	for _, r := range refs {
		u, perr := url.Parse(r.URL)
		if perr != nil {
			continue // непарсимый URL не трогаем — пусть остаётся
		}
		if isExcludedURL(u) {
			ids = append(ids, r.ID)
			points = append(points, pointID(r.URL))
			if len(res.Samples) < 10 {
				res.Samples = append(res.Samples, r.URL)
			}
		}
	}
	res.Matched = len(ids)
	if dryRun || len(ids) == 0 {
		return res, nil
	}

	// Сначала Qdrant (порциями — не слать один гигантский запрос), затем БД. Если
	// упадём между ними, повторный прогон до-удалит хвост (идемпотентно).
	for start := 0; start < len(points); start += 512 {
		end := min(start+512, len(points))
		if err := ix.Qdr.Delete(ctx, points[start:end]); err != nil {
			return res, fmt.Errorf("удаление точек Qdrant: %w", err)
		}
		res.DeletedPoints += end - start
	}
	for start := 0; start < len(ids); start += 1000 {
		end := min(start+1000, len(ids))
		n, err := store.DeleteByIDs(ctx, ids[start:end])
		if err != nil {
			return res, fmt.Errorf("удаление строк: %w", err)
		}
		res.DeletedRows += n
	}
	return res, nil
}
