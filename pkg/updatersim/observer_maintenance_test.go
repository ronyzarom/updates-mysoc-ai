package updatersim

import (
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"testing"
)

func TestObserverExecutorExplicitAndIsolated(t *testing.T) {
	cfg := &Config{Products: []ProductConfig{{Name: "siemcore", ServerType: "pod-observer", PodID: "pod", NodeID: "witness"}}}
	s := &Simulator{config: cfg}
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("missing executor allowed")
	}
	cfg.Simulation.Filesystem.ObserverMaintenance = &ObserverMaintenanceConfig{Protocol: podmaintenance.ObserverProtocol, PodID: "pod", NodeID: "witness"}
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("disabled executor allowed")
	}
	cfg.Simulation.Filesystem.ObserverMaintenance.Enabled = true
	if err := s.validateSiemCoreExecution(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"normal", "pod-active", "pod-stby", ""} {
		cfg.Products[0].ServerType = kind
		if s.validateSiemCoreExecution() == nil {
			t.Fatal("observer executor used for", kind)
		}
	}
	cfg.Simulation.Filesystem.ObserverMaintenance = nil
	cfg.Products[0].ServerType = "normal"
	if err := s.validateSiemCoreExecution(); err != nil {
		t.Fatal("normal behavior changed", err)
	}
}
