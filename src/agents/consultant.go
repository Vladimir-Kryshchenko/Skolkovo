package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"baza-skolkovo/src/aimodels"
	rag "baza-skolkovo/src/rag_service"
)

// LLMProvider отдаёт конфигурацию агентов и моделей (реализуется *aimodels.Store).
// Может быть nil — тогда консультант формирует ответ из RAG-фрагментов без LLM.
type LLMProvider interface {
	ListAgents(ctx context.Context) ([]aimodels.Agent, error)
	GetModel(ctx context.Context, id string) (aimodels.Model, error)
}

// chatFunc — вызов LLM (по умолчанию aimodels.ChatWithAgent; переопределяется в тестах).
type chatFunc func(ctx context.Context, m aimodels.Model, a aimodels.Agent, userMessage string) (string, int, error)

// ConsultantResponse — ответ агента-консультанта.
type ConsultantResponse struct {
	// Answer — сформированный ответ на вопрос.
	Answer string `json:"answer"`
	// Sources — ссылки на документы, из которых извлечён ответ.
	Sources []DocumentReference `json:"sources"`
	// Confidence — уверенность ответа от 0 до 1.
	Confidence float64 `json:"confidence"`
}

// DocumentReference — ссылка на фрагмент документа.
type DocumentReference struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	SourceURL  string  `json:"source_url"`
	Score      float32 `json:"score"`
}

