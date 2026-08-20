package updatersim

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// ErrRestartPending signals that a new updater binary has been staged and
// activated, and the process must exit so the service manager (systemd)
// relaunches it through the managed symlink. Callers treat it as a clean
// shutdown request, not a failure.
var ErrRestartPending = errors.New("updater restart pending: new binary activated")

// selfUpdateValidateTimeout bounds execution of the staged candidate binary.
const selfUpdateValidateTimeout = 30 * time.Second

// executablePath resolves the running binary; a variable so tests can point
// the simulator at a fake managed layout.
var executablePath = os.Executable

// SelfUpdateProduct is the release product name this node watches for its own
// binary. Updater binaries are architecture-specific single files, so each
// platform is its own product (e.g. "updater-linux-amd64"): publishing a
// release for that product on the updates server offers a self-update to every
// node of that platform, cascaded through relays like any other release.
func SelfUpdateProduct() string {
	return fmt.Sprintf("updater-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// SelfUpdater manages the versioned on-disk layout that lets an unprivileged
// service replace its own executable:
//
//	<dir>/releases/<version>/<binary>   one directory per staged updater
//	<dir>/current                       symlink -> releases/<version>
//	<dir>/.previous                     prior symlink target, for restore
//
// The service manager must launch the updater through <dir>/current (directly
// or via a stable symlink such as /usr/local/bin/<name>): activation is then
// an atomic symlink swap in a directory the service user owns, and the next
// restart executes the new binary. Nothing outside <dir> is touched.
type SelfUpdater struct {
	Dir string
}

// Manages reports whether path (a resolved executable path) lives inside the
// managed layout. When it does not, activation would never take effect because
// the service manager is not launching through the current symlink.
func (u *SelfUpdater) Manages(path string) bool {
	dir, err := filepath.Abs(u.Dir)
	if err != nil {
		return false
	}
	// The executable path comes from /proc/self/exe with symlinks resolved,
	// while the configured layout root may contain symlinked components (e.g.
	// /var -> /private/var), so compare every combination of resolved and
	// unresolved forms.
	dirs := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
		dirs = append(dirs, resolved)
	}
	paths := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		paths = append(paths, resolved)
	}
	for _, d := range dirs {
		for _, p := range paths {
			rel, err := filepath.Rel(d, p)
			if err != nil {
				continue
			}
			if rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return true
			}
		}
	}
	return false
}

func (u *SelfUpdater) releaseDir(version string) string {
	return filepath.Join(u.Dir, "releases", sanitizeSegment(version))
}

func (u *SelfUpdater) currentLink() string {
	return filepath.Join(u.Dir, "current")
}

func (u *SelfUpdater) previousFile() string {
	return filepath.Join(u.Dir, ".previous")
}

// Stage copies the verified artifact into its own version directory under the
// managed layout, preserving the running binary's file name so the current
// symlink path stays stable across versions.
func (u *SelfUpdater) Stage(artifactPath, version, binaryName string) (string, error) {
	if u.Dir == "" {
		return "", fmt.Errorf("self-update dir is not configured")
	}
	dir := u.releaseDir(version)
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clear staging dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}

	source, err := os.Open(artifactPath)
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	defer source.Close()

	staged := filepath.Join(dir, sanitizeSegment(binaryName))
	if err := writeStream(staged, source, 0o755); err != nil {
		return "", fmt.Errorf("stage binary: %w", err)
	}
	return staged, nil
}

// ValidateStaged executes the staged candidate ("<binary> version") and
// requires its output to contain the expected version. This catches corrupt
// downloads, wrong-architecture binaries, and mislabeled builds before the
// swap, which is the main crash-loop risk of a service self-update.
func (u *SelfUpdater) ValidateStaged(ctx context.Context, stagedPath, version string) error {
	runCtx, cancel := context.WithTimeout(ctx, selfUpdateValidateTimeout)
	defer cancel()

	output, err := exec.CommandContext(runCtx, stagedPath, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("staged updater failed to execute: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), version) {
		return fmt.Errorf(
			"staged updater reports %q, expected version %s",
			strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0]),
			version,
		)
	}
	return nil
}

// Activate records the current symlink target for restore and atomically
// points current at the staged version directory.
func (u *SelfUpdater) Activate(version string) error {
	prev, _ := os.Readlink(u.currentLink())
	if err := os.WriteFile(u.previousFile(), []byte(prev), 0o644); err != nil {
		return fmt.Errorf("record previous updater: %w", err)
	}
	return u.swapCurrent(u.releaseDir(version))
}

