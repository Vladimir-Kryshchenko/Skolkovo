package classifier

import (
	"context"
	"math"
	"testing"
)

// mockEmbedder — заглушка embed.Embedder, возвращающая фиксированные эмбеддинги.
type mockEmbedder struct {
	embeddings map[string][]float32
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		if emb, ok := m.embeddings[text]; ok {
			result[i] = emb
		} else {
			// Детерминированный «хэш» для неизвестных текстов.
			result[i] = deterministicEmbed(text)
		}
	}
	return result, nil
}

// deterministicEmbed генерирует псевдо-эмбеддинг из текста (для тестов).
func deterministicEmbed(text string) []float32 {
	// Размерность 64 — достаточно для тестирования cosine similarity.
	const dim = 64
	vec := make([]float32, dim)
	for i := 0; i < len(text) && i < dim; i++ {
		vec[i] = float32(text[i]) / 255.0
	}
	// Нормализуем, чтобы вектор был единичной длины.
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := float32(math.Sqrt(sum))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{0.1, 0.2, 0.3, 0.4}
	b := []float32{0.1, 0.2, 0.3, 0.4}

	got := cosineSimilarity(a, b)
	if got < 0.999 {
		t.Errorf("cosineSimilarity(identical) = %f, want ~1.0", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0, 0}
	b := []float32{0, 1, 0, 0}

	got := cosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("cosineSimilarity(orthogonal) = %f, want 0", got)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}

	got := cosineSimilarity(a, b)
	if got > -0.999 {
		t.Errorf("cosineSimilarity(opposite) = %f, want ~-1.0", got)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}

	got := cosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("cosineSimilarity(different lengths) = %f, want 0", got)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	got := cosineSimilarity([]float32{}, []float32{})
	if got != 0 {
		t.Errorf("cosineSimilarity(empty) = %f, want 0", got)
	}
}

func TestCosineSimilarity_ZeroVectors(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{0, 0, 0}

	got := cosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("cosineSimilarity(zero vectors) = %f, want 0", got)
	}
}

func TestNewDocumentClassifier_DefaultThreshold(t *testing.T) {
	c := NewDocumentClassifier(&mockEmbedder{}, ClassifierConfig{})
	if c.config.Threshold != 0.5 {
		t.Errorf("default threshold = %f, want 0.5", c.config.Threshold)
	}
}

func TestGetCategories(t *testing.T) {
	c := NewDocumentClassifier(&mockEmbedder{}, ClassifierConfig{})
	cats := c.GetCategories()

	if len(cats) != len(DefaultCategories) {
		t.Errorf("got %d categories, want %d", len(cats), len(DefaultCategories))
	}

	expected := map[string]bool{
		"Документы фонда": true,
		"Требования":      true,
		"Процедуры":       true,
		"Отчётность":      true,
		"Новости":         true,
		"Мероприятия":     true,
		"Конкурсы":        true,
		"FAQ":             true,
		"Телеграм":        true,
	}

	for _, cat := range cats {
		if !expected[cat] {
			t.Errorf("unexpected category: %q", cat)
		}
	}
}

