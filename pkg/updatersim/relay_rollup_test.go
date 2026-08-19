package updatersim

import (
	"encoding/json"
	"testing"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestRelayChildrenFlowIntoHeartbeat(t *testing.T) {
	cfg := &Config{}
	cfg.Instance.ID = "mysoc-op1"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "mysoc"
	cfg.Relay.Enabled = true
	cfg.Relay.Listen = "127.0.0.1:0"
	cfg.Relay.CacheDir = t.TempDir()
	cfg.Simulation.StateFile = t.TempDir() + "/state.json"
	cfg.Simulation.ArtifactDir = t.TempDir()
	cfg.Server.URL = "http://127.0.0.1:1"

	client, err := NewClient(cfg.Server)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := NewRelay(cfg, client, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	relay.mu.Lock()
	relay.children["siemcore-a"] = &childState{
		Heartbeat: platformtypes.Heartbeat{InstanceID: "siemcore-a", ProductTier: "siemcore", CustomerID: "acme"},
		LastSeen:  time.Now(),
	}
	relay.mu.Unlock()

	sim, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	sim.SetChildrenProvider(relay.ChildrenReport)

	hb := sim.buildHeartbeat()
	if len(hb.Children) != 1 {
		t.Fatalf("expected 1 child in heartbeat, got %d", len(hb.Children))
	}
	data, _ := json.Marshal(hb)
	if !json.Valid(data) {
		t.Fatal("invalid json")
	}
	var round map[string]interface{}
	json.Unmarshal(data, &round)
	if _, ok := round["children"]; !ok {
		t.Fatalf("children missing from serialized heartbeat: %s", data)
	}
}
