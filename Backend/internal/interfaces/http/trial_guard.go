package http

import (
	"encoding/json"
	"net/http"
	"strings"

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// trialExemptPrefixes are the routes that are not a practice's own writes: sign-in and
// session routes must keep working for an expired practice (so its owner can read it).
var trialExemptPrefixes = []string{
	APIPrefix + "/auth/",
}

// trialExemptPaths are public routes that do not belong to the caller's practice: the
// signup creates a new practice, and the invitation preview and acceptance are answered
// to someone who may not hold a session at all. Creating an invitation is NOT here: it is
// the owner's own write, and an expired practice must not send them.
var trialExemptPaths = map[string]bool{
	APIPrefix + "/signup":              true,
	APIPrefix + "/invitations/preview": true,
	APIPrefix + "/invitations/accept":  true,
}

// trialGuard refuses writes from an authenticated caller whose practice has an expired
// trial (L58). Reads pass. Without a valid bearer token the request passes through, and
// the route answers 401 as it always did: the guard only decides for a real practice.
//
// It runs before the routes, so a refused write never reaches validation or a use case.
// If the trial cannot be read, the write is refused (503): a practice the guard cannot
// place is not allowed to write.
func trialGuard(tokens *authn.AccessTokens, expired *tenantapp.TrialExpiredUseCase) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isWrite(r.Method) || isTrialExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := tokens.Verify(token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// The guard is often the first place the request's tenant becomes known
			// in the chain above huma: report it, so the completion line names the
			// practice even when the guard itself refuses the write with 403 or
			// its read fails with 503.
			recordRequestIdentity(r.Context(), Identity{UserID: claims.Subject, TenantID: claims.TenantID})

			ended, err := expired.Execute(r.Context(), claims.TenantID)
			if err != nil {
				writeProblemJSON(w, http.StatusServiceUnavailable, "trial_unavailable",
					"The practice's trial could not be read. Try again in a moment.")
				return
			}
			if ended {
				writeProblemJSON(w, http.StatusForbidden, trialExpiredCode, trialExpiredDetail)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// The wire form of the refusal. The guard writes it for a signed-in caller and the
// invitation routes return it as an error, so both answer the same body.
const (
	trialExpiredCode   = "trial_expired"
	trialExpiredDetail = "The trial of this practice has ended. It can be read, not changed."
)

// trialExpiredError is the 403 that carries the stable `code` the client acts on.
// huma's own error model has no such field, so the refusal is a type of its own.
type trialExpiredError struct{}

func (trialExpiredError) Error() string  { return trialExpiredDetail }
func (trialExpiredError) GetStatus() int { return http.StatusForbidden }

// ContentType makes huma answer application/problem+json, like its own errors.
func (trialExpiredError) ContentType(string) string { return "application/problem+json" }

func (trialExpiredError) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"title":  http.StatusText(http.StatusForbidden),
		"status": http.StatusForbidden,
		"detail": trialExpiredDetail,
		"code":   trialExpiredCode,
	})
}

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isTrialExempt(path string) bool {
	if trialExemptPaths[path] {
		return true
	}
	for _, prefix := range trialExemptPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// writeProblemJSON answers in the problem shape the API uses, with a stable code the
// client can act on.
func writeProblemJSON(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = jsonEncoder(w).Encode(map[string]any{
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
		"code":   code,
	})
}
