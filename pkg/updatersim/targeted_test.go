package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

type failingTargetExecutor struct {
	recordingExecutor
	fail bool
}

func (e *failingTargetExecutor) Validate(ctx context.Context, u Update) error {
	e.validated = true
	if e.fail {
		return fmt.Errorf("unhealthy target")
	}
	return nil
}

func TestTargetedReleaseUsesSignedPipelineAndReportsTruth(t *testing.T) {
	for _, scenario := range []string{"success", "bad-signature", "bad-health", "stale-current"} {
		t.Run(scenario, func(t *testing.T) {
			pub, priv, _ := ed25519.GenerateKey(rand.Reader)
			artifact := []byte("targeted signed test artifact")
			sum := sha256.Sum256(artifact)
			checksum := hex.EncodeToString(sum[:])
			sig := signing.Sign(priv, "siemcore", "1.1.0", checksum)
			if scenario == "bad-signature" {
				sig = signing.Sign(priv, "siemcore", "other", checksum)
			}
			var reports []UpdateReportRequest
			downloads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/releases/siemcore/1.1.0":
					writeTestJSON(t, w, types.Release{ProductName: "siemcore", Version: "1.1.0", Checksum: checksum, Signature: sig})
				case "/api/v1/releases/siemcore/1.1.0/download":
					downloads++
					w.Write(artifact)
				case "/api/v1/heartbeat":
					writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
				case "/api/v1/updates/siemcore/report":
					var report UpdateReportRequest
					json.NewDecoder(r.Body).Decode(&report)
					reports = append(reports, report)
					writeTestJSON(t, w, map[string]string{"status": "ok"})
				default:
					t.Errorf("unexpected policy/fleet mutation request %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 400)
				}
			}))
			defer server.Close()
			cfg := newSimulatorTestConfig(t, server.URL, ModeObserve)
			cfg.Signing = SigningConfig{PublicKey: hex.EncodeToString(pub), Require: true}
			executor := &failingTargetExecutor{fail: scenario == "bad-health"}
			s, err := NewSimulator(cfg, executor, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			expected := "1.0.0"
			if scenario == "stale-current" {
				expected = "0.9.0"
			}
			err = s.RunTargetedRelease(context.Background(), "siemcore", "1.1.0", expected)
			if scenario == "success" {
				if err != nil || !executor.validated || len(reports) != 1 || !reports[0].Success {
					t.Fatalf("success pipeline: %v %#v", err, reports)
				}
				state, _ := LoadState(cfg.Simulation.StateFile)
				if state.ProductVersions["siemcore"] != "1.1.0" {
					t.Fatal("version not persisted")
				}
			} else {
				if err == nil {
					t.Fatal("expected failure")
				}
				if cfg.Products[0].CurrentVersion != "1.0.0" {
					t.Fatal("failed attempt advanced version")
				}
				if scenario == "bad-health" {
					if !executor.rolledBack || len(reports) != 1 || reports[0].Success {
						t.Fatal("failed health must roll back and report failure")
					}
				} else if downloads != 0 || executor.applied || len(reports) != 0 {
					t.Fatal("precondition/signature failure touched application")
				}
			}
		})
	}
}
