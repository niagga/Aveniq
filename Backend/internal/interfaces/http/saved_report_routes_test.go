package http_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// fakeSavedReportRepository keeps the saved reports per practice, with the same name rule as
// the table: unique inside a practice, ignoring case and outer spaces.
type fakeSavedReportRepository struct {
	stored map[string]map[report.SavedID]report.SavedReport
}

var _ port.SavedReportRepository = (*fakeSavedReportRepository)(nil)

func newFakeSavedReportRepository() *fakeSavedReportRepository {
	return &fakeSavedReportRepository{stored: map[string]map[report.SavedID]report.SavedReport{}}
}

func savedNameKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func (r *fakeSavedReportRepository) nameTaken(tenantID string, id report.SavedID, name string) bool {
	for other, saved := range r.stored[tenantID] {
		if other != id && savedNameKey(saved.Name) == savedNameKey(name) {
			return true
		}
	}
	return false
}

func (r *fakeSavedReportRepository) Create(ctx context.Context, saved report.SavedReport) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return report.ErrTenantScopeMissing
	}
	if r.nameTaken(tenantID, saved.ID, saved.Name) {
		return report.ErrSavedNameTaken
	}
	if r.stored[tenantID] == nil {
		r.stored[tenantID] = map[report.SavedID]report.SavedReport{}
	}
	r.stored[tenantID][saved.ID] = saved
	return nil
}

func (r *fakeSavedReportRepository) ByID(ctx context.Context, id report.SavedID) (report.SavedReport, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return report.SavedReport{}, report.ErrTenantScopeMissing
	}
	saved, found := r.stored[tenantID][id]
	if !found {
		return report.SavedReport{}, report.ErrSavedNotFound
	}
	return saved, nil
}

func (r *fakeSavedReportRepository) List(ctx context.Context) ([]port.SavedReportItem, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, report.ErrTenantScopeMissing
	}
	out := []port.SavedReportItem{}
	for _, saved := range r.stored[tenantID] {
		out = append(out, port.SavedReportItem{Report: saved})
	}
	sort.Slice(out, func(i, j int) bool { return savedNameKey(out[i].Report.Name) < savedNameKey(out[j].Report.Name) })
	return out, nil
}

func (r *fakeSavedReportRepository) Update(ctx context.Context, saved report.SavedReport) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return report.ErrTenantScopeMissing
	}
	if _, found := r.stored[tenantID][saved.ID]; !found {
		return report.ErrSavedNotFound
	}
	if r.nameTaken(tenantID, saved.ID, saved.Name) {
		return report.ErrSavedNameTaken
	}
	r.stored[tenantID][saved.ID] = saved
	return nil
}

func (r *fakeSavedReportRepository) Delete(ctx context.Context, id report.SavedID) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return report.ErrTenantScopeMissing
	}
	if _, found := r.stored[tenantID][id]; !found {
		return report.ErrSavedNotFound
	}
	delete(r.stored[tenantID], id)
	return nil
}

// tokenAs mints a token for a known person, so a test can make someone the author of a report.
func tokenAs(t *testing.T, h *harness, tenantID string, role iam.RoleCode, userID string) string {
	t.Helper()
	token, err := h.tokens.Issue(authn.AccessClaims{Subject: userID, TenantID: tenantID, Role: string(role)})
	if err != nil {
		t.Fatalf("cannot issue a token: %v", err)
	}
	return token
}

const savedPath = "/api/v1/reports/saved"

func monthlyReport(name string) map[string]any {
	return map[string]any{"name": name, "period": "this-month", "groupBy": "professional"}
}

func saveReport(t *testing.T, h *harness, token string, body map[string]any) string {
	t.Helper()
	status, created := h.do(t, http.MethodPost, savedPath, token, body)
	if status != http.StatusCreated {
		t.Fatalf("save = %d, want 201: %v", status, created)
	}
	id, _ := created["id"].(string)
	return id
}

func canManage(t *testing.T, h *harness, token, id string) bool {
	t.Helper()
	status, body := h.do(t, http.MethodGet, savedPath, token, nil)
	if status != http.StatusOK {
		t.Fatalf("list = %d, want 200: %v", status, body)
	}
	for _, raw := range listOf(body, "items") {
		item, _ := raw.(map[string]any)
		if item["id"] == id {
			value, _ := item["canManage"].(bool)
			return value
		}
	}
	t.Fatalf("report %s is not in the list", id)
	return false
}

