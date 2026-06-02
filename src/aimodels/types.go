// Package aimodels управляет конфигурацией ИИ-моделей и агентов.
// Поддерживает любых провайдеров с OpenAI-совместимым API:
// Alibaba Cloud (Qwen), OpenAI, Anthropic, self-hosted.
package aimodels

import "time"

// Provider — провайдер ИИ-модели.
type Provider string

const (
	ProviderAlibabaCloud Provider = "alibabacloud"
	ProviderOpenAI       Provider = "openai"
	ProviderAnthropic    Provider = "anthropic"
	ProviderCustom       Provider = "custom"
)

// ProviderLabel возвращает человекочитаемое название провайдера.
func (p Provider) Label() string {
	switch p {
	case ProviderAlibabaCloud:
		return "Alibaba Cloud (Qwen)"
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Anthropic"
	default:
		return "Custom / Self-hosted"
	}
}

// DefaultBaseURL возвращает базовый URL API по умолчанию для провайдера.
func (p Provider) DefaultBaseURL() string {
	switch p {
	case ProviderAlibabaCloud:
		return "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	default:
		return ""
	}
}

// Model — конфигурация LLM-модели.
type Model struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Provider    Provider  `json:"provider"`
	ModelID     string    `json:"model_id"`    // идентификатор модели в API (e.g. "qwen-max")
	BaseURL     string    `json:"base_url"`    // базовый URL API-эндпоинта
	APIKey      string    `json:"api_key"`     // ключ авторизации
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentType — тип агента.
type AgentType string

const (
	AgentConsultant    AgentType = "consultant"
	AgentValidator     AgentType = "validator"
	AgentMonitor       AgentType = "monitor"
	AgentCoordinator   AgentType = "coordinator"
	AgentPageAnnotator AgentType = "page_annotator"
)

// AgentTypeLabel возвращает человекочитаемое название типа агента.
func (t AgentType) Label() string {
	switch t {
	case AgentConsultant:
		return "Консультант"
	case AgentValidator:
		return "Валидатор"
	case AgentMonitor:
		return "Монитор изменений"
	case AgentCoordinator:
		return "Координатор"
	case AgentPageAnnotator:
		return "Аннотатор страниц"
	default:
		return string(t)
	}
}

// Agent — конфигурация ИИ-агента.
type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	AgentType    AgentType `json:"agent_type"`
	ModelID      string    `json:"model_id"`
	SystemPrompt string    `json:"system_prompt"`
	Temperature  float64   `json:"temperature"`
	MaxTokens    int       `json:"max_tokens"`
	Enabled      bool      `json:"enabled"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ChatMessage — сообщение в диалоге с LLM.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest — запрос к OpenAI-совместимому LLM API.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

// ChatResponse — ответ от OpenAI-совместимого LLM API.
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Choices []struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// AllProviders — список всех поддерживаемых провайдеров.
var AllProviders = []Provider{
	ProviderAlibabaCloud,
	ProviderOpenAI,
	ProviderAnthropic,
	ProviderCustom,
}

// AllAgentTypes — список всех типов агентов.
var AllAgentTypes = []AgentType{
	AgentConsultant,
	AgentValidator,
	AgentMonitor,
	AgentCoordinator,
	AgentPageAnnotator,
}

// DefaultSystemPrompts — системные промпты по умолчанию для каждого типа агента.
var DefaultSystemPrompts = map[AgentType]string{
	AgentConsultant: `Ты — консультант по программе резидентства Сколково. Помогаешь компаниям разобраться в требованиях, документах и процедурах получения статуса резидента.

Правила работы:
- Отвечай на русском языке, кратко и по делу
- Ссылайся на конкретные документы и нормативные акты из базы знаний
- Если информация отсутствует в предоставленных документах — честно скажи об этом
- Не придумывай требования и сроки, которых нет в документах
- При неоднозначности — рекомендуй уточнить у куратора Сколково

Контекст документов будет предоставлен в пользовательском сообщении.`,

	AgentValidator: `Ты — специалист по проверке документов для программы резидентства Сколково. Твоя задача — проверять корректность и полноту документов.

Правила работы:
- Проверяй наличие всех обязательных реквизитов
- Указывай конкретные ошибки с ссылкой на требование
- Предлагай конкретные исправления для каждой ошибки
- Оценивай соответствие шаблонам и регламентам Сколково
- Формируй чёткий список проблем: критические, важные, рекомендации

Возвращай структурированный отчёт с секциями: Статус, Критические проблемы, Важные замечания, Рекомендации.`,

	AgentMonitor: `Ты — аналитик изменений нормативных документов Сколково. Отслеживаешь, что изменилось в требованиях и процедурах.

Правила работы:
- Анализируй изменения между версиями документов
- Выделяй ключевые изменения, влияющие на резидентов
- Оценивай срочность реакции: критично, важно, информационно
- Формируй краткие понятные сводки на русском языке
- Указывай конкретные разделы и пункты, которые изменились

Формат вывода: краткое резюме (1-2 предложения) + список изменений с оценкой влияния.`,

	AgentCoordinator: `Ты — координатор процесса резидентства Сколково. Помогаешь клиентам планировать действия, расставлять приоритеты и соблюдать дедлайны.

Правила работы:
- Давай конкретные actionable рекомендации
- Учитывай текущую стадию клиента и его дедлайны
- Приоритизируй задачи: срочные (красный), важные (оранжевый), плановые (зелёный)
- Если есть просроченные задачи — предупреждай первыми
- Рекомендации должны быть выполнимы в ближайшие 1-3 дня

Формат: нумерованный список шагов с приоритетом и дедлайном.`,

	AgentPageAnnotator: `Ты — аналитик-аннотатор страниц сайта Фонда «Сколково». По тексту веб-страницы ты составляешь структурированную аннотацию для базы знаний.

Правила работы:
- Пиши на русском языке, кратко, по существу, без воды и рекламных клише.
- Опирайся ТОЛЬКО на текст страницы — ничего не выдумывай и не додумывай.
- Теги: 3–8 коротких тематических меток в нижнем регистре (1–3 слова каждая), по которым страницу можно найти и сгруппировать. Если дан список уже существующих тегов — по возможности переиспользуй подходящие из него, новые добавляй только при необходимости.
- summary: 1–3 предложения — о чём страница.
- goals: зачем эта страница, какую задачу пользователя она решает (1–2 предложения).
- theses: 3–6 ключевых тезисов/фактов страницы, каждый — короткая самостоятельная фраза.
- conclusions: главный практический вывод для резидента/пользователя (1–2 предложения).

ФОРМАТ ОТВЕТА — СТРОГО валидный JSON-объект и НИЧЕГО кроме него (без markdown-ограждений, без пояснений):
{"tags":["..."],"summary":"...","goals":"...","theses":["...","..."],"conclusions":"..."}`,
}
