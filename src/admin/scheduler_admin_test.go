package admin

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"baza-skolkovo/src/jobsched"
)

// TestSplitIntervalRoundTrip — разложение интервала в (число, единицу) и сборка обратно.
func TestSplitIntervalRoundTrip(t *testing.T) {
	cases := []struct {
		seconds  int64
		wantVal  int64
		wantUnit string
	}{
		{6 * 3600, 6, "hours"},
		{30 * 60, 30, "minutes"},
		{2 * 86400, 2, "days"},
		{90 * 60, 90, "minutes"}, // 1.5 ч → минуты, т.к. на часы нацело не делится
		{86400, 1, "days"},
		{0, 6, "hours"}, // защита от нуля
	}
	for _, c := range cases {
		v, u := splitInterval(c.seconds)
		if v != c.wantVal || u != c.wantUnit {
			t.Errorf("splitInterval(%d) = (%d,%s), хотим (%d,%s)", c.seconds, v, u, c.wantVal, c.wantUnit)
		}
		if c.seconds > 0 {
			if got := intervalToSeconds(v, u); got != c.seconds {
				t.Errorf("round-trip %d: получили %d", c.seconds, got)
			}
		}
	}
}

func TestFmtDurMs(t *testing.T) {
	cases := map[int64]string{
		0:       "0 мс",
		350:     "350 мс",
		1500:    "1.5 с",
		65000:   "1 мин 5 с",
		3725000: "1 ч 2 мин",
	}
	for ms, want := range cases {
		if got := fmtDurMs(ms); got != want {
			t.Errorf("fmtDurMs(%d) = %q, хотим %q", ms, got, want)
		}
	}
}

func TestIntervalHuman(t *testing.T) {
	if got := intervalHuman(6 * 3600); got != "6 ч" {
		t.Errorf("intervalHuman(6ч) = %q", got)
	}
	if got := intervalHuman(2 * 86400); got != "2 дн" {
		t.Errorf("intervalHuman(2дн) = %q", got)
	}
}

func TestStatusLabelAndClass(t *testing.T) {
	if statusLabel("success") != "Успех" || statusClass("success") != "ok" {
		t.Errorf("success: %q/%q", statusLabel("success"), statusClass("success"))
	}
	if statusClass("error") != "err" || statusClass("skipped") != "muted" {
		t.Errorf("class error/skipped: %q/%q", statusClass("error"), statusClass("skipped"))
	}
}

// TestJobsTemplatesRender — все четыре шаблона раздела исполняются без ошибок и
// содержат ключевые элементы.
func TestJobsTemplatesRender(t *testing.T) {
	now := time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)
	next := now.Add(6 * time.Hour)

	// Страница расписания.
	jobs := jobsPageData{Active: "jobs", Available: true, CanRun: true,
		Jobs: []jobRow{{
			JobConfig: jobsched.JobConfig{Name: "sitepages", Title: "Страницы сайта",
				IntervalSeconds: 6 * 3600, Enabled: true, LastRunAt: &now, NextRunAt: &next,
				LastStatus: "success"},
			IntervalValue: 6, IntervalUnit: "hours",
		}},
	}
	var b1 bytes.Buffer
	if err := jobsPageTmpl.ExecuteTemplate(&b1, "layout", jobs); err != nil {
		t.Fatalf("jobsPageTmpl: %v", err)
	}
	for _, want := range []string{"Страницы сайта", "Сохранить", "▶ Сейчас", "interval_unit", "Расписание и журнал"} {
		if !strings.Contains(b1.String(), want) {
			t.Errorf("jobs: нет %q", want)
		}
	}

	// Журнал запусков.
	fin := now.Add(3 * time.Second)
	runs := runsPageData{Active: "runs", Available: true, JobNames: []string{"documents", "sitepages"},
		Runs: []jobsched.Run{{ID: "run-1", JobName: "documents", Trigger: "manual",
			StartedAt: now, FinishedAt: &fin, DurationMs: 3000, Status: "success",
			ItemsNew: 5, ItemsUpdated: 2, ItemsTotal: 7}},
	}
	var b2 bytes.Buffer
	if err := runsPageTmpl.ExecuteTemplate(&b2, "layout", runs); err != nil {
		t.Fatalf("runsPageTmpl: %v", err)
	}
	for _, want := range []string{"documents", "вручную", "детали", "/jobs/runs/run-1"} {
		if !strings.Contains(b2.String(), want) {
			t.Errorf("runs: нет %q", want)
		}
	}

	// Детали прогона + ИИ-вызовы.
	detail := runDetailData{Active: "runs",
		Run: jobsched.Run{ID: "run-1", JobName: "sitepages", Trigger: "schedule",
			StartedAt: now, FinishedAt: &fin, DurationMs: 3000, Status: "success",
			Details: map[string]any{"visited": 42}},
		Usage: []jobsched.AIUsage{{AgentType: "page_annotator", ModelID: "qwen-max",
			Provider: "alibabacloud", PromptTokens: 1200, CompletionTokens: 300,
			TotalTokens: 1500, DurationMs: 800, Success: true, CreatedAt: now}},
	}
	var b3 bytes.Buffer
	if err := runDetailTmpl.ExecuteTemplate(&b3, "layout", detail); err != nil {
		t.Fatalf("runDetailTmpl: %v", err)
	}
	for _, want := range []string{"Аннотатор страниц", "qwen-max", "1500", "Alibaba"} {
		if !strings.Contains(b3.String(), want) {
			t.Errorf("detail: нет %q", want)
		}
	}

	// Расход токенов.
	usage := aiUsageData{Active: "ai-usage", Available: true, SinceDays: 30,
		ByModel: []jobsched.UsageAgg{{Key: "qwen-max", Calls: 10, PromptTokens: 12000,
			CompletionTokens: 3000, TotalTokens: 15000, AvgDurationMs: 800}},
		ByAgent: []jobsched.UsageAgg{{Key: "page_annotator", Calls: 10, TotalTokens: 15000}},
		Recent: []jobsched.AIUsage{{AgentType: "consultant", ModelID: "qwen-plus",
			Provider: "alibabacloud", TotalTokens: 500, Success: true, CreatedAt: now}},
	}
	var b4 bytes.Buffer
	if err := aiUsageTmpl.ExecuteTemplate(&b4, "layout", usage); err != nil {
		t.Fatalf("aiUsageTmpl: %v", err)
	}
	for _, want := range []string{"Расход токенов по моделям", "qwen-max", "15000", "Консультант"} {
		if !strings.Contains(b4.String(), want) {
			t.Errorf("usage: нет %q", want)
		}
	}
}
