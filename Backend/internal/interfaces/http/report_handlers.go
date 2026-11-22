package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	billingapp "github.com/niagga/aveniq/backend/internal/application/billing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/report"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// reportMoneyBody is the sum of one currency. Currencies are never added together or converted.
type reportMoneyBody struct {
	Currency    string `json:"currency" pattern:"^[A-Z]{3}$"`
	Count       int    `json:"count" doc:"How many vouchers or sessions make the amount."`
	AmountMinor int64  `json:"amountMinor" doc:"In minor units (centavos)."`
}

// reportFiguresBody are the numbers of the whole report or of one row.
type reportFiguresBody struct {
	Billed           []reportMoneyBody `json:"billed" doc:"What the issued vouchers billed, by currency. A voided voucher is not in it."`
	Pending          []reportMoneyBody `json:"pending" doc:"What the attended sessions without an issued voucher are worth at the tariff in force on their day, by currency. Only the valued sessions are in it."`
	UnvaluedSessions int               `json:"unvaluedSessions" doc:"Attended sessions without an issued voucher that no tariff values. Counted apart, never as zero."`
}

// reportRowBody is one group of a report.
type reportRowBody struct {
	Key              string            `json:"key" doc:"Identifies the group: an id, a word, or the first day of the period. Empty for the lines that carry no value for the cut."`
	Label            string            `json:"label" doc:"A name to show. Empty when the group has none; the client says so."`
	Billed           []reportMoneyBody `json:"billed"`
	Pending          []reportMoneyBody `json:"pending"`
	UnvaluedSessions int               `json:"unvaluedSessions"`
}

// billingReportBody is the answer: the frame, the totals and one row per group of the cut.
type billingReportBody struct {
	From     string            `json:"from" doc:"First day, as asked."`
	To       string            `json:"to" doc:"Last day, as asked. Included."`
	Timezone string            `json:"timezone" doc:"The practice's time zone: the range and the periods are read in it."`
	GroupBy  string            `json:"groupBy" enum:"none,professional,coverage,insurer,modality,week,month,year"`
	Scope    string            `json:"scope" enum:"practice,own" doc:"own is a professional reading only their sessions and vouchers."`
	Totals   reportFiguresBody `json:"totals"`
	Rows     []reportRowBody   `json:"rows" doc:"Empty when there is no cut. Their amounts add up to the totals."`
}

type billingReportInput struct {
	From           string `query:"from" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$" required:"true" doc:"First day of the range, YYYY-MM-DD, in the practice's time zone."`
	To             string `query:"to" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$" required:"true" doc:"Last day of the range, YYYY-MM-DD, included."`
	GroupBy        string `query:"groupBy" enum:"none,professional,coverage,insurer,modality,week,month,year" doc:"The cut. Default none: only the totals."`
	ProfessionalID string `query:"professionalId" format:"uuid" doc:"Only this professional. A professional with the own scope always reads their own."`
	Coverage       string `query:"coverage" enum:"private,insurer" doc:"Only private patients or only patients covered by a mutual."`
	InsurerID      string `query:"insurerId" format:"uuid" doc:"Only the patients covered by this mutual."`
	Modality       string `query:"modality" enum:"online,presencial" doc:"Only this modality."`
}

type billingReportOutput struct {
	Body billingReportBody
}

// ─── routes ──────────────────────────────────────────────────────────────────

type reportHandlers struct {
	billing *billingapp.ReportUseCase
	logger  *slog.Logger
}

func registerReportRoutes(api huma.API, deps Deps) {
	handlers := &reportHandlers{billing: deps.BillingReport, logger: deps.Logger}

	// Either permission opens the route; the use case decides the scope from the role, so a
	// professional is held to their own figures even if the request names a colleague.
	guard := permissionGuard(api, deps.AccessTokens, iam.PermReportsRead, iam.PermReportsReadOwn)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID: "report-billing",
		Method:      http.MethodGet,
		Path:        APIPrefix + "/reports/billing",
		Summary:     "Billed and attended-but-unbilled figures",
		Description: "What the practice billed (issued vouchers, never voided ones) and what it attended " +
			"without billing (valued at the tariff in force on the day of each session; sessions with no " +
			"tariff are counted apart), by currency and without conversion. The range is read in the " +
			"practice's time zone. A professional reads only their own figures and never the voucher list.",
		Tags:          []string{"reports"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.billingHandler)
}

func (h *reportHandlers) billingHandler(ctx context.Context, in *billingReportInput) (*billingReportOutput, error) {
	who := identityOf(ctx)
	view, err := h.billing.Execute(ctx, billingapp.ReportInput{
		TenantID:   who.TenantID,
		ViewerID:   who.UserID,
		ViewerRole: who.Role,
		From:       in.From,
		To:         in.To,
		GroupBy:    in.GroupBy,
		Filter: report.Filter{
			ProfessionalID: in.ProfessionalID,
			Coverage:       in.Coverage,
			InsurerID:      in.InsurerID,
			Modality:       in.Modality,
		},
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &billingReportOutput{Body: toBillingReportBody(view)}, nil
}

// ─── mapping ─────────────────────────────────────────────────────────────────

func toBillingReportBody(view billingapp.ReportView) billingReportBody {
	rows := make([]reportRowBody, 0, len(view.Report.Rows))
	for _, row := range view.Report.Rows {
		figures := toReportFiguresBody(row.Figures)
		rows = append(rows, reportRowBody{
			Key:              row.Key,
			Label:            row.Label,
			Billed:           figures.Billed,
			Pending:          figures.Pending,
			UnvaluedSessions: figures.UnvaluedSessions,
		})
	}
	return billingReportBody{
		From:     view.From,
		To:       view.To,
		Timezone: view.Timezone,
		GroupBy:  string(view.GroupBy),
		Scope:    view.Scope,
		Totals:   toReportFiguresBody(view.Report.Totals),
		Rows:     rows,
	}
}

func toReportFiguresBody(figures report.Figures) reportFiguresBody {
	return reportFiguresBody{
		Billed:           toReportMoneyBodies(figures.Billed),
		Pending:          toReportMoneyBodies(figures.Pending),
		UnvaluedSessions: figures.Unvalued,
	}
}

func toReportMoneyBodies(figures []report.Money) []reportMoneyBody {
	out := make([]reportMoneyBody, 0, len(figures))
	for _, money := range figures {
		out = append(out, reportMoneyBody{Currency: money.Currency, Count: money.Count, AmountMinor: money.AmountMinor})
	}
	return out
}
