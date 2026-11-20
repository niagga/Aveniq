package billing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

func str(value string) *string { return &value }

var settlementNow = time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC)

func mutual(taxID *string) billing.SettlementInsurer {
	return billing.SettlementInsurer{ID: "0c1f3e2a-1111-4222-8333-944455556666", Name: "OSDE Demo", TaxID: taxID, Active: true}
}

func item(appointmentID, currency string, amount int64) billing.SettlementItem {
	return billing.SettlementItem{
		AppointmentID: appointmentID, PatientID: "patient-1", PatientName: "Ana Pérez", MemberNumber: "A-778",
		ProfessionalID: "pro-1", ProfessionalName: "Dra. Demo", Modality: "presencial",
		StartsAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC), Currency: currency, AmountMinor: amount,
	}
}

func period(t *testing.T, value string) time.Time {
	t.Helper()
	first, err := billing.ParsePeriod(value)
	if err != nil {
		t.Fatalf("ParsePeriod(%q): %v", value, err)
	}
	return first
}

func TestParsePeriodReadsAMonthAndRefusesTheRest(t *testing.T) {
	first := period(t, "2026-11")
	if first.Day() != 1 || first.Month() != time.November || first.Year() != 2026 {
		t.Fatalf("ParsePeriod(2026-11) = %s, want 2026-11-01", first)
	}
	for _, bad := range []string{"", "2026-13", "2026-00", "2026-1", "11-2026", "2026-11-01", "abc"} {
		_, err := billing.ParsePeriod(bad)
		var v *billing.ValidationError
		if !errors.As(err, &v) || v.Field != "period" {
			t.Errorf("ParsePeriod(%q) = %v, want a period validation error", bad, err)
		}
	}
}

func TestMonthBoundsAreTheCivilMonthInThePracticeTimezone(t *testing.T) {
	location, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	from, to := billing.MonthBounds(period(t, "2026-11"), location)
	if got := from.In(location).Format(time.RFC3339); got != "2026-11-01T00:00:00-03:00" {
		t.Errorf("from = %s, want the first instant of 1 November locally", got)
	}
	if got := to.In(location).Format(time.RFC3339); got != "2026-12-01T00:00:00-03:00" {
		t.Errorf("to = %s, want the first instant of 1 December locally", got)
	}
}

func TestASettlementNeedsTheMutualsCUIT(t *testing.T) {
	_, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(nil),
		period(t, "2026-11"), []billing.SettlementItem{item("a-1", "ARS", 800000)}, settlementNow)
	var v *billing.ValidationError
	if !errors.As(err, &v) || v.Field != "tax_id" {
		t.Fatalf("settlement without CUIT: %v, want a tax_id validation error", err)
	}

	short := str("2030111")
	_, err = billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(short),
		period(t, "2026-11"), []billing.SettlementItem{item("a-1", "ARS", 800000)}, settlementNow)
	if !errors.As(err, &v) || v.Field != "tax_id" {
		t.Fatalf("settlement with a short CUIT: %v, want a tax_id validation error", err)
	}
}

func TestAnEmptySettlementIsNotADocument(t *testing.T) {
	_, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(str("20301112223")),
		period(t, "2026-11"), nil, settlementNow)
	var v *billing.ValidationError
	if !errors.As(err, &v) || v.Field != "period" {
		t.Fatalf("settlement with no sessions: %v, want a period validation error", err)
	}
}

func TestASessionIsSettledOncePerSettlement(t *testing.T) {
	_, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(str("20301112223")),
		period(t, "2026-11"), []billing.SettlementItem{item("a-1", "ARS", 800000), item("a-1", "ARS", 800000)}, settlementNow)
	var v *billing.ValidationError
	if !errors.As(err, &v) || v.Field != "appointment_id" {
		t.Fatalf("duplicate session: %v, want an appointment_id validation error", err)
	}
}

func TestTheSettlementCoversTheWholeMonthAndSnapshotsTheMutual(t *testing.T) {
	built, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "  Consultorio Demo  ", mutual(str("20301112223")),
		period(t, "2026-11"), []billing.SettlementItem{item("a-1", "ARS", 800000)}, settlementNow)
	if err != nil {
		t.Fatalf("NewSettlement: %v", err)
	}
	if built.PeriodStart.Format("2006-01-02") != "2026-11-01" || built.PeriodEnd.Format("2006-01-02") != "2026-11-30" {
		t.Errorf("period = %s..%s, want 2026-11-01..2026-11-30", built.PeriodStart.Format("2006-01-02"), built.PeriodEnd.Format("2006-01-02"))
	}
	if built.PracticeName != "Consultorio Demo" || built.InsurerTaxID != "20301112223" || built.InsurerName != "OSDE Demo" {
		t.Errorf("snapshot = %q / %q / %q, want the trimmed practice and the mutual as read", built.PracticeName, built.InsurerName, built.InsurerTaxID)
	}
	if built.Status != billing.StatusIssued {
		t.Errorf("status = %q, want issued", built.Status)
	}
}

