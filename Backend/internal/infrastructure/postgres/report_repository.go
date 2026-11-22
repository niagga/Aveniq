package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// ReportRepository is the PostgreSQL adapter for port.ReportRepository. It only reads.
type ReportRepository struct {
	queries *sqlcgen.Queries
}

// NewReportRepository builds the adapter over an existing pool.
func NewReportRepository(pool *pgxpool.Pool) *ReportRepository {
	return &ReportRepository{queries: sqlcgen.New(pool)}
}

var _ port.ReportRepository = (*ReportRepository)(nil)

func (r *ReportRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// BilledLines returns the issued vouchers issued in the window, oldest first.
func (r *ReportRepository) BilledLines(ctx context.Context, window report.Window, filter report.Filter) ([]report.BilledLine, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	professionalID, err := optionalBillingUUID(filter.ProfessionalID, "professionalId")
	if err != nil {
		return nil, err
	}
	insurerID, err := optionalBillingUUID(filter.InsurerID, "insurerId")
	if err != nil {
		return nil, err
	}
	rows, err := queries.ReportBilledVouchers(ctx, sqlcgen.ReportBilledVouchersParams{
		FromAt:         window.From,
		ToAt:           window.To,
		ProfessionalID: professionalID,
		Coverage:       optionalText(filter.Coverage),
		InsurerID:      insurerID,
		Modality:       optionalText(filter.Modality),
	})
	if err != nil {
		return nil, fmt.Errorf("read the billed vouchers: %w", err)
	}
	out := make([]report.BilledLine, 0, len(rows))
	for _, row := range rows {
		out = append(out, report.BilledLine{
			Line: report.Line{
				At:               row.IssuedAt.UTC(),
				Currency:         row.Currency,
				Coverage:         textOrEmpty(row.CoverageKind),
				InsurerID:        uuidText(row.InsurerID),
				InsurerName:      textOrEmpty(row.InsurerName),
				Modality:         textOrEmpty(row.Modality),
				ProfessionalID:   uuidText(row.ProfessionalUserID),
				ProfessionalName: textOrEmpty(row.ProfessionalName),
			},
			AmountMinor: row.AmountMinor,
		})
	}
	return out, nil
}

// PendingSessions returns the attended sessions of the window that no issued voucher bills,
// oldest first.
func (r *ReportRepository) PendingSessions(ctx context.Context, window report.Window, filter report.Filter) ([]report.Line, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	professionalID, err := optionalBillingUUID(filter.ProfessionalID, "professionalId")
	if err != nil {
		return nil, err
	}
	insurerID, err := optionalBillingUUID(filter.InsurerID, "insurerId")
	if err != nil {
		return nil, err
	}
	rows, err := queries.ReportPendingSessions(ctx, sqlcgen.ReportPendingSessionsParams{
		FromAt:         window.From,
		ToAt:           window.To,
		ProfessionalID: professionalID,
		Coverage:       optionalText(filter.Coverage),
		InsurerID:      insurerID,
		Modality:       optionalText(filter.Modality),
	})
	if err != nil {
		return nil, fmt.Errorf("read the sessions without a voucher: %w", err)
	}
	out := make([]report.Line, 0, len(rows))
	for _, row := range rows {
		out = append(out, report.Line{
			At:               row.StartsAt.UTC(),
			Currency:         row.BillingCurrency,
			Coverage:         row.CoverageKind,
			InsurerID:        uuidText(row.InsurerID),
			InsurerName:      textOrEmpty(row.InsurerName),
			Modality:         row.Modality,
			ProfessionalID:   uuidText(row.ProfessionalUserID),
			ProfessionalName: row.ProfessionalName,
		})
	}
	return out, nil
}
