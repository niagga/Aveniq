package patientapp_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	patientapp "github.com/niagga/aveniq/backend/internal/application/patient"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// The tests run entirely on in-memory doubles: no database, no tenant scope
// implementation. What they fix is the contract the HTTP layer will code against,
// including the ordering rule that normalization happens before the tenant
// transaction opens.

// ─── doubles ─────────────────────────────────────────────────────────────────

type fakePatientRepository struct {
	stored     map[patient.ID]patient.Patient
	listFilter port.PatientFilter
}

func newFakePatientRepository() *fakePatientRepository {
	return &fakePatientRepository{stored: make(map[patient.ID]patient.Patient)}
}

func (f *fakePatientRepository) Create(_ context.Context, subject patient.Patient) error {
	if _, exists := f.stored[subject.ID]; exists {
		return patient.ErrDocumentTaken
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakePatientRepository) ByID(_ context.Context, id patient.ID) (patient.Patient, error) {
	subject, ok := f.stored[id]
	if !ok {
		return patient.Patient{}, patient.ErrNotFound
	}
	return subject, nil
}

func (f *fakePatientRepository) Update(_ context.Context, subject patient.Patient) error {
	if _, ok := f.stored[subject.ID]; !ok {
		return patient.ErrNotFound
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakePatientRepository) List(_ context.Context, filter port.PatientFilter) ([]patient.Patient, error) {
	f.listFilter = filter
	rows := make([]patient.Patient, 0, len(f.stored))
	for _, subject := range f.stored {
		rows = append(rows, subject)
	}
	return rows, nil
}

// fakeDocumentCatalog only carries the catalog rows the tests use, shaped like
// the seed of migration 0005. Unknown codes are refused the same way the
// PostgreSQL adapter refuses them: patient.ErrDocumentInvalid.
type fakeDocumentCatalog struct {
	docKinds           map[string]patient.DocumentKind
	taxKinds           map[string]patient.DocumentKind
	fiscal             map[string]port.FiscalCondition
	countries          map[string]string
	tenantCountryCalls int
}

func newFakeDocumentCatalog() *fakeDocumentCatalog {
	return &fakeDocumentCatalog{
		docKinds: map[string]patient.DocumentKind{
			"AR/dni": {Country: "AR", Code: "dni", Label: "DNI", Pattern: "^[0-9]{7,8}$", Normalizer: "digits"},
			"ES/nie": {Country: "ES", Code: "nie", Label: "NIE", Pattern: "^[XYZ][0-9]{7}[A-Z]$", Normalizer: "alnum_upper", Checksum: "es_nie"},
		},
		taxKinds: map[string]patient.DocumentKind{
			"AR/cuit": {Country: "AR", Code: "cuit", Label: "CUIT", Pattern: "^[0-9]{11}$", Normalizer: "digits", Checksum: "ar_cuit"},
		},
		fiscal: map[string]port.FiscalCondition{
			"AR/consumidor_final": {Country: "AR", Code: "consumidor_final", Label: "Consumidor Final"},
			"AR/monotributo":      {Country: "AR", Code: "monotributo", Label: "Monotributo"},
		},
		countries: map[string]string{
			"tenant-1":    "AR",
			"tenant-es":   "ES",
			"tenant-miss": "PT",
		},
	}
}

func (f *fakeDocumentCatalog) DocumentKind(_ context.Context, country, code string) (patient.DocumentKind, error) {
	kind, ok := f.docKinds[country+"/"+code]
	if !ok {
		return patient.DocumentKind{}, fmt.Errorf("%w: %s has no document kind %q", patient.ErrDocumentInvalid, country, code)
	}
	return kind, nil
}

func (f *fakeDocumentCatalog) TaxIDKind(_ context.Context, country, code string) (patient.DocumentKind, error) {
	kind, ok := f.taxKinds[country+"/"+code]
	if !ok {
		return patient.DocumentKind{}, fmt.Errorf("%w: %s has no tax id kind %q", patient.ErrDocumentInvalid, country, code)
	}
	return kind, nil
}

func (f *fakeDocumentCatalog) FiscalCondition(_ context.Context, country, code string) (port.FiscalCondition, error) {
	row, ok := f.fiscal[country+"/"+code]
	if !ok {
		return port.FiscalCondition{}, fmt.Errorf("%w: %s has no fiscal condition %q", patient.ErrDocumentInvalid, country, code)
	}
	return row, nil
}

func (f *fakeDocumentCatalog) TenantCountry(_ context.Context, tenantID string) (string, error) {
	f.tenantCountryCalls++
	country, ok := f.countries[tenantID]
	if !ok {
		return "", patient.ErrNotFound
	}
	return country, nil
}

// fakeScope runs the unit of work directly and records every opening, which is
// how the tests prove the scope discipline: exactly one open per operation, and
// none before normalization finishes.
type fakeScope struct {
	opened  int
	tenants []string
}

func (s *fakeScope) WithTenant(ctx context.Context, tenantID string, fn func(ctx context.Context) error) error {
	s.opened++
	s.tenants = append(s.tenants, tenantID)
	return fn(ctx)
}

type fixedClock struct{ now time.Time }

// Now returns the current instant and steps one minute forward, so a use case
// that runs later observes a strictly later time: "updated_at moved" has to be
// observable, and a frozen clock cannot show it.
func (c *fixedClock) Now() time.Time {
	now := c.now
	c.now = c.now.Add(time.Minute)
	return now
}

type seqIDs struct{ n int }

func (s *seqIDs) NewID() string { s.n++; return fmt.Sprintf("patient-%06d", s.n) }

// ─── world ───────────────────────────────────────────────────────────────────

const start = "2025-06-15T10:30:00Z"

type world struct {
	repo       *fakePatientRepository
	catalog    *fakeDocumentCatalog
	scope      *fakeScope
	create     *patientapp.CreateUseCase
	update     *patientapp.UpdateUseCase
	deactivate *patientapp.DeactivateUseCase
	activate   *patientapp.ActivateUseCase
	get        *patientapp.GetUseCase
	list       *patientapp.ListUseCase
}

func newWorld(t *testing.T) *world {
	t.Helper()
	repo := newFakePatientRepository()
	catalog := newFakeDocumentCatalog()
	scope := &fakeScope{}
	now, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatalf("parse fixture time: %v", err)
	}
	clock := &fixedClock{now: now}
	ids := &seqIDs{}

	return &world{
		repo:       repo,
		catalog:    catalog,
		scope:      scope,
		create:     patientapp.NewCreateUseCase(repo, catalog, scope, clock, ids),
		update:     patientapp.NewUpdateUseCase(repo, catalog, scope, clock),
		deactivate: patientapp.NewDeactivateUseCase(repo, scope, clock),
		activate:   patientapp.NewActivateUseCase(repo, scope, clock),
		get:        patientapp.NewGetUseCase(repo, scope),
		list:       patientapp.NewListUseCase(repo, scope),
	}
}

func (w *world) createPatient(t *testing.T, mutate func(*patientapp.CreateInput)) patientapp.PatientView {
	t.Helper()
	in := patientapp.CreateInput{TenantID: "tenant-1", FullName: "Ana García"}
	if mutate != nil {
		mutate(&in)
	}
	view, err := w.create.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}
	return view
}

