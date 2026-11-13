package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	patientapp "github.com/niagga/aveniq/backend/internal/application/patient"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// patientDocumentBody is the identity document as the client sees it.
//
// Number is what the person typed; the canonical form is internal (the aggregate
// keeps it for comparisons, and echoing it back would invite clients to send it).
type patientDocumentBody struct {
	Country string `json:"country" doc:"ISO 3166-1 alpha-2 country the document belongs to."`
	Kind    string `json:"kind" doc:"Document kind code for that country, for example dni or nie."`
	Number  string `json:"number" doc:"Document number as it appears on the card; separators are accepted."`
}

// patientTaxIDBody is the fiscal identifier, optional like the document.
type patientTaxIDBody struct {
	Kind   string `json:"kind" doc:"Tax id kind code for the country, for example cuit or nif."`
	Number string `json:"number" doc:"Tax id number as typed."`
}

type patientBody struct {
	ID                     string               `json:"id" format:"uuid" doc:"Patient identifier."`
	TenantID               string               `json:"tenantId" format:"uuid" doc:"Practice the patient belongs to."`
	Country                string               `json:"country" doc:"Country whose rules validate the document and fiscal data."`
	FullName               string               `json:"fullName" doc:"Name as the practice registered it."`
	BirthDate              string               `json:"birthDate,omitempty" doc:"Date of birth, YYYY-MM-DD."`
	Document               *patientDocumentBody `json:"document,omitempty"`
	Phone                  string               `json:"phone,omitempty"`
	Email                  string               `json:"email,omitempty"`
	EmergencyContactName   string               `json:"emergencyContactName,omitempty"`
	EmergencyContactPhone  string               `json:"emergencyContactPhone,omitempty"`
	TaxID                  *patientTaxIDBody    `json:"taxId,omitempty"`
	FiscalCondition        string               `json:"fiscalCondition,omitempty"`
	FiscalAddress          string               `json:"fiscalAddress,omitempty"`
	AdministrativeNotes    string               `json:"administrativeNotes,omitempty"`
	TreatingProfessionalID string               `json:"treatingProfessionalId,omitempty" doc:"The professional who treats the patient. Empty until one is assigned."`
	IsActive               bool                 `json:"isActive" doc:"False after a deactivation. Patients are never deleted."`
	CreatedAt              string               `json:"createdAt" format:"date-time"`
	UpdatedAt              string               `json:"updatedAt" format:"date-time"`
}

func toPatientBody(view patientapp.PatientView) patientBody {
	body := patientBody{
		ID:                     view.ID,
		TenantID:               view.TenantID,
		Country:                view.Country,
		FullName:               view.FullName,
		BirthDate:              view.BirthDate,
		Phone:                  view.Phone,
		Email:                  view.Email,
		EmergencyContactName:   view.EmergencyContactName,
		EmergencyContactPhone:  view.EmergencyContactPhone,
		FiscalCondition:        view.FiscalCondition,
		FiscalAddress:          view.FiscalAddress,
		AdministrativeNotes:    view.AdministrativeNotes,
		TreatingProfessionalID: view.TreatingProfessionalID,
		IsActive:               view.IsActive,
		CreatedAt:              view.CreatedAt,
		UpdatedAt:              view.UpdatedAt,
	}

	if view.Document != nil {
		body.Document = &patientDocumentBody{
			Country: view.Document.Country,
			Kind:    view.Document.Kind,
			Number:  view.Document.Number,
		}
	}
	if view.TaxID != nil {
		body.TaxID = &patientTaxIDBody{Kind: view.TaxID.Kind, Number: view.TaxID.Number}
	}
	return body
}