// RestorePrevious points current back at the recorded previous target. It is
// the watchdog path when the wrong binary comes up after an activation.
func (u *SelfUpdater) RestorePrevious() error {
	data, err := os.ReadFile(u.previousFile())
	if err != nil {
		return fmt.Errorf("read previous updater target: %w", err)
	}
	target := strings.TrimSpace(string(data))
	if target == "" {
		return fmt.Errorf("no previous updater target recorded")
	}
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("previous updater target missing: %w", err)
	}
	return u.swapCurrent(target)
}

func (u *SelfUpdater) swapCurrent(target string) error {
	link := u.currentLink()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", link, os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// SetBinaryVersion records the ldflags-stamped version of the running binary.
// A stamped version is authoritative over the config's informational
// updater_version, so heartbeats always report what is actually executing.
func (s *Simulator) SetBinaryVersion(version string) {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" {
		return
	}
	s.binaryVersion = version
	s.config.Instance.UpdaterVersion = version
}

// maybeSelfUpdate checks the parent for a newer updater binary and, when one
// is offered, verifies, stages, validates, and activates it, then returns
// ErrRestartPending so the process exits for the service manager to relaunch.
func (s *Simulator) maybeSelfUpdate(ctx context.Context, mode Mode) error {
	if s.config.SelfUpdate.Disabled {
		return nil
	}
	if s.binaryVersion == "" {
		// Unstamped (dev) builds have no version to compare against.
		return nil
	}

	product := SelfUpdateProduct()
	operatingSystem, architecture := s.platform()
	offer, err := s.client.CheckUpdate(ctx, product, UpdateCheckRequest{
		InstanceID:       s.config.Instance.ID,
		CurrentVersion:   s.binaryVersion,
		UpdaterVersion:   s.binaryVersion,
		OS:               operatingSystem,
		Arch:             architecture,
		Hostname:         s.config.Instance.Hostname,
		Channel:          s.config.SelfUpdate.Channel,
		ProductTier:      s.config.Instance.ProductTier,
		ParentInstanceID: s.config.Instance.ParentID,
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 405) {
			// No updater releases published for this platform; nothing to do.
			return nil
		}
		return fmt.Errorf("self-update check: %w", err)
	}
	if !offer.UpdateAvailable || offer.LatestVersion == s.binaryVersion {
		return nil
	}

	s.logger.Info(
		"updater self-update available",
		"product", product,
		"current_version", s.binaryVersion,
		"target_version", offer.LatestVersion,
		"mode", mode,
	)
	if mode == ModeObserve {
		return nil
	}

	result, err := s.verifyAndDownload(ctx, offer)
	if err != nil {
		return fmt.Errorf("self-update %s: %w", offer.LatestVersion, err)
	}
	if mode == ModeDownload {
		s.logger.Info("self-update downloaded and verified; download mode, not applying",
			"target_version", offer.LatestVersion, "path", result.Path)
		return nil
	}
	return s.applySelfUpdate(ctx, offer, result.Path)
}

// applySelfUpdate performs the staged swap. On success the pending marker is
// intentionally left in place: the replacement binary finalizes it at startup
// (ResolveSelfUpdate), which is the proof the handoff worked.
func (s *Simulator) applySelfUpdate(ctx context.Context, offer *UpdateOffer, artifactPath string) error {
	updater := &SelfUpdater{Dir: s.config.SelfUpdate.Dir}

	exePath, err := executablePath()
	if err != nil {
		return fmt.Errorf("resolve running executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if !updater.Manages(exePath) {
		// Legacy install (binary in /usr/local/bin, not launched through the
		// managed symlink): a swap would never take effect. Requires a kit
		// reinstall with the self-updatable layout.
		s.logger.Warn(
			"self-update offered but this install cannot apply it: binary is outside the managed layout",
			"exe", exePath,
			"managed_dir", s.config.SelfUpdate.Dir,
			"target_version", offer.LatestVersion,
		)
		return nil
	}

	s.state.PendingSelfUpdate = &SelfUpdateState{
		FromVersion: s.binaryVersion,
		ToVersion:   offer.LatestVersion,
		StartedAt:   time.Now().UTC(),
	}
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		return err
	}

	fail := func(stage string, cause error) error {
		s.state.PendingSelfUpdate = nil
		stateErr := SaveState(s.config.Simulation.StateFile, s.state)
		reportErr := s.client.ReportUpdate(ctx, offer.Product, UpdateReportRequest{
			InstanceID:  s.config.Instance.ID,
			FromVersion: s.binaryVersion,
			ToVersion:   offer.LatestVersion,
			Success:     false,
			Error:       fmt.Sprintf("self-update %s: %v", stage, cause),
			Kind:        "self_update",
			Stage:       stage,
		})
		return errors.Join(fmt.Errorf("self-update %s: %w", stage, cause), stateErr, reportErr)
	}

	staged, err := updater.Stage(artifactPath, offer.LatestVersion, filepath.Base(exePath))
	if err != nil {
		return fail("stage", err)
	}
	if err := updater.ValidateStaged(ctx, staged, offer.LatestVersion); err != nil {
		return fail("validate", err)
	}
	if err := updater.Activate(offer.LatestVersion); err != nil {
		return fail("activate", err)
	}

	s.logger.Info(
		"self-update activated; exiting for service manager restart",
		"from", s.binaryVersion,
		"to", offer.LatestVersion,
		"staged", staged,
	)
	return ErrRestartPending
}

