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

func TestNodePrivilegedResponseParsing(t *testing.T) {
	valid := `{"protocol":"pod-node-update-v1","observed_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}`
	if _, e := parseNodeResponse([]byte(valid)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{strings.Replace(valid, `"protocol":`, `"protocol":"wrong","protocol":`, 1), strings.TrimSuffix(valid, "}") + `,"health":{"ready":true,"ready":false}}`, valid + valid, strings.TrimSuffix(valid, "}") + `,"unexpected":true}`, strings.Replace(valid, time.Now().UTC().Format("2006"), "2000", 1)} {
		if _, e := parseNodeResponse([]byte(raw)); e == nil {
			t.Fatalf("invalid response accepted: %s", raw)
		}
	}
	if nodeDigest(strings.Repeat("z", 64)) || nodeDigest(strings.Repeat("A", 64)) || !nodeDigest(strings.Repeat("a", 64)) {
		t.Fatal("digest validation")
	}
}

func nodeSecurityFixture(t *testing.T) (*Simulator, Update) {
	t.Helper()
	dir := t.TempDir()
	e := newFSExecutor(t, filepath.Join(dir, "install"))
	artifact := filepath.Join(dir, "target.tar.gz")
	makeTarGz(t, artifact, map[string]string{"VERSION": "3.3.152.38"})
	e.RestartCommand = []string{"/must-not-execute-product-through-mutable-path"}
	e.HealthCommand = e.RestartCommand
	cfg := &Config{Products: []ProductConfig{{Name: "siemcore", ServerType: "pod-node", NodeID: "1", CurrentVersion: "3.3.152.37"}}}
	cfg.Simulation.StateFile = filepath.Join(dir, "state.json")
	cfg.Simulation.Filesystem.IndependentNodeUpdate = true
	cfg.Simulation.Filesystem.IndependentNodeBootstrap = true
	cfg.Simulation.Filesystem.InstallRoot = "/opt/siemcore-cascade"
	cfg.Simulation.Filesystem.RestartCommand = []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}
	cfg.Simulation.Filesystem.HealthCommand = cfg.Simulation.Filesystem.RestartCommand
	return &Simulator{config: cfg, state: &State{}, executor: e}, Update{Product: "siemcore", FromVersion: "3.3.152.37", ToVersion: "3.3.152.38", ArtifactPath: artifact, ArtifactSHA256: strings.Repeat("a", 64), ArtifactSignature: "fixture-signature"}
}

func TestNodeAcceptedPointersAndReadOnlyRetry(t *testing.T) {
	s, u := nodeSecurityFixture(t)
	calls := []string{}
	s.nodeAdapterCall = func(_ context.Context, a string, q nodeRequest) (nodeResponse, error) {
		calls = append(calls, a)
		if a == "readiness" {
			return nodeResponse{ServerType: "pod-node", NodeID: "1", AdapterManifestSHA256: strings.Repeat("b", 64), Capabilities: []string{nodeUpdateProtocol}}, nil
		}
		return nodeResponse{OperationID: q.OperationID, OperationSHA256: strings.Repeat("c", 64), TargetVersion: q.Target.Version, ArtifactSHA256: q.Target.SHA256, Phase: "accepted"}, nil
	}
	if e := s.applyIndependentNodeUpdate(context.Background(), u); e != nil {
		t.Fatal(e)
	}
	op := s.state.NodeUpdateOperation.OperationID
	if e := s.applyIndependentNodeUpdate(context.Background(), u); e != nil {
		t.Fatal(e)
	}
	if strings.Join(calls, ",") != "readiness,apply,status" || s.state.NodeUpdateOperation.OperationID != op {
		t.Fatal(calls)
	}
	changed := u
	changed.ArtifactSHA256 = strings.Repeat("d", 64)
	if s.applyIndependentNodeUpdate(context.Background(), changed) == nil {
		t.Fatal("replaced retained target")
	}
}