func assertOneScope(t *testing.T, w *world, tenantID string) {
	t.Helper()
	if w.scope.opened != 1 {
		t.Fatalf("tenant scope openings = %d, want exactly 1", w.scope.opened)
	}
	if len(w.scope.tenants) != 1 || w.scope.tenants[0] != tenantID {
		t.Fatalf("scope opened for %v, want [%s]", w.scope.tenants, tenantID)
	}
}

// ─── create ──────────────────────────────────────────────────────────────────

func TestCreateResolvesCountryFromPractice(t *testing.T) {
	w := newWorld(t)

	view, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:        "tenant-1",
		FullName:        "Ana García",
		DocumentKind:    "dni",
		DocumentNumber:  "30.111.222",
		FiscalCondition: "consumidor_final",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if view.Country != "AR" {
		t.Fatalf("country = %q, want the practice's AR", view.Country)
	}
	if w.catalog.tenantCountryCalls != 1 {
		t.Fatalf("TenantCountry calls = %d, want 1", w.catalog.tenantCountryCalls)
	}
	stored, ok := w.repo.stored[patient.ID(view.ID)]
	if !ok {
		t.Fatalf("patient was not persisted")
	}
	if stored.Country != "AR" {
		t.Fatalf("stored country = %q, want AR", stored.Country)
	}
	if !view.IsActive {
		t.Fatalf("a new patient must be active")
	}
	if view.CreatedAt != start {
		t.Fatalf("createdAt = %q, want %q", view.CreatedAt, start)
	}
}