// patientProfileBody is what a client may send to create or edit a patient.
//
// Empty strings mean absent, which is how the application layer reads them: a JSON
// null and an omitted field would be two ways of saying the same thing, and the
// form has one.
type patientProfileBody struct {
	FullName              string `json:"fullName" minLength:"1" maxLength:"200" doc:"Name of the patient."`
	Country               string `json:"country,omitempty" pattern:"^[A-Z]{2}$" doc:"Optional ISO country. Defaults to the practice's."`
	BirthDate             string `json:"birthDate,omitempty" format:"date" doc:"Optional date of birth, YYYY-MM-DD."`
	DocumentKind          string `json:"documentKind,omitempty" doc:"Document kind for the country, for example dni or nie."`
	DocumentNumber        string `json:"documentNumber,omitempty" doc:"Document number as typed. Separators are accepted and normalized."`
	Phone                 string `json:"phone,omitempty"`
	Email                 string `json:"email,omitempty"`
	EmergencyContactName  string `json:"emergencyContactName,omitempty"`
	EmergencyContactPhone string `json:"emergencyContactPhone,omitempty"`
	TaxIDKind             string `json:"taxIdKind,omitempty"`
	TaxIDNumber           string `json:"taxIdNumber,omitempty"`
	FiscalCondition       string `json:"fiscalCondition,omitempty"`
	FiscalAddress         string `json:"fiscalAddress,omitempty"`
	AdministrativeNotes   string `json:"administrativeNotes,omitempty"`
}

// profile converts the wire body into the application input, parsing the date.
func (b patientProfileBody) profile() (patientapp.ProfileInput, error) {
	profile := patientapp.ProfileInput{
		FullName:              b.FullName,
		Country:               b.Country,
		DocumentKind:          b.DocumentKind,
		DocumentNumber:        b.DocumentNumber,
		Phone:                 b.Phone,
		Email:                 b.Email,
		EmergencyContactName:  b.EmergencyContactName,
		EmergencyContactPhone: b.EmergencyContactPhone,
		TaxIDKind:             b.TaxIDKind,
		TaxIDNumber:           b.TaxIDNumber,
		FiscalCondition:       b.FiscalCondition,
		FiscalAddress:         b.FiscalAddress,
		AdministrativeNotes:   b.AdministrativeNotes,
	}

	if b.BirthDate != "" {
		parsed, err := time.Parse(time.DateOnly, b.BirthDate)
		if err != nil {
			return patientapp.ProfileInput{}, huma.Error422UnprocessableEntity("birthDate must be a date in YYYY-MM-DD form")
		}
		profile.BirthDate = &parsed
	}
	return profile, nil
}

// ─── handlers ────────────────────────────────────────────────────────────────

type patientHandlers struct {
	create     *patientapp.CreateUseCase
	update     *patientapp.UpdateUseCase
	deactivate *patientapp.DeactivateUseCase
	activate   *patientapp.ActivateUseCase
	get        *patientapp.GetUseCase
	list       *patientapp.ListUseCase
	logger     *slog.Logger
}

