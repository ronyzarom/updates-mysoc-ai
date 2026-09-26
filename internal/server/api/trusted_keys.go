package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/auth"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/releases"
)

// AddTrustedKeyRequest registers a public key issuers may seal releases with.
type AddTrustedKeyRequest struct {
	PublicKey string `json:"public_key"` // hex ed25519
	// Issuer limits the key to one issuer; empty trusts it for all issuers.
	Issuer string `json:"issuer,omitempty"`
	Label  string `json:"label,omitempty"`
}

func (s *Server) handleListTrustedKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.releaseService().ListTrustedKeys(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"keys":                      keys,
		"max_active_keys_per_scope": releases.MaxActiveKeysPerScope,
	})
}

func (s *Server) handleAddTrustedKey(w http.ResponseWriter, r *http.Request) {
	var req AddTrustedKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var createdBy string
	if user := auth.GetUserFromContext(r.Context()); user != nil {
		createdBy = user.Email
	}
	key, err := s.releaseService().AddTrustedKey(r.Context(), req.PublicKey, req.Issuer, req.Label, createdBy)
	switch {
	case errors.Is(err, releases.ErrTrustedKeyExists), errors.Is(err, releases.ErrTooManyActiveKeys):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, releases.ErrInvalidIssuer), errors.Is(err, releases.ErrInvalidPublicKey):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (s *Server) handleRetireTrustedKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.releaseService().RetireTrustedKey(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, releases.ErrTrustedKeyNotFound) {
		writeError(w, http.StatusNotFound, "trusted key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, key)
}
