// Package docenrich — ИИ-разметка нормативных документов: автоматически
// проставляет КАТЕГОРИЮ (из канонических 8), уточняющую ПОДКАТЕГОРИЮ и ТЕГИ
// через агента «Аннотатор документов» (aimodels.AgentDocAnnotator).
//
// Повторяет паттерн src/sitepages/enrich.go: гибридный растущий словарь тегов
// (document_tags), разметка только при изменении файла (enrich_hash <> file_hash),
// устойчивость к ошибкам отдельного документа, троттлинг между LLM-запросами.
// Безопасен, когда агент/модель не настроены — тогда разметка пропускается.
package docenrich

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"baza-skolkovo/src/aimodels"
	"baza-skolkovo/src/common/extract"
	"baza-skolkovo/src/common/model"
	"baza-skolkovo/src/common/store"
)

const (
	maxPromptText = 6000 // сколько символов текста документа отдаём модели
	maxTags       = 8    // верхний предел тегов на документ
)

// CanonicalCategories — закрытый список категорий (совпадает со scraper.CategoryNames).
// ИИ обязан выбрать одну из них; иначе оставляем прежнюю категорию документа.
var CanonicalCategories = []string{
	"Законодательные акты",
	"Правила проектирования",
	"Иные нормативные документы",
	"Развитие территории",
	"Закупки и тендеры",
	"Утратившие силу",
	"Антикоррупция",
	"Кибербезопасность и перс. данные",
}

// Classification — результат разметки документа, который возвращает ИИ.
// JSON-теги совпадают с форматом ответа в промпте AgentDocAnnotator.
type Classification struct {
	Category    string   `json:"category"`
	Subcategory string   `json:"subcategory"`
	Tags        []string `json:"tags"`
}

// chatFunc — вызов LLM (по умолчанию aimodels.ChatWithAgent; переопределяется в тестах).
type chatFunc func(ctx context.Context, m aimodels.Model, a aimodels.Agent, userMessage string) (string, int, error)

// Enricher размечает документы через агента «Аннотатор документов».
type Enricher struct {
	AI    *aimodels.Store      // конфигурация ИИ-моделей и агентов
	Store *store.PostgresStore // реестр документов (UpdateClassification, словарь тегов)
	Delay time.Duration        // пауза между LLM-запросами (троттлинг провайдера)

	chat     chatFunc
	skipOnce sync.Once
}

// New создаёт обогатитель документов.
func New(ai *aimodels.Store, st *store.PostgresStore, delay time.Duration) *Enricher {
	return &Enricher{AI: ai, Store: st, Delay: delay, chat: aimodels.ChatWithAgent}
}

// EnrichNeeded размечает действующие документы, которым нужна разметка (ещё не
// размечены или изменился файл). limit<=0 — все. Возвращает счётчики.
func (e *Enricher) EnrichNeeded(ctx context.Context, limit int) (done, skipped, failed int, err error) {
	docs, err := e.Store.ListNeedingEnrichment(ctx, limit)
	if err != nil {
		return 0, 0, 0, err
	}
	d, s, f := e.EnrichBatch(ctx, docs)
	return d, s, f, nil
}

// EnrichBatch размечает документы последовательно (с паузой Delay), устойчиво к
// ошибкам отдельного документа. Если включённого агента «Аннотатор документов»
// с рабочей моделью нет — пропускает весь батч.
func (e *Enricher) EnrichBatch(ctx context.Context, docs []model.Document) (done, skipped, failed int) {
	if len(docs) == 0 {
		return 0, 0, 0
	}
	agent, mdl, err := e.AI.EnabledAgentWithModel(ctx, aimodels.AgentDocAnnotator)
	if err != nil {
		e.skipOnce.Do(func() {
			log.Printf("[docenrich] агент «Аннотатор документов» не настроен — разметка пропущена (%v)", err)
		})
		return 0, len(docs), 0
	}

	known, _ := e.Store.ListTags(ctx)
	chat := e.chat
	if chat == nil {
		chat = aimodels.ChatWithAgent
	}

	for i, d := range docs {
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

		cls, err := e.annotate(ctx, chat, agent, mdl, d, known)
		if err != nil {
			failed++
			log.Printf("[docenrich] %s (%s): %v", d.ID, d.Title, err)
			continue
		}
		// Категория: принимаем только из канонического списка, иначе оставляем прежнюю.
		category := canonicalCategory(cls.Category)
		if category == "" {
			category = d.Category
		}
		if err := e.Store.UpdateClassification(ctx, d.ID, category, cls.Subcategory, cls.Tags, d.FileHash); err != nil {
			failed++
			log.Printf("[docenrich] сохранение %s: %v", d.ID, err)
			continue
		}
		_ = e.Store.BumpTags(ctx, cls.Tags)
		known = mergeKnown(known, cls.Tags)
		done++
	}
	return done, skipped, failed
}

