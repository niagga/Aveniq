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
	ID                   string    `json:"id" format:"uuid"`
	Number               int       `json:"number" doc:"Consecutive per practice. A void keeps it."`
	Status               string    `json:"status" enum:"issued,voided"`
	Concept              string    `json:"concept"`
	AmountMinor          int64     `json:"amountMinor" doc:"Amount in minor units (centavos)."`
	Currency             string    `json:"currency" doc:"ISO 4217 code."`
	PatientID            string    `json:"patientId,omitempty" format:"uuid"`
	AppointmentID        string    `json:"appointmentId,omitempty" format:"uuid"`
	PracticeName         string    `json:"practiceName" doc:"Stamped from the practice at issue."`
	BuyerName            *string   `json:"buyerName,omitempty" doc:"Snapshot of the patient at issue."`
	BuyerTaxID           *string   `json:"buyerTaxId,omitempty"`
	BuyerFiscalCondition *string   `json:"buyerFiscalCondition,omitempty"`
	Photo                photoBody `json:"photo" doc:"How the patient was covered, the modality of the session and the professional who gave it, as they were at issue. A voucher issued before the photo existed has them all null."`
	IssuedBy             string    `json:"issuedBy" format:"uuid"`
	IssuedAt             string    `json:"issuedAt" format:"date-time"`
	VoidedBy             string    `json:"voidedBy,omitempty" format:"uuid"`
	VoidedAt             *string   `json:"voidedAt,omitempty" format:"date-time"`
	VoidReason           *string   `json:"voidReason,omitempty"`
}

// photoBody is the photo of a voucher. A field the voucher does not carry is null.
type photoBody struct {
	Coverage         *string `json:"coverage" enum:"private,insurer" doc:"private: a particular patient. insurer: covered by a mutual."`
	InsurerID        *string `json:"insurerId" format:"uuid"`
	InsurerName      *string `json:"insurerName"`
	Modality         *string `json:"modality" enum:"online,presencial"`
	ProfessionalID   *string `json:"professionalId" format:"uuid"`
	ProfessionalName *string `json:"professionalName"`
}

// unbilledSessionBody is one row of the worklist: an attended session that no voucher bills.
type unbilledSessionBody struct {
	ID               string `json:"id" format:"uuid"`
	PatientID        string `json:"patientId" format:"uuid"`
	PatientName      string `json:"patientName"`
	ProfessionalID   string `json:"professionalId" format:"uuid"`
	ProfessionalName string `json:"professionalName"`
	Modality         string `json:"modality" enum:"online,presencial"`
	StartsAt         string `json:"startsAt" format:"date-time"`
}

type listVouchersOutput struct {
	Body struct {
		Items []voucherBody `json:"items"`
	}
}

// issueVoucherBody asks for a voucher. There is no buyer, no practice name and no photo:
// the server takes the buyer and the practice name from the patient and the practice, and
// the photo from the patient, the session and the professional.
//
// A voucher for a session names the appointment, and its patient is the session's. A loose
// charge names no appointment; it may name a patient, and a professional who is optional.
type issueVoucherBody struct {
	PatientID          string `json:"patientId,omitempty" format:"uuid" doc:"Omit for a loose charge with no patient behind it. A voucher for an appointment takes the session's patient."`
	AppointmentID      string `json:"appointmentId,omitempty" format:"uuid" doc:"The session this voucher bills. A session is billed once: an issued voucher that already bills it answers 409."`
	ProfessionalUserID string `json:"professionalUserId,omitempty" format:"uuid" doc:"The professional of a voucher without an appointment. Optional. A voucher for an appointment takes the session's professional and refuses this field."`
	Concept            string `json:"concept" minLength:"1" maxLength:"200"`
	AmountMinor        int64  `json:"amountMinor" minimum:"1" doc:"Amount in minor units (centavos). The tariff proposes it for a session; the emission can change it."`
	Currency           string `json:"currency" pattern:"^[A-Z]{3}$" doc:"ISO 4217 code, for example ARS. For a patient it must be the patient's billing currency."`
}

