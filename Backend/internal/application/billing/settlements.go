package billingapp

import (
	"context"
	"errors"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// SettlementMonthLayout is how a period reads on the wire: a calendar month, YYYY-MM.
const SettlementMonthLayout = "2006-01"

// ─── views ───────────────────────────────────────────────────────────────────

// SettlementInsurerView is a mutual the practice can settle with. Settlable is false until the
// mutual has its CUIT: a settlement is addressed to it.
type SettlementInsurerView struct {
	ID        string
	Name      string
	TaxID     *string
	Settlable bool
}

// SettlementItemView is one session of a settlement, with the mutual's share as valued.
type SettlementItemView struct {
	AppointmentID    string
	PatientName      string
	MemberNumber     string
	ProfessionalName string
	Modality         string
	StartsAt         time.Time
	Currency         string
	AmountMinor      int64
	CopagoMinor      int64
	ReleasedAt       *time.Time
}

// CurrencyTotalView is the sum of a settlement's sessions in one currency.
type CurrencyTotalView struct {
	Currency    string
	Sessions    int
	AmountMinor int64
}

// UnvaluedSessionView is an attended session the tariff in force does not price. It is listed,
// and left out of the settlement: it is settled once a tariff prices it.
type UnvaluedSessionView struct {
	AppointmentID string
	PatientName   string
	Currency      string
	StartsAt      time.Time
}

// SettlementPreviewView is what a settlement would contain, computed the same way it is issued.
type SettlementPreviewView struct {
	InsurerID   string
	InsurerName string
	Period      string
	Sessions    []SettlementItemView
	Unvalued    []UnvaluedSessionView
	Totals      []CurrencyTotalView
}

// SettlementView is an issued settlement: the snapshot and its sessions.
type SettlementView struct {
	ID           string
	InsurerID    string
	InsurerName  string
	InsurerTaxID string
	PracticeName string
	Period       string
	Status       string
	CreatedBy    string
	CreatedAt    time.Time
	VoidedBy     string
	VoidedAt     *time.Time
	VoidReason   *string
	Items        []SettlementItemView
	Totals       []CurrencyTotalView
}

func toSettlementInsurerView(insurer billing.SettlementInsurer) SettlementInsurerView {
	return SettlementInsurerView{
		ID:        insurer.ID,
		Name:      insurer.Name,
		TaxID:     insurer.TaxID,
		Settlable: insurer.TaxID != nil,
	}
}

func toSettlementItemView(item billing.SettlementItem) SettlementItemView {
	return SettlementItemView{
		AppointmentID:    item.AppointmentID,
		PatientName:      item.PatientName,
		MemberNumber:     item.MemberNumber,
		ProfessionalName: item.ProfessionalName,
		Modality:         item.Modality,
		StartsAt:         item.StartsAt,
		Currency:         item.Currency,
		AmountMinor:      item.AmountMinor,
		CopagoMinor:      item.CopagoMinor,
		ReleasedAt:       item.ReleasedAt,
	}
}

func toCurrencyTotalViews(totals []billing.CurrencyTotal) []CurrencyTotalView {
	out := make([]CurrencyTotalView, 0, len(totals))
	for _, total := range totals {
		out = append(out, CurrencyTotalView{Currency: total.Currency, Sessions: total.Sessions, AmountMinor: total.AmountMinor})
	}
	return out
}

func toSettlementView(settlement billing.Settlement) SettlementView {
	items := make([]SettlementItemView, 0, len(settlement.Items))
	for _, item := range settlement.Items {
		items = append(items, toSettlementItemView(item))
	}
	return SettlementView{
		ID:           string(settlement.ID),
		InsurerID:    settlement.InsurerID,
		InsurerName:  settlement.InsurerName,
		InsurerTaxID: settlement.InsurerTaxID,
		PracticeName: settlement.PracticeName,
		Period:       settlement.PeriodStart.Format(SettlementMonthLayout),
		Status:       string(settlement.Status),
		CreatedBy:    settlement.CreatedBy,
		CreatedAt:    settlement.CreatedAt,
		VoidedBy:     settlement.VoidedBy,
		VoidedAt:     settlement.VoidedAt,
		VoidReason:   settlement.VoidReason,
		Items:        items,
		Totals:       toCurrencyTotalViews(settlement.Totals()),
	}
}

// ─── valuation ───────────────────────────────────────────────────────────────

// settlementValuation is the valued content of one month for one mutual. The preview shows it and
// the issue writes it, from the same code, so what is previewed is what is issued.
type settlementValuation struct {
	insurer  billing.SettlementInsurer
	firstDay time.Time
	items    []billing.SettlementItem
	unvalued []billing.SettlableSession
}

// valueSettlement reads the attended sessions of the mutual's patients in the month and values each
// one at the mutual's share of the tariff in force on its civil day. A session the tariff does not
// price is unvalued. It must run inside the tenant scope, so the reads and the write share a snapshot.
func valueSettlement(
	ctx context.Context,
	settlements port.SettlementRepository,
	tariffs port.TariffRepository,
	location *time.Location,
	insurerID string,
	firstDay time.Time,
) (settlementValuation, error) {
	insurer, err := settlements.Insurer(ctx, insurerID)
	if err != nil {
		return settlementValuation{}, err
	}
	from, to := billing.MonthBounds(firstDay, location)
	sessions, err := settlements.SettlableSessions(ctx, insurerID, from, to)
	if err != nil {
		return settlementValuation{}, err
	}

	valuation := settlementValuation{insurer: insurer, firstDay: firstDay}
	for _, session := range sessions {
		day := session.StartsAt.In(location)
		tariff, err := tariffs.InForce(ctx, port.TariffQuery{
			ProfessionalUserID: session.ProfessionalID,
			Coverage:           string(billing.CoverageInsurer),
			InsurerID:          insurerID,
			Modality:           session.Modality,
			Currency:           session.Currency,
			Day:                time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		})
		if errors.Is(err, pricing.ErrNotFound) || (err == nil && tariff.InsurerMinor <= 0) {
			valuation.unvalued = append(valuation.unvalued, session)
			continue
		}
		if err != nil {
			return settlementValuation{}, err
		}
		valuation.items = append(valuation.items, billing.SettlementItem{
			AppointmentID:    session.AppointmentID,
			PatientID:        session.PatientID,
			PatientName:      session.PatientName,
			MemberNumber:     session.MemberNumber,
			ProfessionalID:   session.ProfessionalID,
			ProfessionalName: session.ProfessionalName,
			Modality:         session.Modality,
			StartsAt:         session.StartsAt,
			Currency:         session.Currency,
			AmountMinor:      tariff.InsurerMinor,
			CopagoMinor:      tariff.PatientMinor,
		})
	}
	return valuation, nil
}

// practiceCalendar reads the practice's timezone; an unset one is the product default.
func practiceCalendar(ctx context.Context, tenants port.TenantRepository, tenantID string) (tenant.Tenant, *time.Location, error) {
	practice, err := tenants.ByID(ctx, tenant.ID(tenantID))
	if err != nil {
		return tenant.Tenant{}, nil, err
	}
	location, err := practiceLocation(practice.Timezone)
	if err != nil {
		return tenant.Tenant{}, nil, err
	}
	return practice, location, nil
}

// ─── preview ─────────────────────────────────────────────────────────────────

// SettlementPreviewUseCase shows what a settlement of one mutual for one month would contain.
// It writes nothing.
type SettlementPreviewUseCase struct {
	settlements port.SettlementRepository
	tariffs     port.TariffRepository
	tenants     port.TenantRepository
	scope       port.TenantScope
}

// NewSettlementPreviewUseCase wires the use case.
func NewSettlementPreviewUseCase(
	settlements port.SettlementRepository,
	tariffs port.TariffRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
) *SettlementPreviewUseCase {
	return &SettlementPreviewUseCase{settlements: settlements, tariffs: tariffs, tenants: tenants, scope: scope}
}

// Execute values the month. A period that is not a month, or a mutual the practice does not hold,
// is refused before anything is read.
func (uc *SettlementPreviewUseCase) Execute(ctx context.Context, tenantID, insurerID, period string) (SettlementPreviewView, error) {
	firstDay, err := billing.ParsePeriod(period)
	if err != nil {
		return SettlementPreviewView{}, err
	}
	_, location, err := practiceCalendar(ctx, uc.tenants, tenantID)
	if err != nil {
		return SettlementPreviewView{}, err
	}

	var view SettlementPreviewView
	err = uc.scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		valuation, err := valueSettlement(ctx, uc.settlements, uc.tariffs, location, insurerID, firstDay)
		if err != nil {
			return err
		}
		view = toSettlementPreviewView(valuation)
		return nil
	})
	if err != nil {
		return SettlementPreviewView{}, err
	}
	return view, nil
}

