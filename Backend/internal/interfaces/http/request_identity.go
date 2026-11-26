package http

import (
	"context"
	"sync"
)

// ─── per-request identity holder ─────────────────────────────────────────────
//
// The completion log line is written by `requestLogger`, which sits before the
// route middlewares: authentication and huma derive fresh contexts for what they
// hand downstream, so the values they put in the context are not visible to the
// logger once `next.ServeHTTP` has returned.
//
// The holder closes that gap by reference: the logger creates one per request,
// stores it in the request context, and every middleware that learns the caller's
// identity writes into it through the context it received. When the handler chain
// returns, the logger reads the same struct it created, without re-fetching a
// context value that authentication replaced.

// requestIdentity is what the middlewares learned about the caller during a
// request: the practice the token resolved the work to, and the user whose token
// it was. Identifiers only — never a token, an email or a request body.
type requestIdentity struct {
	TenantID string
	UserID   string
}

// requestIdentityKey is unexported so only this package can install a holder in
// a request context: a caller cannot attach values to a request it does not own.
type requestIdentityKey struct{}

// requestIdentityHolder is the per-request memory the completion log reads. It
// starts empty and grows as the middlewares establish the identity; a request
// that never authenticates leaves it empty, and the log names nothing. It dies
// with the request, so one request cannot see another's values.
type requestIdentityHolder struct {
	mu       sync.Mutex
	identity requestIdentity
}

// setTenant records the practice the request acts upon. An empty id is a
// non-event: a tenant_id that looks like "" is the lie the log must not tell.
func (h *requestIdentityHolder) setTenant(tenantID string) {
	if tenantID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.identity.TenantID = tenantID
}

// setUser records the user whose token the request carried.
func (h *requestIdentityHolder) setUser(userID string) {
	if userID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.identity.UserID = userID
}

// snapshot returns what was learned so far. It is read exactly once per request,
// after the handler chain has returned; the mutex keeps that read well defined
// even if a handler finished its work on another goroutine.
func (h *requestIdentityHolder) snapshot() requestIdentity {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.identity
}

// withRequestIdentityHolder installs the logger's holder in the request context.
func withRequestIdentityHolder(ctx context.Context, holder *requestIdentityHolder) context.Context {
	return context.WithValue(ctx, requestIdentityKey{}, holder)
}

// holderFrom returns the holder the request logger installed, or nil when it did
// not (a request answered by a mux this package never wrapped). Record readers
// fail safe: without a holder there is nothing to record and no panic either.
func holderFrom(ctx context.Context) *requestIdentityHolder {
	holder, _ := ctx.Value(requestIdentityKey{}).(*requestIdentityHolder)
	return holder
}

// recordRequestIdentity remembers what the caller's token established, for the
// completion log to report. It tolerates any context: one without a holder
// simply records nothing.
func recordRequestIdentity(ctx context.Context, identity Identity) {
	holder := holderFrom(ctx)
	if holder == nil {
		return
	}
	holder.setTenant(identity.TenantID)
	holder.setUser(identity.UserID)
}
