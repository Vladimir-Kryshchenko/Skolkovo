package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"baza-skolkovo/src/common/model"
)

// PostgresSourceStore — реализация хранилищ для расширенных источников
// (мероприятия, конкурсы, FAQ, Telegram, резиденты, связи документов) поверх PostgreSQL.
type PostgresSourceStore struct {
	db *pgxpool.Pool
}

// NewPostgresSourceStore создаёт хранилище, принимая готовый пул подключений.
func NewPostgresSourceStore(db *pgxpool.Pool) *PostgresSourceStore {
	return &PostgresSourceStore{db: db}
}

// ============================================================================
// EventStore
// ============================================================================

func (s *PostgresSourceStore) CreateEvent(ctx context.Context, event *model.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO events (id, title, description, event_date, event_end_date, location, source_url, status, category)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.ID, event.Title, nullStrPtr(event.Description),
		event.EventDate, nullableDate(event.EventEndDate),
		nullStrPtr(event.Location), event.SourceURL,
		string(event.Status), nullStrPtr(event.Category))
	return err
}

func (s *PostgresSourceStore) GetEvent(ctx context.Context, id string) (*model.Event, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events WHERE id = $1`, id)
	return scanEvent(row)
}

func (s *PostgresSourceStore) ListEvents(ctx context.Context, category string, status model.EventStatus, dateFrom, dateTo *time.Time) ([]*model.Event, error) {
	var rows pgx.Rows
	var err error

	switch {
	case dateFrom != nil && dateTo != nil && category != "" && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events
WHERE event_date >= $1 AND event_date <= $2 AND category = $3 AND status = $4
ORDER BY event_date DESC`, *dateFrom, *dateTo, category, string(status))
	case dateFrom != nil && dateTo != nil && category != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events
WHERE event_date >= $1 AND event_date <= $2 AND category = $3
ORDER BY event_date DESC`, *dateFrom, *dateTo, category)
	case dateFrom != nil && dateTo != nil && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events
WHERE event_date >= $1 AND event_date <= $2 AND status = $3
ORDER BY event_date DESC`, *dateFrom, *dateTo, string(status))
	case dateFrom != nil && dateTo != nil:
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events
WHERE event_date >= $1 AND event_date <= $2
ORDER BY event_date DESC`, *dateFrom, *dateTo)
	case category != "" && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events WHERE category = $1 AND status = $2
ORDER BY event_date DESC`, category, string(status))
	case category != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events WHERE category = $1
ORDER BY event_date DESC`, category)
	case status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events WHERE status = $1
ORDER BY event_date DESC`, string(status))
	default:
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, event_date, event_end_date, location, source_url, status, category, created_at
FROM events ORDER BY event_date DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) UpdateEvent(ctx context.Context, event *model.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
UPDATE events SET title=$2, description=$3, event_date=$4, event_end_date=$5,
       location=$6, source_url=$7, status=$8, category=$9
WHERE id = $1`,
		event.ID, event.Title, nullStrPtr(event.Description),
		event.EventDate, nullableDate(event.EventEndDate),
		nullStrPtr(event.Location), event.SourceURL,
		string(event.Status), nullStrPtr(event.Category))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) DeleteEvent(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM events WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) CountEvents(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM events`).Scan(&count)
	return count, err
}

// ============================================================================
// ContestStore
// ============================================================================

func (s *PostgresSourceStore) CreateContest(ctx context.Context, contest *model.Contest) error {
	if err := validateContest(contest); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO contests (id, title, description, start_date, end_date, requirements, prize, source_url, status, category)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		contest.ID, contest.Title, nullStrPtr(contest.Description),
		contest.StartDate, contest.EndDate,
		nullStrPtr(contest.Requirements), nullStrPtr(contest.Prize),
		contest.SourceURL, string(contest.Status), nullStrPtr(contest.Category))
	return err
}

