// Package pricing holds the fee schedule (tarifario) of a practice (L61): what one session
// is worth for a professional, under one coverage, in one modality and one billing
// currency, from a start date on. It is pure domain code, like the other aggregates.
//
// A tariff is a version of a price, not a mutable number. A new price is a new tariff
// with a later start date; the older ones stay, because a past session keeps the value
// that was in force when it happened.
package pricing

import (
	"errors"
	"fmt"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// ID is the opaque identity of a tariff.
type ID string

// Sentinel errors the adapters and the transport map onto their own vocabulary.
var (
	// ErrNotFound reports that no tariff of this practice has the identity.
	ErrNotFound = errors.New("pricing: tariff not found")
	// ErrValidityTaken reports that the practice already has a price for the same
	// professional, coverage, modality and currency starting on the same day.
	ErrValidityTaken = errors.New("pricing: a tariff already starts on this day for this combination")
	// ErrTenantScopeMissing reports a repository call made outside WithTenant, refused
	// before any statement runs.
	ErrTenantScopeMissing = errors.New("pricing: no tenant context: the operation must run inside WithTenant")
)

// ValidationError names the field a caller got wrong, so the transport can answer with
// it instead of a generic failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("pricing: invalid %s: %s", e.Field, e.Message)
}

// Tariff is the price of one session for one combination, from ValidFrom on.
//
// InsurerMinor is what the mutual pays per session and PatientMinor what the patient
// pays. A particular session has no mutual: InsurerMinor is zero and PatientMinor is the
// whole fee. Amounts are in minor units of Currency, as the vouchers are.
type Tariff struct {
	ID        ID
	TenantID  tenant.ID
	CreatedAt time.Time
	UpdatedAt time.Time

	ProfessionalUserID string
	Coverage           patient.Coverage
	// InsurerID is set exactly when Coverage is an insurer.
	InsurerID *string
	Modality  schedule.Modality
	Currency  patient.Currency
	// ValidFrom is a civil date in the practice's calendar, stored as midnight UTC.
	ValidFrom time.Time

	InsurerMinor int64
	PatientMinor int64
}

// TotalMinor is the value of one session: what the mutual and the patient pay together.
// It is what the voucher proposal starts from (T54).
func (t Tariff) TotalMinor() int64 {
	return t.InsurerMinor + t.PatientMinor
}

// Draft is the data a caller may provide to create a tariff.
type Draft struct {
	ProfessionalUserID string
	Coverage           patient.Coverage
	InsurerID          *string
	Modality           schedule.Modality
	Currency           patient.Currency
	ValidFrom          time.Time
	InsurerMinor       int64
	PatientMinor       int64
}

// Amounts is what an existing tariff can change. Its key (professional, coverage,
// modality, currency and start day) is fixed: to move a price to another day, or to
// another combination, the practice removes it and creates it again.
type Amounts struct {
	InsurerMinor int64
	PatientMinor int64
}

// New builds a tariff and validates every invariant the schema also enforces.
func New(id ID, tenantID tenant.ID, draft Draft, now time.Time) (Tariff, error) {
	if id == "" {
		return Tariff{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Tariff{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	subject, err := fromDraft(draft)
	if err != nil {
		return Tariff{}, err
	}
	subject.ID = id
	subject.TenantID = tenantID
	subject.CreatedAt = stored(now)
	subject.UpdatedAt = stored(now)
	return subject, nil
}

// Reconstitute rebuilds a tariff from persisted state. Rows already passed New at
// creation, so this only checks what a corrupt row would break.
func Reconstitute(subject Tariff) (Tariff, error) {
	if subject.ID == "" {
		return Tariff{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if err := checkAmounts(subject.Coverage, subject.InsurerMinor, subject.PatientMinor); err != nil {
		return Tariff{}, err
	}
	subject.CreatedAt = subject.CreatedAt.UTC()
	subject.UpdatedAt = subject.UpdatedAt.UTC()
	subject.ValidFrom = civilDay(subject.ValidFrom)
	return subject, nil
}

// ChangeAmounts applies a new split to an existing tariff. The coverage rules still hold:
// a particular tariff keeps the mutual at zero.
func (t Tariff) ChangeAmounts(amounts Amounts, now time.Time) (Tariff, error) {
	if err := checkAmounts(t.Coverage, amounts.InsurerMinor, amounts.PatientMinor); err != nil {
		return Tariff{}, err
	}
	t.InsurerMinor = amounts.InsurerMinor
	t.PatientMinor = amounts.PatientMinor
	t.UpdatedAt = stored(now)
	return t, nil
}

// fromDraft validates the creation fields. It is shared by New so every rule lives in one
// place.
func fromDraft(draft Draft) (Tariff, error) {
	if draft.ProfessionalUserID == "" {
		return Tariff{}, &ValidationError{Field: "professional_user_id", Message: "must not be empty"}
	}
	if draft.Coverage != patient.CoveragePrivate && draft.Coverage != patient.CoverageInsurer {
		return Tariff{}, &ValidationError{Field: "coverage", Message: "must be private or insurer"}
	}
	switch {
	case draft.Coverage == patient.CoveragePrivate && draft.InsurerID != nil:
		return Tariff{}, &ValidationError{Field: "insurer_id", Message: "must be empty for a particular price"}
	case draft.Coverage == patient.CoverageInsurer && (draft.InsurerID == nil || *draft.InsurerID == ""):
		return Tariff{}, &ValidationError{Field: "insurer_id", Message: "is required for a mutual price"}
	}
	if !draft.Modality.Valid() {
		return Tariff{}, &ValidationError{Field: "modality", Message: "must be online or presencial"}
	}
	if !patient.ValidCurrency(draft.Currency) {
		return Tariff{}, &ValidationError{Field: "billing_currency", Message: "must be ARS, EUR or USD"}
	}
	if draft.ValidFrom.IsZero() {
		return Tariff{}, &ValidationError{Field: "valid_from", Message: "is required"}
	}
	if err := checkAmounts(draft.Coverage, draft.InsurerMinor, draft.PatientMinor); err != nil {
		return Tariff{}, err
	}

	return Tariff{
		ProfessionalUserID: draft.ProfessionalUserID,
		Coverage:           draft.Coverage,
		InsurerID:          draft.InsurerID,
		Modality:           draft.Modality,
		Currency:           draft.Currency,
		ValidFrom:          civilDay(draft.ValidFrom),
		InsurerMinor:       draft.InsurerMinor,
		PatientMinor:       draft.PatientMinor,
	}, nil
}

// checkAmounts holds the split rules shared by creation, edits and reconstitution.
func checkAmounts(coverage patient.Coverage, insurerMinor, patientMinor int64) error {
	if insurerMinor < 0 {
		return &ValidationError{Field: "insurer_minor", Message: "must not be negative"}
	}
	if patientMinor < 0 {
		return &ValidationError{Field: "patient_minor", Message: "must not be negative"}
	}
	if coverage == patient.CoveragePrivate && insurerMinor != 0 {
		return &ValidationError{Field: "insurer_minor", Message: "must be zero for a particular price"}
	}
	if insurerMinor+patientMinor == 0 {
		return &ValidationError{Field: "patient_minor", Message: "the session must be worth something"}
	}
	return nil
}

// civilDay keeps the calendar date of a timestamp and drops the clock, so that a price
// starts on a day, whatever zone the caller parsed it in.
func civilDay(at time.Time) time.Time {
	year, month, day := at.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// stored truncates a timestamp to the resolution Postgres keeps, so the time the domain
// returns is the time storage returns.
func stored(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}
