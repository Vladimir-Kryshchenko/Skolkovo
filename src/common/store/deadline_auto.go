// deadline_auto.go — единая логика автосоздания дедлайна квартальной отчётности
// при переходе клиента на стадию «отчётность». Один источник истины для обоих
// путей смены стадии: HTTP/JSON-API Резидентство-Админ (:8091) и MCP update_client_stage.
package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"baza-skolkovo/src/common/model"
)

// ReportingDeadlineDays — срок первой квартальной отчётности после входа в стадию «отчётность».
const ReportingDeadlineDays = 90

// DeadlineEnsurer — минимальное подмножество DeadlineStore, нужное для автосоздания
// дедлайна отчётности. *PostgresClientStore и интерфейс DeadlineStore ему удовлетворяют.
type DeadlineEnsurer interface {
	CreateDeadline(ctx context.Context, deadline *model.Deadline) error
	ListDeadlines(ctx context.Context, clientID string, daysAhead int) ([]*model.Deadline, error)
}

// EnsureReportingDeadline создаёт дедлайн квартальной отчётности при переходе клиента
// на стадию «отчётность», если у клиента ещё нет незакрытого дедлайна отчётности.
// No-op при to != «отчётность», nil-хранилище или уже существующем дедлайне.
func EnsureReportingDeadline(ctx context.Context, dc DeadlineEnsurer, clientID string, to model.ResidencyStage) {
	if to != model.StageReporting || dc == nil {
		return
	}
	// Не дублируем, если уже есть незакрытый дедлайн отчётности.
	if existing, err := dc.ListDeadlines(ctx, clientID, 0); err == nil {
		for _, d := range existing {
			if d.Type == model.DeadlineReporting && d.Status != model.DeadlineCompleted {
				return
			}
		}
	}
	_ = dc.CreateDeadline(ctx, &model.Deadline{
		ID:        uuid.New().String(),
		ClientID:  clientID,
		Title:     "Квартальный отчёт резидента",
		DueDate:   time.Now().AddDate(0, 0, ReportingDeadlineDays),
		Type:      model.DeadlineReporting,
		Status:    model.DeadlineUpcoming,
		CreatedAt: time.Now(),
	})
}
