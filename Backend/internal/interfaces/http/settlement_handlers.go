package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	billingapp "github.com/niagga/aveniq/backend/internal/application/billing"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// settlementInsurerBody is a mutual the practice can settle with. Settlable is false until the
// mutual has its CUIT, which the settlement is addressed to.
type settlementInsurerBody struct {
	ID        string  `json:"id" format:"uuid"`
	Name      string  `json:"name"`
	TaxID     *string `json:"taxId,omitempty" doc:"The mutual's CUIT, 11 digits. A settlement needs it."`
	Settlable bool    `json:"settlable"`
}

// settlementItemBody is one session of a settlement. AmountMinor is the mutual's share, valued at
// issue from the tariff in force on the day of the session.
type settlementItemBody struct {
	AppointmentID    string  `json:"appointmentId" format:"uuid"`
	PatientName      string  `json:"patientName"`
	MemberNumber     string  `json:"memberNumber"`
	ProfessionalName string  `json:"professionalName"`
	Modality         string  `json:"modality" enum:"online,presencial"`
	StartsAt         string  `json:"startsAt" format:"date-time"`
	Currency         string  `json:"currency" pattern:"^[A-Z]{3}$"`
	AmountMinor      int64   `json:"amountMinor" doc:"The mutual's share in minor units (centavos)."`
	CopagoMinor      int64   `json:"copagoMinor" doc:"The patient's copago in minor units. Shown on the settlement, never summed into its totals."`
	ReleasedAt       *string `json:"releasedAt,omitempty" format:"date-time" doc:"Set when the settlement is voided: the session can be settled again."`
}

// unvaluedSessionBody is an attended session the tariff in force does not price. It is left out
// of the settlement and listed, so the practice sees what is not yet billable to the mutual.
type unvaluedSessionBody struct {
	AppointmentID string `json:"appointmentId" format:"uuid"`
	PatientName   string `json:"patientName"`
	Currency      string `json:"currency" pattern:"^[A-Z]{3}$"`
	StartsAt      string `json:"startsAt" format:"date-time"`
}

// currencyTotalBody is the sum of a settlement's sessions in one currency. Never converted.
type currencyTotalBody struct {
	Currency    string `json:"currency" pattern:"^[A-Z]{3}$"`
	Sessions    int    `json:"sessions"`
	AmountMinor int64  `json:"amountMinor"`
}

// settlementBody is an issued settlement: the snapshot of the mutual and the practice, its
// sessions, and the totals per currency.
type settlementBody struct {
	ID           string               `json:"id" format:"uuid"`
	InsurerID    string               `json:"insurerId" format:"uuid"`
	InsurerName  string               `json:"insurerName" doc:"Snapshot of the mutual at issue."`
	InsurerTaxID string               `json:"insurerTaxId" doc:"Snapshot of the mutual's CUIT at issue."`
	PracticeName string               `json:"practiceName" doc:"Snapshot of the practice at issue."`
	Period       string               `json:"period" pattern:"^[0-9]{4}-(0[1-9]|1[0-2])$" doc:"The month the sessions were attended in, YYYY-MM."`
	Status       string               `json:"status" enum:"issued,voided"`
	CreatedBy    string               `json:"createdBy" format:"uuid"`
	CreatedAt    string               `json:"createdAt" format:"date-time"`
	VoidedBy     string               `json:"voidedBy,omitempty" format:"uuid"`
	VoidedAt     *string              `json:"voidedAt,omitempty" format:"date-time"`
	VoidReason   *string              `json:"voidReason,omitempty"`
	Items        []settlementItemBody `json:"items"`
	Totals       []currencyTotalBody  `json:"totals"`
}

// settlementPreviewBody is what a settlement would contain, computed the way it is issued.
type settlementPreviewBody struct {
	InsurerID   string                `json:"insurerId" format:"uuid"`
	InsurerName string                `json:"insurerName"`
	Period      string                `json:"period" pattern:"^[0-9]{4}-(0[1-9]|1[0-2])$"`
	Sessions    []settlementItemBody  `json:"sessions"`
	Unvalued    []unvaluedSessionBody `json:"unvalued"`
	Totals      []currencyTotalBody   `json:"totals"`
}

type listSettlementInsurersOutput struct {
	Body struct {
		Items []settlementInsurerBody `json:"items"`
	}
}

type listSettlementsOutput struct {
	Body struct {
		Items []settlementBody `json:"items"`
	}
}

type settlementOutput struct {
	Body settlementBody
}

type settlementPreviewOutput struct {
	Body settlementPreviewBody
}

// ─── inputs ──────────────────────────────────────────────────────────────────

type settlementPreviewInput struct {
	InsurerID string `query:"insurerId" format:"uuid" required:"true" doc:"The mutual to settle with."`
	Period    string `query:"period" pattern:"^[0-9]{4}-(0[1-9]|1[0-2])$" required:"true" doc:"The month, YYYY-MM."`
}

type listSettlementsInput struct {
	InsurerID string `query:"insurerId" format:"uuid" doc:"Only the settlements of this mutual."`
	Limit     int32  `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset    int32  `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type settlementIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Settlement identifier."`
}

