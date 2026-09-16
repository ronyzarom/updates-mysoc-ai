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
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestSignedPodHealthTimeoutDoesNotAdvanceVersion(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(t.TempDir(), "old.tar.gz")
	next := filepath.Join(t.TempDir(), "next.tar.gz")
	makeTarGz(t, old, map[string]string{"VERSION": "1.0.0"})
	makeTarGz(t, next, map[string]string{"VERSION": "1.1.0"})
	artifact, err := os.ReadFile(next)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := signing.Sign(priv, "siemcore", "1.1.0", checksum)
	var reports []UpdateReportRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/releases/siemcore/1.1.0":
			writeTestJSON(t, w, types.Release{ProductName: "siemcore", Version: "1.1.0", Checksum: checksum, Signature: signature})
		case "/api/v1/releases/siemcore/1.1.0/download":
			w.Write(artifact)
		case "/api/v1/heartbeat":
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/report":
			var report UpdateReportRequest
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Error(err)
			}
			reports = append(reports, report)
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "unexpected", 400)
		}
	}))
	defer server.Close()
	shim := filepath.Join(t.TempDir(), "shim")
	// exec ensures the timeout kills the health process, with no orphan child.
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nif [ \"$1\" = health ]; then exec sleep 10; fi\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	executor := NewFilesystemExecutor(FilesystemConfig{InstallRoot: root, RestartCommand: []string{shim}, HealthCommand: []string{shim}, CommandTimeout: Duration{Duration: time.Second}}, discardLogger())
	if err := executor.Apply(context.Background(), Update{Product: "siemcore", ToVersion: "1.0.0", ArtifactPath: old}); err != nil {
		t.Fatal(err)
	}
	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	cfg.Signing = SigningConfig{PublicKey: hex.EncodeToString(pub), Require: true}
	s, err := NewSimulator(cfg, executor, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RunTargetedRelease(context.Background(), "siemcore", "1.1.0", "1.0.0"); err == nil {
		t.Fatal("health timeout reported success")
	}
	state, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if state.ProductVersions["siemcore"] == "1.1.0" || cfg.Products[0].CurrentVersion != "1.0.0" {
		t.Fatal("timeout advanced installed version")
	}
	if len(reports) != 1 || reports[0].Success || reports[0].ToVersion != "1.1.0" || reports[0].Error == "" {
		t.Fatalf("incorrect failure report: %+v", reports)
	}
	if !strings.HasSuffix(resolveCurrent(t, executor, "siemcore"), "1.0.0") {
		t.Fatal("timeout failed to restore prior pointer")
	}
}