func toSettlementPreviewView(valuation settlementValuation) SettlementPreviewView {
	items := make([]SettlementItemView, 0, len(valuation.items))
	for _, item := range valuation.items {
		items = append(items, toSettlementItemView(item))
	}
	unvalued := make([]UnvaluedSessionView, 0, len(valuation.unvalued))
	for _, session := range valuation.unvalued {
		unvalued = append(unvalued, UnvaluedSessionView{
			AppointmentID: session.AppointmentID,
			PatientName:   session.PatientName,
			Currency:      session.Currency,
			StartsAt:      session.StartsAt,
		})
	}
	return SettlementPreviewView{
		InsurerID:   valuation.insurer.ID,
		InsurerName: valuation.insurer.Name,
		Period:      valuation.firstDay.Format(SettlementMonthLayout),
		Sessions:    items,
		Unvalued:    unvalued,
		Totals:      toCurrencyTotalViews((billing.Settlement{Items: valuation.items}).Totals()),
	}
}

// ─── issue ───────────────────────────────────────────────────────────────────

// IssueSettlementInput asks for the settlement of one mutual for one month. The sessions are not
// in the request: the server values them, so the mutual is billed for what the tariff says.
type IssueSettlementInput struct {
	TenantID  string
	ActorID   string
	InsurerID string
	Period    string
}

