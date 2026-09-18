package updatersim

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestObserverPrivilegedResponseParsing(t *testing.T) {
	valid := `{"protocol":"observer-unlinked-update-v1","observed_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}`
	if _, e := parseObserverResponse([]byte(valid)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{strings.Replace(valid, `"protocol":`, `"protocol":"wrong","protocol":`, 1), strings.TrimSuffix(valid, "}") + `,"health":{"ready":true,"ready":false}}`, valid + valid, strings.TrimSuffix(valid, "}") + `,"unexpected":true}`, strings.Replace(valid, time.Now().UTC().Format("2006"), "2000", 1)} {
		if _, e := parseObserverResponse([]byte(raw)); e == nil {
			t.Fatalf("invalid response accepted: %s", raw)
		}
	}
	if observerDigest(strings.Repeat("z", 64)) || observerDigest(strings.Repeat("A", 64)) || !observerDigest(strings.Repeat("a", 64)) {
		t.Fatal("digest validation")
	}
}

func observerSecurityFixture(t *testing.T) (*Simulator, Update) {
	t.Helper()
	dir := t.TempDir()
	e := newFSExecutor(t, filepath.Join(dir, "install"))
	artifact := filepath.Join(dir, "target.tar.gz")
	makeTarGz(t, artifact, map[string]string{"VERSION": "3.3.152.38"})
	e.RestartCommand = []string{"/must-not-execute-product-through-mutable-path"}
	e.HealthCommand = e.RestartCommand
	cfg := &Config{Products: []ProductConfig{{Name: "siemcore", ServerType: "observer-unlinked", CurrentVersion: "3.3.152.37"}}}
	cfg.Simulation.StateFile = filepath.Join(dir, "state.json")
	cfg.Simulation.Filesystem.ObserverUnlinkedUpdate = true
	cfg.Simulation.Filesystem.InstallRoot = "/opt/siemcore-cascade"
	cfg.Simulation.Filesystem.RestartCommand = []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}
	cfg.Simulation.Filesystem.HealthCommand = cfg.Simulation.Filesystem.RestartCommand
	return &Simulator{config: cfg, state: &State{}, executor: e}, Update{Product: "siemcore", FromVersion: "3.3.152.37", ToVersion: "3.3.152.38", ArtifactPath: artifact, ArtifactSHA256: strings.Repeat("a", 64), ArtifactSignature: "fixture-signature"}
}

func TestObserverAcceptedPointersAndReadOnlyRetry(t *testing.T) {
	s, u := observerSecurityFixture(t)
	calls := []string{}
	s.observerAdapterCall = func(_ context.Context, a string, q observerRequest) (observerResponse, error) {
		calls = append(calls, a)
		if a == "readiness" {
			return observerResponse{ServerType: "observer-unlinked", AdapterManifestSHA256: strings.Repeat("b", 64), Capabilities: []string{observerUpdateProtocol}}, nil
		}
		return observerResponse{OperationID: q.OperationID, OperationSHA256: strings.Repeat("c", 64), TargetVersion: q.Target.Version, ArtifactSHA256: q.Target.SHA256, Phase: "accepted"}, nil
	}
	if e := s.applyObserverSecurityUpdate(context.Background(), u); e != nil {
		t.Fatal(e)
	}
	op := s.state.ObserverUpdateOperation.OperationID
	if e := s.applyObserverSecurityUpdate(context.Background(), u); e != nil {
		t.Fatal(e)
	}
	if strings.Join(calls, ",") != "readiness,apply,status" || s.state.ObserverUpdateOperation.OperationID != op {
		t.Fatal(calls)
	}
	changed := u
	changed.ArtifactSHA256 = strings.Repeat("d", 64)
	if s.applyObserverSecurityUpdate(context.Background(), changed) == nil {
		t.Fatal("replaced retained target")
	}
}

func TestObserverFailureReconciliationDoesNotRequireParentOrReadiness(t *testing.T) {
	s, u := observerSecurityFixture(t)
	calls := []string{}
	s.state.ObserverUpdateOperation = &ObserverUpdateOperation{OperationID: "b71bd9f1-45f1-4aa2-bd4e-bd043937a552", FromVersion: u.FromVersion, TargetVersion: u.ToVersion, SHA256: u.ArtifactSHA256, Signature: u.ArtifactSignature, ArtifactPath: u.ArtifactPath, Phase: "uncertain"}
	s.observerAdapterCall = func(_ context.Context, a string, q observerRequest) (observerResponse, error) {
		calls = append(calls, a)
		phase := "recovery_required"
		if a == "recover" {
			phase = "blocked"
		}
		return observerResponse{OperationID: q.OperationID, OperationSHA256: strings.Repeat("c", 64), TargetVersion: q.Target.Version, ArtifactSHA256: q.Target.SHA256, Phase: phase, ErrorCode: "predecessor_ui_unprotected"}, nil
	}
	pending, e := s.resumePendingIndependentObserver(context.Background())
	if !pending || e == nil || strings.Join(calls, ",") != "status,recover" {
		t.Fatal(pending, e, calls)
	}
	if s.state.LastUpdateAttempt.Success {
		t.Fatal("claimed success")
	}
	if _, e = os.Lstat(s.executor.(*FilesystemExecutor).currentLink("siemcore")); !os.IsNotExist(e) {
		t.Fatal("failure published pointer")
	}
}

func TestObserverUncertainWorkerRetainsDurableIdentity(t *testing.T) {
	s, u := observerSecurityFixture(t)
	s.observerAdapterCall = func(_ context.Context, a string, q observerRequest) (observerResponse, error) {
		if a == "readiness" {
			return observerResponse{ServerType: "observer-unlinked", AdapterManifestSHA256: strings.Repeat("b", 64), Capabilities: []string{observerUpdateProtocol}}, nil
		}
		return observerResponse{}, errors.New("adapter transport failed")
	}
	if s.applyObserverSecurityUpdate(context.Background(), u) == nil {
		t.Fatal("uncertainty ignored")
	}
	raw, e := os.ReadFile(s.config.Simulation.StateFile)
	if e != nil {
		t.Fatal(e)
	}
	var state State
	if e = json.Unmarshal(raw, &state); e != nil {
		t.Fatal(e)
	}
	if state.ObserverUpdateOperation.Phase != "uncertain" || state.ObserverUpdateOperation.OperationID == "" {
		t.Fatal(state.ObserverUpdateOperation)
	}
}

func TestObserverOptInCannotAffectNormal(t *testing.T) {
	s, _ := observerSecurityFixture(t)
	s.config.Products[0].ServerType = "normal"
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("Observer executor accepted Normal")
	}
	s.config.Simulation.Filesystem.ObserverUnlinkedUpdate = false
	if e := s.validateSiemCoreExecution(); e != nil {
		t.Fatal("Normal default changed", e)
	}
	called := false
	s.observerAdapterCall = func(context.Context, string, observerRequest) (observerResponse, error) {
		called = true
		return observerResponse{}, nil
	}
	_, caps, e := s.podCapabilities(context.Background(), "siemcore")
	if e != nil || len(caps) != 0 || called {
		t.Fatal("Normal queried Observer", e)
	}
}
