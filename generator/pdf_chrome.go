package generator

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// generatePDFChrome рендерит HTML в настоящий бинарный PDF через headless-Chrome.
// chromePath — путь к Chrome/Edge. Тело оборачивается в печатное HTML-оформление.
func generatePDFChrome(parent context.Context, chromePath, htmlContent, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("generatePDFChrome: создание директории: %w", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	defer cancelAlloc()
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	dataURL := "data:text/html;charset=utf-8," + url.PathEscape(wrapHTML(htmlContent))

	var buf []byte
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, _, err = page.PrintToPDF().WithPrintBackground(true).Do(ctx)
			return err
		}),
	); err != nil {
		return fmt.Errorf("generatePDFChrome: рендеринг PDF: %w", err)
	}

	if err := os.WriteFile(outputPath, buf, 0o644); err != nil {
		return fmt.Errorf("generatePDFChrome: запись %q: %w", outputPath, err)
	}
	return nil
}