func (s *PostgresSourceStore) GetContest(ctx context.Context, id string) (*model.Contest, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, title, description, start_date, end_date, requirements, prize, source_url, status, category, created_at
FROM contests WHERE id = $1`, id)
	return scanContest(row)
}

func (s *PostgresSourceStore) ListContests(ctx context.Context, category string, status model.ContestStatus) ([]*model.Contest, error) {
	var rows pgx.Rows
	var err error

	switch {
	case category != "" && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, start_date, end_date, requirements, prize, source_url, status, category, created_at
FROM contests WHERE category = $1 AND status = $2
ORDER BY start_date DESC`, category, string(status))
	case category != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, start_date, end_date, requirements, prize, source_url, status, category, created_at
FROM contests WHERE category = $1
ORDER BY start_date DESC`, category)
	case status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, start_date, end_date, requirements, prize, source_url, status, category, created_at
FROM contests WHERE status = $1
ORDER BY start_date DESC`, string(status))
	default:
		rows, err = s.db.Query(ctx, `
SELECT id, title, description, start_date, end_date, requirements, prize, source_url, status, category, created_at
FROM contests ORDER BY start_date DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Contest
	for rows.Next() {
		c, err := scanContest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) UpdateContest(ctx context.Context, contest *model.Contest) error {
	if err := validateContest(contest); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
UPDATE contests SET title=$2, description=$3, start_date=$4, end_date=$5,
       requirements=$6, prize=$7, source_url=$8, status=$9, category=$10
WHERE id = $1`,
		contest.ID, contest.Title, nullStrPtr(contest.Description),
		contest.StartDate, contest.EndDate,
		nullStrPtr(contest.Requirements), nullStrPtr(contest.Prize),
		contest.SourceURL, string(contest.Status), nullStrPtr(contest.Category))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) DeleteContest(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM contests WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) CountActiveContests(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM contests WHERE status = 'active'`).Scan(&count)
	return count, err
}

// ============================================================================
// FAQStore
// ============================================================================

func (s *PostgresSourceStore) CreateFAQItem(ctx context.Context, item *model.FAQItem) error {
	if err := validateFAQItem(item); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO faq_items (id, question, answer, category, source_url)
VALUES ($1, $2, $3, $4, $5)`,
		item.ID, item.Question, item.Answer,
		nullStrPtr(item.Category), nullStrPtr(item.SourceURL))
	return err
}

func (s *PostgresSourceStore) GetFAQItem(ctx context.Context, id string) (*model.FAQItem, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, question, answer, category, source_url, created_at
FROM faq_items WHERE id = $1`, id)
	return scanFAQItem(row)
}

func (s *PostgresSourceStore) ListFAQItems(ctx context.Context, category string) ([]*model.FAQItem, error) {
	var rows pgx.Rows
	var err error

	if category != "" {
		rows, err = s.db.Query(ctx, `
SELECT id, question, answer, category, source_url, created_at
FROM faq_items WHERE category = $1
ORDER BY created_at DESC`, category)
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, question, answer, category, source_url, created_at
FROM faq_items ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.FAQItem
	for rows.Next() {
		f, err := scanFAQItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) UpdateFAQItem(ctx context.Context, item *model.FAQItem) error {
	if err := validateFAQItem(item); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
UPDATE faq_items SET question=$2, answer=$3, category=$4, source_url=$5
WHERE id = $1`,
		item.ID, item.Question, item.Answer,
		nullStrPtr(item.Category), nullStrPtr(item.SourceURL))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) DeleteFAQItem(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM faq_items WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) CountFAQItems(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM faq_items`).Scan(&count)
	return count, err
}

// ============================================================================
// TelegramStore
// ============================================================================

func (s *PostgresSourceStore) CreateTelegramPost(ctx context.Context, post *model.TelegramPost) error {
	if err := validateTelegramPost(post); err != nil {
		return err
	}

	mediaJSON, err := json.Marshal(post.MediaURLs)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
INSERT INTO telegram_posts (id, channel, text, published_at, source_url, media_urls)
VALUES ($1, $2, $3, $4, $5, $6)`,
		post.ID, post.Channel, post.Text,
		nullableTime(post.PublishedAt), post.SourceURL,
		mediaJSON)
	return err
}

