package billingapp

import (
	"context"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// IssueInput asks to issue one voucher. There is no buyer, no practice name and no photo on
// the way in: the server takes the buyer and the practice name from the patient and the
// tenant, and the photo from the patient, the session and the professional.
//
// A voucher for a session names the session and takes its patient, modality and professional
// from it; PatientID is then optional and must agree with the session. A loose charge names
// no session: it may name a patient, and a professional who is optional.
type IssueInput struct {
	TenantID           string
	ActorID            string
	PatientID          string
	AppointmentID      string
	ProfessionalUserID string
	Concept            string
	AmountMinor        int64
	Currency           string
}

// IssueUseCase issues a voucher with the next number of the practice.
type IssueUseCase struct {
	billing port.BillingRepository
	tenants port.TenantRepository
	scope   port.TenantScope
	audit   port.AuditLog
	clock   port.Clock
	ids     port.IDGenerator
}

// NewIssueUseCase wires the use case.
func NewIssueUseCase(
	billing port.BillingRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
) *IssueUseCase {
	return &IssueUseCase{billing: billing, tenants: tenants, scope: scope, audit: audit, clock: clock, ids: ids}
}

// Execute issues the voucher. The patient, when there is one, must be visible in the
// practice: otherwise the answer is billing.ErrPatientNotVisible, the same as for a
// patient that does not exist.
func (uc *IssueUseCase) Execute(ctx context.Context, in IssueInput) (VoucherView, error) {
	practice, err := uc.tenants.ByID(ctx, tenant.ID(in.TenantID))
	if err != nil {
		return VoucherView{}, err
	}

	draft := billing.IssueDraft{
		AppointmentID: in.AppointmentID,
		Concept:       in.Concept,
		AmountMinor:   in.AmountMinor,
		Currency:      in.Currency,
		PracticeName:  practice.Name,
	}

	var view VoucherView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		patientID := in.PatientID
		var photo billing.Photo

		if in.AppointmentID != "" {
			session, err := uc.billing.Appointment(ctx, in.AppointmentID)
			if err != nil {
				return err
			}
			if err := session.CheckBillable(in.PatientID); err != nil {
				return err
			}
			patientID = session.PatientID
			photo.Modality = session.Modality
			photo.ProfessionalID = session.ProfessionalID
			photo.ProfessionalName = session.ProfessionalName
		} else if in.ProfessionalUserID != "" {
			professional, err := uc.billing.Professional(ctx, in.ProfessionalUserID)
			if err != nil {
				return err
			}
			photo.ProfessionalID = professional.ID
			photo.ProfessionalName = professional.FullName
		}

		if patientID != "" {
			subject, err := uc.billing.Subject(ctx, patientID)
			if err != nil {
				return err
			}
			draft = subject.ApplyBuyer(draft)

			terms, err := uc.billing.PatientBilling(ctx, patientID)
			if err != nil {
				return err
			}
			// A patient's vouchers are issued in the currency the patient is billed in. The
			// currency of a loose charge is free; the patient's is not.
			if strings.ToUpper(strings.TrimSpace(in.Currency)) != terms.Currency {
				return &billing.ValidationError{Field: "currency", Message: "must be the billing currency of the patient"}
			}
			photo.Coverage = terms.Coverage
			if terms.Coverage == billing.CoverageInsurer {
				photo.InsurerID = terms.InsurerID
				photo.InsurerName = terms.InsurerName
			}
		}
		draft.Photo = photo

		voucher, err := billing.NewIssued(
			billing.VoucherID(uc.ids.NewID()),
			tenant.ID(in.TenantID),
			in.ActorID,
			draft,
			uc.clock.Now(),
		)
		if err != nil {
			return err
		}

		saved, err := uc.billing.Issue(ctx, voucher)
		if err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "billing.issue",
			Entity:   "voucher",
			EntityID: string(saved.ID),
			Metadata: map[string]any{"number": saved.Number, "amountMinor": saved.AmountMinor},
		}); err != nil {
			return err
		}
		view = toVoucherView(saved)
		return nil
	})
	if err != nil {
		return VoucherView{}, err
	}
	return view, nil
}

// VoidInput asks to void one issued voucher. The reason is required: a void is a
// correction, and a correction without a reason is not one.
type VoidInput struct {
	TenantID  string
	ActorID   string
	VoucherID string
	Reason    string
}

// VoidUseCase voids an issued voucher. The number stays with the voucher.
type VoidUseCase struct {
	billing port.BillingRepository
	scope   port.TenantScope
	audit   port.AuditLog
	clock   port.Clock
}

// NewVoidUseCase wires the use case.
func NewVoidUseCase(billing port.BillingRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock) *VoidUseCase {
	return &VoidUseCase{billing: billing, scope: scope, audit: audit, clock: clock}
}

