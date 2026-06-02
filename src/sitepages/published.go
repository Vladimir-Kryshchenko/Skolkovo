package sitepages

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Этот файл отвечает за единственную задачу: определить дату публикации страницы
// НА САЙТЕ Сколково. Это принципиально иная сущность, чем FirstSeen/LastChanged
// (времена обхода нашим краулером). Дата берётся из самой страницы, в порядке
// надёжности источников:
//
//  1. <meta property="article:published_time"> (Open Graph article) и родственные
//     (datePublished, date, pubdate, dc.date…) — самый точный машинный источник;
//  2. JSON-LD (<script type="application/ld+json">) поле datePublished/dateCreated;
//  3. <time datetime="…"> — HTML5-разметка времени;
//  4. видимая надпись с датой в блоке поста sk.ru (<div class="data">25 мая 2017 г.</div>)
//     — у новостей/постов sk.ru машинной разметки дат нет, дата только здесь.
//
// На каждом обходе дата извлекается заново; см. crawler.parse и Upsert
// (published_at заполняется и для уже существующих страниц, у которых его не было).

// dateMetaNames — name/property у <meta>, в которых сайты публикуют дату.
var dateMetaNames = map[string]bool{
	"article:published_time": true,
	"article:published":      true,
	"og:published_time":      true,
	"datepublished":          true,
	"date":                   true,
	"pubdate":                true,
	"publish-date":           true,
	"dc.date":                true,
	"dc.date.issued":         true,
	"sailthru.date":          true,
}

// publishedSignals — собранные при обходе DOM кандидаты на дату публикации,
// упорядоченные по убыванию надёжности источника.
type publishedSignals struct {
	meta     []string // содержимое date-related <meta>
	jsonLD   []string // сырые тела <script type="application/ld+json">
	timeAttr []string // datetime у <time>
	visible  []string // текст элементов с class~="data"/"date" (видимая надпись)
}

// resolve выбирает первую распарсиваемую дату, перебирая источники по надёжности.
func (s publishedSignals) resolve() *time.Time {
	groups := [][]string{s.meta, s.timeAttr, s.visible}
	// JSON-LD требует отдельного разбора — вставляем сразу после meta.
	for _, blob := range s.jsonLD {
		if t, ok := parseJSONLDDate(blob); ok {
			return &t
		}
	}
	for _, group := range groups {
		for _, raw := range group {
			if t, ok := parseAnyDate(raw); ok {
				return &t
			}
		}
	}
	return nil
}

// collectPublishedSignal обновляет signals по одному узлу DOM. Вызывается из
// общего обхода в crawler.parse, чтобы не ходить по дереву второй раз.
func collectPublishedSignal(n *html.Node, sig *publishedSignals) {
	if n.Type != html.ElementNode {
		return
	}
	switch n.Data {
	case "meta":
		var name, content string
		for _, a := range n.Attr {
			switch strings.ToLower(a.Key) {
			case "name", "property", "itemprop":
				name = strings.ToLower(strings.TrimSpace(a.Val))
			case "content":
				content = a.Val
			}
		}
		if content != "" && dateMetaNames[name] {
			sig.meta = append(sig.meta, content)
		}
	case "time":
		for _, a := range n.Attr {
			if strings.ToLower(a.Key) == "datetime" && strings.TrimSpace(a.Val) != "" {
				sig.timeAttr = append(sig.timeAttr, a.Val)
			}
		}
	case "script":
		isLD := false
		for _, a := range n.Attr {
			if strings.ToLower(a.Key) == "type" && strings.Contains(strings.ToLower(a.Val), "ld+json") {
				isLD = true
			}
		}
		if isLD && n.FirstChild != nil {
			sig.jsonLD = append(sig.jsonLD, nodeRawText(n))
		}
	default:
		// Видимая надпись с датой: элементы с классом data/date (sk.ru: .post-page__info .data).
		for _, a := range n.Attr {
			if strings.ToLower(a.Key) != "class" {
				continue
			}
			for _, cls := range strings.Fields(strings.ToLower(a.Val)) {
				if cls == "data" || cls == "date" || strings.Contains(cls, "publish") || strings.Contains(cls, "pubdate") {
					if txt := strings.TrimSpace(nodeText(n)); txt != "" {
						sig.visible = append(sig.visible, txt)
					}
				}
			}
		}
	}
}

