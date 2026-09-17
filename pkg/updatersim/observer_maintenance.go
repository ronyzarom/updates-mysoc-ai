package updatersim

import (
	"context"
	"errors"
	"fmt"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/podmaintenance"
	"os"
	"path/filepath"
)

func (s *Simulator) observerCoordinator() *podmaintenance.ObserverCoordinator {
	cfg := s.config.Simulation.Filesystem.ObserverMaintenance
	return &podmaintenance.ObserverCoordinator{Directory: cfg.JournalDirectory, Adapter: podmaintenance.CommandAdapter{Command: cfg.AdapterCommand, Timeout: s.config.Simulation.Filesystem.CommandTimeout.Duration}}
}
func (s *Simulator) applyObserverMaintenance(ctx context.Context, u Update) error {
	if err := s.validateSiemCoreExecution(); err != nil {
		return err
	}
	cfg := s.config.Simulation.Filesystem.ObserverMaintenance
	if cfg == nil || u.Product != "siemcore" {
		return fmt.Errorf("observer executor not configured")
	}
	if err := s.verifyRetainedPodArtifact(u); err != nil {
		return err
	}
	previous := ""
	j, err := podmaintenance.ReadObserverJournal(cfg.JournalDirectory)
	if err == nil && j.Phase != "accepted" {
		previous = j.Binding.PreviousArtifactSHA256
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if previous == "" {
		var meta releaseMetadata
		if err := readJSONFile(filepath.Join(s.config.Simulation.Filesystem.InstallRoot, "siemcore", "current", ".updater-release.json"), &meta); err != nil {
			return fmt.Errorf("observer predecessor evidence: %w", err)
		}
		if meta.Product != u.Product || meta.Version != u.FromVersion {
			return fmt.Errorf("observer predecessor mismatch")
		}
		previous = meta.SHA256
	}
	return s.observerCoordinator().Run(ctx, podmaintenance.ObserverBinding{Protocol: podmaintenance.ObserverProtocol, PodID: cfg.PodID, NodeID: cfg.NodeID, UpdaterID: s.config.Instance.ID, Product: u.Product, FromVersion: u.FromVersion, TargetVersion: u.ToVersion, ArtifactPath: u.ArtifactPath, ArtifactSignature: u.ArtifactSignature, ArtifactSHA256: u.ArtifactSHA256, PreviousArtifactSHA256: previous})
}
func (s *Simulator) resumePendingObserver(ctx context.Context) (bool, error) {
	cfg := s.config.Simulation.Filesystem.ObserverMaintenance
	if cfg == nil {
		return false, nil
	}
	j, err := podmaintenance.ReadObserverJournal(cfg.JournalDirectory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if err = s.validateSiemCoreExecution(); err != nil {
		return true, err
	}
	b := j.Binding
	if b.PodID != cfg.PodID || b.NodeID != cfg.NodeID || b.UpdaterID != s.config.Instance.ID || b.Product != "siemcore" {
		return true, fmt.Errorf("observer journal identity mismatch")
	}
	if p, ok := s.config.Product(b.Product); ok && j.Phase == "accepted" && p.CurrentVersion == b.TargetVersion {
		return false, nil
	}
	u := Update{Product: b.Product, FromVersion: b.FromVersion, ToVersion: b.TargetVersion, ArtifactPath: b.ArtifactPath, ArtifactSHA256: b.ArtifactSHA256, ArtifactSignature: b.ArtifactSignature}
	if err = s.verifyRetainedPodArtifact(u); err != nil {
		return true, err
	}
	err = s.observerCoordinator().Run(ctx, b)
	message := ""
	if err != nil {
		message = err.Error()
	}
	s.recordAttempt(u, err == nil, message)
	return true, errors.Join(err, SaveState(s.config.Simulation.StateFile, s.state), s.client.ReportUpdate(ctx, b.Product, UpdateReportRequest{InstanceID: s.config.Instance.ID, FromVersion: b.FromVersion, ToVersion: b.TargetVersion, ArtifactDigest: b.ArtifactSHA256, Success: err == nil, Kind: "observer_maintenance", Stage: "reconciliation", Error: message}))
}
