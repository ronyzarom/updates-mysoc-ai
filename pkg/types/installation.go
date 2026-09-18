package types

import (
	"fmt"
	"regexp"
)

// InstallationIdentity is recorded by the installer and retained independently
// of failover roles. Older clients omit it; omission never means normal.
type InstallationIdentity struct {
	Kind   string `json:"kind"`
	PodID  string `json:"pod_id,omitempty"`
	NodeID string `json:"node_id,omitempty"`
}

var installationPodID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,100}$`)

func (i *InstallationIdentity) Validate() error {
	if i == nil {
		return nil
	}
	if (i.Kind == "normal" || i.Kind == "observer-unlinked") && i.PodID == "" && i.NodeID == "" {
		return nil
	}
	if i.Kind == "pod-node" && i.PodID == "" && (i.NodeID == "1" || i.NodeID == "2") {
		return nil
	}
	if i.Kind == "pod" && installationPodID.MatchString(i.PodID) && (i.NodeID == "1" || i.NodeID == "2" || i.NodeID == "witness") {
		return nil
	}
	return fmt.Errorf("invalid immutable installation identity")
}
