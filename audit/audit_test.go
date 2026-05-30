package audit

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		cov  Coverage
		want Status
	}{
		{"выключен", Coverage{Enabled: false}, StatusDisabled},
		{"падает", Coverage{Enabled: true, HealthState: "failing", Items: 5}, StatusFailing},
		{"устарел", Coverage{Enabled: true, HealthState: "stale", Items: 5}, StatusStale},
		{"нет данных", Coverage{Enabled: true, HealthState: "ok", Items: 0}, StatusNoData},
		{"покрыт по health", Coverage{Enabled: true, HealthState: "ok", Items: 10}, StatusCovered},
		{"покрыт по данным без health", Coverage{Enabled: true, HealthState: "", Items: 3}, StatusCovered},
		{"неизвестно", Coverage{Enabled: true, HealthState: "", Items: -1}, StatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.cov); got != tt.want {
				t.Errorf("Classify() = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestBuildCountsCovered(t *testing.T) {
	rep := Build([]Coverage{
		{Key: "a", Enabled: true, HealthState: "ok", Items: 5},    // covered
		{Key: "b", Enabled: false},                                // disabled
		{Key: "c", Enabled: true, HealthState: "stale", Items: 1}, // stale
	})
	if rep.TotalN != 3 {
		t.Errorf("TotalN = %d, ожидалось 3", rep.TotalN)
	}
	if rep.CoveredN != 1 {
		t.Errorf("CoveredN = %d, ожидалось 1", rep.CoveredN)
	}
}
