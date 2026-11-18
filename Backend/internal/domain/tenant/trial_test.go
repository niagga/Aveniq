package tenant_test

import (
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// The trial rule is a boundary question: the end is inclusive, and only a trial can
// expire.
func TestTrialExpired(t *testing.T) {
	created := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	ends := created.Add(tenant.TrialDuration)

	trialWith := func(status tenant.Status, endsAt time.Time) tenant.Tenant {
		t.Helper()
		subject, err := tenant.Reconstitute(tenant.ID("11111111-1111-4111-8111-111111111111"), "consultorio-trial", "Consultorio", status, "", created, endsAt)
		if err != nil {
			t.Fatalf("cannot reconstitute the practice: %v", err)
		}
		return subject
	}

	cases := []struct {
		name    string
		subject tenant.Tenant
		now     time.Time
		want    bool
	}{
		{"a trial inside its days is not expired", trialWith(tenant.StatusTrial, ends), ends.Add(-time.Second), false},
		{"the instant the trial ends it is expired", trialWith(tenant.StatusTrial, ends), ends, true},
		{"a trial past its end is expired", trialWith(tenant.StatusTrial, ends), ends.Add(time.Hour), true},
		{"an active practice never expires by the trial rule", trialWith(tenant.StatusActive, ends), ends.Add(365 * 24 * time.Hour), false},
		{"a suspended practice is decided elsewhere", trialWith(tenant.StatusSuspended, ends), ends.Add(time.Hour), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.subject.TrialExpired(tc.now); got != tc.want {
				t.Fatalf("TrialExpired(%s) = %v, want %v", tc.now.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}
