package patient

import (
	"fmt"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// ID is the opaque identity of a patient.
type ID string

// Patient is the practice's client record.
//
// The clinical record is deliberately not here: it is a separate aggregate with a
// different audience (0006). Mixing them would make the permission matrix unable
// to tell "the receptionist can see the phone number" from "the receptionist can
// read the sessions".
type Patient struct {
	ID       ID
	TenantID tenant.ID

	// Country is the one the document and the fiscal data are validated against.
	// It is always resolved before saving: a patient inherits the practice's
	// country when nobody chooses one, so no row can carry a document without a
	// country to check it against.
	Country string

	FullName  string
	BirthDate *time.Time

	// Document is optional: a practice must be able to register someone who walks
	// in without one, or the real data ends up in the notes field.
	Document *Document

	Phone                 *string
	Email                 *string
	EmergencyContactName  *string
	EmergencyContactPhone *string

	// Fiscal data, optional as a group: not every patient is invoiced.
	TaxID           *TaxID
	FiscalCondition *string
	FiscalAddress   *string

	AdministrativeNotes *string

	// TreatingProfessionalID is the professional who treats the patient (the tratante).
	// Nil means nobody treats it yet: patients registered before the field existed
	// have none until an owner or admin assigns one. Its meaning and who may change
	// it are decided in domain/care, not here.
	TreatingProfessionalID *string

	// Coverage says who pays for the sessions. A private patient has no mutual and no
	// member number; an insured one has both, and the mutual belongs to this practice.
	CoverageKind Coverage
	InsurerID    *string
	MemberNumber *string

	// BillingCurrency is chosen by hand at registration. Nothing derives it from the
	// country, so an Argentine patient can be billed in dollars.
	BillingCurrency Currency

	// IsActive is the deactivation flag. Patients are never deleted: clinical
	// notes will reference this row.
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WithTreating returns the patient with the given professional as its treating one.
// The receiver is a value, so the caller decides whether to persist the result.
func (p Patient) WithTreating(professionalUserID string) Patient {
	p.TreatingProfessionalID = &professionalUserID
	return p
}

// Draft is the data a caller may provide. Identity, timestamps and the active flag
// are absent because the system decides them.
type Draft struct {
	FullName              string
	Country               string
	BirthDate             *time.Time
	Document              *Document
	Phone                 *string
	Email                 *string
	EmergencyContactName  *string
	EmergencyContactPhone *string
	TaxID                 *TaxID
	FiscalCondition       *string
	FiscalAddress         *string
	AdministrativeNotes   *string

	CoverageKind    Coverage
	InsurerID       *string
	MemberNumber    *string
	BillingCurrency Currency
}

// New builds a patient and validates every invariant the schema also enforces, so
// a bad request fails in the domain with a field name instead of as a SQLSTATE.
func New(id ID, tenantID tenant.ID, draft Draft, now time.Time) (Patient, error) {
	if id == "" {
		return Patient{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Patient{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}

	subject, err := fromDraft(id, tenantID, draft, now)
	if err != nil {
		return Patient{}, err
	}
	subject.IsActive = true
	subject.CreatedAt = stored(now)
	subject.UpdatedAt = stored(now)
	return subject, nil
}

// Stored is the persisted state of a patient, field for field.
//
// It is a struct instead of a seventeen-argument constructor because the adapter
// builds it positional-adjacent and two swapped *string parameters would compile,
// run, and quietly write the emergency contact into the fiscal address.
type Stored struct {
	ID       ID
	TenantID tenant.ID

	Country  string
	FullName string

	BirthDate *time.Time
	Document  *Document

	Phone                 *string
	Email                 *string
	EmergencyContactName  *string
	EmergencyContactPhone *string

	TaxID           *TaxID
	FiscalCondition *string
	FiscalAddress   *string

	AdministrativeNotes *string

	TreatingProfessionalID *string

	CoverageKind    Coverage
	InsurerID       *string
	MemberNumber    *string
	BillingCurrency Currency

	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Reconstitute rebuilds a patient from persisted state. Rows already passed New at
// creation; revalidating here would only duplicate the checks, so this constructor
// trusts persistence and lets the adapter wrap any anomaly in its own error.
func Reconstitute(stored Stored) (Patient, error) {
	if stored.ID == "" {
		return Patient{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if strings.Trim(stored.FullName, " ") == "" {
		return Patient{}, &ValidationError{Field: "full_name", Message: "must not be blank"}
	}

	return Patient{
		ID:                     stored.ID,
		TenantID:               stored.TenantID,
		Country:                stored.Country,
		FullName:               stored.FullName,
		BirthDate:              stored.BirthDate,
		Document:               stored.Document,
		Phone:                  stored.Phone,
		Email:                  stored.Email,
		EmergencyContactName:   stored.EmergencyContactName,
		EmergencyContactPhone:  stored.EmergencyContactPhone,
		TaxID:                  stored.TaxID,
		FiscalCondition:        stored.FiscalCondition,
		FiscalAddress:          stored.FiscalAddress,
		AdministrativeNotes:    stored.AdministrativeNotes,
		TreatingProfessionalID: stored.TreatingProfessionalID,
		CoverageKind:           stored.CoverageKind,
		InsurerID:              stored.InsurerID,
		MemberNumber:           stored.MemberNumber,
		BillingCurrency:        stored.BillingCurrency,
		IsActive:               stored.IsActive,
		CreatedAt:              stored.CreatedAt.UTC(),
		UpdatedAt:              stored.UpdatedAt.UTC(),
	}, nil
}

// fromDraft validates everything a caller can get wrong. It is shared by New and
// ChangeProfile so an edit cannot bypass a rule the creation enforced.
func fromDraft(id ID, tenantID tenant.ID, draft Draft, now time.Time) (Patient, error) {
	// btrim trims spaces but not tabs, and the schema constraint does the same.
	// Mirroring the quirk is deliberate: a domain that trimmed tabs would accept a
	// name the database rejects, and the caller would see a SQLSTATE instead of a
	// field.
	if strings.Trim(draft.FullName, " ") == "" {
		return Patient{}, &ValidationError{Field: "full_name", Message: "must not be blank"}
	}

	if !ValidCountry(draft.Country) {
		return Patient{}, &ValidationError{Field: "country", Message: "must be a two-letter ISO country code"}
	}

	if draft.Document != nil {
		if draft.Document.Country != draft.Country {
			// The catalog foreign key is (country, kind): a document validated under
			// one country cannot be stored under another.
			return Patient{}, &ValidationError{
				Field:   "document_country",
				Message: "must match the patient's country",
			}
		}
		if draft.Document.Kind == "" || draft.Document.Normalized == "" {
			return Patient{}, &ValidationError{Field: "document_number", Message: "must be normalized before it reaches the aggregate"}
		}
	}

	if draft.TaxID != nil {
		if draft.TaxID.Kind == "" || draft.TaxID.Normalized == "" {
			return Patient{}, &ValidationError{Field: "tax_id", Message: "must be normalized before it reaches the aggregate"}
		}
	}

	if draft.BirthDate != nil && draft.BirthDate.After(now) {
		return Patient{}, &ValidationError{Field: "birth_date", Message: "must not be in the future"}
	}

	if !ValidCurrency(draft.BillingCurrency) {
		return Patient{}, &ValidationError{Field: "billing_currency", Message: "must be ARS, EUR or USD"}
	}
	coverage, insurerID, memberNumber, err := coverageFromDraft(draft)
	if err != nil {
		return Patient{}, err
	}

	return Patient{
		ID:                    id,
		TenantID:              tenantID,
		Country:               draft.Country,
		FullName:              strings.TrimSpace(draft.FullName),
		BirthDate:             draft.BirthDate,
		Document:              draft.Document,
		Phone:                 sanitize(draft.Phone),
		Email:                 sanitizeEmail(draft.Email),
		EmergencyContactName:  sanitize(draft.EmergencyContactName),
		EmergencyContactPhone: sanitize(draft.EmergencyContactPhone),
		TaxID:                 draft.TaxID,
		FiscalCondition:       sanitize(draft.FiscalCondition),
		FiscalAddress:         sanitize(draft.FiscalAddress),
		AdministrativeNotes:   sanitize(draft.AdministrativeNotes),
		CoverageKind:          coverage,
		InsurerID:             insurerID,
		MemberNumber:          memberNumber,
		BillingCurrency:       draft.BillingCurrency,
	}, nil
}

// coverageFromDraft checks the coverage as a group. A private patient must not name a
// mutual or a member number, because the schema refuses that row and the caller would
// otherwise see a SQLSTATE. An insured patient must name both.
func coverageFromDraft(draft Draft) (Coverage, *string, *string, error) {
	insurerID := sanitize(draft.InsurerID)
	memberNumber := sanitize(draft.MemberNumber)

	switch draft.CoverageKind {
	case CoveragePrivate:
		if insurerID != nil {
			return "", nil, nil, &ValidationError{Field: "insurer_id", Message: "must be empty for a private patient"}
		}
		if memberNumber != nil {
			return "", nil, nil, &ValidationError{Field: "member_number", Message: "must be empty for a private patient"}
		}
		return CoveragePrivate, nil, nil, nil
	case CoverageInsurer:
		if insurerID == nil {
			return "", nil, nil, &ValidationError{Field: "insurer_id", Message: "is required for a patient with a mutual"}
		}
		if memberNumber == nil {
			return "", nil, nil, &ValidationError{Field: "member_number", Message: "is required for a patient with a mutual"}
		}
		return CoverageInsurer, insurerID, memberNumber, nil
	default:
		return "", nil, nil, &ValidationError{Field: "coverage_kind", Message: "must be private or insurer"}
	}
}

// ChangeProfile applies an edit, keeping identity and creation untouched.
func (p Patient) ChangeProfile(draft Draft, now time.Time) (Patient, error) {
	if !p.IsActive {
		return Patient{}, fmt.Errorf("%w: an inactive patient must be reactivated before editing", ErrInvalidTransition)
	}

	updated, err := fromDraft(p.ID, p.TenantID, draft, now)
	if err != nil {
		return Patient{}, err
	}

	updated.IsActive = p.IsActive
	updated.CreatedAt = p.CreatedAt
	updated.UpdatedAt = stored(now)
	return updated, nil
}

// Deactivate stops the patient from appearing in new appointments without losing
// anything: the row, its documents and its clinical history stay readable.
func (p Patient) Deactivate(now time.Time) (Patient, error) {
	if !p.IsActive {
		return Patient{}, fmt.Errorf("%w: the patient is already inactive", ErrInvalidTransition)
	}
	p.IsActive = false
	p.UpdatedAt = stored(now)
	return p, nil
}

// Activate reverses Deactivate.
func (p Patient) Activate(now time.Time) (Patient, error) {
	if p.IsActive {
		return Patient{}, fmt.Errorf("%w: the patient is already active", ErrInvalidTransition)
	}
	p.IsActive = true
	p.UpdatedAt = stored(now)
	return p, nil
}

// sanitize trims an optional free-text field and turns an empty result into nil,
// so "absent" and "empty string" are one state instead of two.
func sanitize(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// sanitizeEmail lowercases and trims, mirroring how it will be compared.
func sanitizeEmail(value *string) *string {
	cleaned := sanitize(value)
	if cleaned == nil {
		return nil
	}
	lowered := strings.ToLower(*cleaned)
	return &lowered
}

// stored truncates a timestamp to the resolution Postgres keeps.
//
// The schema stores microseconds, so a nanosecond time comes back different from
// the one that was written. Truncating in the aggregate is what keeps "the time the
// domain says" and "the time storage returns" the same value instead of leaving a
// comparison that silently depends on where it runs.
func stored(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}
