package iam_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

var (
	testSessionID = iam.SessionID("0192f3a4-0000-7000-8000-0000000000b1")
	now           = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
)

func TestNewSession(t *testing.T) {
	t.Parallel()

	newSession := func(t *testing.T) (iam.Session, error) {
		t.Helper()
		return iam.NewSession(
			testSessionID,
			testTenantID,
			testUserID,
			iam.RefreshHash("sha256-of-refresh"),
			now,
			now.Add(30*24*time.Hour),
		)
	}

	t.Run("accepts a valid session and applies the defaults", func(t *testing.T) {
		t.Parallel()

		got, err := newSession(t)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != testSessionID || got.TenantID != testTenantID || got.UserID != testUserID {
			t.Errorf("identity = %q/%q/%q, want %q/%q/%q", got.ID, got.TenantID, got.UserID, testSessionID, testTenantID, testUserID)
		}
		if got.RevokedAt != nil {
			t.Errorf("RevokedAt = %v, want nil (a new session is live)", got.RevokedAt)
		}
		if got.ReplacedBy != nil {
			t.Errorf("ReplacedBy = %q, want nil (a new session has no successor)", *got.ReplacedBy)
		}
		if !got.CreatedAt.Equal(now) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
		}
	})

	t.Run("keeps optional metadata", func(t *testing.T) {
		t.Parallel()

		agent := "Mozilla/5.0"
		ip := "10.0.0.7"

		got, err := iam.NewSession(
			testSessionID, testTenantID, testUserID,
			iam.RefreshHash("sha256-of-refresh"),
			now, now.Add(30*24*time.Hour),
			iam.WithUserAgent(&agent), iam.WithIP(&ip),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.UserAgent == nil || *got.UserAgent != agent {
			t.Errorf("UserAgent = %v, want %q", got.UserAgent, agent)
		}
		if got.IP == nil || got.IP.String() != ip {
			t.Errorf("IP = %v, want %s", got.IP, ip)
		}
	})

	invalid := []struct {
		name      string
		hash      iam.RefreshHash
		created   time.Time
		expires   time.Time
		wantField string
	}{
		{name: "blank refresh hash", hash: "  ", created: now, expires: now.Add(time.Hour), wantField: "refresh_hash"},
		{name: "expiry before creation", hash: "sha256-of-refresh", created: now, expires: now, wantField: ""},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := iam.NewSession(
				testSessionID, testTenantID, testUserID,
				tc.hash, tc.created, tc.expires,
			)
			if err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
			if tc.wantField == "" {
				if !errors.Is(err, iam.ErrInvalidTransition) {
					t.Fatalf("error = %v, want %v", err, iam.ErrInvalidTransition)
				}
				return
			}
			var validationErr *iam.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Field != tc.wantField {
				t.Fatalf("error = %v, want *iam.ValidationError on field %q", err, tc.wantField)
			}
		})
	}
}

func TestSessionRevocation(t *testing.T) {
	t.Parallel()

	live, err := iam.NewSession(
		testSessionID, testTenantID, testUserID,
		iam.RefreshHash("sha256-of-refresh"),
		now, now.Add(30*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	later := now.Add(time.Hour)
	evenLater := later.Add(time.Hour)

	t.Run("revokes a live session", func(t *testing.T) {
		t.Parallel()

		got, err := live.Revoke(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.RevokedAt == nil || !got.RevokedAt.Equal(later) {
			t.Errorf("RevokedAt = %v, want %v", got.RevokedAt, later)
		}
		if live.RevokedAt != nil {
			t.Errorf("Revoke must not mutate the receiver: RevokedAt = %v", live.RevokedAt)
		}
	})

	t.Run("refuses to revoke an already revoked session", func(t *testing.T) {
		t.Parallel()

		revoked, err := live.Revoke(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := revoked.Revoke(evenLater); !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v", err, iam.ErrInvalidTransition)
		}
	})

	t.Run("rotates a live session onto its successor", func(t *testing.T) {
		t.Parallel()

		successor := iam.SessionID("0192f3a4-0000-7000-8000-000000000002")
		got, err := live.ReplaceWith(successor, later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.RevokedAt == nil || !got.RevokedAt.Equal(later) {
			t.Errorf("RevokedAt = %v, want %v (rotation revokes the predecessor)", got.RevokedAt, later)
		}
		if got.ReplacedBy == nil || *got.ReplacedBy != successor {
			t.Errorf("ReplacedBy = %v, want %q", got.ReplacedBy, successor)
		}
		if live.ReplacedBy != nil {
			t.Errorf("ReplaceWith must not mutate the receiver: ReplacedBy = %v", live.ReplacedBy)
		}
	})

	t.Run("refuses to rotate a revoked session", func(t *testing.T) {
		t.Parallel()

		revoked, err := live.Revoke(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := revoked.ReplaceWith(iam.SessionID("successor"), evenLater); !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v", err, iam.ErrInvalidTransition)
		}
	})
}
