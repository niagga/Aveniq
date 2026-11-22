// Package client holds the billing client of a practice: who pays for sessions (S26).
// It is separate from the patient aggregate on purpose: one client may cover several
// patients, and fiscal data belongs here rather than on the clinical register.
package client

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// ID is the opaque identity of a billing client.
type ID string

var (
	ErrNotFound           = errors.New("client: not found")
	ErrInvalidTransition  = errors.New("client: invalid transition")
	ErrTenantScopeMissing = errors.New("client: no tenant context: the operation must run inside WithTenant")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("client: invalid %s: %s", e.Field, e.Message)
}

// Client is who pays for one or more patients of a practice.
type Client struct {
	ID       ID
	TenantID tenant.ID

	DisplayName string
	LegalName   *string
	Country     string

	TaxID           *patient.TaxID
	FiscalCondition *string
	FiscalAddress   *string

	BillingCurrency patient.Currency

	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Draft is what a caller provides when creating or editing a billing client.
type Draft struct {
	DisplayName string
	LegalName   *string
	Country     string

	TaxID           *patient.TaxID
	FiscalCondition *string
	FiscalAddress   *string

	BillingCurrency patient.Currency
}

// New builds an active client.
func New(id ID, tenantID tenant.ID, draft Draft, now time.Time) (Client, error) {
	if id == "" {
		return Client{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Client{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}

	subject, err := fromDraft(id, tenantID, draft)
	if err != nil {
		return Client{}, err
	}
	subject.IsActive = true
	subject.CreatedAt = stored(now)
	subject.UpdatedAt = stored(now)
	return subject, nil
}

type Stored struct {
	ID       ID
	TenantID tenant.ID

	DisplayName string
	LegalName   *string
	Country     string

	TaxID           *patient.TaxID
	FiscalCondition *string
	FiscalAddress   *string

	BillingCurrency patient.Currency

	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func Reconstitute(stored Stored) (Client, error) {
	if stored.ID == "" {
		return Client{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if strings.TrimSpace(stored.DisplayName) == "" {
		return Client{}, &ValidationError{Field: "display_name", Message: "must not be blank"}
	}
	return Client{
		ID:              stored.ID,
		TenantID:        stored.TenantID,
		DisplayName:     stored.DisplayName,
		LegalName:       stored.LegalName,
		Country:         stored.Country,
		TaxID:           stored.TaxID,
		FiscalCondition: stored.FiscalCondition,
		FiscalAddress:   stored.FiscalAddress,
		BillingCurrency: stored.BillingCurrency,
		IsActive:        stored.IsActive,
		CreatedAt:       stored.CreatedAt.UTC(),
		UpdatedAt:       stored.UpdatedAt.UTC(),
	}, nil
}

func (c Client) ChangeProfile(draft Draft, now time.Time) (Client, error) {
	if !c.IsActive {
		return Client{}, fmt.Errorf("%w: an inactive client must be reactivated before editing", ErrInvalidTransition)
	}
	updated, err := fromDraft(c.ID, c.TenantID, draft)
	if err != nil {
		return Client{}, err
	}
	updated.IsActive = c.IsActive
	updated.CreatedAt = c.CreatedAt
	updated.UpdatedAt = stored(now)
	return updated, nil
}

func (c Client) Deactivate(now time.Time) (Client, error) {
	if !c.IsActive {
		return Client{}, fmt.Errorf("%w: the client is already inactive", ErrInvalidTransition)
	}
	c.IsActive = false
	c.UpdatedAt = stored(now)
	return c, nil
}

func (c Client) Activate(now time.Time) (Client, error) {
	if c.IsActive {
		return Client{}, fmt.Errorf("%w: the client is already active", ErrInvalidTransition)
	}
	c.IsActive = true
	c.UpdatedAt = stored(now)
	return c, nil
}

func fromDraft(id ID, tenantID tenant.ID, draft Draft) (Client, error) {
	name := strings.TrimSpace(draft.DisplayName)
	if name == "" {
		return Client{}, &ValidationError{Field: "display_name", Message: "must not be blank"}
	}
	if !patient.ValidCountry(draft.Country) {
		return Client{}, &ValidationError{Field: "country", Message: "must be a two-letter ISO country code"}
	}
	if !patient.ValidCurrency(draft.BillingCurrency) {
		return Client{}, &ValidationError{Field: "billing_currency", Message: "must be ARS, EUR or USD"}
	}
	if draft.TaxID != nil {
		if draft.TaxID.Kind == "" || draft.TaxID.Normalized == "" {
			return Client{}, &ValidationError{Field: "tax_id", Message: "must be normalized before it reaches the aggregate"}
		}
	}

	return Client{
		ID:              id,
		TenantID:        tenantID,
		DisplayName:     name,
		LegalName:       sanitize(draft.LegalName),
		Country:         draft.Country,
		TaxID:           draft.TaxID,
		FiscalCondition: sanitize(draft.FiscalCondition),
		FiscalAddress:   sanitize(draft.FiscalAddress),
		BillingCurrency: draft.BillingCurrency,
	}, nil
}

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

func stored(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}