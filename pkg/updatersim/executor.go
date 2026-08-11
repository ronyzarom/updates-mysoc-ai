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

// ReconcilingExecutor is the integration seam for desired-state reconciliation
// driven by a system-template.json manifest. It is a superset of Executor:
// product teams implement it to apply the eight updater capabilities (code
// change, database migration, multi-container orchestration, self-update,
// configuration render, and health/security verification) as gated stages that
// the agent core sequences and rolls back.
//
// Every stage MUST be safe to run more than once and MUST NOT commit
// irreversibly until HealthCheck and SecurityCheck pass. The agent core takes a
// restore point in Prepare and calls RollbackReconcile on any failure.
type ReconcilingExecutor interface {
	// Prepare guards the operation: take binary backups, snapshot or expand the
	// database, and acquire the single-writer migration lock.
	Prepare(context.Context, Plan) error
	// Migrate applies database migrations, ideally expand/contract and locked.
	Migrate(context.Context, Plan) error
	// ApplyContainers rolls the container set in dependency order with per
	// container readiness gates.
	ApplyContainers(context.Context, Plan) error
	// RenderConfig materializes configuration templates from the manifest.
	RenderConfig(context.Context, Plan) error
	// SelfUpdate replaces the running updater using an OS-native or helper
	// mechanism that can swap a live executable.
	SelfUpdate(context.Context, Plan) error
	// HealthCheck verifies liveness, readiness, and expected versions. It gates
	// commit versus rollback.
	HealthCheck(context.Context, Plan) error
	// SecurityCheck verifies the post-change security posture. It gates commit
	// versus rollback.
	SecurityCheck(context.Context, Plan) error
	// RollbackReconcile restores the pre-change state captured in Prepare.
	RollbackReconcile(context.Context, Plan) error
}

// NoopExecutor simulates lifecycle latency without changing the machine. It
// satisfies both Executor and ReconcilingExecutor so the simulator can exercise
// the full reconcile pipeline safely by default.
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

// Prepare simulates taking a restore point.
func (e NoopExecutor) Prepare(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// Migrate simulates applying database migrations.
func (e NoopExecutor) Migrate(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// ApplyContainers simulates rolling the container set.
func (e NoopExecutor) ApplyContainers(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// RenderConfig simulates rendering configuration templates.
func (e NoopExecutor) RenderConfig(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// SelfUpdate simulates replacing the updater and confirming it heartbeats.
func (e NoopExecutor) SelfUpdate(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// HealthCheck simulates a passing post-change health gate.
func (e NoopExecutor) HealthCheck(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// SecurityCheck simulates a passing post-change security gate.
func (e NoopExecutor) SecurityCheck(ctx context.Context, _ Plan) error {
	return waitForSimulation(ctx, e.Delay)
}

// RollbackReconcile simulates restoring the pre-change state.
func (e NoopExecutor) RollbackReconcile(ctx context.Context, _ Plan) error {
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
