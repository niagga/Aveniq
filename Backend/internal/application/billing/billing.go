// Package billingapp holds the internal voucher use cases: issue, void, read the
// history and find the subjects a voucher can be issued to.
//
// Three decisions live here rather than in the handler. The buyer of a voucher comes
// from the patient record, so the request cannot name who pays. The practice name is
// read from the tenant, so the letterhead is never typed by the caller. Issuing and
// voiding leave an audit entry in the same transaction as the change.
package billingapp

import (
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

// PageSize defaults and bounds for the voucher history.
const (
	DefaultVoucherPageSize int32 = 20
	MaxVoucherPageSize     int32 = 100
)

// SubjectPageSize bounds the subject search. It is not a roster: the search answers a
// handful of matches for the name being typed, never the whole practice.
const (
	DefaultSubjectPageSize int32 = 20
	MaxSubjectPageSize     int32 = 20
	minSubjectFragment           = 3
)

// VoucherView is one voucher as the API hands it out.
type VoucherView struct {
	ID                   string
	Number               int
	Status               string
	Concept              string
	AmountMinor          int64
	Currency             string
	PatientID            string
	AppointmentID        string
	PracticeName         string
	BuyerName            *string
	BuyerTaxID           *string
	BuyerFiscalCondition *string
	IssuedBy             string
	IssuedAt             string
	VoidedBy             string
	VoidedAt             *string
	VoidReason           *string
	// Photo is nil-fielded for a voucher issued before the photo existed (0020) and for the
	// parts of the photo that do not apply to it.
	Photo PhotoView
}

// PhotoView is the coverage, modality and professional a voucher was issued with. A nil field
// is not recorded; a voucher without a patient has no coverage.
type PhotoView struct {
	Coverage         *string
	InsurerID        *string
	InsurerName      *string
	Modality         *string
	ProfessionalID   *string
	ProfessionalName *string
}

// SubjectView is a billing subject: a patient seen as a buyer.
type SubjectView struct {
	PatientID       string
	FullName        string
	DocumentNumber  *string
	TaxID           *string
	FiscalCondition *string
	BillingCurrency string
}

// AppointmentView is one session of a patient, as the emission screen reads it. Proposal is
// the amount the tariff proposes for it, nil when the practice has no tariff that prices it.
type AppointmentView struct {
	ID               string
	PatientID        string
	ProfessionalID   string
	ProfessionalName string
	Modality         string
	StartsAt         string
	Status           string
	BilledVoucherID  *string
	Proposal         *ProposalView
}

// ProposalView is what the tariff proposes for a session: the total of the tariff (the full fee
// for a particular patient, the mutual's share plus the copago for a mutual).
type ProposalView struct {
	AmountMinor int64
	Currency    string
}

// ProfessionalView is an active professional, the only role a voucher may name.
type ProfessionalView struct {
	ID       string
	FullName string
}

func toVoucherView(voucher billing.Voucher) VoucherView {
	return VoucherView{
		ID:                   string(voucher.ID),
		Number:               voucher.Number,
		Status:               string(voucher.Status),
		Concept:              voucher.Concept,
		AmountMinor:          voucher.AmountMinor,
		Currency:             voucher.Currency,
		PatientID:            voucher.PatientID,
		AppointmentID:        voucher.AppointmentID,
		PracticeName:         voucher.PracticeName,
		BuyerName:            voucher.BuyerName,
		BuyerTaxID:           voucher.BuyerTaxID,
		BuyerFiscalCondition: voucher.BuyerFiscalCondition,
		IssuedBy:             voucher.IssuedBy,
		IssuedAt:             voucher.IssuedAt.UTC().Format(time.RFC3339),
		VoidedBy:             voucher.VoidedBy,
		VoidedAt:             formatOptional(voucher.VoidedAt),
		VoidReason:           voucher.VoidReason,
		Photo:                toPhotoView(voucher.Photo),
	}
}

func toPhotoView(photo billing.Photo) PhotoView {
	return PhotoView{
		Coverage:         textPointer(string(photo.Coverage)),
		InsurerID:        textPointer(photo.InsurerID),
		InsurerName:      textPointer(photo.InsurerName),
		Modality:         textPointer(photo.Modality),
		ProfessionalID:   textPointer(photo.ProfessionalID),
		ProfessionalName: textPointer(photo.ProfessionalName),
	}
}

func toAppointmentView(session billing.Appointment, proposal *ProposalView) AppointmentView {
	return AppointmentView{
		ID:               session.ID,
		PatientID:        session.PatientID,
		ProfessionalID:   session.ProfessionalID,
		ProfessionalName: session.ProfessionalName,
		Modality:         session.Modality,
		StartsAt:         session.StartsAt.UTC().Format(time.RFC3339),
		Status:           session.Status,
		BilledVoucherID:  textPointer(session.BilledVoucherID),
		Proposal:         proposal,
	}
}

func toProfessionalView(professional billing.Professional) ProfessionalView {
	return ProfessionalView{ID: professional.ID, FullName: professional.FullName}
}

// textPointer maps an empty text to nil, so the API says "not recorded" instead of "".
func textPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func toSubjectView(subject billing.Subject) SubjectView {
	return SubjectView{
		PatientID:       subject.PatientID,
		FullName:        subject.FullName,
		DocumentNumber:  subject.DocumentNumber,
		TaxID:           subject.TaxID,
		FiscalCondition: subject.FiscalCondition,
		BillingCurrency: subject.BillingCurrency,
	}
}

func formatOptional(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.UTC().Format(time.RFC3339)
	return &text
}
