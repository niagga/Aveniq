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

	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
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

// ClinicalRepository persists the clinical record and its notes.
//
// One port for both aggregates because the screen reads them as a unit and the writes
// share a tenant transaction; splitting them would create two seams that always move
// together.
//
// Implementations must map their failures onto the clinical sentinels
// (clinical.ErrNotFound, clinical.ErrPatientNotVisible) so the application layer never
// sees a driver code. Every call runs inside a tenant scope: see TenantScope.
type ClinicalRepository interface {
	// UpsertRecord writes the frame of a treatment, creating it on first save: the
	// record is one per patient, so an edit is an update, never a second row.
	UpsertRecord(ctx context.Context, record clinical.Record) (clinical.Record, error)
	RecordByPatient(ctx context.Context, patientID string) (clinical.Record, error)
	// AddNote appends one encounter. There is no update and no delete: a correction is
	// a new note, because deleting clinical history is a legal problem rather than a
	// UI decision.
	AddNote(ctx context.Context, note clinical.Note) error
	NotesByPatient(ctx context.Context, patientID string, limit, offset int32) ([]clinical.Note, error)
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

// ScheduleRepository persists appointments and weekly series.
//
// Implementations must map overlap onto schedule.ErrOverlap: two concurrent
// requests must lose against the exclusion constraint, not against a Go check.
type ScheduleRepository interface {
	CreateAppointment(ctx context.Context, slot schedule.Appointment) error
	AppointmentByID(ctx context.Context, id schedule.AppointmentID) (schedule.Appointment, error)
	ListAppointments(ctx context.Context, filter AppointmentFilter) ([]schedule.Appointment, error)
	SaveAppointment(ctx context.Context, slot schedule.Appointment) error

	CreateSeries(ctx context.Context, series schedule.Series) error
	SeriesByID(ctx context.Context, id schedule.SeriesID) (schedule.Series, error)
	SaveSeries(ctx context.Context, series schedule.Series) error
	DeleteScheduledFrom(ctx context.Context, seriesID schedule.SeriesID, from time.Time) error
	CancelScheduled(ctx context.Context, seriesID schedule.SeriesID, at time.Time) error
}

// AppointmentFilter is the window the weekly screen asks for.
type AppointmentFilter struct {
	From           time.Time
	To             time.Time
	ProfessionalID string // empty means every professional of the practice
}

// AuditEvent is one entry in the append-only trail.
//
// TenantID is always set for a clinical event: the record belongs to a practice and
// the reader belongs to that practice. The nullable tenant the schema allows exists
// for platform events, which E2 does not have (L26).
type AuditEvent struct {
	TenantID string
	ActorID  string
	// Action is a verb the log can be filtered by, for example "clinical.read".
	Action string
	// Entity and EntityID name what was touched.
	Entity   string
	EntityID string
	Metadata map[string]any
}

// AuditLog records what happened, inside the transaction of the thing that happened.
//
// The transaction is the whole point: an implementation that writes on its own
// connection would let a clinical read commit while its trail did not, and a read
// without a trail is what this port exists to prevent.
type AuditLog interface {
	Record(ctx context.Context, event AuditEvent) error
}

// RateLimiter counts attempts per key inside a fixed window. Hit records one attempt
// and answers how many the window holds now, this one included. The counter lives
// in a shared store, so every API instance sees the same attempts. An error means
// the store could not answer; callers decide whether that fails open or closed.
type RateLimiter interface {
	Hit(ctx context.Context, key string, window time.Duration) (int64, error)
}

// InvitationRepository persists the pending invitations of a practice.
//
// Every method runs inside a tenant scope (see TenantScope). The public accept
// path opens that scope from the tenant hint in the token before ByTokenHash, so a
// token presented under another practice's hint matches nothing under RLS.
// Implementations map their failures onto iam.ErrNotFound and
// iam.ErrInvitationUnusable.
type InvitationRepository interface {
	Create(ctx context.Context, subject iam.Invitation) error
	ByID(ctx context.Context, id iam.InvitationID) (iam.Invitation, error)
	ByTokenHash(ctx context.Context, hash string) (iam.Invitation, error)
	// Pending lists invitations that are neither accepted nor expired at now.
	Pending(ctx context.Context, now time.Time, limit, offset int32) ([]iam.Invitation, error)
	// MarkAccepted is the single-use guard: it only updates an invitation that is
	// still unaccepted and unexpired, and answers iam.ErrInvitationUnusable when
	// another request won the race.
	MarkAccepted(ctx context.Context, id iam.InvitationID, at time.Time) error
	// Delete removes a pending invitation. An accepted one stays: it is part of
	// who joined the practice and when.
	Delete(ctx context.Context, id iam.InvitationID) error
}
