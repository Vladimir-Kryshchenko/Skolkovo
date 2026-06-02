package admin

// scheduler_admin.go — раздел «Расписание и журнал» главной админки (:8090, /jobs):
//   - настройка периодичности проверки изменений по каждому источнику (часы/дни);
//   - журнал запусков с результатом и ошибками каждого прогона;
//   - учёт расхода токенов ИИ-агентов по моделям и агентам.
//
// Данные берутся из пакета jobsched (таблицы scheduler_jobs / scheduler_runs /
// ai_usage_log). Ручной запуск задания идёт через jobRunner.Trigger.

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/jobsched"
)

// WithJobStore подключает хранилище планировщика (расписание, журнал, токены ИИ).
func (s *Server) WithJobStore(js *jobsched.Store) *Server {
	s.jobStore = js
	return s
}

// WithJobRunner подключает движок планировщика (для кнопки «Запустить сейчас»).
func (s *Server) WithJobRunner(r *jobsched.Runner) *Server {
	s.jobRunner = r
	return s
}

// ─── вспомогательное ─────────────────────────────────────────────────────────

// jobRow — строка таблицы расписания (конфиг + разложенный на число+единицу интервал).
type jobRow struct {
	jobsched.JobConfig
	IntervalValue int64
	IntervalUnit  string // minutes | hours | days
}

// splitInterval раскладывает интервал в секундах на (значение, единицу) — берёт
// наибольшую единицу, на которую делится без остатка (дни → часы → минуты).
func splitInterval(seconds int64) (int64, string) {
	if seconds <= 0 {
		return 6, "hours"
	}
	if seconds%86400 == 0 {
		return seconds / 86400, "days"
	}
	if seconds%3600 == 0 {
		return seconds / 3600, "hours"
	}
	if seconds%60 == 0 {
		return seconds / 60, "minutes"
	}
	return seconds, "seconds"
}

// intervalToSeconds собирает интервал из числа и единицы.
func intervalToSeconds(value int64, unit string) int64 {
	switch unit {
	case "days":
		return value * 86400
	case "hours":
		return value * 3600
	case "minutes":
		return value * 60
	case "seconds":
		return value
	default:
		return value * 3600
	}
}

// jobsFuncs — функции шаблонов раздела.
var jobsFuncs = template.FuncMap{
	"fmtTime": func(t *time.Time) string {
		if t == nil || t.IsZero() {
			return "—"
		}
		return t.Local().Format("02.01.2006 15:04:05")
	},
	"fmtTimeV": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Local().Format("02.01.2006 15:04:05")
	},
	"fmtDurMs":      fmtDurMs,
	"intervalHuman": intervalHuman,
	"statusClass":   statusClass,
	"statusLabel":   statusLabel,
	"triggerLabel":  triggerLabel,
	"agentLabel": func(t string) string {
		if t == "" {
			return "—"
		}
		return aimodels.AgentType(t).Label()
	},
	"providerLabel": func(p string) string {
		if p == "" {
			return "—"
		}
		return aimodels.Provider(p).Label()
	},
	"shortID": func(id string) string {
		if len(id) > 8 {
			return id[:8]
		}
		return id
	},
	"orDash": func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "—"
		}
		return s
	},
}

// fmtDurMs форматирует длительность из миллисекунд по-человечески.
func fmtDurMs(ms int64) string {
	if ms <= 0 {
		return "0 мс"
	}
	if ms < 1000 {
		return strconv.FormatInt(ms, 10) + " мс"
	}
	sec := ms / 1000
	if sec < 60 {
		return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + " с"
	}
	m := sec / 60
	s := sec % 60
	if m < 60 {
		return strconv.FormatInt(m, 10) + " мин " + strconv.FormatInt(s, 10) + " с"
	}
	h := m / 60
	m = m % 60
	return strconv.FormatInt(h, 10) + " ч " + strconv.FormatInt(m, 10) + " мин"
}