func (s *PostgresSourceStore) ListTelegramPosts(ctx context.Context, channel string, limit int) ([]*model.TelegramPost, error) {
	if limit < 0 {
		return nil, ErrNegativeLimit
	}

	var rows pgx.Rows
	var err error

	if channel != "" {
		rows, err = s.db.Query(ctx, `
SELECT id, channel, text, published_at, source_url, media_urls, created_at
FROM telegram_posts WHERE channel = $1
ORDER BY published_at DESC
LIMIT $2`, channel, limit)
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, channel, text, published_at, source_url, media_urls, created_at
FROM telegram_posts
ORDER BY published_at DESC
LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.TelegramPost
	for rows.Next() {
		p, err := scanTelegramPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) GetLatestPostDate(ctx context.Context, channel string) (*time.Time, error) {
	var t sql.NullTime
	err := s.db.QueryRow(ctx, `
SELECT published_at FROM telegram_posts
WHERE channel = $1 AND published_at IS NOT NULL
ORDER BY published_at DESC LIMIT 1`, channel).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.Valid {
		return &t.Time, nil
	}
	return nil, nil
}

func (s *PostgresSourceStore) CountPosts(ctx context.Context, channel string) (int, error) {
	var count int
	var err error

	if channel != "" {
		err = s.db.QueryRow(ctx, `SELECT count(*) FROM telegram_posts WHERE channel = $1`, channel).Scan(&count)
	} else {
		err = s.db.QueryRow(ctx, `SELECT count(*) FROM telegram_posts`).Scan(&count)
	}
	return count, err
}

// ============================================================================
// ResidentStore
// ============================================================================

func (s *PostgresSourceStore) CreateResident(ctx context.Context, resident *model.Resident) error {
	if err := validateResident(resident); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO residents (id, name, inn, industry, join_date, status, source_url)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		resident.ID, resident.Name, nullStrPtr(resident.INN),
		nullStrPtr(resident.Industry), nullableDate(resident.JoinDate),
		string(resident.Status), resident.SourceURL)
	return err
}

func (s *PostgresSourceStore) GetResident(ctx context.Context, id string) (*model.Resident, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents WHERE id = $1`, id)
	return scanResident(row)
}

func (s *PostgresSourceStore) ListResidents(ctx context.Context, industry string, status model.ResidentStatus, query string) ([]*model.Resident, error) {
	var rows pgx.Rows
	var err error

	switch {
	case industry != "" && status != "" && query != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents
WHERE industry = $1 AND status = $2 AND (name ILIKE $3 OR inn ILIKE $3)
ORDER BY name ASC`, industry, string(status), "%"+query+"%")
	case industry != "" && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents WHERE industry = $1 AND status = $2
ORDER BY name ASC`, industry, string(status))
	case industry != "" && query != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents
WHERE industry = $1 AND (name ILIKE $2 OR inn ILIKE $2)
ORDER BY name ASC`, industry, "%"+query+"%")
	case status != "" && query != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents
WHERE status = $1 AND (name ILIKE $2 OR inn ILIKE $2)
ORDER BY name ASC`, string(status), "%"+query+"%")
	case industry != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents WHERE industry = $1
ORDER BY name ASC`, industry)
	case status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents WHERE status = $1
ORDER BY name ASC`, string(status))
	case query != "":
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents WHERE name ILIKE $1 OR inn ILIKE $1
ORDER BY name ASC`, "%"+query+"%")
	default:
		rows, err = s.db.Query(ctx, `
SELECT id, name, inn, industry, join_date, status, source_url, created_at
FROM residents ORDER BY name ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Resident
	for rows.Next() {
		r, err := scanResident(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) UpdateResident(ctx context.Context, resident *model.Resident) error {
	if err := validateResident(resident); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
UPDATE residents SET name=$2, inn=$3, industry=$4, join_date=$5, status=$6, source_url=$7
WHERE id = $1`,
		resident.ID, resident.Name, nullStrPtr(resident.INN),
		nullStrPtr(resident.Industry), nullableDate(resident.JoinDate),
		string(resident.Status), resident.SourceURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) DeleteResident(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM residents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) CountResidents(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM residents`).Scan(&count)
	return count, err
}

// ============================================================================
// DocumentLinkStore
// ============================================================================

func (s *PostgresSourceStore) CreateDocumentLink(ctx context.Context, link *model.DocumentLink) error {
	if err := validateDocumentLink(link); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO document_links (id, source_id, target_id, link_type)
VALUES ($1, $2, $3, $4)`,
		link.ID, link.SourceID, link.TargetID, string(link.LinkType))
	return err
}