// IssueSettlementUseCase issues a settlement and marks its sessions as settled.
type IssueSettlementUseCase struct {
	settlements port.SettlementRepository
	tariffs     port.TariffRepository
	tenants     port.TenantRepository
	scope       port.TenantScope
	audit       port.AuditLog
	clock       port.Clock
	ids         port.IDGenerator
}

// NewIssueSettlementUseCase wires the use case.
func NewIssueSettlementUseCase(
	settlements port.SettlementRepository,
	tariffs port.TariffRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
) *IssueSettlementUseCase {
	return &IssueSettlementUseCase{
		settlements: settlements, tariffs: tariffs, tenants: tenants,
		scope: scope, audit: audit, clock: clock, ids: ids,
	}
}

// Execute values the month and writes the settlement with its sessions in one transaction. A
// session already in an active settlement answers 409; a mutual given up answers 422.
func (uc *IssueSettlementUseCase) Execute(ctx context.Context, in IssueSettlementInput) (SettlementView, error) {
	firstDay, err := billing.ParsePeriod(in.Period)
	if err != nil {
		return SettlementView{}, err
	}
	practice, location, err := practiceCalendar(ctx, uc.tenants, in.TenantID)
	if err != nil {
		return SettlementView{}, err
	}

	var view SettlementView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		valuation, err := valueSettlement(ctx, uc.settlements, uc.tariffs, location, in.InsurerID, firstDay)
		if err != nil {
			return err
		}
		if !valuation.insurer.Active {
			return &billing.ValidationError{Field: "insurer_id", Message: "the mutual is deactivated: it cannot be settled"}
		}

		settlement, err := billing.NewSettlement(
			billing.SettlementID(uc.ids.NewID()),
			tenant.ID(in.TenantID),
			in.ActorID,
			practice.Name,
			valuation.insurer,
			valuation.firstDay,
			valuation.items,
			uc.clock.Now(),
		)
		if err != nil {
			return err
		}

		saved, err := uc.settlements.Issue(ctx, settlement)
		if err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "settlement.issue",
			Entity:   "settlement",
			EntityID: string(saved.ID),
			Metadata: map[string]any{"insurerId": saved.InsurerID, "period": in.Period, "sessions": len(saved.Items)},
		}); err != nil {
			return err
		}
		view = toSettlementView(saved)
		return nil
	})
	if err != nil {
		return SettlementView{}, err
	}
	return view, nil
}

// ─── void ────────────────────────────────────────────────────────────────────

// VoidSettlementInput asks to void one settlement. The reason is required, as for a voucher.
type VoidSettlementInput struct {
	TenantID     string
	ActorID      string
	SettlementID string
	Reason       string
}

// VoidSettlementUseCase voids a settlement and releases its sessions, which can be settled again.
type VoidSettlementUseCase struct {
	settlements port.SettlementRepository
	scope       port.TenantScope
	audit       port.AuditLog
	clock       port.Clock
}

