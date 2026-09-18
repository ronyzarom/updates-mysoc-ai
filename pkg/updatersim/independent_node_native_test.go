package updatersim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in native component qualification. Uses a disposable root host, real
// artifact signature/download and protected sudo hook; not a deployed relay.
func TestIndependentNodeNativePipeline(t *testing.T) {
	if os.Getenv("INDEPENDENT_NODE_NATIVE_TEST") != "1" {
		t.Skip("isolated installed root fixture only")
	}
	var input struct {
		Version   string `json:"version"`
		SHA256    string `json:"sha256"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
		NodeID    string `json:"node_id"`
		UpdaterID string `json:"updater_id"`
	}
	raw, err := os.ReadFile("/fixture/native-node.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var report UpdateReportRequest
	reports := 0
	var corrupt atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/check":
			writeTestJSON(t, w, UpdateCheckResponse{UpdateAvailable: true, LatestVersion: input.Version, DownloadURL: "/artifact", SHA256: input.SHA256, Signature: input.Signature, Channel: "stable", UpdateGroup: "alpha"})
		case "/artifact":
			if corrupt.Load() {
				_, _ = w.Write([]byte("corrupt fixture artifact"))
				return
			}
			http.ServeFile(w, r, "/fixture/node-release.tar.gz")
		case "/api/v1/updates/siemcore/report":
			mu.Lock()
			defer mu.Unlock()
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Error(err)
			}
			reports++
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	cfg.Server.Timeout = Duration{Duration: time.Minute}
	cfg.Instance.ID = input.UpdaterID
	cfg.Instance.Arch = "arm64"
	cfg.Signing = SigningConfig{Require: true, PublicKey: input.PublicKey}
	cfg.Products = []ProductConfig{{Name: "siemcore", ServerType: "pod-node", NodeID: input.NodeID, CurrentVersion: "0.0.0", Channel: "stable"}}
	cfg.Simulation.MaxDownloadBytes = 1 << 30
	cfg.Simulation.ArtifactDir = "/var/lib/siemcore-cascade-updater/artifacts"
	cfg.Simulation.Filesystem = FilesystemConfig{IndependentNodeBootstrap: true, InstallRoot: "/opt/siemcore-cascade", RestartCommand: []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}, HealthCommand: []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}, CommandTimeout: Duration{Duration: 5 * time.Minute}}
	executor := NewFilesystemExecutor(cfg.Simulation.Filesystem, discardLogger())
	simulator, err := NewSimulator(cfg, executor, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err = simulator.RunCycle(context.Background(), ModeReal); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if reports != 1 || !report.Success || report.ToVersion != input.Version || report.ArtifactDigest != input.SHA256 {
		t.Fatalf("incorrect success report: %+v count=%d", report, reports)
	}
	mu.Unlock()
	saved, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ProductVersions["siemcore"] != input.Version || saved.SiemCoreInstallation == nil || saved.SiemCoreInstallation.ServerType != "pod-node" || saved.SiemCoreInstallation.NodeID != input.NodeID {
		t.Fatal("installed identity/version not persisted")
	}
	// Verify independent signature rejection before execution, without modifying state.
	bad := &UpdateOffer{Product: "siemcore", LatestVersion: input.Version, Checksum: input.SHA256, Signature: "invalid", DownloadURL: server.URL + "/artifact"}
	if _, err = simulator.verifyAndDownload(context.Background(), bad); err == nil {
		t.Fatal("invalid signature accepted")
	}
	bad.Signature = input.Signature
	corrupt.Store(true)
	if _, err = simulator.verifyAndDownload(context.Background(), bad); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("corrupted bytes were not checksum-rejected: %v", err)
	}
	t.Log("PASS: compiled updater signed download, real root hook replay/health, success report and immutable identity persistence; invalid signature and corrupted checksum rejected")
}
