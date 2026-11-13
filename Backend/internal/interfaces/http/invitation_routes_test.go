package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── doubles for the invitation routes ───────────────────────────────────────

// fakeInvitationRepository follows the contract of the SQL adapter: rows are per
// tenant, and the accept and revoke guards refuse what the statement would not
// match. The real guard is a conditional UPDATE; here it is the same condition.
type fakeInvitationRepository struct {
	rows map[string]map[iam.InvitationID]iam.Invitation
}

func newFakeInvitationRepository() *fakeInvitationRepository {
	return &fakeInvitationRepository{rows: map[string]map[iam.InvitationID]iam.Invitation{}}
}

func (r *fakeInvitationRepository) Create(ctx context.Context, subject iam.Invitation) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.ErrTenantScopeMissing
	}
	if r.rows[tenantID] == nil {
		r.rows[tenantID] = map[iam.InvitationID]iam.Invitation{}
	}
	r.rows[tenantID][subject.ID] = subject
	return nil
}

func (r *fakeInvitationRepository) ByID(ctx context.Context, id iam.InvitationID) (iam.Invitation, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.Invitation{}, iam.ErrTenantScopeMissing
	}
	subject, found := r.rows[tenantID][id]
	if !found {
		return iam.Invitation{}, iam.ErrNotFound
	}
	return subject, nil
}

func (r *fakeInvitationRepository) ByTokenHash(ctx context.Context, hash string) (iam.Invitation, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.Invitation{}, iam.ErrTenantScopeMissing
	}
	for _, subject := range r.rows[tenantID] {
		if subject.TokenHash == hash {
			return subject, nil
		}
	}
	return iam.Invitation{}, iam.ErrNotFound
}

func (r *fakeInvitationRepository) Pending(ctx context.Context, now time.Time, limit, offset int32) ([]iam.Invitation, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, iam.ErrTenantScopeMissing
	}
	var pending []iam.Invitation
	for _, subject := range r.rows[tenantID] {
		if subject.AcceptedAt == nil && now.Before(subject.ExpiresAt) {
			pending = append(pending, subject)
		}
	}
	if int(offset) >= len(pending) {
		return nil, nil
	}
	return pending[offset:min(int(offset)+int(limit), len(pending))], nil
}

func (r *fakeInvitationRepository) MarkAccepted(ctx context.Context, id iam.InvitationID, at time.Time) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.ErrTenantScopeMissing
	}
	subject, found := r.rows[tenantID][id]
	if !found || subject.AcceptedAt != nil || !at.Before(subject.ExpiresAt) {
		return iam.ErrInvitationUnusable
	}
	accepted := at.UTC()
	subject.AcceptedAt = &accepted
	r.rows[tenantID][id] = subject
	return nil
}

func (r *fakeInvitationRepository) Delete(ctx context.Context, id iam.InvitationID) error {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return iam.ErrTenantScopeMissing
	}
	subject, found := r.rows[tenantID][id]
	if !found || subject.AcceptedAt != nil {
		return iam.ErrNotFound
	}
	delete(r.rows[tenantID], id)
	return nil
}

var _ port.InvitationRepository = (*fakeInvitationRepository)(nil)

// fakeHasher stands in for argon2id. The stored value only has to differ from the
// plaintext, which is what the tests check.
type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) {
	return "hashed:" + password, nil
}

// ─── tests ───────────────────────────────────────────────────────────────────

const (
	testPassword = "clave-de-prueba-12"
	testFullName = "Ana Pérez"
)

// testNow is the fixed instant the harness clock returns.
var testNow = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

// issueInvitation creates an invitation through the route and returns the token
// and the invitation identifier from the answer.
func issueInvitation(t *testing.T, h *harness, token, email, role string) (secret, id string) {
	t.Helper()
	status, body := h.do(t, http.MethodPost, "/api/v1/invitations", token, map[string]any{"email": email, "role": role})
	if status != http.StatusCreated {
		t.Fatalf("create invitation for %s = %d, want 201: %v", email, status, body)
	}
	invitation, _ := body["invitation"].(map[string]any)
	secret, _ = body["token"].(string)
	id, _ = invitation["id"].(string)
	if secret == "" || id == "" {
		t.Fatalf("create answer lacks the token or the id: %v", body)
	}
	return secret, id
}

