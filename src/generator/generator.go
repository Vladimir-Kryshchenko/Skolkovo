// Package generator генерирует документы из шаблонов, подставляя данные клиента.
package generator

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"baza-skolkovo/src/common/model"
)

// ClientStore — интерфейс для получения данных клиента.
type ClientStore interface {
	GetClient(ctx context.Context, clientID string) (*model.Client, error)
	ListClientDocuments(ctx context.Context, clientID string) ([]model.ClientDocument, error)
}

// TemplateStore — интерфейс для работы с шаблонами документов.
type TemplateStore interface {
	GetTemplate(ctx context.Context, templateID string) (*model.DocumentTemplate, error)
	ListTemplates(ctx context.Context) ([]model.DocumentTemplate, error)
	ReadTemplateFile(ctx context.Context, templateFile string) ([]byte, error)
}

// GeneratorConfig — конфигурация генератора документов.
type GeneratorConfig struct {
	// TemplateDir — путь к директории с шаблонами.
	TemplateDir string
	// OutputDir — путь для выходных (сгенерированных) файлов.
	OutputDir string
	// DefaultFormat — формат по умолчанию: "pdf" или "docx".
	DefaultFormat string
}

// ApplyDefaults заполняет пустые поля значениями по умолчанию.
func (c *GeneratorConfig) ApplyDefaults() {
	if c.TemplateDir == "" {
		c.TemplateDir = "./templates"
	}
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join("Документы_Сколково", "Сгенерированные")
	}
	if c.DefaultFormat == "" {
		c.DefaultFormat = "pdf"
	}
}

// DocumentGenerator — генератор документов из шаблонов.
type DocumentGenerator struct {
	config        GeneratorConfig
	clientStore   ClientStore
	templateStore TemplateStore
}

// NewDocumentGenerator создаёт новый экземпляр DocumentGenerator.
func NewDocumentGenerator(config GeneratorConfig, clientStore ClientStore, templateStore TemplateStore) *DocumentGenerator {
	config.ApplyDefaults()
	return &DocumentGenerator{
		config:        config,
		clientStore:   clientStore,
		templateStore: templateStore,
	}
}

// RenderTemplate рендерит шаблон с данными клиента и кастомными переменными.
//
// supported extensions:
//
//	.go.tpl  → рендерит Go template → сохраняет как HTML → конвертирует в PDF
//	.docx.tpl → рендерит Go template → сохраняет как DOCX
//	.html.tpl → рендерит Go template → сохраняет как HTML
func (g *DocumentGenerator) RenderTemplate(ctx context.Context, templateID, clientID string, variables map[string]string) (outputPath string, err error) {
	// 1. Загружаем шаблон
	tmpl, err := g.templateStore.GetTemplate(ctx, templateID)
	if err != nil {
		return "", fmt.Errorf("generator: загрузка шаблона %q: %w", templateID, err)
	}

	// 2. Загружаем данные клиента
	client, err := g.clientStore.GetClient(ctx, clientID)
	if err != nil {
		return "", fmt.Errorf("generator: загрузка клиента %q: %w", clientID, err)
	}

	// 3. Загружаем файл шаблона
	tplContent, err := g.templateStore.ReadTemplateFile(ctx, tmpl.TemplateFile)
	if err != nil {
		return "", fmt.Errorf("generator: чтение файла шаблона %q: %w", tmpl.TemplateFile, err)
	}

	// 4. Определяем тип шаблона по расширению
	tplExt := templateExtension(tmpl.TemplateFile)

	// 5. Подготавливаем данные для рендеринга
	data := g.buildTemplateData(client, variables)

	// 6. Рендерим Go template
	tpl, err := template.New(templateID).Funcs(template.FuncMap{
		"date":  func() string { return data["Date"].(string) },
		"today": func() string { return data["Today"].(string) },
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.ToTitle,
	}).Parse(string(tplContent))
	if err != nil {
		return "", fmt.Errorf("generator: парсинг шаблона %q: %w", templateID, err)
	}

	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("generator: рендеринг шаблона %q: %w", templateID, err)
	}

	// 7. Формируем имя выходного файла
	now := time.Now()
	baseName := fmt.Sprintf("%s_%s_%s",
		sanitizeFilename(client.Name),
		sanitizeFilename(tmpl.Name),
		now.Format("2006-01-02"))

	// 8. Сохраняем результат в зависимости от типа
	switch tplExt {
	case ".go.tpl":
		// Рендерим в HTML, затем конвертируем в PDF
		htmlPath := filepath.Join(g.config.OutputDir, baseName+".html")
		if err := os.MkdirAll(g.config.OutputDir, 0o755); err != nil {
			return "", fmt.Errorf("generator: создание директории %q: %w", g.config.OutputDir, err)
		}
		if err := os.WriteFile(htmlPath, rendered.Bytes(), 0o644); err != nil {
			return "", fmt.Errorf("generator: сохранение HTML %q: %w", htmlPath, err)
		}
		outputPath = filepath.Join(g.config.OutputDir, baseName+".pdf")
		if err := GeneratePDF(rendered.String(), outputPath); err != nil {
			return "", fmt.Errorf("generator: конвертация в PDF: %w", err)
		}

	case ".docx.tpl":
		outputPath = filepath.Join(g.config.OutputDir, baseName+".docx")
		if err := os.MkdirAll(g.config.OutputDir, 0o755); err != nil {
			return "", fmt.Errorf("generator: создание директории %q: %w", g.config.OutputDir, err)
		}
		if err := GenerateDOCX(rendered.String(), variables, outputPath); err != nil {
			return "", fmt.Errorf("generator: генерация DOCX: %w", err)
		}

	case ".html.tpl":
		outputPath = filepath.Join(g.config.OutputDir, baseName+".html")
		if err := os.MkdirAll(g.config.OutputDir, 0o755); err != nil {
			return "", fmt.Errorf("generator: создание директории %q: %w", g.config.OutputDir, err)
		}
		if err := os.WriteFile(outputPath, rendered.Bytes(), 0o644); err != nil {
			return "", fmt.Errorf("generator: сохранение HTML %q: %w", outputPath, err)
		}

	default:
		return "", fmt.Errorf("generator: неизвестный тип шаблона %q (файл: %s)", tplExt, tmpl.TemplateFile)
	}

	return outputPath, nil
}

