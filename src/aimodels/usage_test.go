package aimodels

import (
	"context"
	"testing"
	"time"
)

func TestWithRunID(t *testing.T) {
	ctx := context.Background()
	if got := runIDFromContext(ctx); got != "" {
		t.Errorf("пустой ctx должен дать пустой run_id, получили %q", got)
	}
	ctx = WithRunID(ctx, "run-42")
	if got := runIDFromContext(ctx); got != "run-42" {
		t.Errorf("runIDFromContext = %q, хотим run-42", got)
	}
	// Пустой id не оборачивает контекст.
	if got := runIDFromContext(WithRunID(context.Background(), "")); got != "" {
		t.Errorf("пустой id не должен сохраняться, получили %q", got)
	}
}

func TestRecordUsageHook(t *testing.T) {
	t.Cleanup(func() { SetUsageRecorder(nil) })

	got := make(chan UsageEvent, 1)
	SetUsageRecorder(func(ev UsageEvent) { got <- ev })

	ctx := WithRunID(context.Background(), "run-1")
	m := Model{ModelID: "qwen-max", Name: "Qwen Max", Provider: ProviderAlibabaCloud}
	a := Agent{AgentType: AgentPageAnnotator}
	recordUsage(ctx, m, a, Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}, 250*time.Millisecond, nil)

	select {
	case ev := <-got:
		if ev.RunID != "run-1" || ev.ModelID != "qwen-max" || ev.ModelLabel != "Qwen Max" {
			t.Errorf("событие: %+v", ev)
		}
		if ev.Provider != ProviderAlibabaCloud || ev.AgentType != AgentPageAnnotator {
			t.Errorf("агент/провайдер: %+v", ev)
		}
		if ev.Usage.TotalTokens != 120 || !ev.Success {
			t.Errorf("usage/success: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("recordUsage не вызвал приёмник")
	}
}

func TestRecordUsageNoRecorder(t *testing.T) {
	SetUsageRecorder(nil)
	// Не должно паниковать при отсутствии приёмника.
	recordUsage(context.Background(), Model{}, Agent{}, Usage{}, 0, nil)
}
