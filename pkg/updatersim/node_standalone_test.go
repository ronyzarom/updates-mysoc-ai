package updatersim

import (
	"context"
	"strings"
	"testing"
)

func standaloneFixture(t *testing.T) (*Simulator, Update) {
	s, u := nodeSecurityFixture(t)
	s.config.Simulation.Filesystem.IndependentNodeUpdate = false
	s.config.Simulation.Filesystem.IndependentNodeStandalone = true
	return s, u
}

func TestStandaloneRootOperationAndRoutineUpdateBlock(t *testing.T) {
	s, u := standaloneFixture(t)
	const operation = "b71bd9f1-45f1-4aa2-bd4e-bd043937a552"
	var calls []string
	s.standaloneAdapterCall = func(_ context.Context, action string, q standaloneRequest) (standaloneResponse, error) {
		calls = append(calls, action)
		r := standaloneResponse{OperationID: operation, OperationSHA256: strings.Repeat("c", 64), TargetVersion: u.ToVersion, ArtifactSHA256: u.ArtifactSHA256, Phase: "accepted"}
		if action == "readiness" {
			r.ServerType, r.NodeID = "pod-node", "1"
			r.AdapterManifestSHA256 = strings.Repeat("b", 64)
			r.Capabilities = []string{nodeStandaloneProtocol}
		} else if q.OperationID != operation {
			t.Fatal("did not use protected root operation")
		}
		return r, nil
	}
	if err := s.applyIndependentStandalone(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if s.state.NodeStandaloneOperation.OperationID != operation {
		t.Fatal("operation changed")
	}
	next := u
	next.FromVersion = u.ToVersion
	next.ToVersion = "3.3.152.99"
	if err := s.applyIndependentStandalone(context.Background(), next); err == nil {
		t.Fatal("unqualified routine update allowed")
	}
	if strings.Join(calls, ",") != "readiness,apply" {
		t.Fatal(calls)
	}
}

func TestStandaloneCannotUseNormalOrManagementExecutor(t *testing.T) {
	s, _ := standaloneFixture(t)
	if err := s.validateSiemCoreExecution(); err != nil {
		t.Fatal(err)
	}
	s.config.Simulation.Filesystem.IndependentNodeUpdate = true
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("two executors enabled")
	}
	s.config.Simulation.Filesystem.IndependentNodeUpdate = false
	s.config.Products[0].ServerType = "normal"
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("Normal accepted standalone executor")
	}
}

func TestStandaloneRetainsButDoesNotResumeAcceptedManagementHistory(t *testing.T) {
	s, u := standaloneFixture(t)
	s.state.NodeUpdateOperation = &NodeUpdateOperation{Phase: "accepted", TargetVersion: u.FromVersion}
	pending, err := s.resumePendingIndependentNode(context.Background())
	if pending || err != nil {
		t.Fatal(pending, err)
	}
	s.state.NodeUpdateOperation.Phase = "uncertain"
	pending, err = s.resumePendingIndependentNode(context.Background())
	if !pending || err == nil {
		t.Fatal("unresolved management history ignored")
	}
}