func TestInvitationLifecycle(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	ownerToken, _ := h.member(t, tenantA, iam.RoleOwner)

	secret, invitationID := issueInvitation(t, h, ownerToken, "  Ana.Perez@Practica.test ", string(iam.RoleProfessional))

	t.Run("the answer never repeats the token or its hash", func(t *testing.T) {
		_, body := h.do(t, http.MethodGet, "/api/v1/invitations", ownerToken, nil)
		items, _ := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("pending list = %d items, want 1", len(items))
		}
		entry, _ := items[0].(map[string]any)
		for _, forbidden := range []string{"token", "tokenHash", "token_hash"} {
			if _, leaked := entry[forbidden]; leaked {
				t.Fatalf("pending entry carries %q: %v", forbidden, entry)
			}
		}
		if entry["email"] != "ana.perez@practica.test" {
			t.Fatalf("email = %v, want the canonical form", entry["email"])
		}
	})

	t.Run("the link previews the invitation without a session", func(t *testing.T) {
		status, body := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token="+secret, "", nil)
		if status != http.StatusOK {
			t.Fatalf("preview = %d, want 200: %v", status, body)
		}
		if body["email"] != "ana.perez@practica.test" || body["role"] != string(iam.RoleProfessional) {
			t.Fatalf("preview = %v", body)
		}
	})

	t.Run("accepting creates the account with the chosen password hashed", func(t *testing.T) {
		status, body := h.do(t, http.MethodPost, "/api/v1/invitations/accept", "", map[string]any{
			"token": secret, "fullName": testFullName, "password": testPassword,
		})
		if status != http.StatusCreated {
			t.Fatalf("accept = %d, want 201: %v", status, body)
		}
		userID, _ := body["userId"].(string)
		account, found := h.users.rows[tenantA][iam.UserID(userID)]
		if !found {
			t.Fatalf("no account %q was created", userID)
		}
		if account.Role != iam.RoleProfessional || account.Email != "ana.perez@practica.test" {
			t.Fatalf("account = %+v, want the invitation's email and role", account)
		}
		if string(account.PasswordHash) == testPassword || !strings.HasPrefix(string(account.PasswordHash), "hashed:") {
			t.Fatalf("password was stored as %q; it must be hashed", account.PasswordHash)
		}
	})

	t.Run("an accepted link answers 410 everywhere, and leaves the pending list", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token="+secret, "", nil); status != http.StatusGone {
			t.Fatalf("preview after accept = %d, want 410", status)
		}
		status, body := h.do(t, http.MethodPost, "/api/v1/invitations/accept", "", map[string]any{
			"token": secret, "fullName": testFullName, "password": testPassword,
		})
		if status != http.StatusGone {
			t.Fatalf("second accept = %d, want 410: %v", status, body)
		}
		_, list := h.do(t, http.MethodGet, "/api/v1/invitations", ownerToken, nil)
		if items, _ := list["items"].([]any); len(items) != 0 {
			t.Fatalf("accepted invitation still pending: %v", items)
		}
	})

	t.Run("the trail records the creation and the acceptance", func(t *testing.T) {
		actions := map[string]bool{}
		for _, event := range h.audit.events {
			if event.Entity == "invitation" && event.EntityID == invitationID {
				actions[event.Action] = true
			}
		}
		for _, want := range []string{"invitation.created", "invitation.accepted"} {
			if !actions[want] {
				t.Fatalf("trail is missing %q; got %v", want, actions)
			}
		}
	})
}

