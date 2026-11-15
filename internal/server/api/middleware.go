package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/security"
)

// ctxKeyAPIKey is the context key under which requireScope stashes the managed
// API key that authenticated the request (nil for the static master key or an
// admin JWT, which carry no per-tier binding). Handlers read it to enforce a
// key's ProductTier binding — see apiKeyFromContext.
type ctxKeyAPIKey struct{}

// apiKeyFromContext returns the managed API key that authenticated the request,
// or nil when the caller used the static master key, an admin JWT, or no key.
func apiKeyFromContext(ctx context.Context) *security.APIKey {
	key, _ := ctx.Value(ctxKeyAPIKey{}).(*security.APIKey)
	return key
}

// adminAuth authorizes the full admin surface. It is a convenience alias for
// requireScope(security.ScopeAdmin).
func (s *Server) adminAuth(next http.Handler) http.Handler {
	return s.requireScope(security.ScopeAdmin)(next)
}

// requireScope authorizes management endpoints for a given scope. It accepts,
// in order of precedence:
//
//   - the static admin API key (X-API-Key) — full-admin, satisfies any scope;
//   - a managed API key (X-API-Key) — must carry a scope that covers the
//     required scope (see security.ScopeAllows);
//   - an authenticated admin JWT (Authorization: Bearer) — full-admin.
//
// It fails closed:
//   - API keys are accepted only from the X-API-Key header, never from query
//     strings (which leak into logs and referrers).
//   - A presented-but-invalid X-API-Key is rejected outright; it does not fall
//     through to the JWT path.
//   - Unauthenticated callers receive 401; authenticated-but-insufficient
//     callers receive 403.
func (s *Server) requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// API-key path (X-API-Key only).
			if presented := r.Header.Get("X-API-Key"); presented != "" {
				// Static master key: full admin, satisfies every scope.
				if configuredKey := s.config.Server.APIKey; configuredKey != "" &&
					subtle.ConstantTimeCompare([]byte(presented), []byte(configuredKey)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
				// Managed keys: look up and enforce scope.
				if s.apiKeys != nil {
					key, err := s.apiKeys.Authenticate(r.Context(), presented)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "failed to verify api key")
						return
					}
					if key != nil {
						if security.ScopeAllows(key.Scope, scope) {
							// Stash the key so handlers can enforce a per-tier
							// binding (e.g. a self-service update-group key).
							ctx := context.WithValue(r.Context(), ctxKeyAPIKey{}, key)
							next.ServeHTTP(w, r.WithContext(ctx))
							return
						}
						writeError(w, http.StatusForbidden, "api key not authorized for this action")
						return
					}
				}
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}

			// JWT path: require a valid, active admin user.
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				writeError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			user, err := s.authService.GetUserFromToken(r.Context(), parts[1])
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			if !user.IsActive {
				writeError(w, http.StatusForbidden, "account is disabled")
				return
			}
			if user.Role != "admin" {
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			ctx := auth.SetUserInContext(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// instanceAuth middleware checks for valid instance API key.
func (s *Server) instanceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}

		// TODO: Validate instance API key against database.
		// For now, just check it's not empty.

		next.ServeHTTP(w, r)
	})
}
