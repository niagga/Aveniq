package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// The constraint names of 0021, so a violation maps to the answer the caller can act on.
const (
	settlementsInsurerFK          = "settlements_insurer_same_tenant"
	settlementsCreatorFK          = "settlements_creator_same_tenant"
	settlementsVoiderFK           = "settlements_voider_same_tenant"
	settlementItemsAppointmentFK  = "settlement_items_appointment_same_tenant"
	settlementItemsPatientFK      = "settlement_items_patient_same_tenant"
	settlementItemsProfessionalFK = "settlement_items_professional_same_tenant"
	settlementItemsOneActive      = "settlement_items_one_active_per_appointment"

	defaultSettlementPage = int32(20)
)

// SettlementRepository is the PostgreSQL adapter for port.SettlementRepository.
type SettlementRepository struct {
	queries *sqlcgen.Queries
}

// NewSettlementRepository builds the adapter over an existing pool.
func NewSettlementRepository(pool *pgxpool.Pool) *SettlementRepository {
	return &SettlementRepository{queries: sqlcgen.New(pool)}
}

var _ port.SettlementRepository = (*SettlementRepository)(nil)

func (r *SettlementRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Insurer reads a mutual of this practice, active or not, or billing.ErrInsurerNotVisible.
func (r *SettlementRepository) Insurer(ctx context.Context, insurerID string) (billing.SettlementInsurer, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.SettlementInsurer{}, err
	}
	parsed, err := parseBillingUUID(insurerID, "insurer_id")
	if err != nil {
		return billing.SettlementInsurer{}, err
	}
	row, err := queries.GetSettlementInsurer(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.SettlementInsurer{}, billing.ErrInsurerNotVisible
	}
	if err != nil {
		return billing.SettlementInsurer{}, translateSettlementError(err)
	}
	return settlementInsurer(row.ID, row.Name, row.TaxID, row.IsActive), nil
}

// Insurers lists the active mutuales of this practice, by name.
func (r *SettlementRepository) Insurers(ctx context.Context) ([]billing.SettlementInsurer, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSettlementInsurers(ctx)
	if err != nil {
		return nil, translateSettlementError(err)
	}
	out := make([]billing.SettlementInsurer, 0, len(rows))
	for _, row := range rows {
		out = append(out, settlementInsurer(row.ID, row.Name, row.TaxID, row.IsActive))
	}
	return out, nil
}

// SettlableSessions returns the attended sessions of the mutual's patients in [from, to) that no
// active settlement liquidates.
func (r *SettlementRepository) SettlableSessions(
	ctx context.Context,
	insurerID string,
	from, to time.Time,
) ([]billing.SettlableSession, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	parsed, err := parseBillingUUID(insurerID, "insurer_id")
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSettlableSessions(ctx, sqlcgen.ListSettlableSessionsParams{
		InsurerID: parsed,
		FromAt:    from,
		ToAt:      to,
	})
	if err != nil {
		return nil, translateSettlementError(err)
	}
	out := make([]billing.SettlableSession, 0, len(rows))
	for _, row := range rows {
		out = append(out, billing.SettlableSession{
			AppointmentID:    row.ID.String(),
			PatientID:        row.PatientID.String(),
			PatientName:      row.PatientName,
			MemberNumber:     textOrEmpty(row.MemberNumber),
			ProfessionalID:   row.ProfessionalUserID.String(),
			ProfessionalName: row.ProfessionalName,
			Modality:         row.Modality,
			StartsAt:         row.StartsAt.UTC(),
			Currency:         row.BillingCurrency,
		})
	}
	return out, nil
}

// Issue writes the settlement and its sessions in the caller's transaction.
func (r *SettlementRepository) Issue(ctx context.Context, settlement billing.Settlement) (billing.Settlement, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Settlement{}, err
	}
	id, err := parseBillingUUID(string(settlement.ID), "id")
	if err != nil {
		return billing.Settlement{}, err
	}
	tenantID, err := toPGUUIDFromTenant(settlement.TenantID)
	if err != nil {
		return billing.Settlement{}, err
	}
	insurerID, err := parseBillingUUID(settlement.InsurerID, "insurer_id")
	if err != nil {
		return billing.Settlement{}, err
	}
	createdBy, err := parseBillingUUID(settlement.CreatedBy, "created_by")
	if err != nil {
		return billing.Settlement{}, err
	}

	row, err := queries.CreateSettlement(ctx, sqlcgen.CreateSettlementParams{
		ID:           id,
		TenantID:     tenantID,
		InsurerID:    insurerID,
		InsurerName:  settlement.InsurerName,
		InsurerTaxID: settlement.InsurerTaxID,
		PracticeName: settlement.PracticeName,
		PeriodStart:  pgtype.Date{Time: settlement.PeriodStart, Valid: true},
		PeriodEnd:    pgtype.Date{Time: settlement.PeriodEnd, Valid: true},
		CreatedBy:    createdBy,
		CreatedAt:    settlement.CreatedAt,
		UpdatedAt:    settlement.UpdatedAt,
	})
	if err != nil {
		return billing.Settlement{}, translateSettlementError(err)
	}

	for _, item := range settlement.Items {
		params, err := settlementItemParams(tenantID, id, item)
		if err != nil {
			return billing.Settlement{}, err
		}
		if err := queries.CreateSettlementItem(ctx, params); err != nil {
			return billing.Settlement{}, translateSettlementError(err)
		}
	}
	return settlementFromRow(row, settlement.Items), nil
}

