package updatersim

import (
	"context"
	"time"
)

// Update describes verified work passed to a product-specific executor.
type Update struct {
	Product        string
	FromVersion    string
	ToVersion      string
	Channel        string
	UpdateGroup    string
	ReleaseNotes   string
	ArtifactPath   string
	ArtifactSHA256 string
	SimulationOnly bool
}

// Executor is the integration seam for a real SiemCore or SWF updater.
//
// The simulator never executes downloaded artifacts itself. Product teams can
// replace NoopExecutor with an implementation that applies, validates, and
// rolls back their product according to the updater guidelines.
type Executor interface {
	Apply(context.Context, Update) error
	Validate(context.Context, Update) error
	Rollback(context.Context, Update) error
}

// NoopExecutor simulates lifecycle latency without changing the machine.
type NoopExecutor struct {
	Delay time.Duration
}

// Apply waits for the configured delay and performs no installation.
func (e NoopExecutor) Apply(ctx context.Context, _ Update) error {
	return waitForSimulation(ctx, e.Delay)
}

// Validate waits for the configured delay and reports simulated success.
func (e NoopExecutor) Validate(ctx context.Context, _ Update) error {
	return waitForSimulation(ctx, e.Delay)
}

// Rollback waits for the configured delay and performs no rollback.
func (e NoopExecutor) Rollback(ctx context.Context, _ Update) error {
	return waitForSimulation(ctx, e.Delay)
}

func waitForSimulation(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
