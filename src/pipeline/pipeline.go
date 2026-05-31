// Package pipeline оркеструет регулярный конвейер: парсинг документов и новостей →
// индексация действующих документов → отчёт → уведомление. Используется
// как разово (skolkovo sync), так и по расписанию (skolkovo serve).
package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baza-skolkovo/src/changes"
	"baza-skolkovo/src/health"
	"baza-skolkovo/src/news"
	"baza-skolkovo/src/notify"
	rag "baza-skolkovo/src/rag_service"
	"baza-skolkovo/src/relevance"
	"baza-skolkovo/src/scraper"
)

// Pipeline объединяет подсистемы одного цикла актуализации.
type Pipeline struct {
	Scraper  *scraper.Scraper
	Rag      *rag.Service             // может быть nil — тогда без индексации
	News     *news.Monitor            // может быть nil — тогда без новостей
	Notifier *notify.Notifier         // webhook; может быть nil
	Changes  changes.Store            // лента изменений; может быть nil
	TG       *notify.TelegramNotifier // Telegram-алерты консультанту; может быть nil
	Health   health.Store             // мониторинг свежести источников; может быть nil
	// Analyzer/Dispatcher — трек «Актуальность изменений». Если Analyzer задан,
	// изменения документов проходят классификацию важности и адресную рассылку.
	Analyzer   *relevance.Analyzer
	Dispatcher *relevance.Dispatcher
	ReportDir  string
}

// RunOnce выполняет один полный цикл актуализации.
func (p *Pipeline) RunOnce(ctx context.Context) error {
	log.Println("[pipeline] старт цикла: парсинг dochub.sk.ru")
	rep, err := p.Scraper.Run(ctx)
	p.recordHealth(ctx, "documents", docItems(rep), err)
	if err != nil {
		return fmt.Errorf("парсинг: %w", err)
	}
	log.Printf("[pipeline] документы: страниц %d, в каталог %d, файлов %d, обновлено %d, без изменений %d, ошибок %d",
		rep.Visited, rep.Catalogued, rep.Downloaded, rep.Updated, rep.Unchanged, len(rep.Errors))

	var idx *rag.IndexResult
	if p.Rag != nil {
		if err := p.Rag.Init(ctx); err != nil {
			log.Printf("[pipeline] Qdrant недоступен: %v (индексация пропущена)", err)
		} else if idx, err = p.Rag.IndexActive(ctx, false); err != nil {
			log.Printf("[pipeline] индексация: %v", err)
		} else {
			log.Printf("[pipeline] проиндексировано документов %d, фрагментов %d", idx.Documents, idx.Chunks)
		}
	}

	var newsRes *news.Result
	if p.News != nil {
		newsRes, err = p.News.Sync(ctx)
		p.recordHealth(ctx, "news", newsItems(newsRes), err)
		if err != nil {
			log.Printf("[pipeline] новости: %v", err)
		} else {
			log.Printf("[pipeline] новости: получено %d, новых %d, обновлено %d", newsRes.Fetched, newsRes.New, newsRes.Updated)
		}
	}

	if err := writeReport(p.ReportDir, rep, idx, newsRes); err != nil {
		log.Printf("[pipeline] не удалось записать отчёт: %v", err)
	}

	p.maybeNotify(ctx, rep, newsRes)
	p.processRelevance(ctx)
	p.notifyChanges(ctx)
	return nil
}