type issueSettlementBody struct {
	InsurerID string `json:"insurerId" format:"uuid" doc:"The mutual to settle with. It needs its CUIT."`
	Period    string `json:"period" pattern:"^[0-9]{4}-(0[1-9]|1[0-2])$" doc:"The month of the sessions, YYYY-MM. The server values the sessions; the request carries none."`
}

type issueSettlementInput struct {
	Body issueSettlementBody
}

type voidSettlementBody struct {
	Reason string `json:"reason" minLength:"1" maxLength:"500" doc:"Why the settlement is void. Required."`
}

type voidSettlementInput struct {
	ID   string `path:"id" format:"uuid" doc:"Settlement identifier."`
	Body voidSettlementBody
}

// ─── routes ──────────────────────────────────────────────────────────────────

type settlementHandlers struct {
	insurers *billingapp.SettlementInsurersUseCase
	preview  *billingapp.SettlementPreviewUseCase
	list     *billingapp.ListSettlementsUseCase
	get      *billingapp.GetSettlementUseCase
	issue    *billingapp.IssueSettlementUseCase
	void     *billingapp.VoidSettlementUseCase
	logger   *slog.Logger
}

// registerSettlementRoutes declares the monthly settlements to the mutuales. Reading them is
// billing:read, as for the vouchers; issuing one is billing:issue and voiding one is billing:void,
// the same permissions a voucher takes.
func registerSettlementRoutes(api huma.API, deps Deps) {
	handlers := &settlementHandlers{
		insurers: deps.SettlementInsurers,
		preview:  deps.PreviewSettlement,
		list:     deps.ListSettlements,
		get:      deps.GetSettlement,
		issue:    deps.IssueSettlement,
		void:     deps.VoidSettlement,
		logger:   deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingRead)
	issueGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingIssue)
	voidGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingVoid)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-settlement-insurers",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/settlement-insurers",
		Summary:       "List the mutuales that can be settled",
		Description:   "The active mutuales of the practice, by name. Settlable is false for a mutual without its CUIT.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listInsurersHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "preview-settlement",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/settlements/preview",
		Summary:       "Preview the settlement of a mutual for a month",
		Description:   "What the settlement would contain, valued the way it is issued. Writes nothing. Sessions the tariff does not price are listed as unvalued.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.previewHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "list-settlements",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/settlements",
		Summary:       "List the settlements of the practice",
		Description:   "Newest first, optionally of one mutual. Each one with its sessions and its totals per currency.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "get-settlement",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/billing/settlements/{id}",
		Summary:       "Read one settlement",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.getHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "issue-settlement",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/billing/settlements",
		Summary:       "Issue the settlement of a mutual for a month",
		Description:   "Values the attended sessions of the mutual's patients in the month and writes the settlement with them. A session already in an active settlement answers 409. Recorded in the audit trail.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   issueGuard,
	}, handlers.issueHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "void-settlement",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/billing/settlements/{id}/void",
		Summary:       "Void an issued settlement",
		Description:   "A void is final and releases its sessions, which can be settled again. A settlement is never deleted. Recorded in the audit trail.",
		Tags:          []string{"billing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   voidGuard,
	}, handlers.voidHandler)
}

func (h *settlementHandlers) listInsurersHandler(ctx context.Context, _ *struct{}) (*listSettlementInsurersOutput, error) {
	who := identityOf(ctx)
	views, err := h.insurers.Execute(ctx, who.TenantID)
	if err != nil {
		return nil, settlementProblem(ctx, h.logger, err)
	}
	out := &listSettlementInsurersOutput{}
	out.Body.Items = make([]settlementInsurerBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, settlementInsurerBody{
			ID: view.ID, Name: view.Name, TaxID: view.TaxID, Settlable: view.Settlable,
		})
	}
	return out, nil
}

func (h *settlementHandlers) previewHandler(ctx context.Context, in *settlementPreviewInput) (*settlementPreviewOutput, error) {
	who := identityOf(ctx)
	view, err := h.preview.Execute(ctx, who.TenantID, in.InsurerID, in.Period)
	if err != nil {
		return nil, settlementProblem(ctx, h.logger, err)
	}
	return &settlementPreviewOutput{Body: toSettlementPreviewBody(view)}, nil
}

func (h *settlementHandlers) listHandler(ctx context.Context, in *listSettlementsInput) (*listSettlementsOutput, error) {
	who := identityOf(ctx)
	views, err := h.list.Execute(ctx, billingapp.ListSettlementsInput{
		TenantID:  who.TenantID,
		InsurerID: in.InsurerID,
		Limit:     in.Limit,
		Offset:    in.Offset,
	})
	if err != nil {
		return nil, settlementProblem(ctx, h.logger, err)
	}
	out := &listSettlementsOutput{}
	out.Body.Items = make([]settlementBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toSettlementBody(view))
	}
	return out, nil
}

