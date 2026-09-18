package updatersim

import (
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"slices"
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
	case "normal", "observer-unlinked", "pod-node":
		return kind, nil
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
	if role == "normal" || role == "observer-unlinked" {
		if p.PodID != "" || p.NodeID != "" || cfg.Simulation.Filesystem.PodMaintenance != nil {
			return fmt.Errorf("normal installation cannot have pod identity or executor")
		}
	} else if p.ServerType == "pod-node" {
		if p.PodID != "" || (p.NodeID != "1" && p.NodeID != "2") || cfg.Simulation.Filesystem.PodMaintenance != nil {
			return fmt.Errorf("independent node requires immutable local slot without pod binding")
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
	if s.config.Simulation.Filesystem.IndependentNodeUpdate && (!ok || p.ServerType != "pod-node") {
		return fmt.Errorf("independent node update executor cannot serve other installation types")
	}
	if s.config.Simulation.Filesystem.IndependentNodeBootstrap && (!ok || p.ServerType != "pod-node") {
		return fmt.Errorf("independent node bootstrap cannot serve other installation types")
	}
	if s.config.Simulation.Filesystem.ObserverUnlinkedUpdate && (!ok || p.ServerType != "observer-unlinked") {
		return fmt.Errorf("independent Observer update executor cannot serve other installation types")
	}
	if s.config.Simulation.Filesystem.ObserverMaintenance != nil && (!ok || p.ServerType != "pod-observer") {
		return fmt.Errorf("observer executor cannot handle normal or data nodes")
	}
	if !ok || p.ServerType == "" {
		return nil
	}
	if p.ServerType == "pod-node" {
		f := s.config.Simulation.Filesystem
		expected := []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}
		if !f.IndependentNodeBootstrap {
			return fmt.Errorf("independent node delivery disabled pending joint product qualification; normal fallback refused")
		}
		if f.PodMaintenance != nil || f.ObserverMaintenance != nil || p.PodID != "" || (p.NodeID != "1" && p.NodeID != "2") || f.InstallRoot != "/opt/siemcore-cascade" || !slices.Equal(f.RestartCommand, expected) || !slices.Equal(f.HealthCommand, expected) {
			return fmt.Errorf("independent node requires protected bootstrap executor without pod authority")
		}
		return nil
	}
	role, e := serverTypeRole(p.ServerType)
	if e != nil {
		return e
	}
	m := s.config.Simulation.Filesystem.PodMaintenance
	switch role {
	case "observer-unlinked":
		f := s.config.Simulation.Filesystem
		expected := []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}
		if m != nil || f.ObserverMaintenance != nil || p.PodID != "" || p.NodeID != "" || f.InstallRoot != "/opt/siemcore-cascade" || !slices.Equal(f.RestartCommand, expected) || !slices.Equal(f.HealthCommand, expected) {
			return fmt.Errorf("independent Observer requires protected bootstrap executor without pod authority")
		}
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
	if saved.ServerType == "normal" || saved.ServerType == "observer-unlinked" || saved.ServerType == "pod-node" {
		kind = saved.ServerType
	}
	result := &platformtypes.InstallationIdentity{Kind: kind, PodID: saved.PodID, NodeID: saved.NodeID}
	if result.Validate() != nil {
		return nil
	}
	return result
}
