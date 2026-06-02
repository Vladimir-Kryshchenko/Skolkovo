package jobsched

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"baza-skolkovo/src/aimodels"
)

// JobFunc — выполнение одного задания. Возвращает счётчики и (опц.) разбивку.
type JobFunc func(ctx context.Context) (RunResult, error)

// ErrAlreadyRunning возвращается Trigger, если задание уже выполняется.
var ErrAlreadyRunning = errors.New("задание уже выполняется")

// ErrUnknownJob возвращается Trigger для незарегистрированного задания.
var ErrUnknownJob = errors.New("задание не найдено")

// tickInterval — частота опроса БД на предмет «пора запускать».
const tickInterval = 30 * time.Second

type job struct {
	name            string
	title           string
	defaultInterval time.Duration
	fn              JobFunc
}

// Runner оркеструет периодический запуск зарегистрированных заданий: читает
// расписание из БД, запускает просроченные задания последовательно, ведёт журнал
// прогонов и привязывает ИИ-вызовы к прогону через контекст. Все прогоны (и по
// расписанию, и ручные) исполняются в одной горутине Run — пересечение исключено.
type Runner struct {
	store           *Store
	defaultInterval time.Duration

	mu      sync.Mutex
	jobs    []*job
	jobMap  map[string]*job
	running map[string]bool

	trigger chan string
}

// NewRunner создаёт раннер. defaultInterval — интервал по умолчанию для новых
// заданий (обычно cfg.ScrapeInterval).
func NewRunner(store *Store, defaultInterval time.Duration) *Runner {
	if defaultInterval <= 0 {
		defaultInterval = 6 * time.Hour
	}
	return &Runner{
		store:           store,
		defaultInterval: defaultInterval,
		jobMap:          make(map[string]*job),
		running:         make(map[string]bool),
		trigger:         make(chan string, 32),
	}
}

// Register добавляет задание. interval<=0 — взять defaultInterval раннера.
// Регистрировать нужно до запуска Run.
func (r *Runner) Register(name, title string, interval time.Duration, fn JobFunc) {
	if interval <= 0 {
		interval = r.defaultInterval
	}
	j := &job{name: name, title: title, defaultInterval: interval, fn: fn}
	r.mu.Lock()
	r.jobs = append(r.jobs, j)
	r.jobMap[name] = j
	r.mu.Unlock()
}

// Jobs возвращает имена и названия зарегистрированных заданий (порядок регистрации).
func (r *Runner) Jobs() []JobConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobConfig, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, JobConfig{Name: j.name, Title: j.title,
			IntervalSeconds: int64(j.defaultInterval.Seconds())})
	}
	return out
}

// Trigger ставит задание на внеочередной запуск (кнопка «Запустить сейчас»).
// Возвращает ErrUnknownJob/ErrAlreadyRunning либо nil, если запуск поставлен.
func (r *Runner) Trigger(name string) error {
	r.mu.Lock()
	_, ok := r.jobMap[name]
	inflight := r.running[name]
	r.mu.Unlock()
	if !ok {
		return ErrUnknownJob
	}
	if inflight {
		return ErrAlreadyRunning
	}
	select {
	case r.trigger <- name:
		return nil
	default:
		return errors.New("очередь запусков переполнена, попробуйте позже")
	}
}

// Run — главный цикл планировщика. Блокирующий; запускать в горутине.
func (r *Runner) Run(ctx context.Context) {
	// Регистрируем дефолтные конфиги заданий (без перезаписи настроек админа).
	for _, j := range r.snapshotJobs() {
		if err := r.store.UpsertJobDefault(ctx, j.name, j.title, int64(j.defaultInterval.Seconds())); err != nil {
			log.Printf("[jobsched] регистрация задания %s: %v", j.name, err)
		}
	}
	log.Printf("[jobsched] планировщик запущен, заданий: %d", len(r.snapshotJobs()))

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	r.scan(ctx) // первый прогон просроченных/новых заданий сразу при старте
	for {
		select {
		case <-ctx.Done():
			log.Println("[jobsched] планировщик остановлен")
			return
		case <-ticker.C:
			r.scan(ctx)
		case name := <-r.trigger:
			r.runManual(ctx, name)
		}
	}
}

