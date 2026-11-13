// Package http is the delivery layer: it translates HTTP into application calls
// and back. It depends on the application layer only; it never touches SQL, the
// domain's persistence, or infrastructure directly.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	authapp "github.com/niagga/aveniq/backend/internal/application/auth"
	clinicalapp "github.com/niagga/aveniq/backend/internal/application/clinical"
	patientapp "github.com/niagga/aveniq/backend/internal/application/patient"
	scheduleapp "github.com/niagga/aveniq/backend/internal/application/schedule"
	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	userapp "github.com/niagga/aveniq/backend/internal/application/user"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

const (
	// APIPrefix versions the public surface so breaking changes can ship a v2
	// without breaking deployed clients.
	APIPrefix = "/api/v1"

	// securitySchemeBearer names the bearer scheme in the OpenAPI document, so the
	// generated TypeScript client knows which operations need a token.
	securitySchemeBearer = "bearerAuth"

	maxRequestBodyBytes = 1 << 20 // 1 MiB
	serverErrorStatus   = http.StatusInternalServerError
)

// ReadyCheck is one dependency the readiness endpoint must verify.
type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Deps holds everything the router needs. The composition root builds and
// injects them, which keeps this package free of wiring decisions.
type Deps struct {
	Logger             *slog.Logger
	ServiceVersion     string
	CreateTenant       *tenantapp.CreateUseCase
	GetTenantByID      *tenantapp.GetByIDUseCase
	Login              *authapp.LoginUseCase
	Refresh            *authapp.RefreshUseCase
	Logout             *authapp.LogoutUseCase
	CreatePatient      *patientapp.CreateUseCase
	UpdatePatient      *patientapp.UpdateUseCase
	DeactivatePatient  *patientapp.DeactivateUseCase
	ActivatePatient    *patientapp.ActivateUseCase
	GetPatient         *patientapp.GetUseCase
	GetClinicalRecord  *clinicalapp.GetRecordUseCase
	SaveClinicalRecord *clinicalapp.SaveRecordUseCase
	ListClinicalNotes  *clinicalapp.ListNotesUseCase
	AddClinicalNote    *clinicalapp.AddNoteUseCase
	ListPatients       *patientapp.ListUseCase
	CreateAppointment  *scheduleapp.CreateAppointmentUseCase
	ListAppointments   *scheduleapp.ListAppointmentsUseCase
	AttendAppointment  *scheduleapp.AttendAppointmentUseCase
	CancelAppointment  *scheduleapp.CancelAppointmentUseCase
	CreateSeries       *scheduleapp.CreateSeriesUseCase
	UpdateSeries       *scheduleapp.UpdateSeriesUseCase
	CancelSeries       *scheduleapp.CancelSeriesUseCase
	ListUsers          *userapp.ListUseCase
	GetUser            *userapp.GetUseCase
	ChangeUserRole     *userapp.ChangeRoleUseCase
	DeactivateUser     *userapp.DeactivateUseCase
	ActivateUser       *userapp.ActivateUseCase
	AccessTokens       *authn.AccessTokens
	RefreshCookie      RefreshCookieConfig
	ReadyChecks        []ReadyCheck
}

// NewRouter builds the HTTP handler: operational endpoints, then the versioned
// API described by an OpenAPI document.
func NewRouter(deps Deps) http.Handler {
	router := chi.NewRouter()

	// Order matters: request id first so every later log line can carry it, and
	// the recoverer close to the top so a handler panic still yields a response.
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(requestLogger(deps.Logger))
	router.Use(middleware.Recoverer)
	router.Use(securityHeaders)
	router.Use(bodyLimit)

	router.Get("/healthz", livenessHandler)
	router.Get("/readyz", readinessHandler(deps.ReadyChecks))

	// humachi registers operations directly on the chi router. Mounting it at the
	// root keeps every OpenAPI path identical to the real URL, so the generated
	// TypeScript client needs no prefix knowledge.
	api := humachi.New(router, apiConfig(deps.ServiceVersion))
	registerTenantRoutes(api, deps)
	registerAuthRoutes(api, deps)
	registerPatientRoutes(api, deps)
	registerClinicalRoutes(api, deps)
	registerScheduleRoutes(api, deps)
	registerUserRoutes(api, deps)

	return router
}

func apiConfig(version string) huma.Config {
	cfg := huma.DefaultConfig("Aveniq API", version)
	cfg.Info.Description = "Multi-tenant practice management API for psychology practices. " +
		"Every business record belongs to exactly one tenant; a tenant is resolved from the request host."
	cfg.Info.Contact = &huma.Contact{
		Name: "Aveniq",
	}
	// The generated document is the single source of truth for the TypeScript
	// client, so it must be served without authentication.
	// huma appends the requested format to this path, so it must not carry an
	// extension: "/openapi" serves /openapi.json and /openapi.yaml. The document
	// is the single source of truth for the generated TypeScript client.
	cfg.OpenAPIPath = "/openapi"
	cfg.DocsPath = "/docs"

	// The access token is a bearer credential signed with HMAC-SHA256; declaring it
	// here is what makes the document say which operations need one.
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		securitySchemeBearer: {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "HMAC-SHA256",
		},
	}
	return cfg
}

// ─── Operational endpoints ───────────────────────────────────────────────────

