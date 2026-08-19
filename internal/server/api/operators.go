package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/server/licensing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// operatorSummary is the list/detail shape returned to the dashboard. The
// platform key is always masked here; the full key is only ever returned by
// create and rotate-key responses.
type operatorSummary struct {
	types.Operator
	LicenseID     string         `json:"license_id,omitempty"`
	LicenseKey    string         `json:"license_key,omitempty"` // masked
	KeyExpiresAt  *time.Time     `json:"key_expires_at,omitempty"`
	KeyIssuedAt   *time.Time     `json:"key_issued_at,omitempty"`
	KeyActive     bool           `json:"key_active"`
	NodesByTier   map[string]int `json:"nodes_by_tier,omitempty"`
	TotalNodes    int            `json:"total_nodes"`
	LastHeartbeat *time.Time     `json:"last_heartbeat,omitempty"`
}

func (s *Server) handleListOperators(w http.ResponseWriter, r *http.Request) {
	licenseSvc := licensing.NewService(s.db)
	operators, err := licenseSvc.ListOperators(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fleet summary per operator, derived from instances attached to the
	// operator's licenses.
	instanceRepo := licensing.NewInstanceRepository(s.db)
	instances, err := instanceRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	licenses, err := licenseSvc.ListLicenses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	licenseOperator := map[string]string{} // license id -> operator ref
	for _, l := range licenses {
		if l.OperatorRef != "" {
			licenseOperator[l.ID] = l.OperatorRef
		}
	}

	type fleetAgg struct {
		byTier map[string]int
		total  int
		latest *time.Time
	}
	agg := map[string]*fleetAgg{}
	for i := range instances {
		inst := &instances[i]
		op := licenseOperator[inst.LicenseID]
		if op == "" {
			continue
		}
		a := agg[op]
		if a == nil {
			a = &fleetAgg{byTier: map[string]int{}}
			agg[op] = a
		}
		a.total++
		tier := inst.ProductTier
		if tier == "" {
			tier = "unknown"
		}
		a.byTier[tier]++
		if inst.LastHeartbeat != nil && (a.latest == nil || inst.LastHeartbeat.After(*a.latest)) {
			a.latest = inst.LastHeartbeat
		}
	}

	out := make([]operatorSummary, 0, len(operators))
	for _, op := range operators {
		summary := operatorSummary{Operator: op.Operator}
		if op.License != nil {
			summary.LicenseID = op.License.ID
			summary.LicenseKey = maskLicenseKey(op.License.LicenseKey)
			summary.KeyExpiresAt = &op.License.ExpiresAt
			summary.KeyIssuedAt = &op.License.IssuedAt
			summary.KeyActive = op.License.IsActive
		}
		if a := agg[op.ID]; a != nil {
			summary.NodesByTier = a.byTier
			summary.TotalNodes = a.total
			summary.LastHeartbeat = a.latest
		}
		out = append(out, summary)
	}
	writeJSON(w, http.StatusOK, out)
}

type createOperatorRequest struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// createOperatorResponse carries the full platform key exactly once.
type createOperatorResponse struct {
	Operator   types.Operator `json:"operator"`
	LicenseKey string         `json:"license_key"`
	ExpiresAt  time.Time      `json:"expires_at"`
}

func (s *Server) handleCreateOperator(w http.ResponseWriter, r *http.Request) {
	var req createOperatorRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	licenseSvc := licensing.NewService(s.db)
	result, err := licenseSvc.CreateOperator(r.Context(), req.ID, req.Name, req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, createOperatorResponse{
		Operator:   result.Operator,
		LicenseKey: result.License.LicenseKey,
		ExpiresAt:  result.License.ExpiresAt,
	})
}

func (s *Server) handleRotateOperatorKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	licenseSvc := licensing.NewService(s.db)
	license, err := licenseSvc.RotateOperatorKey(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"operator_id": id,
		"license_key": license.LicenseKey,
		"expires_at":  license.ExpiresAt,
	})
}

func (s *Server) handleUpdateOperator(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		IsActive *bool  `json:"is_active"`
		Name     string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	licenseSvc := licensing.NewService(s.db)
	if req.Name != "" {
		if _, err := s.db.Pool.Exec(r.Context(),
			`UPDATE operators SET name = $2, updated_at = NOW() WHERE id = $1`, id, strings.TrimSpace(req.Name)); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.IsActive != nil {
		if err := licenseSvc.SetOperatorActive(r.Context(), id, *req.IsActive); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	op, err := licenseSvc.GetOperator(r.Context(), id)
	if err != nil || op == nil {
		writeError(w, http.StatusNotFound, "operator not found")
		return
	}
	writeJSON(w, http.StatusOK, op)
}

// handleSigningKey publishes the release-signing public key (safe to expose;
// only the private seed must stay secret).
func (s *Server) handleSigningKey(w http.ResponseWriter, r *http.Request) {
	svc := s.releaseService()
	pub := svc.SigningPublicKeyHex()
	if pub == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"signing_enabled": false,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"signing_enabled": true,
		"algorithm":       "ed25519",
		"public_key":      pub,
	})
}
