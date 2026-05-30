// Package classifier реализует ИИ-классификацию документов по категориям
// на основе семантической близости эмбеддингов (cosine similarity).
package classifier

import (
	"context"
	"math"
	"strings"
	"sync"

	"baza-skolkovo/src/common/embed"
)

// Поддерживаемые категории классификации.
var DefaultCategories = []string{
	"Документы фонда",
	"Требования",
	"Процедуры",
	"Отчётность",
	"Новости",
	"Мероприятия",
	"Конкурсы",
	"FAQ",
	"Телеграм",
}

// ClassifierConfig хранит параметры классификатора.
type ClassifierConfig struct {
	// Threshold — порог cosine similarity для уверенной классификации (0..1).
	// Значения ниже порога считаются «не определено».
	Threshold float64
}

// ClassificationResult — результат классификации одного документа.
type ClassificationResult struct {
	// Category — наиболее подходящая категория.
	Category string
	// Confidence — уверенность классификации (0..1), cosine similarity с лучшим совпадением.
	Confidence float64
	// AllScores — cosine similarity для каждой категории.
	AllScores map[string]float64
}

// DocumentClassifier классифицирует документы по категориям.
type DocumentClassifier struct {
	embedClient        embed.Embedder
	config             ClassifierConfig
	categoryEmbeddings map[string][]float32 // кэш эмбеддингов категорий
	mu                 sync.RWMutex
}

// NewDocumentClassifier создаёт классификатор с заданным клиентом эмбеддингов.
func NewDocumentClassifier(embedClient embed.Embedder, config ClassifierConfig) *DocumentClassifier {
	if config.Threshold <= 0 {
		config.Threshold = 0.5
	}
	return &DocumentClassifier{
		embedClient:        embedClient,
		config:             config,
		categoryEmbeddings: make(map[string][]float32),
	}
}

// GetCategories возвращает список поддерживаемых категорий.
func (c *DocumentClassifier) GetCategories() []string {
	out := make([]string, len(DefaultCategories))
	copy(out, DefaultCategories)
	return out
}

// PrecomputeCategoryEmbeddings предвычисляет и кэширует эмбеддинги названий категорий.
// Использует префикс "query: " для моделей e5.
func (c *DocumentClassifier) PrecomputeCategoryEmbeddings(ctx context.Context, categories []string) error {
	if len(categories) == 0 {
		categories = DefaultCategories
	}

	texts := make([]string, len(categories))
	for i, cat := range categories {
		texts[i] = embed.PrefixQuery + cat
	}

	embeddings, err := c.embedClient.Embed(ctx, texts)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for i, cat := range categories {
		c.categoryEmbeddings[cat] = embeddings[i]
	}

	return nil
}

// Classify классифицирует документ по тексту и заголовку.
// Возвращает наиболее подходящую категорию, уверенность и все scores.
func (c *DocumentClassifier) Classify(ctx context.Context, documentText, title string) (ClassificationResult, error) {
	// Формируем входной текст: заголовок + тело документа.
	input := buildInput(title, documentText)

	emb, err := c.embedClient.Embed(ctx, []string{embed.PrefixQuery + input})
	if err != nil {
		return ClassificationResult{}, err
	}
	docEmbedding := emb[0]

	c.mu.RLock()
	catEmbs := make(map[string][]float32, len(c.categoryEmbeddings))
	for k, v := range c.categoryEmbeddings {
		catEmbs[k] = v
	}
	c.mu.RUnlock()

	result := ClassificationResult{
		AllScores: make(map[string]float64),
	}

	for category, catEmb := range catEmbs {
		sim := cosineSimilarity(docEmbedding, catEmb)
		result.AllScores[category] = sim

		if sim > result.Confidence {
			result.Confidence = sim
			result.Category = category
		}
	}

	// Если уверенность ниже порога — категория не определена.
	if result.Confidence < c.config.Threshold {
		result.Category = ""
	}

	return result, nil
}

// buildInput формирует текст для классификации из заголовка и тела документа.
func buildInput(title, body string) string {
	var sb strings.Builder
	if title != "" {
		sb.WriteString(title)
		sb.WriteString(". ")
	}
	sb.WriteString(body)
	return sb.String()
}

// cosineSimilarity вычисляет косинусную близость двух векторов.
// Возвращает значение в диапазоне [-1, 1], для неотрицательных эмбеддингов [0, 1].
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
