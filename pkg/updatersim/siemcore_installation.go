package updatersim

import (
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// ServerType records installer intent, never authority to activate a node.
// Current ACTIVE/STBY authority must still be measured by the pod adapter.
type SiemCoreInstallation struct {
	ServerType string `json:"server_type"`
	PodID      string `json:"pod_id,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
}

func serverTypeRole(kind string) (string, error) {
	switch kind {
	case "normal":
		return "normal", nil
	case "pod-active", "pod-stby":
		return "pod-node", nil
	case "pod-observer":
		return "witness", nil
	}
	return "", fmt.Errorf("unknown siemcore server type %q", kind)
}
func rememberSiemCoreInstallation(cfg *Config, state *State) error {
	p, ok := cfg.Product("siemcore")
	if !ok {
		return nil
	}
	if p.ServerType == "" && state.SiemCoreInstallation != nil {
		saved := state.SiemCoreInstallation
		p.ServerType = saved.ServerType
		p.PodID = saved.PodID
		p.NodeID = saved.NodeID
	}
	if cfg.Simulation.Filesystem.ObserverMaintenance != nil && p.ServerType != "pod-observer" {
		return fmt.Errorf("observer executor requires explicit observer installation")
	}
	if p.ServerType == "" {
		return nil
	} // Existing unspecified installations retain legacy behavior.
	role, e := serverTypeRole(p.ServerType)
	if e != nil {
		return e
	}
	if p.DeploymentRole != "" && p.DeploymentRole != role {
		return fmt.Errorf("siemcore deployment role conflicts with server type")
	}
	if role == "normal" {
		if p.PodID != "" || p.NodeID != "" || cfg.Simulation.Filesystem.PodMaintenance != nil {
			return fmt.Errorf("normal installation cannot have pod identity or executor")
		}
	} else {
		if p.PodID == "" || p.NodeID == "" {
			return fmt.Errorf("pod installation requires explicit pod and node identity")
		}
		if role == "pod-node" && p.NodeID != "1" && p.NodeID != "2" {
			return fmt.Errorf("pod data node must be 1 or 2")
		}
		if role == "witness" && p.NodeID != "witness" {
			return fmt.Errorf("pod observer node identity must be witness")
		}
		if m := cfg.Simulation.Filesystem.PodMaintenance; m != nil && (m.PodID != p.PodID || m.NodeID != p.NodeID || role != "pod-node") {
			return fmt.Errorf("pod executor identity conflicts with installation")
		}
	}
	next := SiemCoreInstallation{p.ServerType, p.PodID, p.NodeID}
	if old := state.SiemCoreInstallation; old != nil {
		oldRole, e := serverTypeRole(old.ServerType)
		if e != nil {
			return e
		}
		if oldRole != role || old.PodID != next.PodID || old.NodeID != next.NodeID {
			return fmt.Errorf("persisted installation identity cannot be silently reclassified")
		}
	}
	p.DeploymentRole = role
	state.SiemCoreInstallation = &next
	return SaveState(cfg.Simulation.StateFile, state)
}
func (s *Simulator) validateSiemCoreExecution() error {
	p, ok := s.config.Product("siemcore")
	if s.config.Simulation.Filesystem.ObserverMaintenance != nil && (!ok || p.ServerType != "pod-observer") {
		return fmt.Errorf("observer executor cannot handle normal or data nodes")
	}
	if !ok || p.ServerType == "" {
		return nil
	}
	role, e := serverTypeRole(p.ServerType)
	if e != nil {
		return e
	}
	m := s.config.Simulation.Filesystem.PodMaintenance
	switch role {
	case "normal":
		if m != nil {
			return fmt.Errorf("normal server cannot use pod executor")
		}
	case "pod-node":
		if m == nil || m.PodID != p.PodID || m.NodeID != p.NodeID {
			return fmt.Errorf("pod server requires matching coordinated executor; normal fallback refused")
		}
	case "witness":
		o := s.config.Simulation.Filesystem.ObserverMaintenance
		if m != nil || o == nil || !o.Enabled || o.Protocol != podmaintenance.ObserverProtocol || o.PodID != p.PodID || o.NodeID != "witness" || p.NodeID != "witness" {
			return fmt.Errorf("pod-observer requires enabled matching observer-specific executor; no fallback")
		}
	}
	return nil
}

// Installation class is immutable metadata, never the dynamic ACTIVE/STBY role.
func (s *Simulator) installationIdentity() *platformtypes.InstallationIdentity {
	if s.state == nil || s.state.SiemCoreInstallation == nil {
		return nil
	}
	saved := s.state.SiemCoreInstallation
	kind := "pod"
	if saved.ServerType == "normal" {
		kind = "normal"
	}
	result := &platformtypes.InstallationIdentity{Kind: kind, PodID: saved.PodID, NodeID: saved.NodeID}
	if result.Validate() != nil {
		return nil
	}
	return result
}
