package http

import (
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/niagga/aveniq/backend/internal/application/port"
)

// ratePolicy is one bucket of public traffic: how many attempts a client may make
// in a window. Each route family has its own bucket, so hammering one cannot use up
// another's allowance.
type ratePolicy struct {
	name   string
	limit  int64
	window time.Duration
}

var (
	// signupPolicy: a practice is registered rarely. Five attempts an hour per client
	// leaves room for a typo-driven retry and stops a scripted sweep of slugs.
	signupPolicy = ratePolicy{name: "signup", limit: 5, window: time.Hour}

	// invitationLinkPolicy covers preview and accept. Thirty attempts in ten minutes
	// is generous for someone opening a link, and too slow for guessing secrets.
	invitationLinkPolicy = ratePolicy{name: "invitation-link", limit: 30, window: 10 * time.Minute}
)

// rateLimited is a huma middleware that counts the attempt before the handler runs.
//
// It fails closed: if the counter store cannot answer, the request is refused with
// 503 instead of passed through. A public door that opens whenever Redis blinks is
// no limit at all, and these routes are exactly the ones an attacker probes.
//
// The client is identified by the address the request arrived from. The router's
// RealIP middleware rewrites it from forwarding headers, which is only safe behind a
// proxy that overwrites those headers. That is a deployment condition recorded on
// the T36 board entry, not something this code can enforce.
func rateLimited(api huma.API, limiter port.RateLimiter, logger *slog.Logger, policy ratePolicy) huma.Middlewares {
	return huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
		key := policy.name + ":" + clientAddress(ctx.RemoteAddr())

		count, err := limiter.Hit(ctx.Context(), key, policy.window)
		if err != nil {
			logger.ErrorContext(ctx.Context(), "rate limit store unavailable", slog.String("policy", policy.name), slog.String("error", err.Error()))
			_ = huma.WriteErr(api, ctx, 503, "The service is temporarily unavailable")
			return
		}
		if count > policy.limit {
			ctx.SetHeader("Retry-After", strconv.Itoa(int(policy.window.Seconds())))
			_ = huma.WriteErr(api, ctx, 429, "Too many attempts; try again later")
			return
		}

		next(ctx)
	}}
}

// clientAddress strips the port from a remote address so every connection from one
// host shares a bucket. A value without a port is returned unchanged.
func clientAddress(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}
