package jobsched

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunnerRegisterAndJobs(t *testing.T) {
	r := NewRunner(nil, 2*time.Hour)
	r.Register("a", "Задание A", 0, func(ctx context.Context) (RunResult, error) { return RunResult{}, nil })
	r.Register("b", "Задание B", 30*time.Minute, func(ctx context.Context) (RunResult, error) { return RunResult{}, nil })

	jobs := r.Jobs()
	if len(jobs) != 2 {
		t.Fatalf("ожидали 2 задания, получили %d", len(jobs))
	}
	// Порядок регистрации сохраняется.
	if jobs[0].Name != "a" || jobs[1].Name != "b" {
		t.Errorf("порядок: %s, %s", jobs[0].Name, jobs[1].Name)
	}
	// interval<=0 → берётся defaultInterval раннера.
	if jobs[0].IntervalSeconds != int64((2 * time.Hour).Seconds()) {
		t.Errorf("default interval для a: %d", jobs[0].IntervalSeconds)
	}
	if jobs[1].IntervalSeconds != int64((30 * time.Minute).Seconds()) {
		t.Errorf("interval для b: %d", jobs[1].IntervalSeconds)
	}
}

func TestRunnerTriggerUnknown(t *testing.T) {
	r := NewRunner(nil, time.Hour)
	r.Register("known", "Известное", time.Hour, func(ctx context.Context) (RunResult, error) { return RunResult{}, nil })

	if err := r.Trigger("missing"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("Trigger(missing) = %v, хотим ErrUnknownJob", err)
	}
	// Известное задание — запуск ставится в очередь (буфер свободен).
	if err := r.Trigger("known"); err != nil {
		t.Errorf("Trigger(known) = %v, хотим nil", err)
	}
}

func TestRunnerTriggerAlreadyRunning(t *testing.T) {
	r := NewRunner(nil, time.Hour)
	r.Register("job", "Задание", time.Hour, func(ctx context.Context) (RunResult, error) { return RunResult{}, nil })
	// Имитируем, что задание уже выполняется.
	r.mu.Lock()
	r.running["job"] = true
	r.mu.Unlock()
	if err := r.Trigger("job"); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("Trigger(running) = %v, хотим ErrAlreadyRunning", err)
	}
}

func TestSafeRunRecoversPanic(t *testing.T) {
	_, err := safeRun(context.Background(), func(ctx context.Context) (RunResult, error) {
		panic("бум")
	})
	if err == nil {
		t.Fatal("ожидали ошибку из паники")
	}
	if got := err.Error(); got == "" {
		t.Errorf("пустой текст ошибки паники")
	}
}

func TestJobConfigInterval(t *testing.T) {
	j := JobConfig{IntervalSeconds: 3600}
	if j.Interval() != time.Hour {
		t.Errorf("Interval() = %v", j.Interval())
	}
}
