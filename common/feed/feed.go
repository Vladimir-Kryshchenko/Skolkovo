// Package feed разбирает ленты RSS 2.0 и Atom в единый список элементов.
package feed

import (
	"encoding/xml"
	"strings"
	"time"
)

// Item — нормализованный элемент ленты.
type Item struct {
	Title     string
	Link      string
	Summary   string
	Published *time.Time
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomFeed struct {
	XMLName xml.Name `xml:"feed"`
	Entries []struct {
		Title   string `xml:"title"`
		Summary string `xml:"summary"`
		Updated string `xml:"updated"`
		Links   []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// Parse распознаёт RSS 2.0 или Atom и возвращает элементы.
func Parse(data []byte) []Item {
	var rss rssFeed
	if err := xml.Unmarshal(data, &rss); err == nil && len(rss.Channel.Items) > 0 {
		out := make([]Item, 0, len(rss.Channel.Items))
		for _, it := range rss.Channel.Items {
			out = append(out, Item{
				Title:     strings.TrimSpace(it.Title),
				Link:      strings.TrimSpace(it.Link),
				Summary:   it.Description,
				Published: ParseTime(it.PubDate),
			})
		}
		return out
	}
	var atom atomFeed
	if err := xml.Unmarshal(data, &atom); err == nil && len(atom.Entries) > 0 {
		out := make([]Item, 0, len(atom.Entries))
		for _, e := range atom.Entries {
			link := ""
			for _, l := range e.Links {
				if l.Rel == "" || l.Rel == "alternate" {
					link = l.Href
					break
				}
			}
			out = append(out, Item{
				Title:     strings.TrimSpace(e.Title),
				Link:      strings.TrimSpace(link),
				Summary:   e.Summary,
				Published: ParseTime(e.Updated),
			})
		}
		return out
	}
	return nil
}

// ParseTime разбирает дату в распространённых форматах лент.
func ParseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// StripTags грубо удаляет HTML-теги из строки (для описаний лент).
func StripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
