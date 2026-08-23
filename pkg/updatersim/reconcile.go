package updatersim

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Reconcile pipeline stage identifiers, reported as monitoring telemetry.
const (
	StagePrepare    = "prepare"
	StageMigrate    = "migrate"
	StageContainers = "containers"
	StageConfig     = "config"
	StageSelfUpdate = "self_update"
	StageHealth     = "health"
	StageSecurity   = "security"
	StageCommit     = "commit"
)

// reportKindReconcile marks results produced by the reconcile pipeline.
const reportKindReconcile = "reconcile"

// RunReconcile drives one desired-state reconciliation from a manifest through a
// staged, health-gated, rollback-capable pipeline. It never mutates the machine
// itself: every stage is delegated to the configured ReconcilingExecutor, which
// defaults to a safe no-op. Modes mirror RunCycle:
//
//   - observe:  compute and log the plan only.
//   - download: run every stage but do not commit state or report success.
//   - simulate: run every stage, commit simulated state, and report the result.
func (s *Simulator) RunReconcile(ctx context.Context, mode Mode, manifest *SystemTemplate) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	if !s.cycleMu.TryLock() {
		return ErrCycleInProgress
	}
	defer s.cycleMu.Unlock()

	if mode == "" {
		mode = s.config.Simulation.Mode
	}
	switch mode {
	case ModeObserve, ModeDownload, ModeReal:
	default:
		return fmt.Errorf("invalid cycle mode %q", mode)
	}

	current := s.currentState()
	plan, err := Reconcile(current, manifest)
	if err != nil {
		return fmt.Errorf("plan reconcile: %w", err)
	}

	s.logger.Info(
		"reconcile plan computed",
		"product", plan.Product,
		"release", plan.Release,
		"actions", len(plan.Actions),
		"signed", manifest.Signed(),
		"mode", mode,
	)
	for _, action := range plan.Actions {
		s.logger.Info(
			"reconcile action",
			"kind", action.Kind,
			"target", action.Target,
			"from", action.FromVersion,
			"to", action.ToVersion,
		)
	}

	// Heartbeat first so the server observes the agent before any change.
	if _, err := s.SendHeartbeat(ctx); err != nil {
		return err
	}

	if !plan.HasWork() {
		s.logger.Info("system already at desired state", "release", plan.Release)
		return nil
	}
	if mode == ModeObserve {
		s.logger.Info("observe mode; not applying reconcile plan", "release", plan.Release)
		return nil
	}

	reconciler, ok := s.executor.(ReconcilingExecutor)
	if !ok {
		reconciler = NoopExecutor{}
	}

	fromRelease := s.state.SystemRelease

	if err := s.runStage(ctx, plan, StagePrepare, reconciler.Prepare); err != nil {
		return s.failReconcile(ctx, plan, StagePrepare, err, reconciler)
	}
	if plan.Has(ActionMigrate) {
		if err := s.runStage(ctx, plan, StageMigrate, reconciler.Migrate); err != nil {
			return s.failReconcile(ctx, plan, StageMigrate, err, reconciler)
		}
	}
	if err := s.runStage(ctx, plan, StageContainers, reconciler.ApplyContainers); err != nil {
		return s.failReconcile(ctx, plan, StageContainers, err, reconciler)
	}
	if plan.Has(ActionRenderConfig) {
		if err := s.runStage(ctx, plan, StageConfig, reconciler.RenderConfig); err != nil {
			return s.failReconcile(ctx, plan, StageConfig, err, reconciler)
		}
	}
	if plan.Has(ActionSelfUpdate) {
		if err := s.runSelfUpdate(ctx, plan, reconciler); err != nil {
			return s.failReconcile(ctx, plan, StageSelfUpdate, err, reconciler)
		}
	}
	if err := s.runStage(ctx, plan, StageHealth, reconciler.HealthCheck); err != nil {
		return s.failReconcile(ctx, plan, StageHealth, err, reconciler)
	}
	if err := s.runStage(ctx, plan, StageSecurity, reconciler.SecurityCheck); err != nil {
		return s.failReconcile(ctx, plan, StageSecurity, err, reconciler)
	}

	if mode == ModeDownload {
		s.logger.Info("download mode; staged but not committed", "release", plan.Release)
		return nil
	}

	s.commitReconcile(plan, manifest)
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		return err
	}
	if err := s.client.ReportUpdate(ctx, plan.Product, UpdateReportRequest{
		InstanceID:  s.config.Instance.ID,
		FromVersion: fromRelease,
		ToVersion:   plan.Release,
		Success:     true,
		Kind:        reportKindReconcile,
		Stage:       StageCommit,
	}); err != nil {
		return fmt.Errorf("report reconcile: %w", err)
	}
	s.logger.Info(
		"reconcile completed",
		"product", plan.Product,
		"from_release", fromRelease,
		"to_release", plan.Release,
	)
	return nil
}

// runStage executes one gated stage and records its outcome for monitoring.
func (s *Simulator) runStage(
	ctx context.Context,
	plan Plan,
	stage string,
	execute func(context.Context, Plan) error,
) error {
	s.logger.Info("reconcile stage started", "stage", stage, "release", plan.Release)
	if err := execute(ctx, plan); err != nil {
		s.logger.Error("reconcile stage failed", "stage", stage, "error", err)
		return err
	}
	s.recordReconcile(plan.Release, stage, true, "")
	s.logger.Info("reconcile stage completed", "stage", stage, "release", plan.Release)
	return nil
}

