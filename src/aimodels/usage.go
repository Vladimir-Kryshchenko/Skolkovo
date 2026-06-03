package aimodels

import (
	"context"
	"sync"
	"time"
)

// UsageEvent — событие учёта расхода токенов одного агентского вызова LLM.
// Передаётся в зарегистрированный usageRecorder из ChatWithAgent.
type UsageEvent struct {
	RunID      string    // прогон планировщика (пусто — вызов вне расписания)
	TenantID   string    // тенант, инициировавший вызов (пусто — системный/глобальный)
	AgentType  AgentType // тип агента
	ModelID    string    // идентификатор модели в API (e.g. qwen-max)
	ModelLabel string    // человекочитаемое имя модели из конфигурации
	Provider   Provider  // провайдер модели
	Usage      Usage     // prompt/completion/total токены
	Duration   time.Duration
	Success    bool
	Err        string
}

// runIDKey — ключ контекста для связывания вызова LLM с прогоном планировщика.
type runIDKey struct{}

// tenantIDKey — ключ контекста для привязки вызова к тенанту (MCP, бот, виджет).
type tenantIDKey struct{}

// WithRunID кладёт идентификатор прогона планировщика в контекст. Раннер jobsched
// оборачивает им ctx задания, и все ИИ-вызовы внутри привязываются к этому прогону.
func WithRunID(ctx context.Context, runID string) context.Context {
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, runIDKey{}, runID)
}

// WithTenantID кладёт идентификатор тенанта в контекст. MCP-сервер устанавливает
// его после разрешения API-ключа; Telegram-бот — из BotConfig.TenantID.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantIDKey{}, tenantID)
}

// runIDFromContext извлекает идентификатор прогона из контекста (пусто, если нет).
func runIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(runIDKey{}).(string); ok {
		return v
	}
	return ""
}

// tenantIDFromContext извлекает идентификатор тенанта из контекста (пусто, если нет).
func tenantIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantIDKey{}).(string); ok {
		return v
	}
	return ""
}

var (
	usageMu       sync.RWMutex
	usageRecorder func(UsageEvent)
)

// SetUsageRecorder регистрирует приёмник событий расхода токенов. Вызывается один
// раз при старте serve; nil-приёмник (по умолчанию) отключает учёт. Приёмник
// должен быть быстрым/неблокирующим — он вызывается в горутине.
func SetUsageRecorder(fn func(UsageEvent)) {
	usageMu.Lock()
	usageRecorder = fn
	usageMu.Unlock()
}

// recordUsage формирует UsageEvent и асинхронно передаёт его приёмнику (best-effort:
// учёт никогда не блокирует и не роняет сам LLM-вызов).
func recordUsage(ctx context.Context, m Model, a Agent, usage Usage, dur time.Duration, callErr error) {
	usageMu.RLock()
	rec := usageRecorder
	usageMu.RUnlock()
	if rec == nil {
		return
	}
	ev := UsageEvent{
		RunID:      runIDFromContext(ctx),
		TenantID:   tenantIDFromContext(ctx),
		AgentType:  a.AgentType,
		ModelID:    m.ModelID,
		ModelLabel: m.Name,
		Provider:   m.Provider,
		Usage:      usage,
		Duration:   dur,
		Success:    callErr == nil,
	}
	if callErr != nil {
		ev.Err = callErr.Error()
	}
	go rec(ev)
}
