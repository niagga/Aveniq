// Package insurer holds the mutual catalog of a practice: the obras sociales and
// prepagas whose patients the practice bills (L61). It is pure domain code, like the
// other aggregates: no SQL, no HTTP, no pgx.
package insurer

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// ID is the opaque identity of a mutual.
type ID string

// Sentinel errors the adapters and the transport map onto their own vocabulary.
var (
	// ErrNotFound reports that no mutual of this practice has the identity.
	ErrNotFound = errors.New("insurer: not found")
	// ErrNameTaken reports a per-practice uniqueness conflict on the name, ignoring
	// case and surrounding spaces.
	ErrNameTaken = errors.New("insurer: name already taken in this practice")
	// ErrInvalidTransition reports a lifecycle change the aggregate does not allow.
	ErrInvalidTransition = errors.New("insurer: invalid transition")
	// ErrTenantScopeMissing reports a repository call made outside WithTenant, refused
	// before any statement runs.
	ErrTenantScopeMissing = errors.New("insurer: no tenant context: the operation must run inside WithTenant")
)

// ValidationError names the field a caller got wrong, so the transport can answer
// with it instead of a generic failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("insurer: invalid %s: %s", e.Field, e.Message)
}

// Insurer is a mutual of one practice.
//
// Mutuals are deactivated, never deleted: patients and settlements reference them. A
// deactivated mutual keeps covering the patients it already covers, but it cannot be
// chosen for a patient who is not yet covered by it (see the patient use case).
type Insurer struct {
	ID       ID
	TenantID tenant.ID

	Name string

	// TaxID is the CUIT of the mutual, digits only. It is optional until the first
	// monthly settlement is issued to it (T55).
	TaxID *string

	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Draft is the data a caller may provide. Identity, timestamps and the active flag are
// decided by the system.
type Draft struct {
	Name  string
	TaxID *string
}

// New builds an active mutual and validates every invariant the schema also enforces.
func New(id ID, tenantID tenant.ID, draft Draft, now time.Time) (Insurer, error) {
	if id == "" {
		return Insurer{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Insurer{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}

	subject, err := fromDraft(id, tenantID, draft)
	if err != nil {
		return Insurer{}, err
	}
	subject.IsActive = true
	subject.CreatedAt = stored(now)
	subject.UpdatedAt = stored(now)
	return subject, nil
}

// Stored is the persisted state of a mutual, field for field.
type Stored struct {
	ID       ID
	TenantID tenant.ID
	Name     string
	TaxID    *string
	IsActive bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Reconstitute rebuilds a mutual from persisted state. Rows already passed New at
// creation, so this trusts persistence like the patient aggregate does.
func Reconstitute(stored Stored) (Insurer, error) {
	if stored.ID == "" {
		return Insurer{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if strings.TrimSpace(stored.Name) == "" {
		return Insurer{}, &ValidationError{Field: "name", Message: "must not be blank"}
	}
	return Insurer{
		ID:        stored.ID,
		TenantID:  stored.TenantID,
		Name:      stored.Name,
		TaxID:     stored.TaxID,
		IsActive:  stored.IsActive,
		CreatedAt: stored.CreatedAt.UTC(),
		UpdatedAt: stored.UpdatedAt.UTC(),
	}, nil
}

// ChangeDetails applies an edit to the name and the tax id. An inactive mutual must be
// reactivated first, the same rule a patient follows.
func (i Insurer) ChangeDetails(draft Draft, now time.Time) (Insurer, error) {
	if !i.IsActive {
		return Insurer{}, fmt.Errorf("%w: an inactive mutual must be reactivated before editing", ErrInvalidTransition)
	}
	updated, err := fromDraft(i.ID, i.TenantID, draft)
	if err != nil {
		return Insurer{}, err
	}
	updated.IsActive = i.IsActive
	updated.CreatedAt = i.CreatedAt
	updated.UpdatedAt = stored(now)
	return updated, nil
}

// Deactivate stops new coverage on this mutual without losing anything it covers.
func (i Insurer) Deactivate(now time.Time) (Insurer, error) {
	if !i.IsActive {
		return Insurer{}, fmt.Errorf("%w: the mutual is already inactive", ErrInvalidTransition)
	}
	i.IsActive = false
	i.UpdatedAt = stored(now)
	return i, nil
}

// Activate reverses Deactivate.
func (i Insurer) Activate(now time.Time) (Insurer, error) {
	if i.IsActive {
		return Insurer{}, fmt.Errorf("%w: the mutual is already active", ErrInvalidTransition)
	}
	i.IsActive = true
	i.UpdatedAt = stored(now)
	return i, nil
}

// fromDraft validates the editable fields. It is shared by New and ChangeDetails so an
// edit cannot bypass a rule the creation enforces.
func fromDraft(id ID, tenantID tenant.ID, draft Draft) (Insurer, error) {
	name := strings.TrimSpace(draft.Name)
	if name == "" {
		return Insurer{}, &ValidationError{Field: "name", Message: "must not be blank"}
	}

	taxID, err := normalizeTaxID(draft.TaxID)
	if err != nil {
		return Insurer{}, err
	}

	return Insurer{
		ID:       id,
		TenantID: tenantID,
		Name:     name,
		TaxID:    taxID,
	}, nil
}

// normalizeTaxID keeps the digits of a CUIT and refuses anything that is not eleven of
// them. Separators are accepted because people type them: 30-12345678-9 and 30123456789
// are the same mutual.
func normalizeTaxID(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	var digits strings.Builder
	for _, r := range *value {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '-', r == '.', r == ' ':
			// separators
		default:
			return nil, &ValidationError{Field: "tax_id", Message: "must contain only digits and separators"}
		}
	}
	if digits.Len() == 0 {
		return nil, nil
	}
	if digits.Len() != 11 {
		return nil, &ValidationError{Field: "tax_id", Message: "must have 11 digits"}
	}
	normalized := digits.String()
	return &normalized, nil
}

// stored truncates a timestamp to the resolution Postgres keeps, so the time the
// domain returns is the time storage returns.
func stored(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}
