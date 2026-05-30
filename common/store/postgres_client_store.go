package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/common/model"
)

// PostgresClientStore — реализация всех хранилищ для новых моделей
// (клиенты, чек-листы, дедлайны, шаблоны, тенанты) поверх PostgreSQL.
type PostgresClientStore struct {
	db *pgxpool.Pool
}

// NewPostgresClientStore создаёт хранилище, принимая готовый пул подключений.
func NewPostgresClientStore(db *pgxpool.Pool) *PostgresClientStore {
	return &PostgresClientStore{db: db}
}

// ============================================================================
// ClientStore
// ============================================================================

func (s *PostgresClientStore) CreateClient(ctx context.Context, client *model.Client) error {
	if err := validateClient(client); err != nil {
		return err
	}
	if client.ID == "" {
		client.ID = uuid.New().String()
	}
	now := time.Now()
	client.CreatedAt = now
	client.UpdatedAt = now

	_, err := s.db.Exec(ctx, `
INSERT INTO clients (id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		client.ID, client.Name, client.INN, nullStrPtr(client.ContactEmail),
		nullStrPtr(client.ContactPhone), string(client.ResidencyStage),
		client.TenantID, client.CreatedAt, client.UpdatedAt)
	return err
}

func (s *PostgresClientStore) GetClient(ctx context.Context, id string) (*model.Client, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at
FROM clients WHERE id = $1`, id)
	return scanClient(row)
}