// patientSessionBody is one session of a patient on the emission screen. Proposal is the amount the
// tariff proposes for it, null when the practice has no tariff that prices it.
type patientSessionBody struct {
	ID               string        `json:"id" format:"uuid"`
	PatientID        string        `json:"patientId" format:"uuid"`
	ProfessionalID   string        `json:"professionalId" format:"uuid"`
	ProfessionalName string        `json:"professionalName"`
	Modality         string        `json:"modality" enum:"online,presencial"`
	StartsAt         string        `json:"startsAt" format:"date-time"`
	Status           string        `json:"status" enum:"scheduled,attended,cancelled"`
	BilledVoucherID  *string       `json:"billedVoucherId" format:"uuid" doc:"The issued voucher that bills the session, null when none does."`
	Proposal         *proposalBody `json:"proposal"`
}

type proposalBody struct {
	AmountMinor int64  `json:"amountMinor" doc:"The tariff's total in minor units: the full fee for a particular patient, the mutual's share plus the copago for a mutual."`
	Currency    string `json:"currency"`
}

type listPatientSessionsOutput struct {
	Body struct {
		Items []patientSessionBody `json:"items"`
	}
}

type professionalBody struct {
	ID       string `json:"id" format:"uuid"`
	FullName string `json:"fullName"`
}

type listProfessionalsOutput struct {
	Body struct {
		Items []professionalBody `json:"items"`
	}
}

