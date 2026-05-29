// Package agents — DocumentDraftingAgent: заполняет шаблоны документов Сколково
// на основе профиля компании-клиента и требований из RAG-базы.
package agents

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
	rag "baza-skolkovo/src/rag_service"
)

// DraftRequest — запрос на подготовку черновика документа.
type DraftRequest struct {
	// ClientID — идентификатор клиента, для которого готовится документ.
	ClientID string `json:"client_id"`
	// DocumentType — тип документа: "application", "project_description", "report",
	// "extension_request", "exit_notice", "ird_description".
	DocumentType string `json:"document_type"`
	// TemplateID — ID шаблона из TemplateStore (опционально).
	// Если не указан, используется стандартный шаблон для типа.
	TemplateID string `json:"template_id,omitempty"`
	// ExtraContext — дополнительный контекст от консультанта.
	ExtraContext string `json:"extra_context,omitempty"`
	// Variables — явно переданные переменные для подстановки.
	Variables map[string]string `json:"variables,omitempty"`
}

// DraftResult — результат подготовки черновика.
type DraftResult struct {
	// DocumentType — тип документа.
	DocumentType string `json:"document_type"`
	// ClientID — клиент.
	ClientID string `json:"client_id"`
	// Title — заголовок подготовленного документа.
	Title string `json:"title"`
	// Content — содержимое черновика (Markdown).
	Content string `json:"content"`
	// FilledVariables — подставленные переменные.
	FilledVariables map[string]string `json:"filled_variables"`
	// MissingFields — поля, которые не удалось заполнить (требуется вмешательство).
	MissingFields []string `json:"missing_fields,omitempty"`
	// Warnings — предупреждения (например, некоторые данные заполнены приблизительно).
	Warnings []string `json:"warnings,omitempty"`
	// RequirementsUsed — список документов-требований, использованных при составлении.
	RequirementsUsed []string `json:"requirements_used,omitempty"`
	// GeneratedAt — время генерации.
	GeneratedAt time.Time `json:"generated_at"`
}

// DraftingStores — хранилища, необходимые агенту подготовки документов.
type DraftingStores struct {
	ClientStore   store.ClientStore
	TemplateStore store.TemplateStore
	ChecklistStore store.ChecklistStore
}

// DocumentDraftingAgent — агент, подготавливающий черновики документов.
type DocumentDraftingAgent struct {
	clientStore    store.ClientStore
	templateStore  store.TemplateStore
	checklistStore store.ChecklistStore
	ragService     *rag.Service
}

// NewDocumentDraftingAgent создаёт агента подготовки документов.
func NewDocumentDraftingAgent(stores DraftingStores, ragSvc *rag.Service) *DocumentDraftingAgent {
	return &DocumentDraftingAgent{
		clientStore:    stores.ClientStore,
		templateStore:  stores.TemplateStore,
		checklistStore: stores.ChecklistStore,
		ragService:     ragSvc,
	}
}