func (h *settlementHandlers) getHandler(ctx context.Context, in *settlementIDInput) (*settlementOutput, error) {
	who := identityOf(ctx)
	view, err := h.get.Execute(ctx, who.TenantID, in.ID)
	if err != nil {
		return nil, settlementProblem(ctx, h.logger, err)
	}
	return &settlementOutput{Body: toSettlementBody(view)}, nil
}

func (h *settlementHandlers) issueHandler(ctx context.Context, in *issueSettlementInput) (*settlementOutput, error) {
	who := identityOf(ctx)
	view, err := h.issue.Execute(ctx, billingapp.IssueSettlementInput{
		TenantID:  who.TenantID,
		ActorID:   who.UserID,
		InsurerID: in.Body.InsurerID,
		Period:    in.Body.Period,
	})
	if err != nil {
		return nil, settlementProblem(ctx, h.logger, err)
	}
	return &settlementOutput{Body: toSettlementBody(view)}, nil
}

func (h *settlementHandlers) voidHandler(ctx context.Context, in *voidSettlementInput) (*settlementOutput, error) {
	who := identityOf(ctx)
	view, err := h.void.Execute(ctx, billingapp.VoidSettlementInput{
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		SettlementID: in.ID,
		Reason:       in.Body.Reason,
	})
	if err != nil {
		return nil, settlementProblem(ctx, h.logger, err)
	}
	return &settlementOutput{Body: toSettlementBody(view)}, nil
}

// settlementProblem names the settlement where the shared mapping would name the voucher: the
// sentinels of a void and of a lookup are the billing ones, and the screen must say which document.
func settlementProblem(ctx context.Context, logger *slog.Logger, err error) error {
	switch {
	case errors.Is(err, billing.ErrNotFound):
		return huma.Error404NotFound("The requested settlement does not exist")
	case errors.Is(err, billing.ErrAlreadyVoided):
		return huma.Error409Conflict("That settlement is already voided")
	case errors.Is(err, billing.ErrImmutable):
		return huma.Error409Conflict("That settlement cannot change in this state")
	}
	return problem(ctx, logger, err)
}

// ─── mapping ─────────────────────────────────────────────────────────────────

// settlementInstant formats an instant the way the vouchers do: UTC, RFC 3339.
func settlementInstant(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func settlementOptionalInstant(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := settlementInstant(*value)
	return &text
}

func toSettlementBody(view billingapp.SettlementView) settlementBody {
	items := make([]settlementItemBody, 0, len(view.Items))
	for _, item := range view.Items {
		items = append(items, toSettlementItemBody(item))
	}
	return settlementBody{
		ID:           view.ID,
		InsurerID:    view.InsurerID,
		InsurerName:  view.InsurerName,
		InsurerTaxID: view.InsurerTaxID,
		PracticeName: view.PracticeName,
		Period:       view.Period,
		Status:       view.Status,
		CreatedBy:    view.CreatedBy,
		CreatedAt:    settlementInstant(view.CreatedAt),
		VoidedBy:     view.VoidedBy,
		VoidedAt:     settlementOptionalInstant(view.VoidedAt),
		VoidReason:   view.VoidReason,
		Items:        items,
		Totals:       toCurrencyTotalBodies(view.Totals),
	}
}

func toSettlementPreviewBody(view billingapp.SettlementPreviewView) settlementPreviewBody {
	sessions := make([]settlementItemBody, 0, len(view.Sessions))
	for _, item := range view.Sessions {
		sessions = append(sessions, toSettlementItemBody(item))
	}
	unvalued := make([]unvaluedSessionBody, 0, len(view.Unvalued))
	for _, session := range view.Unvalued {
		unvalued = append(unvalued, unvaluedSessionBody{
			AppointmentID: session.AppointmentID,
			PatientName:   session.PatientName,
			Currency:      session.Currency,
			StartsAt:      settlementInstant(session.StartsAt),
		})
	}
	return settlementPreviewBody{
		InsurerID:   view.InsurerID,
		InsurerName: view.InsurerName,
		Period:      view.Period,
		Sessions:    sessions,
		Unvalued:    unvalued,
		Totals:      toCurrencyTotalBodies(view.Totals),
	}
}

func toSettlementItemBody(item billingapp.SettlementItemView) settlementItemBody {
	return settlementItemBody{
		AppointmentID:    item.AppointmentID,
		PatientName:      item.PatientName,
		MemberNumber:     item.MemberNumber,
		ProfessionalName: item.ProfessionalName,
		Modality:         item.Modality,
		StartsAt:         settlementInstant(item.StartsAt),
		Currency:         item.Currency,
		AmountMinor:      item.AmountMinor,
		CopagoMinor:      item.CopagoMinor,
		ReleasedAt:       settlementOptionalInstant(item.ReleasedAt),
	}
}

func toCurrencyTotalBodies(totals []billingapp.CurrencyTotalView) []currencyTotalBody {
	out := make([]currencyTotalBody, 0, len(totals))
	for _, total := range totals {
		out = append(out, currencyTotalBody{Currency: total.Currency, Sessions: total.Sessions, AmountMinor: total.AmountMinor})
	}
	return out
}
