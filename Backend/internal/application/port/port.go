// Package port declares the boundaries the application layer needs from the
// outside world. Interfaces live with their consumer (not with the implementation)
// so that infrastructure depends on application, never the other way around.
//
// This is the Go form of the dependency-inversion principle: the use cases know
// only these contracts, and every adapter — Postgres, Redis, a fake for tests —
// is interchangeable behind them.
package port

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// UserRepository persists and reads the IAM user aggregate.
//
// Implementations must map their own failures onto the domain sentinels
// (iam.ErrNotFound, iam.ErrEmailTaken) so the application layer never has to
// recognise a driver-specific error. Every call runs inside a tenant scope:
// see TenantScope.
type UserRepository interface {
	Create(ctx context.Context, subject iam.User) error
	ByID(ctx context.Context, id iam.UserID) (iam.User, error)
	ByEmail(ctx context.Context, email string) (iam.User, error)
	List(ctx context.Context, limit, offset int32) ([]iam.User, error)
	Update(ctx context.Context, subject iam.User) error
	Delete(ctx context.Context, id iam.UserID) error
}

// TenantScope runs a unit of work inside a transaction that carries the tenant
// context row-level security reads (app.tenant_id). It is the transactional
// boundary the IAM repositories assume: decision 4 of L6 — the context is set
// per transaction with the transactional form of SET LOCAL, never per
// connection, because pooled connections are reused across tenants.
type TenantScope interface {
	WithTenant(ctx context.Context, tenantID string, fn func(ctx context.Context) error) error
}

// TenantRepository persists and reads the tenant aggregate.
//
// Implementations must map their own failures onto the domain sentinels
// (tenant.ErrNotFound, tenant.ErrSlugTaken) so the application layer never has
// to recognise a driver-specific error.
type TenantRepository interface {
	Create(ctx context.Context, subject tenant.Tenant) error
	BySlug(ctx context.Context, slug string) (tenant.Tenant, error)
	ByID(ctx context.Context, id tenant.ID) (tenant.Tenant, error)
	List(ctx context.Context, limit, offset int32) ([]tenant.Tenant, error)
}

// Clock abstracts time so use cases are deterministic under test.
type Clock interface {
	Now() time.Time
}

// PatientRepository persists and reads the patient aggregate.
//
// Implementations must map their own failures onto the domain sentinels
// (patient.ErrNotFound, patient.ErrDocumentTaken, patient.ErrDocumentInvalid) so the
// application layer never has to recognise a driver-specific error. Every call runs
// inside a tenant scope: see TenantScope.
type PatientRepository interface {
	Create(ctx context.Context, subject patient.Patient) error
	ByID(ctx context.Context, id patient.ID) (patient.Patient, error)
	List(ctx context.Context, filter PatientFilter) ([]patient.Patient, error)
	Update(ctx context.Context, subject patient.Patient) error
}

// PatientFilter narrows a listing. Search matches a fragment of the name or the
// exact normalized document.
type PatientFilter struct {
	Search     string
	ActiveOnly bool
	Limit      int32
	Offset     int32
}

// FiscalCondition is a catalog row: which fiscal situations a country knows.
type FiscalCondition struct {
	Country string
	Code    string
	Label   string
}

// DocumentCatalog answers what a country accepts.
//
// It exists so the application can turn a typed document into a canonical one
// without knowing any country, and so the domain stays free of storage. The
// catalogs are read-only for the application role: they ship with the schema.
//
// An unknown kind is reported as patient.ErrDocumentInvalid: from the caller's
// point of view, a country that does not know "nie" and a request that misspells
// it are the same problem.
type DocumentCatalog interface {
	DocumentKind(ctx context.Context, country, code string) (patient.DocumentKind, error)
	TaxIDKind(ctx context.Context, country, code string) (patient.DocumentKind, error)
	FiscalCondition(ctx context.Context, country, code string) (FiscalCondition, error)
	// TenantCountry is the country a patient inherits when the request does not
	// choose one. It reads the tenants table, which has no row-level security.
	TenantCountry(ctx context.Context, tenantID string) (string, error)
}

// IDGenerator abstracts identity generation for the same reason.
type IDGenerator interface {
	NewID() string
}

// SessionRepository persists the refresh sessions of the IAM aggregate.
//
// Decision 7 of L11 lives in the implementation: a foreign key inside `sessions`
// does not go through row-level security, so building a session on a user UUID
// from another tenant must be refused by the adapter, not by a caller that could
// forget. Every call runs inside a tenant scope (see TenantScope).
type SessionRepository interface {
	Create(ctx context.Context, subject iam.Session) error
	ByRefreshHash(ctx context.Context, hash iam.RefreshHash) (iam.Session, error)
	Rotate(ctx context.Context, previous, successor iam.Session) error
	Revoke(ctx context.Context, id iam.SessionID, at time.Time) error
	RevokeChain(ctx context.Context, id iam.SessionID, at time.Time) (int64, error)
}
