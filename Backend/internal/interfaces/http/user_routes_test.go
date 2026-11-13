package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// ─── doubles for the account routes ──────────────────────────────────────────

type fakeUserRepository struct {
	rows map[string]map[iam.UserID]iam.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{rows: map[string]map[iam.UserID]iam.User{}}
}

func (r *fakeUserRepository) Create(ctx context.Context, subject iam.User) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.ErrTenantScopeMissing
	}
	if r.rows[tenantID] == nil {
		r.rows[tenantID] = map[iam.UserID]iam.User{}
	}
	r.rows[tenantID][subject.ID] = subject
	return nil
}

func (r *fakeUserRepository) ByID(ctx context.Context, id iam.UserID) (iam.User, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.User{}, iam.ErrTenantScopeMissing
	}
	subject, found := r.rows[tenantID][id]
	if !found {
		return iam.User{}, iam.ErrNotFound
	}
	return subject, nil
}

func (r *fakeUserRepository) ByEmail(ctx context.Context, email string) (iam.User, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.User{}, iam.ErrTenantScopeMissing
	}
	for _, subject := range r.rows[tenantID] {
		if subject.Email == email {
			return subject, nil
		}
	}
	return iam.User{}, iam.ErrNotFound
}

func (r *fakeUserRepository) List(ctx context.Context, limit, offset int32) ([]iam.User, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, iam.ErrTenantScopeMissing
	}
	all := make([]iam.User, 0, len(r.rows[tenantID]))
	for _, subject := range r.rows[tenantID] {
		all = append(all, subject)
	}
	if int(offset) >= len(all) {
		return nil, nil
	}
	end := min(int(offset)+int(limit), len(all))
	return all[offset:end], nil
}

func (r *fakeUserRepository) Update(ctx context.Context, subject iam.User) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.ErrTenantScopeMissing
	}
	if _, found := r.rows[tenantID][subject.ID]; !found {
		return iam.ErrNotFound
	}
	r.rows[tenantID][subject.ID] = subject
	return nil
}

func (r *fakeUserRepository) Delete(ctx context.Context, id iam.UserID) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.ErrTenantScopeMissing
	}
	if _, found := r.rows[tenantID][id]; !found {
		return iam.ErrNotFound
	}
	delete(r.rows[tenantID], id)
	return nil
}

var _ port.UserRepository = (*fakeUserRepository)(nil)

// fakeAudit keeps every event so a test can check the trail, not just the status.
type fakeAudit struct {
	events []port.AuditEvent
}

func (a *fakeAudit) Record(_ context.Context, event port.AuditEvent) error {
	a.events = append(a.events, event)
	return nil
}

// member seeds an account in the practice and mints a token whose subject is that
// account, so the use cases find the actor they are checking.
func (h *harness) member(t *testing.T, tenantID string, role iam.RoleCode) (token string, id string) {
	t.Helper()

	id = uuid.NewString()
	user, err := iam.NewUser(iam.UserID(id), tenant.ID(tenantID), id[:8]+"@practica.test", "argon2id-hash",
		"Persona "+id[:8], role, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("cannot build the account: %v", err)
	}
	if h.users.rows[tenantID] == nil {
		h.users.rows[tenantID] = map[iam.UserID]iam.User{}
	}
	h.users.rows[tenantID][user.ID] = user

	token = h.issue(t, id, tenantID, role)
	return token, id
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestAccountReadsFollowTheMatrix(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	_, targetID := h.member(t, tenantA, iam.RoleProfessional)

	assistant := h.token(t, tenantA, iam.RoleAssistant)
	professional := h.token(t, tenantA, iam.RoleProfessional)
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	admin := h.token(t, tenantA, iam.RoleAdmin)

	t.Run("the account list needs users:read, so an assistant is refused", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/users", assistant, nil); status != http.StatusForbidden {
			t.Fatalf("assistant on /users = %d, want 403: emails and status are not front-desk data", status)
		}
	})

	t.Run("the account list is open to admin", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/users", admin, nil); status != http.StatusOK {
			t.Fatalf("admin on /users = %d, want 200", status)
		}
	})

	t.Run("the directory is open to a professional and an assistant", func(t *testing.T) {
		for name, token := range map[string]string{"professional": professional, "assistant": assistant} {
			if status, _ := h.do(t, http.MethodGet, "/api/v1/users/directory", token, nil); status != http.StatusOK {
				t.Fatalf("%s on /users/directory = %d, want 200: it names the authors of entries", name, status)
			}
		}
	})

	t.Run("the directory never carries an email or a status", func(t *testing.T) {
		status, body := h.do(t, http.MethodGet, "/api/v1/users/directory", professional, nil)
		if status != http.StatusOK {
			t.Fatalf("directory = %d, want 200", status)
		}
		items, _ := body["items"].([]any)
		if len(items) == 0 {
			t.Fatal("directory is empty: the seeded account should appear")
		}
		for _, raw := range items {
			entry, _ := raw.(map[string]any)
			for _, forbidden := range []string{"email", "isActive", "passwordHash"} {
				if _, leaked := entry[forbidden]; leaked {
					t.Fatalf("directory entry carries %q: %v", forbidden, entry)
				}
			}
		}
	})

	t.Run("an accountant reads neither the directory nor the account", func(t *testing.T) {
		for _, path := range []string{"/api/v1/users/directory", "/api/v1/users/" + targetID} {
			if status, _ := h.do(t, http.MethodGet, path, accountant, nil); status != http.StatusForbidden {
				t.Fatalf("accountant on %s = %d, want 403", path, status)
			}
		}
	})

	t.Run("an account of another practice is 404", func(t *testing.T) {
		tenantB := h.tenantID(t, "practica-b")
		ownerB := h.token(t, tenantB, iam.RoleOwner)
		if status, _ := h.do(t, http.MethodGet, "/api/v1/users/"+targetID, ownerB, nil); status != http.StatusNotFound {
			t.Fatalf("foreign account = %d, want 404: a foreign account must look absent", status)
		}
	})
}

