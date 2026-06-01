package sitepages

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"baza-skolkovo/src/aimodels"
)

// maxPromptText — сколько символов текста страницы отдаём модели на аннотирование.
const maxPromptText = 6000

// maxTags — верхний предел числа тегов на страницу.
const maxTags = 8

// maxTheses — верхний предел числа тезисов.
const maxTheses = 8

// Annotation — структурированная аннотация страницы, которую возвращает ИИ.
// JSON-теги совпадают с форматом ответа в системном промпте AgentPageAnnotator.
type Annotation struct {
	Tags        []string `json:"tags"`
	Summary     string   `json:"summary"`
	Goals       string   `json:"goals"`
	Theses      []string `json:"theses"`
	Conclusions string   `json:"conclusions"`
}

// chatFunc — вызов LLM (по умолчанию aimodels.ChatWithAgent; переопределяется в тестах).
type chatFunc func(ctx context.Context, m aimodels.Model, a aimodels.Agent, userMessage string) (string, int, error)

// Enricher аннотирует страницы сайта через ИИ-агента «Аннотатор страниц».
// Безопасен, когда агент не настроен: тогда аннотирование пропускается.
type Enricher struct {
	Store *aimodels.Store // конфигурация ИИ-моделей и агентов
	Pages *PostgresStore  // хранилище страниц (сохранение аннотаций, словарь тегов)
	Delay time.Duration   // пауза между LLM-запросами (троттлинг провайдера)

	chat     chatFunc
	skipOnce sync.Once // лог «агент не настроен» — не чаще одного раза
}

// NewEnricher создаёт обогатитель страниц.
func NewEnricher(store *aimodels.Store, pages *PostgresStore, delay time.Duration) *Enricher {
	return &Enricher{Store: store, Pages: pages, Delay: delay, chat: aimodels.ChatWithAgent}
}

// EnrichBatch аннотирует страницы последовательно (с паузой Delay между
// запросами), устойчиво к ошибкам отдельной страницы. Если включённого агента
// «Аннотатор страниц» с рабочей моделью нет — пропускает весь батч.
// Возвращает счётчики: обогащено / пропущено / с ошибкой.
func (e *Enricher) EnrichBatch(ctx context.Context, pages []*Page) (done, skipped, failed int) {
	if len(pages) == 0 {
		return 0, 0, 0
	}
	agent, model, err := e.Store.EnabledAgentWithModel(ctx, aimodels.AgentPageAnnotator)
	if err != nil {
		e.skipOnce.Do(func() {
			log.Printf("[sitepages/enrich] агент «Аннотатор страниц» не настроен — аннотирование пропущено (%v)", err)
		})
		return 0, len(pages), 0
	}

	known, _ := e.Pages.ListTags(ctx)
	chat := e.chat
	if chat == nil {
		chat = aimodels.ChatWithAgent
	}

	for i, p := range pages {
		select {
		case <-ctx.Done():
			return done, skipped, failed
		default:
		}
		if i > 0 && e.Delay > 0 {
			select {
			case <-ctx.Done():
				return done, skipped, failed
			case <-time.After(e.Delay):
			}
		}

		ann, err := e.annotate(ctx, chat, agent, model, p, known)
		if err != nil {
			failed++
			log.Printf("[sitepages/enrich] %s: %v", p.URL, err)
			continue
		}
		if err := e.Pages.UpdateEnrichment(ctx, p.ID, ann, p.ContentHash); err != nil {
			failed++
			log.Printf("[sitepages/enrich] сохранение %s: %v", p.URL, err)
			continue
		}
		_ = e.Pages.BumpTags(ctx, ann.Tags)
		known = mergeKnown(known, ann.Tags) // следующие страницы переиспользуют новые теги
		applyAnnotation(p, ann)             // для последующей переиндексации того же среза
		done++
	}
	return done, skipped, failed
}

