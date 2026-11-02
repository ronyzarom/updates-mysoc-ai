package updatersim

import (
	"context"
	"testing"
)

// rollbackSpyExecutor records whether Rollback was invoked and whether the
// context it received was still live (not already cancelled).
type rollbackSpyExecutor struct {
	rolledBack    bool
	ctxWasLive    bool
	rollbackError error
}

func (e *rollbackSpyExecutor) Apply(context.Context, Update) error    { return nil }
func (e *rollbackSpyExecutor) Validate(context.Context, Update) error { return nil }
func (e *rollbackSpyExecutor) Rollback(ctx context.Context, _ Update) error {
	e.rolledBack = true
	e.ctxWasLive = ctx.Err() == nil
	return e.rollbackError
}

// TestFailAndRollbackRunsOnCancelledCtx proves the production fix: when the
// cycle/product deadline has already fired, failAndRollback still runs the
// rollback hook on a live, detached context instead of a dead one.
func TestFailAndRollbackRunsOnCancelledCtx(t *testing.T) {
	spy := &rollbackSpyExecutor{}
	cfg := &Config{}
	cfg.Instance.ID = "node-1"
	cfg.Instance.Type = "server"
	cfg.Instance.ProductTier = "mysoc"
	cfg.Simulation.StateFile = t.TempDir() + "/state.json"
	cfg.Simulation.ArtifactDir = t.TempDir()
	cfg.Server.URL = "http://127.0.0.1:1"

	sim, err := NewSimulator(cfg, spy, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	// An already-cancelled context, as if the product deadline fired mid-apply.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	update := Update{Product: "mysoc", FromVersion: "1.0.0", ToVersion: "1.1.0"}
	_ = sim.failAndRollback(ctx, update, context.DeadlineExceeded)

	if !spy.rolledBack {
		t.Fatal("rollback was not attempted after a cancelled-context failure")
	}
	if !spy.ctxWasLive {
		t.Fatal("rollback received an already-cancelled context; the hook could not run")
	}
}
