package updatersim

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentObserverIdentityAndExecutionIsolation(t *testing.T) {
	cfg := &Config{Products: []ProductConfig{{Name: "siemcore", ServerType: "observer-unlinked"}}}
	cfg.Simulation.StateFile = filepath.Join(t.TempDir(), "state.json")
	state := &State{}
	if err := rememberSiemCoreInstallation(cfg, state); err != nil {
		t.Fatal(err)
	}
	s := &Simulator{config: cfg, state: state}
	identity := s.installationIdentity()
	if identity == nil || identity.Kind != "observer-unlinked" || identity.PodID != "" || identity.NodeID != "" {
		t.Fatal(identity)
	}
	if s.validateSiemCoreExecution() == nil {
		t.Fatal("unprotected executor allowed")
	}
	cfg.Simulation.Filesystem.InstallRoot = "/opt/siemcore-cascade"
	cfg.Simulation.Filesystem.RestartCommand = []string{"sudo", "-n", "/usr/local/sbin/siemcore-apply-update"}
	cfg.Simulation.Filesystem.HealthCommand = append([]string{}, cfg.Simulation.Filesystem.RestartCommand...)
	if err := s.validateSiemCoreExecution(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Products = []ProductConfig{{Name: "siemcore"}}
	if err = rememberSiemCoreInstallation(cfg, loaded); err != nil {
		t.Fatal(err)
	}
	if cfg.Products[0].ServerType != "observer-unlinked" {
		t.Fatal("identity lost")
	}
	cfg.Products[0].ServerType = "normal"
	cfg.Products[0].DeploymentRole = ""
	if rememberSiemCoreInstallation(cfg, loaded) == nil {
		t.Fatal("reclassified as normal")
	}
}

func TestIndependentObserverRetainedRetry(t *testing.T) {
	for _, kind := range []string{"observer", "node"} {
		for _, failure := range []string{"apply", "health"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				dir := t.TempDir()
				e := newFSExecutor(t, filepath.Join(dir, "install"))
				marker := filepath.Join(dir, "fail")
				os.WriteFile(marker, []byte("fail"), 0600)
				hook := filepath.Join(dir, "hook")
				os.WriteFile(hook, []byte("#!/bin/sh\nif [ \"$UPDATER_PHASE\" = \""+failure+"\" ] && [ -e '"+marker+"' ]; then exit 1; fi\n"), 0700)
				e.RestartCommand = []string{hook}
				e.HealthCommand = []string{hook}
				artifact := filepath.Join(dir, "bundle.tar.gz")
				makeTarGz(t, artifact, map[string]string{"VERSION": "3.3.152.40"})
				update := Update{Product: "siemcore", FromVersion: "0.0.0", ToVersion: "3.3.152.40", ArtifactPath: artifact, ArtifactSHA256: "digest", ArtifactSignature: "signature"}
				s := &Simulator{executor: e}
				apply := s.applyIndependentObserver
				if kind == "node" {
					apply = s.applyIndependentNode
				}
				if apply(context.Background(), update) == nil {
					t.Fatal("failure ignored")
				}
				retained := resolveCurrent(t, e, "siemcore")
				receipt := filepath.Join(retained, ".updater-release.json")
				before, _ := os.ReadFile(receipt)
				os.Remove(marker)
				if err := apply(context.Background(), update); err != nil {
					t.Fatal(err)
				}
				after, _ := os.ReadFile(receipt)
				if string(before) != string(after) {
					t.Fatal("retry rewrote transaction")
				}
				changed := update
				changed.ArtifactSHA256 = "other"
				if apply(context.Background(), changed) == nil {
					t.Fatal("changed artifact accepted")
				}
				changed = update
				changed.ToVersion = "3.3.152.41"
				if apply(context.Background(), changed) == nil {
					t.Fatal("changed version accepted")
				}
				changed = update
				changed.FromVersion = "3.3.152.39"
				if apply(context.Background(), changed) == nil {
					t.Fatal("ordinary upgrade accepted")
				}
			})
		}
	}

}

func TestIndependentObserverFailureNeverClaimsRollback(t *testing.T) {
	for _, kind := range []string{"observer-unlinked", "pod-node"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeTestJSON(t, w, map[string]string{"status": "ok"}) }))
			defer server.Close()
			cfg := newSimulatorTestConfig(t, server.URL, ModeReal)
			cfg.Products = []ProductConfig{{Name: "siemcore", ServerType: kind}}
			if kind == "pod-node" {
				cfg.Products[0].NodeID = "1"
			}
			e := &recordingExecutor{}
			s, err := NewSimulator(cfg, e, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			err = s.failAndRollback(context.Background(), Update{Product: "siemcore", ToVersion: "3.3.152.40"}, errors.New("health failure"))
			if err == nil || e.rolledBack {
				t.Fatal("rollback invoked", err, e)
			}
			if s.state.LastUpdateAttempt.Success || !strings.Contains(s.state.LastUpdateAttempt.Error, "retained for exact retry") {
				t.Fatal(s.state.LastUpdateAttempt)
			}

		})
	}
}