// Execute voids the voucher. A voucher already voided answers billing.ErrAlreadyVoided.
func (uc *VoidUseCase) Execute(ctx context.Context, in VoidInput) (VoucherView, error) {
	var view VoucherView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.billing.ByID(ctx, billing.VoucherID(in.VoucherID))
		if err != nil {
			return err
		}
		voided, err := current.Void(in.ActorID, in.Reason, uc.clock.Now())
		if err != nil {
			return err
		}
		saved, err := uc.billing.Void(ctx, voided)
		if err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "billing.void",
			Entity:   "voucher",
			EntityID: string(saved.ID),
			Metadata: map[string]any{"number": saved.Number, "reason": in.Reason},
		}); err != nil {
			return err
		}
		view = toVoucherView(saved)
		return nil
	})
	if err != nil {
		return VoucherView{}, err
	}
	return view, nil
}

// GetUseCase reads one voucher.
type GetUseCase struct {
	billing port.BillingRepository
	scope   port.TenantScope
}

// NewGetUseCase wires the use case.
func NewGetUseCase(billing port.BillingRepository, scope port.TenantScope) *GetUseCase {
	return &GetUseCase{billing: billing, scope: scope}
}

// Execute returns the voucher, or billing.ErrNotFound.
func (uc *GetUseCase) Execute(ctx context.Context, tenantID, voucherID string) (VoucherView, error) {
	var view VoucherView
	err := uc.scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		voucher, err := uc.billing.ByID(ctx, billing.VoucherID(voucherID))
		if err != nil {
			return err
		}
		view = toVoucherView(voucher)
		return nil
	})
	if err != nil {
		return VoucherView{}, err
	}
	return view, nil
}

// ListInput is one page of the history, optionally narrowed by date, patient and status.
type ListInput struct {
	TenantID  string
	From      time.Time
	To        time.Time
	PatientID string
	Status    string
	Limit     int32
	Offset    int32
}

// ListUseCase reads the history of the practice, newest first.
type ListUseCase struct {
	billing port.BillingRepository
	scope   port.TenantScope
}

// NewListUseCase wires the use case.
func NewListUseCase(billing port.BillingRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{billing: billing, scope: scope}
}

// Execute validates the page and the filters, then reads the history.
func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) ([]VoucherView, error) {
	limit, offset, err := pageOf(in.Limit, in.Offset)
	if err != nil {
		return nil, err
	}
	if in.Status != "" && in.Status != string(billing.StatusIssued) && in.Status != string(billing.StatusVoided) {
		return nil, &billing.ValidationError{Field: "status", Message: "must be issued or voided"}
	}

	var views []VoucherView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		vouchers, err := uc.billing.List(ctx, port.VoucherFilter{
			From:      in.From,
			To:        in.To,
			PatientID: in.PatientID,
			Status:    in.Status,
			Limit:     limit,
			Offset:    offset,
		})
		if err != nil {
			return err
		}
		views = make([]VoucherView, 0, len(vouchers))
		for _, voucher := range vouchers {
			views = append(views, toVoucherView(voucher))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// SearchSubjectsInput asks for the buyers matching a fragment of a name or document.
type SearchSubjectsInput struct {
	TenantID string
	Fragment string
	Limit    int32
}

// SearchSubjectsUseCase finds billing subjects. It refuses an empty or very short
// fragment: the answer is a search, not the practice's roster.
type SearchSubjectsUseCase struct {
	billing port.BillingRepository
	scope   port.TenantScope
}

// NewSearchSubjectsUseCase wires the use case.
func NewSearchSubjectsUseCase(billing port.BillingRepository, scope port.TenantScope) *SearchSubjectsUseCase {
	return &SearchSubjectsUseCase{billing: billing, scope: scope}
}

// Execute returns at most one page of matches.
func (uc *SearchSubjectsUseCase) Execute(ctx context.Context, in SearchSubjectsInput) ([]SubjectView, error) {
	fragment := strings.TrimSpace(in.Fragment)
	if len([]rune(fragment)) < minSubjectFragment {
		return nil, &billing.ValidationError{Field: "search", Message: "must have at least 3 characters"}
	}
	limit := in.Limit
	if limit == 0 {
		limit = DefaultSubjectPageSize
	}
	if limit < 1 || limit > MaxSubjectPageSize {
		return nil, &billing.ValidationError{Field: "limit", Message: "must be between 1 and 20"}
	}

	var views []SubjectView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subjects, err := uc.billing.SearchSubjects(ctx, fragment, limit)
		if err != nil {
			return err
		}
		views = make([]SubjectView, 0, len(subjects))
		for _, subject := range subjects {
			views = append(views, toSubjectView(subject))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// pageOf validates the history page. It is checked here, not forwarded: a negative
// limit would reach the driver as a raw SQLSTATE.
func pageOf(limit, offset int32) (int32, int32, error) {
	if limit == 0 {
		limit = DefaultVoucherPageSize
	}
	if limit < 1 || limit > MaxVoucherPageSize {
		return 0, 0, &billing.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	if offset < 0 {
		return 0, 0, &billing.ValidationError{Field: "offset", Message: "must not be negative"}
	}
	return limit, offset, nil
}