// processRelevance обрабатывает необработанные изменения документов: считает дифф,
// классифицирует важность (LLM/эвристика), определяет затронутые стадии и рассылает
// адресные уведомления. Обработанные события помечаются notified, поэтому generic
// notifyChanges их не дублирует. Если анализатор не подключён — шаг пропускается,
// и документы уходят в notifyChanges как раньше.
func (p *Pipeline) processRelevance(ctx context.Context) {
	if p.Changes == nil || p.Analyzer == nil {
		return
	}
	evs, err := p.Changes.Unanalyzed(ctx, changes.EntityDocument, 50)
	if err != nil {
		log.Printf("[pipeline] актуальность: чтение очереди: %v", err)
		return
	}
	if len(evs) == 0 {
		return
	}
	var done []string
	var alerts int
	for _, ev := range evs {
		res, err := p.Analyzer.Analyze(ctx, ev)
		if err != nil {
			log.Printf("[pipeline] актуальность: анализ %s: %v", ev.ID, err)
			continue // оставляем необработанным — повторим в следующем цикле
		}
		if err := p.Changes.Enrich(ctx, ev.ID, res.ToEnrichment()); err != nil {
			log.Printf("[pipeline] актуальность: запись %s: %v", ev.ID, err)
			continue
		}
		if p.Dispatcher != nil {
			if n, err := p.Dispatcher.Dispatch(ctx, ev, res); err != nil {
				log.Printf("[pipeline] актуальность: рассылка %s: %v", ev.ID, err)
			} else {
				alerts += n
			}
		}
		done = append(done, ev.ID)
	}
	if len(done) > 0 {
		if err := p.Changes.MarkNotified(ctx, done); err != nil {
			log.Printf("[pipeline] актуальность: пометка notified: %v", err)
		}
		log.Printf("[pipeline] актуальность: обработано изменений %d, адресных уведомлений %d", len(done), alerts)
	}
}

// notifyChanges рассылает Telegram-алерты по неотправленным изменениям ленты
// и помечает их отправленными. Если Telegram-нотификатор не настроен — изменения
// остаются в ленте (доступны через get_recent_changes), но алерты не шлются.
func (p *Pipeline) notifyChanges(ctx context.Context) {
	if p.Changes == nil || p.TG == nil || !p.TG.Enabled() {
		return
	}
	evs, err := p.Changes.Unnotified(ctx, 50)
	if err != nil {
		log.Printf("[pipeline] лента изменений: %v", err)
		return
	}
	if len(evs) == 0 {
		return
	}
	ids := make([]string, 0, len(evs))
	for _, ev := range evs {
		switch ev.EntityType {
		case changes.EntityNPA:
			_ = p.TG.SendNewNPA(ctx, ev.Title, ev.SourceURL)
		case changes.EntityContest:
			_ = p.TG.SendNewContest(ctx, ev.Title, ev.SourceURL)
		default:
			_ = p.TG.SendDocumentChanged(ctx, ev.Title, ev.Category, string(ev.Kind))
		}
		ids = append(ids, ev.ID)
	}
	if err := p.Changes.MarkNotified(ctx, ids); err != nil {
		log.Printf("[pipeline] не удалось пометить изменения отправленными: %v", err)
		return
	}
	log.Printf("[pipeline] отправлено алертов по изменениям: %d", len(ids))
}

// recordHealth фиксирует результат прогона источника в мониторинге свежести.
func (p *Pipeline) recordHealth(ctx context.Context, source string, items int, runErr error) {
	if p.Health == nil {
		return
	}
	if err := p.Health.Record(ctx, source, items, runErr); err != nil {
		log.Printf("[pipeline] health[%s]: %v", source, err)
	}
}

// docItems возвращает число новых/обновлённых документов за прогон.
func docItems(rep *scraper.Report) int {
	if rep == nil {
		return 0
	}
	return rep.Catalogued + rep.Downloaded + rep.Updated
}

// newsItems возвращает число новых/обновлённых новостей за прогон.
func newsItems(res *news.Result) int {
	if res == nil {
		return 0
	}
	return res.New + res.Updated
}

