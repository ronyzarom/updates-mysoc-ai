package updatersim

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeUpdaterBinary writes an executable script that mimics the updater's
// "version" subcommand output for the given version.
func fakeUpdaterBinary(t *testing.T, path, version string) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\necho \"updater-simulator %s\"\n", version)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSelfUpdaterStageValidateActivateRestore(t *testing.T) {
	dir := t.TempDir()
	updater := &SelfUpdater{Dir: dir}

	// Simulate the kit-installed 1.0.0 being current.
	fakeUpdaterBinary(t, filepath.Join(dir, "releases", "1.0.0", "updater"), "1.0.0")
	if err := updater.swapCurrent(updater.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}

	// Stage 2.0.0 from a downloaded artifact.
	artifact := filepath.Join(t.TempDir(), "artifact")
	fakeUpdaterBinary(t, artifact, "2.0.0")
	staged, err := updater.Stage(artifact, "2.0.0", "updater")
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if staged != filepath.Join(dir, "releases", "2.0.0", "updater") {
		t.Fatalf("unexpected staged path %s", staged)
	}

	if err := updater.ValidateStaged(context.Background(), staged, "2.0.0"); err != nil {
		t.Fatalf("validate: %v", err)
	}

	if err := updater.Activate("2.0.0"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	target, err := os.Readlink(updater.currentLink())
	if err != nil {
		t.Fatal(err)
	}
	if target != updater.releaseDir("2.0.0") {
		t.Fatalf("current points at %s, want 2.0.0 dir", target)
	}

	// The watchdog path restores the previous target.
	if err := updater.RestorePrevious(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	target, _ = os.Readlink(updater.currentLink())
	if target != updater.releaseDir("1.0.0") {
		t.Fatalf("current points at %s after restore, want 1.0.0 dir", target)
	}
}

func TestSelfUpdaterValidateRejectsWrongVersion(t *testing.T) {
	dir := t.TempDir()
	updater := &SelfUpdater{Dir: dir}

	artifact := filepath.Join(t.TempDir(), "artifact")
	fakeUpdaterBinary(t, artifact, "9.9.9")
	staged, err := updater.Stage(artifact, "2.0.0", "updater")
	if err != nil {
		t.Fatal(err)
	}
	err = updater.ValidateStaged(context.Background(), staged, "2.0.0")
	if err == nil || !strings.Contains(err.Error(), "expected version 2.0.0") {
		t.Fatalf("expected version-mismatch error, got %v", err)
	}
}

func TestSelfUpdaterManages(t *testing.T) {
	dir := t.TempDir()
	updater := &SelfUpdater{Dir: dir}
	if !updater.Manages(filepath.Join(dir, "releases", "1.0.0", "updater")) {
		t.Fatal("expected path inside the layout to be managed")
	}
	if updater.Manages("/usr/local/bin/updater") {
		t.Fatal("expected path outside the layout to be unmanaged")
	}
}

// newSelfUpdateTestSimulator builds a simulator with durable state in a temp
// dir and a client that never needs to reach a server.
func newSelfUpdateTestSimulator(t *testing.T, dir string) *Simulator {
	t.Helper()
	cfg := &Config{}
	cfg.Server.URL = "http://127.0.0.1:1"
	cfg.Instance.ID = "self-update-test"
	cfg.Simulation.StateFile = filepath.Join(dir, "state.json")
	cfg.Simulation.ArtifactDir = filepath.Join(dir, "artifacts")
	cfg.Simulation.MaxDownloadBytes = 1 << 20
	cfg.SelfUpdate.Dir = filepath.Join(dir, "self-update")

	simulator, err := NewSimulator(cfg, NoopExecutor{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return simulator
}

func TestApplySelfUpdateActivatesAndRequestsRestart(t *testing.T) {
	dir := t.TempDir()
	simulator := newSelfUpdateTestSimulator(t, dir)
	simulator.SetBinaryVersion("1.0.0")

	layout := &SelfUpdater{Dir: simulator.config.SelfUpdate.Dir}
	current := filepath.Join(layout.releaseDir("1.0.0"), "updater")
	fakeUpdaterBinary(t, current, "1.0.0")
	if err := layout.swapCurrent(layout.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	// Pretend the process was launched through the managed layout.
	previous := executablePath
	executablePath = func() (string, error) { return current, nil }
	defer func() { executablePath = previous }()

	artifact := filepath.Join(dir, "downloaded-artifact")
	fakeUpdaterBinary(t, artifact, "2.0.0")

	err := simulator.applySelfUpdate(context.Background(), &UpdateOffer{
		Product:        SelfUpdateProduct(),
		CurrentVersion: "1.0.0",
		LatestVersion:  "2.0.0",
	}, artifact)
	if !errors.Is(err, ErrRestartPending) {
		t.Fatalf("expected ErrRestartPending, got %v", err)
	}

	// The pending marker survives for the new binary to finalize.
	state, err := LoadState(simulator.config.Simulation.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingSelfUpdate == nil || state.PendingSelfUpdate.ToVersion != "2.0.0" {
		t.Fatalf("expected pending marker for 2.0.0, got %+v", state.PendingSelfUpdate)
	}
	target, _ := os.Readlink(layout.currentLink())
	if target != layout.releaseDir("2.0.0") {
		t.Fatalf("current points at %s, want 2.0.0", target)
	}
}

func TestApplySelfUpdateSkipsUnmanagedInstall(t *testing.T) {
	dir := t.TempDir()
	simulator := newSelfUpdateTestSimulator(t, dir)
	simulator.SetBinaryVersion("1.0.0")

	previous := executablePath
	executablePath = func() (string, error) { return "/usr/local/bin/updater", nil }
	defer func() { executablePath = previous }()

	artifact := filepath.Join(dir, "downloaded-artifact")
	fakeUpdaterBinary(t, artifact, "2.0.0")

	err := simulator.applySelfUpdate(context.Background(), &UpdateOffer{
		Product:       SelfUpdateProduct(),
		LatestVersion: "2.0.0",
	}, artifact)
	if err != nil {
		t.Fatalf("expected nil (skip) for unmanaged install, got %v", err)
	}
	if simulator.state.PendingSelfUpdate != nil {
		t.Fatal("no pending marker should be persisted for a skipped self-update")
	}
}

func TestResolveSelfUpdateFinalizesOnNewBinary(t *testing.T) {
	dir := t.TempDir()
	simulator := newSelfUpdateTestSimulator(t, dir)
	simulator.state.PendingSelfUpdate = &SelfUpdateState{FromVersion: "1.0.0", ToVersion: "2.0.0"}
	simulator.SetBinaryVersion("2.0.0")

	simulator.ResolveSelfUpdate()

	if simulator.state.PendingSelfUpdate != nil {
		t.Fatal("pending marker should be cleared")
	}
	if simulator.state.UpdaterVersion != "2.0.0" {
		t.Fatalf("updater version = %q, want 2.0.0", simulator.state.UpdaterVersion)
	}
	attempt := simulator.state.LastUpdateAttempt
	if attempt == nil || !attempt.Success || attempt.TargetVersion != "2.0.0" {
		t.Fatalf("expected successful attempt record, got %+v", attempt)
	}
}

func TestResolveSelfUpdateWatchdogRestoresPrevious(t *testing.T) {
	dir := t.TempDir()
	simulator := newSelfUpdateTestSimulator(t, dir)

	layout := &SelfUpdater{Dir: simulator.config.SelfUpdate.Dir}
	fakeUpdaterBinary(t, filepath.Join(layout.releaseDir("1.0.0"), "updater"), "1.0.0")
	fakeUpdaterBinary(t, filepath.Join(layout.releaseDir("2.0.0"), "updater"), "2.0.0")
	if err := layout.swapCurrent(layout.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	// Activate 2.0.0 (records 1.0.0 as previous), but the old binary comes up.
	if err := layout.Activate("2.0.0"); err != nil {
		t.Fatal(err)
	}
	simulator.state.PendingSelfUpdate = &SelfUpdateState{FromVersion: "1.0.0", ToVersion: "2.0.0"}
	simulator.SetBinaryVersion("1.0.0")

	simulator.ResolveSelfUpdate()

	if simulator.state.PendingSelfUpdate != nil {
		t.Fatal("pending marker should be cleared")
	}
	attempt := simulator.state.LastUpdateAttempt
	if attempt == nil || attempt.Success {
		t.Fatalf("expected failed attempt record, got %+v", attempt)
	}
	target, _ := os.Readlink(layout.currentLink())
	if target != layout.releaseDir("1.0.0") {
		t.Fatalf("current points at %s after watchdog, want restored 1.0.0", target)
	}
}