func TestCreateWithExplicitCountrySkipsTenantLookup(t *testing.T) {
	w := newWorld(t)

	view, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:       "tenant-es",
		FullName:       "Ana García",
		Country:        "ES",
		DocumentKind:   "nie",
		DocumentNumber: "x1234567l",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if view.Country != "ES" {
		t.Fatalf("country = %q, want the requested ES", view.Country)
	}
	if w.catalog.tenantCountryCalls != 0 {
		t.Fatalf("TenantCountry calls = %d, want 0 when the request chooses a country", w.catalog.tenantCountryCalls)
	}
	stored := w.repo.stored[patient.ID(view.ID)]
	if stored.Document == nil || stored.Document.Normalized != "X1234567L" {
		t.Fatalf("normalized document = %+v, want X1234567L", stored.Document)
	}
}

func TestCreateKeepsTypedDocumentAndStoresCanonicalForm(t *testing.T) {
	w := newWorld(t)

	view, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:       "tenant-1",
		FullName:       "Ana García",
		DocumentKind:   "dni",
		DocumentNumber: "30.111.222",
		TaxIDKind:      "cuit",
		TaxIDNumber:    "20-12345678-6",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if view.Document == nil {
		t.Fatalf("document view must be present")
	}
	if view.Document.Kind != "dni" || view.Document.Number != "30.111.222" || view.Document.Country != "AR" {
		t.Fatalf("document view = %+v, want the typed form with kind and country", view.Document)
	}
	if view.TaxID == nil || view.TaxID.Kind != "cuit" || view.TaxID.Number != "20-12345678-6" {
		t.Fatalf("tax id view = %+v, want the typed form with kind", view.TaxID)
	}

	stored := w.repo.stored[patient.ID(view.ID)]
	if stored.Document.Normalized != "30111222" {
		t.Fatalf("stored normalized document = %q, want 30111222", stored.Document.Normalized)
	}
	if stored.TaxID.Normalized != "20123456786" {
		t.Fatalf("stored normalized tax id = %q, want 20123456786", stored.TaxID.Normalized)
	}

	// The same document without separators is the same canonical number: that is
	// the whole point of normalizing before storing.
	second, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:       "tenant-1",
		FullName:       "Beto López",
		DocumentKind:   "dni",
		DocumentNumber: "30111222",
	})
	if err != nil {
		t.Fatalf("create without separators: %v", err)
	}
	if stored := w.repo.stored[patient.ID(second.ID)]; stored.Document.Normalized != "30111222" {
		t.Fatalf("second normalized document = %q, want 30111222", stored.Document.Normalized)
	}
}