func TestInvitationRules(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")

	ownerToken, _ := h.member(t, tenantA, iam.RoleOwner)
	adminToken, _ := h.member(t, tenantA, iam.RoleAdmin)
	assistantToken, _ := h.member(t, tenantA, iam.RoleAssistant)
	_, existingID := h.member(t, tenantA, iam.RoleProfessional)
	existingEmail := h.users.rows[tenantA][iam.UserID(existingID)].Email

	create := func(token string, email, role string) (int, map[string]any) {
		return h.do(t, http.MethodPost, "/api/v1/invitations", token, map[string]any{"email": email, "role": role})
	}

	t.Run("an assistant cannot issue invitations", func(t *testing.T) {
		if status, _ := create(assistantToken, "nuevo@practica.test", string(iam.RoleAssistant)); status != http.StatusForbidden {
			t.Fatalf("assistant create = %d, want 403", status)
		}
	})

	t.Run("an admin cannot invite someone as owner", func(t *testing.T) {
		if status, body := create(adminToken, "dueno@practica.test", string(iam.RoleOwner)); status != http.StatusForbidden {
			t.Fatalf("admin invites owner = %d, want 403: %v", status, body)
		}
	})

	t.Run("an admin can invite staff", func(t *testing.T) {
		if status, body := create(adminToken, "staff@practica.test", string(iam.RoleAccountant)); status != http.StatusCreated {
			t.Fatalf("admin invites accountant = %d, want 201: %v", status, body)
		}
	})

	t.Run("an owner can invite an owner", func(t *testing.T) {
		if status, body := create(ownerToken, "dueno2@practica.test", string(iam.RoleOwner)); status != http.StatusCreated {
			t.Fatalf("owner invites owner = %d, want 201: %v", status, body)
		}
	})

	t.Run("an email that already has an account is 409", func(t *testing.T) {
		if status, body := create(ownerToken, existingEmail, string(iam.RoleAssistant)); status != http.StatusConflict {
			t.Fatalf("existing email = %d, want 409: %v", status, body)
		}
	})

	t.Run("a role outside the catalog and a malformed email are 422", func(t *testing.T) {
		if status, _ := create(ownerToken, "ok@practica.test", "superadmin"); status != http.StatusUnprocessableEntity {
			t.Fatalf("unknown role = %d, want 422", status)
		}
		if status, _ := create(ownerToken, "no-es-un-correo", string(iam.RoleAssistant)); status != http.StatusUnprocessableEntity {
			t.Fatalf("malformed email = %d, want 422", status)
		}
	})

	t.Run("a link from another practice does not open an account here", func(t *testing.T) {
		ownerB := h.issueTokenFor(t, tenantB, iam.RoleOwner)
		foreignSecret, _ := issueInvitation(t, h, ownerB, "otra@practica.test", string(iam.RoleAssistant))

		// Present the same secret under practice A's hint. The hash covers the whole
		// string, so it matches nothing.
		swapped := strings.Replace(foreignSecret, tenantB, tenantA, 1)
		status, body := h.do(t, http.MethodPost, "/api/v1/invitations/accept", "", map[string]any{
			"token": swapped, "fullName": testFullName, "password": testPassword,
		})
		if status != http.StatusGone {
			t.Fatalf("cross-practice accept = %d, want 410: %v", status, body)
		}
	})
}

