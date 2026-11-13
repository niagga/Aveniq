package tenant_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

func TestNewTenantStartsATrialOfFourteenDays(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	got, err := tenant.New(tenant.ID("0192f3a4-0000-7000-8000-000000000010"), "consultorio-sur", "Consultorio Sur", created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantEnd := created.Add(14 * 24 * time.Hour)
	if !got.TrialEndsAt.Equal(wantEnd) {
		t.Fatalf("TrialEndsAt = %v, want %v (14 days after creation)", got.TrialEndsAt, wantEnd)
	}
}

func TestReconstituteKeepsTheStoredTrialEnd(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	stored := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	got, err := tenant.Reconstitute(
		tenant.ID("0192f3a4-0000-7000-8000-000000000011"), "sur", "Sur", tenant.StatusTrial,
		tenant.DefaultTimezone, created, stored,
	)
	if err != nil {
		t.Fatalf("Reconstitute: %v", err)
	}
	if !got.TrialEndsAt.Equal(stored) {
		t.Fatalf("TrialEndsAt = %v, want the stored %v: changing TrialDuration must not move existing trials", got.TrialEndsAt, stored)
	}
}

func TestReconstituteRejectsATrialEndNotAfterCreation(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	for name, end := range map[string]time.Time{
		"same instant": created,
		"before":       created.Add(-time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := tenant.Reconstitute(
				tenant.ID("0192f3a4-0000-7000-8000-000000000012"), "sur", "Sur", tenant.StatusTrial,
				tenant.DefaultTimezone, created, end,
			)
			var validation *tenant.ValidationError
			if !errors.As(err, &validation) || validation.Field != "trial_ends_at" {
				t.Fatalf("error = %v, want a ValidationError on trial_ends_at", err)
			}
		})
	}
}