// buildTemplateData подготавливает данные для Go template.
func (g *DocumentGenerator) buildTemplateData(client *model.Client, variables map[string]string) map[string]interface{} {
	now := time.Now()
	data := map[string]interface{}{
		"Client": map[string]string{
			"Name":           client.Name,
			"INN":            client.INN,
			"ContactEmail":   client.ContactEmail,
			"ContactPhone":   client.ContactPhone,
			"ResidencyStage": string(client.ResidencyStage),
		},
		"Today": now.Format("02.01.2006"),
		"Date":  now.Format("02.01.2006"),
	}
	for k, v := range variables {
		data[k] = v
	}
	return data
}

// GeneratePDF сохраняет HTML-контент как PDF.
//
// MVP: сохраняет HTML с .html расширением (полноценная PDF-конверсия
// требует wkhtmltopdf или headless Chrome).
//
// TODO: подключить chromedp для PDF-конверсии:
//
//	import "github.com/chromedp/chromedp"
//
//	func GeneratePDF(htmlContent, outputPath string) error {
//	    ctx, cancel := chromedp.NewContext(context.Background())
//	    defer cancel()
//	    var buf []byte
//	    err := chromedp.Run(ctx,
//	        chromedp.Navigate("data:text/html,"+url.QueryEscape(htmlContent)),
//	        chromedp.ActionFunc(func(ctx context.Context) error {
//	            var err error
//	            buf, err = page.PrintToPDF().WithPrintBackground(true).Do(ctx)
//	            return err
//	        }),
//	    )
//	    if err != nil { return err }
//	    return os.WriteFile(outputPath, buf, 0o644)
//	}
func GeneratePDF(htmlContent string, outputPath string) error {
	// Для MVP сохраняем как HTML-файл с .pdf расширением
	// Реальный PDF можно получить через chromedp (см. TODO выше)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("generatePDF: создание директории: %w", err)
	}

	// Оборачиваем в полноценный HTML-документ
	fullHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
