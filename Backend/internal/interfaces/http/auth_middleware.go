package http

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// Identity is the authenticated caller, resolved from the access token.
//
// It is what the middleware puts in the context, and the only thing later layers
// may trust about "who is calling". Authorization is a separate question: T15 owns
// the permission matrix, so nothing here decides what the identity may do.
type Identity struct {
	UserID   string
	TenantID string
	Role     string
}

// identityContextKey is unexported so a handler cannot forge an identity by
// putting a value in the context itself.
type identityContextKey struct{}

// IdentityFrom returns the identity the authentication middleware established.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	return identity, ok
}

// authenticate verifies the bearer token and stores the identity in the request
// context. It never consults the database: the whole point of a signed access
// token is that the answer travels with the request.
func authenticate(api huma.API, tokens *authn.AccessTokens) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token, ok := bearerToken(ctx.Header("Authorization"))
		if !ok {
			_ = huma.WriteErr(api, ctx, 401, "Missing or malformed Authorization header")
			return
		}

		claims, err := tokens.Verify(token)
		if err != nil {
			// One message for expired, tampered and foreign tokens: the client acts
			// the same way in all three cases, and the difference is a probing aid.
			_ = huma.WriteErr(api, ctx, 401, "The access token is not valid")
			return
		}

		next(huma.WithValue(ctx, identityContextKey{}, Identity{
			UserID:   claims.Subject,
			TenantID: claims.TenantID,
			Role:     claims.Role,
		}))
	}
}

// bearerToken extracts the credential from an Authorization header.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
