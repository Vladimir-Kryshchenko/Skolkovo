-- Migration 002: Extended Sources
-- Created: 2026-05-29
-- Description: Tables for external data sources — events, contests, FAQ, Telegram posts, residents, and document links.

BEGIN;

-- ---------------------------------------------------------------------------
-- Events
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT        NOT NULL,
    description  TEXT,
    event_date   DATE        NOT NULL,
    event_end_date DATE,
    location     TEXT,
    source_url   TEXT,
    status       TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'past', 'cancelled')),
    category     TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN events.event_date     IS 'Start date of the event';
COMMENT ON COLUMN events.event_end_date IS 'End date of the event (null for single-day events)';
COMMENT ON COLUMN events.source_url     IS 'URL of the original source';
COMMENT ON COLUMN events.category       IS 'Event category for grouping and filtering';

CREATE INDEX IF NOT EXISTS idx_events_event_date  ON events(event_date);
CREATE INDEX IF NOT EXISTS idx_events_status       ON events(status);
CREATE INDEX IF NOT EXISTS idx_events_category     ON events(category);

-- ---------------------------------------------------------------------------
-- Contests
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS contests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title         TEXT        NOT NULL,
    description   TEXT,
    start_date    DATE        NOT NULL,
    end_date      DATE        NOT NULL,
    requirements  TEXT,
    prize         TEXT,
    source_url    TEXT,
    status        TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed', 'winner_selected')),
    category      TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN contests.requirements IS 'Eligibility requirements and rules';
COMMENT ON COLUMN contests.prize        IS 'Prize description or amount';
COMMENT ON COLUMN contests.source_url   IS 'URL of the original source';
COMMENT ON COLUMN contests.category     IS 'Contest category for grouping and filtering';

CREATE INDEX IF NOT EXISTS idx_contests_start_date ON contests(start_date);
CREATE INDEX IF NOT EXISTS idx_contests_end_date   ON contests(end_date);
CREATE INDEX IF NOT EXISTS idx_contests_status     ON contests(status);
CREATE INDEX IF NOT EXISTS idx_contests_category   ON contests(category);

-- ---------------------------------------------------------------------------
-- FAQ Items
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS faq_items (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question   TEXT        NOT NULL,
    answer     TEXT        NOT NULL,
    category   TEXT,
    source_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN faq_items.category   IS 'FAQ category for grouping';
COMMENT ON COLUMN faq_items.source_url IS 'URL of the original source';

CREATE INDEX IF NOT EXISTS idx_faq_items_category ON faq_items(category);

-- ---------------------------------------------------------------------------
-- Telegram Posts
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS telegram_posts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel      TEXT        NOT NULL,
    text         TEXT        NOT NULL,
    published_at TIMESTAMPTZ,
    source_url   TEXT,
    media_urls   JSONB       NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN telegram_posts.channel      IS 'Telegram channel name or ID';
COMMENT ON COLUMN telegram_posts.published_at IS 'Original publication timestamp from Telegram';
COMMENT ON COLUMN telegram_posts.source_url   IS 'URL of the original Telegram post';
COMMENT ON COLUMN telegram_posts.media_urls   IS 'Array of media attachment URLs stored as JSON';

CREATE INDEX IF NOT EXISTS idx_telegram_posts_channel     ON telegram_posts(channel);
CREATE INDEX IF NOT EXISTS idx_telegram_posts_published_at ON telegram_posts(published_at);

-- ---------------------------------------------------------------------------
-- Residents
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS residents (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    inn        TEXT,
    industry   TEXT,
    join_date  DATE,
    status     TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    source_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN residents.inn        IS 'Tax identification number';
COMMENT ON COLUMN residents.industry   IS 'Industry or sector';
COMMENT ON COLUMN residents.join_date  IS 'Date the resident joined the program';
COMMENT ON COLUMN residents.source_url IS 'URL of the original source';

CREATE INDEX IF NOT EXISTS idx_residents_inn     ON residents(inn);
CREATE INDEX IF NOT EXISTS idx_residents_status  ON residents(status);
CREATE INDEX IF NOT EXISTS idx_residents_industry ON residents(industry);

-- ---------------------------------------------------------------------------
-- Document Links (self-referencing links between documents)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS document_links (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id  UUID        NOT NULL REFERENCES client_documents(id),
    target_id  UUID        NOT NULL REFERENCES client_documents(id),
    link_type  TEXT        NOT NULL CHECK (link_type IN ('references', 'supersedes', 'related')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN document_links.source_id IS 'The document that holds the link';
COMMENT ON COLUMN document_links.target_id IS 'The document being linked to';
COMMENT ON COLUMN document_links.link_type IS 'Nature of the relationship: references, supersedes, or related';

CREATE INDEX IF NOT EXISTS idx_document_links_source_id ON document_links(source_id);
CREATE INDEX IF NOT EXISTS idx_document_links_target_id ON document_links(target_id);

COMMIT;