// ResolveSelfUpdate settles a pending self-update marker at process start.
// The happy path: the freshly activated binary comes up, sees its own version
// in the marker, and finalizes. The watchdog path: any other version comes up
// (activation did not stick, or a broken binary was replaced by hand), so the
// previous updater is restored and the failure is recorded for the next
// heartbeat to report.
func (s *Simulator) ResolveSelfUpdate() {
	pending := s.state.PendingSelfUpdate
	if pending == nil {
		return
	}

	if s.binaryVersion != "" && s.binaryVersion == pending.ToVersion {
		s.state.UpdaterVersion = pending.ToVersion
		s.config.Instance.UpdaterVersion = pending.ToVersion
		s.state.LastUpdateAttempt = &platformtypes.UpdateAttempt{
			FromVersion:   pending.FromVersion,
			TargetVersion: pending.ToVersion,
			Success:       true,
			Timestamp:     time.Now().UTC(),
		}
		s.state.PendingSelfUpdate = nil
		if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			s.logger.Warn("failed to persist finalized self-update", "error", err)
		}
		s.logger.Info(
			"self-update finalized: new updater is live",
			"from", pending.FromVersion,
			"to", pending.ToVersion,
		)
		return
	}

	updater := &SelfUpdater{Dir: s.config.SelfUpdate.Dir}
	restoreErr := updater.RestorePrevious()
	message := fmt.Sprintf(
		"self-update to %s did not take effect (running %s)",
		pending.ToVersion, s.binaryVersion,
	)
	if restoreErr != nil {
		message += "; restore previous: " + restoreErr.Error()
	} else {
		message += "; previous updater restored"
	}
	s.state.LastUpdateAttempt = &platformtypes.UpdateAttempt{
		FromVersion:   pending.FromVersion,
		TargetVersion: pending.ToVersion,
		Success:       false,
		Error:         message,
		Timestamp:     time.Now().UTC(),
	}
	s.state.PendingSelfUpdate = nil
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		s.logger.Warn("failed to persist self-update watchdog result", "error", err)
	}
	s.logger.Error("self-update watchdog triggered", "detail", message)
}

// verifyAndDownload enforces the release signature policy and downloads the
// artifact with checksum verification. Shared by product updates and
// self-updates so both paths have identical supply-chain guarantees.
func (s *Simulator) verifyAndDownload(ctx context.Context, offer *UpdateOffer) (*DownloadResult, error) {
	if offer.DownloadURL == "" {
		return nil, fmt.Errorf("update %s %s has no download URL", offer.Product, offer.LatestVersion)
	}
	if s.publicKey != nil {
		if err := signing.Verify(s.publicKey, offer.Product, offer.LatestVersion, offer.Checksum, offer.Signature); err != nil {
			return nil, fmt.Errorf("release signature for %s %s: %w", offer.Product, offer.LatestVersion, err)
		}
	} else if s.config.Signing.Require {
		return nil, fmt.Errorf("signing.require is set but signing.public_key is not configured")
	}

	result, err := s.client.DownloadArtifact(
		ctx,
		offer.DownloadURL,
		offer.Checksum,
		s.config.Simulation.ArtifactDir,
		fmt.Sprintf("%s-%s.artifact", offer.Product, offer.LatestVersion),
		s.config.Simulation.MaxDownloadBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("download %s %s: %w", offer.Product, offer.LatestVersion, err)
	}
	s.logger.Info(
		"artifact downloaded and verified",
		"product", offer.Product,
		"target_version", offer.LatestVersion,
		"bytes", result.Size,
		"path", result.Path,
		"sha256", result.Checksum,
	)
	return result, nil
}
