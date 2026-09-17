package updatersim

import (
	"context"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"testing"
	"time"
)

func TestConfiguredAckDrainDoesNotPreemptLiveHandshake(t *testing.T) {
	cfg := &Config{}
	cfg.Simulation.Filesystem.PodMaintenance = &PodMaintenanceConfig{JournalDirectory: t.TempDir(), Drain: &PodDrainConfig{Protocol: podmaintenance.AckDrainProtocol, AdapterCommand: []string{"/must-not-run"}}}
	s := &Simulator{config: cfg}
	j := podmaintenance.Journal{Binding: podmaintenance.Binding{Protocol: podmaintenance.AckProtocol, Deadline: time.Now().Add(time.Minute)}, Generation: 42, Phase: "barrier-acknowledged"}
	handled, err := s.resumePodDrain(context.Background(), j)
	if handled || err != nil {
		t.Fatal("recovery preempted normal handshake", handled, err)
	}
	cfg.Simulation.Filesystem.PodMaintenance.Drain.Protocol = podmaintenance.DrainProtocol
	j.Binding.Deadline = time.Now().Add(-time.Minute)
	handled, err = s.resumePodDrain(context.Background(), j)
	if handled || err != nil {
		t.Fatal("legacy drain used for ACK-v2", handled, err)
	}
}
