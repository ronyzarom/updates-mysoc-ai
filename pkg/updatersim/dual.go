package updatersim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// prerequisiteEvidence only executes the administrator-configured, product-owned
// verifier. It never tries a package manager, registry, or downloaded program.
func (s *Simulator) prerequisiteEvidence(ctx context.Context, product string) (artifactprotocol.Evidence, error) {
	p, ok := s.config.Product(product)
	if !ok || len(p.PrerequisiteVerifier) == 0 {
		return artifactprotocol.Evidence{}, fmt.Errorf("product prerequisite verifier is not configured")
	}
	if !filepath.IsAbs(p.PrerequisiteVerifier[0]) {
		return artifactprotocol.Evidence{}, fmt.Errorf("prerequisite verifier must use an absolute path")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.PrerequisiteVerifier[0], p.PrerequisiteVerifier[1:]...)
	out := &boundedEvidence{}
	cmd.Stdout = out
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return artifactprotocol.Evidence{}, fmt.Errorf("prerequisite verifier failed: %w", err)
	}
	var evidence artifactprotocol.Evidence
	decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, fmt.Errorf("invalid prerequisite evidence: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return evidence, fmt.Errorf("trailing prerequisite evidence")
	}
	if evidence.Lifecycle != "empty" && evidence.Lifecycle != "installed" {
		return evidence, fmt.Errorf("installation incomplete or unverifiable")
	}
	if evidence.Lifecycle == "empty" && evidence.InstalledVersion != "" {
		return evidence, fmt.Errorf("conflicting empty installation evidence")
	}
	if evidence.Lifecycle == "installed" && (evidence.InstalledVersion == "" || evidence.InstalledVersion != p.CurrentVersion) {
		return evidence, fmt.Errorf("installed version evidence mismatch")
	}
	return evidence, nil
}

type boundedEvidence struct{ bytes.Buffer }

func (b *boundedEvidence) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("prerequisite evidence too large")
	}
	return b.Buffer.Write(p)
}

func (s *Simulator) verifyDualOffer(ctx context.Context, offer *UpdateOffer) error {
	if offer.ProtocolVersion != "" && offer.ProtocolVersion != artifactprotocol.Version {
		return fmt.Errorf("unsupported dual artifact protocol")
	}
	if offer.SelectedArtifactKind == "" && len(offer.Artifacts) == 0 {
		return nil
	}
	// Older single-artifact servers may include an informational manifest entry
	// without negotiating this protocol. Never reinterpret it as a thin update.
	if offer.ProtocolVersion == "" && len(offer.Artifacts) <= 1 && (offer.SelectedArtifactKind == "" || offer.SelectedArtifactKind == "legacy" || offer.SelectedArtifactKind == "bootstrap") {
		return nil
	}
	if offer.ProtocolVersion != artifactprotocol.Version {
		return fmt.Errorf("unsupported dual artifact protocol")
	}
	if err := artifactprotocol.ValidateArtifacts(offer.Product, offer.LatestVersion, offer.Artifacts); err != nil {
		return err
	}
	os, arch := s.platform()
	var selected *types.Artifact
	for i := range offer.Artifacts {
		a := &offer.Artifacts[i]
		if a.Arch != os+"/"+arch {
			return fmt.Errorf("artifact architecture mismatch")
		}
		if err := artifactprotocol.Verify(s.publicKey, *a); err != nil {
			return fmt.Errorf("variant signature: %w", err)
		}
		if a.Kind == offer.SelectedArtifactKind {
			selected = a
		}
	}
	if selected == nil || selected.Checksum != offer.Checksum || selected.Signature != offer.Signature {
		return fmt.Errorf("selected artifact identity mismatch")
	}
	evidence, err := s.prerequisiteEvidence(ctx, offer.Product)
	if err != nil {
		return err
	}
	if selected.Kind == "update" {
		choice, status := artifactprotocol.Select(offer.Artifacts, evidence)
		if choice.Kind != "update" || status != "complete" {
			return fmt.Errorf("prerequisite validation %s; refusing update", status)
		}
	}
	return nil
}

func (s *Simulator) recordDualFailure(offer *UpdateOffer, err error) error {
	s.recordAttempt(Update{Product: offer.Product, FromVersion: offer.CurrentVersion, ToVersion: offer.LatestVersion, SelectedArtifactKind: offer.SelectedArtifactKind, DependencyValidation: "mismatch", ArtifactSHA256: offer.Checksum}, false, err.Error())
	if saveErr := SaveState(s.config.Simulation.StateFile, s.state); saveErr != nil {
		return fmt.Errorf("%w; persist failure: %v", err, saveErr)
	}
	return err
}