// Draft генерирует черновик документа для клиента.
func (a *DocumentDraftingAgent) Draft(ctx context.Context, req DraftRequest) (*DraftResult, error) {
	if strings.TrimSpace(req.ClientID) == "" {
		return nil, fmt.Errorf("clientID не указан")
	}
	if strings.TrimSpace(req.DocumentType) == "" {
		return nil, fmt.Errorf("documentType не указан")
	}

	result := &DraftResult{
		DocumentType:    req.DocumentType,
		ClientID:        req.ClientID,
		FilledVariables: make(map[string]string),
		GeneratedAt:     time.Now(),
	}

	// 1. Получаем профиль клиента.
	client, err := a.clientStore.GetClient(ctx, req.ClientID)
	if err != nil {
		return nil, fmt.Errorf("получение клиента: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("клиент %s не найден", req.ClientID)
	}

	// 2. Получаем переменные из профиля клиента.
	vars := a.extractClientVariables(client)
	// Добавляем явно переданные переменные (они перекрывают автоматические).
	for k, v := range req.Variables {
		vars[k] = v
	}

	// 3. Загружаем шаблон.
	template := a.loadTemplate(ctx, req.DocumentType, req.TemplateID)

	// 4. Ищем релевантные требования в RAG.
	ragContext := a.fetchRAGContext(ctx, req.DocumentType)
	if len(ragContext) > 0 {
		result.RequirementsUsed = ragContext
	}

	// 5. Генерируем черновик.
	content, missing, warnings := a.generateDraft(req.DocumentType, template, vars, ragContext, req.ExtraContext)

	result.Title = documentTitle(req.DocumentType, client)
	result.Content = content
	result.FilledVariables = vars
	result.MissingFields = missing
	result.Warnings = warnings

	return result, nil
}

// extractClientVariables извлекает переменные из профиля клиента.
func (a *DocumentDraftingAgent) extractClientVariables(client *model.Client) map[string]string {
	vars := make(map[string]string)

	if client.Name != "" {
		vars["company_name"] = client.Name
		vars["contact_name"] = client.Name
	}
	if client.INN != "" {
		vars["inn"] = client.INN
	}
	if client.ContactEmail != "" {
		vars["contact_email"] = client.ContactEmail
	}
	if client.ContactPhone != "" {
		vars["contact_phone"] = client.ContactPhone
	}
	vars["residency_stage"] = string(client.ResidencyStage)
	vars["current_date"] = time.Now().Format("02.01.2006")
	vars["current_year"] = time.Now().Format("2006")

	return vars
}

// loadTemplate загружает шаблон из TemplateStore (файл на диске) или возвращает встроенный.
func (a *DocumentDraftingAgent) loadTemplate(ctx context.Context, docType, templateID string) string {
	// Пробуем загрузить файл шаблона по ID.
	if templateID != "" && a.templateStore != nil {
		if tmpl, err := a.templateStore.GetTemplate(ctx, templateID); err == nil && tmpl != nil && tmpl.TemplateFile != "" {
			if data, err := os.ReadFile(tmpl.TemplateFile); err == nil {
				return string(data)
			}
		}
	}

	// Ищем шаблон по типу документа.
	if a.templateStore != nil {
		if templates, err := a.templateStore.ListTemplates(ctx, docType); err == nil && len(templates) > 0 {
			if templates[0].TemplateFile != "" {
				if data, err := os.ReadFile(templates[0].TemplateFile); err == nil {
					return string(data)
				}
			}
		}
	}

	// Используем встроенный шаблон.
	return builtinTemplate(docType)
}

// fetchRAGContext получает контекст из RAG для конкретного типа документа.
func (a *DocumentDraftingAgent) fetchRAGContext(ctx context.Context, docType string) []string {
	if a.ragService == nil {
		return nil
	}

	query := ragQueryForDocType(docType)
	if query == "" {
		return nil
	}

	results, err := a.ragService.Search(ctx, query, 5)
	if err != nil {
		return nil
	}

	var titles []string
	for _, r := range results {
		if r.Title != "" {
			titles = append(titles, r.Title)
		}
	}
	return titles
}

// generateDraft формирует текст черновика, подставляя переменные в шаблон.
func (a *DocumentDraftingAgent) generateDraft(
	docType, template string,
	vars map[string]string,
	ragContext []string,
	extraContext string,
) (content string, missing []string, warnings []string) {

	// Подставляем переменные в шаблон.
	content = template

	// Все плейсхолдеры вида {{variable_name}}.
	var unfilled []string
	for {
		start := strings.Index(content, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(content[start:], "}}")
		if end == -1 {
			break
		}
		placeholder := content[start+2 : start+end]
		placeholder = strings.TrimSpace(placeholder)

		if val, ok := vars[placeholder]; ok && val != "" {
			content = strings.Replace(content, "{{"+placeholder+"}}", val, 1)
		} else {
			// Помечаем незаполненное поле.
			unfilled = append(unfilled, placeholder)
			content = strings.Replace(content, "{{"+placeholder+"}}", "[_ЗАПОЛНИТЬ: "+placeholder+"_]", 1)
		}
	}

	missing = unfilled

	// Добавляем контекст RAG в конец если есть.
	if len(ragContext) > 0 {
		content += "\n\n---\n*Документы-основания:*\n"
		for _, ref := range ragContext {
			content += fmt.Sprintf("- %s\n", ref)
		}
	}

	// Добавляем дополнительный контекст от консультанта.
	if extraContext != "" {
		content += "\n\n*Примечание консультанта:*\n" + extraContext
	}

	if len(missing) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("Незаполненные поля (%d): %s. Заполните их перед отправкой в Сколково.",
				len(missing), strings.Join(missing, ", ")))
	}

	return content, missing, warnings
}