// intervalHuman форматирует интервал из секунд по-человечески.
func intervalHuman(seconds int64) string {
	v, unit := splitInterval(seconds)
	switch unit {
	case "days":
		return strconv.FormatInt(v, 10) + " дн"
	case "hours":
		return strconv.FormatInt(v, 10) + " ч"
	case "minutes":
		return strconv.FormatInt(v, 10) + " мин"
	default:
		return strconv.FormatInt(seconds, 10) + " с"
	}
}

func statusClass(st string) string {
	switch jobsched.RunStatus(st) {
	case jobsched.StatusSuccess:
		return "ok"
	case jobsched.StatusError:
		return "err"
	case jobsched.StatusPartial:
		return "warn"
	case jobsched.StatusRunning:
		return "run"
	default:
		return "muted"
	}
}

func statusLabel(st string) string {
	switch jobsched.RunStatus(st) {
	case jobsched.StatusSuccess:
		return "Успех"
	case jobsched.StatusError:
		return "Ошибка"
	case jobsched.StatusPartial:
		return "Частично"
	case jobsched.StatusRunning:
		return "Выполняется"
	case jobsched.StatusSkipped:
		return "Пропущено"
	default:
		return st
	}
}

func triggerLabel(t string) string {
	switch jobsched.Trigger(t) {
	case jobsched.TriggerManual:
		return "вручную"
	case jobsched.TriggerStartup:
		return "при старте"
	default:
		return "по расписанию"
	}
}

// flashFromQuery читает flash-сообщение из query (?msg=&kind=ok|err).
func flashFromQuery(r *http.Request) (string, string) {
	msg := r.URL.Query().Get("msg")
	if msg == "" {
		return "", ""
	}
	class := "flash-ok"
	if r.URL.Query().Get("kind") == "err" {
		class = "flash-err"
	}
	return msg, class
}

func jobsRedirect(w http.ResponseWriter, r *http.Request, path, msg, kind string) {
	http.Redirect(w, r, path+"?msg="+url.QueryEscape(msg)+"&kind="+kind, http.StatusSeeOther)
}

// ─── данные страниц ──────────────────────────────────────────────────────────

type jobsPageData struct {
	Active     string
	Flash      string
	FlashClass string
	Available  bool
	CanRun     bool
	Jobs       []jobRow
}

type runsPageData struct {
	Active     string
	Flash      string
	FlashClass string
	Available  bool
	Runs       []jobsched.Run
	JobNames   []string
	JobFilter  string
	StatusFilt string
}

type runDetailData struct {
	Active     string
	Flash      string
	FlashClass string
	Run        jobsched.Run
	Usage      []jobsched.AIUsage
}

type aiUsageData struct {
	Active     string
	Flash      string
	FlashClass string
	Available  bool
	SinceDays  int
	ByModel    []jobsched.UsageAgg
	ByAgent    []jobsched.UsageAgg
	Recent     []jobsched.AIUsage
}

// ─── обработчики ─────────────────────────────────────────────────────────────

func (s *Server) handleJobsPage(w http.ResponseWriter, r *http.Request) {
	data := jobsPageData{Active: "jobs", CanRun: s.jobRunner != nil}
	data.Flash, data.FlashClass = flashFromQuery(r)
	if s.jobStore != nil {
		data.Available = true
		jobs, err := s.jobStore.ListJobs(r.Context())
		if err != nil {
			log.Printf("[admin] /jobs список: %v", err)
		}
		for _, j := range jobs {
			v, u := splitInterval(j.IntervalSeconds)
			data.Jobs = append(data.Jobs, jobRow{JobConfig: j, IntervalValue: v, IntervalUnit: u})
		}
	}
	renderJobs(w, jobsPageTmpl, data)
}

