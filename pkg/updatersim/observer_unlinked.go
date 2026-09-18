package updatersim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The independent Observer has no preceding installation or authority to roll
// back. Keep failed staging and the root-owned product journal for exact retry.
// The protected root hook verifies the archive, policy, version and identities
// again, including on replay; extracted executable content is never trusted.
func (s *Simulator) applyIndependentObserver(ctx context.Context, update Update) error {
	return s.applyIndependentBootstrap(ctx, update, "Observer")
}

// Independent nodes share retained signed staging, never a Normal update fallback.
// Product root execution owns durable settings and database initialization.
func (s *Simulator) applyIndependentNode(ctx context.Context, update Update) error {
	return s.applyIndependentBootstrap(ctx, update, "POD node")
}

func (s *Simulator) applyIndependentBootstrap(ctx context.Context, update Update, label string) error {
	e, ok := s.executor.(*FilesystemExecutor)
	if !ok {
		return fmt.Errorf("independent %s requires filesystem bootstrap executor", label)
	}
	if update.FromVersion != "0.0.0" && update.FromVersion != "0.0.0.0" && update.FromVersion != "" && update.FromVersion != update.ToVersion {
		return fmt.Errorf("independent %s upgrade is not qualified", label)
	}
	current := e.currentLink(update.Product)
	info, err := os.Lstat(current)
	if os.IsNotExist(err) {
		if err = e.Apply(ctx, update); err != nil {
			return err
		}
	} else {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s retained current is not a symlink", label)
		}
		target, err := filepath.EvalSymlinks(current)
		if err != nil {
			return err
		}
		expected, err := filepath.EvalSymlinks(e.versionDir(update.Product, update.ToVersion))
		if err != nil {
			return err
		}
		if target != expected {
			return fmt.Errorf("%s retained transaction target differs", label)
		}
		metadataPath := filepath.Join(target, ".updater-release.json")
		info, err := os.Lstat(metadataPath)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 65536 {
			return fmt.Errorf("invalid %s retained receipt", label)
		}
		raw, err := os.ReadFile(metadataPath)
		if err != nil {
			return err
		}
		var receipt releaseMetadata
		if err = json.Unmarshal(raw, &receipt); err != nil {
			return err
		}
		if receipt.Product != update.Product || receipt.Version != update.ToVersion || receipt.SHA256 != update.ArtifactSHA256 || receipt.Signature != update.ArtifactSignature || receipt.ArtifactKind != update.SelectedArtifactKind {
			return fmt.Errorf("%s retry differs from retained signed transaction", label)
		}
		if err = e.runCommand(ctx, "restart", "apply", e.RestartCommand, update); err != nil {
			return err
		}
	}
	return e.Validate(ctx, update)
}