// documentTitle формирует заголовок документа.
func documentTitle(docType string, client *model.Client) string {
	company := client.Name
	if company == "" {
		company = "Компания"
	}
	switch docType {
	case "application":
		return fmt.Sprintf("Заявка на резидентство Сколково — %s", company)
	case "project_description":
		return fmt.Sprintf("Описание инновационного проекта — %s", company)
	case "report":
		return fmt.Sprintf("Квартальный отчёт резидента — %s (%s)", company, time.Now().Format("Q1 2006"))
	case "extension_request":
		return fmt.Sprintf("Заявление о продлении статуса резидента — %s", company)
	case "exit_notice":
		return fmt.Sprintf("Уведомление о выходе из проекта Сколково — %s", company)
	case "ird_description":
		return fmt.Sprintf("Описание ИРД (интеллектуальной собственности) — %s", company)
	default:
		return fmt.Sprintf("Документ (%s) — %s", docType, company)
	}
}

// ragQueryForDocType возвращает поисковый запрос в RAG для конкретного типа документа.
func ragQueryForDocType(docType string) string {
	switch docType {
	case "application":
		return "требования к заявке на резидентство Сколково критерии входа документы"
	case "project_description":
		return "описание инновационного проекта требования Сколково НИОКР"
	case "report":
		return "квартальный отчёт резидента Сколково требования к отчётности"
	case "extension_request":
		return "продление статуса резидента Сколково условия требования"
	case "exit_notice":
		return "выход из проекта Сколково порядок уведомление"
	case "ird_description":
		return "описание интеллектуальной собственности РИД Сколково"
	default:
		return "документы резидент Сколково требования"
	}
}

// builtinTemplate возвращает встроенный шаблон для типа документа.
func builtinTemplate(docType string) string {
	switch docType {
	case "application":
		return applicationTemplate
	case "project_description":
		return projectDescriptionTemplate
	case "report":
		return reportTemplate
	case "extension_request":
		return extensionTemplate
	case "exit_notice":
		return exitNoticeTemplate
	case "ird_description":
		return irdDescriptionTemplate
	default:
		return genericTemplate
	}
}

// ---------------------------------------------------------------------------
// Встроенные шаблоны документов
// ---------------------------------------------------------------------------