func (r *Runner) snapshotJobs() []*job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*job(nil), r.jobs...)
}

// scan запускает все включённые задания, у которых наступило время (next_run_at).
func (r *Runner) scan(ctx context.Context) {
	configs, err := r.store.ListJobs(ctx)
	if err != nil {
		log.Printf("[jobsched] чтение расписания: %v", err)
		return
	}
	byName := make(map[string]JobConfig, len(configs))
	for _, c := range configs {
		byName[c.Name] = c
	}
	now := time.Now()
	for _, j := range r.snapshotJobs() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		cfg, ok := byName[j.name]
		if !ok || !cfg.Enabled {
			continue
		}
		if cfg.NextRunAt != nil && cfg.NextRunAt.After(now) {
			continue
		}
		trig := TriggerSchedule
		if cfg.NextRunAt == nil {
			trig = TriggerStartup
		}
		r.execute(ctx, j, trig, cfg.Interval())
		now = time.Now() // задание могло выполняться долго
	}
}

// runManual обрабатывает внеочередной запуск задания по имени.
func (r *Runner) runManual(ctx context.Context, name string) {
	r.mu.Lock()
	j, ok := r.jobMap[name]
	r.mu.Unlock()
	if !ok {
		return
	}
	interval := j.defaultInterval
	if cfg, err := r.store.GetJob(ctx, name); err == nil {
		interval = cfg.Interval()
	}
	r.execute(ctx, j, TriggerManual, interval)
}

// execute выполняет одно задание: журналирует прогон, привязывает ИИ-вызовы к нему
// через контекст и обновляет состояние расписания (next_run_at).
func (r *Runner) execute(ctx context.Context, j *job, trig Trigger, interval time.Duration) {
	r.mu.Lock()
	if r.running[j.name] {
		r.mu.Unlock()
		return
	}
	r.running[j.name] = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, j.name)
		r.mu.Unlock()
	}()

	start := time.Now()
	runID, err := r.store.StartRun(ctx, j.name, trig)
	if err != nil {
		log.Printf("[jobsched] %s: не удалось открыть прогон: %v", j.name, err)
		return
	}

	// run_id в контексте — чтобы ИИ-вызовы внутри задания (аннотатор, монитор)
	// привязались к этому прогону в ai_usage_log.
	jobCtx := aimodels.WithRunID(ctx, runID)

	res, runErr := safeRun(jobCtx, j.fn)
	dur := time.Since(start)

	status := StatusSuccess
	errStr := ""
	if runErr != nil {
		status = StatusError
		errStr = runErr.Error()
		log.Printf("[jobsched] %s: ошибка прогона (%s): %v", j.name, dur.Round(time.Millisecond), runErr)
	} else {
		log.Printf("[jobsched] %s: готово за %s (новых %d, обновлено %d, всего %d)",
			j.name, dur.Round(time.Millisecond), res.ItemsNew, res.ItemsUpdated, res.ItemsTotal)
	}

	if err := r.store.FinishRun(ctx, runID, status, res, errStr, dur); err != nil {
		log.Printf("[jobsched] %s: запись итога прогона: %v", j.name, err)
	}
	next := time.Now().Add(interval)
	if err := r.store.SetJobRunState(ctx, j.name, start, next, string(status), errStr); err != nil {
		log.Printf("[jobsched] %s: обновление расписания: %v", j.name, err)
	}
}

// safeRun выполняет JobFunc, превращая панику в ошибку (один сбойный источник не
// должен ронять весь планировщик).
func safeRun(ctx context.Context, fn JobFunc) (res RunResult, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errPanic(p)
		}
	}()
	return fn(ctx)
}

func errPanic(p any) error {
	return errors.New("паника в задании: " + sprint(p))
}

func sprint(p any) string {
	if e, ok := p.(error); ok {
		return e.Error()
	}
	if s, ok := p.(string); ok {
		return s
	}
	return "неизвестная ошибка"
}
