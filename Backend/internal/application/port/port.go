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

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/landing"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/report"
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
	// AssignTreating persists only the treating professional of the patient. It is
	// separate from Update so editing the file and reassigning the patient are two
	// writes with two permissions.
	AssignTreating(ctx context.Context, subject patient.Patient) error
}

// InsurerRepository persists the mutual catalog of a practice. Like the patient
// repository it runs inside a tenant scope, and it maps its failures onto
// insurer.ErrNotFound and insurer.ErrNameTaken.
type InsurerRepository interface {
	Create(ctx context.Context, subject insurer.Insurer) error
	ByID(ctx context.Context, id insurer.ID) (insurer.Insurer, error)
	// List returns the mutuals ordered by name. ActiveOnly hides the deactivated ones,
	// which the patient forms offer for new coverage.
	List(ctx context.Context, activeOnly bool) ([]insurer.Insurer, error)
	Update(ctx context.Context, subject insurer.Insurer) error
}

// TariffListItem is a tariff with the names the screen shows for it. The names are read
// in the same statement, so the reader needs no permission over the staff directory or
// the catalog to see them.
type TariffListItem struct {
	Tariff           pricing.Tariff
	ProfessionalName string
	// InsurerName is empty for a particular tariff.
	InsurerName string
}

// TariffRepository persists the fee schedule of a practice. Like the other repositories it
// runs inside a tenant scope, and it maps its failures onto pricing.ErrNotFound,
// pricing.ErrValidityTaken and the validation errors of the references it cannot resolve.
type TariffRepository interface {
	Create(ctx context.Context, subject pricing.Tariff) error
	ByID(ctx context.Context, id pricing.ID) (pricing.Tariff, error)
	// List returns every tariff of the practice, ordered for reading by professional.
	List(ctx context.Context) ([]TariffListItem, error)
	// Update persists the amounts of a tariff. Its key is never rewritten.
	Update(ctx context.Context, subject pricing.Tariff) error
	Delete(ctx context.Context, id pricing.ID) error
	// InForce returns the tariff that prices a session, or pricing.ErrNotFound when the
	// practice has none for that professional, coverage, modality and currency on that day.
	InForce(ctx context.Context, query TariffQuery) (pricing.Tariff, error)
}

// TariffQuery is the key a session is priced by. Coverage and modality are the words of
// the domain ("private" or "insurer", "online" or "presencial"); InsurerID is empty for a
// particular patient. Day is the civil day of the session in the practice's timezone.
type TariffQuery struct {
	ProfessionalUserID string
	Coverage           string
	InsurerID          string
	Modality           string
	Currency           string
	Day                time.Time
}

// PatientFilter narrows a listing. Search matches a fragment of the name or the
// exact normalized document.
//
// OwnOnly restricts the listing to the patients the viewer treats or holds a live
// grant on. It is decided by the use case from the viewer's role, and the adapter
// applies it in the query so pagination counts only what the viewer may see.
type PatientFilter struct {
	Search     string
	ActiveOnly bool
	OwnOnly    bool
	ViewerID   string
	Limit      int32
	Offset     int32
}

