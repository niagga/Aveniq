package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/auth"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

const (
	correctPassword = "correct horse battery staple"
	storedHash      = "$argon2id$real-row"
	dummyHash       = "$argon2id$dummy-row"
	accessTokenTTL  = 15 * time.Minute
)

// ─── doubles ─────────────────────────────────────────────────────────────────

type fakeTenantRepository struct {
	bySlug map[string]tenant.Tenant
}

func (f *fakeTenantRepository) Create(context.Context, tenant.Tenant) error { return nil }

func (f *fakeTenantRepository) BySlug(_ context.Context, slug string) (tenant.Tenant, error) {
	subject, ok := f.bySlug[slug]
	if !ok {
		return tenant.Tenant{}, tenant.ErrNotFound
	}
	return subject, nil
}

func (f *fakeTenantRepository) List(context.Context, int32, int32) ([]tenant.Tenant, error) {
	return nil, nil
}

type fakeUserRepository struct {
	users []iam.User
}

func (f *fakeUserRepository) Create(_ context.Context, subject iam.User) error {
	f.users = append(f.users, subject)
	return nil
}

func (f *fakeUserRepository) ByID(_ context.Context, id iam.UserID) (iam.User, error) {
	for _, user := range f.users {
		if user.ID == id {
			return user, nil
		}
	}
	return iam.User{}, iam.ErrNotFound
}

func (f *fakeUserRepository) ByEmail(_ context.Context, email string) (iam.User, error) {
	for _, user := range f.users {
		if user.Email == email {
			return user, nil
		}
	}
	return iam.User{}, iam.ErrNotFound
}

func (f *fakeUserRepository) List(context.Context, int32, int32) ([]iam.User, error) {
	return f.users, nil
}

func (f *fakeUserRepository) Update(context.Context, iam.User) error   { return nil }
func (f *fakeUserRepository) Delete(context.Context, iam.UserID) error { return nil }

// fakeSessionRepository mirrors the behaviour of the PostgreSQL adapter, because
// the use case depends on that behaviour: revocation is sticky, rotation of a
// spent grant is an invalid transition, and a chain walk follows replaced_by.
type fakeSessionRepository struct {
	sessions map[string]iam.Session
}

func newFakeSessionRepository() *fakeSessionRepository {
	return &fakeSessionRepository{sessions: make(map[string]iam.Session)}
}

func (f *fakeSessionRepository) Create(_ context.Context, subject iam.Session) error {
	key := string(subject.RefreshHash)
	if _, exists := f.sessions[key]; exists {
		return errors.New("duplicate refresh hash")
	}
	f.sessions[key] = subject
	return nil
}

func (f *fakeSessionRepository) ByRefreshHash(_ context.Context, hash iam.RefreshHash) (iam.Session, error) {
	session, ok := f.sessions[string(hash)]
	if !ok {
		return iam.Session{}, iam.ErrNotFound
	}
	return session, nil
}

func (f *fakeSessionRepository) Rotate(ctx context.Context, previous, successor iam.Session) error {
	if err := f.Create(ctx, successor); err != nil {
		return err
	}
	current, err := f.ByRefreshHash(ctx, previous.RefreshHash)
	if err != nil {
		return err
	}
	if current.RevokedAt != nil {
		return iam.ErrInvalidTransition
	}
	f.sessions[string(previous.RefreshHash)] = previous
	return nil
}

func (f *fakeSessionRepository) Revoke(_ context.Context, id iam.SessionID, at time.Time) error {
	for key, session := range f.sessions {
		if session.ID != id {
			continue
		}
		if session.RevokedAt != nil {
			return iam.ErrNotFound
		}
		revoked := at.UTC()
		session.RevokedAt = &revoked
		f.sessions[key] = session
		return nil
	}
	return iam.ErrNotFound
}

