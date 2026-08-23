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
		Heartbeat: platformtypes.Heartbeat{
			InstanceID:  "siemcore-a",
			ProductTier: "siemcore",
			CustomerID:  "acme",
			System: platformtypes.SystemMetrics{
				OS:          "linux",
				Arch:        "amd64",
				MemoryTotal: 8 << 30,
				Uptime:      3600,
			},
		},
		LastSeen: time.Now(),
		SourceIP: "10.0.2.30",
	}
	relay.mu.Unlock()

	sim, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	sim.SetChildrenProvider(relay.ChildrenReport)
	sim.SetGuardStatsProvider(relay.GuardStats)

	hb := sim.buildHeartbeat()
	if len(hb.Children) != 1 {
		t.Fatalf("expected 1 child in heartbeat, got %d", len(hb.Children))
	}
	// The rollup must carry the child's host identity/measurements upward so
	// cascaded nodes render OS/arch/uptime on the dashboard like direct ones.
	sys := hb.Children[0].System
	if sys == nil {
		t.Fatal("child system metrics missing from rollup")
	}
	if sys.OS != "linux" || sys.Arch != "amd64" || sys.MemoryTotal != 8<<30 || sys.Uptime != 3600 {
		t.Fatalf("child system metrics not forwarded intact: %+v", sys)
	}
	// The observed source address rolls up so the dashboard can show
	// cascaded nodes' IPs (they never reach the server directly).
	if hb.Children[0].SourceIP != "10.0.2.30" {
		t.Fatalf("child source IP not forwarded: %+v", hb.Children[0])
	}
	// Relay nodes report their port-protection counters.
	if hb.RelayGuard == nil {
		t.Fatal("relay guard stats missing from heartbeat")
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