// ConsultantQueryLog — запись лога запроса к консультанту.
type ConsultantQueryLog struct {
	ID         string    `json:"id"`
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	ClientID   string    `json:"client_id,omitempty"`
	Sources    int       `json:"sources_count"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
}

// QueryLogger — интерфейс для логирования запросов консультанта.
type QueryLogger interface {
	LogQuery(ctx context.Context, entry ConsultantQueryLog) error
}

// ConsultantAgent — агент-консультант, отвечающий на вопросы по базе документов.
type ConsultantAgent struct {
	ragService *rag.Service
	mcpURL     string
	mcpAPIKey  string
	logger     QueryLogger
	ai         LLMProvider // опционально: LLM-синтез ответа
	chat       chatFunc
}

// NewConsultantAgent создаёт агента-консультанта.
func NewConsultantAgent(ragSvc *rag.Service, mcpURL, mcpAPIKey string) *ConsultantAgent {
	return &ConsultantAgent{
		ragService: ragSvc,
		mcpURL:     mcpURL,
		mcpAPIKey:  mcpAPIKey,
		chat:       aimodels.ChatWithAgent,
	}
}

// WithLLM включает синтез ответа языковой моделью (агент типа Consultant).
// Без него консультант возвращает найденные RAG-фрагменты как есть.
func (a *ConsultantAgent) WithLLM(ai LLMProvider) *ConsultantAgent {
	a.ai = ai
	return a
}

// SetLogger устанавливает логгер для запросов консультанта.
func (a *ConsultantAgent) SetLogger(logger QueryLogger) {
	a.logger = logger
}

// Ask отвечает на вопрос, используя RAG-поиск по нормативным документам.
//
// Цепочка обработки:
//  1. RAG-поиск (top 5 результатов).
//  2. Формирование ответа из найденных чанков (MVP: без LLM).
//  3. Добавление ссылок на источники.
//
// Параметры:
//   - ctx — контекст с возможностью отмены.
//   - question — текст вопроса.
//   - clientID — опционально, идентификатор клиента для контекста (может быть пустым).
func (a *ConsultantAgent) Ask(ctx context.Context, question, clientID string) (ConsultantResponse, error) {
	if strings.TrimSpace(question) == "" {
		return ConsultantResponse{}, fmt.Errorf("вопрос не может быть пустым")
	}

	// Шаг 1: RAG-поиск (top 5).
	results, err := a.ragService.Search(ctx, question, 5)
	if err != nil {
		return ConsultantResponse{}, fmt.Errorf("RAG-поиск: %w", err)
	}

	if len(results) == 0 {
		resp := ConsultantResponse{
			Answer:     "К сожалению, в базе документов не найдено информации по вашему вопросу. Попробуйте переформулировать запрос или обратитесь к специалисту.",
			Sources:    nil,
			Confidence: 0,
		}
		a.logQuery(ctx, question, resp, clientID)
		return resp, nil
	}

	// Шаг 2: Синтез ответа. Если подключён LLM — генерируем связный ответ с
	// опорой на найденные фрагменты; иначе отдаём сами фрагменты.
	answer, usedLLM := a.synthesize(ctx, question, results)
	if !usedLLM {
		var parts []string
		for i, r := range results {
			parts = append(parts, fmt.Sprintf("[%d] %s\n%s", i+1, r.Title, r.Text))
		}
		answer = fmt.Sprintf("По вашему запросу найдено %d релевантных фрагментов:\n\n%s",
			len(results), strings.Join(parts, "\n\n---\n\n"))
	}

	// Шаг 3: Ссылки на источники.
	sources := make([]DocumentReference, 0, len(results))
	for _, r := range results {
		sources = append(sources, DocumentReference{
			DocumentID: r.DocumentID,
			Title:      r.Title,
			SourceURL:  r.SourceURL,
			Score:      r.Score,
		})
	}

	// Расчёт уверенности на основе скорреляции результатов.
	confidence := calculateConfidence(results)

	resp := ConsultantResponse{
		Answer:     answer,
		Sources:    sources,
		Confidence: confidence,
	}

	a.logQuery(ctx, question, resp, clientID)
	return resp, nil
}

// synthesize формирует связный ответ языковой моделью на основе найденных
// фрагментов. Возвращает ok=false, если LLM не настроен или вызов не удался —
// тогда вызывающий код отдаёт сами фрагменты.
func (a *ConsultantAgent) synthesize(ctx context.Context, question string, results []rag.Result) (string, bool) {
	if a.ai == nil {
		return "", false
	}
	mdl, agent, ok := a.consultantModel(ctx)
	if !ok {
		return "", false
	}
	chat := a.chat
	if chat == nil {
		chat = aimodels.ChatWithAgent
	}

	var b strings.Builder
	b.WriteString("Вопрос пользователя: ")
	b.WriteString(question)
	b.WriteString("\n\nФрагменты из базы знаний Сколково (используй только их, ссылайся на номер источника [N]):\n\n")
	for i, r := range results {
		fmt.Fprintf(&b, "[%d] %s\n%s\n\n", i+1, r.Title, r.Text)
	}
	b.WriteString("Дай краткий точный ответ на русском со ссылками на источники [N]. " +
		"Если фрагменты не содержат ответа — честно сообщи об этом.")

	answer, _, err := chat(ctx, mdl, agent, b.String())
	if err != nil || strings.TrimSpace(answer) == "" {
		return "", false
	}
	return strings.TrimSpace(answer), true
}

// consultantModel находит включённого агента-консультанта и его включённую модель.
func (a *ConsultantAgent) consultantModel(ctx context.Context) (aimodels.Model, aimodels.Agent, bool) {
	agents, err := a.ai.ListAgents(ctx)
	if err != nil {
		return aimodels.Model{}, aimodels.Agent{}, false
	}
	for _, ag := range agents {
		if ag.AgentType != aimodels.AgentConsultant || !ag.Enabled || ag.ModelID == "" {
			continue
		}
		mdl, err := a.ai.GetModel(ctx, ag.ModelID)
		if err != nil || !mdl.Enabled || strings.TrimSpace(mdl.APIKey) == "" {
			continue
		}
		return mdl, ag, true
	}
	return aimodels.Model{}, aimodels.Agent{}, false
}

// LogQuery вручную логирует запрос к консультанту.
// Метод Ask вызывает логирование автоматически, но этот метод полезен
// для внешних вызовов или переопределения логики логирования.
func (a *ConsultantAgent) LogQuery(ctx context.Context, question, answer string, clientID string, sources int, confidence float64) error {
	return a.logEntry(ctx, ConsultantQueryLog{
		Question:   question,
		Answer:     answer,
		ClientID:   clientID,
		Sources:    sources,
		Confidence: confidence,
		Timestamp:  time.Now(),
	})
}

// logQuery вызывает логирование, если установлен логгер.
func (a *ConsultantAgent) logQuery(ctx context.Context, question string, resp ConsultantResponse, clientID string) {
	if a.logger == nil {
		return
	}
	entry := ConsultantQueryLog{
		Question:   question,
		Answer:     resp.Answer,
		ClientID:   clientID,
		Sources:    len(resp.Sources),
		Confidence: resp.Confidence,
		Timestamp:  time.Now(),
	}
	// Логирование в фоне, чтобы не блокировать ответ.
	go func() {
		_ = a.logger.LogQuery(context.Background(), entry)
	}()
}

func (a *ConsultantAgent) logEntry(ctx context.Context, entry ConsultantQueryLog) error {
	if a.logger == nil {
		return nil
	}
	return a.logger.LogQuery(ctx, entry)
}

// calculateConfidence вычисляет уверенность ответа на основе скорреляции результатов.
// Если top-результат имеет высокий score и результаты согласованы — уверенность высокая.
func calculateConfidence(results []rag.Result) float64 {
	if len(results) == 0 {
		return 0
	}

	topScore := float64(results[0].Score)

	// Нормализуем score в диапазон [0, 1].
	// Qdrant cosine similarity обычно в диапазоне [-1, 1], но для релевантных документов > 0.
	if topScore > 1 {
		topScore = 1
	}
	if topScore < 0 {
		topScore = 0
	}

	// Учитываем количество результатов: больше результатов — чуть выше уверенность.
	countBonus := 0.0
	if len(results) >= 3 {
		countBonus = 0.1
	}
	if len(results) >= 5 {
		countBonus = 0.15
	}

	confidence := topScore*0.8 + countBonus
	if confidence > 1 {
		confidence = 1
	}
	return confidence
}
