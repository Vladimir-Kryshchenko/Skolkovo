package extract

import (
	"strings"
	"testing"
)

func TestHTMLText(t *testing.T) {
	html := []byte(`<html><head><style>.x{}</style><script>var a=1;</script></head>
	<body><h1>Заголовок</h1><p>Текст документа Сколково.</p></body></html>`)
	got, err := HTMLText(html)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Заголовок") || !strings.Contains(got, "Сколково") {
		t.Errorf("не извлечён ожидаемый текст: %q", got)
	}
	if strings.Contains(got, "var a") {
		t.Errorf("содержимое script не должно попадать в текст: %q", got)
	}
}

func TestIsSupported(t *testing.T) {
	for _, p := range []string{"a.pdf", "b.DOCX", "c.html", "d.txt"} {
		if !IsSupported(p) {
			t.Errorf("%s должен поддерживаться", p)
		}
	}
	if IsSupported("e.exe") {
		t.Error("exe не должен поддерживаться")
	}
}