// registerPatientRoutes declares the patient endpoints.
//
// Every route is authenticated and scoped: the practice comes from the signed
// token (T19), never from the body or the URL. Identification (patients:read /
// patients:write) is shared inside the practice; the clinical record is a
// different permission (T33).
//
// There is no DELETE. Deactivating keeps the roster row readable, which is what
// the clinical record will reference.
func registerPatientRoutes(api huma.API, deps Deps) {
	handlers := &patientHandlers{
		create:     deps.CreatePatient,
		update:     deps.UpdatePatient,
		deactivate: deps.DeactivatePatient,
		activate:   deps.ActivatePatient,
		get:        deps.GetPatient,
		list:       deps.ListPatients,
		logger:     deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermPatientsRead)
	writeGuard := permissionGuard(api, deps.AccessTokens, iam.PermPatientsWrite)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "create-patient",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/patients",
		Summary:       "Register a patient",
		Description:   "Creates a patient. The document and fiscal data are validated against the rules of the patient's country; the country defaults to the practice's.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.createPatient)

	huma.Register(api, huma.Operation{
		OperationID:   "list-patients",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/patients",
		Summary:       "List patients",
		Description:   "Returns one page of patients ordered by name. The search matches a fragment of the name or the exact normalized document.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listPatients)

	huma.Register(api, huma.Operation{
		OperationID:   "get-patient",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/patients/{id}",
		Summary:       "Get a patient",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.getPatient)

	huma.Register(api, huma.Operation{
		OperationID:   "update-patient",
		Method:        http.MethodPatch,
		Path:          APIPrefix + "/patients/{id}",
		Summary:       "Edit a patient",
		Description:   "Replaces the editable fields of a patient. Identity, creation time and the active flag are not editable here.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.updatePatient)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivate-patient",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/patients/{id}/deactivate",
		Summary:       "Deactivate a patient",
		Description:   "Stops the patient from appearing in new appointments without losing the row or its clinical record.",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.deactivatePatient)

	huma.Register(api, huma.Operation{
		OperationID:   "activate-patient",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/patients/{id}/activate",
		Summary:       "Reactivate a patient",
		Tags:          []string{"patients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.activatePatient)
}

func (h *patientHandlers) createPatient(ctx context.Context, in *createPatientInput) (*patientOutput, error) {
	who := identityOf(ctx)
	profile, err := in.Body.profile()
	if err != nil {
		return nil, err
	}

	view, err := h.create.Execute(ctx, patientapp.CreateInput{
		ProfileInput: profile,
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &patientOutput{Body: toPatientBody(view)}, nil
}

func (h *patientHandlers) listPatients(ctx context.Context, in *listPatientsInput) (*listPatientsOutput, error) {
	who := identityOf(ctx)
	views, err := h.list.Execute(ctx, patientapp.ListInput{
		TenantID:   who.TenantID,
		ActorID:    who.UserID,
		Role:       who.Role,
		Search:     in.Search,
		ActiveOnly: in.ActiveOnly,
		Limit:      in.Limit,
		Offset:     in.Offset,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listPatientsOutput{}
	out.Body.Items = make([]patientBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toPatientBody(view))
	}
	return out, nil
}

func (h *patientHandlers) getPatient(ctx context.Context, in *patientIDInput) (*patientOutput, error) {
	who := identityOf(ctx)
	view, err := h.get.Execute(ctx, patientapp.GetInput{TenantID: who.TenantID, ActorID: who.UserID, Role: who.Role, ID: in.ID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &patientOutput{Body: toPatientBody(view)}, nil
}

func (h *patientHandlers) updatePatient(ctx context.Context, in *updatePatientInput) (*patientOutput, error) {
	who := identityOf(ctx)
	profile, err := in.Body.profile()
	if err != nil {
		return nil, err
	}

	view, err := h.update.Execute(ctx, patientapp.UpdateInput{
		ProfileInput: profile,
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
		ID:           in.ID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &patientOutput{Body: toPatientBody(view)}, nil
}

func (h *patientHandlers) deactivatePatient(ctx context.Context, in *patientIDInput) (*patientOutput, error) {
	who := identityOf(ctx)
	view, err := h.deactivate.Execute(ctx, patientapp.DeactivateInput{TenantID: who.TenantID, ActorID: who.UserID, Role: who.Role, ID: in.ID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &patientOutput{Body: toPatientBody(view)}, nil
}

func (h *patientHandlers) activatePatient(ctx context.Context, in *patientIDInput) (*patientOutput, error) {
	who := identityOf(ctx)
	view, err := h.activate.Execute(ctx, patientapp.ActivateInput{TenantID: who.TenantID, ActorID: who.UserID, Role: who.Role, ID: in.ID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &patientOutput{Body: toPatientBody(view)}, nil
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type createPatientInput struct {
	Body patientProfileBody
}

type updatePatientInput struct {
	ID   string `path:"id" format:"uuid" doc:"Patient identifier."`
	Body patientProfileBody
}

type patientIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Patient identifier."`
}

type listPatientsInput struct {
	Search     string `query:"search" maxLength:"120" doc:"Name fragment or exact document number."`
	ActiveOnly bool   `query:"activeOnly" default:"true" doc:"Exclude deactivated patients."`
	Limit      int32  `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset     int32  `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type patientOutput struct {
	Body patientBody
}

type listPatientsOutput struct {
	Body struct {
		Items []patientBody `json:"items"`
	}
}
