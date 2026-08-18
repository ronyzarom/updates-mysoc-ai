package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/security"
)

// CreateAPIKeyRequest is the admin request to mint a managed API key.
type CreateAPIKeyRequest struct {
	Name string `json:"name"`
	// Scope defaults to "releases" (least privilege) when empty.
	Scope string `json:"scope,omitempty"`
	// ExpiresInDays optionally sets an expiry; 0 or omitted means never expires.
	ExpiresInDays int `json:"expires_in_days,omitempty"`
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.apiKeys.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []security.APIKey{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req CreateAPIKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, req.ExpiresInDays)
		expiresAt = &t
	}

	var createdBy string
	if user := auth.GetUserFromContext(r.Context()); user != nil {
		createdBy = user.Email
	}

	fullKey, meta, err := s.apiKeys.Create(r.Context(), req.Name, req.Scope, createdBy, expiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The plaintext key is returned exactly once. The client must surface it to
	// the operator immediately; it is not recoverable afterward.
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"api_key": fullKey,
		"key":     meta,
		"warning": "Store this key now — it is shown only once and cannot be retrieved later.",
	})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.apiKeys.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, security.ErrAPIKeyNotFound) {
			writeError(w, http.StatusNotFound, "api key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
