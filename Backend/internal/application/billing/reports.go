package billingapp

import (
	"context"
	"errors"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/report"
)

// The two reaches of a report. A role that reads the practice's figures sees all of them; a
// professional reads only the sessions and vouchers that are their own.
const (
	ReportScopePractice = "practice"
	ReportScopeOwn      = "own"
)

// ReportInput is one question to the report.
type ReportInput struct {
	TenantID string
	// ViewerID and ViewerRole are who asks. They decide the scope: the request cannot widen it.
	ViewerID   string
	ViewerRole string

	// From and To are civil days of the practice, both included.
	From string
	To   string

	// GroupBy is the cut; empty is no cut.
	GroupBy string

	Filter report.Filter
}

// ReportView is the answer: the report and the frame it was computed in.
type ReportView struct {
	From     string
	To       string
	Timezone string
	GroupBy  report.Dimension
	Scope    string
	Report   report.Report
}

// ReportUseCase answers what the practice billed and what it attended without billing.
//
// The figures are read live from the vouchers and the sessions. The sessions without a voucher
// are valued with the tariff in force on their day, the same one the billing screen proposes, so
// the report and the voucher the practice would issue agree.
type ReportUseCase struct {
	reports port.ReportRepository
	tariffs port.TariffRepository
	tenants port.TenantRepository
	scope   port.TenantScope
}

// NewReportUseCase wires the use case.
func NewReportUseCase(
	reports port.ReportRepository,
	tariffs port.TariffRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
) *ReportUseCase {
	return &ReportUseCase{reports: reports, tariffs: tariffs, tenants: tenants, scope: scope}
}

// Execute computes the report. A role that may read no report is report.ErrForbidden even if the
// interface let it through; a professional's own scope narrows the filter to themselves whatever
// the request said, so asking for a colleague's figures returns their own, never the colleague's.
func (uc *ReportUseCase) Execute(ctx context.Context, input ReportInput) (ReportView, error) {
	scope, filter, err := scopeOf(input)
	if err != nil {
		return ReportView{}, err
	}
	dimension, err := report.ParseDimension(input.GroupBy)
	if err != nil {
		return ReportView{}, err
	}
	if err := filter.Validate(); err != nil {
		return ReportView{}, err
	}
	_, location, err := practiceCalendar(ctx, uc.tenants, input.TenantID)
	if err != nil {
		return ReportView{}, err
	}
	window, err := report.ParseWindow(input.From, input.To, location)
	if err != nil {
		return ReportView{}, err
	}

	var built report.Report
	err = uc.scope.WithTenant(ctx, input.TenantID, func(ctx context.Context) error {
		billed, err := uc.reports.BilledLines(ctx, window, filter)
		if err != nil {
			return err
		}
		sessions, err := uc.reports.PendingSessions(ctx, window, filter)
		if err != nil {
			return err
		}
		pending, err := uc.value(ctx, location, sessions)
		if err != nil {
			return err
		}
		built = report.Build(dimension, location, billed, pending)
		return nil
	})
	if err != nil {
		return ReportView{}, err
	}
	return ReportView{
		From:     input.From,
		To:       input.To,
		Timezone: location.String(),
		GroupBy:  dimension,
		Scope:    scope,
		Report:   built,
	}, nil
}

// scopeOf decides what the viewer may read and forces the filter to match it.
func scopeOf(input ReportInput) (string, report.Filter, error) {
	role := iam.RoleCode(input.ViewerRole)
	filter := input.Filter
	switch {
	case iam.Allows(role, iam.PermReportsRead):
		return ReportScopePractice, filter, nil
	case iam.Allows(role, iam.PermReportsReadOwn):
		filter.ProfessionalID = input.ViewerID
		return ReportScopeOwn, filter, nil
	default:
		return "", report.Filter{}, report.ErrForbidden
	}
}

// value prices each session with the tariff in force on its day. A session with no tariff, or
// whose tariff is worth nothing, is left unvalued. The query is what decides a tariff, so
// sessions that share it share the answer and the tariff is asked once for all of them.
func (uc *ReportUseCase) value(ctx context.Context, location *time.Location, sessions []report.Line) ([]report.PendingLine, error) {
	known := map[port.TariffQuery]*int64{}
	out := make([]report.PendingLine, 0, len(sessions))
	for _, session := range sessions {
		day := session.At.In(location)
		query := port.TariffQuery{
			ProfessionalUserID: session.ProfessionalID,
			Coverage:           session.Coverage,
			InsurerID:          session.InsurerID,
			Modality:           session.Modality,
			Currency:           session.Currency,
			Day:                time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		}
		total, seen := known[query]
		if !seen {
			tariff, err := uc.tariffs.InForce(ctx, query)
			switch {
			case errors.Is(err, pricing.ErrNotFound):
				total = nil
			case err != nil:
				return nil, err
			case tariff.TotalMinor() <= 0:
				total = nil
			default:
				amount := tariff.TotalMinor()
				total = &amount
			}
			known[query] = total
		}
		if total == nil {
			out = append(out, report.PendingLine{Line: session})
			continue
		}
		out = append(out, report.PendingLine{Line: session, Valued: true, AmountMinor: *total})
	}
	return out, nil
}