func TestPrecomputeCategoryEmbeddings(t *testing.T) {
	emb := &mockEmbedder{
		embeddings: map[string][]float32{
			"query: Новости":   {0.1, 0.2, 0.3},
			"query: Процедуры": {0.4, 0.5, 0.6},
		},
	}

	c := NewDocumentClassifier(emb, ClassifierConfig{Threshold: 0.5})

	cats := []string{"Новости", "Процедуры"}
	if err := c.PrecomputeCategoryEmbeddings(context.Background(), cats); err != nil {
		t.Fatalf("PrecomputeCategoryEmbeddings error: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.categoryEmbeddings) != 2 {
		t.Errorf("got %d cached embeddings, want 2", len(c.categoryEmbeddings))
	}
}

func TestPrecomputeCategoryEmbeddings_DefaultCategories(t *testing.T) {
	emb := &mockEmbedder{}
	c := NewDocumentClassifier(emb, ClassifierConfig{})

	if err := c.PrecomputeCategoryEmbeddings(context.Background(), nil); err != nil {
		t.Fatalf("PrecomputeCategoryEmbeddings error: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.categoryEmbeddings) != len(DefaultCategories) {
		t.Errorf("got %d cached embeddings, want %d", len(c.categoryEmbeddings), len(DefaultCategories))
	}
}

func TestClassify(t *testing.T) {
	// Создаём эмбеддер, где "query: Новости" очень близок к тексту про новости.
	newsEmb := deterministicEmbed("query: Новости")
	// Текст документа будет иметь эмбеддинг, близкий к «query: Новости».
	docEmb := make([]float32, len(newsEmb))
	copy(docEmb, newsEmb)
	// Делаем чуть отличающимся, но очень близким.
	for i := range docEmb {
		docEmb[i] += 0.001
	}

	emb := &mockEmbedder{
		embeddings: map[string][]float32{
			"query: Новый закон о фондах": docEmb, // текст документа → Новости
		},
	}

	c := NewDocumentClassifier(emb, ClassifierConfig{Threshold: 0.5})

	// Предвычисляем эмбеддинги категорий.
	c.mu.Lock()
	c.categoryEmbeddings["Новости"] = newsEmb
	// Другие категории — сильно отличающиеся.
	c.categoryEmbeddings["Процедуры"] = []float32{1, 0, 0, 0, 0, 0, 0, 0}
	c.categoryEmbeddings["FAQ"] = []float32{0, 1, 0, 0, 0, 0, 0, 0}
	c.mu.Unlock()

	result, err := c.Classify(context.Background(), "Новый закон о фондах принят", "Новый закон")
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}

	if result.Category != "Новости" {
		t.Errorf("category = %q, want %q", result.Category, "Новости")
	}

	if result.Confidence < c.config.Threshold {
		t.Errorf("confidence = %f, should be >= threshold %f", result.Confidence, c.config.Threshold)
	}

	if len(result.AllScores) != 3 {
		t.Errorf("got %d scores, want 3", len(result.AllScores))
	}
}

func TestClassify_BelowThreshold(t *testing.T) {
	// Текст документа сильно отличается от всех категорий.
	emb := &mockEmbedder{}

	c := NewDocumentClassifier(emb, ClassifierConfig{Threshold: 0.95}) // Высокий порог

	c.mu.Lock()
	c.categoryEmbeddings["Новости"] = []float32{1, 0, 0, 0}
	c.categoryEmbeddings["FAQ"] = []float32{0, 1, 0, 0}
	c.mu.Unlock()

	result, err := c.Classify(context.Background(), "Совершенно другой текст", "Заголовок")
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}

	if result.Category != "" {
		t.Errorf("category = %q, want empty (below threshold)", result.Category)
	}
}

func TestBuildInput(t *testing.T) {
	tests := []struct {
		title string
		body  string
		want  string
	}{
		{"Заголовок", "Тело", "Заголовок. Тело"},
		{"", "Только тело", "Только тело"},
		{"Заголовок", "", "Заголовок. "},
		{"", "", ""},
	}

	for _, tt := range tests {
		got := buildInput(tt.title, tt.body)
		if got != tt.want {
			t.Errorf("buildInput(%q, %q) = %q, want %q", tt.title, tt.body, got, tt.want)
		}
	}
}

func TestCosineSimilarity_Symmetric(t *testing.T) {
	a := []float32{0.3, 0.5, 0.2, 0.1}
	b := []float32{0.1, 0.2, 0.5, 0.3}

	ab := cosineSimilarity(a, b)
	ba := cosineSimilarity(b, a)

	if math.Abs(ab-ba) > 1e-10 {
		t.Errorf("cosineSimilarity not symmetric: %f vs %f", ab, ba)
	}
}
