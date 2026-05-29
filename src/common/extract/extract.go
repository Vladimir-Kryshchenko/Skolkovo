// Package extract извлекает текст из документов разных форматов
// (PDF, DOCX, HTML, TXT/MD) для последующей индексации в RAG.
package extract

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
	"golang.org/x/net/html"
)

// SupportedExt перечисляет расширения, из которых умеем извлекать текст.
var SupportedExt = map[string]bool{
	".pdf": true, ".docx": true, ".html": true, ".htm": true, ".txt": true, ".md": true,
}

// IsSupported сообщает, поддерживается ли формат файла по расширению.
func IsSupported(path string) bool {
	return SupportedExt[strings.ToLower(filepath.Ext(path))]
}

// Text извлекает текстовое содержимое файла по его расширению.
func Text(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return fromPDF(path)
	case ".docx":
		return fromDOCX(path)
	case ".html", ".htm":
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return HTMLText(data)
	case ".txt", ".md":
		data, err := os.ReadFile(path)
		return string(data), err
	default:
		return "", fmt.Errorf("формат не поддерживается: %s", filepath.Ext(path))
	}
}

func fromPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	rd, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rd); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// fromDOCX распаковывает word/document.xml и извлекает текстовые узлы.
func fromDOCX(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		return docxXMLText(data), nil
	}
	return "", fmt.Errorf("docx: word/document.xml не найден")
}

// docxXMLText вытаскивает содержимое тегов <w:t> и разбивает абзацы по <w:p>.
func docxXMLText(data []byte) string {
	z := html.NewTokenizer(bytes.NewReader(data))
	var b strings.Builder
	inText := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "w:t", "t":
				inText = true
			case "w:p", "p", "w:br", "br":
				b.WriteString("\n")
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "w:t" || string(name) == "t" {
				inText = false
			}
		case html.TextToken:
			if inText {
				b.Write(z.Text())
			}
		}
	}
}

// HTMLText извлекает видимый текст из HTML, отбрасывая script/style.
func HTMLText(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				b.WriteString(t)
				b.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.TrimSpace(b.String()), nil
}