func (f *fakeSessionRepository) RevokeChain(_ context.Context, id iam.SessionID, at time.Time) (int64, error) {
	var affected int64
	seen := make(map[iam.SessionID]bool)

	var walk func(current iam.SessionID)
	walk = func(current iam.SessionID) {
		if seen[current] {
			return
		}
		seen[current] = true
		for key, session := range f.sessions {
			if session.ID != current {
				continue
			}
			if session.RevokedAt == nil {
				revoked := at.UTC()
				session.RevokedAt = &revoked
				f.sessions[key] = session
				affected++
			}
			if session.ReplacedBy != nil {
				walk(*session.ReplacedBy)
			}
		}
	}
	walk(id)

	if affected == 0 {
		return 0, iam.ErrNotFound
	}
	return affected, nil
}

// fakeScope runs the unit of work directly: the doubles have no transaction to
// open, and what matters for these tests is that the use case does open one.
type fakeScope struct{ opened int }

func (s *fakeScope) WithTenant(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	s.opened++
	return fn(ctx)
}

// recordingVerifier records every hash it is asked about, which is how the tests
// prove the dummy verification actually happens for an unknown user.
type recordingVerifier struct {
	checked []string
}

func (v *recordingVerifier) Verify(password, encoded string) error {
	v.checked = append(v.checked, encoded)
	if encoded == dummyHash {
		return authn.ErrPasswordMismatch
	}
	if password == correctPassword {
		return nil
	}
	return authn.ErrPasswordMismatch
}

type fakeTokens struct{}

func (fakeTokens) Issue(claims authn.AccessClaims) (string, error) {
	return "access|" + claims.Subject + "|" + claims.TenantID + "|" + claims.Role, nil
}

func (fakeTokens) TTL() time.Duration { return accessTokenTTL }

type movingClock struct{ now time.Time }

func (c *movingClock) Now() time.Time { return c.now }

type uuidIDs struct{}

func (uuidIDs) NewID() string { return uuid.NewString() }

// ─── world ───────────────────────────────────────────────────────────────────

type world struct {
	login    *auth.LoginUseCase
	refresh  *auth.RefreshUseCase
	logout   *auth.LogoutUseCase
	users    *fakeUserRepository
	sessions *fakeSessionRepository
	verifier *recordingVerifier
	scope    *fakeScope
	clock    *movingClock
	tenant   tenant.Tenant
	user     iam.User
}

func newWorld(t *testing.T) *world {
	t.Helper()

	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	clock := &movingClock{now: now}

	subject, err := tenant.New(tenant.ID(uuid.NewString()), "consultorio-sur", "Consultorio Sur", now)
	if err != nil {
		t.Fatalf("cannot build the tenant: %v", err)
	}

	user, err := iam.NewUser(
		iam.UserID(uuid.NewString()),
		subject.ID,
		"ana@consultorio.test",
		storedHash,
		"Ana Pérez",
		iam.RoleOwner,
		now,
	)
	if err != nil {
		t.Fatalf("cannot build the user: %v", err)
	}

	users := &fakeUserRepository{}
	if err := users.Create(context.Background(), user); err != nil {
		t.Fatalf("cannot seed the user: %v", err)
	}

	sessions := newFakeSessionRepository()
	scope := &fakeScope{}
	verifier := &recordingVerifier{}

	login, err := auth.NewLoginUseCase(auth.LoginDeps{
		Tenants:   &fakeTenantRepository{bySlug: map[string]tenant.Tenant{"consultorio-sur": subject}},
		Users:     users,
		Sessions:  sessions,
		Scope:     scope,
		Verifier:  verifier,
		Tokens:    fakeTokens{},
		Clock:     clock,
		IDs:       uuidIDs{},
		DummyHash: dummyHash,
	})
	if err != nil {
		t.Fatalf("cannot build the login use case: %v", err)
	}

	return &world{
		login:    login,
		refresh:  auth.NewRefreshUseCase(auth.RefreshDeps{Users: users, Sessions: sessions, Scope: scope, Tokens: fakeTokens{}, Clock: clock, IDs: uuidIDs{}}),
		logout:   auth.NewLogoutUseCase(auth.LogoutDeps{Sessions: sessions, Scope: scope, Clock: clock}),
		users:    users,
		sessions: sessions,
		verifier: verifier,
		scope:    scope,
		clock:    clock,
		tenant:   subject,
		user:     user,
	}
}