// TestAReportSavedByOneIsSharedWithEveryoneWhoReadsReports: it is the practice's, not the
// author's, and the viewer's own right to change it is part of what the list answers.
func TestAReportSavedByOneIsSharedWithEveryoneWhoReadsReports(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	authorID := uuid.NewString()
	accountant := tokenAs(t, h, tenantA, iam.RoleAccountant, authorID)

	id := saveReport(t, h, accountant, map[string]any{
		"name": "Mutuales del trimestre", "period": "last-3-months", "groupBy": "insurer", "coverage": "insurer",
	})

	for _, role := range []iam.RoleCode{iam.RoleOwner, iam.RoleAdmin, iam.RoleProfessional, iam.RoleAccountant} {
		status, body := h.do(t, http.MethodGet, savedPath, h.token(t, tenantA, role), nil)
		if status != http.StatusOK || len(listOf(body, "items")) != 1 {
			t.Fatalf("%s lists = %d %v, want the shared report", role, status, body)
		}
	}

	if !canManage(t, h, accountant, id) {
		t.Error("the author must be able to manage the report")
	}
	if !canManage(t, h, h.token(t, tenantA, iam.RoleOwner), id) || !canManage(t, h, h.token(t, tenantA, iam.RoleAdmin), id) {
		t.Error("the owner and the admin must be able to manage every report")
	}
	if canManage(t, h, h.token(t, tenantA, iam.RoleProfessional), id) {
		t.Error("another reader must not be offered to manage someone else's report")
	}
}

// TestOnlyTheAuthorTheOwnerAndTheAdminChangeOrRemoveAReport.
func TestOnlyTheAuthorTheOwnerAndTheAdminChangeOrRemoveAReport(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	authorID := uuid.NewString()
	author := tokenAs(t, h, tenantA, iam.RoleProfessional, authorID)
	other := h.token(t, tenantA, iam.RoleAccountant)
	id := saveReport(t, h, author, monthlyReport("Mis sesiones"))
	path := savedPath + "/" + id

	if status, _ := h.do(t, http.MethodPut, path, other, monthlyReport("Cambiado")); status != http.StatusForbidden {
		t.Errorf("another reader changes = %d, want 403", status)
	}
	if status, _ := h.do(t, http.MethodDelete, path, other, nil); status != http.StatusForbidden {
		t.Errorf("another reader removes = %d, want 403", status)
	}

	status, changed := h.do(t, http.MethodPut, path, author, map[string]any{"name": "Mis sesiones 2", "period": "this-year", "groupBy": "month"})
	if status != http.StatusOK || changed["name"] != "Mis sesiones 2" || changed["period"] != "this-year" || changed["groupBy"] != "month" {
		t.Fatalf("author changes = %d %v", status, changed)
	}
	if changed["createdBy"] != authorID {
		t.Errorf("a change must keep the author, got %v", changed["createdBy"])
	}

	owner := h.token(t, tenantA, iam.RoleOwner)
	if status, _ := h.do(t, http.MethodPut, path, owner, monthlyReport("Renombrado por el dueño")); status != http.StatusOK {
		t.Errorf("owner changes = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodDelete, path, owner, nil); status != http.StatusNoContent {
		t.Errorf("owner removes = %d, want 204", status)
	}
	if status, _ := h.do(t, http.MethodDelete, path, owner, nil); status != http.StatusNotFound {
		t.Errorf("removing it twice = %d, want 404", status)
	}
	if status, _ := h.do(t, http.MethodPut, path, owner, monthlyReport("Ya no existe")); status != http.StatusNotFound {
		t.Errorf("changing a removed report = %d, want 404", status)
	}
}

// TestTheAuthorRemovesTheirOwnReport.
func TestTheAuthorRemovesTheirOwnReport(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	author := tokenAs(t, h, tenantA, iam.RoleAccountant, uuid.NewString())
	id := saveReport(t, h, author, monthlyReport("Para borrar"))
	if status, _ := h.do(t, http.MethodDelete, savedPath+"/"+id, author, nil); status != http.StatusNoContent {
		t.Fatalf("author removes = %d, want 204", status)
	}
}

// TestASavedReportNameIsUniqueInThePracticeIgnoringCase.
func TestASavedReportNameIsUniqueInThePracticeIgnoringCase(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")
	owner := h.token(t, tenantA, iam.RoleOwner)

	saveReport(t, h, owner, monthlyReport("Cierre mensual"))
	if status, _ := h.do(t, http.MethodPost, savedPath, owner, monthlyReport("  cierre MENSUAL ")); status != http.StatusConflict {
		t.Errorf("same name again = %d, want 409", status)
	}
	if status, _ := h.do(t, http.MethodPost, savedPath, h.token(t, tenantB, iam.RoleOwner), monthlyReport("Cierre mensual")); status != http.StatusCreated {
		t.Errorf("same name in another practice = %d, want 201", status)
	}

	second := saveReport(t, h, owner, monthlyReport("Otro"))
	if status, _ := h.do(t, http.MethodPut, savedPath+"/"+second, owner, monthlyReport("CIERRE mensual")); status != http.StatusConflict {
		t.Errorf("renaming onto an existing name = %d, want 409", status)
	}
	if status, _ := h.do(t, http.MethodPut, savedPath+"/"+second, owner, monthlyReport("Otro")); status != http.StatusOK {
		t.Errorf("saving a report under its own name = %d, want 200", status)
	}
}

// TestASavedReportMustBeAQuestionTheReportCanAsk: a definition the billing report would refuse
// is refused when it is saved.
func TestASavedReportMustBeAQuestionTheReportCanAsk(t *testing.T) {
	h := newHarness(t)
	owner := h.token(t, h.tenantID(t, "practica-a"), iam.RoleOwner)

	cases := map[string]map[string]any{
		"custom without days":            {"name": "a", "period": "custom"},
		"custom with the days reversed":  {"name": "b", "period": "custom", "from": "2026-11-30", "to": "2026-11-01"},
		"relative period with days":      {"name": "c", "period": "this-month", "from": "2026-11-01", "to": "2026-11-30"},
		"a mutual of private patients":   {"name": "d", "period": "this-month", "coverage": "private", "insurerId": uuid.NewString()},
		"blank name":                     {"name": "   ", "period": "this-month"},
		"a day that does not exist":      {"name": "e", "period": "custom", "from": "2026-02-30", "to": "2026-03-01"},
		"a custom range of over 5 years": {"name": "f", "period": "custom", "from": "2020-01-01", "to": "2026-11-30"},
	}
	for label, body := range cases {
		if status, resp := h.do(t, http.MethodPost, savedPath, owner, body); status != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422: %v", label, status, resp)
		}
	}
	if status, _ := h.do(t, http.MethodPost, savedPath, owner, map[string]any{"name": "g", "period": "weekly"}); status != http.StatusUnprocessableEntity {
		t.Errorf("unknown period = %d, want 422", status)
	}

	status, created := h.do(t, http.MethodPost, savedPath, owner, map[string]any{"name": "ok", "period": "custom", "from": "2026-11-01", "to": "2026-11-30"})
	if status != http.StatusCreated || created["groupBy"] != "none" || created["from"] != "2026-11-01" || created["to"] != "2026-11-30" {
		t.Errorf("a custom period = %d %v, want 201 grouped by none", status, created)
	}
}