const applicationTemplate = `# Заявка на получение статуса участника проекта «Сколково»

**Дата:** {{current_date}}

## 1. Сведения об организации

| Поле | Значение |
|------|----------|
| Полное наименование | {{company_name}} |
| ИНН | {{inn}} |
| ОГРН | {{ogrn}} |
| Юридический адрес | {{legal_address}} |
| Фактический адрес | {{actual_address}} |
| Телефон | {{contact_phone}} |
| E-mail | {{contact_email}} |
| Руководитель | {{director_name}}, {{director_position}} |

## 2. Описание инновационного проекта

**Название проекта:** {{project_name}}

**Кластер Сколково:** {{cluster}}
*(IT / Биомедицина / Энергоэффективность / Космос и телекоммуникации / Ядерные технологии)*

**Цель проекта:**
{{project_goal}}

**Описание проекта:**
{{project_description}}

**Стадия разработки:**
{{development_stage}}
*(концепция / прототип / MVP / коммерциализация)*

## 3. Инновационность и новизна

**Описание новизны проекта:**
{{innovation_description}}

**Объекты интеллектуальной собственности (при наличии):**
{{ird_list}}

**Планируемые результаты НИОКР:**
{{rd_results}}

## 4. Команда проекта

**Руководитель проекта:**
{{project_lead}}

**Ключевые участники:**
{{team_members}}

## 5. Финансовые показатели

**Текущая выручка:** {{revenue}} руб./год
**Планируемая выручка через 3 года:** {{revenue_3y}} руб./год
**Привлечённые инвестиции:** {{investments}} руб.

## 6. Целевые рынки

{{target_markets}}

## 7. Запрашиваемые льготы

- [ ] Нулевая ставка налога на прибыль
- [ ] Пониженные страховые взносы (14%)
- [ ] Освобождение от НДС на НИОКР
- [ ] Таможенные льготы

---
*Подпись руководителя:* ___________  {{director_name}}
*Дата:* {{current_date}}
`

const projectDescriptionTemplate = `# Описание инновационного проекта

**Организация:** {{company_name}}
**ИНН:** {{inn}}
**Дата:** {{current_date}}

## 1. Название и суть проекта

**Название:** {{project_name}}

**Краткое описание (не более 500 символов):**
{{project_summary}}

## 2. Научно-техническая новизна

{{scientific_novelty}}

**Отличие от существующих аналогов:**
{{competitive_advantage}}

## 3. Технологический уровень

**Уровень готовности технологии (TRL):** {{trl_level}} из 9
**Обоснование:**
{{trl_justification}}

## 4. Практическая значимость

**Целевые потребители:**
{{target_customers}}

**Экономический эффект:**
{{economic_effect}}

## 5. План развития

| Этап | Срок | Результат |
|------|------|-----------|
| Завершение НИОКР | {{rd_deadline}} | {{rd_result}} |
| Создание прототипа | {{prototype_deadline}} | {{prototype_result}} |
| Пилотное внедрение | {{pilot_deadline}} | {{pilot_result}} |
| Коммерциализация | {{commercial_deadline}} | {{commercial_result}} |

## 6. Интеллектуальная собственность

{{ird_description}}

---
*Подпись:* ___________  {{contact_name}}
*Дата:* {{current_date}}
`

const reportTemplate = `# Квартальный отчёт резидента Сколково

**Организация:** {{company_name}}
**ИНН:** {{inn}}
**Период:** {{report_period}}
**Дата составления:** {{current_date}}

## 1. Общие сведения

**Статус резидента:** Действующий
**Кластер:** {{cluster}}
**Руководитель проекта:** {{project_lead}}

## 2. Выполнение плана НИОКР

### 2.1 Работы, выполненные в отчётном периоде

{{rd_completed_work}}

### 2.2 Результаты НИОКР

{{rd_results_quarter}}

### 2.3 Отклонения от плана (при наличии)

{{rd_deviations}}

## 3. Финансовые показатели

| Показатель | Факт | План | Отклонение |
|-----------|------|------|-----------|
| Выручка (руб.) | {{actual_revenue}} | {{planned_revenue}} | {{revenue_deviation}} |
| Расходы на НИОКР (руб.) | {{rd_expenses}} | {{planned_rd_expenses}} | {{rd_expenses_deviation}} |
| Привлечённые инвестиции (руб.) | {{actual_investments}} | {{planned_investments}} | — |

## 4. Использование льгот

- Налог на прибыль: применена ставка 0%
- Страховые взносы: применена ставка 14%
- НДС на НИОКР: применено освобождение

**Общая сумма налоговой экономии за период:** {{tax_savings}} руб.

## 5. Созданные объекты ИС

{{new_ird}}

## 6. Публикации и патенты

{{publications}}

## 7. Партнёрства и кооперация

{{partnerships}}

## 8. Планы на следующий квартал

{{next_quarter_plans}}

---
*Руководитель:* ___________  {{director_name}}
*Дата:* {{current_date}}
`