func (s *PostgresSourceStore) GetDocumentLinks(ctx context.Context, documentID string, linkType model.DocumentLinkType) ([]*model.DocumentLink, error) {
	var rows pgx.Rows
	var err error

	if linkType != "" {
		rows, err = s.db.Query(ctx, `
SELECT id, source_id, target_id, link_type, created_at
FROM document_links
WHERE (source_id = $1 OR target_id = $1) AND link_type = $2
ORDER BY created_at DESC`, documentID, string(linkType))
	} else {
		rows, err = s.db.Query(ctx, `
SELECT id, source_id, target_id, link_type, created_at
FROM document_links
WHERE source_id = $1 OR target_id = $1
ORDER BY created_at DESC`, documentID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.DocumentLink
	for rows.Next() {
		l, err := scanDocumentLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) DeleteDocumentLink(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM document_links WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) ListAllLinks(ctx context.Context) ([]*model.DocumentLink, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, source_id, target_id, link_type, created_at
FROM document_links ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.DocumentLink
	for rows.Next() {
		l, err := scanDocumentLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ============================================================================
// PreferenceStore
// ============================================================================

func (s *PostgresSourceStore) CreatePreference(ctx context.Context, pref *model.Preference) error {
	if err := validatePreference(pref); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO preferences (ext_id, title, pref_type, benefit_desc, legal_ref, source_url, content, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		pref.ExtID, pref.Title, string(pref.PrefType),
		nullStrPtr(pref.BenefitDesc), nullStrPtr(pref.LegalRef),
		pref.SourceURL, nullStrPtr(pref.Content), string(pref.Status))
	return err
}

func (s *PostgresSourceStore) GetPreference(ctx context.Context, id string) (*model.Preference, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, ext_id, title, pref_type, benefit_desc, legal_ref, source_url, content, status, fetched_at, updated_at
FROM preferences WHERE id = $1`, id)
	return scanPreference(row)
}

func (s *PostgresSourceStore) ListPreferences(ctx context.Context, prefType string, status string) ([]*model.Preference, error) {
	var rows pgx.Rows
	var err error

	switch {
	case prefType != "" && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, pref_type, benefit_desc, legal_ref, source_url, content, status, fetched_at, updated_at
FROM preferences WHERE pref_type = $1 AND status = $2
ORDER BY updated_at DESC`, prefType, status)
	case prefType != "":
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, pref_type, benefit_desc, legal_ref, source_url, content, status, fetched_at, updated_at
FROM preferences WHERE pref_type = $1
ORDER BY updated_at DESC`, prefType)
	case status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, pref_type, benefit_desc, legal_ref, source_url, content, status, fetched_at, updated_at
FROM preferences WHERE status = $1
ORDER BY updated_at DESC`, status)
	default:
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, pref_type, benefit_desc, legal_ref, source_url, content, status, fetched_at, updated_at
FROM preferences ORDER BY updated_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Preference
	for rows.Next() {
		p, err := scanPreference(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) UpdatePreference(ctx context.Context, pref *model.Preference) error {
	if err := validatePreference(pref); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
UPDATE preferences SET ext_id=$2, title=$3, pref_type=$4, benefit_desc=$5,
       legal_ref=$6, source_url=$7, content=$8, status=$9, updated_at=now()
WHERE id = $1`,
		pref.ID, pref.ExtID, pref.Title, string(pref.PrefType),
		nullStrPtr(pref.BenefitDesc), nullStrPtr(pref.LegalRef),
		pref.SourceURL, nullStrPtr(pref.Content), string(pref.Status))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) DeletePreference(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM preferences WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) CountPreferences(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM preferences`).Scan(&count)
	return count, err
}

// ============================================================================
// NPAStore
// ============================================================================

func (s *PostgresSourceStore) CreateNPA(ctx context.Context, npa *model.NPADocument) error {
	if err := validateNPA(npa); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx, `
INSERT INTO npa_documents (ext_id, title, npa_number, npa_type, issued_by, issued_at, effective_at, source_url, summary, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		npa.ExtID, npa.Title, nullStrPtr(npa.NPANumber), string(npa.NPAType),
		nullStrPtr(npa.IssuedBy), nullableDate(npa.IssuedAt),
		nullableDate(npa.EffectiveAt), npa.SourceURL,
		nullStrPtr(npa.Summary), string(npa.Status))
	return err
}

func (s *PostgresSourceStore) GetNPA(ctx context.Context, id string) (*model.NPADocument, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, ext_id, title, npa_number, npa_type, issued_by, issued_at, effective_at, source_url, summary, status, fetched_at, updated_at
FROM npa_documents WHERE id = $1`, id)
	return scanNPA(row)
}

func (s *PostgresSourceStore) ListNPA(ctx context.Context, npaType string, status string) ([]*model.NPADocument, error) {
	var rows pgx.Rows
	var err error

	switch {
	case npaType != "" && status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, npa_number, npa_type, issued_by, issued_at, effective_at, source_url, summary, status, fetched_at, updated_at
FROM npa_documents WHERE npa_type = $1 AND status = $2
ORDER BY updated_at DESC`, npaType, status)
	case npaType != "":
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, npa_number, npa_type, issued_by, issued_at, effective_at, source_url, summary, status, fetched_at, updated_at
FROM npa_documents WHERE npa_type = $1
ORDER BY updated_at DESC`, npaType)
	case status != "":
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, npa_number, npa_type, issued_by, issued_at, effective_at, source_url, summary, status, fetched_at, updated_at
FROM npa_documents WHERE status = $1
ORDER BY updated_at DESC`, status)
	default:
		rows, err = s.db.Query(ctx, `
SELECT id, ext_id, title, npa_number, npa_type, issued_by, issued_at, effective_at, source_url, summary, status, fetched_at, updated_at
FROM npa_documents ORDER BY updated_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.NPADocument
	for rows.Next() {
		n, err := scanNPA(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *PostgresSourceStore) UpdateNPA(ctx context.Context, npa *model.NPADocument) error {
	if err := validateNPA(npa); err != nil {
		return err
	}

	tag, err := s.db.Exec(ctx, `
UPDATE npa_documents SET ext_id=$2, title=$3, npa_number=$4, npa_type=$5,
       issued_by=$6, issued_at=$7, effective_at=$8, source_url=$9, summary=$10,
       status=$11, updated_at=now()
WHERE id = $1`,
		npa.ID, npa.ExtID, npa.Title, nullStrPtr(npa.NPANumber), string(npa.NPAType),
		nullStrPtr(npa.IssuedBy), nullableDate(npa.IssuedAt),
		nullableDate(npa.EffectiveAt), npa.SourceURL,
		nullStrPtr(npa.Summary), string(npa.Status))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) DeleteNPA(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM npa_documents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresSourceStore) CountNPA(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM npa_documents`).Scan(&count)
	return count, err
}

// ============================================================================
// Scan-функции
// ============================================================================

func scanEvent(r row) (*model.Event, error) {
	var e model.Event
	var description, location, category *string
	var eventEndDate sql.NullTime
	var statusStr string
	err := r.Scan(&e.ID, &e.Title, &description, &e.EventDate, &eventEndDate,
		&location, &e.SourceURL, &statusStr, &category, &e.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	e.Description = deref(description)
	if eventEndDate.Valid {
		e.EventEndDate = eventEndDate.Time
	}
	e.Location = deref(location)
	e.Status = model.EventStatus(statusStr)
	e.Category = deref(category)
	return &e, nil
}

func scanContest(r row) (*model.Contest, error) {
	var c model.Contest
	var description, requirements, prize, category *string
	var statusStr string
	err := r.Scan(&c.ID, &c.Title, &description, &c.StartDate, &c.EndDate,
		&requirements, &prize, &c.SourceURL, &statusStr, &category, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Description = deref(description)
	c.Requirements = deref(requirements)
	c.Prize = deref(prize)
	c.Status = model.ContestStatus(statusStr)
	c.Category = deref(category)
	return &c, nil
}

func scanFAQItem(r row) (*model.FAQItem, error) {
	var f model.FAQItem
	var category, sourceURL *string
	err := r.Scan(&f.ID, &f.Question, &f.Answer, &category, &sourceURL, &f.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	f.Category = deref(category)
	f.SourceURL = deref(sourceURL)
	return &f, nil
}

func scanTelegramPost(r row) (*model.TelegramPost, error) {
	var p model.TelegramPost
	var publishedAt sql.NullTime
	var sourceURL *string
	var mediaJSON []byte
	err := r.Scan(&p.ID, &p.Channel, &p.Text, &publishedAt, &sourceURL, &mediaJSON, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if publishedAt.Valid {
		p.PublishedAt = publishedAt.Time
	}
	p.SourceURL = deref(sourceURL)
	if len(mediaJSON) > 0 {
		if err := json.Unmarshal(mediaJSON, &p.MediaURLs); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

func scanResident(r row) (*model.Resident, error) {
	var res model.Resident
	var inn, industry *string
	var joinDate sql.NullTime
	var statusStr string
	err := r.Scan(&res.ID, &res.Name, &inn, &industry, &joinDate, &statusStr, &res.SourceURL, &res.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	res.INN = deref(inn)
	res.Industry = deref(industry)
	if joinDate.Valid {
		res.JoinDate = joinDate.Time
	}
	res.Status = model.ResidentStatus(statusStr)
	return &res, nil
}

func scanDocumentLink(r row) (*model.DocumentLink, error) {
	var l model.DocumentLink
	var linkTypeStr string
	err := r.Scan(&l.ID, &l.SourceID, &l.TargetID, &linkTypeStr, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	l.LinkType = model.DocumentLinkType(linkTypeStr)
	return &l, nil
}

func scanPreference(r row) (*model.Preference, error) {
	var p model.Preference
	var prefTypeStr, benefitDesc, legalRef, content, statusStr string
	err := r.Scan(&p.ID, &p.ExtID, &p.Title, &prefTypeStr, &benefitDesc,
		&legalRef, &p.SourceURL, &content, &statusStr, &p.FetchedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.PrefType = model.PreferenceType(prefTypeStr)
	p.BenefitDesc = benefitDesc
	p.LegalRef = legalRef
	p.Content = content
	p.Status = model.PreferenceStatus(statusStr)
	return &p, nil
}

func scanNPA(r row) (*model.NPADocument, error) {
	var n model.NPADocument
	var npaNumber, npaTypeStr, issuedBy, summary, statusStr string
	var issuedAt, effectiveAt sql.NullTime
	err := r.Scan(&n.ID, &n.ExtID, &n.Title, &npaNumber, &npaTypeStr,
		&issuedBy, &issuedAt, &effectiveAt, &n.SourceURL, &summary,
		&statusStr, &n.FetchedAt, &n.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	n.NPANumber = npaNumber
	n.NPAType = model.NPAType(npaTypeStr)
	n.IssuedBy = issuedBy
	if issuedAt.Valid {
		n.IssuedAt = issuedAt.Time
	}
	if effectiveAt.Valid {
		n.EffectiveAt = effectiveAt.Time
	}
	n.Summary = summary
	n.Status = model.NPAStatus(statusStr)
	return &n, nil
}

// ============================================================================
// Утилиты
// ============================================================================

// nullableDate возвращает sql.NullTime для записи в DATE/TIMESTAMPTZ столбцы.
func nullableDate(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// nullableTime возвращает sql.NullTime для записи в TIMESTAMPTZ столбцы.
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
