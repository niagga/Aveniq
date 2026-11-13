package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	clinicalapp "github.com/niagga/aveniq/backend/internal/application/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// clinicalRecordBody is the frame of a treatment. Every field is optional: a
// professional may open the record before the first session.
type clinicalRecordBody struct {
	ChiefComplaint string `json:"chiefComplaint,omitempty" maxLength:"4000" doc:"Why the person came."`
	Background     string `json:"background,omitempty" maxLength:"8000" doc:"What is known: history, context, antecedents."`
	Objectives     string `json:"objectives,omitempty" maxLength:"4000" doc:"What the treatment is aiming at."`
	UpdatedAt      string `json:"updatedAt,omitempty" format:"date-time" doc:"When the frame was last written."`
}

// clinicalNoteBody is one encounter. There is no author field on the way in: the
// author is whoever the token proves, and the response echoes it back.
type clinicalNoteBody struct {
	ID            string `json:"id,omitempty" format:"uuid"`
	PatientID     string `json:"patientId,omitempty" format:"uuid"`
	AuthorID      string `json:"authorId,omitempty" format:"uuid" doc:"The professional who wrote the note, taken from the token."`
	EncounteredAt string `json:"encounteredAt" format:"date-time" doc:"When the session happened, not when the row was written."`
	Content       string `json:"content" minLength:"1" maxLength:"20000"`
}

type listClinicalNotesOutput struct {
	Body struct {
		Items []clinicalNoteBody `json:"items"`
	}
}

// ─── handlers ────────────────────────────────────────────────────────────────

type clinicalHandlers struct {
	getRecord  *clinicalapp.GetRecordUseCase
	saveRecord *clinicalapp.SaveRecordUseCase
	listNotes  *clinicalapp.ListNotesUseCase
	addNote    *clinicalapp.AddNoteUseCase
	logger     *slog.Logger
}

