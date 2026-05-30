// Package audit оценивает полноту охвата источников Сколково: сверяет известный
// «каталог» источников (документы, новости, мероприятия, конкурсы, гранты, FAQ,
// льготы, НПА, Telegram, реестр резидентов) с тем, что реально настроено, свежо
// и наполнено данными. В отличие от validator (внутренняя целостность записей),
// audit отвечает на вопрос «парсим ли мы вообще всё, что есть на сайте Сколково».
package audit

import (
	"fmt"
	"strings"
	"time"
)

// Status — итоговая оценка покрытия одного источника.
type Status string

const (
	StatusCovered  Status = "covered"  // настроен, свеж и содержит данные
	StatusStale    Status = "stale"    // настроен, но давно не обновлялся
	StatusFailing  Status = "failing"  // настроен, но прогоны падают
	StatusNoData   Status = "no_data"  // настроен, но данных нет
	StatusDisabled Status = "disabled" // не настроен/выключен
	StatusUnknown  Status = "unknown"  // нет данных о состоянии
)

// Coverage — состояние охвата одного источника.
type Coverage struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	Enabled      bool   `json:"enabled"`
	HealthState  string `json:"health_state,omitempty"` // ok|stale|failing|unknown|"" (нет записи)
	ItemsLastRun int    `json:"items_last_run"`
	Items        int    `json:"items"` // всего записей в хранилище; -1 — неизвестно
	Status       Status `json:"status"`
	Note         string `json:"note,omitempty"`
}

// Report — сводный отчёт о покрытии.
type Report struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Sources     []Coverage `json:"sources"`
	CoveredN    int        `json:"covered"`
	TotalN      int        `json:"total"`
}

// Expected — ожидаемый источник контента Сколково (канонический каталог).
type Expected struct {
	Key  string
	Name string
}

// ExpectedSources возвращает канонический список источников, которые система
// должна покрывать, чтобы «закрыть всё, что есть на сайте Сколково».
func ExpectedSources() []Expected {
	return []Expected{
		{"documents", "Документы dochub.sk.ru (нормативные, регламенты)"},
		{"fetch", "Тела файлов документов (обход WAF)"},
		{"news", "Новости sk.ru"},
		{"events", "Мероприятия"},
		{"contests", "Конкурсы и гранты"},
		{"faq", "FAQ"},
		{"preferences", "Льготы резидентов"},
		{"regulations", "НПА (244-ФЗ и поправки)"},
		{"telegram", "Telegram-каналы"},
		{"residents", "Реестр резидентов"},
	}
}

// Classify вычисляет статус покрытия источника.
func Classify(c Coverage) Status {
	if !c.Enabled {
		return StatusDisabled
	}
	switch c.HealthState {
	case "failing":
		return StatusFailing
	case "stale":
		return StatusStale
	}
	if c.Items == 0 {
		return StatusNoData
	}
	if c.HealthState == "ok" || c.Items > 0 {
		return StatusCovered
	}
	return StatusUnknown
}

// Build классифицирует источники и считает сводку. GeneratedAt задаёт вызывающий.
func Build(sources []Coverage) Report {
	rep := Report{Sources: make([]Coverage, 0, len(sources)), TotalN: len(sources)}
	for _, c := range sources {
		c.Status = Classify(c)
		if c.Status == StatusCovered {
			rep.CoveredN++
		}
		rep.Sources = append(rep.Sources, c)
	}
	return rep
}

// ToMarkdown рендерит отчёт о покрытии в Markdown.
func ToMarkdown(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Отчёт о полноте охвата источников — %s\n\n", r.GeneratedAt.Format("02.01.2006 15:04"))
	fmt.Fprintf(&b, "**Покрыто:** %d из %d источников\n\n", r.CoveredN, r.TotalN)
	b.WriteString("| Источник | Статус | Записей | Свежесть | Последний прогон | URL |\n")
	b.WriteString("| :--- | :--- | ---: | :--- | ---: | :--- |\n")
	for _, c := range r.Sources {
		items := "—"
		if c.Items >= 0 {
			items = fmt.Sprintf("%d", c.Items)
		}
		health := c.HealthState
		if health == "" {
			health = "—"
		}
		url := c.URL
		if url == "" {
			url = "—"
		}
		fmt.Fprintf(&b, "| %s | %s %s | %s | %s | %d | %s |\n",
			c.Name, statusEmoji(c.Status), c.Status, items, health, c.ItemsLastRun, url)
	}
	b.WriteString("\n")

	// Рекомендации по непокрытым источникам.
	var recs []string
	for _, c := range r.Sources {
		switch c.Status {
		case StatusDisabled:
			recs = append(recs, fmt.Sprintf("Источник «%s» не настроен — включите и задайте URL.", c.Name))
		case StatusNoData:
			recs = append(recs, fmt.Sprintf("Источник «%s» настроен, но данных нет — проверьте парсер/URL.", c.Name))
		case StatusStale:
			recs = append(recs, fmt.Sprintf("Источник «%s» давно не обновлялся — проверьте планировщик.", c.Name))
		case StatusFailing:
			recs = append(recs, fmt.Sprintf("Источник «%s» падает несколько прогонов подряд — нужно вмешательство.", c.Name))
		}
	}
	if len(recs) > 0 {
		b.WriteString("## Рекомендации\n\n")
		for _, r := range recs {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}
	b.WriteString("---\n\n*Сгенерировано аудитом полноты охвата «База Сколково».*\n")
	return b.String()
}

func statusEmoji(s Status) string {
	switch s {
	case StatusCovered:
		return "✅"
	case StatusStale:
		return "🟡"
	case StatusFailing:
		return "🔴"
	case StatusNoData:
		return "⚪"
	case StatusDisabled:
		return "⚫"
	default:
		return "❓"
	}
}
