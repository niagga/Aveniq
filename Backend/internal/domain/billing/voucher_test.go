package billing_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var now = time.Date(2026, 11, 15, 15, 0, 0, 0, time.UTC)

func validDraft() billing.IssueDraft {
	return billing.IssueDraft{
		Concept:      "  Sesión individual  ",
		AmountMinor:  150000,
		Currency:     "ARS",
		PracticeName: "  Consultorio Norte  ",
	}
}

func newIssued(t *testing.T, draft billing.IssueDraft) billing.Voucher {
	t.Helper()
	voucher, err := billing.NewIssued(
		billing.VoucherID(uuid.NewString()),
		tenant.ID(uuid.NewString()),
		uuid.NewString(),
		draft,
		now,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return voucher
}

func TestNewIssued(t *testing.T) {
	voucher := newIssued(t, validDraft())

	if voucher.Concept != "Sesión individual" {
		t.Errorf("Concept = %q, want it trimmed", voucher.Concept)
	}
	if voucher.PracticeName != "Consultorio Norte" {
		t.Errorf("PracticeName = %q, want it trimmed", voucher.PracticeName)
	}
	if voucher.Number != 0 {
		t.Errorf("Number = %d, want 0 until the repository assigns it", voucher.Number)
	}
	if voucher.Status != billing.StatusIssued {
		t.Errorf("Status = %q, want issued", voucher.Status)
	}
	if voucher.PatientID != "" || voucher.BuyerName != nil || voucher.ClientID != "" {
		t.Errorf("a loose charge must carry no buyer and no client, got patient=%q buyer=%v client=%q", voucher.PatientID, voucher.BuyerName, voucher.ClientID)
	}
}

func TestNewIssuedValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*billing.IssueDraft)
		field  string
	}{
		"blank concept":      {func(d *billing.IssueDraft) { d.Concept = "   " }, "concept"},
		"empty concept":      {func(d *billing.IssueDraft) { d.Concept = "" }, "concept"},
		"concept too long":   {func(d *billing.IssueDraft) { d.Concept = strings.Repeat("a", 201) }, "concept"},
		"zero amount":        {func(d *billing.IssueDraft) { d.AmountMinor = 0 }, "amount_minor"},
		"negative amount":    {func(d *billing.IssueDraft) { d.AmountMinor = -1 }, "amount_minor"},
		"lowercase currency": {func(d *billing.IssueDraft) { d.Currency = "ars" }, "currency"},
		"short currency":     {func(d *billing.IssueDraft) { d.Currency = "AR" }, "currency"},
		"blank practice":     {func(d *billing.IssueDraft) { d.PracticeName = "  " }, "practice_name"},
		"buyer without patient": {func(d *billing.IssueDraft) {
			name := "Ana"
			d.BuyerName = &name
		}, "buyer_name"},
		"patient without buyer": {func(d *billing.IssueDraft) {
			d.PatientID = uuid.NewString()
			d.ClientID = uuid.NewString()
		}, "buyer_name"},
		"patient without client": {func(d *billing.IssueDraft) {
			name := "Ana"
			d.PatientID = uuid.NewString()
			d.BuyerName = &name
		}, "client_id"},
		"client without patient": {func(d *billing.IssueDraft) {
			d.ClientID = uuid.NewString()
		}, "client_id"},
	} {
		t.Run(name, func(t *testing.T) {
			draft := validDraft()
			tc.mutate(&draft)

			_, err := billing.NewIssued(
				billing.VoucherID(uuid.NewString()),
				tenant.ID(uuid.NewString()),
				uuid.NewString(),
				draft,
				now,
			)

			var validation *billing.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want a ValidationError", err)
			}
			if validation.Field != tc.field {
				t.Fatalf("field = %q, want %q", validation.Field, tc.field)
			}
		})
	}
}

func TestNewIssuedRequiresIdentity(t *testing.T) {
	draft := validDraft()
	if _, err := billing.NewIssued("", tenant.ID(uuid.NewString()), uuid.NewString(), draft, now); err == nil {
		t.Fatal("an empty voucher id must be rejected")
	}
	if _, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "", uuid.NewString(), draft, now); err == nil {
		t.Fatal("an empty tenant id must be rejected")
	}
	if _, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), tenant.ID(uuid.NewString()), "  ", draft, now); err == nil {
		t.Fatal("an empty issuer must be rejected")
	}
}

