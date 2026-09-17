package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRememberSiemCoreTypes(t *testing.T) {
	for _, kind := range []string{"normal", "pod-active", "pod-stby", "pod-observer"} {
		t.Run(kind, func(t *testing.T) {
			cfg := &Config{}
			cfg.Simulation.StateFile = filepath.Join(t.TempDir(), "state.json")
			p := ProductConfig{Name: "siemcore", ServerType: kind}
			if kind != "normal" {
				p.PodID = "pod"
				p.NodeID = "1"
				if kind == "pod-observer" {
					p.NodeID = "witness"
				}
			}
			cfg.Products = []ProductConfig{p}
			state := &State{}
			if e := rememberSiemCoreInstallation(cfg, state); e != nil {
				t.Fatal(e)
			}
			loaded, e := LoadState(cfg.Simulation.StateFile)
			if e != nil {
				t.Fatal(e)
			}
			cfg.Products = []ProductConfig{{Name: "siemcore"}}
			if e = rememberSiemCoreInstallation(cfg, loaded); e != nil {
				t.Fatal(e)
			}
			if cfg.Products[0].ServerType != kind || cfg.Products[0].PodID != p.PodID || cfg.Products[0].NodeID != p.NodeID {
				t.Fatal("identity not restored")
			}
			cfg.Products[0].ServerType = "normal"
			cfg.Products[0].PodID = ""
			cfg.Products[0].NodeID = ""
			cfg.Products[0].DeploymentRole = ""
			if kind != "normal" && rememberSiemCoreInstallation(cfg, loaded) == nil {
				t.Fatal("silently downgraded pod")
			}
		})
	}
}
func TestPodTypesNeverUseNormalFallback(t *testing.T) {
	for _, kind := range []string{"pod-active", "pod-stby", "pod-observer"} {
		s := Simulator{config: &Config{Products: []ProductConfig{{Name: "siemcore", ServerType: kind, PodID: "pod", NodeID: "1"}}}}
		if s.validateSiemCoreExecution() == nil {
			t.Fatal("normal fallback", kind)
		}
	}
}
func TestPodDrainStartupWithoutOriginOrOffer(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	key, _, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	b := podmaintenance.Binding{Protocol: podmaintenance.Protocol, OperationID: "original", PodID: "pod", NodeID: "1", UpdaterID: "updater", Product: "siemcore", FromVersion: "1", TargetVersion: "2", ArtifactPath: "/unavailable/target", ArtifactSignature: "retained", ArtifactSHA256: strings.Repeat("a", 64), PreviousArtifactSHA256: strings.Repeat("b", 64), Deadline: now.Add(-time.Hour)}
	raw, _ := json.Marshal(podmaintenance.Journal{Binding: b, Phase: "intent"})
	os.WriteFile(filepath.Join(dir, "operation.json"), raw, 0600)
	response := podmaintenance.DrainResponse{Protocol: podmaintenance.DrainProtocol, Binding: b, Generation: 42, CapturedOwner: podmaintenance.CapturedOwner{NodeID: "1", OwnerGeneration: 7, WatchdogGeneration: 7}, Phase: "draining", ObservedAt: now, ValidUntil: now.Add(50 * time.Second)}
	raw, _ = json.Marshal(response)
	out := filepath.Join(dir, "response.json")
	os.WriteFile(out, raw, 0600)
	script := filepath.Join(dir, "adapter")
	os.WriteFile(script, []byte("#!/bin/sh\ncase \"$1\" in status) cat '"+out+"';; *) exit 77;; esac\n"), 0700)
	cfg := &Config{}
	cfg.Instance.ID = "updater"
	cfg.Products = []ProductConfig{{Name: "siemcore", CurrentVersion: "1", ServerType: "pod-stby", PodID: "pod", NodeID: "1"}}
	cfg.Simulation.Filesystem.PodMaintenance = &PodMaintenanceConfig{PodID: "pod", NodeID: "1", JournalDirectory: dir, Drain: &PodDrainConfig{Protocol: podmaintenance.DrainProtocol, ObserverPublicKey: hex.EncodeToString(key), AdapterCommand: []string{script}}}
	// Deliberately no network client or artifact: restart discovery must not need either.
	s := Simulator{config: cfg, state: &State{}}
	for i := 0; i < 2; i++ {
		pending, e := s.resumePendingPod(context.Background())
		if !pending || e == nil || !strings.Contains(e.Error(), "drain draining") {
			t.Fatal(pending, e)
		}
	}
	j, e := podmaintenance.ReadJournal(dir)
	if e != nil || j.Generation != 42 || j.Binding != b || j.Phase != "intent" {
		t.Fatal(j, e)
	}
	if cfg.Products[0].CurrentVersion != "1" || s.state.LastUpdateAttempt != nil {
		t.Fatal("discovery reported application success")
	}
}