// annotate выполняет один LLM-запрос и нормализует результат.
func (e *Enricher) annotate(ctx context.Context, chat chatFunc, agent aimodels.Agent, model aimodels.Model, p *Page, known []string) (Annotation, error) {
	cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	raw, _, err := chat(cctx, model, agent, buildAnnotatePrompt(p, known))
	if err != nil {
		return Annotation{}, fmt.Errorf("LLM: %w", err)
	}
	ann, err := parseAnnotation(raw)
	if err != nil {
		return Annotation{}, err
	}
	ann.Tags = normalizeTags(ann.Tags, known, maxTags)
	ann.Theses = cleanList(ann.Theses, maxTheses)
	ann.Summary = strings.TrimSpace(ann.Summary)
	ann.Goals = strings.TrimSpace(ann.Goals)
	ann.Conclusions = strings.TrimSpace(ann.Conclusions)
	return ann, nil
}

// buildAnnotatePrompt формирует пользовательское сообщение для агента: метаданные
// страницы, словарь уже существующих тегов (для переиспользования) и текст.
func buildAnnotatePrompt(p *Page, known []string) string {
	var b strings.Builder
	b.WriteString("Заголовок: ")
	b.WriteString(p.Title)
	b.WriteString("\nURL: ")
	b.WriteString(p.URL)
	if p.Section != "" {
		b.WriteString("\nРаздел: ")
		b.WriteString(p.Section)
	}
	if len(known) > 0 {
		b.WriteString("\n\nУже существующие теги (по возможности переиспользуй подходящие): ")
		b.WriteString(strings.Join(firstN(known, 60), ", "))
	}
	text := p.Text
	if strings.TrimSpace(text) == "" {
		text = p.Summary
	}
	b.WriteString("\n\nТекст страницы:\n")
	b.WriteString(truncate(text, maxPromptText))
	b.WriteString("\n\nВерни строго JSON по заданному формату.")
	return b.String()
}

// parseAnnotation извлекает JSON-объект из ответа модели (терпимо к markdown-
// ограждениям и поясняющему тексту вокруг) и разбирает его в Annotation.
func parseAnnotation(raw string) (Annotation, error) {
	s := strings.TrimSpace(raw)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return Annotation{}, fmt.Errorf("в ответе модели не найден JSON-объект")
	}
	var a Annotation
	if err := json.Unmarshal([]byte(s[start:end+1]), &a); err != nil {
		return Annotation{}, fmt.Errorf("разбор JSON аннотации: %w", err)
	}
	return a, nil
}

// normalizeTags приводит теги к нижнему регистру, сжимает пробелы, дедуплицирует,
// отбрасывает пустые/слишком длинные и ограничивает число до max. Гибрид: теги,
// уже присутствующие в словаре known, ставятся первыми (приоритет при обрезке) —
// так фильтр множественного выбора остаётся согласованным. Всегда не-nil.
func normalizeTags(tags, known []string, max int) []string {
	if max <= 0 {
		max = maxTags
	}
	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		if lk := strings.ToLower(strings.TrimSpace(k)); lk != "" {
			knownSet[lk] = true
		}
	}
	seen := make(map[string]bool)
	var inDict, novel []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		t = strings.Join(strings.Fields(t), " ")
		if t == "" || len([]rune(t)) > 40 {
			continue
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		if knownSet[t] {
			inDict = append(inDict, t)
		} else {
			novel = append(novel, t)
		}
	}
	out := append(inDict, novel...)
	if len(out) > max {
		out = out[:max]
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// cleanList обрезает пробелы, отбрасывает пустые элементы, дедуплицирует и
// ограничивает длину списка. Всегда не-nil.
func cleanList(items []string, max int) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		key := strings.ToLower(it)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// mergeKnown добавляет новые (нормализованные) теги к словарю в памяти.
func mergeKnown(known, add []string) []string {
	set := make(map[string]bool, len(known))
	for _, k := range known {
		set[strings.ToLower(k)] = true
	}
	for _, t := range add {
		lt := strings.ToLower(strings.TrimSpace(t))
		if lt != "" && !set[lt] {
			set[lt] = true
			known = append(known, lt)
		}
	}
	return known
}

// applyAnnotation проставляет ИИ-поля в страницу (для переиндексации того же среза).
func applyAnnotation(p *Page, a Annotation) {
	now := time.Now()
	p.Tags = a.Tags
	p.AISummary = a.Summary
	p.Goals = a.Goals
	p.Theses = a.Theses
	p.Conclusions = a.Conclusions
	p.EnrichedAt = &now
	p.EnrichHash = p.ContentHash
}

func firstN(ss []string, n int) []string {
	if n > 0 && len(ss) > n {
		return ss[:n]
	}
	return ss
}