// ClinicalGrantRepository persists the clinical grants a treating professional gives
// over a patient. Like the other repositories it runs inside a tenant scope, and it
// maps its failures onto care.ErrGrantNotFound and care.ErrGrantAlreadyLive.
type ClinicalGrantRepository interface {
	Create(ctx context.Context, grant care.Grant) error
	ByID(ctx context.Context, id string) (care.Grant, error)
	// HasLiveGrant reports whether the professional holds an unrevoked grant on the
	// patient. It is the only grant question the read paths ask.
	HasLiveGrant(ctx context.Context, patientID, professionalUserID string) (bool, error)
	ListByPatient(ctx context.Context, patientID string) ([]care.Grant, error)
	Save(ctx context.Context, grant care.Grant) error
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
	// RescheduleAppointment moves a scheduled slot to a new interval. Overlap maps to schedule.ErrOverlap.
	RescheduleAppointment(ctx context.Context, slot schedule.Appointment) error

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

// BillingRepository persists internal vouchers.
//
// Implementations must map their failures onto the billing sentinels
// (billing.ErrNotFound, billing.ErrAlreadyVoided, billing.ErrPatientNotVisible)
// so the application layer never sees a driver code. Every call runs inside a
// tenant scope: see TenantScope.
//
// Issue reserves the next number and writes the voucher in the same transaction.
// Void never returns that number to the counter.
type BillingRepository interface {
	Issue(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error)
	Void(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error)
	ByID(ctx context.Context, id billing.VoucherID) (billing.Voucher, error)
	List(ctx context.Context, filter VoucherFilter) ([]billing.Voucher, error)
	// Subject answers the billing identity of one patient, including inactive ones: a
	// past session can still owe a receipt. A patient the practice does not hold is
	// billing.ErrPatientNotVisible.
	Subject(ctx context.Context, patientID string) (billing.Subject, error)
	// SearchSubjects finds active patients whose name, document or tax id contains the
	// fragment. It is not a roster: the caller must send a fragment, and the page is
	// bounded.
	SearchSubjects(ctx context.Context, fragment string, limit int32) ([]billing.Subject, error)
	// PatientBilling answers how a patient is billed: coverage, mutual and currency. A patient
	// the practice does not hold is billing.ErrPatientNotVisible.
	PatientBilling(ctx context.Context, patientID string) (billing.PatientBilling, error)
	// Appointment reads one session. A session the practice does not hold is
	// billing.ErrAppointmentNotVisible.
	Appointment(ctx context.Context, appointmentID string) (billing.Appointment, error)
	// PatientAppointments returns the newest sessions of one patient, bounded by limit.
	PatientAppointments(ctx context.Context, patientID string, limit int32) ([]billing.Appointment, error)
	// UnbilledSessions lists the attended sessions in [from, to) that no issued voucher bills,
	// oldest first. A voided voucher bills nothing, so its session is listed again.
	UnbilledSessions(ctx context.Context, from, to time.Time, limit int32) ([]billing.UnbilledSession, error)
	// Professional returns an active professional of the practice, or
	// billing.ErrProfessionalNotVisible.
	Professional(ctx context.Context, userID string) (billing.Professional, error)
	// Professionals lists the active professionals of the practice, by name.
	Professionals(ctx context.Context) ([]billing.Professional, error)
}

// VoucherFilter narrows the history screen: a date window, an optional patient,
// and an optional status. Empty bounds mean unbounded.
type VoucherFilter struct {
	From      time.Time
	To        time.Time
	PatientID string
	Status    string
	Limit     int32
	Offset    int32
}

// SettlementRepository persists the monthly settlements to the mutuales. A settlement and its
// sessions are written in the caller's tenant transaction, so a settlement exists whole or not at all.
type SettlementRepository interface {
	// Insurer reads a mutual of the practice, active or not. A mutual the practice does not hold
	// is billing.ErrInsurerNotVisible.
	Insurer(ctx context.Context, insurerID string) (billing.SettlementInsurer, error)
	// Insurers lists the active mutuales of the practice, by name.
	Insurers(ctx context.Context) ([]billing.SettlementInsurer, error)
	// SettlableSessions returns the attended sessions of the mutual's patients that start in
	// [from, to) and that no active settlement liquidates, oldest first.
	SettlableSessions(ctx context.Context, insurerID string, from, to time.Time) ([]billing.SettlableSession, error)
	// Issue writes the settlement and its sessions. A session already in an active settlement is
	// billing.ErrAppointmentAlreadySettled.
	Issue(ctx context.Context, settlement billing.Settlement) (billing.Settlement, error)
	// Void records the void of an issued settlement and releases its sessions.
	Void(ctx context.Context, settlement billing.Settlement) (billing.Settlement, error)
	// ByID reads one settlement with its sessions, or billing.ErrNotFound.
	ByID(ctx context.Context, id billing.SettlementID) (billing.Settlement, error)
	// List returns one page of settlements, newest first, each with its sessions.
	List(ctx context.Context, filter SettlementFilter) ([]billing.Settlement, error)
}

// SettlementFilter narrows the settlements list to one mutual, when InsurerID is set.
type SettlementFilter struct {
	InsurerID string
	Limit     int32
	Offset    int32
}

// SavedReportItem is a saved report with the names its list shows: who saved it, and the
// professional and the mutual it filters by.
type SavedReportItem struct {
	Report           report.SavedReport
	CreatedByName    string
	ProfessionalName string
	InsurerName      string
}

// SavedReportRepository persists the saved reports of a practice. They are shared: every reader
// of the practice's reports lists the same ones.
type SavedReportRepository interface {
	// Create stores a saved report. A name the practice already uses is report.ErrSavedNameTaken.
	Create(ctx context.Context, saved report.SavedReport) error
	// ByID reads one saved report of the practice, or report.ErrSavedNotFound.
	ByID(ctx context.Context, id report.SavedID) (report.SavedReport, error)
	// List returns every saved report of the practice, by name, with the names the list shows.
	List(ctx context.Context) ([]SavedReportItem, error)
	// Update rewrites what a saved report asks. Its author and creation time are not touched.
	Update(ctx context.Context, saved report.SavedReport) error
	// Delete removes a saved report, or answers report.ErrSavedNotFound when the practice has none.
	Delete(ctx context.Context, id report.SavedID) error
}

// ReportRepository reads the lines the billing report adds up. It only reads: the figures are
// computed by the report package and the application.
type ReportRepository interface {
	// BilledLines returns the issued vouchers of the practice issued in the window, narrowed by
	// the filter. A voided voucher bills nothing and is not in it. A voucher that carries no photo
	// of the cut it is filtered by does not match that filter.
	BilledLines(ctx context.Context, window report.Window, filter report.Filter) ([]report.BilledLine, error)
	// PendingSessions returns the attended sessions that start in the window and that no issued
	// voucher bills, narrowed by the filter. The coverage, mutual and currency are the patient's
	// current ones: they are what a voucher issued today would carry.
	PendingSessions(ctx context.Context, window report.Window, filter report.Filter) ([]report.Line, error)
}

// LandingRepository persists the public landing of a practice. It runs inside a tenant scope
// like the other tenant tables: the scope names the practice, so Get and Save take no tenant
// argument of their own. It maps a practice that has not written its landing yet onto
// landing.ErrNotFound.
type LandingRepository interface {
	Get(ctx context.Context) (landing.Profile, error)
	// Save writes the landing of the practice, creating it on first save.
	Save(ctx context.Context, subject landing.Profile) error
}

// TenantDomainRepository resolves the hosts a practice answers on that are not its slug
// subdomain. It is not tenant-scoped: the host is what identifies the practice, and the
// public landing reads it before any tenant context exists. It answers tenant.ErrNotFound
// for a host it does not know.
type TenantDomainRepository interface {
	TenantIDByHost(ctx context.Context, host string) (tenant.ID, error)
}
