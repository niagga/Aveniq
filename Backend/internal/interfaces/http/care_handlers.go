package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// careHandlers serves the instance-level routes: who treats a patient, and which
// other professionals were granted its clinical record.
type careHandlers struct {
	assign      *careapp.AssignTreatingUseCase
	grant       *careapp.GrantClinicalUseCase
	listGrants  *careapp.ListGrantsUseCase
	revokeGrant *careapp.RevokeGrantUseCase
	logger      *slog.Logger
}

// registerCareRoutes declares the treating-professional and clinical-grant endpoints.
//
// The matrix gates the verb (patients:assign, clinical:grant); the instance rules in
// domain/care decide the patient. An owner holds every permission but still reads no
// clinical record unless it treats the patient or was granted it.
func registerCareRoutes(api huma.API, deps Deps) {
	handlers := &careHandlers{
		assign:      deps.AssignTreating,
		grant:       deps.GrantClinical,
		listGrants:  deps.ListGrants,
		revokeGrant: deps.RevokeGrant,
		logger:      deps.Logger,
	}

	assignGuard := permissionGuard(api, deps.AccessTokens, iam.PermPatientsAssign)
	grantGuard := permissionGuard(api, deps.AccessTokens, iam.PermClinicalGrant)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "assign-treating-professional",
		Method:        http.MethodPut,
		Path:          APIPrefix + "/patients/{id}/treating-professional",
		Summary:       "Assign the treating professional of a patient",
		Description:   "Sets who treats the patient. Only the owner and the admin assign; the professional must hold the professional role.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   assignGuard,
	}, handlers.assignTreating)

	huma.Register(api, huma.Operation{
		OperationID:   "list-clinical-grants",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/patients/{id}/clinical-grants",
		Summary:       "List the clinical grants of a patient",
		Description:   "Lists every grant over the patient, live and revoked. Only the treating professional and the owner see it.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   grantGuard,
	}, handlers.listClinicalGrants)

	huma.Register(api, huma.Operation{
		OperationID:   "grant-clinical-record",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/patients/{id}/clinical-grants",
		Summary:       "Grant a professional access to a patient's clinical record",
		Description:   "Gives another professional read access to the file and the clinical record. Only the treating professional grants. A grant does not allow booking or writing.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   grantGuard,
	}, handlers.grantClinicalRecord)

	huma.Register(api, huma.Operation{
		OperationID:   "revoke-clinical-grant",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/patients/{id}/clinical-grants/{grantId}/revoke",
		Summary:       "Revoke a clinical grant",
		Description:   "Withdraws a grant. The row stays as history; revoking twice is not an error.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   grantGuard,
	}, handlers.revokeClinicalGrant)
}

// ─── treating professional ───────────────────────────────────────────────────

type assignTreatingInput struct {
	ID   string `path:"id" format:"uuid" doc:"Patient identifier."`
	Body struct {
		ProfessionalID string `json:"professionalId" format:"uuid" doc:"The professional who will treat the patient."`
	}
}

type treatingBody struct {
	PatientID      string `json:"patientId" format:"uuid"`
	ProfessionalID string `json:"professionalId" format:"uuid"`
}

type treatingOutput struct {
	Body treatingBody
}

func (h *careHandlers) assignTreating(ctx context.Context, in *assignTreatingInput) (*treatingOutput, error) {
	who := identityOf(ctx)
	view, err := h.assign.Execute(ctx, careapp.AssignTreatingInput{
		TenantID:       who.TenantID,
		ActorID:        who.UserID,
		Role:           who.Role,
		PatientID:      in.ID,
		ProfessionalID: in.Body.ProfessionalID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &treatingOutput{Body: treatingBody{PatientID: view.PatientID, ProfessionalID: view.ProfessionalID}}, nil
}

// ─── clinical grants ─────────────────────────────────────────────────────────

type patientGrantsInput struct {
	ID string `path:"id" format:"uuid" doc:"Patient identifier."`
}

type grantBody struct {
	ID             string  `json:"id" format:"uuid"`
	PatientID      string  `json:"patientId" format:"uuid"`
	ProfessionalID string  `json:"professionalId" format:"uuid"`
	GrantedBy      string  `json:"grantedBy" format:"uuid"`
	GrantedAt      string  `json:"grantedAt" format:"date-time"`
	RevokedAt      *string `json:"revokedAt,omitempty" format:"date-time"`
}

type grantOutput struct {
	Body grantBody
}

type listGrantsOutput struct {
	Body struct {
		Items []grantBody `json:"items"`
	}
}

type grantInput struct {
	ID   string `path:"id" format:"uuid" doc:"Patient identifier."`
	Body struct {
		ProfessionalID string `json:"professionalId" format:"uuid" doc:"The professional who will read the clinical record."`
	}
}

type revokeGrantInput struct {
	ID      string `path:"id" format:"uuid" doc:"Patient identifier."`
	GrantID string `path:"grantId" format:"uuid" doc:"Grant identifier."`
}

func toGrantBody(view careapp.GrantView) grantBody {
	return grantBody{
		ID:             view.ID,
		PatientID:      view.PatientID,
		ProfessionalID: view.ProfessionalID,
		GrantedBy:      view.GrantedBy,
		GrantedAt:      view.GrantedAt,
		RevokedAt:      view.RevokedAt,
	}
}

func (h *careHandlers) listClinicalGrants(ctx context.Context, in *patientGrantsInput) (*listGrantsOutput, error) {
	who := identityOf(ctx)
	views, err := h.listGrants.Execute(ctx, careapp.ListGrantsInput{
		TenantID:  who.TenantID,
		ActorID:   who.UserID,
		Role:      who.Role,
		PatientID: in.ID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &listGrantsOutput{}
	out.Body.Items = make([]grantBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toGrantBody(view))
	}
	return out, nil
}

func (h *careHandlers) grantClinicalRecord(ctx context.Context, in *grantInput) (*grantOutput, error) {
	who := identityOf(ctx)
	view, err := h.grant.Execute(ctx, careapp.GrantInput{
		TenantID:       who.TenantID,
		ActorID:        who.UserID,
		Role:           who.Role,
		PatientID:      in.ID,
		ProfessionalID: in.Body.ProfessionalID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &grantOutput{Body: toGrantBody(view)}, nil
}

func (h *careHandlers) revokeClinicalGrant(ctx context.Context, in *revokeGrantInput) (*grantOutput, error) {
	who := identityOf(ctx)
	view, err := h.revokeGrant.Execute(ctx, careapp.RevokeGrantInput{
		TenantID: who.TenantID,
		ActorID:  who.UserID,
		Role:     who.Role,
		GrantID:  in.GrantID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &grantOutput{Body: toGrantBody(view)}, nil
}