// Schedule запускает RunOnce немедленно и далее каждые interval до отмены контекста.
func (p *Pipeline) Schedule(ctx context.Context, interval time.Duration) {
	log.Printf("[pipeline] планировщик запущен, интервал %s", interval)
	if err := p.RunOnce(ctx); err != nil {
		log.Printf("[pipeline] цикл завершился с ошибкой: %v", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[pipeline] планировщик остановлен")
			return
		case <-t.C:
			if err := p.RunOnce(ctx); err != nil {
				log.Printf("[pipeline] цикл завершился с ошибкой: %v", err)
			}
		}
	}
}

func (p *Pipeline) maybeNotify(ctx context.Context, rep *scraper.Report, newsRes *news.Result) {
	if p.Notifier == nil || !p.Notifier.Enabled() {
		return
	}
	changed := rep.Downloaded + rep.Updated
	newsNew := 0
	if newsRes != nil {
		newsNew = newsRes.New + newsRes.Updated
	}
	if changed == 0 && newsNew == 0 {
		return
	}
	ev := notify.Event{
		Type:      "parsing_cycle",
		Timestamp: time.Now(),
		Message: fmt.Sprintf("Актуализация базы: новых/обновлённых документов %d, новостей %d",
			changed, newsNew),
		Details: map[string]any{
			"documents_new":     rep.Downloaded,
			"documents_updated": rep.Updated,
			"news_new":          newsNew,
		},
	}
	if err := p.Notifier.Send(ctx, ev); err != nil {
		log.Printf("[pipeline] уведомление не отправлено: %v", err)
	}
}

func writeReport(dir string, rep *scraper.Report, idx *rag.IndexResult, newsRes *news.Result) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	now := time.Now()
	var b strings.Builder
	fmt.Fprintf(&b, "# Отчёт актуализации — %s\n\n", now.Format("02.01.2006 15:04"))
	fmt.Fprintf(&b, "**Старт парсинга:** %s\n\n", rep.StartedAt.Format("02.01.2006 15:04:05"))

	b.WriteString("## Документы\n\n")
	b.WriteString("| Метрика | Значение |\n| :--- | ---: |\n")
	fmt.Fprintf(&b, "| Страниц обойдено | %d |\n", rep.Visited)
	fmt.Fprintf(&b, "| Заведено в каталог (RSS) | %d |\n", rep.Catalogued)
	fmt.Fprintf(&b, "| Файлов скачано | %d |\n", rep.Downloaded)
	fmt.Fprintf(&b, "| Обновлено | %d |\n", rep.Updated)
	fmt.Fprintf(&b, "| Без изменений | %d |\n", rep.Unchanged)
	fmt.Fprintf(&b, "| Ошибок | %d |\n\n", len(rep.Errors))

	if idx != nil {
		b.WriteString("## Индексация\n\n")
		b.WriteString("| Метрика | Значение |\n| :--- | ---: |\n")
		fmt.Fprintf(&b, "| Документов проиндексировано | %d |\n", idx.Documents)
		fmt.Fprintf(&b, "| Фрагментов | %d |\n", idx.Chunks)
		fmt.Fprintf(&b, "| Ошибок индексации | %d |\n\n", len(idx.Errors))
	}

	if newsRes != nil {
		b.WriteString("## Новости\n\n")
		b.WriteString("| Метрика | Значение |\n| :--- | ---: |\n")
		fmt.Fprintf(&b, "| Получено из ленты | %d |\n", newsRes.Fetched)
		fmt.Fprintf(&b, "| Новых | %d |\n", newsRes.New)
		fmt.Fprintf(&b, "| Обновлено | %d |\n", newsRes.Updated)
		fmt.Fprintf(&b, "| Без изменений | %d |\n\n", newsRes.Unchanged)
	}

	if len(rep.Errors) > 0 {
		b.WriteString("## Ошибки парсинга\n\n")
		for _, e := range rep.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}
	b.WriteString("---\n\n*Сгенерировано автоматически конвейером «База Сколково».*\n")

	name := fmt.Sprintf("ОТЧЕТ_Актуализация_%s.md", now.Format("2006-01-02_150405"))
	return os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644)
}
