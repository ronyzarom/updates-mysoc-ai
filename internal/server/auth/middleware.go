package auth

import (
	"net/http"
	"strings"
)

// JWTMiddleware creates middleware that validates JWT tokens
func JWTMiddleware(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "authorization header is required")
				return
			}

			// Extract Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				writeError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			tokenString := parts[1]

			// Validate token
			user, err := service.GetUserFromToken(r.Context(), tokenString)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			// Check if user is active
			if !user.IsActive {
				writeError(w, http.StatusForbidden, "account is disabled")
				return
			}

			// Set user in context
			ctx := SetUserInContext(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole middleware checks if the user has the required role
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			// Check if user has required role
			hasRole := false
			for _, role := range roles {
				if user.Role == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalJWTMiddleware validates JWT if present but doesn't require it
func OptionalJWTMiddleware(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// No token, continue without user
				next.ServeHTTP(w, r)
				return
			}

			// Extract Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				// Invalid format, continue without user
				next.ServeHTTP(w, r)
				return
			}

			tokenString := parts[1]

			// Validate token
			user, err := service.GetUserFromToken(r.Context(), tokenString)
			if err != nil || !user.IsActive {
				// Invalid token, continue without user
				next.ServeHTTP(w, r)
				return
			}

			// Set user in context
			ctx := SetUserInContext(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
