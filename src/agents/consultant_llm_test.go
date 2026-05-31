package agents

import (
	"context"
	"testing"

	"baza-skolkovo/src/aimodels"
	rag "baza-skolkovo/src/rag_service"
)

type stubLLM struct {
	agentType aimodels.AgentType
	enabled   bool
	apiKey    string
}

func (s stubLLM) ListAgents(_ context.Context) ([]aimodels.Agent, error) {
	return []aimodels.Agent{{AgentType: s.agentType, Enabled: s.enabled, ModelID: "m1"}}, nil
}
func (s stubLLM) GetModel(_ context.Context, _ string) (aimodels.Model, error) {
	return aimodels.Model{ID: "m1", Enabled: true, APIKey: s.apiKey, ModelID: "test"}, nil
}

func TestSynthesizeUsesLLM(t *testing.T) {
	a := NewConsultantAgent(nil, "", "").
		WithLLM(stubLLM{agentType: aimodels.AgentConsultant, enabled: true, apiKey: "key"})
	a.chat = func(_ context.Context, _ aimodels.Model, _ aimodels.Agent, msg string) (string, int, error) {
		return "Краткий ответ со ссылкой [1].", 5, nil
	}
	results := []rag.Result{{Title: "Документ", Text: "текст", DocumentID: "d1"}}
	ans, ok := a.synthesize(context.Background(), "вопрос?", results, nil)
	if !ok || ans != "Краткий ответ со ссылкой [1]." {
		t.Errorf("synthesize = %q, %v", ans, ok)
	}
}

func TestSynthesizeFallbackNoLLM(t *testing.T) {
	a := NewConsultantAgent(nil, "", "") // без WithLLM
	if _, ok := a.synthesize(context.Background(), "q", []rag.Result{{Title: "t"}}, nil); ok {
		t.Error("без LLM synthesize должен вернуть ok=false")
	}
}

func TestSynthesizeFallbackOnChatError(t *testing.T) {
	a := NewConsultantAgent(nil, "", "").
		WithLLM(stubLLM{agentType: aimodels.AgentConsultant, enabled: true, apiKey: "key"})
	a.chat = func(_ context.Context, _ aimodels.Model, _ aimodels.Agent, _ string) (string, int, error) {
		return "", 0, context.DeadlineExceeded
	}
	if _, ok := a.synthesize(context.Background(), "q", []rag.Result{{Title: "t"}}, nil); ok {
		t.Error("при ошибке LLM synthesize должен вернуть ok=false (фоллбэк)")
	}
}

func TestSynthesizeSkipsWrongAgentType(t *testing.T) {
	a := NewConsultantAgent(nil, "", "").
		WithLLM(stubLLM{agentType: aimodels.AgentMonitor, enabled: true, apiKey: "key"})
	a.chat = func(_ context.Context, _ aimodels.Model, _ aimodels.Agent, _ string) (string, int, error) {
		return "не должно вызваться", 0, nil
	}
	if _, ok := a.synthesize(context.Background(), "q", []rag.Result{{Title: "t"}}, nil); ok {
		t.Error("без агента типа Consultant synthesize должен вернуть ok=false")
	}
}
