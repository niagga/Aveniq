package http_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
)

// fakeTariffRepository keeps the schedule in memory. Like the real adapter it refuses a
// second price for the same combination and start day, and it resolves the mutual's name
// from the catalog the harness shares with the mutual routes.
type fakeTariffRepository struct {
	stored   map[pricing.ID]pricing.Tariff
	insurers *fakeInsurerRepository
}

var _ port.TariffRepository = (*fakeTariffRepository)(nil)

func newFakeTariffRepository(insurers *fakeInsurerRepository) *fakeTariffRepository {
	return &fakeTariffRepository{stored: map[pricing.ID]pricing.Tariff{}, insurers: insurers}
}

func tariffKey(t pricing.Tariff) string {
	insurerID := ""
	if t.InsurerID != nil {
		insurerID = *t.InsurerID
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s", t.ProfessionalUserID, t.Coverage, insurerID, t.Modality, t.Currency, t.ValidFrom.Format("2006-01-02"))
}

func (f *fakeTariffRepository) Create(_ context.Context, subject pricing.Tariff) error {
	for _, other := range f.stored {
		if tariffKey(other) == tariffKey(subject) {
			return pricing.ErrValidityTaken
		}
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeTariffRepository) ByID(_ context.Context, id pricing.ID) (pricing.Tariff, error) {
	subject, ok := f.stored[id]
	if !ok {
		return pricing.Tariff{}, pricing.ErrNotFound
	}
	return subject, nil
}

func (f *fakeTariffRepository) List(_ context.Context) ([]port.TariffListItem, error) {
	items := []port.TariffListItem{}
	for _, subject := range f.stored {
		item := port.TariffListItem{Tariff: subject, ProfessionalName: "Demo Profesional"}
		if subject.InsurerID != nil {
			if mutual, ok := f.insurers.stored[insurer.ID(*subject.InsurerID)]; ok {
				item.InsurerName = mutual.Name
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (f *fakeTariffRepository) Update(_ context.Context, subject pricing.Tariff) error {
	if _, ok := f.stored[subject.ID]; !ok {
		return pricing.ErrNotFound
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeTariffRepository) Delete(_ context.Context, id pricing.ID) error {
	if _, ok := f.stored[id]; !ok {
		return pricing.ErrNotFound
	}
	delete(f.stored, id)
	return nil
}

// InForce mirrors the adapter: the latest validity that starts on or before the day.
func (f *fakeTariffRepository) InForce(_ context.Context, query port.TariffQuery) (pricing.Tariff, error) {
	var found *pricing.Tariff
	for _, subject := range f.stored {
		insurerID := ""
		if subject.InsurerID != nil {
			insurerID = *subject.InsurerID
		}
		if subject.ProfessionalUserID != query.ProfessionalUserID ||
			string(subject.Coverage) != query.Coverage ||
			insurerID != query.InsurerID ||
			string(subject.Modality) != query.Modality ||
			string(subject.Currency) != query.Currency ||
			subject.ValidFrom.After(query.Day) {
			continue
		}
		if found == nil || subject.ValidFrom.After(found.ValidFrom) {
			picked := subject
			found = &picked
		}
	}
	if found == nil {
		return pricing.Tariff{}, pricing.ErrNotFound
	}
	return *found, nil
}

// privateTariffBody is a particular price starting on day, for the professional.
func privateTariffBody(professional, day string) map[string]any {
	return map[string]any{
		"professionalUserId": professional,
		"coverage":           "private",
		"modality":           "presencial",
		"billingCurrency":    "ARS",
		"validFrom":          day,
		"insurerMinor":       0,
		"patientMinor":       1_500_000,
	}
}

// mutualTariffBody is a mutual price starting on day, for the professional.
func mutualTariffBody(professional, insurerID, day string) map[string]any {
	return map[string]any{
		"professionalUserId": professional,
		"coverage":           "insurer",
		"insurerId":          insurerID,
		"modality":           "online",
		"billingCurrency":    "ARS",
		"validFrom":          day,
		"insurerMinor":       1_000_000,
		"patientMinor":       200_000,
	}
}

func TestTariffWritesAreForOwnerAndAdminOnly(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	admin := h.token(t, tenantA, iam.RoleAdmin)
	professional := h.token(t, tenantA, iam.RoleProfessional)
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	pro := uuid.NewString()

	for _, tc := range []struct {
		role  string
		token string
		day   string
		want  int
	}{
		{"owner", owner, "2026-11-01", http.StatusCreated},
		{"admin", admin, "2026-11-02", http.StatusCreated},
		{"professional", professional, "2026-11-03", http.StatusForbidden},
		{"accountant", accountant, "2026-11-04", http.StatusForbidden},
	} {
		status, body := h.do(t, http.MethodPost, "/api/v1/tariffs", tc.token, privateTariffBody(pro, tc.day))
		if status != tc.want {
			t.Errorf("%s create = %d, want %d: %v", tc.role, status, tc.want, body)
		}
	}

	// Reading follows billing:read: the accountant reads the schedule the vouchers start
	// from, and the professional, who holds no billing permission, does not.
	if status, _ := h.do(t, http.MethodGet, "/api/v1/tariffs", accountant, nil); status != http.StatusOK {
		t.Errorf("accountant list = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/tariffs", professional, nil); status != http.StatusForbidden {
		t.Errorf("professional list = %d, want 403", status)
	}
}

func TestTariffCreateRefusesABadBody(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	pro := uuid.NewString()
	mutual := createMutual(t, h, owner, "OSDE Forma Body")

	cases := map[string]func(map[string]any){
		"no modality":         func(b map[string]any) { delete(b, "modality") },
		"unknown modality":    func(b map[string]any) { b["modality"] = "hibrido" },
		"unknown currency":    func(b map[string]any) { b["billingCurrency"] = "BRL" },
		"malformed day":       func(b map[string]any) { b["validFrom"] = "01/11/2026" },
		"negative amount":     func(b map[string]any) { b["patientMinor"] = -1 },
		"no insurer amount":   func(b map[string]any) { delete(b, "insurerMinor") },
		"mutual without one":  func(b map[string]any) { b["coverage"] = "insurer"; delete(b, "insurerId") },
		"particular with one": func(b map[string]any) { b["insurerId"] = mutual },
	}
	for name, mutate := range cases {
		body := privateTariffBody(pro, "2026-11-10")
		mutate(body)
		if status, _ := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, body); status != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422", name, status)
		}
	}
}

func TestTariffMutualPriceNeedsAnActiveMutual(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	pro := uuid.NewString()
	mutual := createMutual(t, h, owner, "OSDE Activa")

	status, body := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, mutualTariffBody(pro, mutual, "2026-11-01"))
	if status != http.StatusCreated {
		t.Fatalf("mutual price = %d, want 201: %v", status, body)
	}
	if body["insurerName"] != "OSDE Activa" || body["totalMinor"] != float64(1_200_000) {
		t.Fatalf("body = %v, want the mutual's name and the session total", body)
	}

	if status, _ := h.do(t, http.MethodPost, "/api/v1/insurers/"+mutual+"/deactivate", owner, nil); status != http.StatusOK {
		t.Fatalf("deactivate = %d", status)
	}
	if status, _ := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, mutualTariffBody(pro, mutual, "2026-12-01")); status != http.StatusUnprocessableEntity {
		t.Fatalf("price for a deactivated mutual = %d, want 422", status)
	}
	if status, _ := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, mutualTariffBody(pro, uuid.NewString(), "2026-12-01")); status != http.StatusUnprocessableEntity {
		t.Fatalf("price for a mutual outside the catalog = %d, want 422", status)
	}
}

func TestTariffSameDayTwiceIsAConflict(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	pro := uuid.NewString()

	if status, _ := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, privateTariffBody(pro, "2026-11-01")); status != http.StatusCreated {
		t.Fatalf("first = %d", status)
	}
	if status, _ := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, privateTariffBody(pro, "2026-11-01")); status != http.StatusConflict {
		t.Fatalf("same day twice = %d, want 409", status)
	}
}

func TestTariffAmountsChangeAndRemoval(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)
	pro := uuid.NewString()

	status, created := h.do(t, http.MethodPost, "/api/v1/tariffs", owner, privateTariffBody(pro, "2026-11-01"))
	if status != http.StatusCreated {
		t.Fatalf("create = %d", status)
	}
	id, _ := created["id"].(string)

	status, body := h.do(t, http.MethodPatch, "/api/v1/tariffs/"+id, owner, map[string]any{"insurerMinor": 0, "patientMinor": 2_000_000})
	if status != http.StatusOK || body["totalMinor"] != float64(2_000_000) {
		t.Fatalf("edit = %d %v, want 200 with the new total", status, body)
	}
	if status, _ := h.do(t, http.MethodPatch, "/api/v1/tariffs/"+id, owner, map[string]any{"insurerMinor": 5, "patientMinor": 5}); status != http.StatusUnprocessableEntity {
		t.Fatalf("mutual share on a particular price = %d, want 422", status)
	}

	if status, _ := h.do(t, http.MethodDelete, "/api/v1/tariffs/"+id, owner, nil); status != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", status)
	}
	if status, _ := h.do(t, http.MethodDelete, "/api/v1/tariffs/"+id, owner, nil); status != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", status)
	}
	status, list := h.do(t, http.MethodGet, "/api/v1/tariffs", owner, nil)
	if status != http.StatusOK {
		t.Fatalf("list = %d", status)
	}
	if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("items after removal = %d, want 0", len(items))
	}
}
