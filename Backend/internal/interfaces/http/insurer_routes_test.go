package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
)

// fakeInsurerRepository keeps the mutual catalog in memory. Like the real adapter it
// refuses a second mutual with the same name, ignoring case and surrounding spaces.
type fakeInsurerRepository struct {
	stored map[insurer.ID]insurer.Insurer
}

var _ port.InsurerRepository = (*fakeInsurerRepository)(nil)

func newFakeInsurerRepository() *fakeInsurerRepository {
	return &fakeInsurerRepository{stored: map[insurer.ID]insurer.Insurer{}}
}

func nameKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func (f *fakeInsurerRepository) nameTaken(name string, except insurer.ID) bool {
	for id, subject := range f.stored {
		if id != except && nameKey(subject.Name) == nameKey(name) {
			return true
		}
	}
	return false
}

func (f *fakeInsurerRepository) Create(_ context.Context, subject insurer.Insurer) error {
	if f.nameTaken(subject.Name, "") {
		return insurer.ErrNameTaken
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeInsurerRepository) ByID(_ context.Context, id insurer.ID) (insurer.Insurer, error) {
	subject, ok := f.stored[id]
	if !ok {
		return insurer.Insurer{}, insurer.ErrNotFound
	}
	return subject, nil
}

func (f *fakeInsurerRepository) List(_ context.Context, activeOnly bool) ([]insurer.Insurer, error) {
	rows := []insurer.Insurer{}
	for _, subject := range f.stored {
		if activeOnly && !subject.IsActive {
			continue
		}
		rows = append(rows, subject)
	}
	return rows, nil
}

func (f *fakeInsurerRepository) Update(_ context.Context, subject insurer.Insurer) error {
	if _, ok := f.stored[subject.ID]; !ok {
		return insurer.ErrNotFound
	}
	if f.nameTaken(subject.Name, subject.ID) {
		return insurer.ErrNameTaken
	}
	f.stored[subject.ID] = subject
	return nil
}

// createMutual posts a mutual as the owner and returns its identifier.
func createMutual(t *testing.T, h *harness, owner, name string) string {
	t.Helper()
	status, body := h.do(t, http.MethodPost, "/api/v1/insurers", owner, map[string]any{"name": name})
	if status != http.StatusCreated {
		t.Fatalf("create mutual %q = %d: %v", name, status, body)
	}
	id, _ := body["id"].(string)
	return id
}

func TestCatalogWritesAreForOwnerAndAdminOnly(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	admin := h.token(t, tenantA, iam.RoleAdmin)
	professional := h.token(t, tenantA, iam.RoleProfessional)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	for _, tc := range []struct {
		role  string
		token string
		want  int
	}{
		{"owner", owner, http.StatusCreated},
		{"admin", admin, http.StatusCreated},
		{"professional", professional, http.StatusForbidden},
		{"accountant", accountant, http.StatusForbidden},
	} {
		status, _ := h.do(t, http.MethodPost, "/api/v1/insurers", tc.token, map[string]any{"name": tc.role + " intenta"})
		if status != tc.want {
			t.Errorf("%s create = %d, want %d", tc.role, status, tc.want)
		}
	}

	// Reading the catalog follows patients:read: the professional registers patients and
	// needs to choose a mutual; the accountant does not register patients.
	if status, _ := h.do(t, http.MethodGet, "/api/v1/insurers", professional, nil); status != http.StatusOK {
		t.Errorf("professional list = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/insurers", accountant, nil); status != http.StatusForbidden {
		t.Errorf("accountant list = %d, want 403", status)
	}
}

func TestMutualNameAndTaxIDAreNormalized(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)

	status, body := h.do(t, http.MethodPost, "/api/v1/insurers", owner, map[string]any{
		"name":  "  OSDE  ",
		"taxId": "30-12345678-9",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d: %v", status, body)
	}
	if body["name"] != "OSDE" || body["taxId"] != "30123456789" || body["isActive"] != true {
		t.Fatalf("body = %v, want the trimmed name, the digits of the CUIT and active", body)
	}
}

func TestDuplicateMutualNameIsConflict(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	createMutual(t, h, owner, "OSDE")

	status, _ := h.do(t, http.MethodPost, "/api/v1/insurers", owner, map[string]any{"name": "  osde "})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
}

func TestMutualTaxIDMustBeElevenDigits(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)

	status, _ := h.do(t, http.MethodPost, "/api/v1/insurers", owner, map[string]any{"name": "OSDE", "taxId": "3012345"})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", status)
	}
}

