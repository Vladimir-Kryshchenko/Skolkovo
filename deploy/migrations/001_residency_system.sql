-- Migration 001: Residency System Tables
-- Created: 2026-05-29
-- Description: Core tables for managing tenant-based residency lifecycle, checklists, deadlines, and documents.

BEGIN;

-- ---------------------------------------------------------------------------
-- Tenants
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    api_key    TEXT        NOT NULL UNIQUE,
    settings   JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    active     BOOLEAN     NOT NULL DEFAULT true
);

COMMENT ON COLUMN tenants.api_key IS 'API key for tenant authentication';
COMMENT ON COLUMN tenants.settings IS 'Tenant-specific configuration stored as JSON';
COMMENT ON COLUMN tenants.active   IS 'Whether the tenant is currently active';

-- ---------------------------------------------------------------------------
-- Clients
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS clients (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT        NOT NULL,
    inn              TEXT        NOT NULL UNIQUE,
    contact_email    TEXT,
    contact_phone    TEXT,
    residency_stage  TEXT,
    tenant_id        UUID        NOT NULL REFERENCES tenants(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN clients.inn              IS 'Tax identification number (unique per client)';
COMMENT ON COLUMN clients.residency_stage  IS 'Current stage in the residency lifecycle';
COMMENT ON COLUMN clients.tenant_id        IS 'Owning tenant for multi-tenant isolation';

CREATE INDEX IF NOT EXISTS idx_clients_tenant_id ON clients(tenant_id);

-- ---------------------------------------------------------------------------
-- Stage Transitions
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS stage_transitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id       UUID        NOT NULL REFERENCES clients(id),
    from_stage      TEXT        NOT NULL,
    to_stage        TEXT        NOT NULL,
    transitioned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    notes           TEXT
);

COMMENT ON COLUMN stage_transitions.from_stage      IS 'Previous residency stage';
COMMENT ON COLUMN stage_transitions.to_stage        IS 'New residency stage after transition';
COMMENT ON COLUMN stage_transitions.transitioned_at IS 'Timestamp when the transition occurred';

CREATE INDEX IF NOT EXISTS idx_stage_transitions_client_id ON stage_transitions(client_id);

-- ---------------------------------------------------------------------------
-- Checklists (templates)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS checklists (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title          TEXT        NOT NULL,
    procedure_type TEXT        NOT NULL CHECK (procedure_type IN ('entry', 'reporting', 'extension', 'exit')),
    steps          JSONB       NOT NULL DEFAULT '[]',
    version        INT         NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN checklists.procedure_type IS 'Type of procedure this checklist covers';
COMMENT ON COLUMN checklists.steps          IS 'Array of checklist step definitions stored as JSON';
COMMENT ON COLUMN checklists.version        IS 'Version number of the checklist template';

-- ---------------------------------------------------------------------------
-- Client Checklists (instances)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS client_checklists (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id    UUID        NOT NULL REFERENCES clients(id),
    checklist_id UUID        NOT NULL REFERENCES checklists(id),
    status       TEXT        NOT NULL DEFAULT 'not_started' CHECK (status IN ('not_started', 'in_progress', 'completed')),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

COMMENT ON COLUMN client_checklists.status IS 'Overall progress status of the checklist for this client';

CREATE INDEX IF NOT EXISTS idx_client_checklists_client_id    ON client_checklists(client_id);
CREATE INDEX IF NOT EXISTS idx_client_checklists_checklist_id ON client_checklists(checklist_id);

-- ---------------------------------------------------------------------------
-- Checklist Step Statuses
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS checklist_step_statuses (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_checklist_id UUID        NOT NULL REFERENCES client_checklists(id),
    step_index          INT         NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'done', 'skipped')),
    completed_at        TIMESTAMPTZ,
    notes               TEXT
);

COMMENT ON COLUMN checklist_step_statuses.step_index   IS 'Zero-based index of the step within the checklist';
COMMENT ON COLUMN checklist_step_statuses.status       IS 'Current status of this individual step';
COMMENT ON COLUMN checklist_step_statuses.completed_at IS 'Timestamp when the step was marked done or skipped';

CREATE INDEX IF NOT EXISTS idx_checklist_step_statuses_client_checklist_id
    ON checklist_step_statuses(client_checklist_id);

-- ---------------------------------------------------------------------------
-- Deadlines
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS deadlines (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id          UUID        NOT NULL REFERENCES clients(id),
    title              TEXT        NOT NULL,
    due_date           DATE        NOT NULL,
    type               TEXT        NOT NULL CHECK (type IN ('reporting', 'extension', 'application', 'document_submission')),
    status             TEXT        NOT NULL DEFAULT 'upcoming' CHECK (status IN ('upcoming', 'overdue', 'completed')),
    notification_sent  BOOLEAN     NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN deadlines.due_date          IS 'Deadline date by which the item must be completed';
COMMENT ON COLUMN deadlines.type              IS 'Category of the deadline';
COMMENT ON COLUMN deadlines.status            IS 'Current state: upcoming, overdue, or completed';
COMMENT ON COLUMN deadlines.notification_sent IS 'Whether a reminder notification has already been sent';

CREATE INDEX IF NOT EXISTS idx_deadlines_client_id ON deadlines(client_id);
CREATE INDEX IF NOT EXISTS idx_deadlines_due_date  ON deadlines(due_date);

-- ---------------------------------------------------------------------------
-- Document Templates
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS document_templates (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        NOT NULL,
    type           TEXT        NOT NULL,
    template_file  TEXT        NOT NULL,
    variables      JSONB       NOT NULL DEFAULT '{}',
    version        INT         NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN document_templates.type          IS 'Document type/category identifier';
COMMENT ON COLUMN document_templates.template_file IS 'Path or identifier of the template file';
COMMENT ON COLUMN document_templates.variables     IS 'Template variable definitions stored as JSON';
COMMENT ON COLUMN document_templates.version       IS 'Version number of the template';

-- ---------------------------------------------------------------------------
-- Client Documents
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS client_documents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id     UUID        NOT NULL REFERENCES clients(id),
    document_id   UUID,
    role          TEXT        NOT NULL DEFAULT 'required' CHECK (role IN ('required', 'optional', 'submitted')),
    status        TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'submitted', 'approved', 'rejected')),
    submitted_at  TIMESTAMPTZ,
    tenant_id     TEXT
);

COMMENT ON COLUMN client_documents.document_id IS 'Optional reference to a document template or external document';
COMMENT ON COLUMN client_documents.role        IS 'Role of this document in the process: required, optional, or submitted';
COMMENT ON COLUMN client_documents.status      IS 'Review status of the submitted document';
COMMENT ON COLUMN client_documents.tenant_id   IS 'External tenant reference (nullable, no FK constraint)';

CREATE INDEX IF NOT EXISTS idx_client_documents_client_id ON client_documents(client_id);
CREATE INDEX IF NOT EXISTS idx_client_documents_tenant_id ON client_documents(tenant_id);

COMMIT;
