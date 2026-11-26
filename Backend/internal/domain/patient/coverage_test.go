package patient_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// A mutual patient keeps the mutual and the member number; the currency belongs to
// the billing client (T72), not to the patient.
func TestInsuredPatientKeepsCoverage(t *testing.T) {
	draft := validDraft()
	draft.CoverageKind = patient.CoverageInsurer
	draft.InsurerID = str("0c1f3e2a-1111-4222-8333-944455556666")
	draft.MemberNumber = str("  12345  ")

	subject := newPatient(t, draft)

	if subject.CoverageKind != patient.CoverageInsurer {
		t.Errorf("CoverageKind = %q, want insurer", subject.CoverageKind)
	}
	if subject.InsurerID == nil || *subject.InsurerID != "0c1f3e2a-1111-4222-8333-944455556666" {
		t.Errorf("InsurerID = %v, want the mutual", subject.InsurerID)
	}
	if subject.MemberNumber == nil || *subject.MemberNumber != "12345" {
		t.Errorf("MemberNumber = %v, want it trimmed", subject.MemberNumber)
	}
}

// A private patient has no mutual and no member number after a profile change either:
// switching coverage back to particular clears both.
func TestChangingToPrivateClearsTheMutual(t *testing.T) {
	draft := validDraft()
	draft.CoverageKind = patient.CoverageInsurer
	draft.InsurerID = str("0c1f3e2a-1111-4222-8333-944455556666")
	draft.MemberNumber = str("12345")
	insured := newPatient(t, draft)

	edited := validDraft()
	edited.CoverageKind = patient.CoveragePrivate

	updated, err := insured.ChangeProfile(edited, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.CoverageKind != patient.CoveragePrivate || updated.InsurerID != nil || updated.MemberNumber != nil {
		t.Errorf("coverage = (%q, %v, %v), want private with no mutual", updated.CoverageKind, updated.InsurerID, updated.MemberNumber)
	}
}

func TestValidCurrencyIsTheMVPSet(t *testing.T) {
	for _, currency := range []patient.Currency{patient.CurrencyARS, patient.CurrencyEUR, patient.CurrencyUSD} {
		if !patient.ValidCurrency(currency) {
			t.Errorf("ValidCurrency(%q) = false, want true", currency)
		}
	}
	for _, currency := range []patient.Currency{"", "ars", "BRL", "USDT"} {
		if patient.ValidCurrency(currency) {
			t.Errorf("ValidCurrency(%q) = true, want false", currency)
		}
	}
}