func (s *PostgresClientStore) GetClientByINN(ctx context.Context, inn string) (*model.Client, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at
FROM clients WHERE inn = $1`, inn)
	return scanClient(row)
}

func (s *PostgresClientStore) UpdateClient(ctx context.Context, client *model.Client) error {
	if err := validateClient(client); err != nil {
		return err
	}
	client.UpdatedAt = time.Now()

	tag, err := s.db.Exec(ctx, `
UPDATE clients SET name=$2, inn=$3, contact_email=$4, contact_phone=$5,
       residency_stage=$6, tenant_id=$7, updated_at=$8
WHERE id = $1`,
		client.ID, client.Name, client.INN, nullStrPtr(client.ContactEmail),
		nullStrPtr(client.ContactPhone), string(client.ResidencyStage),
		client.TenantID, client.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresClientStore) DeleteClient(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM clients WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresClientStore) ListClients(ctx context.Context, tenantID string, stage model.ResidencyStage) ([]*model.Client, error) {
	var rows pgx.Rows
	var err error

	if tenantID == "" && stage == "" {
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at
FROM clients
ORDER BY created_at DESC`)
	} else if tenantID == "" {
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at
FROM clients WHERE residency_stage = $1
ORDER BY created_at DESC`, string(stage))
	} else if stage == "" {
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at
FROM clients WHERE tenant_id = $1
ORDER BY created_at DESC`, tenantID)
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, contact_email, contact_phone, residency_stage, tenant_id, created_at, updated_at
FROM clients WHERE tenant_id = $1 AND residency_stage = $2
ORDER BY created_at DESC`, tenantID, string(stage))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) AddStageTransition(ctx context.Context, t *model.StageTransition) error {
	if err := validateStageTransition(t); err != nil {
		return err
	}
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.TransitionedAt.IsZero() {
		t.TransitionedAt = time.Now()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO stage_transitions (id, client_id, from_stage, to_stage, transitioned_at, notes)
VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.ClientID, string(t.FromStage), string(t.ToStage),
		t.TransitionedAt, t.Notes)
	return err
}

func (s *PostgresClientStore) GetStageHistory(ctx context.Context, clientID string) ([]*model.StageTransition, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, client_id, from_stage, to_stage, transitioned_at, notes
FROM stage_transitions WHERE client_id = $1
ORDER BY transitioned_at ASC`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.StageTransition
	for rows.Next() {
		t, err := scanTransition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ============================================================================
// ChecklistStore
// ============================================================================

func (s *PostgresClientStore) CreateChecklist(ctx context.Context, checklist *model.Checklist) error {
	if err := validateChecklist(checklist); err != nil {
		return err
	}
	if checklist.ID == "" {
		checklist.ID = uuid.New().String()
	}
	if checklist.CreatedAt.IsZero() {
		checklist.CreatedAt = time.Now()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO checklists (id, title, procedure_type, steps, version, created_at)
VALUES ($1, $2, $3, $4, $5, $6)`,
		checklist.ID, checklist.Title, string(checklist.ProcedureType),
		checklist.Steps, checklist.Version, checklist.CreatedAt)
	return err
}

func (s *PostgresClientStore) GetChecklist(ctx context.Context, id string) (*model.Checklist, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, title, procedure_type, steps, version, created_at
FROM checklists WHERE id = $1`, id)
	return scanChecklist(row)
}

func (s *PostgresClientStore) ListChecklists(ctx context.Context, procedureType model.ChecklistType) ([]*model.Checklist, error) {
	var rows pgx.Rows
	var err error

	if procedureType == "" {
		rows, err = s.db.Query(ctx, `
SELECT id, title, procedure_type, steps, version, created_at
FROM checklists ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, title, procedure_type, steps, version, created_at
FROM checklists WHERE procedure_type = $1
ORDER BY created_at DESC`, string(procedureType))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Checklist
	for rows.Next() {
		c, err := scanChecklist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) CreateClientChecklist(ctx context.Context, cc *model.ClientChecklist) error {
	if err := validateClientChecklist(cc); err != nil {
		return err
	}
	if cc.ID == "" {
		cc.ID = uuid.New().String()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO client_checklists (id, client_id, checklist_id, status, started_at, completed_at)
VALUES ($1, $2, $3, $4, $5, $6)`,
		cc.ID, cc.ClientID, cc.ChecklistID, string(cc.Status),
		cc.StartedAt, cc.CompletedAt)
	return err
}

func (s *PostgresClientStore) GetClientChecklist(ctx context.Context, id string) (*model.ClientChecklist, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, client_id, checklist_id, status, started_at, completed_at
FROM client_checklists WHERE id = $1`, id)
	return scanClientChecklist(row)
}

func (s *PostgresClientStore) GetClientChecklists(ctx context.Context, clientID string) ([]*model.ClientChecklist, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, client_id, checklist_id, status, started_at, completed_at
FROM client_checklists WHERE client_id = $1
ORDER BY started_at DESC`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.ClientChecklist
	for rows.Next() {
		cc, err := scanClientChecklist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) UpdateClientChecklist(ctx context.Context, cc *model.ClientChecklist) error {
	tag, err := s.db.Exec(ctx, `
UPDATE client_checklists SET status=$2, started_at=$3, completed_at=$4
WHERE id = $1`,
		cc.ID, string(cc.Status), cc.StartedAt, cc.CompletedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresClientStore) CreateStepStatus(ctx context.Context, ss *model.ChecklistStepStatus) error {
	if ss.ID == "" {
		ss.ID = uuid.New().String()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO checklist_step_statuses (id, client_checklist_id, step_index, status, completed_at, notes)
VALUES ($1, $2, $3, $4, $5, $6)`,
		ss.ID, ss.ClientChecklistID, ss.StepIndex, string(ss.Status),
		ss.CompletedAt, ss.Notes)
	return err
}

func (s *PostgresClientStore) UpdateStepStatus(ctx context.Context, id string, status model.StepStatus, notes string) error {
	var completedAt *time.Time
	if status == model.StepDone || status == model.StepSkipped {
		now := time.Now()
		completedAt = &now
	}

	tag, err := s.db.Exec(ctx, `
UPDATE checklist_step_statuses SET status=$2, notes=$3, completed_at=$4
WHERE id = $1`,
		id, string(status), notes, completedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresClientStore) GetStepStatuses(ctx context.Context, clientChecklistID string) ([]*model.ChecklistStepStatus, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, client_checklist_id, step_index, status, completed_at, notes
FROM checklist_step_statuses WHERE client_checklist_id = $1
ORDER BY step_index ASC`, clientChecklistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.ChecklistStepStatus
	for rows.Next() {
		ss, err := scanStepStatus(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// ============================================================================
// DeadlineStore
// ============================================================================

func (s *PostgresClientStore) CreateDeadline(ctx context.Context, deadline *model.Deadline) error {
	if err := validateDeadline(deadline, time.Now()); err != nil {
		return err
	}
	if deadline.ID == "" {
		deadline.ID = uuid.New().String()
	}
	if deadline.CreatedAt.IsZero() {
		deadline.CreatedAt = time.Now()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO deadlines (id, client_id, title, due_date, type, status, notification_sent, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		deadline.ID, deadline.ClientID, deadline.Title, deadline.DueDate,
		string(deadline.Type), string(deadline.Status), deadline.NotificationSent,
		deadline.CreatedAt)
	return err
}

func (s *PostgresClientStore) GetDeadline(ctx context.Context, id string) (*model.Deadline, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, client_id, title, due_date, type, status, notification_sent, created_at
FROM deadlines WHERE id = $1`, id)
	return scanDeadline(row)
}

func (s *PostgresClientStore) UpdateDeadline(ctx context.Context, deadline *model.Deadline) error {
	tag, err := s.db.Exec(ctx, `
UPDATE deadlines SET client_id=$2, title=$3, due_date=$4, type=$5,
       status=$6, notification_sent=$7
WHERE id = $1`,
		deadline.ID, deadline.ClientID, deadline.Title, deadline.DueDate,
		string(deadline.Type), string(deadline.Status), deadline.NotificationSent)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresClientStore) ListDeadlines(ctx context.Context, clientID string, daysAhead int) ([]*model.Deadline, error) {
	var rows pgx.Rows
	var err error

	if clientID == "" && daysAhead <= 0 {
		rows, err = s.db.Query(ctx, `
SELECT id, client_id, title, due_date, type, status, notification_sent, created_at
FROM deadlines
ORDER BY due_date ASC`)
	} else if clientID == "" {
		rows, err = s.db.Query(ctx, `
SELECT id, client_id, title, due_date, type, status, notification_sent, created_at
FROM deadlines WHERE due_date <= now() + ($1 * interval '1 day')
ORDER BY due_date ASC`, daysAhead)
	} else if daysAhead <= 0 {
		rows, err = s.db.Query(ctx, `
SELECT id, client_id, title, due_date, type, status, notification_sent, created_at
FROM deadlines WHERE client_id = $1
ORDER BY due_date ASC`, clientID)
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, client_id, title, due_date, type, status, notification_sent, created_at
FROM deadlines WHERE client_id = $1 AND due_date <= now() + ($2 * interval '1 day')
ORDER BY due_date ASC`, clientID, daysAhead)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Deadline
	for rows.Next() {
		d, err := scanDeadline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) ListOverdueDeadlines(ctx context.Context) ([]*model.Deadline, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, client_id, title, due_date, type, status, notification_sent, created_at
FROM deadlines WHERE status = 'overdue' OR (status != 'completed' AND due_date < now())
ORDER BY due_date ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Deadline
	for rows.Next() {
		d, err := scanDeadline(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) MarkNotificationSent(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `
UPDATE deadlines SET notification_sent = true WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// TemplateStore
// ============================================================================

func (s *PostgresClientStore) CreateTemplate(ctx context.Context, tmpl *model.DocumentTemplate) error {
	if err := validateTemplate(tmpl); err != nil {
		return err
	}
	if tmpl.ID == "" {
		tmpl.ID = uuid.New().String()
	}
	if tmpl.CreatedAt.IsZero() {
		tmpl.CreatedAt = time.Now()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO document_templates (id, name, type, template_file, variables, version, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tmpl.ID, tmpl.Name, tmpl.Type, tmpl.TemplateFile,
		tmpl.Variables, tmpl.Version, tmpl.CreatedAt)
	return err
}

func (s *PostgresClientStore) GetTemplate(ctx context.Context, id string) (*model.DocumentTemplate, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, name, type, template_file, variables, version, created_at
FROM document_templates WHERE id = $1`, id)
	return scanTemplate(row)
}

func (s *PostgresClientStore) ListTemplates(ctx context.Context, templateType string) ([]*model.DocumentTemplate, error) {
	var rows pgx.Rows
	var err error

	if templateType == "" {
		rows, err = s.db.Query(ctx, `
SELECT id, name, type, template_file, variables, version, created_at
FROM document_templates ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, name, type, template_file, variables, version, created_at
FROM document_templates WHERE type = $1
ORDER BY created_at DESC`, templateType)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.DocumentTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ============================================================================
// ClientDocumentStore
// ============================================================================

func (s *PostgresClientStore) LinkDocument(ctx context.Context, cd *model.ClientDocument) error {
	if err := validateClientDocument(cd); err != nil {
		return err
	}
	if cd.ID == "" {
		cd.ID = uuid.New().String()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO client_documents (id, client_id, document_id, role, status, submitted_at)
VALUES ($1, $2, $3, $4, $5, $6)`,
		cd.ID, cd.ClientID, cd.DocumentID, string(cd.Role),
		string(cd.Status), cd.SubmittedAt)
	return err
}

func (s *PostgresClientStore) GetClientDocument(ctx context.Context, id string) (*model.ClientDocument, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, client_id, document_id, role, status, submitted_at
FROM client_documents WHERE id = $1`, id)
	return scanClientDocument(row)
}

func (s *PostgresClientStore) ListClientDocuments(ctx context.Context, clientID string) ([]*model.ClientDocument, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, client_id, document_id, role, status, submitted_at
FROM client_documents WHERE client_id = $1
ORDER BY submitted_at DESC`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.ClientDocument
	for rows.Next() {
		cd, err := scanClientDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cd)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) UpdateClientDocument(ctx context.Context, cd *model.ClientDocument) error {
	tag, err := s.db.Exec(ctx, `
UPDATE client_documents SET role=$2, status=$3, submitted_at=$4
WHERE id = $1`,
		cd.ID, string(cd.Role), string(cd.Status), cd.SubmittedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// TenantStore
// ============================================================================

func (s *PostgresClientStore) CreateTenant(ctx context.Context, tenant *model.Tenant) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	if tenant.ID == "" {
		tenant.ID = uuid.New().String()
	}
	if tenant.CreatedAt.IsZero() {
		tenant.CreatedAt = time.Now()
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO tenants (id, name, api_key, settings, created_at, active)
VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.ID, tenant.Name, tenant.APIKey, tenant.Settings,
		tenant.CreatedAt, tenant.Active)
	return err
}

func (s *PostgresClientStore) GetTenant(ctx context.Context, id string) (*model.Tenant, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, name, api_key, settings, created_at, active
FROM tenants WHERE id = $1`, id)
	return scanTenant(row)
}

func (s *PostgresClientStore) GetTenantByAPIKey(ctx context.Context, apiKey string) (*model.Tenant, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, name, api_key, settings, created_at, active
FROM tenants WHERE api_key = $1`, apiKey)
	return scanTenant(row)
}

func (s *PostgresClientStore) ListTenants(ctx context.Context) ([]*model.Tenant, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, name, api_key, settings, created_at, active
FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *PostgresClientStore) UpdateTenant(ctx context.Context, tenant *model.Tenant) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `
UPDATE tenants SET name=$2, api_key=$3, settings=$4, active=$5
WHERE id = $1`,
		tenant.ID, tenant.Name, tenant.APIKey, tenant.Settings, tenant.Active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================================
// Scan-функции
// ============================================================================

func scanClient(r row) (*model.Client, error) {
	var c model.Client
	var email, phone, stage *string
	err := r.Scan(&c.ID, &c.Name, &c.INN, &email, &phone, &stage,
		&c.TenantID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.ContactEmail = deref(email)
	c.ContactPhone = deref(phone)
	if stage != nil {
		c.ResidencyStage = model.ResidencyStage(*stage)
	}
	return &c, nil
}

func scanTransition(r row) (*model.StageTransition, error) {
	var t model.StageTransition
	var fromStage, toStage string
	var notes *string
	err := r.Scan(&t.ID, &t.ClientID, &fromStage, &toStage, &t.TransitionedAt, &notes)
	if err != nil {
		return nil, err
	}
	t.FromStage = model.ResidencyStage(fromStage)
	t.ToStage = model.ResidencyStage(toStage)
	t.Notes = deref(notes)
	return &t, nil
}

func scanChecklist(r row) (*model.Checklist, error) {
	var c model.Checklist
	var steps json.RawMessage
	err := r.Scan(&c.ID, &c.Title, &c.ProcedureType, &steps, &c.Version, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Steps = steps
	return &c, nil
}

func scanClientChecklist(r row) (*model.ClientChecklist, error) {
	var cc model.ClientChecklist
	var status string
	err := r.Scan(&cc.ID, &cc.ClientID, &cc.ChecklistID, &status, &cc.StartedAt, &cc.CompletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	cc.Status = model.ChecklistStatus(status)
	return &cc, nil
}

func scanStepStatus(r row) (*model.ChecklistStepStatus, error) {
	var ss model.ChecklistStepStatus
	var status string
	var notes *string
	err := r.Scan(&ss.ID, &ss.ClientChecklistID, &ss.StepIndex, &status, &ss.CompletedAt, &notes)
	if err != nil {
		return nil, err
	}
	ss.Status = model.StepStatus(status)
	ss.Notes = deref(notes)
	return &ss, nil
}

func scanDeadline(r row) (*model.Deadline, error) {
	var d model.Deadline
	var dType, dStatus string
	err := r.Scan(&d.ID, &d.ClientID, &d.Title, &d.DueDate, &dType, &dStatus,
		&d.NotificationSent, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d.Type = model.DeadlineType(dType)
	d.Status = model.DeadlineStatus(dStatus)
	return &d, nil
}

func scanTemplate(r row) (*model.DocumentTemplate, error) {
	var t model.DocumentTemplate
	var variables json.RawMessage
	err := r.Scan(&t.ID, &t.Name, &t.Type, &t.TemplateFile, &variables, &t.Version, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.Variables = variables
	return &t, nil
}

func scanClientDocument(r row) (*model.ClientDocument, error) {
	var cd model.ClientDocument
	var role, status string
	var docID *string
	err := r.Scan(&cd.ID, &cd.ClientID, &docID, &role, &status, &cd.SubmittedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	cd.DocumentID = deref(docID)
	cd.Role = model.ClientDocRole(role)
	cd.Status = model.ClientDocStatus(status)
	return &cd, nil
}

func scanTenant(r row) (*model.Tenant, error) {
	var t model.Tenant
	var settings json.RawMessage
	err := r.Scan(&t.ID, &t.Name, &t.APIKey, &settings, &t.CreatedAt, &t.Active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.Settings = settings
	return &t, nil
}

// ============================================================================
// Утилиты
// ============================================================================

func nullStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
