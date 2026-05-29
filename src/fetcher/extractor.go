// Package fetcher — дополнение: извлечение текста из скачанных файлов (PDF, DOCX, TXT).
// Используется для обогащения RAG-индекса содержимым документов, а не только их метаданными.
package fetcher

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

// ExtractedText — результат извлечения текста из файла.
type ExtractedText struct {
	FilePath string
	Text     string
	Pages    int    // для PDF: число страниц
	Format   string // "pdf" | "docx" | "txt" | "unknown"
	Err      error
}

// ExtractText извлекает текст из файла по его пути.
// Поддерживает PDF, DOCX (базовый XML-парсинг), TXT.
func ExtractText(filePath string) ExtractedText {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf":
		return extractPDF(filePath)
	case ".docx", ".doc":
		return extractDOCX(filePath)
	case ".txt", ".md":
		return extractTXT(filePath)
	default:
		return ExtractedText{
			FilePath: filePath,
			Format:   "unknown",
			Err:      fmt.Errorf("неподдерживаемый формат: %s", ext),
		}
	}
}

// extractPDF извлекает текст из PDF с помощью ledongthuc/pdf.
func extractPDF(filePath string) ExtractedText {
	result := ExtractedText{FilePath: filePath, Format: "pdf"}

	f, r, err := pdf.Open(filePath)
	if err != nil {
		result.Err = fmt.Errorf("открытие PDF: %w", err)
		return result
	}
	defer f.Close()

	totalPages := r.NumPage()
	result.Pages = totalPages

	var sb strings.Builder

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Страница недоступна — пропускаем.
			continue
		}

		sb.WriteString(text)
		sb.WriteString("\n")

		// Ограничение: не более 500 000 символов.
		if sb.Len() > 500_000 {
			sb.WriteString("\n[текст обрезан — документ содержит более 500 000 символов]")
			break
		}
	}

	result.Text = cleanExtractedText(sb.String())
	return result
}

// extractDOCX извлекает текст из DOCX (ZIP с XML внутри).
// Использует базовый XML-парсинг без внешних зависимостей.
func extractDOCX(filePath string) ExtractedText {
	result := ExtractedText{FilePath: filePath, Format: "docx"}

	data, err := os.ReadFile(filePath)
	if err != nil {
		result.Err = fmt.Errorf("чтение DOCX: %w", err)
		return result
	}

	// DOCX — это ZIP-архив. Ищем word/document.xml внутри.
	text, err := extractFromZipXML(data, "word/document.xml")
	if err != nil {
		// Пробуем как старый DOC (бинарный формат).
		text = extractFromBinaryDOC(data)
	}

	result.Text = cleanExtractedText(text)
	return result
}

// extractFromZipXML распаковывает ZIP и читает XML-файл word/document.xml.
func extractFromZipXML(data []byte, xmlPath string) (string, error) {
	// Минимальный ZIP-ридер без archive/zip чтобы не зависеть от stdlib.
	// Ищем сигнатуры и Central Directory.
	reader := bytes.NewReader(data)

	// Ищем PK\x03\x04 (Local File Header) для нужного файла.
	target := []byte(xmlPath)
	var xmlContent []byte

	for i := 0; i < len(data)-4; i++ {
		// Local file header signature
		if data[i] == 0x50 && data[i+1] == 0x4B && data[i+2] == 0x03 && data[i+3] == 0x04 {
			if i+30 >= len(data) {
				break
			}
			// fnLen = bytes 26-27 (little-endian)
			fnLen := int(data[i+26]) | int(data[i+27])<<8
			// exLen = bytes 28-29
			exLen := int(data[i+28]) | int(data[i+29])<<8

			nameStart := i + 30
			if nameStart+fnLen > len(data) {
				break
			}
			name := data[nameStart : nameStart+fnLen]

			if bytes.Equal(name, target) {
				// Нашли нужный файл.
				dataOffset := nameStart + fnLen + exLen
				// compSize = bytes 18-21
				compSize := int(data[i+18]) | int(data[i+19])<<8 | int(data[i+20])<<16 | int(data[i+21])<<24
				// compMethod = bytes 8-9
				compMethod := int(data[i+8]) | int(data[i+9])<<8

				if dataOffset+compSize > len(data) {
					break
				}

				compressed := data[dataOffset : dataOffset+compSize]
				if compMethod == 0 {
					// Stored (без сжатия)
					xmlContent = compressed
				} else if compMethod == 8 {
					// Deflate
					xmlContent = decompressDeflate(compressed)
				}
				break
			}
		}
	}
	_ = reader

	if xmlContent == nil {
		return "", fmt.Errorf("word/document.xml не найден в DOCX")
	}

	return extractTextFromWordXML(xmlContent), nil
}