// TestSavedReportsBelongToOnePractice.
func TestSavedReportsBelongToOnePractice(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")
	id := saveReport(t, h, h.token(t, tenantA, iam.RoleOwner), monthlyReport("Privado de A"))
	ownerB := h.token(t, tenantB, iam.RoleOwner)

	if status, body := h.do(t, http.MethodGet, savedPath, ownerB, nil); status != http.StatusOK || len(listOf(body, "items")) != 0 {
		t.Errorf("other practice lists = %d %v, want nothing", status, body)
	}
	if status, _ := h.do(t, http.MethodPut, savedPath+"/"+id, ownerB, monthlyReport("Robado")); status != http.StatusNotFound {
		t.Errorf("other practice changes = %d, want 404", status)
	}
	if status, _ := h.do(t, http.MethodDelete, savedPath+"/"+id, ownerB, nil); status != http.StatusNotFound {
		t.Errorf("other practice removes = %d, want 404", status)
	}
}

// TestRolesThatReadNoReportCannotUseSavedReports.
func TestRolesThatReadNoReportCannotUseSavedReports(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	id := saveReport(t, h, h.token(t, tenantA, iam.RoleOwner), monthlyReport("Solo lectores"))
	assistant := h.token(t, tenantA, iam.RoleAssistant)

	if status, _ := h.do(t, http.MethodGet, savedPath, "", nil); status != http.StatusUnauthorized {
		t.Errorf("anonymous lists = %d, want 401", status)
	}
	if status, _ := h.do(t, http.MethodGet, savedPath, assistant, nil); status != http.StatusForbidden {
		t.Errorf("assistant lists = %d, want 403", status)
	}
	if status, _ := h.do(t, http.MethodPost, savedPath, assistant, monthlyReport("x")); status != http.StatusForbidden {
		t.Errorf("assistant saves = %d, want 403", status)
	}
	if status, _ := h.do(t, http.MethodDelete, savedPath+"/"+id, assistant, nil); status != http.StatusForbidden {
		t.Errorf("assistant removes = %d, want 403", status)
	}
}
