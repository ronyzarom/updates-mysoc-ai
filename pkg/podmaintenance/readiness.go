package podmaintenance

import (
	"context"
	"errors"
	"time"
)

const ReadinessProtocol = "pod-maintenance-readiness-v1"

type ReadinessRequest struct {
	Protocol  string `json:"protocol"`
	PodID     string `json:"pod_id"`
	NodeID    string `json:"node_id"`
	UpdaterID string `json:"updater_id"`
}
type ReadinessResponse struct {
	Protocol         string    `json:"protocol"`
	PodID            string    `json:"pod_id"`
	NodeID           string    `json:"node_id"`
	UpdaterID        string    `json:"updater_id"`
	Capabilities     []string  `json:"capabilities"`
	Ready            bool      `json:"ready"`
	ObserverVerified bool      `json:"observer_verified"`
	CredentialsReady bool      `json:"credentials_ready"`
	LifecycleReady   bool      `json:"lifecycle_ready"`
	IssuedAt         time.Time `json:"issued_at"`
	ValidUntil       time.Time `json:"valid_until"`
}

func (r ReadinessResponse) Validate(q ReadinessRequest, now time.Time) error {
	if q.Protocol != ReadinessProtocol || q.PodID == "" || (q.NodeID != "1" && q.NodeID != "2") || q.UpdaterID == "" || r.Protocol != q.Protocol || r.PodID != q.PodID || r.NodeID != q.NodeID || r.UpdaterID != q.UpdaterID {
		return errors.New("readiness identity mismatch")
	}
	if !r.Ready || !r.ObserverVerified || !r.CredentialsReady || !r.LifecycleReady {
		return errors.New("pod lifecycle/credentials/observer not ready")
	}
	if now.Before(r.IssuedAt) || !now.Before(r.ValidUntil) || !r.ValidUntil.After(r.IssuedAt) || r.ValidUntil.Sub(r.IssuedAt) > time.Minute {
		return errors.New("readiness proof not fresh")
	}
	seen := map[string]bool{}
	for _, c := range r.Capabilities {
		if (c != Protocol && c != AckProtocol && c != RecoveryProtocol) || seen[c] {
			return errors.New("invalid readiness capability")
		}
		seen[c] = true
	}
	if !seen[Protocol] && !seen[AckProtocol] {
		return errors.New("maintenance protocol readiness required")
	}
	return nil
}
func (a CommandAdapter) Readiness(ctx context.Context, q ReadinessRequest) (ReadinessResponse, error) {
	var r ReadinessResponse
	e := a.invoke(ctx, "readiness", q, &r)
	if e != nil {
		return r, e
	}
	return r, r.Validate(q, time.Now().UTC())
}
