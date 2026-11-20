package billing_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

func patientDraft(photo billing.Photo) billing.IssueDraft {
	name := "Ana Perez"
	draft := validDraft()
	draft.PatientID = uuid.NewString()
	draft.BuyerName = &name
	draft.Photo = photo
	return draft
}

func expectValidation(t *testing.T, err error, field string) {
	t.Helper()
	var validation *billing.ValidationError
	if !errors.As(err, &validation) || validation.Field != field {
		t.Fatalf("error = %v, want a validation error on %s", err, field)
	}
}

func TestAVoucherForAPatientCarriesItsCoverage(t *testing.T) {
	if _, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u", patientDraft(billing.Photo{}), now); err == nil {
		t.Fatal("a voucher for a patient without a coverage must be refused")
	} else {
		expectValidation(t, err, "coverage")
	}

	voucher := newIssued(t, patientDraft(billing.Photo{Coverage: billing.CoveragePrivate}))
	if voucher.Photo.Coverage != billing.CoveragePrivate {
		t.Fatalf("Coverage = %q, want private", voucher.Photo.Coverage)
	}
}

func TestAnInsurersVoucherNamesTheInsurer(t *testing.T) {
	_, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u",
		patientDraft(billing.Photo{Coverage: billing.CoverageInsurer}), now)
	expectValidation(t, err, "insurer_id")

	_, err = billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u",
		patientDraft(billing.Photo{Coverage: billing.CoverageInsurer, InsurerID: uuid.NewString(), InsurerName: "  "}), now)
	expectValidation(t, err, "insurer_name")

	voucher := newIssued(t, patientDraft(billing.Photo{
		Coverage: billing.CoverageInsurer, InsurerID: uuid.NewString(), InsurerName: " OSDE ",
	}))
	if voucher.Photo.InsurerName != "OSDE" {
		t.Errorf("InsurerName = %q, want it trimmed", voucher.Photo.InsurerName)
	}
}

func TestAParticularVoucherNamesNoInsurer(t *testing.T) {
	_, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u",
		patientDraft(billing.Photo{Coverage: billing.CoveragePrivate, InsurerID: uuid.NewString(), InsurerName: "OSDE"}), now)
	expectValidation(t, err, "insurer_id")
}

func TestALooseChargeHasNoCoverage(t *testing.T) {
	draft := validDraft()
	draft.Photo = billing.Photo{Coverage: billing.CoveragePrivate}
	_, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u", draft, now)
	expectValidation(t, err, "coverage")
}

func TestASessionCarriesModalityAndProfessional(t *testing.T) {
	draft := patientDraft(billing.Photo{Coverage: billing.CoveragePrivate})
	draft.AppointmentID = uuid.NewString()
	_, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u", draft, now)
	expectValidation(t, err, "modality")

	draft.Photo.Modality = billing.ModalityOnline
	_, err = billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u", draft, now)
	expectValidation(t, err, "professional_user_id")

	draft.Photo.ProfessionalID = uuid.NewString()
	draft.Photo.ProfessionalName = "Demo Profesional"
	voucher := newIssued(t, draft)
	if voucher.Photo.Modality != billing.ModalityOnline || voucher.Photo.ProfessionalName != "Demo Profesional" {
		t.Fatalf("photo = %+v, want the session's modality and professional", voucher.Photo)
	}
}

func TestAProfessionalNeedsItsName(t *testing.T) {
	draft := validDraft()
	draft.Photo = billing.Photo{ProfessionalID: uuid.NewString()}
	_, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u", draft, now)
	expectValidation(t, err, "professional_user_id")
}

func TestAModalityIsOnlineOrPresencial(t *testing.T) {
	draft := patientDraft(billing.Photo{Coverage: billing.CoveragePrivate, Modality: "hibrido"})
	_, err := billing.NewIssued(billing.VoucherID(uuid.NewString()), "t", "u", draft, now)
	expectValidation(t, err, "modality")
}
