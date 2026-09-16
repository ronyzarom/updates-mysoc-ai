package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPodResumeWithoutOfferAndNoReplay(t *testing.T) {
	d := t.TempDir()
	dir := filepath.Join(d, "journal")
	os.MkdirAll(dir, 0700)
	artifact := filepath.Join(d, "artifact")
	os.WriteFile(artifact, []byte("signed fixture"), 0600)
	sum := sha256.Sum256([]byte("signed fixture"))
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	b := podmaintenance.Binding{Protocol: podmaintenance.Protocol, OperationID: "op-1", PodID: "pod-1", NodeID: "1", UpdaterID: "updater-1", Product: "siemcore", FromVersion: "1", TargetVersion: "2", ArtifactPath: artifact, ArtifactSHA256: hex.EncodeToString(sum[:]), PreviousArtifactSHA256: strings.Repeat("b", 64), Deadline: time.Now().Add(-time.Minute)}
	b.ArtifactSignature = signing.Sign(priv, b.Product, b.TargetVersion, b.ArtifactSHA256)
	h := podmaintenance.Health{Version: "2", SHA256: b.ArtifactSHA256, Role: "STBY", ManagementReady: true, ProcessingDisabled: true, TrafficDisabled: true, AuthorityValid: true, ReplicationStatus: "ready"}
	j := podmaintenance.Journal{Binding: b, Generation: 4, Phase: "completed", Health: &h}
	raw, _ := json.Marshal(j)
	os.WriteFile(filepath.Join(dir, "operation.json"), raw, 0600)
	response := podmaintenance.Response{Capabilities: []string{podmaintenance.Protocol}, Binding: b, Generation: 4, Phase: "completed", PermissionExpires: time.Now().Add(time.Hour), Acceptance: &podmaintenance.Acceptance{Health: h, ServiceHealthy: true}}
	raw, _ = json.Marshal(response)
	out := filepath.Join(d, "response")
	os.WriteFile(out, raw, 0600)
	script := filepath.Join(d, "adapter")
	os.WriteFile(script, []byte("#!/bin/sh\ncase \"$1\" in capabilities|status|acceptance) cat '"+out+"';; *) exit 77;; esac\n"), 0700)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.Contains(r.URL.Path, "check") {
			t.Error("must not need offer")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	cfg := &Config{}
	cfg.Server.URL = server.URL
	cfg.Server.MaxResponseBytes = 1 << 20
	cfg.Instance.ID = "updater-1"
	cfg.Products = []ProductConfig{{Name: "siemcore", CurrentVersion: "1"}}
	cfg.Simulation.StateFile = filepath.Join(d, "state.json")
	cfg.Simulation.Filesystem.PodMaintenance = &PodMaintenanceConfig{PodID: "pod-1", NodeID: "1", JournalDirectory: dir, AdapterCommand: []string{script}}
	client, e := NewClient(cfg.Server)
	if e != nil {
		t.Fatal(e)
	}
	s := &Simulator{config: cfg, client: client, state: &State{}, publicKey: pub}
	pending, e := s.resumePendingPod(context.Background())
	if !pending || e != nil {
		t.Fatal(pending, e)
	}
	if s.state.ProductVersions["siemcore"] != "2" || requests != 1 {
		t.Fatal("not reported accepted")
	}
	after, e := podmaintenance.ReadJournal(dir)
	if e != nil || after.Phase != "accepted" {
		t.Fatal(after, e)
	}
	pending, e = s.resumePendingPod(context.Background())
	if pending || e != nil {
		t.Fatal("already accepted replay", e)
	}
}
func TestPodRetainedArtifactVerification(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	p := filepath.Join(t.TempDir(), "artifact")
	os.WriteFile(p, []byte("changed"), 0600)
	u := Update{Product: "siemcore", ToVersion: "2", ArtifactPath: p, ArtifactSHA256: strings.Repeat("a", 64)}
	u.ArtifactSignature = signing.Sign(priv, u.Product, u.ToVersion, u.ArtifactSHA256)
	s := Simulator{publicKey: pub}
	if s.verifyRetainedPodArtifact(u) == nil {
		t.Fatal("tampered artifact accepted")
	}
	s.publicKey = nil
	if s.verifyRetainedPodArtifact(u) == nil {
		t.Fatal("missing trust accepted")
	}
}