type patientSessionsInput struct {
	PatientID string `path:"patientId" format:"uuid" doc:"The patient whose sessions are listed."`
	Limit     int32  `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"The newest sessions returned."`
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
	BillingCurrency string  `json:"billingCurrency" pattern:"^[A-Z]{3}$" doc:"The currency the patient's vouchers are issued in."`
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

type unbilledSessionsInput struct {
	From  string `query:"from" format:"date-time" doc:"Start of the window, inclusive. Required."`
	To    string `query:"to" format:"date-time" doc:"End of the window, exclusive. Required."`
	Limit int32  `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"Page size."`
}

type unbilledSessionsOutput struct {
	Body struct {
		Items []unbilledSessionBody `json:"items"`
	}
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
	issue       *billingapp.IssueUseCase
	void        *billingapp.VoidUseCase
	get         *billingapp.GetUseCase
	list        *billingapp.ListUseCase
	subjects    *billingapp.SearchSubjectsUseCase
	sessions    *billingapp.PatientSessionsUseCase
	unbilled    *billingapp.UnbilledUseCase
	professions *billingapp.ProfessionalsUseCase
	logger      *slog.Logger
}

// registerBillingRoutes declares the internal voucher endpoints. The HTTP guard is the
// billing permission of each verb: the accountant issues and voids, the owner and the
// admin read, and the clinical roles are refused before a handler runs.
func registerBillingRoutes(api huma.API, deps Deps) {
	handlers := &billingHandlers{
		issue:       deps.IssueVoucher,
		void:        deps.VoidVoucher,
		get:         deps.GetVoucher,
		list:        deps.ListVouchers,
		subjects:    deps.SearchSubjects,
		sessions:    deps.PatientSessions,
		unbilled:    deps.UnbilledSessions,
		professions: deps.Professionals,
		logger:      deps.Logger,
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
		OperationID:   "list-unbilled-sessions",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/unbilled",
		Summary:       "List the sessions no voucher bills",
		Description:   "Attended sessions of the practice inside the window that no issued voucher bills, oldest first: the worklist of the billing screen.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.unbilledSessionsHandler)

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

	// The emission screen reads a patient's sessions and the professionals a loose charge can
	// name. Both belong to issuing: the accountant emits, and an owner or admin who only reads
	// the history does not need the patient's schedule.
	huma.Register(api, huma.Operation{
		OperationID:   "list-patient-sessions",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/subjects/{patientId}/sessions",
		Summary:       "List the sessions a voucher can bill for a patient",
		Description:   "The newest sessions of the patient, each with the tariff's proposed amount and the voucher that already bills it, if any.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   issueGuard,
	}, handlers.listSessionsHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "list-billing-professionals",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/professionals",
		Summary:       "List the professionals a voucher can name",
		Description:   "The active professionals of the practice, by name. A loose charge may name one of them.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   issueGuard,
	}, handlers.listProfessionalsHandler)
}

func (h *billingHandlers) listSessionsHandler(ctx context.Context, in *patientSessionsInput) (*listPatientSessionsOutput, error) {
	who := identityOf(ctx)
	views, err := h.sessions.Execute(ctx, billingapp.PatientSessionsInput{
		TenantID:  who.TenantID,
		PatientID: in.PatientID,
		Limit:     in.Limit,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listPatientSessionsOutput{}
	out.Body.Items = make([]patientSessionBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toPatientSessionBody(view))
	}
	return out, nil
}

func (h *billingHandlers) listProfessionalsHandler(ctx context.Context, _ *struct{}) (*listProfessionalsOutput, error) {
	who := identityOf(ctx)
	views, err := h.professions.Execute(ctx, who.TenantID)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listProfessionalsOutput{}
	out.Body.Items = make([]professionalBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, professionalBody{ID: view.ID, FullName: view.FullName})
	}
	return out, nil
}

func (h *billingHandlers) unbilledSessionsHandler(ctx context.Context, in *unbilledSessionsInput) (*unbilledSessionsOutput, error) {
	who := identityOf(ctx)
	from, err := parseOptionalInstant(in.From, "from")
	if err != nil {
		return nil, err
	}
	to, err := parseOptionalInstant(in.To, "to")
	if err != nil {
		return nil, err
	}

	views, err := h.unbilled.Execute(ctx, billingapp.UnbilledInput{
		TenantID: who.TenantID,
		From:     from,
		To:       to,
		Limit:    in.Limit,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &unbilledSessionsOutput{}
	out.Body.Items = make([]unbilledSessionBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, unbilledSessionBody{
			ID:               view.ID,
			PatientID:        view.PatientID,
			PatientName:      view.PatientName,
			ProfessionalID:   view.ProfessionalID,
			ProfessionalName: view.ProfessionalName,
			Modality:         view.Modality,
			StartsAt:         view.StartsAt,
		})
	}
	return out, nil
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
		TenantID:           who.TenantID,
		ActorID:            who.UserID,
		PatientID:          in.Body.PatientID,
		AppointmentID:      in.Body.AppointmentID,
		ProfessionalUserID: in.Body.ProfessionalUserID,
		Concept:            in.Body.Concept,
		AmountMinor:        in.Body.AmountMinor,
		Currency:           in.Body.Currency,
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
			BillingCurrency: view.BillingCurrency,
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
		Photo: photoBody{
			Coverage:         view.Photo.Coverage,
			InsurerID:        view.Photo.InsurerID,
			InsurerName:      view.Photo.InsurerName,
			Modality:         view.Photo.Modality,
			ProfessionalID:   view.Photo.ProfessionalID,
			ProfessionalName: view.Photo.ProfessionalName,
		},
		IssuedBy:   view.IssuedBy,
		IssuedAt:   view.IssuedAt,
		VoidedBy:   view.VoidedBy,
		VoidedAt:   view.VoidedAt,
		VoidReason: view.VoidReason,
	}
}

func toPatientSessionBody(view billingapp.AppointmentView) patientSessionBody {
	body := patientSessionBody{
		ID:               view.ID,
		PatientID:        view.PatientID,
		ProfessionalID:   view.ProfessionalID,
		ProfessionalName: view.ProfessionalName,
		Modality:         view.Modality,
		StartsAt:         view.StartsAt,
		Status:           view.Status,
		BilledVoucherID:  view.BilledVoucherID,
	}
	if view.Proposal != nil {
		body.Proposal = &proposalBody{AmountMinor: view.Proposal.AmountMinor, Currency: view.Proposal.Currency}
	}
	return body
}