func (w *world) loginInput() auth.LoginInput {
	return auth.LoginInput{TenantSlug: "consultorio-sur", Email: "ana@consultorio.test", Password: correctPassword}
}

// sessionsOf reports the stored sessions, so a test can assert on side effects.
func (w *world) storedSessions() []iam.Session {
	out := make([]iam.Session, 0, len(w.sessions.sessions))
	for _, session := range w.sessions.sessions {
		out = append(out, session)
	}
	return out
}

// ─── login ───────────────────────────────────────────────────────────────────

func TestLogin(t *testing.T) {
	ctx := context.Background()

	t.Run("valid credentials return a token pair and open one session", func(t *testing.T) {
		w := newWorld(t)

		session, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(session.AccessToken, "access|"+string(w.user.ID)) {
			t.Errorf("access token = %q, want the user as its subject", session.AccessToken)
		}
		if session.ExpiresIn != accessTokenTTL {
			t.Errorf("ExpiresIn = %v, want %v", session.ExpiresIn, accessTokenTTL)
		}
		if session.Role != string(iam.RoleOwner) || session.TenantID != string(w.tenant.ID) {
			t.Errorf("identity = %q/%q, want the owner of this tenant", session.Role, session.TenantID)
		}
		if _, err := authn.ParseRefreshToken(session.RefreshToken); err != nil {
			t.Errorf("the issued refresh token does not parse: %v", err)
		}
		if !session.RefreshExpiresAt.Equal(w.clock.now.Add(auth.RefreshSessionTTL)) {
			t.Errorf("RefreshExpiresAt = %v, want %v", session.RefreshExpiresAt, w.clock.now.Add(auth.RefreshSessionTTL))
		}
		if stored := w.storedSessions(); len(stored) != 1 {
			t.Fatalf("stored sessions = %d, want 1", len(stored))
		}
		if w.scope.opened != 1 {
			t.Errorf("tenant scopes opened = %d, want 1: everything but the tenant lookup runs inside one", w.scope.opened)
		}
	})

	t.Run("a wrong password is refused and opens nothing", func(t *testing.T) {
		w := newWorld(t)
		in := w.loginInput()
		in.Password = "wrong"

		if _, err := w.login.Execute(ctx, in); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidCredentials)
		}
		if stored := w.storedSessions(); len(stored) != 0 {
			t.Fatalf("stored sessions = %d, want 0", len(stored))
		}
	})

	// Decision 8 of L11: an unknown address must cost the same as a known one, so
	// the use case has to spend a real verification against the dummy hash.
	t.Run("an unknown email is refused and still spends a verification", func(t *testing.T) {
		w := newWorld(t)
		in := w.loginInput()
		in.Email = "nobody@consultorio.test"

		if _, err := w.login.Execute(ctx, in); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidCredentials)
		}
		if len(w.verifier.checked) != 1 || w.verifier.checked[0] != dummyHash {
			t.Fatalf("verified hashes = %v, want exactly the dummy hash: otherwise the response time says who exists", w.verifier.checked)
		}
	})

	t.Run("an inactive user is refused like a wrong password", func(t *testing.T) {
		w := newWorld(t)
		inactive, err := w.user.Deactivate(w.clock.now)
		if err != nil {
			t.Fatalf("cannot deactivate: %v", err)
		}
		w.users.users = []iam.User{inactive}

		if _, err := w.login.Execute(ctx, w.loginInput()); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidCredentials)
		}
		if len(w.verifier.checked) != 1 || w.verifier.checked[0] != dummyHash {
			t.Fatalf("verified hashes = %v, want the dummy hash", w.verifier.checked)
		}
	})

	t.Run("an unknown tenant is refused with the same error", func(t *testing.T) {
		w := newWorld(t)
		in := w.loginInput()
		in.TenantSlug = "does-not-exist"

		if _, err := w.login.Execute(ctx, in); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidCredentials)
		}
	})

	t.Run("a malformed email is refused without touching the database", func(t *testing.T) {
		w := newWorld(t)
		in := w.loginInput()
		in.Email = "not-an-email"

		if _, err := w.login.Execute(ctx, in); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidCredentials)
		}
		if w.scope.opened != 0 {
			t.Errorf("tenant scopes opened = %d, want 0: an unstorable address cannot belong to a user", w.scope.opened)
		}
	})

	t.Run("login needs a dummy hash to be constructed", func(t *testing.T) {
		w := newWorld(t)
		if _, err := auth.NewLoginUseCase(auth.LoginDeps{}); err == nil {
			t.Fatal("NewLoginUseCase without DummyHash = nil error, want a rejection")
		}
		_ = w
	})
}

