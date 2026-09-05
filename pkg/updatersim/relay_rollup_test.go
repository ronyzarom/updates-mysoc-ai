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

// TestRelayForwardsProductTelemetry proves an SWF leaf's delivery telemetry
// survives the two places the cascade would otherwise drop it: the typed JSON
// decode of the child heartbeat, and the relay's projection of the child into
// its upward rollup (both the full ChildrenReport path and the per-node
// childReport used by delta reporting).
func TestRelayForwardsProductTelemetry(t *testing.T) {
	statusUTC := time.Date(2026, 9, 5, 6, 40, 1, 0, time.UTC)
	swfTelemetry := &platformtypes.ProductTelemetry{
		Ready:            true,
		Connection:       "connected",
		Sent:             14820,
		Seen:             14821,
		Admitted:         14820,
		DeliveryEPSMilli: 2500,
		SpoolEvents:      0,
		StatusUTC:        statusUTC,
		LastError:        "syslog tls: handshake failure",
	}

	// The child heartbeat arrives over the wire as JSON. Decoding into the
	// typed Heartbeat must retain telemetry (a missing type would silently
	// drop it, which is exactly the pre-1.15.0 relay behavior).
	wire := platformtypes.Heartbeat{
		InstanceID:  "swf-acme-WS0042",
		ProductTier: "swf",
		CustomerID:  "acme",
		Products: []platformtypes.ProductStatus{
			{Name: "swf", Version: "2.2.0", Channel: "stable", Status: "running", Telemetry: swfTelemetry},
		},
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var decoded platformtypes.Heartbeat
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Products) != 1 || decoded.Products[0].Telemetry == nil {
		t.Fatalf("telemetry dropped on decode: %+v", decoded.Products)
	}

	cfg := &Config{}
	cfg.Instance.ID = "siemcore-a"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "siemcore"
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
	relay.children["swf-acme-WS0042"] = &childState{
		Heartbeat: decoded,
		LastSeen:  time.Now(),
		SourceIP:  "203.0.113.90",
	}
	relay.mu.Unlock()

	// Full-rollup projection.
	reports := relay.ChildrenReport()
	if len(reports) != 1 || len(reports[0].Products) != 1 {
		t.Fatalf("expected one child with one product, got %+v", reports)
	}
	tel := reports[0].Products[0].Telemetry
	if tel == nil {
		t.Fatal("telemetry missing from full rollup")
	}
	if tel.Connection != "connected" || tel.Sent != 14820 || tel.DeliveryEPSMilli != 2500 {
		t.Fatalf("telemetry counters not forwarded intact: %+v", tel)
	}
	if !tel.StatusUTC.Equal(statusUTC) {
		t.Fatalf("status_utc not forwarded: got %s want %s", tel.StatusUTC, statusUTC)
	}
	if tel.LastError != "syslog tls: handshake failure" {
		t.Fatalf("last_error not forwarded: %q", tel.LastError)
	}

	// Delta-path projection (change-only inventory stream).
	single := childReport(decoded, "online", time.Now(), "203.0.113.90")
	if len(single.Products) != 1 || single.Products[0].Telemetry == nil {
		t.Fatalf("telemetry missing from childReport projection: %+v", single.Products)
	}

	// And it must still be there after the rollup is serialized upward.
	out, err := json.Marshal(reports)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out) {
		t.Fatal("invalid rollup json")
	}
	var back []platformtypes.ChildReport
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if back[0].Products[0].Telemetry == nil || back[0].Products[0].Telemetry.Sent != 14820 {
		t.Fatalf("telemetry lost across rollup serialization: %s", out)
	}
}