func TestDeactivatedMutualCannotCoverANewPatient(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	id := createMutual(t, h, owner, "Medifé")

	if status, _ := h.do(t, http.MethodPost, "/api/v1/insurers/"+id+"/deactivate", owner, nil); status != http.StatusOK {
		t.Fatalf("deactivate = %d, want 200", status)
	}

	status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, newPatientCreateBody(map[string]any{
		"fullName":     "Paciente de Medifé",
		"coverageKind": "insurer",
		"insurerId":    id,
		"memberNumber": "99887",
	}))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %v", status, body)
	}
	if detail, _ := body["detail"].(string); !strings.Contains(detail, "insurer_id") {
		t.Fatalf("detail = %q, want it to name insurer_id", detail)
	}
}

func TestInsuredPatientRoundTripsItsCoverageAndCurrency(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	id := createMutual(t, h, owner, "OSDE")

	status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, newPatientCreateBody(map[string]any{
		"fullName":     "Ana con OSDE",
		"coverageKind": "insurer",
		"insurerId":    id,
		"memberNumber": "12345-01",
		"client":       map[string]any{"billingCurrency": "USD"},
	}))
	if status != http.StatusCreated {
		t.Fatalf("create = %d: %v", status, body)
	}
	if body["coverageKind"] != "insurer" || body["insurerId"] != id || body["memberNumber"] != "12345-01" {
		t.Fatalf("body = %v, want the mutual and the member number", body)
	}
	clientID, _ := body["clientId"].(string)
	if clientID == "" {
		t.Fatalf("body = %v, want clientId", body)
	}
	clientStatus, clientBody := h.do(t, http.MethodGet, "/api/v1/clients/"+clientID, owner, nil)
	if clientStatus != http.StatusOK || clientBody["billingCurrency"] != "USD" {
		t.Fatalf("client = %d %v, want billing currency USD", clientStatus, clientBody)
	}
}

func TestPatientCoverageAndCurrencyAreRequired(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)

	for name, payload := range map[string]map[string]any{
		"no client":            {"fullName": "Sin cliente", "coverageKind": "private"},
		"no coverage":          {"fullName": "Sin cobertura", "client": map[string]any{"billingCurrency": "ARS"}},
		"currency outside MVP": {"fullName": "Real", "coverageKind": "private", "client": map[string]any{"billingCurrency": "BRL"}},
		"unknown coverage":     {"fullName": "Prepaga", "coverageKind": "prepaga", "client": map[string]any{"billingCurrency": "ARS"}},
		"mutual without id":    {"fullName": "Sin mutual", "coverageKind": "insurer", "memberNumber": "1", "client": map[string]any{"billingCurrency": "ARS"}},
		"private naming one":   {"fullName": "Particular con mutual", "coverageKind": "private", "insurerId": uuid.NewString(), "client": map[string]any{"billingCurrency": "ARS"}},
	} {
		t.Run(name, func(t *testing.T) {
			status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, payload)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %v", status, body)
			}
		})
	}
}

func TestCatalogRejectsAnUnknownIdentity(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)

	status, _ := h.do(t, http.MethodPatch, "/api/v1/insurers/"+uuid.NewString(), owner, map[string]any{"name": "Fantasma"})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}