// livenessHandler answers "is the process running?". It must not touch
// dependencies: restarting a healthy process because a database blinked is worse
// than the blink itself.
func livenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readinessHandler answers "can this instance serve traffic?".
func readinessHandler(checks []ReadyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		failed := make([]string, 0, len(checks))
		for _, check := range checks {
			if err := check.Check(ctx); err != nil {
				failed = append(failed, check.Name)
			}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if len(failed) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			writeJSON(w, map[string]any{"status": "unavailable", "failed": failed})
			return
		}

		w.WriteHeader(http.StatusOK)
		writeJSON(w, map[string]any{"status": "ready"})
	}
}

func writeJSON(w http.ResponseWriter, payload map[string]any) {
	encoder := jsonEncoder(w)
	_ = encoder.Encode(payload)
}

// ─── Middleware ──────────────────────────────────────────────────────────────

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(wrapped, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.Status()),
				slog.Int("bytes", wrapped.BytesWritten()),
				slog.Duration("duration", time.Since(started)),
			)
		})
	}
}

// securityHeaders sets conservative defaults. The CSP is intentionally absent:
// the HTML is rendered by Next.js, and a wrong policy there is a debugging trap.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Error mapping ───────────────────────────────────────────────────────────

// problem turns a domain or application error into a transport error.
//
// Unknown failures are logged with their details and answered with a generic
// message: an internal error must never leak storage or schema information to a
// client.
func problem(ctx context.Context, logger *slog.Logger, err error) error {
	var validationErr *tenant.ValidationError
	var patientValidationErr *patient.ValidationError
	var clinicalValidationErr *clinical.ValidationError
	var scheduleValidationErr *schedule.ValidationError
	var iamValidationErr *iam.ValidationError
	var clientErr huma.StatusError

	switch {
	case errors.As(err, &clientErr):
		// Already a transport-level error produced deeper in this layer.
		return err
	case errors.As(err, &patientValidationErr):
		// 422 with the field the domain refused: a form that loses that detail turns
		// "the DNI is invalid" into "we could not save", and the phone rings.
		return huma.Error422UnprocessableEntity(patientValidationErr.Error())
	case errors.Is(err, patient.ErrDocumentInvalid):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, patient.ErrDocumentTaken):
		return huma.Error409Conflict("A patient with that document already exists in this practice")
	case errors.Is(err, patient.ErrNotFound):
		return huma.Error404NotFound("The requested patient does not exist")
	case errors.As(err, &clinicalValidationErr):
		return huma.Error422UnprocessableEntity(clinicalValidationErr.Error())
	case errors.Is(err, clinical.ErrNotFound):
		// A patient or record of another practice answers the same thing as one that
		// does not exist: confirming it exists is itself information.
		return huma.Error404NotFound("The requested clinical record does not exist")
	case errors.Is(err, clinical.ErrPatientNotVisible):
		return huma.Error404NotFound("The requested patient does not exist")
	case errors.As(err, &scheduleValidationErr):
		return huma.Error422UnprocessableEntity(scheduleValidationErr.Error())
	case errors.Is(err, schedule.ErrNotFound):
		return huma.Error404NotFound("The requested appointment does not exist")
	case errors.Is(err, schedule.ErrOverlap):
		return huma.Error409Conflict("That professional already has an appointment in that interval")
	case errors.Is(err, schedule.ErrInvalidTransition):
		return huma.Error409Conflict("That state transition is not allowed")
	case errors.Is(err, schedule.ErrPatientNotVisible), errors.Is(err, schedule.ErrProfessionalNotVisible):
		return huma.Error404NotFound("The requested appointment does not exist")
	case errors.Is(err, patient.ErrInvalidTransition):
		return huma.Error409Conflict("That state transition is not allowed")
	case errors.As(err, &iamValidationErr):
		return huma.Error422UnprocessableEntity(iamValidationErr.Error())
	case errors.Is(err, iam.ErrChangeNotAllowed):
		// The message is fixed on purpose: the rule that refused the change is
		// information about the other accounts, and the caller does not need it.
		return huma.Error403Forbidden("This change is not allowed for your role")
	case errors.Is(err, iam.ErrNotFound):
		return huma.Error404NotFound("The requested user does not exist")
	case errors.Is(err, iam.ErrInvalidTransition):
		return huma.Error409Conflict("That state transition is not allowed")
	case errors.Is(err, tenant.ErrNotFound):
		return huma.Error404NotFound("The requested tenant does not exist")
	case errors.Is(err, tenant.ErrSlugTaken):
		return huma.Error409Conflict("That slug is already taken by another tenant")
	case errors.Is(err, tenant.ErrInvalidTransition):
		return huma.Error409Conflict("That state transition is not allowed")
	case errors.As(err, &validationErr):
		return huma.Error422UnprocessableEntity(validationErr.Error())
	case errors.Is(err, authapp.ErrInvalidCredentials):
		return huma.Error401Unauthorized("The credentials are not valid")
	case errors.Is(err, authapp.ErrInvalidRefreshToken):
		return huma.Error401Unauthorized("The refresh token is not valid")
	case errors.Is(err, authapp.ErrRefreshReused):
		return huma.Error401Unauthorized("This refresh token was already used; the session chain was revoked")
	case errors.Is(err, authapp.ErrSessionExpired):
		return huma.Error401Unauthorized("The session expired; log in again")
	default:
		logger.ErrorContext(ctx, "unhandled error", slog.String("error", err.Error()))
		return huma.NewError(serverErrorStatus, "Unexpected error")
	}
}
