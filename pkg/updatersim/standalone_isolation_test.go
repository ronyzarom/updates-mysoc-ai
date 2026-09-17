package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/updatecapability"
)

// Exercise the normal automatic cycle, including signature/checksum, executor,
// health/rollback and reporting. Unconfigured pod files must never be consulted.
func TestStandaloneAutomaticUpdateIsolatedFromPod(t *testing.T) {
	for _, serverType := range []string{"", "normal"} {
		for _, scenario := range []string{"success", "bad-signature", "bad-checksum", "bad-health"} {
			t.Run(serverType+"/"+scenario, func(t *testing.T) {
				pub, priv, _ := ed25519.GenerateKey(rand.Reader)
				artifact := []byte("standalone signed release")
				sum := sha256.Sum256(artifact)
				digest := hex.EncodeToString(sum[:])
				sig := signing.Sign(priv, "siemcore", "1.1.0", digest)
				if scenario == "bad-signature" {
					sig = signing.Sign(priv, "siemcore", "wrong", digest)
				}
				var reports []UpdateReportRequest
				checks, heartbeats, selfChecks := 0, 0, 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.URL.Path == "/api/v1/heartbeat":
						heartbeats++
						writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
					case strings.Contains(r.URL.Path, "/updates/updater-"):
						selfChecks++
						writeTestJSON(t, w, UpdateCheckResponse{})
					case r.URL.Path == "/api/v1/updates/siemcore/check":
						checks++
						var q UpdateCheckRequest
						if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
							t.Error(e)
						}
						if q.DeploymentRole != "" && q.DeploymentRole != "normal" {
							t.Errorf("standalone advertised pod role %q", q.DeploymentRole)
						}
						offer := UpdateCheckResponse{UpdateAvailable: true, LatestVersion: "1.1.0", SHA256: digest, Signature: sig, DownloadURL: "/download"}
						// Normal hosts must not acquire pod prerequisites even on a shared release.
						if serverType == "normal" {
							offer.UpdaterRequirements = &updatecapability.Requirements{Scope: "pod-node", Capabilities: []string{"pod-maintenance-v1"}, MinUpdaterVersion: "999.0.0"}
						}
						writeTestJSON(t, w, offer)
					case r.URL.Path == "/download":
						if scenario == "bad-checksum" {
							w.Write([]byte("corrupt"))
						} else {
							w.Write(artifact)
						}
					case strings.HasSuffix(r.URL.Path, "/report"):
						var report UpdateReportRequest
						json.NewDecoder(r.Body).Decode(&report)
						reports = append(reports, report)
						writeTestJSON(t, w, map[string]string{"status": "ok"})
					default:
						t.Errorf("unexpected standalone request %s", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
				cfg.Products[0].ServerType = serverType
				cfg.Signing = SigningConfig{PublicKey: hex.EncodeToString(pub), Require: true}
				// No PodMaintenance config: even stray similarly named journals are inert.
				stray := filepath.Join(filepath.Dir(cfg.Simulation.StateFile), "pod-maintenance")
				os.MkdirAll(stray, 0700)
				for _, name := range []string{"operation.json", "drain-v1.json", "recovery-v2.json"} {
					os.WriteFile(filepath.Join(stray, name), []byte("not a valid pod journal"), 0600)
				}
				executor := &failingTargetExecutor{fail: scenario == "bad-health"}
				s, e := NewSimulator(cfg, executor, discardLogger())
				if e != nil {
					t.Fatal(e)
				}
				s.binaryVersion = "1.16.1.24"
				pending, e := s.resumePendingPod(context.Background())
				if pending || e != nil {
					t.Fatal("standalone startup entered pod recovery", pending, e)
				}
				e = s.RunCycle(context.Background(), ModeReal)
				if checks != 1 || heartbeats != 1 || selfChecks != 1 {
					t.Fatal("normal automatic cycle changed", checks, heartbeats, selfChecks)
				}
				if scenario == "success" {
					if e != nil || !executor.applied || !executor.validated || executor.rolledBack || len(reports) != 1 || !reports[0].Success {
						t.Fatal("normal update failed", e, executor, reports)
					}
					// Restart with the type omitted: a persisted normal installation stays normal.
					cfg.Products[0].ServerType = ""
					cfg.Products[0].DeploymentRole = ""
					restarted, e := NewSimulator(cfg, &recordingExecutor{}, discardLogger())
					if e != nil {
						t.Fatal(e)
					}
					if restarted.config.Products[0].CurrentVersion != "1.1.0" {
						t.Fatal("normal version not retained")
					}
					if serverType == "normal" && restarted.config.Products[0].ServerType != "normal" {
						t.Fatal("normal identity not retained")
					}
				} else {
					if e == nil || cfg.Products[0].CurrentVersion != "1.0.0" {
						t.Fatal("failed normal update advanced version", e)
					}
					if scenario == "bad-health" {
						if !executor.applied || !executor.validated || !executor.rolledBack || len(reports) != 1 || reports[0].Success {
							t.Fatal("normal rollback changed", executor, reports)
						}
					} else if executor.applied {
						t.Fatal("unverified artifact applied")
					}
				}
				for _, r := range reports {
					if r.Kind == "pod_maintenance" {
						t.Fatal("standalone reported pod execution")
					}
				}
				for _, name := range []string{"operation.json", "drain-v1.json", "recovery-v2.json"} {
					raw, _ := os.ReadFile(filepath.Join(stray, name))
					if string(raw) != "not a valid pod journal" {
						t.Fatal("standalone touched pod state")
					}
				}
			})
		}
	}
}

func TestNormalRejectsPodConfigurationBeforeAdapterRuns(t *testing.T) {
	cfg := newSimulatorTestConfig(t, "https://example.invalid", ModeReal)
	cfg.Products[0].ServerType = "normal"
	cfg.Simulation.Filesystem.PodMaintenance = &PodMaintenanceConfig{AdapterCommand: []string{"/must-not-execute"}, Drain: &PodDrainConfig{AdapterCommand: []string{"/must-not-execute"}}, Recovery: &PodRecoveryConfig{NextOperationFile: "/must-not-read"}}
	if _, e := NewSimulator(cfg, &recordingExecutor{}, discardLogger()); e == nil {
		t.Fatal("conflicting normal/pod configuration accepted")
	}
}