// decompressDeflate распаковывает deflate-сжатые данные.
func decompressDeflate(data []byte) []byte {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	result, err := io.ReadAll(r)
	if err != nil {
		return nil
	}
	return result
}

// extractTextFromWordXML извлекает текст из XML word/document.xml.
// Ищет содержимое тегов <w:t>.
func extractTextFromWordXML(xmlData []byte) string {
	var sb strings.Builder
	content := string(xmlData)

	// Простой XML-парсер: ищем <w:t>...</w:t> и <w:t xml:space="preserve">...</w:t>
	for {
		start := strings.Index(content, "<w:t")
		if start == -1 {
			break
		}
		// Пропускаем открывающий тег.
		end := strings.Index(content[start:], ">")
		if end == -1 {
			break
		}
		textStart := start + end + 1

		// Ищем закрывающий тег.
		closeTag := strings.Index(content[textStart:], "</w:t>")
		if closeTag == -1 {
			break
		}

		text := content[textStart : textStart+closeTag]
		if text != "" {
			sb.WriteString(text)
		}

		content = content[textStart+closeTag+6:]
	}

	return sb.String()
}

// extractFromBinaryDOC пытается извлечь читаемый текст из бинарного DOC.
func extractFromBinaryDOC(data []byte) string {
	// Базовое извлечение: берём все ASCII-читаемые последовательности длиной > 4 символов.
	var sb strings.Builder
	var current strings.Builder

	for _, b := range data {
		if (b >= 0x20 && b < 0x7F) || b == 0x0A || b == 0x0D {
			current.WriteByte(b)
		} else {
			if current.Len() > 4 {
				sb.WriteString(current.String())
				sb.WriteString(" ")
			}
			current.Reset()
		}
	}

	return sb.String()
}

// extractTXT читает текстовый файл как есть.
func extractTXT(filePath string) ExtractedText {
	result := ExtractedText{FilePath: filePath, Format: "txt"}

	data, err := os.ReadFile(filePath)
	if err != nil {
		result.Err = fmt.Errorf("чтение TXT: %w", err)
		return result
	}

	result.Text = cleanExtractedText(string(data))
	return result
}

// cleanExtractedText нормализует извлечённый текст:
// убирает лишние пробелы, нормализует переносы строк.
func cleanExtractedText(text string) string {
	// Нормализуем переносы строк.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// Убираем тройные и более переносы строк.
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	// Убираем trailing пробелы в строках.
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	text = strings.Join(lines, "\n")

	return strings.TrimSpace(text)
}

// ExtractAndSaveText извлекает текст из файла и сохраняет рядом с ним в .txt файл.
// Возвращает путь к .txt файлу.
func ExtractAndSaveText(filePath string) (string, error) {
	extracted := ExtractText(filePath)
	if extracted.Err != nil {
		return "", extracted.Err
	}
	if extracted.Text == "" {
		return "", fmt.Errorf("не удалось извлечь текст из %s", filePath)
	}

	txtPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".txt"
	if err := os.WriteFile(txtPath, []byte(extracted.Text), 0644); err != nil {
		return "", fmt.Errorf("сохранение текста: %w", err)
	}

	return txtPath, nil
}

// IsSupportedFormat проверяет, поддерживается ли формат файла для извлечения текста.
func IsSupportedFormat(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf", ".docx", ".doc", ".txt", ".md":
		return true
	}
	return false
}