// ─── refresh ─────────────────────────────────────────────────────────────────

func TestRefresh(t *testing.T) {
	ctx := context.Background()

	t.Run("a live grant rotates and the presented one is spent", func(t *testing.T) {
		w := newWorld(t)
		first, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("cannot log in: %v", err)
		}

		second, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken})
		if err != nil {
			t.Fatalf("cannot refresh: %v", err)
		}
		if second.RefreshToken == first.RefreshToken {
			t.Fatal("the rotation reused the presented token instead of minting a new one")
		}

		parsedFirst, err := authn.ParseRefreshToken(first.RefreshToken)
		if err != nil {
			t.Fatalf("cannot parse the first token: %v", err)
		}
		spent, err := w.sessions.ByRefreshHash(ctx, iam.RefreshHash(parsedFirst.Hash))
		if err != nil {
			t.Fatalf("cannot read the spent grant: %v", err)
		}
		if spent.RevokedAt == nil {
			t.Error("the rotated grant is still live")
		}
		if spent.ReplacedBy == nil {
			t.Error("the rotated grant does not point at its successor")
		}

		// And the new token works, which is what proves the successor was stored.
		if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: second.RefreshToken}); err != nil {
			t.Fatalf("the successor does not rotate: %v", err)
		}
	})

	// Decision 3 of L11: a replay kills every successor, not just the leaf.
	t.Run("reuse revokes the whole chain", func(t *testing.T) {
		w := newWorld(t)
		first, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("cannot log in: %v", err)
		}
		second, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken})
		if err != nil {
			t.Fatalf("cannot refresh: %v", err)
		}

		// The stolen copy is presented after the legitimate rotation.
		if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken}); !errors.Is(err, auth.ErrRefreshReused) {
			t.Fatalf("error = %v, want %v", err, auth.ErrRefreshReused)
		}

		// Both the spent grant and the live successor are now dead.
		for _, raw := range []string{first.RefreshToken, second.RefreshToken} {
			parsed, err := authn.ParseRefreshToken(raw)
			if err != nil {
				t.Fatalf("cannot parse: %v", err)
			}
			session, err := w.sessions.ByRefreshHash(ctx, iam.RefreshHash(parsed.Hash))
			if err != nil {
				t.Fatalf("cannot read the session: %v", err)
			}
			if session.RevokedAt == nil {
				t.Errorf("session %q survived the reuse revocation", session.ID)
			}
		}

		// The revoked successor must not rotate either.
		if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: second.RefreshToken}); !errors.Is(err, auth.ErrRefreshReused) {
			t.Fatalf("error = %v, want %v: a revoked chain must not mint new tokens", err, auth.ErrRefreshReused)
		}
	})

	t.Run("an expired grant is refused", func(t *testing.T) {
		w := newWorld(t)
		first, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("cannot log in: %v", err)
		}

		w.clock.now = w.clock.now.Add(auth.RefreshSessionTTL + time.Minute)

		if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken}); !errors.Is(err, auth.ErrSessionExpired) {
			t.Fatalf("error = %v, want %v", err, auth.ErrSessionExpired)
		}
	})

	t.Run("a token that parses but matches nothing is refused", func(t *testing.T) {
		w := newWorld(t)
		orphan, err := authn.NewRefreshToken(string(w.tenant.ID))
		if err != nil {
			t.Fatalf("cannot mint a token: %v", err)
		}

		if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: orphan.Raw}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidRefreshToken)
		}
	})

	t.Run("a malformed token never reaches the database", func(t *testing.T) {
		w := newWorld(t)

		for _, raw := range []string{"", "v1.not-a-uuid.abc", "nonsense"} {
			if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: raw}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
				t.Fatalf("token %q: error = %v, want %v", raw, err, auth.ErrInvalidRefreshToken)
			}
		}
		if w.scope.opened != 0 {
			t.Errorf("tenant scopes opened = %d, want 0", w.scope.opened)
		}
	})

	t.Run("a deactivated user cannot refresh and the chain is revoked", func(t *testing.T) {
		w := newWorld(t)
		first, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("cannot log in: %v", err)
		}

		deactivated, err := w.user.Deactivate(w.clock.now)
		if err != nil {
			t.Fatalf("cannot deactivate: %v", err)
		}
		w.users.users = []iam.User{deactivated}

		if _, err := w.refresh.Execute(ctx, auth.RefreshInput{RefreshToken: first.RefreshToken}); !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("error = %v, want %v", err, auth.ErrInvalidRefreshToken)
		}

		parsed, err := authn.ParseRefreshToken(first.RefreshToken)
		if err != nil {
			t.Fatalf("cannot parse: %v", err)
		}
		stored, err := w.sessions.ByRefreshHash(ctx, iam.RefreshHash(parsed.Hash))
		if err != nil {
			t.Fatalf("cannot read the session: %v", err)
		}
		if stored.RevokedAt == nil {
			t.Error("the grant of a deactivated user is still live")
		}
	})
}

