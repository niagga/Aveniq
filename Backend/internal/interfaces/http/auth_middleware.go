package http

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
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
		identity, ok := resolveIdentity(api, ctx, tokens)
		if !ok {
			return
		}
		next(withIdentity(ctx, identity))
	}
}

// requireRoles authenticates and then refuses an identity whose role is not in the
// allowed set.
//
// The set is deliberately coarse (owner, admin, professional for clinical work)
// and it exists to be replaced: the per-domain permission matrix is E3, and a
// guard that pretends to be that matrix would be worse than an honest one. It
// runs on domain routes only — the tenant comes from the token, so this is also
// what makes `app.tenant_id` unforgeable.
func requireRoles(api huma.API, tokens *authn.AccessTokens, roles ...iam.RoleCode) func(huma.Context, func(huma.Context)) {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[string(role)] = struct{}{}
	}

	return func(ctx huma.Context, next func(huma.Context)) {
		identity, ok := resolveIdentity(api, ctx, tokens)
		if !ok {
			return
		}

		if _, permitted := allowed[identity.Role]; !permitted {
			// 403 and not 404: the caller is authenticated, so hiding the route would
			// only cost whoever debugs it. What the role cannot do is the useful answer.
			_ = huma.WriteErr(api, ctx, 403, "This role is not allowed to use this resource")
			return
		}

		next(withIdentity(ctx, identity))
	}
}

// resolveIdentity turns the Authorization header into an Identity, writing the
// refusal itself so every caller reports it the same way.
func resolveIdentity(api huma.API, ctx huma.Context, tokens *authn.AccessTokens) (Identity, bool) {
	token, ok := bearerToken(ctx.Header("Authorization"))
	if !ok {
		_ = huma.WriteErr(api, ctx, 401, "Missing or malformed Authorization header")
		return Identity{}, false
	}

	claims, err := tokens.Verify(token)
	if err != nil {
		// One message for expired, tampered and foreign tokens: the client acts the
		// same way in all three cases, and the difference is a probing aid.
		_ = huma.WriteErr(api, ctx, 401, "The access token is not valid")
		return Identity{}, false
	}

	return Identity{UserID: claims.Subject, TenantID: claims.TenantID, Role: claims.Role}, true
}

// withIdentity stores the identity under the unexported key. Handlers read it with
// IdentityFrom and cannot forge it, because the key is private to this package.
func withIdentity(ctx huma.Context, identity Identity) huma.Context {
	return huma.WithValue(ctx, identityContextKey{}, identity)
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
