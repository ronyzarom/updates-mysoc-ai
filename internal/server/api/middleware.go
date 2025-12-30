package api

import (
	"net/http"
)

// adminAuth middleware checks for admin API key
func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if no admin key is configured
		if s.config.Server.APIKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = r.URL.Query().Get("api_key")
		}

		if apiKey != s.config.Server.APIKey {
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}

		next.ServeHTTP(w, r)
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