func TestRevokeInvitation(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	ownerToken, _ := h.member(t, tenantA, iam.RoleOwner)
	adminToken, _ := h.member(t, tenantA, iam.RoleAdmin)
	assistantToken, _ := h.member(t, tenantA, iam.RoleAssistant)

	revoke := func(token, id string) (int, map[string]any) {
		return h.do(t, http.MethodDelete, "/api/v1/invitations/"+id, token, nil)
	}

	t.Run("an assistant cannot revoke", func(t *testing.T) {
		_, id := issueInvitation(t, h, ownerToken, "a@practica.test", string(iam.RoleAssistant))
		if status, _ := revoke(assistantToken, id); status != http.StatusForbidden {
			t.Fatalf("assistant revoke = %d, want 403", status)
		}
	})

	t.Run("a pending invitation is revoked and its link stops working", func(t *testing.T) {
		secret, id := issueInvitation(t, h, ownerToken, "b@practica.test", string(iam.RoleAssistant))
		if status, body := revoke(ownerToken, id); status != http.StatusNoContent {
			t.Fatalf("revoke = %d, want 204: %v", status, body)
		}
		if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token="+secret, "", nil); status != http.StatusGone {
			t.Fatalf("preview after revoke = %d, want 410", status)
		}
	})

	t.Run("revoking twice is 404", func(t *testing.T) {
		_, id := issueInvitation(t, h, ownerToken, "c@practica.test", string(iam.RoleAssistant))
		revoke(ownerToken, id)
		if status, _ := revoke(ownerToken, id); status != http.StatusNotFound {
			t.Fatalf("second revoke = %d, want 404", status)
		}
	})

	t.Run("an admin cannot withdraw an owner invitation", func(t *testing.T) {
		_, id := issueInvitation(t, h, ownerToken, "d@practica.test", string(iam.RoleOwner))
		if status, _ := revoke(adminToken, id); status != http.StatusForbidden {
			t.Fatalf("admin revokes owner invitation = %d, want 403", status)
		}
	})

	t.Run("an accepted invitation cannot be revoked", func(t *testing.T) {
		secret, id := issueInvitation(t, h, ownerToken, "e@practica.test", string(iam.RoleAssistant))
		if status, body := h.do(t, http.MethodPost, "/api/v1/invitations/accept", "", map[string]any{
			"token": secret, "fullName": testFullName, "password": testPassword,
		}); status != http.StatusCreated {
			t.Fatalf("accept = %d, want 201: %v", status, body)
		}
		if status, _ := revoke(ownerToken, id); status != http.StatusConflict {
			t.Fatalf("revoke accepted = %d, want 409", status)
		}
	})
}

func TestInvitationLinkExpiresAndPasswordIsChecked(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	ownerToken, _ := h.member(t, tenantA, iam.RoleOwner)

	t.Run("an expired link answers 410", func(t *testing.T) {
		secret, id := issueInvitation(t, h, ownerToken, "vencida@practica.test", string(iam.RoleAssistant))
		stored := h.invitations.rows[tenantA][iam.InvitationID(id)]
		stored.ExpiresAt = testNow.Add(-time.Minute)
		h.invitations.rows[tenantA][iam.InvitationID(id)] = stored

		if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token="+secret, "", nil); status != http.StatusGone {
			t.Fatalf("preview of expired link = %d, want 410", status)
		}
		if status, _ := h.do(t, http.MethodPost, "/api/v1/invitations/accept", "", map[string]any{
			"token": secret, "fullName": testFullName, "password": testPassword,
		}); status != http.StatusGone {
			t.Fatalf("accept of expired link = %d, want 410", status)
		}
	})

	t.Run("a malformed or unknown token answers the same 410", func(t *testing.T) {
		for _, token := range []string{"no-es-un-token", "inv1.no-uuid.abc", "v1.00000000-0000-0000-0000-000000000000.AAAA"} {
			if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token="+token, "", nil); status != http.StatusGone {
				t.Fatalf("preview(%q) = %d, want 410", token, status)
			}
		}
	})

	t.Run("a password under twelve characters is 422 and the link survives", func(t *testing.T) {
		secret, _ := issueInvitation(t, h, ownerToken, "corta@practica.test", string(iam.RoleAssistant))
		if status, _ := h.do(t, http.MethodPost, "/api/v1/invitations/accept", "", map[string]any{
			"token": secret, "fullName": testFullName, "password": "clave-corta",
		}); status != http.StatusUnprocessableEntity {
			t.Fatalf("short password = %d, want 422", status)
		}
		if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token="+secret, "", nil); status != http.StatusOK {
			t.Fatalf("link after a refused password = %d, want 200: the invitee may retry", status)
		}
	})
}

// issueTokenFor mints a token for an owner of another practice, seeded as an
// account so the actor is real.
func (h *harness) issueTokenFor(t *testing.T, tenantID string, role iam.RoleCode) string {
	t.Helper()
	token, _ := h.member(t, tenantID, role)
	return token
}
