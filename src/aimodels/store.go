package aimodels

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store — PostgreSQL-хранилище конфигураций ИИ-моделей и агентов.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore создаёт хранилище.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ─── Models ───────────────────────────────────────────────────────────────────

const modelColumns = `id, name, provider, model_id, base_url, api_key,
    max_tokens, temperature, enabled, description, created_at, updated_at`

func scanModel(row pgx.Row) (Model, error) {
	var m Model
	err := row.Scan(
		&m.ID, &m.Name, &m.Provider, &m.ModelID, &m.BaseURL, &m.APIKey,
		&m.MaxTokens, &m.Temperature, &m.Enabled, &m.Description,
		&m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

// ListModels возвращает все модели, упорядоченные по провайдеру и имени.
func (s *Store) ListModels(ctx context.Context) ([]Model, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+modelColumns+` FROM ai_models ORDER BY provider, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Model
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// GetModel возвращает модель по ID.
func (s *Store) GetModel(ctx context.Context, id string) (Model, error) {
	m, err := scanModel(s.pool.QueryRow(ctx,
		`SELECT `+modelColumns+` FROM ai_models WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Model{}, fmt.Errorf("модель %s не найдена", id)
	}
	return m, err
}

// CreateModel создаёт модель и возвращает её с заполненным ID.
func (s *Store) CreateModel(ctx context.Context, m Model) (Model, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ai_models
		  (name, provider, model_id, base_url, api_key, max_tokens, temperature, enabled, description)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING `+modelColumns,
		m.Name, m.Provider, m.ModelID, m.BaseURL, m.APIKey,
		m.MaxTokens, m.Temperature, m.Enabled, m.Description,
	).Scan(
		&m.ID, &m.Name, &m.Provider, &m.ModelID, &m.BaseURL, &m.APIKey,
		&m.MaxTokens, &m.Temperature, &m.Enabled, &m.Description,
		&m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

// UpdateModel обновляет конфигурацию модели.
func (s *Store) UpdateModel(ctx context.Context, m Model) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ai_models SET
			name=$1, provider=$2, model_id=$3, base_url=$4, api_key=$5,
			max_tokens=$6, temperature=$7, enabled=$8, description=$9,
			updated_at=NOW()
		WHERE id=$10`,
		m.Name, m.Provider, m.ModelID, m.BaseURL, m.APIKey,
		m.MaxTokens, m.Temperature, m.Enabled, m.Description, m.ID)
	return err
}

// DeleteModel удаляет модель.
func (s *Store) DeleteModel(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ai_models WHERE id=$1`, id)
	return err
}

// CountModelsByProvider возвращает количество моделей указанного провайдера.
func (s *Store) CountModelsByProvider(ctx context.Context, provider Provider) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_models WHERE provider=$1`, provider).Scan(&n)
	return n, err
}

// ─── Agents ───────────────────────────────────────────────────────────────────

const agentColumns = `id, name, agent_type, COALESCE(model_id::text,''), system_prompt,
    temperature, max_tokens, enabled, description, created_at, updated_at`

func scanAgent(row pgx.Row) (Agent, error) {
	var a Agent
	err := row.Scan(
		&a.ID, &a.Name, &a.AgentType, &a.ModelID, &a.SystemPrompt,
		&a.Temperature, &a.MaxTokens, &a.Enabled, &a.Description,
		&a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

// ListAgents возвращает всех агентов.
func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+agentColumns+` FROM ai_agents ORDER BY agent_type, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetAgent возвращает агента по ID.
func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	a, err := scanAgent(s.pool.QueryRow(ctx,
		`SELECT `+agentColumns+` FROM ai_agents WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, fmt.Errorf("агент %s не найден", id)
	}
	return a, err
}