// registerClinicalRoutes declares the clinical endpoints.
//
// They live under the patient they belong to. The HTTP guard is the clinical
// permission (own or any). Which patient a professional may treat is T38.
func registerClinicalRoutes(api huma.API, deps Deps) {
	handlers := &clinicalHandlers{
		getRecord:  deps.GetClinicalRecord,
		saveRecord: deps.SaveClinicalRecord,
		listNotes:  deps.ListClinicalNotes,
		addNote:    deps.AddClinicalNote,
		logger:     deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermClinicalReadOwn, iam.PermClinicalReadAny)
	writeGuard := permissionGuard(api, deps.AccessTokens, iam.PermClinicalWriteOwn)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "get-clinical-record",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/patients/{id}/clinical-record",
		Summary:       "Read the clinical record of a patient",
		Description:   "Returns the frame of the treatment. Reading clinical data is recorded in the audit trail in the same transaction.",
		Tags:          []string{"clinical"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.getRecordHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "save-clinical-record",
		Method:        http.MethodPut,
		Path:          APIPrefix + "/patients/{id}/clinical-record",
		Summary:       "Write the clinical record of a patient",
		Description:   "Creates the frame on first save and replaces it afterwards. One record per patient.",
		Tags:          []string{"clinical"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.saveRecordHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "list-clinical-notes",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/patients/{id}/clinical-notes",
		Summary:       "List the encounters of a patient",
		Description:   "Returns one page of clinical notes, most recent encounter first. The read is recorded in the audit trail.",
		Tags:          []string{"clinical"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listNotesHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "add-clinical-note",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/patients/{id}/clinical-notes",
		Summary:       "Append an encounter",
		Description:   "Notes are append-only: a correction is a new note. The author is the caller, taken from the token.",
		Tags:          []string{"clinical"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.addNoteHandler)
}

func (h *clinicalHandlers) getRecordHandler(ctx context.Context, in *patientIDInput) (*clinicalRecordOutput, error) {
	who := identityOf(ctx)
	view, err := h.getRecord.Execute(ctx, clinicalapp.RecordInput{
		TenantID:  who.TenantID,
		ActorID:   who.UserID,
		Role:      who.Role,
		PatientID: in.ID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &clinicalRecordOutput{}
	out.Body = clinicalRecordBody{
		ChiefComplaint: view.ChiefComplaint,
		Background:     view.Background,
		Objectives:     view.Objectives,
		UpdatedAt:      view.UpdatedAt,
	}
	return out, nil
}

func (h *clinicalHandlers) saveRecordHandler(ctx context.Context, in *saveRecordInput) (*clinicalRecordOutput, error) {
	who := identityOf(ctx)
	view, err := h.saveRecord.Execute(ctx, clinicalapp.RecordDraftInput{
		TenantID:       who.TenantID,
		ActorID:        who.UserID,
		Role:           who.Role,
		PatientID:      in.ID,
		ChiefComplaint: in.Body.ChiefComplaint,
		Background:     in.Body.Background,
		Objectives:     in.Body.Objectives,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &clinicalRecordOutput{}
	out.Body = clinicalRecordBody{
		ChiefComplaint: view.ChiefComplaint,
		Background:     view.Background,
		Objectives:     view.Objectives,
		UpdatedAt:      view.UpdatedAt,
	}
	return out, nil
}

func (h *clinicalHandlers) listNotesHandler(ctx context.Context, in *listClinicalNotesInput) (*listClinicalNotesOutput, error) {
	who := identityOf(ctx)
	views, err := h.listNotes.Execute(ctx, clinicalapp.ListNotesInput{
		TenantID:  who.TenantID,
		ActorID:   who.UserID,
		Role:      who.Role,
		PatientID: in.ID,
		Limit:     in.Limit,
		Offset:    in.Offset,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listClinicalNotesOutput{}
	out.Body.Items = make([]clinicalNoteBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, clinicalNoteBody{
			ID:            view.ID,
			PatientID:     view.PatientID,
			AuthorID:      view.AuthorID,
			EncounteredAt: view.EncounteredAt,
			Content:       view.Content,
		})
	}
	return out, nil
}

func (h *clinicalHandlers) addNoteHandler(ctx context.Context, in *addClinicalNoteInput) (*clinicalNoteOutput, error) {
	who := identityOf(ctx)

	encounteredAt, err := time.Parse(time.RFC3339, in.Body.EncounteredAt)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("encounteredAt must be an RFC 3339 timestamp")
	}

	view, err := h.addNote.Execute(ctx, clinicalapp.AddNoteInput{
		TenantID:      who.TenantID,
		ActorID:       who.UserID,
		Role:          who.Role,
		PatientID:     in.ID,
		EncounteredAt: encounteredAt,
		Content:       in.Body.Content,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &clinicalNoteOutput{}
	out.Body = clinicalNoteBody{
		ID:            view.ID,
		PatientID:     view.PatientID,
		AuthorID:      view.AuthorID,
		EncounteredAt: view.EncounteredAt,
		Content:       view.Content,
	}
	return out, nil
}

// identityOf reads the verified identity. The routes are behind the guard, so an empty
// result is a wiring defect; filling empty strings then fails at the use case with a
// validation error instead of writing into a zero UUID.
func identityOf(ctx context.Context) Identity {
	identity, _ := IdentityFrom(ctx)
	return identity
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type clinicalRecordOutput struct {
	Body clinicalRecordBody
}

type clinicalNoteOutput struct {
	Body clinicalNoteBody
}

type saveRecordInput struct {
	ID   string `path:"id" format:"uuid" doc:"Patient identifier."`
	Body clinicalRecordBody
}

type listClinicalNotesInput struct {
	ID     string `path:"id" format:"uuid" doc:"Patient identifier."`
	Limit  int32  `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset int32  `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type addClinicalNoteInput struct {
	ID   string `path:"id" format:"uuid" doc:"Patient identifier."`
	Body clinicalNoteBody
}
