package updatersim

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRestorePreviousRefusesStalePrevious proves the staleness guard: if the
// swap to the failed target never took effect (current still points at the old
// release), RestorePrevious must refuse rather than act on a possibly
// two-versions-back .previous and wrongly downgrade.
func TestRestorePreviousRefusesStalePrevious(t *testing.T) {
	dir := t.TempDir()
	u := &SelfUpdater{Dir: dir}

	// current -> 1.0.0 (the swap to the failed 2.0.0 never happened).
	fakeUpdaterBinary(t, filepath.Join(dir, "releases", "1.0.0", "updater"), "1.0.0")
	if err := u.swapCurrent(u.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	// A stale .previous pointing two versions back.
	if err := os.WriteFile(u.previousFile(), []byte(u.releaseDir("0.9.0")), 0o644); err != nil {
		t.Fatal(err)
	}

	err := u.RestorePrevious("2.0.0") // failed target is 2.0.0, but current != 2.0.0
	if err == nil {
		t.Fatal("RestorePrevious must refuse when current does not point at the failed target")
	}
	// current must be untouched.
	target, _ := os.Readlink(u.currentLink())
	if target != u.releaseDir("1.0.0") {
		t.Fatalf("current changed on a refused restore: %s", target)
	}
}

// TestActivateIdempotentAndPreviousGuard proves Activate is a no-op when already
// active and never records the target as its own .previous.
func TestActivateIdempotentAndPreviousGuard(t *testing.T) {
	dir := t.TempDir()
	u := &SelfUpdater{Dir: dir}
	fakeUpdaterBinary(t, filepath.Join(dir, "releases", "1.0.0", "updater"), "1.0.0")
	if err := u.swapCurrent(u.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	fakeUpdaterBinary(t, filepath.Join(dir, "releases", "2.0.0", "updater"), "2.0.0")

	if err := u.Activate("2.0.0"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	prev1, _ := os.ReadFile(u.previousFile())
	if string(prev1) != u.releaseDir("1.0.0") {
		t.Fatalf(".previous = %q, want 1.0.0 dir", prev1)
	}

	// Second Activate of the same version is a no-op and must NOT overwrite
	// .previous with 2.0.0 (which would make a later restore loop).
	if err := u.Activate("2.0.0"); err != nil {
		t.Fatalf("idempotent activate: %v", err)
	}
	prev2, _ := os.ReadFile(u.previousFile())
	if string(prev2) != u.releaseDir("1.0.0") {
		t.Fatalf(".previous changed on idempotent activate: %q", prev2)
	}
}

// TestSelfUpdaterPruneKeepsCurrentPreviousAndN proves Prune removes stale
// release directories while never touching current or previous and keeping the N
// most-recent others.
func TestSelfUpdaterPruneKeepsCurrentPreviousAndN(t *testing.T) {
	dir := t.TempDir()
	u := &SelfUpdater{Dir: dir}

	versions := []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0", "1.4.0"}
	base := time.Now().Add(-time.Hour)
	for i, v := range versions {
		rd := u.releaseDir(v)
		if err := os.MkdirAll(rd, 0o755); err != nil {
			t.Fatal(err)
		}
		// Ascending mtime so 1.4.0 is newest.
		mt := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(rd, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	// current -> 1.0.0, previous -> 1.1.0 (both oldest, must survive regardless).
	if err := u.swapCurrent(u.releaseDir("1.0.0")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(u.previousFile(), []byte(u.releaseDir("1.1.0")), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := u.Prune(1); err != nil {
		t.Fatalf("prune: %v", err)
	}

	exists := func(v string) bool {
		_, err := os.Stat(u.releaseDir(v))
		return err == nil
	}
	// Protected (current, previous) + the 1 newest non-protected (1.4.0) survive.
	for _, keep := range []string{"1.0.0", "1.1.0", "1.4.0"} {
		if !exists(keep) {
			t.Fatalf("%s should have survived prune", keep)
		}
	}
	// 1.2.0 and 1.3.0 are stale and beyond keep=1 => removed.
	for _, gone := range []string{"1.2.0", "1.3.0"} {
		if exists(gone) {
			t.Fatalf("%s should have been pruned", gone)
		}
	}
}

func newQuarantineSim(t *testing.T) *Simulator {
	t.Helper()
	cfg := &Config{}
	cfg.Instance.ID = "node-1"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "mysoc"
	cfg.Simulation.StateFile = t.TempDir() + "/state.json"
	cfg.Simulation.ArtifactDir = t.TempDir()
	cfg.Server.URL = "http://127.0.0.1:1"
	sim, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	return sim
}

// TestDeferredFinalizeOnHeartbeat proves a booted new binary is not declared a
// successful self-update until a heartbeat succeeds.
func TestDeferredFinalizeOnHeartbeat(t *testing.T) {
	sim := newQuarantineSim(t)
	sim.binaryVersion = "2.0.0"
	sim.state.PendingSelfUpdate = &SelfUpdateState{FromVersion: "1.0.0", ToVersion: "2.0.0"}

	// ResolveSelfUpdate (boot) must NOT finalize the happy path.
	sim.ResolveSelfUpdate()
	if sim.state.PendingSelfUpdate == nil {
		t.Fatal("boot must not finalize a pending self-update before a heartbeat")
	}
	if sim.state.LastSelfUpdate != nil && sim.state.LastSelfUpdate.Success {
		t.Fatal("self-update must not be marked successful at boot")
	}

	// A successful heartbeat finalizes it.
	sim.finalizeSelfUpdateOnHeartbeat()
	if sim.state.PendingSelfUpdate != nil {
		t.Fatal("pending marker should be cleared after a successful heartbeat")
	}
	if sim.state.LastSelfUpdate == nil || !sim.state.LastSelfUpdate.Success {
		t.Fatal("self-update should be marked successful after the first heartbeat")
	}
}

// TestWatchdogQuarantinesFailedTarget proves a self-update that comes up as the
// wrong version quarantines the target version.
func TestWatchdogQuarantinesFailedTarget(t *testing.T) {
	sim := newQuarantineSim(t)
	sim.binaryVersion = "1.0.0" // came up as the OLD version => watchdog path
	sim.config.SelfUpdate.Dir = t.TempDir()
	sim.state.PendingSelfUpdate = &SelfUpdateState{FromVersion: "1.0.0", ToVersion: "2.0.0"}

	sim.ResolveSelfUpdate()
	if sim.state.QuarantinedSelfUpdateVersion != "2.0.0" {
		t.Fatalf("failed target not quarantined, got %q", sim.state.QuarantinedSelfUpdateVersion)
	}
	if sim.state.PendingSelfUpdate != nil {
		t.Fatal("pending marker should be cleared after the watchdog path")
	}
}