func TestRoleChangeRules(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")

	ownerToken, ownerID := h.member(t, tenantA, iam.RoleOwner)
	adminToken, _ := h.member(t, tenantA, iam.RoleAdmin)
	_, professionalID := h.member(t, tenantA, iam.RoleProfessional)
	_, otherOwnerID := h.member(t, tenantA, iam.RoleOwner)
	_, foreignID := h.member(t, tenantB, iam.RoleProfessional)

	put := func(token, id, role string) (int, map[string]any) {
		return h.do(t, http.MethodPut, "/api/v1/users/"+id+"/role", token, map[string]any{"role": role})
	}

	t.Run("an admin moves a professional to assistant", func(t *testing.T) {
		status, body := put(adminToken, professionalID, string(iam.RoleAssistant))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200: %v", status, body)
		}
		if body["role"] != string(iam.RoleAssistant) {
			t.Fatalf("role = %v, want assistant", body["role"])
		}
	})

	t.Run("an admin cannot grant the owner role", func(t *testing.T) {
		status, body := put(adminToken, professionalID, string(iam.RoleOwner))
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %v", status, body)
		}
		if _, leaked := body["role"]; leaked {
			t.Fatalf("a refused change echoed the account: %v", body)
		}
	})

	t.Run("an admin cannot demote an owner", func(t *testing.T) {
		if status, _ := put(adminToken, otherOwnerID, string(iam.RoleAdmin)); status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", status)
		}
	})

	t.Run("an owner cannot change their own role", func(t *testing.T) {
		if status, _ := put(ownerToken, ownerID, string(iam.RoleAdmin)); status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: the last owner must not demote themselves", status)
		}
	})

	t.Run("an owner grants and changes the owner role", func(t *testing.T) {
		if status, body := put(ownerToken, professionalID, string(iam.RoleOwner)); status != http.StatusOK {
			t.Fatalf("status = %d, want 200: %v", status, body)
		}
	})

	t.Run("the same role again is 409", func(t *testing.T) {
		if status, _ := put(ownerToken, otherOwnerID, string(iam.RoleOwner)); status != http.StatusConflict {
			t.Fatalf("status = %d, want 409", status)
		}
	})

	t.Run("a role outside the catalog is refused by the schema", func(t *testing.T) {
		if status, _ := put(ownerToken, otherOwnerID, "superadmin"); status != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", status)
		}
	})

	t.Run("an account of another practice is 404", func(t *testing.T) {
		if status, _ := put(ownerToken, foreignID, string(iam.RoleAdmin)); status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", status)
		}
	})

	t.Run("every change that happened is in the trail, with who and from what", func(t *testing.T) {
		transitions := map[string]bool{}
		for _, event := range h.audit.events {
			if event.Action != "user.role_changed" || event.EntityID != professionalID {
				continue
			}
			if event.ActorID == "" || event.TenantID != tenantA {
				t.Fatalf("audit event has no actor or the wrong practice: %+v", event)
			}
			transitions[event.Metadata["from"].(string)+">"+event.Metadata["to"].(string)] = true
		}
		for _, want := range []string{"professional>assistant", "assistant>owner"} {
			if !transitions[want] {
				t.Fatalf("trail is missing the transition %s; got %v", want, transitions)
			}
		}
	})
}

func TestDeactivationKeepsTheAccount(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	ownerToken, _ := h.member(t, tenantA, iam.RoleOwner)
	_, staffID := h.member(t, tenantA, iam.RoleAssistant)

	if status, _ := h.do(t, http.MethodPost, "/api/v1/users/"+staffID+"/deactivate", ownerToken, nil); status != http.StatusOK {
		t.Fatalf("deactivate = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodPost, "/api/v1/users/"+staffID+"/deactivate", ownerToken, nil); status != http.StatusConflict {
		t.Fatalf("second deactivate = %d, want 409", status)
	}

	status, body := h.do(t, http.MethodGet, "/api/v1/users/"+staffID, ownerToken, nil)
	if status != http.StatusOK {
		t.Fatalf("the deactivated account must stay readable: %d", status)
	}
	if body["isActive"] != false {
		t.Fatalf("isActive = %v, want false", body["isActive"])
	}

	if status, _ := h.do(t, http.MethodPost, "/api/v1/users/"+staffID+"/activate", ownerToken, nil); status != http.StatusOK {
		t.Fatalf("activate = %d, want 200", status)
	}

	actions := map[string]bool{}
	for _, event := range h.audit.events {
		actions[event.Action] = true
	}
	for _, want := range []string{"user.deactivated", "user.activated"} {
		if !actions[want] {
			t.Fatalf("trail is missing %q", want)
		}
	}
}

// issue mints an access token for an explicit subject. The guard reads only the
// signed claims, so the subject must be an account the use cases can load.
func (h *harness) issue(t *testing.T, subject, tenantID string, role iam.RoleCode) string {
	t.Helper()

	token, err := h.tokens.Issue(authn.AccessClaims{
		Subject:  subject,
		TenantID: tenantID,
		Role:     string(role),
	})
	if err != nil {
		t.Fatalf("cannot issue a token: %v", err)
	}
	return token
}
