package main

import (
	"context"
	"log"

	"baza-skolkovo/src/common/config"
	"baza-skolkovo/src/common/store"
	"baza-skolkovo/src/fetcher"
	rag "baza-skolkovo/src/rag_service"
)

// headlessCollect выполняет headless-обход сайта (обход WAF) и скачивание
// недостающих тел файлов в одном проходе: сначала EnumerateSiteAuto пополняет
// каталог (быстрый HTTP-парсинг категорий, fallback на headless-обход), затем
// EnrichMissing выкачивает тела (за WAF, через headless) и индексирует
// «действующие». Возвращает (найдено, скачано).
//
// onWAF (может быть nil) вызывается при WAF-бане для смены прокси.
// No-op при недоступном Chrome — это не критичная ошибка для общего цикла.
func headlessCollect(ctx context.Context, cfg config.Config, st store.Store, svc *rag.Service, onWAF func() string) (found, fetched int, err error) {
	f, ferr := fetcher.New(cfg.ChromePath, cfg.ProxyURL, cfg.FetchWait, func() string { return cfg.ProxyURL })
	if ferr != nil {
		return 0, 0, ferr
	}
	applyFetchProfile(f, cfg)
	f.OnWAFBlocked = onWAF

	// 1. Каталогизация: HTTP-парсинг категорий, fallback на headless-обход.
	items, eerr := f.EnumerateSiteAuto(ctx, cfg.SourceURL, catalogSpecs(), cfg.CrawlMaxPages)
	if eerr != nil {
		log.Printf("[collect] каталогизация: %v", eerr)
	} else {
		added, merged := upsertCatalogItems(ctx, st, items)
		found = len(items)
		log.Printf("[collect] каталог: найдено %d, добавлено %d, дополнено %d", found, added, merged)
	}

	// 2. Скачивание недостающих тел файлов.
	indexFn := func(ctx context.Context, id string) error {
		if svc == nil {
			return nil
		}
		if err := svc.Init(ctx); err != nil {
			return err
		}
		_, err := svc.IndexDocument(ctx, id)
		return err
	}
	done, errs := f.EnrichMissing(ctx, st, cfg.DocsDir, cfg.FetchLimit, indexFn)
	fetched = done
	log.Printf("[collect] скачано тел файлов %d, ошибок %d", done, len(errs))
	for _, e := range errs {
		log.Printf("[collect] ошибка: %v", e)
	}
	return found, fetched, nil
}

// runScheduledCollect — обёртка headlessCollect для планировщика: фиксирует
// результат в мониторинге свежести. No-op при недоступном Chrome.
func runScheduledCollect(ctx context.Context, cfg config.Config, st store.Store, svc *rag.Service, recordHealth func(string, int, error)) {
	log.Printf("[serve:collect] headless-обход сайта + скачивание тел (обход WAF)")
	found, fetched, err := headlessCollect(ctx, cfg, st, svc, nil)
	if err != nil {
		log.Printf("[serve:collect] headless-браузер недоступен: %v", err)
		recordHealth("collect", 0, err)
		return
	}
	recordHealth("collect", found+fetched, nil)
}
