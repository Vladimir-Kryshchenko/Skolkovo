// Package model описывает доменные сущности «База Сколково».
package model

import "time"

// PreferenceType — тип льготы.
type PreferenceType string

const (
	PrefTaxProfit  PreferenceType = "tax_profit"
	PrefInsurance  PreferenceType = "insurance"
	PrefVAT        PreferenceType = "vat"
	PrefCustoms    PreferenceType = "customs"
	PrefOther      PreferenceType = "other"
)

// PreferenceStatus — статус льготы.
type PreferenceStatus string

const (
	PrefStatusActive   PreferenceStatus = "active"
	PrefStatusOutdated PreferenceStatus = "outdated"
)

// Preference — льгота резидента Сколково.
type Preference struct {
	ID          string            `json:"id"`
	ExtID       string            `json:"ext_id"`
	Title       string            `json:"title"`
	PrefType    PreferenceType    `json:"pref_type"`
	BenefitDesc string            `json:"benefit_desc,omitempty"`
	LegalRef    string            `json:"legal_ref,omitempty"`
	SourceURL   string            `json:"source_url"`
	Content     string            `json:"content,omitempty"`
	Status      PreferenceStatus  `json:"status"`
	FetchedAt   time.Time         `json:"fetched_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// IsActive проверяет, действует ли льгота.
func (p *Preference) IsActive() bool {
	return p.Status == PrefStatusActive
}

// NPAStatus — статус нормативно-правового акта.
type NPAStatus string

const (
	NPAStatusActive   NPAStatus = "active"
	NPAStatusAmended  NPAStatus = "amended"
	NPAStatusRevoked  NPAStatus = "revoked"
)

// NPAType — тип нормативно-правового акта.
type NPAType string

const (
	NPATypeLaw      NPAType = "law"
	NPATypeDecree   NPAType = "decree"
	NPATypeOrder    NPAType = "order"
	NPATypeDecision NPAType = "decision"
)

// NPADocument — нормативно-правовой акт.
type NPADocument struct {
	ID          string    `json:"id"`
	ExtID       string    `json:"ext_id"`
	Title       string    `json:"title"`
	NPANumber   string    `json:"npa_number,omitempty"`
	NPAType     NPAType   `json:"npa_type,omitempty"`
	IssuedBy    string    `json:"issued_by,omitempty"`
	IssuedAt    time.Time `json:"issued_at,omitempty"`
	EffectiveAt time.Time `json:"effective_at,omitempty"`
	SourceURL   string    `json:"source_url"`
	Summary     string    `json:"summary,omitempty"`
	Status      NPAStatus `json:"status"`
	FetchedAt   time.Time `json:"fetched_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IsActive проверяет, действует ли НПА.
func (n *NPADocument) IsActive() bool {
	return n.Status == NPAStatusActive
}

// IsRevoked проверяет, отменён ли НПА.
func (n *NPADocument) IsRevoked() bool {
	return n.Status == NPAStatusRevoked
}

// IsExpired проверяет, наступила ли дата вступления в силу.
func (n *NPADocument) IsExpired(now time.Time) bool {
	return !n.EffectiveAt.IsZero() && now.Before(n.EffectiveAt)
}
