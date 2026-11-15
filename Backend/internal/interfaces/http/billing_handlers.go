package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	billingapp "github.com/niagga/aveniq/backend/internal/application/billing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// voucherBody is one internal voucher. The buyer and the practice name are a snapshot
// taken at issue: they are read here, never written by the client.
type voucherBody struct {
	ID                   string  `json:"id" format:"uuid"`
	Number               int     `json:"number" doc:"Consecutive per practice. A void keeps it."`
	Status               string  `json:"status" enum:"issued,voided"`
	Concept              string  `json:"concept"`
	AmountMinor          int64   `json:"amountMinor" doc:"Amount in minor units (centavos)."`
	Currency             string  `json:"currency" doc:"ISO 4217 code."`
	PatientID            string  `json:"patientId,omitempty" format:"uuid"`
	AppointmentID        string  `json:"appointmentId,omitempty" format:"uuid"`
	PracticeName         string  `json:"practiceName" doc:"Stamped from the practice at issue."`
	BuyerName            *string `json:"buyerName,omitempty" doc:"Snapshot of the patient at issue."`
	BuyerTaxID           *string `json:"buyerTaxId,omitempty"`
	BuyerFiscalCondition *string `json:"buyerFiscalCondition,omitempty"`
	IssuedBy             string  `json:"issuedBy" format:"uuid"`
	IssuedAt             string  `json:"issuedAt" format:"date-time"`
	VoidedBy             string  `json:"voidedBy,omitempty" format:"uuid"`
	VoidedAt             *string `json:"voidedAt,omitempty" format:"date-time"`
	VoidReason           *string `json:"voidReason,omitempty"`
}

type listVouchersOutput struct {
	Body struct {
		Items []voucherBody `json:"items"`
	}
}

// issueVoucherBody asks for a voucher. There is no buyer and no practice name: the
// server takes both from the patient and the practice.
type issueVoucherBody struct {
	PatientID     string `json:"patientId,omitempty" format:"uuid" doc:"Omit for a loose charge with no patient behind it."`
	AppointmentID string `json:"appointmentId,omitempty" format:"uuid"`
	Concept       string `json:"concept" minLength:"1" maxLength:"200"`
	AmountMinor   int64  `json:"amountMinor" minimum:"1" doc:"Amount in minor units (centavos)."`
	Currency      string `json:"currency" pattern:"^[A-Z]{3}$" doc:"ISO 4217 code, for example ARS."`
}

type voidVoucherBody struct {
	Reason string `json:"reason" minLength:"1" maxLength:"500" doc:"Why the voucher is void. Required."`
}

type subjectBody struct {
	PatientID       string  `json:"patientId" format:"uuid"`
	FullName        string  `json:"fullName"`
	DocumentNumber  *string `json:"documentNumber,omitempty"`
	TaxID           *string `json:"taxId,omitempty"`
	FiscalCondition *string `json:"fiscalCondition,omitempty"`
}

type listSubjectsOutput struct {
	Body struct {
		Items []subjectBody `json:"items"`
	}
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type voucherIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Voucher identifier."`
}

type listVouchersInput struct {
	From      string `query:"from" format:"date-time" doc:"Only vouchers issued at or after this instant."`
	To        string `query:"to" format:"date-time" doc:"Only vouchers issued before this instant."`
	PatientID string `query:"patientId" format:"uuid" doc:"Only vouchers of this patient."`
	Status    string `query:"status" enum:"issued,voided" doc:"Only vouchers in this state."`
	Limit     int32  `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset    int32  `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type issueVoucherInput struct {
	Body issueVoucherBody
}

type voidVoucherInput struct {
	ID   string `path:"id" format:"uuid" doc:"Voucher identifier."`
	Body voidVoucherBody
}

type voucherOutput struct {
	Body voucherBody
}

type searchSubjectsInput struct {
	Search string `query:"search" doc:"A fragment of the name, document or tax id. At least 3 characters."`
	Limit  int32  `query:"limit" minimum:"1" maximum:"20" default:"20" doc:"Maximum matches returned."`
}

// ─── handlers ────────────────────────────────────────────────────────────────

type billingHandlers struct {
	issue    *billingapp.IssueUseCase
	void     *billingapp.VoidUseCase
	get      *billingapp.GetUseCase
	list     *billingapp.ListUseCase
	subjects *billingapp.SearchSubjectsUseCase
	logger   *slog.Logger
}

// registerBillingRoutes declares the internal voucher endpoints. The HTTP guard is the
// billing permission of each verb: the accountant issues and voids, the owner and the
// admin read, and the clinical roles are refused before a handler runs.
func registerBillingRoutes(api huma.API, deps Deps) {
	handlers := &billingHandlers{
		issue:    deps.IssueVoucher,
		void:     deps.VoidVoucher,
		get:      deps.GetVoucher,
		list:     deps.ListVouchers,
		subjects: deps.SearchSubjects,
		logger:   deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingRead)
	issueGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingIssue)
	voidGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingVoid)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-vouchers",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/vouchers",
		Summary:       "List the vouchers of the practice",
		Description:   "The history, newest first. Filters by date window, patient and state.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listVouchersHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "get-voucher",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/vouchers/{id}",
		Summary:       "Read one voucher",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.getVoucherHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "issue-voucher",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/billing/vouchers",
		Summary:       "Issue an internal voucher",
		Description:   "Takes the next number of the practice. The buyer is the patient's snapshot and the practice name is the tenant's; the request cannot set either. Recorded in the audit trail.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   issueGuard,
	}, handlers.issueVoucherHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "void-voucher",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/billing/vouchers/{id}/void",
		Summary:       "Void an issued voucher",
		Description:   "A void is final and keeps the number. A voucher is never deleted. Recorded in the audit trail.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   voidGuard,
	}, handlers.voidVoucherHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "search-billing-subjects",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/subjects",
		Summary:       "Search the patients a voucher can be issued to",
		Description:   "A search, not the roster: at least 3 characters, at most 20 matches. Only the fields a receipt prints.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.searchSubjectsHandler)
}

