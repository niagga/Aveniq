package patient_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// A mutual patient keeps the mutual and the member number, and the currency is the one
// chosen, not one derived from the country.
func TestInsuredPatientKeepsCoverageAndChosenCurrency(t *testing.T) {
	draft := validDraft()
	draft.CoverageKind = patient.CoverageInsurer
	draft.InsurerID = str("0c1f3e2a-1111-4222-8333-944455556666")
	draft.MemberNumber = str("  12345  ")
	draft.BillingCurrency = patient.CurrencyUSD

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
	if subject.BillingCurrency != patient.CurrencyUSD {
		t.Errorf("BillingCurrency = %q, want USD", subject.BillingCurrency)
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
	edited.BillingCurrency = patient.CurrencyEUR

	updated, err := insured.ChangeProfile(edited, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.CoverageKind != patient.CoveragePrivate || updated.InsurerID != nil || updated.MemberNumber != nil {
		t.Errorf("coverage = (%q, %v, %v), want private with no mutual", updated.CoverageKind, updated.InsurerID, updated.MemberNumber)
	}
	if updated.BillingCurrency != patient.CurrencyEUR {
		t.Errorf("BillingCurrency = %q, want EUR", updated.BillingCurrency)
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
