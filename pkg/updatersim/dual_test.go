package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestDualVerificationLocalPrerequisites(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		t.Run(product, func(t *testing.T) {
			cfg := newSimulatorTestConfig(t, "https://unused.invalid", ModeReal)
			cfg.Products[0].Name = product
			evidencePath := filepath.Join(t.TempDir(), "evidence.json")
			cfg.Products[0].PrerequisiteVerifier = []string{"/bin/cat", evidencePath}
			sim, err := NewSimulator(cfg, &recordingExecutor{}, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			sim.publicKey = pub
			osname, arch := sim.platform()
			a := types.Artifact{Product: product, Version: "1.1.0", Kind: "bootstrap", Name: "boot", Arch: osname + "/" + arch, SourceCommit: strings.Repeat("a", 40), Checksum: strings.Repeat("b", 64), Size: 1}
			b := a
			b.Kind = "update"
			b.Name = "thin"
			b.RequiredDependencies = []types.Dependency{{Reference: "local-component", Digest: "sha256:" + strings.Repeat("c", 64)}}
			artifactprotocol.Sign(key, &a)
			artifactprotocol.Sign(key, &b)
			offer := &UpdateOffer{Product: product, LatestVersion: b.Version, SelectedArtifactKind: "update", ProtocolVersion: artifactprotocol.Version, Artifacts: []types.Artifact{a, b}, Checksum: b.Checksum, Signature: b.Signature}
			write := func(e artifactprotocol.Evidence) {
				raw, _ := json.Marshal(e)
				if err := os.WriteFile(evidencePath, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: cfg.Products[0].CurrentVersion, Dependencies: b.RequiredDependencies})
			if err := sim.verifyDualOffer(context.Background(), offer); err != nil {
				t.Fatal(err)
			}
			write(artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: cfg.Products[0].CurrentVersion, Dependencies: []types.Dependency{}})
			if sim.verifyDualOffer(context.Background(), offer) == nil {
				t.Fatal("changed dependencies accepted")
			}
			write(artifactprotocol.Evidence{Lifecycle: "partial"})
			if sim.verifyDualOffer(context.Background(), offer) == nil {
				t.Fatal("interrupted installation accepted")
			}
			write(artifactprotocol.Evidence{Lifecycle: "empty"})
			offer.SelectedArtifactKind = "bootstrap"
			offer.Checksum = a.Checksum
			offer.Signature = a.Signature
			if err := sim.verifyDualOffer(context.Background(), offer); err != nil {
				t.Fatal(err)
			}
			offer.ProtocolVersion = "unknown"
			if sim.verifyDualOffer(context.Background(), offer) == nil {
				t.Fatal("unknown protocol accepted")
			}
			if err := sim.verifyDualOffer(context.Background(), &UpdateOffer{SelectedArtifactKind: "bootstrap", Artifacts: []types.Artifact{{Kind: "bootstrap"}}}); err != nil {
				t.Fatal("old informational single-artifact rejected", err)
			}
			if err := sim.verifyDualOffer(context.Background(), &UpdateOffer{}); err != nil {
				t.Fatal("legacy response rejected", err)
			}
		})
	}
}

func TestIndependentVerificationLocalPrerequisites(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	for _, product := range []string{"mysoc", "siemcore", "swf"} {
		t.Run(product, func(t *testing.T) {
			cfg := newSimulatorTestConfig(t, "https://unused.invalid", ModeReal)
			cfg.Products[0].Name = product
			evidencePath := filepath.Join(t.TempDir(), "evidence.json")
			cfg.Products[0].PrerequisiteVerifier = []string{"/bin/cat", evidencePath}
			sim, err := NewSimulator(cfg, &recordingExecutor{}, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			sim.publicKey = pub
			osname, arch := sim.platform()
			a := types.Artifact{Product: product, Version: "1.1.0", Kind: "bootstrap", Name: "boot", Arch: osname + "/" + arch, SourceCommit: strings.Repeat("a", 40), Checksum: strings.Repeat("b", 64), Size: 1}
			b := a
			b.Kind = "update"
			b.Name = "thin"
			b.RequiredDependencies = []types.Dependency{{Reference: "local-component", Digest: "sha256:" + strings.Repeat("c", 64)}}
			artifactprotocol.Sign(key, &a)
			artifactprotocol.Sign(key, &b)
			offer := &UpdateOffer{Product: product, LatestVersion: b.Version, SelectedArtifactKind: "update", ProtocolVersion: artifactprotocol.Version, Artifacts: []types.Artifact{b}, Checksum: b.Checksum, Signature: b.Signature}
			write := func(e artifactprotocol.Evidence) {
				raw, _ := json.Marshal(e)
				if err := os.WriteFile(evidencePath, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: cfg.Products[0].CurrentVersion, Dependencies: b.RequiredDependencies})
			if err := sim.verifyDualOffer(context.Background(), offer); err != nil {
				t.Fatal(err)
			}
			write(artifactprotocol.Evidence{Lifecycle: "installed", InstalledVersion: cfg.Products[0].CurrentVersion, Dependencies: []types.Dependency{}})
			if sim.verifyDualOffer(context.Background(), offer) == nil {
				t.Fatal("changed dependencies accepted")
			}
			write(artifactprotocol.Evidence{Lifecycle: "partial"})
			if sim.verifyDualOffer(context.Background(), offer) == nil {
				t.Fatal("interrupted installation accepted")
			}
			write(artifactprotocol.Evidence{Lifecycle: "empty"})
			offer.Artifacts = []types.Artifact{a}
			offer.SelectedArtifactKind = "bootstrap"
			offer.Checksum = a.Checksum
			offer.Signature = a.Signature
			if err := sim.verifyDualOffer(context.Background(), offer); err != nil {
				t.Fatal(err)
			}
			offer.ProtocolVersion = "unknown"
			if sim.verifyDualOffer(context.Background(), offer) == nil {
				t.Fatal("unknown protocol accepted")
			}
			if err := sim.verifyDualOffer(context.Background(), &UpdateOffer{SelectedArtifactKind: "bootstrap", Artifacts: []types.Artifact{{Kind: "bootstrap"}}}); err != nil {
				t.Fatal("old informational single-artifact rejected", err)
			}
			if err := sim.verifyDualOffer(context.Background(), &UpdateOffer{}); err != nil {
				t.Fatal("legacy response rejected", err)
			}
		})
	}
}
