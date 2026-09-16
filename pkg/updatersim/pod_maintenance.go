package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	"io"
	"os"
	"path/filepath"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
)

func (s *Simulator) applyPodMaintenance(ctx context.Context, u Update) error {
	cfg := s.config.Simulation.Filesystem.PodMaintenance
	if cfg == nil {
		return fmt.Errorf("pod maintenance not configured")
	}
	if u.Product != "siemcore" {
		return fmt.Errorf("pod maintenance only configured for siemcore")
	}
	// The coordinator validates the retained journal again under its exclusive lock.
	// This read only recovers the predecessor binding after an interrupted apply.
	previous := ""

	j, err := podmaintenance.ReadJournal(cfg.JournalDirectory)
	if err == nil {
		if j.Phase != "accepted" {
			if j.Binding.Product != u.Product || j.Binding.TargetVersion != u.ToVersion || j.Binding.ArtifactSHA256 != u.ArtifactSHA256 {
				return fmt.Errorf("another pod operation requires recovery")
			}
			previous = j.Binding.PreviousArtifactSHA256
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if previous == "" {
		// First coordinated upgrade requires a retained predecessor. Clean installation
		// uses the separate bootstrap contract, never an invented rollback digest.
		var meta releaseMetadata
		path := filepath.Join(s.config.Simulation.Filesystem.InstallRoot, sanitizeSegment(u.Product), "current", ".updater-release.json")
		if err := readJSONFile(path, &meta); err != nil {
			return fmt.Errorf("retained predecessor: %w", err)
		}
		if meta.Product != u.Product || meta.Version != u.FromVersion {
			return fmt.Errorf("predecessor identity mismatch")
		}
		previous = meta.SHA256
	}
	coordinator := podmaintenance.Coordinator{Directory: cfg.JournalDirectory, Adapter: podmaintenance.CommandAdapter{Command: cfg.AdapterCommand, Timeout: s.config.Simulation.Filesystem.CommandTimeout.Duration}}
	if err := s.verifyRetainedPodArtifact(u); err != nil {
		return err
	}
	return coordinator.Run(ctx, podmaintenance.Binding{ArtifactSignature: u.ArtifactSignature, Protocol: podmaintenance.Protocol, PodID: cfg.PodID, NodeID: cfg.NodeID, UpdaterID: s.config.Instance.ID, Product: u.Product, FromVersion: u.FromVersion, TargetVersion: u.ToVersion, ArtifactPath: u.ArtifactPath, ArtifactSHA256: u.ArtifactSHA256, PreviousArtifactSHA256: previous})
}

func (s *Simulator) verifyRetainedPodArtifact(u Update) error {
	if len(s.publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("pod maintenance requires pinned signing key")
	}
	if err := signing.Verify(s.publicKey, u.Product, u.ToVersion, u.ArtifactSHA256, u.ArtifactSignature); err != nil {
		return err
	}
	f, err := os.Open(u.ArtifactPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != u.ArtifactSHA256 {
		return fmt.Errorf("retained pod artifact checksum mismatch")
	}
	return nil
}

// resumePendingPod reconciles only the existing bound operation, independently
// of new offers or origin availability. It never substitutes a newer release.
func (s *Simulator) resumePendingPod(ctx context.Context) (bool, error) {
	cfg := s.config.Simulation.Filesystem.PodMaintenance
	if cfg == nil {
		return false, nil
	}
	j, err := podmaintenance.ReadJournal(cfg.JournalDirectory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	b := j.Binding
	if b.PodID != cfg.PodID || b.NodeID != cfg.NodeID || b.UpdaterID != s.config.Instance.ID || b.Product != "siemcore" {
		return true, fmt.Errorf("retained maintenance identity does not match configuration")
	}
	p, ok := s.config.Product(b.Product)
	if !ok {
		return true, fmt.Errorf("retained product not configured")
	}
	if j.Phase == "accepted" && p.CurrentVersion == b.TargetVersion {
		return false, nil
	}
	if handled, err := s.resumePodRecovery(ctx, j); handled {
		return true, err
	}
	if j.Phase == "rolled-back" {
		return false, nil
	}
	u := Update{Product: b.Product, FromVersion: b.FromVersion, ToVersion: b.TargetVersion, ArtifactPath: b.ArtifactPath, ArtifactSHA256: b.ArtifactSHA256, ArtifactSignature: b.ArtifactSignature}
	if err = s.verifyRetainedPodArtifact(u); err != nil {
		return true, err
	}
	c := podmaintenance.Coordinator{Directory: cfg.JournalDirectory, Adapter: podmaintenance.CommandAdapter{Command: cfg.AdapterCommand, Timeout: s.config.Simulation.Filesystem.CommandTimeout.Duration}}
	err = c.Run(ctx, b)
	s.recordAttempt(u, err == nil, func() string {
		if err != nil {
			return err.Error()
		}
		return ""
	}())
	saveErr := SaveState(s.config.Simulation.StateFile, s.state)
	reportErr := s.client.ReportUpdate(ctx, b.Product, UpdateReportRequest{InstanceID: s.config.Instance.ID, FromVersion: b.FromVersion, ToVersion: b.TargetVersion, ArtifactDigest: b.ArtifactSHA256, Success: err == nil, Kind: "pod_maintenance", Stage: "reconciliation", Error: func() string {
		if err != nil {
			return err.Error()
		}
		return ""
	}()})
	return true, errors.Join(err, saveErr, reportErr)
}

var ErrPodPredecessorRestored = errors.New("pod recovery accepted the verified predecessor; target upgrade failed")

func (s *Simulator) resumePodRecovery(ctx context.Context, j podmaintenance.Journal) (bool, error) {
	cfg := s.config.Simulation.Filesystem.PodMaintenance
	prior, priorErr := podmaintenance.ReadRecoveryJournal(cfg.JournalDirectory)
	hasPrior := priorErr == nil
	if priorErr != nil && !os.IsNotExist(priorErr) {
		return true, priorErr
	}
	if hasPrior && j.Phase == "rolled-back" && prior.Phase == "accepted" && prior.Outcome == podmaintenance.PredecessorRestored && prior.Binding == j.Binding && prior.Generation == j.Generation {
		if p, ok := s.config.Product(j.Binding.Product); ok && p.CurrentVersion == j.Binding.FromVersion && s.state.ProductVersions[j.Binding.Product] == j.Binding.FromVersion {
			return false, nil
		}
	}
	if cfg.Recovery == nil {
		if hasPrior {
			return true, fmt.Errorf("v2 recovery journal requires explicitly configured recovery-v2 adapter")
		}
		return false, nil
	}
	rc := cfg.Recovery
	if rc.Protocol != podmaintenance.RecoveryProtocol {
		return true, fmt.Errorf("unsupported pod recovery protocol")
	}
	key, err := signing.ParsePublicKeyHex(rc.ObserverPublicKey)
	if err != nil {
		return true, err
	}
	var auth podmaintenance.RecoveryAuthorization
	var artifact podmaintenance.RetainedArtifact
	if hasPrior {
		auth = prior.Authorization
		artifact = prior.Artifact
	}
	if rc.AuthorizationFile != "" {
		incoming, e := podmaintenance.ReadRecoveryAuthorization(rc.AuthorizationFile)
		if e == nil {
			auth = incoming
		} else if !hasPrior || !os.IsNotExist(e) {
			return true, e
		}
	}
	if rc.ArtifactReceiptFile != "" {
		incoming, e := podmaintenance.ReadRetainedArtifact(rc.ArtifactReceiptFile)
		if e == nil {
			artifact = incoming
		} else if !hasPrior || !os.IsNotExist(e) {
			return true, e
		}
	}
	if auth.PayloadBase64 == "" {
		return false, nil
	}
	c := podmaintenance.RecoveryCoordinator{Directory: cfg.JournalDirectory, Adapter: podmaintenance.CommandAdapter{Command: cfg.AdapterCommand, Timeout: s.config.Simulation.Filesystem.CommandTimeout.Duration}, ObserverKey: key, ReleaseKey: s.publicKey}
	outcome, err := c.Run(ctx, auth, artifact)
	b := j.Binding
	u := Update{Product: b.Product, FromVersion: b.FromVersion, ToVersion: b.TargetVersion, ArtifactSHA256: b.ArtifactSHA256, ArtifactSignature: b.ArtifactSignature}
	success := err == nil && outcome == podmaintenance.TargetInstalled
	if err == nil && outcome == podmaintenance.PredecessorRestored {
		// Report actual accepted predecessor without ever recording target success.
		if s.state.ProductVersions == nil {
			s.state.ProductVersions = map[string]string{}
		}
		s.state.ProductVersions[b.Product] = b.FromVersion
		if p, ok := s.config.Product(b.Product); ok {
			p.CurrentVersion = b.FromVersion
		}
		err = ErrPodPredecessorRestored
	}
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	s.recordAttempt(u, success, detail)
	saveErr := SaveState(s.config.Simulation.StateFile, s.state)
	reportErr := s.client.ReportUpdate(ctx, b.Product, UpdateReportRequest{InstanceID: s.config.Instance.ID, FromVersion: b.FromVersion, ToVersion: b.TargetVersion, ArtifactDigest: b.ArtifactSHA256, Success: success, Error: detail, Kind: "pod_maintenance", Stage: "recovery-v2"})
	return true, errors.Join(err, saveErr, reportErr)
}