// nodeRawText собирает текст всех потомков узла без сжатия пробелов (для JSON-LD).
func nodeRawText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// parseJSONLDDate вытаскивает datePublished/dateCreated из тела JSON-LD (объект
// или массив объектов, в т.ч. с @graph).
func parseJSONLDDate(blob string) (time.Time, bool) {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return time.Time{}, false
	}
	var raw any
	if err := json.Unmarshal([]byte(blob), &raw); err != nil {
		return time.Time{}, false
	}
	return searchJSONLDDate(raw)
}

func searchJSONLDDate(v any) (time.Time, bool) {
	switch t := v.(type) {
	case map[string]any:
		for _, key := range []string{"datePublished", "dateCreated", "datePosted", "uploadDate"} {
			if s, ok := t[key].(string); ok {
				if dt, ok := parseAnyDate(s); ok {
					return dt, true
				}
			}
		}
		for _, key := range []string{"@graph", "mainEntity", "itemListElement"} {
			if sub, ok := t[key]; ok {
				if dt, ok := searchJSONLDDate(sub); ok {
					return dt, true
				}
			}
		}
	case []any:
		for _, e := range t {
			if dt, ok := searchJSONLDDate(e); ok {
				return dt, true
			}
		}
	}
	return time.Time{}, false
}

// ruMonths — родительный падеж русских месяцев (как пишут на sk.ru: «25 мая 2017 г.»).
var ruMonths = map[string]time.Month{
	"января": time.January, "февраля": time.February, "марта": time.March,
	"апреля": time.April, "мая": time.May, "июня": time.June,
	"июля": time.July, "августа": time.August, "сентября": time.September,
	"октября": time.October, "ноября": time.November, "декабря": time.December,
	// именительный падеж — на случай иной разметки
	"январь": time.January, "февраль": time.February, "март": time.March,
	"апрель": time.April, "май": time.May, "июнь": time.June,
	"июль": time.July, "август": time.August, "сентябрь": time.September,
	"октябрь": time.October, "ноябрь": time.November, "декабрь": time.December,
}

var (
	reRuDate  = regexp.MustCompile(`(\d{1,2})\s+([А-Яа-яЁё]+)\s+(\d{4})`)
	reDMYDate = regexp.MustCompile(`\b(\d{1,2})[.\-/](\d{1,2})[.\-/](\d{4})\b`)
	reISODate = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
)

// minYear/maxYear — рамки правдоподобия: отсекают мусорные «даты» из вёрстки.
const minYear, maxYear = 1996, 2100

// parseAnyDate пытается разобрать дату из произвольной строки несколькими
// способами: ISO/RFC3339 → «25 мая 2017» → «25.05.2017» → «2017-05-25».
func parseAnyDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	// Машинные форматы (meta/time/JSON-LD обычно отдают именно их).
	for _, layout := range []string{
		time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05",
		"2006-01-02 15:04:05", "2006-01-02", "02.01.2006 15:04:05", "02.01.2006",
	} {
		if t, err := time.Parse(layout, s); err == nil && plausible(t) {
			return normalizeDay(t), true
		}
	}
	// Русская словесная дата: «25 мая 2017 г.».
	if m := reRuDate.FindStringSubmatch(s); m != nil {
		day, _ := strconv.Atoi(m[1])
		if mon, ok := ruMonths[strings.ToLower(m[2])]; ok {
			year, _ := strconv.Atoi(m[3])
			if t := mkDate(year, mon, day); plausible(t) {
				return t, true
			}
		}
	}
	// Числовая ДД.ММ.ГГГГ (и через - или /).
	if m := reDMYDate.FindStringSubmatch(s); m != nil {
		day, _ := strconv.Atoi(m[1])
		mon, _ := strconv.Atoi(m[2])
		year, _ := strconv.Atoi(m[3])
		if mon >= 1 && mon <= 12 {
			if t := mkDate(year, time.Month(mon), day); plausible(t) {
				return t, true
			}
		}
	}
	// ISO-фрагмент в любой строке.
	if m := reISODate.FindStringSubmatch(s); m != nil {
		year, _ := strconv.Atoi(m[1])
		mon, _ := strconv.Atoi(m[2])
		day, _ := strconv.Atoi(m[3])
		if mon >= 1 && mon <= 12 {
			if t := mkDate(year, time.Month(mon), day); plausible(t) {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func mkDate(year int, mon time.Month, day int) time.Time {
	if day < 1 || day > 31 {
		return time.Time{}
	}
	return time.Date(year, mon, day, 0, 0, 0, 0, time.UTC)
}

func normalizeDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func plausible(t time.Time) bool {
	return !t.IsZero() && t.Year() >= minYear && t.Year() <= maxYear
}