// ─── logout ──────────────────────────────────────────────────────────────────

func TestLogout(t *testing.T) {
	ctx := context.Background()

	t.Run("logout revokes the presented grant", func(t *testing.T) {
		w := newWorld(t)
		first, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("cannot log in: %v", err)
		}

		if err := w.logout.Execute(ctx, auth.LogoutInput{RefreshToken: first.RefreshToken}); err != nil {
			t.Fatalf("cannot log out: %v", err)
		}

		parsed, err := authn.ParseRefreshToken(first.RefreshToken)
		if err != nil {
			t.Fatalf("cannot parse: %v", err)
		}
		stored, err := w.sessions.ByRefreshHash(ctx, iam.RefreshHash(parsed.Hash))
		if err != nil {
			t.Fatalf("cannot read the session: %v", err)
		}
		if stored.RevokedAt == nil {
			t.Error("the session is still live after logout")
		}
	})

	// Logout must not become an oracle: every input answers success.
	t.Run("logout is idempotent and silent about unknown tokens", func(t *testing.T) {
		w := newWorld(t)
		first, err := w.login.Execute(ctx, w.loginInput())
		if err != nil {
			t.Fatalf("cannot log in: %v", err)
		}

		for _, tc := range []struct {
			name  string
			token string
		}{
			{"second logout of the same grant", first.RefreshToken},
			{"garbage", "not-a-token"},
			{"empty", ""},
			{"orphan uuid", "v1." + uuid.NewString() + "." + strings.Repeat("a", 43)},
		} {
			if err := w.logout.Execute(ctx, auth.LogoutInput{RefreshToken: tc.token}); err != nil {
				t.Errorf("%s: error = %v, want nil", tc.name, err)
			}
		}
	})
}
