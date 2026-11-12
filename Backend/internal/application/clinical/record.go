package clinicalapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// RecordInput names the patient whose frame is being read.
type RecordInput struct {
	TenantID  string
	ActorID   string
	PatientID string
}

// RecordDraftInput is what a caller may write. Empty strings mean "cleared": the
// frame is replaced, not merged, because a professional editing it expects what they
// see to be what gets saved.
type RecordDraftInput struct {
	TenantID       string
	ActorID        string
	PatientID      string
	ChiefComplaint string
	Background     string
	Objectives     string
}

// GetRecordUseCase reads the frame of a treatment and records the access.
type GetRecordUseCase struct {
	records port.ClinicalRepository
	scope   port.TenantScope
	audit   port.AuditLog
}

// NewGetRecordUseCase wires the use case.
func NewGetRecordUseCase(records port.ClinicalRepository, scope port.TenantScope, audit port.AuditLog) *GetRecordUseCase {
	return &GetRecordUseCase{records: records, scope: scope, audit: audit}
}

// Execute reads the record inside the tenant transaction and audits it there.
func (uc *GetRecordUseCase) Execute(ctx context.Context, in RecordInput) (RecordView, error) {
	who := caller{tenantID: in.TenantID, actorID: in.ActorID}

	var view RecordView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		record, err := uc.records.RecordByPatient(ctx, in.PatientID)
		if err != nil {
			return err
		}

		// Audit after reading and inside the same transaction: if this fails, the read
		// rolls back and the caller gets an error instead of unaudited data.
		if err := auditRead(ctx, uc.audit, who, in.PatientID, "clinical_record"); err != nil {
			return err
		}

		view = toRecordView(record)
		return nil
	})
	if err != nil {
		return RecordView{}, err
	}
	return view, nil
}

// SaveRecordUseCase writes the frame, creating it on first save.
type SaveRecordUseCase struct {
	records port.ClinicalRepository
	scope   port.TenantScope
	audit   port.AuditLog
	clock   port.Clock
	ids     port.IDGenerator
}

// NewSaveRecordUseCase wires the use case.
func NewSaveRecordUseCase(
	records port.ClinicalRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
) *SaveRecordUseCase {
	return &SaveRecordUseCase{records: records, scope: scope, audit: audit, clock: clock, ids: ids}
}

// Execute upserts the frame for a patient.
func (uc *SaveRecordUseCase) Execute(ctx context.Context, in RecordDraftInput) (RecordView, error) {
	who := caller{tenantID: in.TenantID, actorID: in.ActorID}

	// The aggregate is built before the transaction opens: an invalid draft must not
	// acquire a connection.
	record, err := clinical.NewRecord(
		clinical.RecordID(uc.ids.NewID()),
		tenant.ID(in.TenantID),
		in.PatientID,
		clinical.RecordDraft{
			ChiefComplaint: optional(in.ChiefComplaint),
			Background:     optional(in.Background),
			Objectives:     optional(in.Objectives),
		},
		uc.clock.Now(),
	)
	if err != nil {
		return RecordView{}, err
	}

	var view RecordView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		saved, err := uc.records.UpsertRecord(ctx, record)
		if err != nil {
			return err
		}
		if err := auditWrite(ctx, uc.audit, who, in.PatientID, "clinical_record"); err != nil {
			return err
		}
		view = toRecordView(saved)
		return nil
	})
	if err != nil {
		return RecordView{}, err
	}
	return view, nil
}

// optional turns an empty string into nil, so the aggregate sees one absent state.
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
