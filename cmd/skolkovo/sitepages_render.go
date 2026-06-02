package main

import (
	"context"
	"time"

	"github.com/chromedp/chromedp"
)

// chromeRenderer рендерит страницы через headless-Chrome (chromedp) и возвращает
// HTML уже после исполнения JavaScript. Нужен для разделов сайта, где ссылки и
// контент подставляются скриптами и не видны при простом HTTP-запросе.
//
// Аллокатор браузера создаётся один раз и переиспользуется; на каждую страницу
// открывается отдельная вкладка с таймаутом. Реализует sitepages.Renderer.
type chromeRenderer struct {
	allocCtx context.Context
	wait     time.Duration // пауза после Navigate, чтобы JS успел отрисовать
}

// newChromeRenderer поднимает headless-браузер. Возвращает рендерер и функцию
// освобождения ресурсов (её следует вызвать при остановке сервиса).
func newChromeRenderer(chromePath, proxyURL string, wait time.Duration) (*chromeRenderer, func()) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("lang", "ru-RU"),
	)
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	if proxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(proxyURL))
	}
	if wait <= 0 {
		wait = 2 * time.Second
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	return &chromeRenderer{allocCtx: allocCtx, wait: wait}, cancelAlloc
}

// Render открывает страницу в новой вкладке, ждёт отрисовки и возвращает HTML.
// Отмена обхода (ctx) прерывает рендеринг.
func (cr *chromeRenderer) Render(ctx context.Context, pageURL string) ([]byte, error) {
	tabCtx, cancelTab := chromedp.NewContext(cr.allocCtx)
	defer cancelTab()
	// Прокидываем отмену обхода на вкладку браузера.
	stop := context.AfterFunc(ctx, cancelTab)
	defer stop()

	tctx, cancelTimeout := context.WithTimeout(tabCtx, 45*time.Second)
	defer cancelTimeout()

	var html string
	if err := chromedp.Run(tctx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(cr.wait),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return nil, err
	}
	return []byte(html), nil
}
