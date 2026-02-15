package api

import (
	"net/http"
	"strings"
)

// adminAuth middleware checks for admin API key OR valid JWT token with admin role
func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if no admin key is configured
		if s.config.Server.APIKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check for API key first
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey == s.config.Server.APIKey {
			next.ServeHTTP(w, r)
			return
		}

		// Check for JWT token (Bearer token)
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := s.authService.ValidateAccessToken(token)
			if err == nil && claims != nil {
				// Valid JWT - check if user has admin role
				if claims.Role == "admin" {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		writeError(w, http.StatusUnauthorized, "invalid or missing API key")
	})
}

// instanceAuth middleware checks for valid instance API key
func (s *Server) instanceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}

		// TODO: Validate instance API key against database
		// For now, just check it's not empty

		next.ServeHTTP(w, r)
	})
}
