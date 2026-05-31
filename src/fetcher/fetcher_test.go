package fetcher

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// pad дополняет тело до >=100 байт, чтобы пройти проверку минимального размера.
func pad(prefix string) []byte {
	return []byte(prefix + strings.Repeat("\x00", 200))
}

func TestValidateFileBytes(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		data    []byte
		wantWAF bool
	}{
		{"pdf ok", "https://x/doc.pdf", pad("%PDF-1.7\n"), false},
		{"docx ok", "https://x/doc.docx", pad("PK\x03\x04"), false},
		{"xlsx ok", "https://x/t.xlsx", pad("PK\x03\x04"), false},
		{"doc ole ok", "https://x/d.doc", pad("\xD0\xCF\x11\xE0"), false},
		{"rtf ok", "https://x/d.rtf", pad("{\\rtf1"), false},
		{"unknown ext passes", "https://x/file.bin", pad("BINARYDATA"), false},
		{"too small", "https://x/doc.pdf", []byte("%PDF"), true},
		{"html challenge", "https://x/doc.pdf", pad("<!DOCTYPE html><html>variti"), true},
		{"html lowercase tag", "https://x/doc.pdf", pad("<html><body>проверка"), true},
		{"pdf wrong magic", "https://x/doc.pdf", pad("NOTPDF"), true},
		{"docx not zip", "https://x/doc.docx", pad("NOTZIP-content"), true},
		{"doc not ole", "https://x/d.doc", pad("plain text"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateFileBytes(c.data, c.url)
			gotWAF := errors.Is(err, errWAFBlock)
			if gotWAF != c.wantWAF {
				t.Fatalf("validateFileBytes(%q) err=%v, wantWAF=%v", c.name, err, c.wantWAF)
			}
		})
	}
}

func TestClassifyRunErr(t *testing.T) {
	if got := classifyRunErr(nil); got != nil {
		t.Fatalf("nil → %v", got)
	}
	waf := fmt.Errorf("обёртка: %w", errWAFBlock)
	if got := classifyRunErr(waf); !errors.Is(got, errWAFBlock) {
		t.Fatalf("WAF-ошибка потеряна: %v", got)
	}
	noLink := fmt.Errorf("обёртка: %w", errNoLink)
	if got := classifyRunErr(noLink); !errors.Is(got, errNoLink) {
		t.Fatalf("errNoLink потеряна: %v", got)
	}
	net := errors.New("dial tcp: timeout")
	got := classifyRunErr(net)
	if errors.Is(got, errWAFBlock) || errors.Is(got, errNoLink) {
		t.Fatalf("сетевую ошибку нельзя классифицировать как WAF/NoLink: %v", got)
	}
}

func TestBetweenFilesDelayBounds(t *testing.T) {
	f := &Fetcher{
		Wait:         10 * time.Second,
		LongPausePct: 0, // без длинных пауз — детерминированные границы
		Rng:          rand.New(rand.NewSource(1)),
	}
	min := f.Wait - f.Wait/2 // 5s
	max := f.Wait + f.Wait/2 // 15s
	for i := 0; i < 2000; i++ {
		d := f.betweenFilesDelay()
		if d < min || d >= max {
			t.Fatalf("delay %v вне [%v,%v)", d, min, max)
		}
	}
}

func TestBetweenFilesDelayLongPause(t *testing.T) {
	f := &Fetcher{
		Wait:         5 * time.Second,
		LongPausePct: 100, // всегда длинная пауза
		Rng:          rand.New(rand.NewSource(2)),
	}
	// База ∈ [2.5s,7.5s) + длинная добавка ∈ [20s,60s) ⇒ всегда > 20s.
	for i := 0; i < 500; i++ {
		if d := f.betweenFilesDelay(); d < 20*time.Second {
			t.Fatalf("ожидалась длинная пауза, получено %v", d)
		}
	}
}

func TestBetweenFilesDelayZeroWait(t *testing.T) {
	f := &Fetcher{Wait: 0, LongPausePct: 0, Rng: rand.New(rand.NewSource(3))}
	for i := 0; i < 100; i++ {
		if d := f.betweenFilesDelay(); d < time.Second {
			t.Fatalf("пауза должна быть >= 1s, получено %v", d)
		}
	}
}

func TestCategoryFromURL(t *testing.T) {
	slugName := map[string]string{
		"legislative_acts": "Законодательные акты",
		"design_rules":     "Правила проектирования",
	}
	cases := map[string]string{
		"https://dochub.sk.ru/foundation/documents/p/legislative_acts.aspx": "Законодательные акты",
		"https://dochub.sk.ru/foundation/documents/p/design_rules.aspx":     "Правила проектирования",
		"https://dochub.sk.ru/foundation/documents/p/unknown_slug.aspx":     "", // нет в карте
		"https://dochub.sk.ru/foundation/documents/":                        "", // не страница категории
	}
	for url, want := range cases {
		if got := categoryFromURL(url, slugName); got != want {
			t.Errorf("categoryFromURL(%q)=%q, want %q", url, got, want)
		}
	}
}

func TestJSString(t *testing.T) {
	cases := map[string]string{
		`https://x/sitemap.xml`:    `"https://x/sitemap.xml"`,
		`a"b`:                      `"a\"b"`,
		`a\b`:                      `"a\\b"`,
		`https://x/a"?p=1\foo.xml`: `"https://x/a\"?p=1\\foo.xml"`,
	}
	for in, want := range cases {
		if got := jsString(in); got != want {
			t.Errorf("jsString(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestLocRe(t *testing.T) {
	xml := `<?xml version="1.0"?><urlset>
	<url><loc>https://dochub.sk.ru/foundation/documents/p/other.aspx</loc></url>
	<url><loc> https://dochub.sk.ru/foundation/documents/p/tenders.aspx </loc></url>
	</urlset>`
	matches := locRe.FindAllStringSubmatch(xml, -1)
	if len(matches) != 2 {
		t.Fatalf("ожидалось 2 <loc>, получено %d", len(matches))
	}
	if strings.TrimSpace(matches[0][1]) != "https://dochub.sk.ru/foundation/documents/p/other.aspx" {
		t.Errorf("loc[0]=%q", matches[0][1])
	}
}

func TestMaskProxy(t *testing.T) {
	if got := maskProxy(""); got != "(прямое соединение)" {
		t.Errorf("пустой прокси: %q", got)
	}
	got := maskProxy("http://user:secret@proxy.example.com:8080")
	if strings.Contains(got, "secret") || strings.Contains(got, "user") {
		t.Errorf("учётные данные не замаскированы: %q", got)
	}
	if !strings.Contains(got, "proxy.example.com:8080") {
		t.Errorf("хост потерян: %q", got)
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"https://dochub.sk.ru/m/docs/file.pdf":   "file.pdf",
		"https://dochub.sk.ru/m/docs/a%20b.docx": "a b.docx",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q)=%q, want %q", in, got, want)
		}
	}
}