// Void records the void of an issued settlement and releases its sessions.
func (r *SettlementRepository) Void(ctx context.Context, settlement billing.Settlement) (billing.Settlement, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Settlement{}, err
	}
	if settlement.Status != billing.StatusVoided || settlement.VoidedAt == nil || settlement.VoidReason == nil {
		return billing.Settlement{}, billing.ErrImmutable
	}
	id, err := parseBillingUUID(string(settlement.ID), "id")
	if err != nil {
		return billing.Settlement{}, err
	}
	voidedBy, err := parseBillingUUID(settlement.VoidedBy, "voided_by")
	if err != nil {
		return billing.Settlement{}, err
	}

	row, err := queries.VoidSettlement(ctx, sqlcgen.VoidSettlementParams{
		ID:         id,
		VoidedBy:   voidedBy,
		VoidedAt:   pgtype.Timestamptz{Time: *settlement.VoidedAt, Valid: true},
		VoidReason: settlement.VoidReason,
		UpdatedAt:  settlement.UpdatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := queries.GetSettlement(ctx, id)
		if getErr != nil {
			return billing.Settlement{}, translateSettlementError(getErr)
		}
		if existing.Status == string(billing.StatusVoided) {
			return billing.Settlement{}, billing.ErrAlreadyVoided
		}
		return billing.Settlement{}, billing.ErrImmutable
	}
	if err != nil {
		return billing.Settlement{}, translateSettlementError(err)
	}

	if err := queries.ReleaseSettlementItems(ctx, sqlcgen.ReleaseSettlementItemsParams{
		SettlementID: id,
		ReleasedAt:   pgtype.Timestamptz{Time: *settlement.VoidedAt, Valid: true},
	}); err != nil {
		return billing.Settlement{}, translateSettlementError(err)
	}
	return settlementFromRow(row, settlement.Items), nil
}

// ByID reads one settlement with its sessions, or billing.ErrNotFound.
func (r *SettlementRepository) ByID(ctx context.Context, id billing.SettlementID) (billing.Settlement, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Settlement{}, err
	}
	parsed, err := parseBillingUUID(string(id), "id")
	if err != nil {
		return billing.Settlement{}, err
	}
	row, err := queries.GetSettlement(ctx, parsed)
	if err != nil {
		return billing.Settlement{}, translateSettlementError(err)
	}
	items, err := r.itemsOf(ctx, queries, []pgtype.UUID{row.ID})
	if err != nil {
		return billing.Settlement{}, err
	}
	return settlementFromRow(row, items[uuidText(row.ID)]), nil
}

// List returns one page of settlements, newest first, each with its sessions.
func (r *SettlementRepository) List(ctx context.Context, filter port.SettlementFilter) ([]billing.Settlement, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultSettlementPage
	}
	rows, err := queries.ListSettlements(ctx, sqlcgen.ListSettlementsParams{
		InsurerID: optionalID(filter.InsurerID),
		Lim:       limit,
		Off:       filter.Offset,
	})
	if err != nil {
		return nil, translateSettlementError(err)
	}

	ids := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	items, err := r.itemsOf(ctx, queries, ids)
	if err != nil {
		return nil, err
	}

	out := make([]billing.Settlement, 0, len(rows))
	for _, row := range rows {
		out = append(out, settlementFromRow(row, items[uuidText(row.ID)]))
	}
	return out, nil
}

// itemsOf reads the sessions of several settlements in one query, grouped by settlement id.
func (r *SettlementRepository) itemsOf(
	ctx context.Context,
	queries *sqlcgen.Queries,
	settlementIDs []pgtype.UUID,
) (map[string][]billing.SettlementItem, error) {
	grouped := map[string][]billing.SettlementItem{}
	if len(settlementIDs) == 0 {
		return grouped, nil
	}
	rows, err := queries.ListSettlementItems(ctx, settlementIDs)
	if err != nil {
		return nil, translateSettlementError(err)
	}
	for _, row := range rows {
		key := uuidText(row.SettlementID)
		grouped[key] = append(grouped[key], settlementItemFromRow(row))
	}
	return grouped, nil
}

