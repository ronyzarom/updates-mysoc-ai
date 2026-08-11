package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
)

// adminAuth authorizes management endpoints. It accepts either a configured
// admin API key (presented via the X-API-Key header) or an authenticated admin
// JWT (Bearer token).
//
// It fails closed:
//   - When no admin API key is configured, the API-key path is unavailable and
//     callers must present a valid admin JWT. It never bypasses authentication.
//   - API keys are only accepted from the X-API-Key header, never from query
//     strings (which leak into logs and referrers).
//   - Unauthenticated callers receive 401; authenticated non-admin callers
//     receive 403.
func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API-key path: only available when an admin key is configured.
		if configuredKey := s.config.Server.APIKey; configuredKey != "" {
			if presented := r.Header.Get("X-API-Key"); presented != "" {
				if subtle.ConstantTimeCompare([]byte(presented), []byte(configuredKey)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
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
