package updatersim

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFirstInstallNotAnAvailabilityViolation proves a fresh-host first_install
// (which legitimately runs far longer than a steady-state restart) is not
// measured as an availability window, so it never flags a violation.
func TestFirstInstallNotAnAvailabilityViolation(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(t.TempDir(), "slow-first-install.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.0.0"})

	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           root,
		RestartCommand:        []string{script},
		CommandTimeout:        Duration{Duration: defaultCommandTimeout},
		FirstInstallTimeout:   Duration{Duration: defaultCommandTimeout},
		MaxUnavailableSeconds: 1, // tiny budget: a measured ~1s window would breach it
	}, nil)

	// FromVersion 0.0.0 => fresh host => first_install phase.
	update := Update{Product: "siemcore", FromVersion: "0.0.0", ToVersion: "v1.0.0", ArtifactPath: artifact}
	if err := e.Apply(context.Background(), update); err != nil {
		t.Fatalf("first install apply: %v", err)
	}
	seconds, violation := e.LastUnavailable()
	if violation {
		t.Fatalf("first_install must not be an availability violation (seconds=%d)", seconds)
	}
	if seconds != 0 {
		t.Fatalf("first_install window must not be measured, got %ds", seconds)
	}
}

// TestResetUnavailableBeforeApply proves a stale measurement from a prior update
// is cleared so a failure before the restart does not report an old window.
func TestResetUnavailableBeforeApply(t *testing.T) {
	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           t.TempDir(),
		MaxUnavailableSeconds: 1,
	}, nil)
	e.recordUnavailable(600 * time.Second) // simulate a prior big window + violation
	if s, v := e.LastUnavailable(); s == 0 || !v {
		t.Fatalf("precondition: expected a recorded violation, got %d/%v", s, v)
	}
	// Apply fails early (no artifact path) but must reset first.
	_ = e.Apply(context.Background(), Update{Product: "p", FromVersion: "1", ToVersion: "2"})
	if s, v := e.LastUnavailable(); s != 0 || v {
		t.Fatalf("stale measurement not reset at Apply start: %d/%v", s, v)
	}
}

// TestRecordAttemptCarriesAvailabilityIntoHeartbeat proves the measured window
// reaches the heartbeat's last_update_attempt JSON the promotion gate reads.
func TestRecordAttemptCarriesAvailabilityIntoHeartbeat(t *testing.T) {
	cfg := &Config{}
	cfg.Instance.ID = "node-1"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "mysoc"
	cfg.Simulation.StateFile = t.TempDir() + "/state.json"
	cfg.Simulation.ArtifactDir = t.TempDir()
	cfg.Server.URL = "http://127.0.0.1:1"

	fe := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:           t.TempDir(),
		MaxUnavailableSeconds: 10,
	}, discardLogger())
	fe.recordUnavailable(45 * time.Second) // 45s > 10s budget => violation

	sim, err := NewSimulator(cfg, fe, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	sim.recordAttempt(Update{Product: "mysoc", FromVersion: "1.0.0", ToVersion: "1.1.0"}, true, "")

	hb := sim.buildHeartbeat()
	if hb.LastUpdateAttempt == nil {
		t.Fatal("heartbeat carries no last_update_attempt")
	}
	raw, err := json.Marshal(hb.LastUpdateAttempt)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["unavailable_seconds"] == nil {
		t.Fatalf("unavailable_seconds absent from heartbeat JSON: %s", raw)
	}
	if v, _ := decoded["availability_violation"].(bool); !v {
		t.Fatalf("availability_violation not true in heartbeat JSON: %s", raw)
	}
}