// NewVoidSettlementUseCase wires the use case.
func NewVoidSettlementUseCase(
	settlements port.SettlementRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
) *VoidSettlementUseCase {
	return &VoidSettlementUseCase{settlements: settlements, scope: scope, audit: audit, clock: clock}
}

// Execute voids the settlement. One already voided answers billing.ErrAlreadyVoided.
func (uc *VoidSettlementUseCase) Execute(ctx context.Context, in VoidSettlementInput) (SettlementView, error) {
	var view SettlementView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.settlements.ByID(ctx, billing.SettlementID(in.SettlementID))
		if err != nil {
			return err
		}
		voided, err := current.Void(in.ActorID, in.Reason, uc.clock.Now())
		if err != nil {
			return err
		}
		saved, err := uc.settlements.Void(ctx, voided)
		if err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "settlement.void",
			Entity:   "settlement",
			EntityID: string(saved.ID),
			Metadata: map[string]any{"reason": in.Reason, "sessions": len(saved.Items)},
		}); err != nil {
			return err
		}
		view = toSettlementView(saved)
		return nil
	})
	if err != nil {
		return SettlementView{}, err
	}
	return view, nil
}

// ─── reads ───────────────────────────────────────────────────────────────────

// GetSettlementUseCase reads one settlement with its sessions.
type GetSettlementUseCase struct {
	settlements port.SettlementRepository
	scope       port.TenantScope
}

// NewGetSettlementUseCase wires the use case.
func NewGetSettlementUseCase(settlements port.SettlementRepository, scope port.TenantScope) *GetSettlementUseCase {
	return &GetSettlementUseCase{settlements: settlements, scope: scope}
}

// Execute returns the settlement, or billing.ErrNotFound.
func (uc *GetSettlementUseCase) Execute(ctx context.Context, tenantID, settlementID string) (SettlementView, error) {
	var view SettlementView
	err := uc.scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		settlement, err := uc.settlements.ByID(ctx, billing.SettlementID(settlementID))
		if err != nil {
			return err
		}
		view = toSettlementView(settlement)
		return nil
	})
	if err != nil {
		return SettlementView{}, err
	}
	return view, nil
}

// ListSettlementsInput is one page of settlements, optionally narrowed to one mutual.
type ListSettlementsInput struct {
	TenantID  string
	InsurerID string
	Limit     int32
	Offset    int32
}

// ListSettlementsUseCase reads the settlements of the practice, newest first.
type ListSettlementsUseCase struct {
	settlements port.SettlementRepository
	scope       port.TenantScope
}

// NewListSettlementsUseCase wires the use case.
func NewListSettlementsUseCase(settlements port.SettlementRepository, scope port.TenantScope) *ListSettlementsUseCase {
	return &ListSettlementsUseCase{settlements: settlements, scope: scope}
}

// Execute validates the page and reads it.
func (uc *ListSettlementsUseCase) Execute(ctx context.Context, in ListSettlementsInput) ([]SettlementView, error) {
	limit, offset, err := pageOf(in.Limit, in.Offset)
	if err != nil {
		return nil, err
	}
	var views []SettlementView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		settlements, err := uc.settlements.List(ctx, port.SettlementFilter{
			InsurerID: in.InsurerID,
			Limit:     limit,
			Offset:    offset,
		})
		if err != nil {
			return err
		}
		views = make([]SettlementView, 0, len(settlements))
		for _, settlement := range settlements {
			views = append(views, toSettlementView(settlement))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// SettlementInsurersUseCase lists the active mutuales, and whether each one can be settled.
type SettlementInsurersUseCase struct {
	settlements port.SettlementRepository
	scope       port.TenantScope
}

// NewSettlementInsurersUseCase wires the use case.
func NewSettlementInsurersUseCase(settlements port.SettlementRepository, scope port.TenantScope) *SettlementInsurersUseCase {
	return &SettlementInsurersUseCase{settlements: settlements, scope: scope}
}

// Execute returns the active mutuales of the practice, by name.
func (uc *SettlementInsurersUseCase) Execute(ctx context.Context, tenantID string) ([]SettlementInsurerView, error) {
	var views []SettlementInsurerView
	err := uc.scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		insurers, err := uc.settlements.Insurers(ctx)
		if err != nil {
			return err
		}
		views = make([]SettlementInsurerView, 0, len(insurers))
		for _, insurer := range insurers {
			views = append(views, toSettlementInsurerView(insurer))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}
