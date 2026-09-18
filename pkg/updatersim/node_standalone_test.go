package updatersim

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestStandaloneSuccessorRequiresExactRestoredRootBinding(t *testing.T) {
	for _, bad := range []string{"", "digest", "identity", "same"} {
		t.Run(bad, func(t *testing.T) {
			s, u := standaloneFixture(t)
			old := NodeStandaloneOperation{OperationID: "b71bd9f1-45f1-4aa2-bd4e-bd043937a552", OperationSHA256: strings.Repeat("a", 64), Phase: "restored", FromVersion: u.FromVersion, TargetVersion: u.ToVersion}
			s.state.NodeStandaloneOperation = &old
			s.standaloneAdapterCall = func(_ context.Context, action string, q standaloneRequest) (standaloneResponse, error) {
				if action != "readiness" {
					t.Fatal("must only read successor authorization")
				}
				r := standaloneResponse{OperationID: "7441fb5b-0f35-4709-ad20-45ba9009f794", PreviousOperationID: old.OperationID, PreviousOperationSHA256: old.OperationSHA256, ServerType: "pod-node", NodeID: "1", AdapterManifestSHA256: strings.Repeat("b", 64)}
				if bad == "digest" {
					r.PreviousOperationSHA256 = strings.Repeat("c", 64)
				}
				if bad == "identity" {
					r.PreviousOperationID = r.OperationID
				}
				if bad == "same" {
					r.OperationID = old.OperationID
				}
				return r, nil
			}
			pending, err := s.resumePendingIndependentStandalone(context.Background())
			if bad != "" {
				if !pending || err == nil || s.state.NodeStandaloneOperation != &old || len(s.state.NodeStandaloneHistory) != 0 {
					t.Fatal("invalid successor admitted")
				}
				return
			}
			if pending || err != nil || s.state.NodeStandaloneOperation != nil || len(s.state.NodeStandaloneHistory) != 1 || s.state.NodeStandaloneHistory[0] != old {
				t.Fatal("restored history not preserved", pending, err)
			}
		})
	}
}

func TestRestoredStandaloneStillChecksSignedSelfUpdate(t *testing.T) {
	heartbeats, selfChecks, productChecks := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/heartbeat" {
			heartbeats++
			writeTestJSON(t, w, HeartbeatResponse{Status: "ok"})
		} else if strings.Contains(r.URL.Path, "/updates/updater-") {
			selfChecks++
			writeTestJSON(t, w, UpdateCheckResponse{})
		} else {
			productChecks++
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s, u := standaloneFixture(t)
	cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
	s.client, _ = NewClient(cfg.Server)
	s.logger = discardLogger()
	s.binaryVersion = "1.16.1.33"
	s.config.SelfUpdate.Channel = "stable"
	old := &NodeStandaloneOperation{OperationID: "b71bd9f1-45f1-4aa2-bd4e-bd043937a552", OperationSHA256: strings.Repeat("a", 64), Phase: "restored", FromVersion: u.FromVersion, TargetVersion: u.ToVersion}
	s.state.NodeStandaloneOperation = old
	s.standaloneAdapterCall = func(_ context.Context, action string, q standaloneRequest) (standaloneResponse, error) {
		if action != "readiness" {
			t.Fatal("restored operation must not reapply", action)
		}
		return standaloneResponse{OperationID: old.OperationID, ServerType: "pod-node", NodeID: "1", AdapterManifestSHA256: strings.Repeat("b", 64)}, nil
	}
	if err := s.RunCycle(context.Background(), ModeReal); err == nil {
		t.Fatal("product hold disappeared")
	}
	if heartbeats != 1 || selfChecks != 1 || productChecks != 0 || s.state.NodeStandaloneOperation != old {
		t.Fatalf("heartbeat=%d self=%d products=%d", heartbeats, selfChecks, productChecks)
	}
}
