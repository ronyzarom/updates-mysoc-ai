package updatersim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSimulatorObserveDoesNotDownloadOrReport(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	paths := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/api/v1/heartbeat":
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/check":
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true,
				LatestVersion:   "1.1.0",
				DownloadURL:     "/artifact",
				SHA256:          "deadbeef",
				UpdateGroup:     "alpha",
			})
		default:
			t.Fatalf("observe mode made unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeObserve)
	simulator, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}
	if err := simulator.RunCycle(context.Background(), ModeObserve); err != nil {
		t.Fatalf("run observe cycle: %v", err)
	}
	if cfg.Products[0].CurrentVersion != "1.0.0" {
		t.Fatalf("observe mode advanced version to %s", cfg.Products[0].CurrentVersion)
	}

	mu.Lock()
	defer mu.Unlock()
	if paths["/artifact"] != 0 || paths["/api/v1/updates/siemcore/report"] != 0 {
		t.Fatalf("observe mode downloaded or reported: %#v", paths)
	}
}

func TestSimulatorRealModeDownloadsReportsAndPersistsVersion(t *testing.T) {
	t.Parallel()

	artifact := []byte("simulated artifact")
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])
	var report UpdateReportRequest
	var reportCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/check":
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true,
				LatestVersion:   "1.1.0",
				DownloadURL:     "/artifact",
				SHA256:          checksum,
				Channel:         "stable",
				UpdateGroup:     "alpha",
			})
		case "/artifact":
			w.Header().Set("X-Checksum-SHA256", checksum)
			_, _ = w.Write(artifact)
		case "/api/v1/updates/siemcore/report":
			reportCount++
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatalf("decode report: %v", err)
			}
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	executor := &recordingExecutor{}
	simulator, err := NewSimulator(cfg, executor, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}
	if err := simulator.RunCycle(context.Background(), ModeReal); err != nil {
		t.Fatalf("run simulate cycle: %v", err)
	}

	if !executor.applied || !executor.validated || executor.rolledBack {
		t.Fatalf("unexpected executor calls: %#v", executor)
	}
	if reportCount != 1 || !report.Success ||
		report.FromVersion != "1.0.0" ||
		report.ToVersion != "1.1.0" {
		t.Fatalf("unexpected report: count=%d report=%#v", reportCount, report)
	}
	if cfg.Products[0].CurrentVersion != "1.1.0" {
		t.Fatalf("simulated version = %s, want 1.1.0", cfg.Products[0].CurrentVersion)
	}

	state, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.ProductVersions["siemcore"] != "1.1.0" ||
		state.LastUpdateAttempt == nil ||
		!state.LastUpdateAttempt.Success {
		t.Fatalf("unexpected persisted state: %#v", state)
	}
}

func TestSimulatorRealModeWithoutExecutorRefusesToReportSuccess(t *testing.T) {
	t.Parallel()

	artifact := []byte("artifact that must never be reported installed")
	sum := sha256.Sum256(artifact)
	checksum := hex.EncodeToString(sum[:])
	var report UpdateReportRequest
	var reportCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/check":
			writeTestJSON(t, w, UpdateCheckResponse{
				UpdateAvailable: true,
				LatestVersion:   "1.1.0",
				DownloadURL:     "/artifact",
				SHA256:          checksum,
				Channel:         "stable",
				UpdateGroup:     "alpha",
			})
		case "/artifact":
			w.Header().Set("X-Checksum-SHA256", checksum)
			_, _ = w.Write(artifact)
		case "/api/v1/updates/siemcore/report":
			reportCount++
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatalf("decode report: %v", err)
			}
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	simulator, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}

	// The cycle must error, the attempt must be reported as a FAILURE, and
	// the tracked version must not advance: an install that did not happen
	// is never reported as one (the silent failure of the retired
	// "simulate" mode).
	if err := simulator.RunCycle(context.Background(), ModeReal); err == nil {
		t.Fatal("real mode with no executor reported a clean cycle")
	}
	if reportCount != 1 || report.Success {
		t.Fatalf("expected one failure report, got count=%d report=%#v", reportCount, report)
	}
	if report.Error == "" {
		t.Fatalf("failure report carries no error message: %#v", report)
	}
	if cfg.Products[0].CurrentVersion != "1.0.0" {
		t.Fatalf("version advanced to %s despite no install", cfg.Products[0].CurrentVersion)
	}
}

func TestConfigNormalizesLegacySimulateMode(t *testing.T) {
	t.Parallel()

	cfg := newSimulatorTestConfig(t, "http://127.0.0.1:1", ModeSimulate)
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("legacy simulate config rejected: %v", err)
	}
	if cfg.Simulation.Mode != ModeReal || !cfg.Simulation.LegacySimulateMode {
		t.Fatalf("legacy simulate not normalized: mode=%q flag=%v",
			cfg.Simulation.Mode, cfg.Simulation.LegacySimulateMode)
	}
}

type recordingExecutor struct {
	applied    bool
	validated  bool
	rolledBack bool
}

func (e *recordingExecutor) Apply(_ context.Context, _ Update) error {
	e.applied = true
	return nil
}

func (e *recordingExecutor) Validate(_ context.Context, _ Update) error {
	e.validated = true
	return nil
}

func (e *recordingExecutor) Rollback(_ context.Context, _ Update) error {
	e.rolledBack = true
	return nil
}

func newSimulatorTestConfig(t *testing.T, serverURL string, mode Mode) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		Server: ServerConfig{
			URL:              serverURL,
			Timeout:          Duration{Duration: time.Second},
			MaxResponseBytes: 1 << 20,
		},
		Instance: InstanceConfig{
			ID:             "sim-siemcore-test",
			Type:           "simulator",
			Hostname:       "sim-test",
			MachineID:      "sim-machine-test",
			UpdaterVersion: "updater-simulator/test",
			OS:             "linux",
			Arch:           "amd64",
		},
		Heartbeat: HeartbeatConfig{
			Interval: Duration{Duration: time.Minute},
			Jitter:   Duration{Duration: time.Second},
		},
		Simulation: SimulationConfig{
			Mode:             mode,
			ArtifactDir:      filepath.Join(dir, "artifacts"),
			StateFile:        filepath.Join(dir, "state.json"),
			MaxDownloadBytes: 1 << 20,
		},
		Products: []ProductConfig{{
			Name:           "siemcore",
			CurrentVersion: "1.0.0",
			Channel:        "stable",
		}},
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write JSON: %v", err)
	}
}