// runSelfUpdate performs the two-phase updater self-update: it persists a
// pending marker before the handoff so a crash mid-swap is recoverable, then
// hands off to the executor. If the handoff fails, a watchdog restores the
// previous updater instead of leaving the agent wedged.
func (s *Simulator) runSelfUpdate(
	ctx context.Context,
	plan Plan,
	reconciler ReconcilingExecutor,
) error {
	var fromVersion, toVersion string
	for _, action := range plan.Actions {
		if action.Kind == ActionSelfUpdate {
			fromVersion = action.FromVersion
			toVersion = action.ToVersion
			break
		}
	}

	s.logger.Info("reconcile stage started", "stage", StageSelfUpdate, "release", plan.Release)

	// Phase 1: stage and persist the pending self-update before handing off.
	s.state.PendingSelfUpdate = &SelfUpdateState{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		StartedAt:   time.Now().UTC(),
	}
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		return err
	}

	// Phase 2: hand off to the replacement updater and wait for it to check in.
	if err := reconciler.SelfUpdate(ctx, plan); err != nil {
		s.logger.Error(
			"self-update handoff failed; watchdog restoring previous updater",
			"from", fromVersion,
			"to", toVersion,
			"error", err,
		)
		s.state.PendingSelfUpdate = nil
		if saveErr := SaveState(s.config.Simulation.StateFile, s.state); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		return err
	}

	// Finalize: the new updater started and heartbeats.
	s.config.Instance.UpdaterVersion = toVersion
	s.state.UpdaterVersion = toVersion
	s.state.PendingSelfUpdate = nil
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		return err
	}
	s.recordReconcile(plan.Release, StageSelfUpdate, true, "")
	s.logger.Info(
		"self-update finalized; new updater heartbeating",
		"version", toVersion,
		"release", plan.Release,
	)
	return nil
}

// failReconcile rolls back, records the failed stage, reports it, and joins all
// resulting errors.
func (s *Simulator) failReconcile(
	ctx context.Context,
	plan Plan,
	stage string,
	cause error,
	reconciler ReconcilingExecutor,
) error {
	rollbackErr := reconciler.RollbackReconcile(ctx, plan)
	message := fmt.Sprintf("stage=%s: %v", stage, cause)
	if rollbackErr != nil {
		message += "; rollback: " + rollbackErr.Error()
	}

	s.recordReconcile(plan.Release, stage, false, message)
	stateErr := SaveState(s.config.Simulation.StateFile, s.state)
	reportErr := s.client.ReportUpdate(ctx, plan.Product, UpdateReportRequest{
		InstanceID:  s.config.Instance.ID,
		FromVersion: s.state.SystemRelease,
		ToVersion:   plan.Release,
		Success:     false,
		Error:       message,
		Kind:        reportKindReconcile,
		Stage:       stage,
	})
	return errors.Join(cause, rollbackErr, stateErr, reportErr)
}

// commitReconcile advances simulated state to reflect the applied plan.
func (s *Simulator) commitReconcile(plan Plan, manifest *SystemTemplate) {
	if s.state.ContainerVersions == nil {
		s.state.ContainerVersions = make(map[string]string)
	}
	if s.state.ConfigHashes == nil {
		s.state.ConfigHashes = make(map[string]string)
	}

	configSHAByPath := make(map[string]string, len(manifest.ConfigTemplates))
	for _, template := range manifest.ConfigTemplates {
		configSHAByPath[template.Path] = template.SHA256
	}

	for _, action := range plan.Actions {
		switch action.Kind {
		case ActionMigrate:
			s.state.DBSchemaVersion = action.ToVersion
		case ActionContainerAdd, ActionContainerUpgrade:
			s.state.ContainerVersions[action.Target] = action.ToVersion
		case ActionContainerRemove:
			delete(s.state.ContainerVersions, action.Target)
		case ActionRenderConfig:
			s.state.ConfigHashes[action.Target] = configSHAByPath[action.Target]
		}
	}
	s.state.SystemRelease = plan.Release
	s.recordReconcile(plan.Release, StageCommit, true, "")
}

// currentState builds the agent's view of installed state from durable state.
func (s *Simulator) currentState() CurrentState {
	containers := make(map[string]string, len(s.state.ContainerVersions))
	for name, version := range s.state.ContainerVersions {
		containers[name] = version
	}
	configHashes := make(map[string]string, len(s.state.ConfigHashes))
	for path, sha := range s.state.ConfigHashes {
		configHashes[path] = sha
	}
	updaterVersion := s.config.Instance.UpdaterVersion
	if s.state.UpdaterVersion != "" {
		updaterVersion = s.state.UpdaterVersion
	}
	return CurrentState{
		Containers:     containers,
		DBSchema:       s.state.DBSchemaVersion,
		ConfigHashes:   configHashes,
		UpdaterVersion: updaterVersion,
	}
}

// recordReconcile updates the in-memory per-stage monitoring status.
func (s *Simulator) recordReconcile(release, stage string, success bool, message string) {
	s.state.LastReconcile = &ReconcileStatus{
		Release:   release,
		Stage:     stage,
		Success:   success,
		Error:     message,
		Timestamp: time.Now().UTC(),
	}
}