func TestCreateRefusesDocumentKindTheCountryDoesNotHave(t *testing.T) {
	w := newWorld(t)

	_, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:       "tenant-1",
		FullName:       "Ana García",
		DocumentKind:   "nie",
		DocumentNumber: "X1234567L",
	})
	if !errors.Is(err, patient.ErrDocumentInvalid) {
		t.Fatalf("error = %v, want patient.ErrDocumentInvalid", err)
	}
	if len(w.repo.stored) != 0 {
		t.Fatalf("a refused create must not persist anything")
	}
	if w.scope.opened != 0 {
		t.Fatalf("normalization must happen before the tenant transaction; scope opened %d times", w.scope.opened)
	}
}

func TestCreateRefusesFiscalConditionTheCountryDoesNotHave(t *testing.T) {
	w := newWorld(t)

	_, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:        "tenant-es",
		FullName:        "Ana García",
		FiscalCondition: "monotributo",
	})
	if !errors.Is(err, patient.ErrDocumentInvalid) {
		t.Fatalf("error = %v, want patient.ErrDocumentInvalid", err)
	}
	if len(w.repo.stored) != 0 {
		t.Fatalf("a refused create must not persist anything")
	}
}

func TestCreateReturnsDomainValidationErrorUntouched(t *testing.T) {
	w := newWorld(t)

	// A document whose normalized form is blank: the domain's field error must
	// reach the caller as-is, so the HTTP layer can answer 422 with a field.
	_, err := w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:       "tenant-1",
		FullName:       "Ana García",
		DocumentKind:   "dni",
		DocumentNumber: "sin números",
	})
	var invalid *patient.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v (%T), want *patient.ValidationError untouched", err, err)
	}
	if invalid.Field != "document_number" {
		t.Fatalf("field = %q, want document_number", invalid.Field)
	}

	// A check-digit failure is the document sentinel, not a field error.
	_, err = w.create.Execute(context.Background(), patientapp.CreateInput{
		TenantID:    "tenant-1",
		FullName:    "Ana García",
		TaxIDKind:   "cuit",
		TaxIDNumber: "20-12345678-5",
	})
	if !errors.Is(err, patient.ErrDocumentInvalid) {
		t.Fatalf("error = %v, want patient.ErrDocumentInvalid", err)
	}
	if errors.As(err, &invalid) {
		t.Fatalf("a checksum failure must not be reported as a field error")
	}

	if len(w.repo.stored) != 0 {
		t.Fatalf("a refused create must not persist anything")
	}
}

func TestCreateOpensExactlyOneTenantScope(t *testing.T) {
	w := newWorld(t)

	w.createPatient(t, nil)

	assertOneScope(t, w, "tenant-1")
}

// ─── update ──────────────────────────────────────────────────────────────────

