package updatersim

import (
	"path/filepath"
	"testing"
)

func TestIndependentNodeIdentityRetainedAndDeliveryGated(t *testing.T) {
	for _, node := range []string{"1", "2"} {
		t.Run(node, func(t *testing.T) {
			cfg := &Config{Products: []ProductConfig{{Name: "siemcore", ServerType: "pod-node", NodeID: node}}}
			cfg.Simulation.StateFile = filepath.Join(t.TempDir(), "state.json")
			state := &State{}
			if err := rememberSiemCoreInstallation(cfg, state); err != nil {
				t.Fatal(err)
			}
			s := &Simulator{config: cfg, state: state}
			id := s.installationIdentity()
			if id == nil || id.Kind != "pod-node" || id.NodeID != node || id.PodID != "" {
				t.Fatal(id)
			}
			if s.validateSiemCoreExecution() == nil {
				t.Fatal("unqualified delivery permitted")
			}
			saved, err := LoadState(cfg.Simulation.StateFile)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Products = []ProductConfig{{Name: "siemcore"}}
			if err := rememberSiemCoreInstallation(cfg, saved); err != nil {
				t.Fatal(err)
			}
			if cfg.Products[0].ServerType != "pod-node" || cfg.Products[0].NodeID != node {
				t.Fatal("identity lost")
			}
			cfg.Products[0].ServerType = "normal"
			cfg.Products[0].NodeID = ""
			cfg.Products[0].DeploymentRole = ""
			if rememberSiemCoreInstallation(cfg, saved) == nil {
				t.Fatal("reclassification accepted")
			}
		})
	}
}

func TestIndependentNodeRejectsFakePodAndUnknownSlot(t *testing.T) {
	for _, p := range []ProductConfig{{Name: "siemcore", ServerType: "pod-node", PodID: "fake", NodeID: "1"}, {Name: "siemcore", ServerType: "pod-node", NodeID: "witness"}, {Name: "siemcore", ServerType: "pod-node"}} {
		cfg := &Config{Products: []ProductConfig{p}}
		cfg.Simulation.StateFile = filepath.Join(t.TempDir(), "state.json")
		if rememberSiemCoreInstallation(cfg, &State{}) == nil {
			t.Fatal("invalid identity accepted", p)
		}
	}
}