func settlementInsurer(id pgtype.UUID, name string, taxID *string, active bool) billing.SettlementInsurer {
	return billing.SettlementInsurer{ID: id.String(), Name: name, TaxID: taxID, Active: active}
}

func settlementItemParams(tenantID, settlementID pgtype.UUID, item billing.SettlementItem) (sqlcgen.CreateSettlementItemParams, error) {
	appointmentID, err := parseBillingUUID(item.AppointmentID, "appointment_id")
	if err != nil {
		return sqlcgen.CreateSettlementItemParams{}, err
	}
	patientID, err := parseBillingUUID(item.PatientID, "patient_id")
	if err != nil {
		return sqlcgen.CreateSettlementItemParams{}, err
	}
	professionalID, err := parseBillingUUID(item.ProfessionalID, "professional_user_id")
	if err != nil {
		return sqlcgen.CreateSettlementItemParams{}, err
	}
	return sqlcgen.CreateSettlementItemParams{
		TenantID:           tenantID,
		SettlementID:       settlementID,
		AppointmentID:      appointmentID,
		PatientID:          patientID,
		PatientName:        item.PatientName,
		MemberNumber:       item.MemberNumber,
		ProfessionalUserID: professionalID,
		ProfessionalName:   item.ProfessionalName,
		Modality:           item.Modality,
		StartsAt:           item.StartsAt,
		Currency:           item.Currency,
		AmountMinor:        item.AmountMinor,
		CopagoMinor:        item.CopagoMinor,
	}, nil
}

func settlementFromRow(row sqlcgen.Settlement, items []billing.SettlementItem) billing.Settlement {
	settlement := billing.Settlement{
		ID:           billing.SettlementID(uuidText(row.ID)),
		TenantID:     tenant.ID(uuidText(row.TenantID)),
		InsurerID:    uuidText(row.InsurerID),
		InsurerName:  row.InsurerName,
		InsurerTaxID: row.InsurerTaxID,
		PracticeName: row.PracticeName,
		PeriodStart:  row.PeriodStart.Time.UTC(),
		PeriodEnd:    row.PeriodEnd.Time.UTC(),
		Status:       billing.Status(row.Status),
		CreatedBy:    uuidText(row.CreatedBy),
		CreatedAt:    row.CreatedAt.UTC(),
		VoidedBy:     uuidText(row.VoidedBy),
		VoidReason:   row.VoidReason,
		UpdatedAt:    row.UpdatedAt.UTC(),
		Items:        items,
	}
	if row.VoidedAt.Valid {
		voidedAt := row.VoidedAt.Time.UTC()
		settlement.VoidedAt = &voidedAt
	}
	return settlement
}

func settlementItemFromRow(row sqlcgen.SettlementItem) billing.SettlementItem {
	item := billing.SettlementItem{
		AppointmentID:    uuidText(row.AppointmentID),
		PatientID:        uuidText(row.PatientID),
		PatientName:      row.PatientName,
		MemberNumber:     row.MemberNumber,
		ProfessionalID:   uuidText(row.ProfessionalUserID),
		ProfessionalName: row.ProfessionalName,
		Modality:         row.Modality,
		StartsAt:         row.StartsAt.UTC(),
		Currency:         row.Currency,
		AmountMinor:      row.AmountMinor,
		CopagoMinor:      row.CopagoMinor,
	}
	if row.ReleasedAt.Valid {
		releasedAt := row.ReleasedAt.Time.UTC()
		item.ReleasedAt = &releasedAt
	}
	return item
}

// translateSettlementError maps the constraint that refused a write to the domain answer the
// caller acts on. Anything else is returned as it is.
func translateSettlementError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case foreignKeyViolationCode:
		switch pgErr.ConstraintName {
		case settlementsInsurerFK:
			return fmt.Errorf("%w: constraint %s", billing.ErrInsurerNotVisible, pgErr.ConstraintName)
		case settlementItemsAppointmentFK:
			return fmt.Errorf("%w: constraint %s", billing.ErrAppointmentNotVisible, pgErr.ConstraintName)
		case settlementItemsPatientFK:
			return fmt.Errorf("%w: constraint %s", billing.ErrPatientNotVisible, pgErr.ConstraintName)
		case settlementItemsProfessionalFK:
			return fmt.Errorf("%w: constraint %s", billing.ErrProfessionalNotVisible, pgErr.ConstraintName)
		case settlementsCreatorFK, settlementsVoiderFK:
			return fmt.Errorf("%w: constraint %s", billing.ErrActorNotVisible, pgErr.ConstraintName)
		}
	case uniqueViolationCode:
		if pgErr.ConstraintName == settlementItemsOneActive {
			return fmt.Errorf("%w: constraint %s", billing.ErrAppointmentAlreadySettled, pgErr.ConstraintName)
		}
	case restrictViolationCode:
		return fmt.Errorf("%w: %s", billing.ErrImmutable, pgErr.Message)
	}
	return err
}