func (s *Server) handleJobUpdate(w http.ResponseWriter, r *http.Request) {
	if s.jobStore == nil {
		jobsRedirect(w, r, "/jobs", "Планировщик недоступен (требуется Postgres)", "err")
		return
	}
	name := r.PathValue("name")
	if err := r.ParseForm(); err != nil {
		jobsRedirect(w, r, "/jobs", "Некорректная форма", "err")
		return
	}
	value, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("interval_value")), 10, 64)
	if value <= 0 {
		jobsRedirect(w, r, "/jobs", "Интервал должен быть больше нуля", "err")
		return
	}
	unit := r.FormValue("interval_unit")
	seconds := intervalToSeconds(value, unit)
	if seconds < 60 {
		seconds = 60 // не чаще раза в минуту
	}
	enabled := r.FormValue("enabled") == "on"
	if err := s.jobStore.UpdateJobConfig(r.Context(), name, seconds, enabled); err != nil {
		jobsRedirect(w, r, "/jobs", "Не удалось сохранить: "+err.Error(), "err")
		return
	}
	jobsRedirect(w, r, "/jobs", "Расписание обновлено: "+name, "ok")
}

func (s *Server) handleJobRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.jobRunner == nil {
		jobsRedirect(w, r, "/jobs", "Ручной запуск недоступен", "err")
		return
	}
	switch err := s.jobRunner.Trigger(name); err {
	case nil:
		jobsRedirect(w, r, "/jobs", "Запуск поставлен в очередь: "+name, "ok")
	case jobsched.ErrAlreadyRunning:
		jobsRedirect(w, r, "/jobs", "Задание уже выполняется: "+name, "err")
	default:
		jobsRedirect(w, r, "/jobs", "Не удалось запустить: "+err.Error(), "err")
	}
}

func (s *Server) handleRunsPage(w http.ResponseWriter, r *http.Request) {
	data := runsPageData{Active: "runs"}
	data.Flash, data.FlashClass = flashFromQuery(r)
	data.JobFilter = r.URL.Query().Get("job")
	data.StatusFilt = r.URL.Query().Get("status")
	if s.jobStore != nil {
		data.Available = true
		runs, err := s.jobStore.ListRuns(r.Context(), jobsched.RunFilter{
			JobName: data.JobFilter,
			Status:  data.StatusFilt,
			Limit:   200,
		})
		if err != nil {
			log.Printf("[admin] /jobs/runs: %v", err)
		}
		data.Runs = runs
		if jobs, err := s.jobStore.ListJobs(r.Context()); err == nil {
			for _, j := range jobs {
				data.JobNames = append(data.JobNames, j.Name)
			}
		}
	}
	renderJobs(w, runsPageTmpl, data)
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	if s.jobStore == nil {
		http.Error(w, "планировщик недоступен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	run, err := s.jobStore.GetRun(r.Context(), id)
	if err != nil {
		http.Error(w, "прогон не найден", http.StatusNotFound)
		return
	}
	usage, _ := s.jobStore.ListUsage(r.Context(), jobsched.AIUsageFilter{RunID: id, Limit: 200})
	renderJobs(w, runDetailTmpl, runDetailData{Active: "runs", Run: run, Usage: usage})
}

func (s *Server) handleAIUsagePage(w http.ResponseWriter, r *http.Request) {
	data := aiUsageData{Active: "ai-usage", SinceDays: 30}
	if d, err := strconv.Atoi(r.URL.Query().Get("since_days")); err == nil && d > 0 {
		data.SinceDays = d
	}
	if s.jobStore != nil {
		data.Available = true
		data.ByModel, _ = s.jobStore.UsageByModel(r.Context(), data.SinceDays)
		data.ByAgent, _ = s.jobStore.UsageByAgent(r.Context(), data.SinceDays)
		data.Recent, _ = s.jobStore.ListUsage(r.Context(), jobsched.AIUsageFilter{SinceDays: data.SinceDays, Limit: 100})
	}
	renderJobs(w, aiUsageTmpl, data)
}

func renderJobs(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[admin] jobs template: %v", err)
	}
}
