package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	clinicalapp "github.com/niagga/aveniq/backend/internal/application/clinical"
	invitationapp "github.com/niagga/aveniq/backend/internal/application/invitation"
	patientapp "github.com/niagga/aveniq/backend/internal/application/patient"
	"github.com/niagga/aveniq/backend/internal/application/port"
	signupapp "github.com/niagga/aveniq/backend/internal/application/signup"
	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	userapp "github.com/niagga/aveniq/backend/internal/application/user"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	httpiface "github.com/niagga/aveniq/backend/internal/interfaces/http"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// The HTTP layer was the last thing in the project verified only by hand. These
// tests drive the real router — real huma registration, real guard, real error
// mapping — over doubles for the ports, so a regression in a status code or in the
// tenant the handlers read fails in CI instead of in a curl transcript.

const testSecret = "http-layer-test-signing-key"

// ─── doubles ─────────────────────────────────────────────────────────────────

// tenantContextKey is how the fake scope tells the fake repository which tenant the
// unit of work belongs to. The real ones agree through app.tenant_id; here the
// agreement is the same idea with less moving parts.
type tenantContextKey struct{}

type fakeScope struct{}

func (fakeScope) WithTenant(ctx context.Context, tenantID string, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, tenantContextKey{}, tenantID))
}

type fakePatientRepository struct {
	rows map[string]map[patient.ID]patient.Patient
}

func newFakePatientRepository() *fakePatientRepository {
	return &fakePatientRepository{rows: map[string]map[patient.ID]patient.Patient{}}
}

func scopedTenant(ctx context.Context) (string, bool) {
	tenantID, ok := ctx.Value(tenantContextKey{}).(string)
	return tenantID, ok
}

func (r *fakePatientRepository) Create(ctx context.Context, subject patient.Patient) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return patient.ErrTenantScopeMissing
	}

	for _, existing := range r.rows[tenantID] {
		if existing.Document != nil && subject.Document != nil &&
			existing.Document.Normalized == subject.Document.Normalized {
			return patient.ErrDocumentTaken
		}
	}
	if r.rows[tenantID] == nil {
		r.rows[tenantID] = map[patient.ID]patient.Patient{}
	}
	r.rows[tenantID][subject.ID] = subject
	return nil
}

func (r *fakePatientRepository) ByID(ctx context.Context, id patient.ID) (patient.Patient, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return patient.Patient{}, patient.ErrTenantScopeMissing
	}
	subject, found := r.rows[tenantID][id]
	if !found {
		return patient.Patient{}, patient.ErrNotFound
	}
	return subject, nil
}