func TestUpdateAppliesProfileAndKeepsCreationUntouched(t *testing.T) {
	w := newWorld(t)
	view := w.createPatient(t, nil)

	updated, err := w.update.Execute(context.Background(), patientapp.UpdateInput{
		TenantID:       "tenant-1",
		ID:             view.ID,
		FullName:       "Ana García López",
		Phone:          "  5411 4444  ",
		Email:          "ANA@Example.com",
		DocumentKind:   "dni",
		DocumentNumber: "30.111.222",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	stored := w.repo.stored[patient.ID(view.ID)]
	if stored.FullName != "Ana García López" {
		t.Fatalf("full name = %q, want the edited one", stored.FullName)
	}
	if stored.Phone == nil || *stored.Phone != "5411 4444" {
		t.Fatalf("phone = %v, want trimmed", stored.Phone)
	}
	if stored.Email == nil || *stored.Email != "ana@example.com" {
		t.Fatalf("email = %v, want lowercased", stored.Email)
	}
	if stored.Document == nil || stored.Document.Normalized != "30111222" {
		t.Fatalf("document = %+v, want the normalized edit", stored.Document)
	}
	if stored.CreatedAt.Format(time.RFC3339) != view.CreatedAt {
		// Creation time is identity, not profile: the edit must not move it.
		t.Fatalf("createdAt moved from %q to %v", view.CreatedAt, stored.CreatedAt)
	}
	if stored.UpdatedAt.Equal(stored.CreatedAt) {
		t.Fatalf("updated_at must move on an edit")
	}
	if updated.CreatedAt != view.CreatedAt {
		t.Fatalf("createdAt in the view changed from %q to %q", view.CreatedAt, updated.CreatedAt)
	}
}

func TestUpdateMissingPatientIsNotFound(t *testing.T) {
	w := newWorld(t)

	_, err := w.update.Execute(context.Background(), patientapp.UpdateInput{
		TenantID: "tenant-1",
		ID:       "patient-missing",
		FullName: "Alguien",
	})
	if !errors.Is(err, patient.ErrNotFound) {
		t.Fatalf("error = %v, want patient.ErrNotFound", err)
	}
}

// ─── lifecycle ───────────────────────────────────────────────────────────────

func TestDeactivateThenActivateRoundTrip(t *testing.T) {
	w := newWorld(t)
	view := w.createPatient(t, nil)

	off, err := w.deactivate.Execute(context.Background(), patientapp.DeactivateInput{TenantID: "tenant-1", ID: view.ID})
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if off.IsActive {
		t.Fatalf("patient must be inactive after deactivation")
	}
	if w.repo.stored[patient.ID(view.ID)].IsActive {
		t.Fatalf("the stored row must keep is_active = false")
	}

	on, err := w.activate.Execute(context.Background(), patientapp.ActivateInput{TenantID: "tenant-1", ID: view.ID})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if !on.IsActive {
		t.Fatalf("patient must be active after activation")
	}

	// Double transitions are refused by the domain, and the use case does not
	// swallow them.
	if _, err := w.activate.Execute(context.Background(), patientapp.ActivateInput{TenantID: "tenant-1", ID: view.ID}); !errors.Is(err, patient.ErrInvalidTransition) {
		t.Fatalf("second activate error = %v, want patient.ErrInvalidTransition", err)
	}

	if _, err := w.deactivate.Execute(context.Background(), patientapp.DeactivateInput{TenantID: "tenant-1", ID: view.ID}); err != nil {
		t.Fatalf("deactivate after activate: %v", err)
	}
	if _, err := w.deactivate.Execute(context.Background(), patientapp.DeactivateInput{TenantID: "tenant-1", ID: view.ID}); !errors.Is(err, patient.ErrInvalidTransition) {
		t.Fatalf("second deactivate error = %v, want patient.ErrInvalidTransition", err)
	}
}

func TestLifecycleOfMissingPatientIsNotFound(t *testing.T) {
	w := newWorld(t)

	if _, err := w.deactivate.Execute(context.Background(), patientapp.DeactivateInput{TenantID: "tenant-1", ID: "patient-missing"}); !errors.Is(err, patient.ErrNotFound) {
		t.Fatalf("deactivate error = %v, want patient.ErrNotFound", err)
	}
	if _, err := w.activate.Execute(context.Background(), patientapp.ActivateInput{TenantID: "tenant-1", ID: "patient-missing"}); !errors.Is(err, patient.ErrNotFound) {
		t.Fatalf("activate error = %v, want patient.ErrNotFound", err)
	}
}

// ─── reads ───────────────────────────────────────────────────────────────────

func TestGetReturnsPatientAndMissingIsNotFound(t *testing.T) {
	w := newWorld(t)
	view := w.createPatient(t, nil)

	got, err := w.get.Execute(context.Background(), patientapp.GetInput{TenantID: "tenant-1", ID: view.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != view.ID || got.FullName != view.FullName || got.TenantID != "tenant-1" {
		t.Fatalf("got = %+v, want the created patient", got)
	}

	if _, err := w.get.Execute(context.Background(), patientapp.GetInput{TenantID: "tenant-1", ID: "patient-missing"}); !errors.Is(err, patient.ErrNotFound) {
		t.Fatalf("error = %v, want patient.ErrNotFound", err)
	}
}

func TestListRejectsInvalidPagination(t *testing.T) {
	cases := []struct {
		name   string
		limit  int32
		offset int32
		field  string
	}{
		{"negative limit", -1, 0, "limit"},
		{"limit over 100", 101, 0, "limit"},
		{"negative offset", 20, -1, "offset"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)

			_, err := w.list.Execute(context.Background(), patientapp.ListInput{
				TenantID: "tenant-1", Limit: tc.limit, Offset: tc.offset,
			})

			var invalid *patient.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v (%T), want *patient.ValidationError", err, err)
			}
			if invalid.Field != tc.field {
				t.Fatalf("field = %q, want %q", invalid.Field, tc.field)
			}
			if w.scope.opened != 0 {
				t.Fatalf("a rejected page must not open a tenant scope; opened %d", w.scope.opened)
			}
		})
	}
}

func TestListAppliesDefaultPageAndPassesFilterThrough(t *testing.T) {
	w := newWorld(t)
	w.createPatient(t, nil)
	w.createPatient(t, func(in *patientapp.CreateInput) { in.FullName = "Beto López" })

	rows, err := w.list.Execute(context.Background(), patientapp.ListInput{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if w.repo.listFilter.Limit != 20 {
		t.Fatalf("limit passed to the repository = %d, want the default 20", w.repo.listFilter.Limit)
	}
	if w.repo.listFilter.Offset != 0 {
		t.Fatalf("offset passed to the repository = %d, want 0", w.repo.listFilter.Offset)
	}
	for _, row := range rows {
		if row.Country != "AR" {
			t.Fatalf("row country = %q, want the practice's AR", row.Country)
		}
	}

	_, err = w.list.Execute(context.Background(), patientapp.ListInput{
		TenantID: "tenant-1", Search: "ana", ActiveOnly: true, Limit: 5, Offset: 10,
	})
	if err != nil {
		t.Fatalf("list with filter: %v", err)
	}
	filter := w.repo.listFilter
	if filter.Search != "ana" || !filter.ActiveOnly || filter.Limit != 5 || filter.Offset != 10 {
		t.Fatalf("filter = %+v, want the requested one passed through", filter)
	}
}

// ─── scope discipline ────────────────────────────────────────────────────────

// Every repository call the port declares refuses to run outside a tenant scope,
// so reads need one exactly as much as writes do.
func TestEveryOperationOpensExactlyOneTenantScope(t *testing.T) {
	w := newWorld(t)
	view := w.createPatient(t, nil)

	w.scope.opened, w.scope.tenants = 0, nil

	if _, err := w.get.Execute(context.Background(), patientapp.GetInput{TenantID: "tenant-1", ID: view.ID}); err != nil {
		t.Fatalf("get: %v", err)
	}
	assertOneScope(t, w, "tenant-1")

	w.scope.opened, w.scope.tenants = 0, nil

	if _, err := w.update.Execute(context.Background(), patientapp.UpdateInput{
		TenantID: "tenant-1", ID: view.ID, FullName: "Ana García López",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	assertOneScope(t, w, "tenant-1")

	w.scope.opened, w.scope.tenants = 0, nil

	if _, err := w.deactivate.Execute(context.Background(), patientapp.DeactivateInput{TenantID: "tenant-1", ID: view.ID}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	assertOneScope(t, w, "tenant-1")

	w.scope.opened, w.scope.tenants = 0, nil

	if _, err := w.activate.Execute(context.Background(), patientapp.ActivateInput{TenantID: "tenant-1", ID: view.ID}); err != nil {
		t.Fatalf("activate: %v", err)
	}
	assertOneScope(t, w, "tenant-1")

	w.scope.opened, w.scope.tenants = 0, nil

	if _, err := w.list.Execute(context.Background(), patientapp.ListInput{TenantID: "tenant-1"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	assertOneScope(t, w, "tenant-1")
}
