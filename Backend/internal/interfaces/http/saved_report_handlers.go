package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	reportapp "github.com/niagga/aveniq/backend/internal/application/reports"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/report"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// savedReportBody is a saved report: the definition of a question, never its figures.
type savedReportBody struct {
	ID               string `json:"id" format:"uuid"`
	Name             string `json:"name"`
	Period           string `json:"period" enum:"this-month,last-month,last-3-months,this-year,custom" doc:"A relative period keeps meaning what it says when the report is opened later."`
	From             string `json:"from,omitempty" doc:"First day of a custom period, YYYY-MM-DD."`
	To               string `json:"to,omitempty" doc:"Last day of a custom period, YYYY-MM-DD, included."`
	GroupBy          string `json:"groupBy" enum:"none,professional,coverage,insurer,modality,week,month,year"`
	ProfessionalID   string `json:"professionalId,omitempty" format:"uuid"`
	ProfessionalName string `json:"professionalName,omitempty"`
	Coverage         string `json:"coverage,omitempty" enum:"private,insurer"`
	InsurerID        string `json:"insurerId,omitempty" format:"uuid"`
	InsurerName      string `json:"insurerName,omitempty"`
	Modality         string `json:"modality,omitempty" enum:"online,presencial"`
	CreatedBy        string `json:"createdBy" format:"uuid"`
	CreatedByName    string `json:"createdByName,omitempty" doc:"Empty right after a write: the list names the author."`
	CreatedAt        string `json:"createdAt" format:"date-time"`
	UpdatedAt        string `json:"updatedAt" format:"date-time"`
	CanManage        bool   `json:"canManage" doc:"Whether the caller may change or remove it: its author, the owner and the admin."`
}

// savedReportWriteBody is what a person writes when saving or changing a report.
type savedReportWriteBody struct {
	Name           string `json:"name" minLength:"1" maxLength:"80" doc:"Unique inside the practice, ignoring case and outer spaces."`
	Period         string `json:"period" enum:"this-month,last-month,last-3-months,this-year,custom"`
	From           string `json:"from,omitempty" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$" doc:"Only for a custom period."`
	To             string `json:"to,omitempty" pattern:"^[0-9]{4}-[0-9]{2}-[0-9]{2}$" doc:"Only for a custom period."`
	GroupBy        string `json:"groupBy,omitempty" enum:"none,professional,coverage,insurer,modality,week,month,year" doc:"Default none."`
	ProfessionalID string `json:"professionalId,omitempty" format:"uuid"`
	Coverage       string `json:"coverage,omitempty" enum:"private,insurer"`
	InsurerID      string `json:"insurerId,omitempty" format:"uuid"`
	Modality       string `json:"modality,omitempty" enum:"online,presencial"`
}

type savedReportIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Saved report identifier."`
}

type savedReportCreateInput struct {
	Body savedReportWriteBody
}

type savedReportUpdateInput struct {
	ID   string `path:"id" format:"uuid" doc:"Saved report identifier."`
	Body savedReportWriteBody
}

type savedReportOutput struct {
	Body savedReportBody
}

type savedReportListOutput struct {
	Body struct {
		Items []savedReportBody `json:"items"`
	}
}

// ─── routes ──────────────────────────────────────────────────────────────────

type savedReportHandlers struct {
	list   *reportapp.ListUseCase
	create *reportapp.CreateUseCase
	update *reportapp.UpdateUseCase
	remove *reportapp.DeleteUseCase
	logger *slog.Logger
}