func (r *fakePatientRepository) List(ctx context.Context, filter port.PatientFilter) ([]patient.Patient, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, patient.ErrTenantScopeMissing
	}

	subjects := make([]patient.Patient, 0, len(r.rows[tenantID]))
	for _, subject := range r.rows[tenantID] {
		if filter.ActiveOnly && !subject.IsActive {
			continue
		}
		if filter.Search != "" && !strings.Contains(subject.FullName, filter.Search) {
			continue
		}
		// The clinical grants are not modelled here: the adapter's query is covered by the
		// Postgres tests. The treating professional alone decides the narrowing in this double.
		if filter.OwnOnly && (subject.TreatingProfessionalID == nil || *subject.TreatingProfessionalID != filter.ViewerID) {
			continue
		}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// AssignTreating stores the treating professional the way the adapter does: as a row write.
func (r *fakePatientRepository) AssignTreating(ctx context.Context, subject patient.Patient) error {
	return r.Update(ctx, subject)
}

// fakeGrantRepository keeps grants in memory. It enforces the one live grant per patient and
// professional that the partial unique index enforces in the database.
type fakeGrantRepository struct {
	rows []care.Grant
}

func newFakeGrantRepository() *fakeGrantRepository { return &fakeGrantRepository{} }

func (g *fakeGrantRepository) Create(_ context.Context, grant care.Grant) error {
	for _, row := range g.rows {
		if row.Live() && row.PatientID == grant.PatientID && row.ProfessionalUserID == grant.ProfessionalUserID {
			return care.ErrGrantAlreadyLive
		}
	}
	g.rows = append(g.rows, grant)
	return nil
}

func (g *fakeGrantRepository) ByID(_ context.Context, id string) (care.Grant, error) {
	for _, row := range g.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return care.Grant{}, care.ErrGrantNotFound
}

func (g *fakeGrantRepository) HasLiveGrant(_ context.Context, patientID, professionalID string) (bool, error) {
	for _, row := range g.rows {
		if row.Live() && row.PatientID == patientID && row.ProfessionalUserID == professionalID {
			return true, nil
		}
	}
	return false, nil
}

func (g *fakeGrantRepository) ListByPatient(_ context.Context, patientID string) ([]care.Grant, error) {
	var out []care.Grant
	for _, row := range g.rows {
		if row.PatientID == patientID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (g *fakeGrantRepository) Save(_ context.Context, grant care.Grant) error {
	for i, row := range g.rows {
		if row.ID == grant.ID {
			g.rows[i] = grant
			return nil
		}
	}
	return care.ErrGrantNotFound
}

func (r *fakePatientRepository) Update(ctx context.Context, subject patient.Patient) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return patient.ErrTenantScopeMissing
	}
	if _, found := r.rows[tenantID][subject.ID]; !found {
		return patient.ErrNotFound
	}
	r.rows[tenantID][subject.ID] = subject
	return nil
}

// fakeCatalog serves the two countries the tests need and refuses the rest, which
// is how the real catalog behaves for a kind a country does not have.
type fakeCatalog struct{}

func (fakeCatalog) DocumentKind(_ context.Context, country, code string) (patient.DocumentKind, error) {
	switch country + "/" + code {
	case "AR/dni":
		return patient.DocumentKind{Country: "AR", Code: "dni", Label: "DNI", Pattern: `^[0-9]{7,8}$`, Normalizer: "digits"}, nil
	}
	return patient.DocumentKind{}, patient.ErrDocumentInvalid
}

func (fakeCatalog) TaxIDKind(_ context.Context, country, code string) (patient.DocumentKind, error) {
	switch country + "/" + code {
	case "AR/cuit":
		return patient.DocumentKind{Country: "AR", Code: "cuit", Label: "CUIT", Pattern: `^[0-9]{11}$`, Normalizer: "digits", Checksum: "ar_cuit"}, nil
	}
	return patient.DocumentKind{}, patient.ErrDocumentInvalid
}

func (fakeCatalog) FiscalCondition(_ context.Context, country, code string) (port.FiscalCondition, error) {
	if country == "AR" && code == "monotributo" {
		return port.FiscalCondition{Country: country, Code: code, Label: "Monotributo"}, nil
	}
	return port.FiscalCondition{}, patient.ErrDocumentInvalid
}

func (fakeCatalog) TenantCountry(_ context.Context, tenantID string) (string, error) {
	if tenantID == "" {
		return "", patient.ErrNotFound
	}
	return "AR", nil
}

// fakeTenantRepository answers only for the practices the tests create.
type fakeTenantRepository struct {
	byID map[tenant.ID]tenant.Tenant
}

// Create enforces the slug uniqueness the table enforces, so a signup that reuses a
// slug answers the conflict the real repository would.
func (r *fakeTenantRepository) Create(_ context.Context, subject tenant.Tenant) error {
	for _, existing := range r.byID {
		if existing.Slug == subject.Slug {
			return tenant.ErrSlugTaken
		}
	}
	r.byID[subject.ID] = subject
	return nil
}

func (r *fakeTenantRepository) BySlug(_ context.Context, slug string) (tenant.Tenant, error) {
	for _, subject := range r.byID {
		if subject.Slug == slug {
			return subject, nil
		}
	}
	return tenant.Tenant{}, tenant.ErrNotFound
}

func (r *fakeTenantRepository) ByID(_ context.Context, id tenant.ID) (tenant.Tenant, error) {
	subject, found := r.byID[id]
	if !found {
		return tenant.Tenant{}, tenant.ErrNotFound
	}
	return subject, nil
}

func (r *fakeTenantRepository) List(context.Context, int32, int32) ([]tenant.Tenant, error) {
	return nil, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type uuidIDs struct{}

func (uuidIDs) NewID() string { return uuid.NewString() }

// ─── harness ─────────────────────────────────────────────────────────────────

type harness struct {
	router  http.Handler
	tokens  *authn.AccessTokens
	repo    *fakePatientRepository
	tenants *fakeTenantRepository
	users   *fakeUserRepository
	audit   *fakeAudit

	invitations *fakeInvitationRepository
	limiter     *fakeRateLimiter
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	clock := fixedClock{now: now}

	tokens, err := authn.NewAccessTokens([]byte(testSecret), 15*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatalf("cannot build the signer: %v", err)
	}

	repo := newFakePatientRepository()
	catalog := fakeCatalog{}
	scope := fakeScope{}
	grants := newFakeGrantRepository()
	access := careapp.NewAccess(repo, grants)
	clinicalRecords := newFakeClinicalRepository()

	tenants := &fakeTenantRepository{byID: map[tenant.ID]tenant.Tenant{}}
	for _, slug := range []string{"practica-a", "practica-b"} {
		subject, err := tenant.New(tenant.ID(uuid.NewString()), slug, slug, now)
		if err != nil {
			t.Fatalf("cannot build the tenant: %v", err)
		}
		tenants.byID[subject.ID] = subject
	}

	users := newFakeUserRepository()
	audit := &fakeAudit{}
	invitations := newFakeInvitationRepository()
	limiter := &fakeRateLimiter{hits: map[string]int64{}}

	router := httpiface.NewRouter(httpiface.Deps{
		Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServiceVersion:         "test",
		Signup:                 signupapp.NewUseCase(tenants, users, scope, audit, clock, uuidIDs{}, fakeHasher{}),
		RateLimiter:            limiter,
		GetTenantByID:          tenantapp.NewGetByIDUseCase(tenants),
		CreatePatient:          patientapp.NewCreateUseCase(repo, catalog, scope, clock, uuidIDs{}),
		UpdatePatient:          patientapp.NewUpdateUseCase(repo, catalog, scope, clock, access),
		DeactivatePatient:      patientapp.NewDeactivateUseCase(repo, scope, clock, access),
		ActivatePatient:        patientapp.NewActivateUseCase(repo, scope, clock, access),
		GetPatient:             patientapp.NewGetUseCase(scope, access),
		AssignTreating:         careapp.NewAssignTreatingUseCase(repo, users, scope, audit, clock, access),
		GrantClinical:          careapp.NewGrantClinicalUseCase(grants, users, scope, audit, clock, uuidIDs{}, access),
		ListGrants:             careapp.NewListGrantsUseCase(grants, scope, access),
		RevokeGrant:            careapp.NewRevokeGrantUseCase(grants, scope, audit, clock, access),
		GetClinicalRecord:      clinicalapp.NewGetRecordUseCase(clinicalRecords, scope, audit, access),
		SaveClinicalRecord:     clinicalapp.NewSaveRecordUseCase(clinicalRecords, scope, audit, clock, uuidIDs{}, access),
		ListPatients:           patientapp.NewListUseCase(repo, scope),
		AccessTokens:           tokens,
		RefreshCookie:          httpiface.DefaultRefreshCookieConfig(30*24*time.Hour, false),
		ListUsers:              userapp.NewListUseCase(users, scope),
		GetUser:                userapp.NewGetUseCase(users, scope),
		ChangeUserRole:         userapp.NewChangeRoleUseCase(users, scope, audit, clock),
		DeactivateUser:         userapp.NewDeactivateUseCase(users, scope, audit, clock),
		ActivateUser:           userapp.NewActivateUseCase(users, scope, audit, clock),
		CreateInvitation:       invitationapp.NewCreateUseCase(invitations, users, scope, audit, clock, uuidIDs{}),
		ListPendingInvitations: invitationapp.NewListPendingUseCase(invitations, scope, clock),
		RevokeInvitation:       invitationapp.NewRevokeUseCase(invitations, scope, audit),
		PreviewInvitation:      invitationapp.NewPreviewUseCase(invitations, scope, clock),
		AcceptInvitation:       invitationapp.NewAcceptUseCase(invitations, users, scope, audit, clock, uuidIDs{}, fakeHasher{}),
	})

	return &harness{router: router, tokens: tokens, repo: repo, tenants: tenants, users: users, audit: audit, invitations: invitations, limiter: limiter}
}

// token mints a signed access token for a role and a practice. It bypasses the
// login use case on purpose: the token is the contract the guard reads.
func (h *harness) token(t *testing.T, tenantID string, role iam.RoleCode) string {
	t.Helper()

	token, err := h.tokens.Issue(authn.AccessClaims{
		Subject:  uuid.NewString(),
		TenantID: tenantID,
		Role:     string(role),
	})
	if err != nil {
		t.Fatalf("cannot issue a token: %v", err)
	}
	return token
}

func (h *harness) tenantID(t *testing.T, slug string) string {
	t.Helper()
	for id, subject := range h.tenants.byID {
		if subject.Slug == slug {
			return string(id)
		}
	}
	t.Fatalf("unknown tenant %q", slug)
	return ""
}

// do performs a request and returns the status and the decoded body.
func (h *harness) do(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("cannot encode the request: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}

	request := httptest.NewRequest(method, path, payload)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, request)

	decoded := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestPatientRoutesRequireAuthentication(t *testing.T) {
	h := newHarness(t)

	for _, tc := range []struct{ name, method, path string }{
		{"list", http.MethodGet, "/api/v1/patients"},
		{"create", http.MethodPost, "/api/v1/patients"},
		{"tenants", http.MethodGet, "/api/v1/tenants"},
		{"me", http.MethodGet, "/api/v1/auth/me"},
	} {
		t.Run(tc.name+" without a token", func(t *testing.T) {
			status, _ := h.do(t, tc.method, tc.path, "", nil)
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", status)
			}
		})
	}

	t.Run("a forged token is refused", func(t *testing.T) {
		other, err := authn.NewAccessTokens([]byte("a-completely-different-secret"), time.Minute, time.Now)
		if err != nil {
			t.Fatalf("cannot build the second signer: %v", err)
		}
		forged, err := other.Issue(authn.AccessClaims{Subject: uuid.NewString(), TenantID: uuid.NewString(), Role: "owner"})
		if err != nil {
			t.Fatalf("cannot issue: %v", err)
		}

		status, _ := h.do(t, http.MethodGet, "/api/v1/patients", forged, nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401: a token signed by another key is not a token", status)
		}
	})
}

func TestDomainRoutesAreGuardedByPermission(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	clinicalPath := "/api/v1/patients/" + uuid.NewString() + "/clinical-record"

	accountant := h.token(t, tenantA, iam.RoleAccountant)
	for _, path := range []string{"/api/v1/patients", "/api/v1/tenants", clinicalPath, "/api/v1/appointments"} {
		status, body := h.do(t, http.MethodGet, path, accountant, nil)
		if status != http.StatusForbidden {
			t.Fatalf("%s with accountant = %d, want 403", path, status)
		}
		if _, leaked := body["items"]; leaked {
			t.Fatalf("%s answered a body to a forbidden role: %v", path, body)
		}
	}

	assistant := h.token(t, tenantA, iam.RoleAssistant)
	if status, _ := h.do(t, http.MethodGet, "/api/v1/patients", assistant, nil); status != http.StatusOK {
		t.Fatalf("assistant on patients = %d, want 200: identification is shared", status)
	}
	if status, body := h.do(t, http.MethodGet, clinicalPath, assistant, nil); status != http.StatusForbidden {
		t.Fatalf("assistant on clinical = %d, want 403: the assistant must not read HC", status)
	} else if _, leaked := body["chiefComplaint"]; leaked {
		t.Fatalf("assistant received a clinical body: %v", body)
	}

	admin := h.token(t, tenantA, iam.RoleAdmin)
	if status, _ := h.do(t, http.MethodGet, "/api/v1/patients", admin, nil); status != http.StatusOK {
		t.Fatalf("admin on patients = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodGet, clinicalPath, admin, nil); status != http.StatusForbidden {
		t.Fatalf("admin on clinical = %d, want 403: admin has no clinical:* permission", status)
	}

	owner := h.token(t, tenantA, iam.RoleOwner)
	if status, _ := h.do(t, http.MethodGet, "/api/v1/patients", owner, nil); status != http.StatusOK {
		t.Fatalf("owner on patients = %d, want 200", status)
	}
}

// TestTenantComesFromTheToken is the cross-tenant leak test T16 exists for: a
// practice must not reach another practice's patients by any input it controls.
func TestTenantComesFromTheToken(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")

	ownerA := h.token(t, tenantA, iam.RoleOwner)
	ownerB := h.token(t, tenantB, iam.RoleOwner)

	status, created := h.do(t, http.MethodPost, "/api/v1/patients", ownerA, map[string]any{
		"fullName":       "Ana de la práctica A",
		"documentKind":   "dni",
		"documentNumber": "30111222",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %v", status, created)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("the create answered no id: %v", created)
	}

	t.Run("the other practice cannot read it by id", func(t *testing.T) {
		status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, ownerB, nil)
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: a foreign patient must look like an absent one", status)
		}
	})

	t.Run("the other practice does not see it in its list", func(t *testing.T) {
		status, body := h.do(t, http.MethodGet, "/api/v1/patients", ownerB, nil)
		if status != http.StatusOK {
			t.Fatalf("list = %d, want 200", status)
		}
		items, _ := body["items"].([]any)
		if len(items) != 0 {
			t.Fatalf("tenant B sees %d patients of tenant A", len(items))
		}
	})

	t.Run("the owner reads it", func(t *testing.T) {
		status, body := h.do(t, http.MethodGet, "/api/v1/patients/"+id, ownerA, nil)
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		if body["fullName"] != "Ana de la práctica A" {
			t.Fatalf("fullName = %v", body["fullName"])
		}
	})
}

func TestPatientRouteErrors(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)

	t.Run("a document the country does not have is 422 with the field", func(t *testing.T) {
		status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{
			"fullName":       "Con NIE argentino",
			"documentKind":   "nie",
			"documentNumber": "X1234567L",
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", status)
		}
		detail, _ := body["detail"].(string)
		if !strings.Contains(detail, "document") {
			t.Fatalf("detail = %q, want it to name the offending field", detail)
		}
	})

	created := func(t *testing.T, fullName, document string) string {
		t.Helper()
		status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{
			"fullName":       fullName,
			"documentKind":   "dni",
			"documentNumber": document,
		})
		if status != http.StatusCreated {
			t.Fatalf("create = %d: %v", status, body)
		}
		id, _ := body["id"].(string)
		return id
	}

	t.Run("the same document twice is 409", func(t *testing.T) {
		created(t, "Duplicada", "27111222")
		status, _ := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{
			"fullName":       "Duplicada otra vez",
			"documentKind":   "dni",
			"documentNumber": "27.111.222",
		})
		if status != http.StatusConflict {
			t.Fatalf("status = %d, want 409: the normalized document is what collides", status)
		}
	})

	t.Run("an unknown id is 404", func(t *testing.T) {
		status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+uuid.NewString(), owner, nil)
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
	})

	t.Run("a second deactivation is 409", func(t *testing.T) {
		id := created(t, "Se va dos veces", "28111222")

		if status, _ := h.do(t, http.MethodPost, "/api/v1/patients/"+id+"/deactivate", owner, nil); status != http.StatusOK {
			t.Fatalf("first deactivate = %d, want 200", status)
		}
		status, _ := h.do(t, http.MethodPost, "/api/v1/patients/"+id+"/deactivate", owner, nil)
		if status != http.StatusConflict {
			t.Fatalf("second deactivate = %d, want 409", status)
		}
	})

	t.Run("editing a deactivated patient is 409", func(t *testing.T) {
		id := created(t, "Inactiva", "29111222")
		if status, _ := h.do(t, http.MethodPost, "/api/v1/patients/"+id+"/deactivate", owner, nil); status != http.StatusOK {
			t.Fatalf("deactivate = %d, want 200", status)
		}

		status, _ := h.do(t, http.MethodPatch, "/api/v1/patients/"+id, owner, map[string]any{
			"fullName":       "Nombre nuevo",
			"documentKind":   "dni",
			"documentNumber": "29111222",
		})
		if status != http.StatusConflict {
			t.Fatalf("patch = %d, want 409", status)
		}
	})

	t.Run("a page size over the maximum is rejected by the schema", func(t *testing.T) {
		status, _ := h.do(t, http.MethodGet, "/api/v1/patients?limit=500", owner, nil)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", status)
		}
	})
}

func TestWhoamiDescribesTheTokenIdentity(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)

	status, body := h.do(t, http.MethodGet, "/api/v1/auth/me", owner, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["tenantId"] != tenantA {
		t.Fatalf("tenantId = %v, want %s: the answer comes from the token", body["tenantId"], tenantA)
	}
	if body["role"] != string(iam.RoleOwner) {
		t.Fatalf("role = %v, want owner", body["role"])
	}
}
