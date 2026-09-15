package api

import (
	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
)

func (s *Server) handleInactiveChildren(w http.ResponseWriter, r *http.Request) {
	repo := licensing.NewInstanceRepository(s.db)
	parent, err := repo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, "failed to load parent")
		return
	}
	if parent == nil {
		writeError(w, 404, "parent not found")
		return
	}
	items, err := repo.InactiveChildren(r.Context(), parent.InstanceID)
	if err != nil {
		writeError(w, 500, "failed to load inactive children")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleCleanupChildren(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if decodeJSON(r, &req) != nil || len(req.IDs) == 0 || len(req.IDs) > 1000 {
		writeError(w, 400, "select between 1 and 1000 entries")
		return
	}
	for _, id := range req.IDs {
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, 400, "invalid entry id")
			return
		}
	}
	repo := licensing.NewInstanceRepository(s.db)
	parent, err := repo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, "failed to load parent")
		return
	}
	if parent == nil {
		writeError(w, 404, "parent not found")
		return
	}
	count, err := repo.CleanupChildren(r.Context(), parent.InstanceID, req.IDs)
	if err != nil {
		writeError(w, 500, "failed to remove inactive entries")
		return
	}
	writeJSON(w, 200, map[string]any{"removed": count})
}
