package updatersim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRelayPreservesChildRoleForEveryServerType(t *testing.T) {
	for _, kind := range []string{"", "normal", "pod-active", "pod-stby", "pod-observer"} {
		t.Run(kind, func(t *testing.T) {
			childRole := ""
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q UpdateCheckRequest
				if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
					t.Error(e)
				}
				if q.DeploymentRole != childRole || q.InstanceID != "child" {
					t.Errorf("relay substituted its own identity/role: %+v", q)
				}
				writeTestJSON(t, w, UpdateCheckResponse{UpdateAvailable: false})
			}))
			defer origin.Close()
			cfg := newSimulatorTestConfig(t, origin.URL, ModeReal)
			cfg.Products[0].ServerType = kind
			cfg.Relay.Enabled = true
			cfg.Relay.CacheDir = t.TempDir()
			cfg.Server.LicenseKey = "fixture"
			if kind != "" && kind != "normal" {
				cfg.Products[0].PodID = "pod"
				cfg.Products[0].NodeID = "1"
				if kind == "pod-observer" {
					cfg.Products[0].NodeID = "witness"
				}
			}
			sim, e := NewSimulator(cfg, NoopExecutor{}, discardLogger())
			if e != nil {
				t.Fatal(e)
			}
			upstream, e := NewClient(cfg.Server)
			if e != nil {
				t.Fatal(e)
			}
			relay, e := NewRelay(cfg, upstream, discardLogger())
			if e != nil {
				t.Fatal(e)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/updates/{product}/check", relay.handleChildCheck)
			hop := httptest.NewServer(mux)
			defer hop.Close()
			childCfg := cfg.Server
			childCfg.URL = hop.URL
			client, e := NewClient(childCfg)
			if e != nil {
				t.Fatal(e)
			}
			for _, role := range []string{"normal", "pod-node", "witness"} {
				childRole = role
				if _, e = client.CheckUpdate(context.Background(), "siemcore", UpdateCheckRequest{InstanceID: "child", CurrentVersion: "1", DeploymentRole: role}); e != nil {
					t.Fatal(kind, role, e)
				}
			}
			heartbeatChild(t, relay, "reporting-child", "")
			sim.SetChildrenProvider(relay.ChildrenReport)
			if len(sim.buildHeartbeat().Children) != 1 {
				t.Fatal("relay rollup suppressed by server type")
			}
		})
	}
}