func (h *billingHandlers) listVouchersHandler(ctx context.Context, in *listVouchersInput) (*listVouchersOutput, error) {
	who := identityOf(ctx)
	from, err := parseOptionalInstant(in.From, "from")
	if err != nil {
		return nil, err
	}
	to, err := parseOptionalInstant(in.To, "to")
	if err != nil {
		return nil, err
	}

	views, err := h.list.Execute(ctx, billingapp.ListInput{
		TenantID:  who.TenantID,
		From:      from,
		To:        to,
		PatientID: in.PatientID,
		Status:    in.Status,
		Limit:     in.Limit,
		Offset:    in.Offset,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listVouchersOutput{}
	out.Body.Items = make([]voucherBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toVoucherBody(view))
	}
	return out, nil
}

func (h *billingHandlers) getVoucherHandler(ctx context.Context, in *voucherIDInput) (*voucherOutput, error) {
	who := identityOf(ctx)
	view, err := h.get.Execute(ctx, who.TenantID, in.ID)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &voucherOutput{Body: toVoucherBody(view)}, nil
}

func (h *billingHandlers) issueVoucherHandler(ctx context.Context, in *issueVoucherInput) (*voucherOutput, error) {
	who := identityOf(ctx)
	view, err := h.issue.Execute(ctx, billingapp.IssueInput{
		TenantID:      who.TenantID,
		ActorID:       who.UserID,
		PatientID:     in.Body.PatientID,
		AppointmentID: in.Body.AppointmentID,
		Concept:       in.Body.Concept,
		AmountMinor:   in.Body.AmountMinor,
		Currency:      in.Body.Currency,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &voucherOutput{Body: toVoucherBody(view)}, nil
}

func (h *billingHandlers) voidVoucherHandler(ctx context.Context, in *voidVoucherInput) (*voucherOutput, error) {
	who := identityOf(ctx)
	view, err := h.void.Execute(ctx, billingapp.VoidInput{
		TenantID:  who.TenantID,
		ActorID:   who.UserID,
		VoucherID: in.ID,
		Reason:    in.Body.Reason,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &voucherOutput{Body: toVoucherBody(view)}, nil
}

func (h *billingHandlers) searchSubjectsHandler(ctx context.Context, in *searchSubjectsInput) (*listSubjectsOutput, error) {
	who := identityOf(ctx)
	views, err := h.subjects.Execute(ctx, billingapp.SearchSubjectsInput{
		TenantID: who.TenantID,
		Fragment: in.Search,
		Limit:    in.Limit,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listSubjectsOutput{}
	out.Body.Items = make([]subjectBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, subjectBody{
			PatientID:       view.PatientID,
			FullName:        view.FullName,
			DocumentNumber:  view.DocumentNumber,
			TaxID:           view.TaxID,
			FiscalCondition: view.FiscalCondition,
		})
	}
	return out, nil
}

// parseOptionalInstant reads an optional RFC 3339 filter. An absent value is the zero
// time, which the repository reads as unbounded.
func parseOptionalInstant(raw, field string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity(field + " must be an RFC 3339 timestamp")
	}
	return parsed, nil
}

func toVoucherBody(view billingapp.VoucherView) voucherBody {
	return voucherBody{
		ID:                   view.ID,
		Number:               view.Number,
		Status:               view.Status,
		Concept:              view.Concept,
		AmountMinor:          view.AmountMinor,
		Currency:             view.Currency,
		PatientID:            view.PatientID,
		AppointmentID:        view.AppointmentID,
		PracticeName:         view.PracticeName,
		BuyerName:            view.BuyerName,
		BuyerTaxID:           view.BuyerTaxID,
		BuyerFiscalCondition: view.BuyerFiscalCondition,
		IssuedBy:             view.IssuedBy,
		IssuedAt:             view.IssuedAt,
		VoidedBy:             view.VoidedBy,
		VoidedAt:             view.VoidedAt,
		VoidReason:           view.VoidReason,
	}
}
