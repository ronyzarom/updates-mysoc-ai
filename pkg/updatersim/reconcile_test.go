package updatersim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

var errForcedStageFailure = errors.New("forced stage failure")

// reconcileTestServer records heartbeats and captures the last update report.
type reconcileTestServer struct {
	*httptest.Server
	mu         sync.Mutex
	heartbeats int
	reports    []UpdateReportRequest
}

func newReconcileTestServer(t *testing.T) *reconcileTestServer {
	t.Helper()
	rts := &reconcileTestServer{}
	rts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			rts.mu.Lock()
			rts.heartbeats++
			rts.mu.Unlock()
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		case "/api/v1/updates/siemcore/report":
			var report UpdateReportRequest
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Fatalf("decode report: %v", err)
			}
			rts.mu.Lock()
			rts.reports = append(rts.reports, report)
			rts.mu.Unlock()
			writeTestJSON(t, w, map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected request: %s", r.URL.Path)
		}
	}))
	return rts
}

func (rts *reconcileTestServer) lastReport(t *testing.T) UpdateReportRequest {
	t.Helper()
	rts.mu.Lock()
	defer rts.mu.Unlock()
	if len(rts.reports) == 0 {
		t.Fatal("no report received")
	}
	return rts.reports[len(rts.reports)-1]
}

// stageFailReconciler is a safe no-op reconciler that fails at one named stage
// and records whether rollback ran.
type stageFailReconciler struct {
	NoopExecutor
	fail             string
	rolledBack       bool
	selfUpdateCalled bool
}

func (e *stageFailReconciler) Migrate(_ context.Context, _ Plan) error {
	if e.fail == StageMigrate {
		return errForcedStageFailure
	}
	return nil
}

func (e *stageFailReconciler) HealthCheck(_ context.Context, _ Plan) error {
	if e.fail == StageHealth {
		return errForcedStageFailure
	}
	return nil
}

func (e *stageFailReconciler) SecurityCheck(_ context.Context, _ Plan) error {
	if e.fail == StageSecurity {
		return errForcedStageFailure
	}
	return nil
}

func (e *stageFailReconciler) SelfUpdate(_ context.Context, _ Plan) error {
	e.selfUpdateCalled = true
	if e.fail == StageSelfUpdate {
		return errForcedStageFailure
	}
	return nil
}

func (e *stageFailReconciler) RollbackReconcile(_ context.Context, _ Plan) error {
	e.rolledBack = true
	return nil
}

func TestReconcileSimulateHappyPathCommitsAndReports(t *testing.T) {
	t.Parallel()
	server := newReconcileTestServer(t)
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	simulator, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}
	manifest := validTestManifest()

	if err := simulator.RunReconcile(context.Background(), ModeReal, manifest); err != nil {
		t.Fatalf("run reconcile: %v", err)
	}

	report := server.lastReport(t)
	if !report.Success || report.Kind != reportKindReconcile ||
		report.Stage != StageCommit || report.ToVersion != "2.2.0" {
		t.Fatalf("unexpected report: %#v", report)
	}

	state, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.SystemRelease != "2.2.0" ||
		state.DBSchemaVersion != manifest.RequiredDBSchema ||
		state.ContainerVersions["siemcore-api"] != "2.2.0" ||
		state.ConfigHashes["/opt/siemcore/config.yaml"] != "abc123" ||
		state.UpdaterVersion != manifest.SelfUpdate.Version {
		t.Fatalf("state not committed: %#v", state)
	}
	if state.PendingSelfUpdate != nil {
		t.Fatalf("pending self-update not cleared: %#v", state.PendingSelfUpdate)
	}
	if state.LastReconcile == nil || !state.LastReconcile.Success ||
		state.LastReconcile.Stage != StageCommit {
		t.Fatalf("unexpected reconcile status: %#v", state.LastReconcile)
	}

	// A second reconcile against the same manifest must be a no-op.
	server.mu.Lock()
	reportsBefore := len(server.reports)
	server.mu.Unlock()
	if err := simulator.RunReconcile(context.Background(), ModeReal, manifest); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	server.mu.Lock()
	reportsAfter := len(server.reports)
	server.mu.Unlock()
	if reportsAfter != reportsBefore {
		t.Fatalf("settled reconcile should not report: before=%d after=%d", reportsBefore, reportsAfter)
	}
}