func TestVoidingASettlementReleasesItsSessionsAndIsFinal(t *testing.T) {
	built, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(str("20301112223")),
		period(t, "2026-11"), []billing.SettlementItem{item("a-1", "ARS", 800000), item("a-2", "ARS", 800000)}, settlementNow)
	if err != nil {
		t.Fatalf("NewSettlement: %v", err)
	}

	voided, err := built.Void("actor-2", "  Se cargó mal el CUIT  ", settlementNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if voided.Status != billing.StatusVoided || voided.VoidReason == nil || *voided.VoidReason != "Se cargó mal el CUIT" {
		t.Fatalf("void = %+v, want voided with the trimmed reason", voided)
	}
	for _, released := range voided.Items {
		if released.ReleasedAt == nil {
			t.Errorf("session %s is not released by the void", released.AppointmentID)
		}
	}

	if _, err := voided.Void("actor-2", "otra vez", settlementNow.Add(2*time.Hour)); !errors.Is(err, billing.ErrAlreadyVoided) {
		t.Fatalf("second void: %v, want ErrAlreadyVoided", err)
	}
}

func TestVoidNeedsAReason(t *testing.T) {
	built, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(str("20301112223")),
		period(t, "2026-11"), []billing.SettlementItem{item("a-1", "ARS", 800000)}, settlementNow)
	if err != nil {
		t.Fatalf("NewSettlement: %v", err)
	}
	_, err = built.Void("actor-2", "   ", settlementNow.Add(time.Hour))
	var v *billing.ValidationError
	if !errors.As(err, &v) || v.Field != "void_reason" {
		t.Fatalf("void without reason: %v, want a void_reason validation error", err)
	}
}

func TestTotalsAreSummedPerCurrencyWithoutConverting(t *testing.T) {
	built, err := billing.NewSettlement("s-1", tenant.ID("t-1"), "actor-1", "Consultorio", mutual(str("20301112223")),
		period(t, "2026-11"),
		[]billing.SettlementItem{
			item("a-1", "USD", 2500),
			item("a-2", "ARS", 800000),
			item("a-3", "ARS", 650000),
		}, settlementNow)
	if err != nil {
		t.Fatalf("NewSettlement: %v", err)
	}
	totals := built.Totals()
	if len(totals) != 2 {
		t.Fatalf("totals = %+v, want two currencies", totals)
	}
	if totals[0].Currency != "ARS" || totals[0].Sessions != 2 || totals[0].AmountMinor != 1450000 {
		t.Errorf("ARS total = %+v, want 2 sessions and 1450000", totals[0])
	}
	if totals[1].Currency != "USD" || totals[1].Sessions != 1 || totals[1].AmountMinor != 2500 {
		t.Errorf("USD total = %+v, want 1 session and 2500", totals[1])
	}
}

// TestTheCopagoIsShownNeverSummedAndNeverNegative: the settlement bills the mutual for its share.
// The copago is kept on the item for the detail, so it stays out of the totals; a negative one is
// refused.
func TestTheCopagoIsShownNeverSummedAndNeverNegative(t *testing.T) {
	withCopago := item("appt-1", "ARS", 800000)
	withCopago.CopagoMinor = 150000
	settlement, err := billing.NewSettlement("settlement-1", tenant.ID("tenant-a"), "actor-1", "IAM A",
		mutual(str("30711223334")), period(t, "2026-11"), []billing.SettlementItem{withCopago}, settlementNow)
	if err != nil {
		t.Fatalf("NewSettlement: %v", err)
	}
	if settlement.Items[0].CopagoMinor != 150000 {
		t.Errorf("copago = %d, want 150000 kept on the item", settlement.Items[0].CopagoMinor)
	}
	totals := settlement.Totals()
	if len(totals) != 1 || totals[0].AmountMinor != 800000 {
		t.Errorf("totals = %+v, want the mutual's share only (800000)", totals)
	}

	negative := item("appt-2", "ARS", 800000)
	negative.CopagoMinor = -1
	_, err = billing.NewSettlement("settlement-2", tenant.ID("tenant-a"), "actor-1", "IAM A",
		mutual(str("30711223334")), period(t, "2026-11"), []billing.SettlementItem{negative}, settlementNow)
	var v *billing.ValidationError
	if !errors.As(err, &v) || v.Field != "copago_minor" {
		t.Errorf("negative copago: error = %v, want a copago_minor validation error", err)
	}
}