func TestTheVoucherRemembersItsClient(t *testing.T) {
	name := "Cliente Uno S.A."
	draft := validDraft()
	draft.PatientID = uuid.NewString()
	draft.ClientID = uuid.NewString()
	draft.BuyerName = &name
	draft.Photo = billing.Photo{Coverage: billing.CoveragePrivate}

	voucher := newIssued(t, draft)
	if voucher.ClientID != draft.ClientID {
		t.Fatalf("ClientID = %q, want the payer stamped at issue", voucher.ClientID)
	}

	voided, err := voucher.Void(uuid.NewString(), "Error de carga", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if voided.ClientID != draft.ClientID {
		t.Error("Void rewrote the client snapshot")
	}
}

func TestNewIssuedWithPatientSnapshot(t *testing.T) {
	name := "  Ana Pérez  "
	tax := "  20123456789  "
	draft := validDraft()
	draft.PatientID = uuid.NewString()
	draft.ClientID = uuid.NewString()
	draft.BuyerName = &name
	draft.BuyerTaxID = &tax
	draft.Photo = billing.Photo{Coverage: billing.CoveragePrivate}

	voucher := newIssued(t, draft)
	if voucher.BuyerName == nil || *voucher.BuyerName != "Ana Pérez" {
		t.Errorf("BuyerName = %v, want it trimmed", voucher.BuyerName)
	}
	if voucher.BuyerTaxID == nil || *voucher.BuyerTaxID != "20123456789" {
		t.Errorf("BuyerTaxID = %v, want it trimmed", voucher.BuyerTaxID)
	}
}

func TestAssignNumber(t *testing.T) {
	voucher := newIssued(t, validDraft())

	numbered, err := voucher.AssignNumber(1)
	if err != nil {
		t.Fatalf("AssignNumber: %v", err)
	}
	if numbered.Number != 1 {
		t.Fatalf("Number = %d, want 1", numbered.Number)
	}

	if _, err := numbered.AssignNumber(2); err == nil {
		t.Fatal("AssignNumber twice must be rejected")
	}
	if _, err := voucher.AssignNumber(0); err == nil {
		t.Fatal("a non-positive number must be rejected")
	}
}

func TestVoid(t *testing.T) {
	voucher := newIssued(t, validDraft())
	numbered, err := voucher.AssignNumber(4)
	if err != nil {
		t.Fatalf("AssignNumber: %v", err)
	}

	voided, err := numbered.Void(uuid.NewString(), "  Error de carga  ", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Void: %v", err)
	}
	if voided.Status != billing.StatusVoided {
		t.Fatalf("Status = %q, want voided", voided.Status)
	}
	if voided.Number != 4 {
		t.Errorf("Number = %d, want 4: voiding must not reuse or drop the number", voided.Number)
	}
	if voided.VoidReason == nil || *voided.VoidReason != "Error de carga" {
		t.Errorf("VoidReason = %v, want it trimmed", voided.VoidReason)
	}
	if voided.PracticeName != numbered.PracticeName {
		t.Error("Void rewrote the practice snapshot")
	}

	if _, err := voided.Void(uuid.NewString(), "otra vez", now.Add(2*time.Minute)); !errors.Is(err, billing.ErrAlreadyVoided) {
		t.Fatalf("second void: error = %v, want %v", err, billing.ErrAlreadyVoided)
	}
}

func TestVoidValidation(t *testing.T) {
	voucher := newIssued(t, validDraft())

	if _, err := voucher.Void("  ", "motivo", now); err == nil {
		t.Fatal("an empty voider must be rejected")
	}
	if _, err := voucher.Void(uuid.NewString(), "   ", now); err == nil {
		t.Fatal("a blank reason must be rejected")
	}
	if _, err := voucher.Void(uuid.NewString(), "motivo", now.Add(-time.Hour)); err == nil {
		t.Fatal("voiding before issued_at must be rejected")
	}
}

func TestReconstitute(t *testing.T) {
	id := billing.VoucherID(uuid.NewString())
	tenantID := tenant.ID(uuid.NewString())

	client := "3e5f6a7b-0000-4000-8000-000000000003"
	got, err := billing.Reconstitute(
		id, tenantID, 3, "", client, "", "Sesión", 1000, "ARS", "Norte",
		nil, nil, nil, billing.Photo{}, billing.StatusIssued, uuid.NewString(), now,
		"", nil, nil, now, now,
	)
	if err != nil {
		t.Fatalf("Reconstitute: %v", err)
	}
	if got.ID != id || got.Number != 3 || got.Concept != "Sesión" {
		t.Fatalf("got = %+v, want the stored values", got)
	}
	if got.ClientID != client {
		t.Fatalf("ClientID = %q, want %q", got.ClientID, client)
	}
	if _, err := billing.Reconstitute("", tenantID, 1, "", "", "", "x", 1, "ARS", "N", nil, nil, nil, billing.Photo{}, billing.StatusIssued, "", now, "", nil, nil, now, now); err == nil {
		t.Fatal("Reconstitute without an id must fail")
	}
}
