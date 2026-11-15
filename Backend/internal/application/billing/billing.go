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
}

// SubjectView is a billing subject: a patient seen as a buyer.
type SubjectView struct {
	PatientID       string
	FullName        string
	DocumentNumber  *string
	TaxID           *string
	FiscalCondition *string
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
	}
}

func toSubjectView(subject billing.Subject) SubjectView {
	return SubjectView{
		PatientID:       subject.PatientID,
		FullName:        subject.FullName,
		DocumentNumber:  subject.DocumentNumber,
		TaxID:           subject.TaxID,
		FiscalCondition: subject.FiscalCondition,
	}
}

func formatOptional(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.UTC().Format(time.RFC3339)
	return &text
}
