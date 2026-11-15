package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

// The constraint names are the contract between the migrations and the error mapping. A
// renamed constraint would silently turn a refused voucher into an unmapped 500, so the names
// the mapping relies on are checked here without a database.
func TestBillingConstraintsMapToTheirErrors(t *testing.T) {
	cases := []struct {
		constraint string
		want       error
	}{
		{"vouchers_patient_same_tenant", billing.ErrPatientNotVisible},
		{"vouchers_appointment_of_patient", billing.ErrAppointmentNotVisible},
		{"vouchers_issuer_same_tenant", billing.ErrActorNotVisible},
	}
	for _, tc := range cases {
		t.Run(tc.constraint, func(t *testing.T) {
			err := translateBillingError(&pgconn.PgError{Code: foreignKeyViolationCode, ConstraintName: tc.constraint})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

}