func TestReconcileStageFailureRollsBackAndReports(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{StageMigrate, StageHealth, StageSecurity} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			server := newReconcileTestServer(t)
			defer server.Close()

			cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
			executor := &stageFailReconciler{fail: stage}
			simulator, err := NewSimulator(cfg, executor, discardLogger())
			if err != nil {
				t.Fatalf("new simulator: %v", err)
			}

			err = simulator.RunReconcile(context.Background(), ModeReal, validTestManifest())
			if err == nil {
				t.Fatalf("expected reconcile failure at %s", stage)
			}
			if !executor.rolledBack {
				t.Fatalf("stage %s failure did not roll back", stage)
			}

			report := server.lastReport(t)
			if report.Success || report.Stage != stage || report.Kind != reportKindReconcile {
				t.Fatalf("unexpected failure report: %#v", report)
			}

			state, err := LoadState(cfg.Simulation.StateFile)
			if err != nil {
				t.Fatalf("load state: %v", err)
			}
			if state.SystemRelease == "2.2.0" {
				t.Fatalf("failed reconcile must not commit release: %#v", state)
			}
			if state.LastReconcile == nil || state.LastReconcile.Success ||
				state.LastReconcile.Stage != stage {
				t.Fatalf("unexpected reconcile status: %#v", state.LastReconcile)
			}
		})
	}
}

func TestReconcileSelfUpdateWatchdogRestoresOnHandoffFailure(t *testing.T) {
	t.Parallel()
	server := newReconcileTestServer(t)
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	previousUpdater := cfg.Instance.UpdaterVersion
	executor := &stageFailReconciler{fail: StageSelfUpdate}
	simulator, err := NewSimulator(cfg, executor, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}

	err = simulator.RunReconcile(context.Background(), ModeReal, validTestManifest())
	if err == nil {
		t.Fatal("expected self-update handoff failure")
	}
	if !executor.selfUpdateCalled || !executor.rolledBack {
		t.Fatalf("watchdog did not run: called=%v rolledBack=%v", executor.selfUpdateCalled, executor.rolledBack)
	}

	state, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.PendingSelfUpdate != nil {
		t.Fatalf("watchdog must clear pending self-update: %#v", state.PendingSelfUpdate)
	}
	if state.UpdaterVersion == validTestManifest().SelfUpdate.Version {
		t.Fatal("failed self-update must not advance updater version")
	}
	if cfg.Instance.UpdaterVersion != previousUpdater {
		t.Fatalf("updater version changed on failed handoff: %s", cfg.Instance.UpdaterVersion)
	}

	report := server.lastReport(t)
	if report.Success || report.Stage != StageSelfUpdate {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestReconcileObserveModePlansOnly(t *testing.T) {
	t.Parallel()
	server := newReconcileTestServer(t)
	defer server.Close()

	cfg := newSimulatorTestConfig(t, server.URL, ModeObserve)
	simulator, err := NewSimulator(cfg, NoopExecutor{}, discardLogger())
	if err != nil {
		t.Fatalf("new simulator: %v", err)
	}

	if err := simulator.RunReconcile(context.Background(), ModeObserve, validTestManifest()); err != nil {
		t.Fatalf("observe reconcile: %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.heartbeats != 1 {
		t.Fatalf("observe should send exactly one heartbeat, got %d", server.heartbeats)
	}
	if len(server.reports) != 0 {
		t.Fatalf("observe should not report: %#v", server.reports)
	}

	state, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.SystemRelease != "" {
		t.Fatalf("observe must not commit state: %#v", state)
	}
}
