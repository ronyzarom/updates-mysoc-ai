package updatersim

import (
	"testing"
	"time"
)

func newProgressTestSim(t *testing.T) *Simulator {
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

// TestCycleProgressingFreshAndStale checks the watchdog progress gate: a recent
// progress mark is alive; a mark older than cycle_timeout + grace is wedged.
func TestCycleProgressingFreshAndStale(t *testing.T) {
	sim := newProgressTestSim(t)

	// Fresh progress -> alive.
	sim.noteProgress()
	if !sim.cycleProgressing() {
		t.Fatal("fresh noteProgress should be considered progressing")
	}

	// Simulate a mark far enough in the past to exceed cycle_timeout + grace.
	stale := time.Now().Add(-(sim.config.cycleTimeout() + watchdogProgressGrace + time.Second))
	sim.lastProgress.Store(stale.UnixNano())
	if sim.cycleProgressing() {
		t.Fatal("a mark older than cycle_timeout+grace must read as wedged")
	}

	// A new cycle boundary revives it.
	sim.noteProgress()
	if !sim.cycleProgressing() {
		t.Fatal("noteProgress should revive the progress gate")
	}
}

// TestCycleProgressingUninitialized ensures the watchdog is not starved before
// the first cycle marks progress.
func TestCycleProgressingUninitialized(t *testing.T) {
	sim := newProgressTestSim(t)
	sim.lastProgress.Store(0)
	if !sim.cycleProgressing() {
		t.Fatal("uninitialized progress marker must not starve the watchdog at boot")
	}
}
