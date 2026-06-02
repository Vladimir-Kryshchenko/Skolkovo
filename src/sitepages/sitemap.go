package sitepages

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"strings"
)

// discoverFromSitemaps собирает URL страниц из sitemap-ов каждого origin-а
// (scheme://host) стартовых URL. Источники sitemap: robots.txt (директивы
// Sitemap:) и стандартные пути /sitemap.xml, /sitemap_index.xml. Поддерживаются
// индексы sitemap (рекурсивно) и gzip-сжатые карты. Возвращает плоский список
// URL страниц (<loc> из <url>).
func (c *Crawler) discoverFromSitemaps(ctx context.Context, origins []string) []string {
	seen := map[string]bool{} // обойдённые sitemap-файлы (защита от циклов)
	out := map[string]bool{}  // найденные URL страниц
	const maxSitemaps = 200   // потолок числа sitemap-файлов на обход
	processed := 0

	var visit func(sm string, depth int)
	visit = func(sm string, depth int) {
		if depth > 5 || processed >= maxSitemaps || seen[sm] {
			return
		}
		seen[sm] = true
		processed++
		data, err := c.fetch(ctx, sm)
		if err != nil {
			return
		}
		locs, sub := parseSitemap(data, sm)
		for _, u := range sub { // вложенные sitemap-ы (индекс) — рекурсивно
			visit(u, depth+1)
		}
		for _, u := range locs {
			out[u] = true
		}
	}

	for _, base := range origins {
		// robots.txt → Sitemap:
		if data, err := c.fetch(ctx, base+"/robots.txt"); err == nil {
			for _, sm := range sitemapsFromRobots(data) {
				visit(sm, 0)
			}
		}
		// Стандартные расположения.
		for _, p := range []string{"/sitemap.xml", "/sitemap_index.xml"} {
			visit(base+p, 0)
		}
	}

	res := make([]string, 0, len(out))
	for u := range out {
		res = append(res, u)
	}
	return res
}

// sitemapsFromRobots вытаскивает значения директив «Sitemap:» из robots.txt.
func sitemapsFromRobots(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if len(line) >= 8 && strings.EqualFold(line[:8], "sitemap:") {
			if v := strings.TrimSpace(line[8:]); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// sitemapDoc разбирает оба формата: набор страниц (<urlset><url><loc>) и индекс
// (<sitemapindex><sitemap><loc>) — независимо от корневого элемента.
type sitemapDoc struct {
	URLs     []sitemapLoc `xml:"url"`
	Sitemaps []sitemapLoc `xml:"sitemap"`
}

type sitemapLoc struct {
	Loc string `xml:"loc"`
}

// parseSitemap возвращает (URL страниц, вложенные sitemap-ы). Прозрачно
// распаковывает gzip (.xml.gz или gzip-тело).
func parseSitemap(data []byte, srcURL string) (locs []string, sitemaps []string) {
	if strings.HasSuffix(strings.ToLower(srcURL), ".gz") || isGzip(data) {
		if r, err := gzip.NewReader(bytes.NewReader(data)); err == nil {
			if un, err := io.ReadAll(r); err == nil {
				data = un
			}
			_ = r.Close()
		}
	}
	var doc sitemapDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, nil
	}
	for _, u := range doc.URLs {
		if loc := strings.TrimSpace(u.Loc); loc != "" {
			locs = append(locs, loc)
		}
	}
	for _, s := range doc.Sitemaps {
		if loc := strings.TrimSpace(s.Loc); loc != "" {
			sitemaps = append(sitemaps, loc)
		}
	}
	return locs, sitemaps
}

// isGzip определяет gzip-поток по магическим байтам 0x1f 0x8b.
func isGzip(b []byte) bool {
	return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b
}