func TestNodeFailureReconciliationDoesNotRequireParentOrReadiness(t *testing.T) {
	s, u := nodeSecurityFixture(t)
	calls := []string{}
	s.state.NodeUpdateOperation = &NodeUpdateOperation{OperationID: "b71bd9f1-45f1-4aa2-bd4e-bd043937a552", FromVersion: u.FromVersion, TargetVersion: u.ToVersion, SHA256: u.ArtifactSHA256, Signature: u.ArtifactSignature, ArtifactPath: u.ArtifactPath, Phase: "uncertain"}
	s.nodeAdapterCall = func(_ context.Context, a string, q nodeRequest) (nodeResponse, error) {
		calls = append(calls, a)
		phase := "recovery_required"
		if a == "recover" {
			phase = "blocked"
		}
		return nodeResponse{OperationID: q.OperationID, OperationSHA256: strings.Repeat("c", 64), TargetVersion: q.Target.Version, ArtifactSHA256: q.Target.SHA256, Phase: phase, ErrorCode: "predecessor_ui_unprotected"}, nil
	}
	pending, e := s.resumePendingIndependentNode(context.Background())
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

func TestNodeUncertainWorkerRetainsDurableIdentity(t *testing.T) {
	s, u := nodeSecurityFixture(t)
	s.nodeAdapterCall = func(_ context.Context, a string, q nodeRequest) (nodeResponse, error) {
		if a == "readiness" {
			return nodeResponse{ServerType: "pod-node", NodeID: "1", AdapterManifestSHA256: strings.Repeat("b", 64), Capabilities: []string{nodeUpdateProtocol}}, nil
		}
		return nodeResponse{}, errors.New("adapter transport failed")
	}
	if s.applyIndependentNodeUpdate(context.Background(), u) == nil {
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
	if state.NodeUpdateOperation.Phase != "uncertain" || state.NodeUpdateOperation.OperationID == "" {
		t.Fatal(state.NodeUpdateOperation)
	}
}

func TestNodeOptInCannotAffectNormal(t *testing.T) {
	s, _ := nodeSecurityFixture(t)
	s.config.Products[0].ServerType = "normal"
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("Node executor accepted Normal")
	}
	s.config.Simulation.Filesystem.IndependentNodeUpdate = false
	s.config.Simulation.Filesystem.IndependentNodeBootstrap = false
	if e := s.validateSiemCoreExecution(); e != nil {
		t.Fatal("Normal default changed", e)
	}
	called := false
	s.nodeAdapterCall = func(context.Context, string, nodeRequest) (nodeResponse, error) {
		called = true
		return nodeResponse{}, nil
	}
	_, caps, e := s.podCapabilities(context.Background(), "siemcore")
	if e != nil || len(caps) != 0 || called {
		t.Fatal("Normal queried Node", e)
	}
}

func TestNodeNextAutomaticUpgradeRetainsAcceptedPredecessor(t *testing.T) {
	s, u := nodeSecurityFixture(t)
	s.state.NodeUpdateOperation = &NodeUpdateOperation{OperationID: "bd12f2a6-bfbe-46ae-bbd8-7f3ce341f8e8", TargetVersion: u.FromVersion, Phase: "accepted"}
	old := s.state.NodeUpdateOperation.OperationID
	s.nodeAdapterCall = func(_ context.Context, a string, q nodeRequest) (nodeResponse, error) {
		if a == "readiness" {
			return nodeResponse{ServerType: "pod-node", NodeID: "1", AdapterManifestSHA256: strings.Repeat("b", 64), Capabilities: []string{nodeUpdateProtocol}}, nil
		}
		if q.OperationID == old {
			t.Fatal("reused prior operation identity")
		}
		return nodeResponse{OperationID: q.OperationID, OperationSHA256: strings.Repeat("c", 64), TargetVersion: q.Target.Version, ArtifactSHA256: q.Target.SHA256, Phase: "accepted"}, nil
	}
	if err := s.applyIndependentNodeUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if s.state.NodeUpdateOperation.TargetVersion != u.ToVersion {
		t.Fatal("next upgrade not retained")
	}
}

func TestNodeReadinessRejectsWrongSlotAndObserver(t *testing.T) {
	for _, role := range []string{"normal", "observer-unlinked", "pod-node"} {
		s, _ := nodeSecurityFixture(t)
		s.nodeAdapterCall = func(context.Context, string, nodeRequest) (nodeResponse, error) {
			return nodeResponse{ServerType: role, NodeID: "2", AdapterManifestSHA256: strings.Repeat("b", 64), Capabilities: []string{nodeUpdateProtocol}}, nil
		}
		if _, err := s.nodeUpdateReady(context.Background()); err == nil {
			t.Fatalf("wrong readiness accepted: %s", role)
		}
	}
}
func TestNodeRetainedOperationCannotChangeChannel(t *testing.T) {
	s, u := nodeSecurityFixture(t)
	s.state.NodeUpdateOperation = &NodeUpdateOperation{OperationID: "b71bd9f1-45f1-4aa2-bd4e-bd043937a552", FromVersion: u.FromVersion, TargetVersion: u.ToVersion, SHA256: u.ArtifactSHA256, Signature: u.ArtifactSignature, Channel: "original", Phase: "uncertain"}
	s.nodeAdapterCall = func(context.Context, string, nodeRequest) (nodeResponse, error) {
		t.Fatal("changed offer invoked adapter")
		return nodeResponse{}, nil
	}
	if err := s.applyIndependentNodeUpdate(context.Background(), u); err == nil {
		t.Fatal("channel changed")
	}
}
