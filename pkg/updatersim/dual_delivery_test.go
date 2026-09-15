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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// This is protocol E2E with a recording executor, not product installation qualification.
func TestDualFixtureDeliveryAndRetry(t *testing.T) {
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		for _, scenario := range []string{"empty", "installed", "changed-after-download", "interrupted-download"} {
			t.Run(product+"/"+scenario, func(t *testing.T) {
				pub, key, _ := ed25519.GenerateKey(rand.Reader)
				payload := []byte("signed fixture artifact")
				sum := sha256.Sum256(payload)
				a := types.Artifact{Product: product, Version: "1.1.0", Kind: "bootstrap", Name: "boot", Arch: "linux/amd64", SourceCommit: strings.Repeat("a", 40), Checksum: hex.EncodeToString(sum[:]), Size: int64(len(payload))}
				b := a
				b.Kind = "update"
				b.Name = "thin"
				b.RequiredDependencies = []types.Dependency{{Reference: "fixture-image", Digest: "sha256:" + strings.Repeat("c", 64)}}
				artifactprotocol.Sign(key, &a)
				artifactprotocol.Sign(key, &b)
				variants := []types.Artifact{a, b}
				evidencePath := filepath.Join(t.TempDir(), "evidence.json")
				evidence := artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: "1.0.0", Dependencies: b.RequiredDependencies}
				if scenario == "empty" {
					evidence = artifactprotocol.Evidence{Lifecycle: "empty"}
				}
				writeEvidence := func(e artifactprotocol.Evidence) {
					raw, _ := json.Marshal(e)
					if err := os.WriteFile(evidencePath, raw, 0600); err != nil {
						t.Error(err)
					}
				}
				writeEvidence(evidence)
				first := true
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/check"):
						var req UpdateCheckRequest
						_ = json.NewDecoder(r.Body).Decode(&req)
						if !artifactprotocol.Advertised(req.ProtocolVersion, req.Capabilities) {
							t.Error("capability not advertised")
						}
						selected, status := artifactprotocol.Select(variants, artifactprotocol.Evidence{Lifecycle: req.Lifecycle, InstalledVersion: req.InstalledVersion, Dependencies: req.CachedDependencies})
						writeTestJSON(t, w, UpdateCheckResponse{ProtocolVersion: artifactprotocol.Version, UpdateAvailable: true, LatestVersion: a.Version, DownloadURL: "/download", SHA256: selected.Checksum, Signature: selected.Signature, Artifacts: variants, SelectedArtifactKind: selected.Kind, DependencyValidation: status})
					case r.URL.Path == "/download":
						if first && scenario == "changed-after-download" {
							writeEvidence(artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: "1.0.0", Dependencies: []types.Dependency{}})
						}
						if first && scenario == "interrupted-download" {
							w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
							_, _ = w.Write(payload[:3])
							first = false
							return
						}
						first = false
						_, _ = w.Write(payload)
					case strings.HasSuffix(r.URL.Path, "/report"):
						writeTestJSON(t, w, map[string]string{"status": "ok"})
					default:
						t.Errorf("unexpected network path %s", r.URL.Path)
						w.WriteHeader(404)
					}
				}))
				defer server.Close()
				cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
				cfg.Products[0].Name = product
				cfg.Products[0].PrerequisiteVerifier = []string{"/bin/cat", evidencePath}
				executor := &recordingExecutor{}
				sim, err := NewSimulator(cfg, executor, discardLogger())
				if err != nil {
					t.Fatal(err)
				}
				sim.publicKey = pub
				offer, err := sim.Check(context.Background(), product)
				if err != nil {
					t.Fatal(err)
				}
				want := "update"
				if scenario == "empty" {
					want = "bootstrap"
				}
				if offer.SelectedArtifactKind != want {
					t.Fatal("wrong artifact selected")
				}
				err = sim.processOffer(context.Background(), ModeReal, offer)
				fail := scenario == "changed-after-download" || scenario == "interrupted-download"
				if fail {
					if err == nil || executor.applied {
						t.Fatalf("failure did not stop apply: %v", err)
					}
					state, loadErr := LoadState(cfg.Simulation.StateFile)
					if loadErr != nil || state.LastUpdateAttempt == nil || state.LastUpdateAttempt.Success {
						t.Fatal("failure not persisted")
					}
					writeEvidence(evidence)
					if err = sim.processOffer(context.Background(), ModeReal, offer); err != nil || executor.applied {
						t.Fatal("same-target retry must remain deferred", err)
					}
					if err = sim.RetryProduct(product, offer.LatestVersion, offer.Checksum); err != nil {
						t.Fatal(err)
					}
					if err = sim.processOffer(context.Background(), ModeReal, offer); err != nil {
						t.Fatal("retry failed", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if !executor.applied || sim.state.LastUpdateAttempt.SelectedArtifactKind != want || sim.state.LastUpdateAttempt.ArtifactDigest != a.Checksum {
					t.Fatal("apply/status missing")
				}
			})
		}
	}
}
