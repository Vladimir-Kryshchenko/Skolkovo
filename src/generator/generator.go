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
	// ChromePath — путь к Chrome/Edge для генерации настоящего PDF через headless-браузер.
	// Если пусто — PDF сохраняется как HTML-файл (fallback без бинарной конвертации).
	ChromePath string
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

// OutputDir возвращает директорию, куда сохраняются сгенерированные документы.
func (g *DocumentGenerator) OutputDir() string { return g.config.OutputDir }

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
		// Если задан ChromePath — генерируем настоящий PDF через headless-браузер;
		// при ошибке откатываемся на HTML-fallback, чтобы документ всё равно был выдан.
		if g.config.ChromePath != "" {
			if err := generatePDFChrome(ctx, g.config.ChromePath, rendered.String(), outputPath); err != nil {
				if ferr := GeneratePDF(rendered.String(), outputPath); ferr != nil {
					return "", fmt.Errorf("generator: конвертация в PDF (chrome: %v): %w", err, ferr)
				}
			}
		} else if err := GeneratePDF(rendered.String(), outputPath); err != nil {
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

// GeneratePDF сохраняет HTML-контент как PDF (HTML-fallback без headless-браузера).
func GeneratePDF(htmlContent string, outputPath string) error {
	// Fallback без headless-браузера: сохраняем оформленный HTML с .pdf расширением.
	// Настоящий бинарный PDF генерируется через generatePDFChrome при заданном ChromePath.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("generatePDF: создание директории: %w", err)
	}
	return os.WriteFile(outputPath, []byte(wrapHTML(htmlContent)), 0o644)
}

// wrapHTML оборачивает тело документа в полноценный HTML с печатным оформлением.
func wrapHTML(body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
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
</html>`, body)
}

// GenerateDOCX создаёт DOCX-файл из HTML-контента.
//
// DOCX — это ZIP-архив с XML-файлами внутри: [Content_Types].xml, _rels/.rels,
// word/document.xml. HTML-разметка тела конвертируется в отдельные абзацы Word
// (заголовки, абзацы, пункты списков), а не вставляется одной строкой.
func GenerateDOCX(templateContent string, variables map[string]string, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("generateDOCX: создание директории: %w", err)
	}

	// Подставляем переменные в контент (простая замена {{.Key}} → value)
	content := templateContent
	for k, v := range variables {
		content = strings.ReplaceAll(content, "{{."+k+"}}", v)
	}

	if err := writeMinimalDOCX(htmlToParagraphs(content), outputPath); err != nil {
		return fmt.Errorf("generateDOCX: запись файла: %w", err)
	}

	return nil
}

// writeMinimalDOCX создаёт валидный DOCX-файл из набора абзацев.
func writeMinimalDOCX(paragraphs []string, outputPath string) error {
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

	// word/document.xml — каждый абзац в отдельном <w:p>.
	var body strings.Builder
	if len(paragraphs) == 0 {
		body.WriteString("<w:p/>")
	}
	for _, p := range paragraphs {
		body.WriteString(fmt.Sprintf("<w:p><w:r><w:t xml:space=\"preserve\">%s</w:t></w:r></w:p>", escapeXML(p)))
	}
	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:body>
%s
</w:body>
</w:document>`, body.String())
	if err := writeFileToZip(zw, "word/document.xml", docXML); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("writeMinimalDOCX: закрытие ZIP: %w", err)
	}

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

// htmlToParagraphs превращает HTML-тело в плоский список текстовых абзацев:
// блочные теги (p, h1-h6, li, br, div, tr) становятся границами абзацев,
// остальные теги вырезаются, HTML-сущности декодируются.
func htmlToParagraphs(s string) []string {
	// Блочные закрывающие/одиночные теги → разделитель абзацев.
	repl := strings.NewReplacer(
		"</p>", "\n", "</P>", "\n",
		"</h1>", "\n", "</h2>", "\n", "</h3>", "\n", "</h4>", "\n", "</h5>", "\n", "</h6>", "\n",
		"</li>", "\n", "</div>", "\n", "</tr>", "\n",
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
	)
	s = repl.Replace(s)

	// Удаляем оставшиеся теги.
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}

	// Декодируем базовые HTML-сущности.
	unescaper := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&apos;", "'", "&nbsp;", " ",
	)

	var paragraphs []string
	for _, line := range strings.Split(b.String(), "\n") {
		line = strings.TrimSpace(unescaper.Replace(line))
		if line != "" {
			paragraphs = append(paragraphs, line)
		}
	}
	return paragraphs
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

// ListTemplateInfos возвращает полные метаданные доступных шаблонов.
func (g *DocumentGenerator) ListTemplateInfos(ctx context.Context) ([]model.DocumentTemplate, error) {
	return g.templateStore.ListTemplates(ctx)
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