const extensionTemplate = `# Заявление о продлении статуса резидента проекта «Сколково»

**Дата:** {{current_date}}

Управляющей компании Фонда «Сколково»

## Просьба о продлении

Настоящим {{company_name}} (ИНН {{inn}}) в лице {{director_name}}, действующего на основании {{authority_document}}, просит продлить статус участника проекта «Сколково».

## Основания для продления

**Срок действия текущего статуса:** {{status_valid_until}}

**Выполнение обязательств за период резидентства:**

{{obligations_fulfillment}}

**Достигнутые результаты:**

{{achievements}}

**Финансовые показатели деятельности:**

| Год | Выручка (руб.) | Расходы на НИОКР (руб.) | Создано ИС |
|-----|----------------|------------------------|-----------|
| {{year_1}} | {{revenue_y1}} | {{rd_y1}} | {{ird_y1}} |
| {{year_2}} | {{revenue_y2}} | {{rd_y2}} | {{ird_y2}} |

## Планы на следующий период

{{next_period_plans}}

## Приложения

- [ ] Отчёт о деятельности за последний период
- [ ] Финансовая отчётность
- [ ] Перечень объектов ИС
- [ ] Иные документы: {{additional_docs}}

---
*Руководитель:* ___________  {{director_name}}
*{{director_position}}*
*Дата:* {{current_date}}
`

const exitNoticeTemplate = `# Уведомление о выходе из проекта «Сколково»

**Дата:** {{current_date}}

Управляющей компании Фонда «Сколково»

## Уведомление

Настоящим {{company_name}} (ИНН {{inn}}) уведомляет о намерении выйти из состава участников проекта «Сколково».

**Планируемая дата прекращения статуса:** {{exit_date}}

## Причина выхода

{{exit_reason}}

## Обязательства при выходе

**Использованные гранты (при наличии):**
{{grants_used}}

**Порядок возврата неиспользованных средств:**
{{funds_return}}

**Заключительный отчёт:**
Будет представлен до {{final_report_date}}.

## Подтверждение

Подтверждаем отсутствие задолженности перед Фондом «Сколково»: {{debt_confirmation}}

---
*Руководитель:* ___________  {{director_name}}
*{{director_position}}*
*Дата:* {{current_date}}

**Контактное лицо:** {{contact_name}}, тел. {{contact_phone}}, e-mail: {{contact_email}}
`

const irdDescriptionTemplate = `# Описание результата интеллектуальной деятельности (РИД)

**Организация:** {{company_name}}
**ИНН:** {{inn}}
**Дата:** {{current_date}}

## 1. Наименование РИД

{{ird_name}}

## 2. Вид охраняемого объекта

{{ird_type}}
*(изобретение / полезная модель / программа для ЭВМ / база данных / топология / ноу-хау)*

## 3. Описание РИД

### 3.1 Техническое описание

{{technical_description}}

### 3.2 Область применения

{{application_area}}

### 3.3 Существенные признаки, отличающие РИД от аналогов

{{distinctive_features}}

## 4. Правовая охрана

**Статус:** {{legal_status}}
*(заявка подана / патент получен / охраняется как ноу-хау)*

**Номер охранного документа (при наличии):** {{patent_number}}
**Дата приоритета:** {{priority_date}}

## 5. Связь с проектом Сколково

{{project_connection}}

## 6. Коммерческий потенциал

{{commercial_potential}}

---
*Составил:* {{contact_name}}
*Дата:* {{current_date}}
`

const genericTemplate = `# Документ

**Организация:** {{company_name}}
**ИНН:** {{inn}}
**Дата:** {{current_date}}

## Содержание

{{content}}

---
*Подпись:* ___________  {{contact_name}}
*Дата:* {{current_date}}
`