func registerSavedReportRoutes(api huma.API, deps Deps) {
	handlers := &savedReportHandlers{
		list:   deps.ListSavedReports,
		create: deps.CreateSavedReport,
		update: deps.UpdateSavedReport,
		remove: deps.DeleteSavedReport,
		logger: deps.Logger,
	}

	// Every reader of reports reads and saves. Who may change or remove a given report is the
	// use case's rule (its author, the owner and the admin), because it depends on the report.
	guard := permissionGuard(api, deps.AccessTokens, iam.PermReportsRead, iam.PermReportsReadOwn)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-saved-reports",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/reports/saved",
		Summary:       "List the saved reports of the practice",
		Description:   "The definitions the practice shares, by name. They hold no figures: opening one asks the billing report with the caller's own role.",
		Tags:          []string{"reports"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.listHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "create-saved-report",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/reports/saved",
		Summary:       "Save a report for the whole practice",
		Description:   "Stores a period, a cut and the filters under a name that is unique in the practice. Everyone who reads reports sees it.",
		Tags:          []string{"reports"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.createHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "update-saved-report",
		Method:        http.MethodPut,
		Path:          APIPrefix + "/reports/saved/{id}",
		Summary:       "Change a saved report",
		Description:   "Replaces what the report asks. Only its author, the owner and the admin may change it.",
		Tags:          []string{"reports"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.updateHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "delete-saved-report",
		Method:        http.MethodDelete,
		Path:          APIPrefix + "/reports/saved/{id}",
		Summary:       "Remove a saved report",
		Description:   "Only its author, the owner and the admin may remove it. Nothing else is affected: a saved report holds no figures.",
		Tags:          []string{"reports"},
		DefaultStatus: http.StatusNoContent,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.deleteHandler)
}

func viewerOf(ctx context.Context) reportapp.Viewer {
	who := identityOf(ctx)
	return reportapp.Viewer{TenantID: who.TenantID, UserID: who.UserID, Role: who.Role}
}

func draftOf(body savedReportWriteBody) report.SavedDraft {
	return report.SavedDraft{
		Name:    body.Name,
		Period:  body.Period,
		From:    body.From,
		To:      body.To,
		GroupBy: body.GroupBy,
		Filter: report.Filter{
			ProfessionalID: body.ProfessionalID,
			Coverage:       body.Coverage,
			InsurerID:      body.InsurerID,
			Modality:       body.Modality,
		},
	}
}

func (h *savedReportHandlers) listHandler(ctx context.Context, _ *struct{}) (*savedReportListOutput, error) {
	views, err := h.list.Execute(ctx, viewerOf(ctx))
	if err != nil {
		return nil, savedReportProblem(ctx, h.logger, err)
	}
	out := &savedReportListOutput{}
	out.Body.Items = make([]savedReportBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toSavedReportBody(view))
	}
	return out, nil
}

func (h *savedReportHandlers) createHandler(ctx context.Context, in *savedReportCreateInput) (*savedReportOutput, error) {
	view, err := h.create.Execute(ctx, viewerOf(ctx), draftOf(in.Body))
	if err != nil {
		return nil, savedReportProblem(ctx, h.logger, err)
	}
	return &savedReportOutput{Body: toSavedReportBody(view)}, nil
}

func (h *savedReportHandlers) updateHandler(ctx context.Context, in *savedReportUpdateInput) (*savedReportOutput, error) {
	view, err := h.update.Execute(ctx, viewerOf(ctx), in.ID, draftOf(in.Body))
	if err != nil {
		return nil, savedReportProblem(ctx, h.logger, err)
	}
	return &savedReportOutput{Body: toSavedReportBody(view)}, nil
}

func (h *savedReportHandlers) deleteHandler(ctx context.Context, in *savedReportIDInput) (*struct{}, error) {
	if err := h.remove.Execute(ctx, viewerOf(ctx), in.ID); err != nil {
		return nil, savedReportProblem(ctx, h.logger, err)
	}
	return nil, nil
}

// savedReportProblem answers the errors that are about a saved report, and leaves the rest to
// the shared mapping.
func savedReportProblem(ctx context.Context, logger *slog.Logger, err error) error {
	switch {
	case errors.Is(err, report.ErrSavedNotFound):
		return huma.Error404NotFound("The requested saved report does not exist")
	case errors.Is(err, report.ErrSavedNameTaken):
		return huma.Error409Conflict("A saved report with that name already exists in this practice")
	}
	return problem(ctx, logger, err)
}

// ─── mapping ─────────────────────────────────────────────────────────────────

func toSavedReportBody(view reportapp.SavedView) savedReportBody {
	return savedReportBody{
		ID:               view.ID,
		Name:             view.Name,
		Period:           view.Period,
		From:             view.From,
		To:               view.To,
		GroupBy:          view.GroupBy,
		ProfessionalID:   view.ProfessionalID,
		ProfessionalName: view.ProfessionalName,
		Coverage:         view.Coverage,
		InsurerID:        view.InsurerID,
		InsurerName:      view.InsurerName,
		Modality:         view.Modality,
		CreatedBy:        view.CreatedBy,
		CreatedByName:    view.CreatedByName,
		CreatedAt:        view.CreatedAt,
		UpdatedAt:        view.UpdatedAt,
		CanManage:        view.CanManage,
	}
}
