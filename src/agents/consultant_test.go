package agents

import (
	"context"
	"testing"

	rag "baza-skolkovo/src/rag_service"
)

func TestNewConsultantAgent(t *testing.T) {
	agent := NewConsultantAgent(nil, "http://localhost:8080", "test-key")
	if agent == nil {
		t.Fatal("ожидался не-nil агент")
	}
	if agent.mcpURL != "http://localhost:8080" {
		t.Errorf("mcpURL = %q, хотел %q", agent.mcpURL, "http://localhost:8080")
	}
	if agent.mcpAPIKey != "test-key" {
		t.Errorf("mcpAPIKey = %q, хотел %q", agent.mcpAPIKey, "test-key")
	}
}

func TestConsultantAsk_EmptyQuestion(t *testing.T) {
	agent := NewConsultantAgent(nil, "", "")
	_, err := agent.Ask(context.Background(), "", "")
	if err == nil {
		t.Fatal("ожидалась ошибка на пустой вопрос")
	}
}

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name    string
		scores  []float32
		wantMin float64
		wantMax float64
	}{
		{
			name:    "empty results",
			scores:  nil,
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "single high score",
			scores:  []float32{0.9},
			wantMin: 0.7,
			wantMax: 1.0,
		},
		{
			name:    "single low score",
			scores:  []float32{0.3},
			wantMin: 0.2,
			wantMax: 0.3,
		},
		{
			name:    "many results with bonus",
			scores:  []float32{0.8, 0.7, 0.6, 0.5, 0.4},
			wantMin: 0.75,
			wantMax: 1.0,
		},
		{
			name:    "three results small bonus",
			scores:  []float32{0.7, 0.6, 0.5},
			wantMin: 0.6,
			wantMax: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ragResults []rag.Result
			for _, s := range tt.scores {
				ragResults = append(ragResults, rag.Result{Score: s})
			}

			conf := calculateConfidence(ragResults)
			if conf < tt.wantMin || conf > tt.wantMax {
				t.Errorf("confidence = %.3f, хотел в [%.3f, %.3f]", conf, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestConsultantLogQuery_NoLogger(t *testing.T) {
	agent := NewConsultantAgent(nil, "", "")
	// Без логгера — не должно паниковать.
	err := agent.LogQuery(context.Background(), "вопрос", "ответ", "", 2, 0.8)
	if err != nil {
		t.Errorf("неожиданная ошибка: %v", err)
	}
}

func TestConsultantSetLogger(t *testing.T) {
	agent := NewConsultantAgent(nil, "", "")
	logger := &mockQueryLogger{}
	agent.SetLogger(logger)
	if agent.logger != logger {
		t.Error("логгер не установлен")
	}
}

type mockQueryLogger struct {
	entries []ConsultantQueryLog
}

func (m *mockQueryLogger) LogQuery(_ context.Context, entry ConsultantQueryLog) error {
	m.entries = append(m.entries, entry)
	return nil
}
