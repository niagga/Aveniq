// Package reportapp holds the saved-report use cases: the definitions of the questions a
// practice asks the billing report again and again (T60).
//
// A saved report stores no figures, so sharing one never shares data: whoever opens it asks the
// billing report with their own role, and a professional still gets only their own numbers.
//
// Everyone who reads reports (owner, admin, accountant, professional) sees every saved report of
// the practice and may save one. Changing or removing one belongs to its author and to whoever
// holds tenant:settings (owner and admin), so one person cannot quietly remove another's report.
package reportapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/report"
)

// SavedView is the read model the application exposes to delivery: flat and string-based, so
// transports never see domain types. CanManage says whether the viewer may change or remove it.
type SavedView struct {
	ID               string
	Name             string
	Period           string
	From             string
	To               string
	GroupBy          string
	ProfessionalID   string
	ProfessionalName string
	Coverage         string
	InsurerID        string
	InsurerName      string
	Modality         string
	CreatedBy        string
	CreatedByName    string
	CreatedAt        string
	UpdatedAt        string
	CanManage        bool
}

// Viewer is who asks: it decides what they may do with a saved report.
type Viewer struct {
	TenantID string
	UserID   string
	Role     string
}

// mayRead refuses a role that reads no report at all, even if the interface let it through.
func (v Viewer) mayRead() error {
	if !iam.AllowsAny(iam.RoleCode(v.Role), iam.PermReportsRead, iam.PermReportsReadOwn) {
		return report.ErrForbidden
	}
	return nil
}

// mayManage is the author, or whoever holds tenant:settings.
func (v Viewer) mayManage(saved report.SavedReport) bool {
	return saved.CreatedBy == v.UserID || iam.Allows(iam.RoleCode(v.Role), iam.PermTenantSettings)
}

func toView(viewer Viewer, item port.SavedReportItem) SavedView {
	saved := item.Report
	return SavedView{
		ID:               string(saved.ID),
		Name:             saved.Name,
		Period:           string(saved.Period),
		From:             saved.From,
		To:               saved.To,
		GroupBy:          string(saved.GroupBy),
		ProfessionalID:   saved.Filter.ProfessionalID,
		ProfessionalName: item.ProfessionalName,
		Coverage:         saved.Filter.Coverage,
		InsurerID:        saved.Filter.InsurerID,
		InsurerName:      item.InsurerName,
		Modality:         saved.Filter.Modality,
		CreatedBy:        saved.CreatedBy,
		CreatedByName:    item.CreatedByName,
		CreatedAt:        saved.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        saved.UpdatedAt.UTC().Format(time.RFC3339),
		CanManage:        viewer.mayManage(saved),
	}
}

// ListUseCase lists the saved reports of the practice.
type ListUseCase struct {
	repo  port.SavedReportRepository
	scope port.TenantScope
}

// NewListUseCase wires the use case.
func NewListUseCase(repo port.SavedReportRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{repo: repo, scope: scope}
}

// Execute returns the saved reports by name.
func (uc *ListUseCase) Execute(ctx context.Context, viewer Viewer) ([]SavedView, error) {
	if err := viewer.mayRead(); err != nil {
		return nil, err
	}
	var views []SavedView
	err := uc.scope.WithTenant(ctx, viewer.TenantID, func(ctx context.Context) error {
		items, err := uc.repo.List(ctx)
		if err != nil {
			return err
		}
		views = make([]SavedView, 0, len(items))
		for _, item := range items {
			views = append(views, toView(viewer, item))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// CreateUseCase saves a report.
type CreateUseCase struct {
	repo  port.SavedReportRepository
	scope port.TenantScope
	clock port.Clock
	ids   port.IDGenerator
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo port.SavedReportRepository, scope port.TenantScope, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, scope: scope, clock: clock, ids: ids}
}

// Execute validates the draft in the domain and stores it. The names (author, professional,
// mutual) are not re-read: the screen reloads the list after a write.
func (uc *CreateUseCase) Execute(ctx context.Context, viewer Viewer, draft report.SavedDraft) (SavedView, error) {
	if err := viewer.mayRead(); err != nil {
		return SavedView{}, err
	}
	var view SavedView
	err := uc.scope.WithTenant(ctx, viewer.TenantID, func(ctx context.Context) error {
		saved, err := report.NewSaved(report.SavedID(uc.ids.NewID()), viewer.TenantID, viewer.UserID, draft, uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Create(ctx, saved); err != nil {
			return err
		}
		view = toView(viewer, port.SavedReportItem{Report: saved})
		return nil
	})
	if err != nil {
		return SavedView{}, err
	}
	return view, nil
}

// UpdateUseCase changes what a saved report asks.
type UpdateUseCase struct {
	repo  port.SavedReportRepository
	scope port.TenantScope
	clock port.Clock
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo port.SavedReportRepository, scope port.TenantScope, clock port.Clock) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, scope: scope, clock: clock}
}

// Execute loads the report, checks that the viewer may change it, replaces its definition and
// persists it. A report the viewer may not change is refused before the draft is even read.
func (uc *UpdateUseCase) Execute(ctx context.Context, viewer Viewer, id string, draft report.SavedDraft) (SavedView, error) {
	if err := viewer.mayRead(); err != nil {
		return SavedView{}, err
	}
	var view SavedView
	err := uc.scope.WithTenant(ctx, viewer.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, report.SavedID(id))
		if err != nil {
			return err
		}
		if !viewer.mayManage(current) {
			return report.ErrForbidden
		}
		revised, err := current.Revise(draft, uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Update(ctx, revised); err != nil {
			return err
		}
		view = toView(viewer, port.SavedReportItem{Report: revised})
		return nil
	})
	if err != nil {
		return SavedView{}, err
	}
	return view, nil
}

// DeleteUseCase removes a saved report.
type DeleteUseCase struct {
	repo  port.SavedReportRepository
	scope port.TenantScope
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo port.SavedReportRepository, scope port.TenantScope) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, scope: scope}
}

// Execute removes the report if the viewer may. One the practice does not hold is not found. A
// report the viewer sees but may not remove is forbidden: saved reports are shared, so its
// existence is no secret from anyone who reads reports.
func (uc *DeleteUseCase) Execute(ctx context.Context, viewer Viewer, id string) error {
	if err := viewer.mayRead(); err != nil {
		return err
	}
	return uc.scope.WithTenant(ctx, viewer.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, report.SavedID(id))
		if err != nil {
			return err
		}
		if !viewer.mayManage(current) {
			return report.ErrForbidden
		}
		return uc.repo.Delete(ctx, report.SavedID(id))
	})
}