// CreateAgent создаёт агента.
func (s *Store) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	var modelID *string
	if a.ModelID != "" {
		modelID = &a.ModelID
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO ai_agents
		  (name, agent_type, model_id, system_prompt, temperature, max_tokens, enabled, description)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+agentColumns,
		a.Name, a.AgentType, modelID, a.SystemPrompt,
		a.Temperature, a.MaxTokens, a.Enabled, a.Description,
	).Scan(
		&a.ID, &a.Name, &a.AgentType, &a.ModelID, &a.SystemPrompt,
		&a.Temperature, &a.MaxTokens, &a.Enabled, &a.Description,
		&a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

// UpdateAgent обновляет конфигурацию агента.
func (s *Store) UpdateAgent(ctx context.Context, a Agent) error {
	var modelID *string
	if a.ModelID != "" {
		modelID = &a.ModelID
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE ai_agents SET
			name=$1, agent_type=$2, model_id=$3, system_prompt=$4,
			temperature=$5, max_tokens=$6, enabled=$7, description=$8,
			updated_at=NOW()
		WHERE id=$9`,
		a.Name, a.AgentType, modelID, a.SystemPrompt,
		a.Temperature, a.MaxTokens, a.Enabled, a.Description, a.ID)
	return err
}

// DeleteAgent удаляет агента.
func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ai_agents WHERE id=$1`, id)
	return err
}

// SeedQwenModels создаёт стандартные модели Qwen, если их ещё нет.
// Вызывается при старте сервиса, если задан QWEN_API_KEY.
func (s *Store) SeedQwenModels(ctx context.Context, apiKey string) error {
	count, err := s.CountModelsByProvider(ctx, ProviderAlibabaCloud)
	if err != nil {
		return fmt.Errorf("проверка существующих Qwen-моделей: %w", err)
	}
	if count > 0 {
		return nil // уже есть, не дублируем
	}

	baseURL := ProviderAlibabaCloud.DefaultBaseURL()

	models := []Model{
		{
			Name: "Qwen Max", Provider: ProviderAlibabaCloud,
			ModelID: "qwen-max", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Флагманская модель Qwen — наивысшее качество рассуждений",
		},
		{
			Name: "Qwen Plus", Provider: ProviderAlibabaCloud,
			ModelID: "qwen-plus", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Баланс качества и скорости — рекомендуется для продакшна",
		},
		{
			Name: "Qwen Turbo", Provider: ProviderAlibabaCloud,
			ModelID: "qwen-turbo", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Быстрая и экономичная модель для простых задач",
		},
		{
			Name: "Qwen Long", Provider: ProviderAlibabaCloud,
			ModelID: "qwen-long", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 16384, Temperature: 0.7, Enabled: true,
			Description: "Длинный контекст до 1M токенов — для анализа больших документов",
		},
		{
			Name: "Qwen 2.5 72B Instruct", Provider: ProviderAlibabaCloud,
			ModelID: "qwen2.5-72b-instruct", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Qwen 2.5 на 72B параметрах — сильное следование инструкциям",
		},
		{
			Name: "Qwen 2.5 32B Instruct", Provider: ProviderAlibabaCloud,
			ModelID: "qwen2.5-32b-instruct", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Qwen 2.5 на 32B — хорошее качество при меньших затратах",
		},
		{
			Name: "Qwen 2.5 14B Instruct", Provider: ProviderAlibabaCloud,
			ModelID: "qwen2.5-14b-instruct", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Qwen 2.5 на 14B — эффективная модель среднего размера",
		},
		{
			Name: "Qwen 2.5 7B Instruct", Provider: ProviderAlibabaCloud,
			ModelID: "qwen2.5-7b-instruct", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Qwen 2.5 на 7B — лёгкая и быстрая модель",
		},
		{
			Name: "Qwen VL Plus", Provider: ProviderAlibabaCloud,
			ModelID: "qwen-vl-plus", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 4096, Temperature: 0.7, Enabled: true,
			Description: "Мультимодальная модель для работы с изображениями и текстом",
		},
		{
			Name: "Qwen Max Latest", Provider: ProviderAlibabaCloud,
			ModelID: "qwen-max-latest", BaseURL: baseURL, APIKey: apiKey,
			MaxTokens: 8192, Temperature: 0.7, Enabled: true,
			Description: "Последняя версия Qwen Max — всегда актуальный релиз",
		},
	}

	for _, m := range models {
		if _, err := s.CreateModel(ctx, m); err != nil {
			return fmt.Errorf("создание модели %s: %w", m.Name, err)
		}
	}
	return nil
}

// SeedDefaultAgents создаёт стандартных агентов с дефолтными промптами,
// если агентов ещё нет.
func (s *Store) SeedDefaultAgents(ctx context.Context, defaultModelID string) error {
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return err
	}
	if len(agents) > 0 {
		return nil
	}

	defaults := []Agent{
		{
			Name: "Консультант Сколково", AgentType: AgentConsultant,
			ModelID: defaultModelID,
			SystemPrompt: DefaultSystemPrompts[AgentConsultant],
			Temperature: 0.7, MaxTokens: 4096, Enabled: true,
			Description: "Отвечает на вопросы по документам и процедурам Сколково",
		},
		{
			Name: "Валидатор документов", AgentType: AgentValidator,
			ModelID: defaultModelID,
			SystemPrompt: DefaultSystemPrompts[AgentValidator],
			Temperature: 0.3, MaxTokens: 4096, Enabled: true,
			Description: "Проверяет документы на соответствие требованиям Сколково",
		},
		{
			Name: "Монитор изменений", AgentType: AgentMonitor,
			ModelID: defaultModelID,
			SystemPrompt: DefaultSystemPrompts[AgentMonitor],
			Temperature: 0.5, MaxTokens: 2048, Enabled: true,
			Description: "Анализирует изменения в нормативных документах",
		},
		{
			Name: "Координатор резидентства", AgentType: AgentCoordinator,
			ModelID: defaultModelID,
			SystemPrompt: DefaultSystemPrompts[AgentCoordinator],
			Temperature: 0.6, MaxTokens: 2048, Enabled: true,
			Description: "Рекомендует следующие шаги и помогает с планированием",
		},
	}

	for _, a := range defaults {
		if _, err := s.CreateAgent(ctx, a); err != nil {
			return fmt.Errorf("создание агента %s: %w", a.Name, err)
		}
	}
	return nil
}
