package health

import (
	"testing"
	"time"
)

func TestSourceState(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour)
	old := now.Add(-48 * time.Hour)

	tests := []struct {
		name       string
		src        Source
		staleAfter time.Duration
		want       Status
	}{
		{
			name: "никогда не запускался",
			src:  Source{},
			want: StatusUnknown,
		},
		{
			name:       "недавний успех — ok",
			src:        Source{LastSuccessAt: &recent},
			staleAfter: 24 * time.Hour,
			want:       StatusOK,
		},
		{
			name:       "давний успех — stale",
			src:        Source{LastSuccessAt: &old},
			staleAfter: 24 * time.Hour,
			want:       StatusStale,
		},
		{
			name:       "три ошибки подряд — failing",
			src:        Source{LastSuccessAt: &recent, ConsecutiveFailures: 3},
			staleAfter: 24 * time.Hour,
			want:       StatusFailing,
		},
		{
			name:       "две ошибки, но успех свежий — ok",
			src:        Source{LastSuccessAt: &recent, ConsecutiveFailures: 2},
			staleAfter: 24 * time.Hour,
			want:       StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.src.State(tt.staleAfter, now); got != tt.want {
				t.Errorf("State() = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}