// annotate выполняет один LLM-запрос и нормализует результат.
func (e *Enricher) annotate(ctx context.Context, chat chatFunc, agent aimodels.Agent, mdl aimodels.Model, d model.Document, known []string) (Classification, error) {
	cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	raw, _, err := chat(cctx, mdl, agent, buildPrompt(d, known))
	if err != nil {
		return Classification{}, fmt.Errorf("LLM: %w", err)
	}
	cls, err := parseClassification(raw)
	if err != nil {
		return Classification{}, err
	}
	cls.Category = strings.TrimSpace(cls.Category)
	cls.Subcategory = strings.TrimSpace(cls.Subcategory)
	cls.Tags = normalizeTags(cls.Tags, known, maxTags)
	return cls, nil
}

// buildPrompt формирует сообщение для агента: заголовок, текущая категория,
// словарь существующих тегов и (если есть файл) фрагмент текста документа.
func buildPrompt(d model.Document, known []string) string {
	var b strings.Builder
	b.WriteString("Заголовок документа: ")
	b.WriteString(d.Title)
	if d.Category != "" {
		b.WriteString("\nТекущая категория (можешь уточнить/исправить): ")
		b.WriteString(d.Category)
	}
	b.WriteString("\nДопустимые категории: ")
	b.WriteString(strings.Join(CanonicalCategories, "; "))
	if len(known) > 0 {
		b.WriteString("\nУже существующие теги (по возможности переиспользуй подходящие): ")
		b.WriteString(strings.Join(firstN(known, 60), ", "))
	}
	if text := documentText(d); text != "" {
		b.WriteString("\n\nТекст документа:\n")
		b.WriteString(truncate(text, maxPromptText))
	}
	b.WriteString("\n\nВерни строго JSON по формату " +
		`{"category":"...","subcategory":"...","tags":["...","..."]}.`)
	return b.String()
}

// documentText извлекает текст документа из локального файла (если он есть и
// поддерживается). Пусто — классифицируем только по заголовку и категории.
func documentText(d model.Document) string {
	if d.LocalPath == "" || !extract.IsSupported(d.LocalPath) {
		return ""
	}
	text, err := extract.Text(d.LocalPath)
	if err != nil {
		return ""
	}
	return text
}

// parseClassification извлекает JSON-объект из ответа модели (терпимо к
// markdown-ограждениям и тексту вокруг) и разбирает его в Classification.
func parseClassification(raw string) (Classification, error) {
	s := strings.TrimSpace(raw)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return Classification{}, fmt.Errorf("в ответе модели не найден JSON-объект")
	}
	var c Classification
	if err := json.Unmarshal([]byte(s[start:end+1]), &c); err != nil {
		return Classification{}, fmt.Errorf("разбор JSON разметки: %w", err)
	}
	return c, nil
}

// canonicalCategory сопоставляет ответ модели каноническому названию категории
// (без учёта регистра/пробелов). Возвращает "" если не из списка.
func canonicalCategory(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	for _, c := range CanonicalCategories {
		if strings.ToLower(c) == s {
			return c
		}
	}
	return ""
}

// normalizeTags — нижний регистр, сжатие пробелов, дедуп, отбрасывание пустых/
// длинных, лимит. Гибрид: теги из словаря known идут первыми. Всегда не-nil.
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
		t = strings.Join(strings.Fields(strings.ToLower(t)), " ")
		if t == "" || len([]rune(t)) > 40 || seen[t] {
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

func firstN(ss []string, n int) []string {
	if n > 0 && len(ss) > n {
		return ss[:n]
	}
	return ss
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