body { font-family: "Times New Roman", serif; margin: 2cm; line-height: 1.5; }
h1 { text-align: center; }
p { text-align: justify; }
</style>
</head>
<body>
%s
</body>
</html>`, htmlContent)

	return os.WriteFile(outputPath, []byte(fullHTML), 0o644)
}

// GenerateDOCX создаёт DOCX-файл из HTML-контента.
//
// DOCX — это ZIP-архив с XML-файлами внутри.
// Для MVP создаём простую ZIP-структуру с word/document.xml.
//
// TODO: полноценная генерация через github.com/nguyenthenguyen/docx:
//
//	import "github.com/nguyenthenguyen/docx"
//
//	func GenerateDOCX(content string, variables map[string]string, outputPath string) error {
//	    r, err := docx.ReadDocxFromMemory("template.docx", 0o755)
//	    if err != nil { return err }
//	    doc := r.Editable()
//	    for k, v := range variables {
//	        doc.Replace(k, v)
//	    }
//	    return doc.WriteToFile(outputPath)
//	}
func GenerateDOCX(templateContent string, variables map[string]string, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("generateDOCX: создание директории: %w", err)
	}

	// Подставляем переменные в контент (простая замена {{.Key}} → value)
	content := templateContent
	for k, v := range variables {
		content = strings.ReplaceAll(content, "{{."+k+"}}", v)
	}

	// Формируем минимальную DOCX-структуру (ZIP с XML)
	// DOCX = ZIP containing [Content_Types].xml, word/document.xml, _rels/.rels
	if err := writeMinimalDOCX(content, outputPath); err != nil {
		return fmt.Errorf("generateDOCX: запись файла: %w", err)
	}

	return nil
}

// writeMinimalDOCX создаёт минимальный валидный DOCX файл.
func writeMinimalDOCX(bodyContent string, outputPath string) error {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// [Content_Types].xml
	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	if err := writeFileToZip(zw, "[Content_Types].xml", contentTypes); err != nil {
		return err
	}

	// _rels/.rels
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	if err := writeFileToZip(zw, "_rels/.rels", rels); err != nil {
		return err
	}

	// word/document.xml
	// Экранируем XML-опасные символы
	escaped := escapeXML(bodyContent)
	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:body>
<w:p>
<w:r>
<w:t xml:space="preserve">%s</w:t>
</w:r>
</w:p>
</w:body>
</w:document>`, escaped)
	if err := writeFileToZip(zw, "word/document.xml", docXML); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("writeMinimalDOCX: закрытие ZIP: %w", err)
	}

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

// CreateDefaultTemplates создаёт стандартные шаблоны в TemplateDir.
func (g *DocumentGenerator) CreateDefaultTemplates() error {
	templates := map[string]string{
		"Заявление_на_резидентство.go.tpl": заявлениеНаРезидентство,
		"Квартальный_отчёт.go.tpl":         квартальныйОтчёт,
		"Годовой_отчёт.go.tpl":             годовойОтчёт,
		"Запрос_на_продление.go.tpl":       запросНаПродление,
		"Уведомление_о_выходе.go.tpl":      уведомлениеОВыходе,
	}

	if err := os.MkdirAll(g.config.TemplateDir, 0o755); err != nil {
		return fmt.Errorf("CreateDefaultTemplates: создание директории %q: %w", g.config.TemplateDir, err)
	}

	for name, content := range templates {
		path := filepath.Join(g.config.TemplateDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("CreateDefaultTemplates: запись %q: %w", name, err)
		}
	}

	return nil
}

// ListAvailableTemplates возвращает список доступных шаблонов.
func (g *DocumentGenerator) ListAvailableTemplates(ctx context.Context) ([]string, error) {
	tmpls, err := g.templateStore.ListTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListAvailableTemplates: %w", err)
	}

	names := make([]string, 0, len(tmpls))
	for _, t := range tmpls {
		names = append(names, t.ID)
	}
	return names, nil
}

// ---- Утилиты ----

func templateExtension(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".docx.tpl"):
		return ".docx.tpl"
	case strings.HasSuffix(lower, ".html.tpl"):
		return ".html.tpl"
	case strings.HasSuffix(lower, ".go.tpl"):
		return ".go.tpl"
	default:
		return filepath.Ext(lower)
	}
}

func sanitizeFilename(name string) string {
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, ch := range invalid {
		result = strings.ReplaceAll(result, ch, "_")
	}
	return strings.TrimSpace(result)
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func writeFileToZip(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("writeFileToZip %q: %w", name, err)
	}
	_, err = w.Write([]byte(content))
	return err
}

// ---- Стандартные шаблоны ----

const заявлениеНаРезидентство = `<h1>ЗАЯВЛЕНИЕ НА РЕЗИДЕНТСТВО</h1>
<p>в Фонде "Сколково"</p>

<p>От: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Контактный email: {{.Client.ContactEmail}}</p>
<p>Контактный телефон: {{.Client.ContactPhone}}</p>
<p>Стадия резидентства: {{.Client.ResidencyStage}}</p>

<h2>Заявление</h2>
<p>Настоящим {{.Client.Name}} (ИНН {{.Client.INN}}) обращается с заявлением о вступлении в число резидентов Фонда "Сколково".</p>
<p>Подтверждаем, что организация соответствует критериям резидента Фонда "Сколково" в соответствии с Федеральным законом от 28.09.2010 N 244-ФЗ.</p>
<p>Обязуемся соблюдать условия деятельности в качестве резидента Фонда "Сколково".</p>

<p>Дата: {{.Date}}</p>
<p>Подпись: _________________</p>`

