package billing

import (
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// VoucherID is the opaque identity of an internal voucher.
type VoucherID string

// Status is the lifecycle of a voucher. There is no draft: writing it issues it.
type Status string

const (
	StatusIssued Status = "issued"
	StatusVoided Status = "voided"
)

// Voucher is one internal receipt. Numbers are consecutive per practice and never
// reused: a void keeps its number, the same way a clinical correction is a new note
// rather than an edit.
//
// The buyer and the practice name are a snapshot taken at issue. A later edit to
// the patient, the appointment or the client must not rewrite what was issued — Void
// does not touch them.
type Voucher struct {
	ID       VoucherID
	TenantID tenant.ID
	Number   int

	PatientID string
	// ClientID is the payer the buyer was stamped from (S26, T73). It is present exactly
	// when PatientID is: a loose charge has neither.
	ClientID      string
	AppointmentID string

	Concept     string
	AmountMinor int64
	Currency    string

	PracticeName         string
	BuyerName            *string
	BuyerTaxID           *string
	BuyerFiscalCondition *string

	// Photo is the coverage, modality and professional at issue. Like the buyer, it is never
	// rewritten: Void does not touch it.
	Photo Photo

	Status   Status
	IssuedBy string
	IssuedAt time.Time

	VoidedBy   string
	VoidedAt   *time.Time
	VoidReason *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IssueDraft is what a caller may write. The number is not here: the repository
// assigns it inside the issuing transaction so a rolled-back issue leaves no gap.
type IssueDraft struct {
	PatientID string
	// ClientID is the payer of the patient (S26): stamped by the application from the client,
	// never carried by the request. It is present exactly when PatientID is.
	ClientID      string
	AppointmentID string

	Concept     string
	AmountMinor int64
	Currency    string

	PracticeName         string
	BuyerName            *string
	BuyerTaxID           *string
	BuyerFiscalCondition *string

	// Photo is stamped by the application from the patient, the appointment and the
	// professional, never copied from the request.
	Photo Photo
}

// NewIssued validates and builds an issued voucher with no number yet.
//
// PracticeName is stamped by the caller from the tenant, never from the request:
// the accountant must not invent the letterhead (L46).
func NewIssued(id VoucherID, tenantID tenant.ID, issuedBy string, draft IssueDraft, now time.Time) (Voucher, error) {
	if id == "" {
		return Voucher{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Voucher{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if isBlank(issuedBy) {
		return Voucher{}, &ValidationError{Field: "issued_by", Message: "must not be empty"}
	}

	concept := strings.TrimSpace(draft.Concept)
	if concept == "" {
		return Voucher{}, &ValidationError{Field: "concept", Message: "must not be blank"}
	}
	if len([]rune(concept)) > maxConceptLength {
		return Voucher{}, &ValidationError{Field: "concept", Message: "must be at most 200 characters"}
	}
	if draft.AmountMinor <= 0 {
		return Voucher{}, &ValidationError{Field: "amount_minor", Message: "must be greater than zero"}
	}
	currency := strings.TrimSpace(draft.Currency)
	if !isISOCurrency(currency) {
		return Voucher{}, &ValidationError{Field: "currency", Message: "must be a three-letter ISO code"}
	}

	practice := strings.TrimSpace(draft.PracticeName)
	if practice == "" {
		return Voucher{}, &ValidationError{Field: "practice_name", Message: "must not be blank"}
	}

	patientID := strings.TrimSpace(draft.PatientID)
	appointmentID := strings.TrimSpace(draft.AppointmentID)
	// An appointment is one patient's: a voucher that names one must name that patient.
	// The schema checks that the appointment is the patient's; this refuses the half-filled case.
	if appointmentID != "" && patientID == "" {
		return Voucher{}, &ValidationError{Field: "appointment_id", Message: "needs a patient: an appointment belongs to one patient"}
	}
	clientID := strings.TrimSpace(draft.ClientID)
	// The payer belongs to the patient it pays for (S26, T73): exactly when the patient is
	// there, the client is too. The schema mirrors this with two CHECKs in 0027.
	if patientID != "" && clientID == "" {
		return Voucher{}, &ValidationError{Field: "client_id", Message: "a voucher for a patient must name its client"}
	}
	if clientID != "" && patientID == "" {
		return Voucher{}, &ValidationError{Field: "client_id", Message: "a loose charge carries no client"}
	}
	buyerName := sanitize(draft.BuyerName)
	buyerTaxID := sanitize(draft.BuyerTaxID)
	buyerFiscal := sanitize(draft.BuyerFiscalCondition)

	if patientID == "" {
		if buyerName != nil || buyerTaxID != nil || buyerFiscal != nil {
			return Voucher{}, &ValidationError{Field: "buyer_name", Message: "a loose charge carries no buyer snapshot"}
		}
	} else if buyerName == nil {
		return Voucher{}, &ValidationError{Field: "buyer_name", Message: "a voucher for a patient must carry the buyer snapshot"}
	}

	photo := draft.Photo.normalized()
	if err := photo.validate(patientID != "", appointmentID != ""); err != nil {
		return Voucher{}, err
	}

	when := stored(now)
	return Voucher{
		ID:                   id,
		TenantID:             tenantID,
		PatientID:            patientID,
		ClientID:             clientID,
		AppointmentID:        appointmentID,
		Concept:              concept,
		AmountMinor:          draft.AmountMinor,
		Currency:             currency,
		PracticeName:         practice,
		BuyerName:            buyerName,
		BuyerTaxID:           buyerTaxID,
		BuyerFiscalCondition: buyerFiscal,
		Photo:                photo,
		Status:               StatusIssued,
		IssuedBy:             strings.TrimSpace(issuedBy),
		IssuedAt:             when,
		CreatedAt:            when,
		UpdatedAt:            when,
	}, nil
}

// AssignNumber stamps the consecutive number the repository just reserved.
//
// It is the only mutation of Number. Calling it twice, or with a non-positive
// value, is a programmer error: the counter already moved.
func (v Voucher) AssignNumber(n int) (Voucher, error) {
	if v.Number != 0 {
		return Voucher{}, &ValidationError{Field: "number", Message: "already assigned"}
	}
	if n <= 0 {
		return Voucher{}, &ValidationError{Field: "number", Message: "must be greater than zero"}
	}
	v.Number = n
	return v, nil
}

// Void marks an issued voucher as voided. The number stays; the snapshot stays.
func (v Voucher) Void(voidedBy, reason string, now time.Time) (Voucher, error) {
	if v.Status == StatusVoided {
		return Voucher{}, ErrAlreadyVoided
	}
	if v.Status != StatusIssued {
		return Voucher{}, ErrImmutable
	}
	if isBlank(voidedBy) {
		return Voucher{}, &ValidationError{Field: "voided_by", Message: "must not be empty"}
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return Voucher{}, &ValidationError{Field: "void_reason", Message: "must not be blank"}
	}
	when := stored(now)
	if when.Before(v.IssuedAt) {
		return Voucher{}, &ValidationError{Field: "voided_at", Message: "must not be before issued_at"}
	}
	v.Status = StatusVoided
	v.VoidedBy = strings.TrimSpace(voidedBy)
	v.VoidedAt = &when
	v.VoidReason = &trimmed
	v.UpdatedAt = when
	return v, nil
}

// Reconstitute rebuilds a voucher from persisted state.
func Reconstitute(
	id VoucherID,
	tenantID tenant.ID,
	number int,
	patientID, clientID, appointmentID string,
	concept string,
	amountMinor int64,
	currency string,
	practiceName string,
	buyerName, buyerTaxID, buyerFiscalCondition *string,
	photo Photo,
	status Status,
	issuedBy string,
	issuedAt time.Time,
	voidedBy string,
	voidedAt *time.Time,
	voidReason *string,
	createdAt, updatedAt time.Time,
) (Voucher, error) {
	if id == "" {
		return Voucher{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	return Voucher{
		ID:                   id,
		TenantID:             tenantID,
		Number:               number,
		PatientID:            patientID,
		ClientID:             clientID,
		AppointmentID:        appointmentID,
		Concept:              concept,
		AmountMinor:          amountMinor,
		Currency:             currency,
		PracticeName:         practiceName,
		BuyerName:            buyerName,
		BuyerTaxID:           buyerTaxID,
		BuyerFiscalCondition: buyerFiscalCondition,
		Photo:                photo.normalized(),
		Status:               status,
		IssuedBy:             issuedBy,
		IssuedAt:             issuedAt.UTC(),
		VoidedBy:             voidedBy,
		VoidedAt:             utcPtr(voidedAt),
		VoidReason:           voidReason,
		CreatedAt:            createdAt.UTC(),
		UpdatedAt:            updatedAt.UTC(),
	}, nil
}

func utcPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	when := value.UTC()
	return &when
}
