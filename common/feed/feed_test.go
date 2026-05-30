package feed

import "testing"

func TestParseRSS(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
  <item><title>Новость 1</title><link>https://sk.ru/n/1</link><description>Текст &lt;b&gt;один&lt;/b&gt;</description><pubDate>Mon, 02 Jan 2006 15:04:05 +0300</pubDate></item>
  <item><title>Новость 2</title><link>https://sk.ru/n/2</link><description>Текст два</description></item>
</channel></rss>`)
	items := Parse(data)
	if len(items) != 2 {
		t.Fatalf("ожидали 2 элемента, получили %d", len(items))
	}
	if items[0].Title != "Новость 1" || items[0].Link != "https://sk.ru/n/1" {
		t.Errorf("неверно разобран первый элемент: %+v", items[0])
	}
	if items[0].Published == nil {
		t.Error("дата первого элемента не разобрана")
	}
}

func TestParseAtom(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry><title>Событие</title><summary>Описание</summary><updated>2026-05-29T10:00:00Z</updated>
    <link rel="alternate" href="https://sk.ru/e/1"/></entry>
</feed>`)
	items := Parse(data)
	if len(items) != 1 {
		t.Fatalf("ожидали 1 запись, получили %d", len(items))
	}
	if items[0].Link != "https://sk.ru/e/1" {
		t.Errorf("ссылка Atom не разобрана: %q", items[0].Link)
	}
}

func TestStripTags(t *testing.T) {
	if got := StripTags("Текст <b>жирный</b> и <a href='x'>ссылка</a>"); got != "Текст жирный и ссылка" {
		t.Errorf("StripTags = %q", got)
	}
}
