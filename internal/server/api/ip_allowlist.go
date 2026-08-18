package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/security"
)

// enforceIPAllowed applies the allowlist-only IP control to a request on the
// updater data-plane channel. When enforcement is disabled it is a no-op and
// returns true. When enabled, it authorizes the request's source IP against the
// global entries plus entries scoped to instanceID (pass "" for instance-less
// endpoints such as artifact download). On denial it writes a 403 and returns
// false; callers must stop processing when it returns false.
func (s *Server) enforceIPAllowed(w http.ResponseWriter, r *http.Request, instanceID string) bool {
	if !s.config.Server.IPAllowlistEnforced {
		return true
	}

	clientIP := getClientIP(r)
	allowed, err := s.ipACL.IsAllowed(r.Context(), instanceID, clientIP)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to evaluate ip allowlist")
		return false
	}
	if !allowed {
		// Log the rejection for security auditing. The channel is intentionally
		// opaque to unknown sources: return a generic 403 without echoing the IP.
		log.Printf("[ip-allowlist] denied request path=%s instance_id=%q source_ip=%s",
			r.URL.Path, instanceID, clientIP)
		writeError(w, http.StatusForbidden, "source address not allowed")
		return false
	}
	return true
}

// CreateIPAllowlistRequest is the admin request to add an allowlist entry.
type CreateIPAllowlistRequest struct {
	InstanceID string `json:"instance_id,omitempty"` // empty = global entry
	CIDR       string `json:"cidr"`
	Note       string `json:"note,omitempty"`
}

func (s *Server) handleListIPAllowlist(w http.ResponseWriter, r *http.Request) {
	entries, err := s.ipACL.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []security.IPAllowlistEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enforced": s.config.Server.IPAllowlistEnforced,
		"entries":  entries,
	})
}

func (s *Server) handleCreateIPAllowlist(w http.ResponseWriter, r *http.Request) {
	var req CreateIPAllowlistRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CIDR == "" {
		writeError(w, http.StatusBadRequest, "cidr is required")
		return
	}

	entry, err := s.ipACL.Create(r.Context(), req.InstanceID, req.CIDR, req.Note)
	if err != nil {
		// Validation failures (bad CIDR) are client errors.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) handleDeleteIPAllowlist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.ipACL.Delete(r.Context(), id); err != nil {
		if errors.Is(err, security.ErrEntryNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