const квартальныйОтчёт = `<h1>КВАРТАЛЬНЫЙ ОТЧЁТ РЕЗИДЕНТА</h1>

<p>Резидент: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Отчётный период: {{.Quarter | default "I квартал"}} {{.Year | default "2026"}}</p>
<p>Дата формирования: {{.Date}}</p>

<h2>1. Общая информация</h2>
<p>Наименование организации: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Стадия резидентства: {{.Client.ResidencyStage}}</p>

<h2>2. Результаты деятельности за отчётный период</h2>
<p>{{.ActivityResult | default "Деятельность осуществлялась в соответствии с утверждённой программой."}}</p>

<h2>3. Финансовые показатели</h2>
<p>{{.FinancialResult | default "Финансовые показатели соответствуют утверждённому бюджету."}}</p>

<h2>4. Использование гранта</h2>
<p>{{.GrantUsage | default "Использование гранта осуществляется в соответствии с условиями соглашения."}}</p>

<p>Контактный email: {{.Client.ContactEmail}}</p>
<p>Контактный телефон: {{.Client.ContactPhone}}</p>

<p>Подпись: _________________</p>`

const годовойОтчёт = `<h1>ГОДОВОЙ ОТЧЁТ РЕЗИДЕНТА</h1>

<p>Резидент: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Отчётный год: {{.Year | default "2026"}}</p>
<p>Дата формирования: {{.Date}}</p>

<h2>1. Общая информация о резиденте</h2>
<p>Наименование: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Стадия резидентства: {{.Client.ResidencyStage}}</p>
<p>Email: {{.Client.ContactEmail}}</p>
<p>Телефон: {{.Client.ContactPhone}}</p>

<h2>2. Основные результаты деятельности</h2>
<p>{{.AnnualResults | default "За отчётный период организация выполнила ключевые показатели деятельности."}}</p>

<h2>3. Финансовый отчёт</h2>
<p>{{.FinancialSummary | default "Финансовые результаты соответствуют утверждённому бюджету."}}</p>

<h2>4. Научно-технические результаты</h2>
<p>{{.ScienceResults | default "Получены результаты в соответствии с программой исследований."}}</p>

<h2>5. План на следующий год</h2>
<p>{{.NextYearPlan | default "Планируется продолжение деятельности в соответствии с программой."}}</p>

<p>Подпись руководителя: _________________</p>`

const запросНаПродление = `<h1>ЗАПРОС НА ПРОДЛЕНИЕ РЕЗИДЕНТСТВА</h1>

<p>От: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Email: {{.Client.ContactEmail}}</p>
<p>Телефон: {{.Client.ContactPhone}}</p>
<p>Текущая стадия: {{.Client.ResidencyStage}}</p>
<p>Дата запроса: {{.Date}}</p>

<h2>Запрос</h2>
<p>{{.Client.Name}} (ИНН {{.Client.INN}}) обращается с запросом о продлении резидентства в Фонде "Сколково".</p>

<h2>Обоснование</h2>
<p>{{.ExtensionReason | default "Необходимость продления связана с продолжением научно-исследовательской деятельности."}}</p>

<h2>Дополнительная информация</h2>
<p>{{.AdditionalInfo | default "Все отчёты представлены в срок. Задолженности отсутствуют."}}</p>

<p>Срок продления: {{.ExtensionPeriod | default "12 месяцев"}}</p>

<p>Подпись: _________________</p>`

const уведомлениеОВыходе = `<h1>УВЕДОМЛЕНИЕ О ВЫХОДЕ ИЗ ПРОЕКТА</h1>

<p>От: {{.Client.Name}}</p>
<p>ИНН: {{.Client.INN}}</p>
<p>Email: {{.Client.ContactEmail}}</p>
<p>Телефон: {{.Client.ContactPhone}}</p>
<p>Стадия: {{.Client.ResidencyStage}}</p>
<p>Дата уведомления: {{.Date}}</p>

<h2>Уведомление</h2>
<p>{{.Client.Name}} (ИНН {{.Client.INN}}) настоящим уведомляет о выходе из числа резидентов Фонда "Сколково".</p>

<h2>Причина выхода</h2>
<p>{{.ExitReason | default "Выход связан с завершением проекта и выполнением всех обязательств."}}</p>

<h2>Обязательства</h2>
<p>{{.Obligations | default "Все обязательства перед Фондом исполнены. Отчёты представлены в полном объёме."}}</p>

<p>Дата вступления в силу: {{.EffectiveDate | default "30 календарных дней с даты уведомления"}}</p>

<p>Подпись руководителя: _________________</p>`
