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
	"sync/atomic"
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
	Delay time.Duration        // пауза между LLM-запросами (троттлинг, для последовательного режима)
	// Concurrency — число параллельных воркеров разметки (1 — последовательно).
	Concurrency int

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
	agent, models, err := e.AI.EnabledAgentWithModels(ctx, aimodels.AgentDocAnnotator)
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
	conc := e.Concurrency
	if conc < 1 {
		conc = 1
	}

	var doneN, failedN int64
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup

	for _, d := range docs {
		select {
		case <-ctx.Done():
			wg.Wait()
			return int(doneN), 0, int(failedN)
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(d model.Document) {
			defer wg.Done()
			defer func() { <-sem }()

			cls, err := e.annotate(ctx, chat, agent, models, d, known)
			if err != nil {
				atomic.AddInt64(&failedN, 1)
				log.Printf("[docenrich] %s (%s): %v", d.ID, d.Title, err)
				return
			}
			// Категория: принимаем только из канонического списка, иначе оставляем прежнюю.
			category := canonicalCategory(cls.Category)
			if category == "" {
				category = d.Category
			}
			if err := e.Store.UpdateClassification(ctx, d.ID, category, cls.Subcategory, cls.Tags, d.FileHash); err != nil {
				atomic.AddInt64(&failedN, 1)
				log.Printf("[docenrich] сохранение %s: %v", d.ID, err)
				return
			}
			_ = e.Store.BumpTags(ctx, cls.Tags)
			atomic.AddInt64(&doneN, 1)
		}(d)
	}
	wg.Wait()
	return int(doneN), 0, int(failedN)
}

// annotate выполняет LLM-запрос с авто-переключением моделей и нормализует
// результат. Перебирает models по порядку (основная, затем резервные): при ошибке
// вызова (сеть, лимит/квота, API-ошибка) переходит к следующей модели.
func (e *Enricher) annotate(ctx context.Context, chat chatFunc, agent aimodels.Agent, models []aimodels.Model, d model.Document, known []string) (Classification, error) {
	cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	prompt := buildPrompt(d, known)
	var raw string
	var lastErr error
	for i, mdl := range models {
		var err error
		raw, _, err = chat(cctx, mdl, agent, prompt)
		if err == nil {
			break
		}
		lastErr = err
		log.Printf("[docenrich] модель %s не справилась (%d/%d): %v — пробую следующую", mdl.ModelID, i+1, len(models), err)
	}
	if raw == "" && lastErr != nil {
		return Classification{}, fmt.Errorf("LLM (все %d моделей): %w", len(models), lastErr)
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
