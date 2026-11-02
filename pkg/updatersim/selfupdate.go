package updatersim

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// selfUpdateReleasesKeep is how many old release directories to retain beyond
// the current and previous targets, so a fleet keeps a short rollback history
// without letting releases/ grow without bound.
const selfUpdateReleasesKeep = 2

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

// resolvedExecutable returns the on-disk path of the running binary with any
// symlinks resolved, so managed-layout checks compare real paths (the service
// launches the updater through the managed "current" symlink).
func resolvedExecutable() (string, error) {
	exePath, err := executablePath()
	if err != nil {
		return "", err
	}
	if resolved, resErr := filepath.EvalSymlinks(exePath); resErr == nil {
		exePath = resolved
	}
	return exePath, nil
}

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
	// Refuse to stage when the layout's filesystem lacks room, so a full disk is
	// a clean skip-and-retry rather than a half-written binary that cannot be
	// validated or activated.
	if err := ensureFreeDiskForArtifact(artifactPath, u.Dir); err != nil {
		return "", err
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

	cmd := exec.CommandContext(runCtx, stagedPath, "version")
	hardenCommand(cmd, commandWaitDelay)
	out := &boundedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	output := out.String()
	if err != nil {
		return fmt.Errorf("staged updater failed to execute: %w: %s", err, strings.TrimSpace(output))
	}
	if !strings.Contains(output, version) {
		return fmt.Errorf(
			"staged updater reports %q, expected version %s",
			strings.TrimSpace(strings.SplitN(output, "\n", 2)[0]),
			version,
		)
	}
	return nil
}

// Preflight runs the staged candidate's read-only "preflight" subcommand
// against the host's real config and state. Where ValidateStaged proves the
// binary can report its version, Preflight proves it can actually parse THIS
// host's configuration and state and get far enough to run — the real
// crash-loop risk a bare version check misses (a config key the new binary
// rejects, a state file it cannot read). It is strictly read-only and bounded.
func (u *SelfUpdater) Preflight(ctx context.Context, stagedPath, configPath, statePath string) error {
	runCtx, cancel := context.WithTimeout(ctx, selfUpdateValidateTimeout)
	defer cancel()

	args := []string{"preflight", "--config", configPath}
	if statePath != "" {
		args = append(args, "--state", statePath)
	}
	cmd := exec.CommandContext(runCtx, stagedPath, args...)
	hardenCommand(cmd, commandWaitDelay)
	out := &boundedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("staged updater preflight failed: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// Activate records the current symlink target for restore and atomically
// points current at the staged version directory. It is idempotent (a no-op
// when current already points at the target) and never records target as its
// own .previous, so a restore can never loop back to the version it is rolling
// back from.
func (u *SelfUpdater) Activate(version string) error {
	target := u.releaseDir(version)
	prev, _ := os.Readlink(u.currentLink())
	if filepath.Clean(prev) == filepath.Clean(target) {
		// Already active: Activate ran before, or a retry after a crash past the
		// swap. Nothing to do, and do not overwrite .previous with target.
		return nil
	}
	if prev != "" {
		// Atomic .previous write: a crash mid-write must not leave a truncated
		// pointer that a later restore would act on.
		if err := atomicWriteFile(u.previousFile(), []byte(prev), 0o644); err != nil {
			return fmt.Errorf("record previous updater: %w", err)
		}
	}
	return u.swapCurrent(target)
}

// RestorePrevious points current back at the recorded previous target. It is
// the watchdog path when the wrong binary comes up after an activation. It
// refuses to act unless current actually resolves to failedTarget's release:
// if the swap never took effect (a crash between the pending marker and the
// symlink swap), current still points at the old release and the recorded
// .previous may be two versions back, so restoring it would wrongly downgrade.
func (u *SelfUpdater) RestorePrevious(failedVersion string) error {
	cur, _ := os.Readlink(u.currentLink())
	if filepath.Clean(cur) != filepath.Clean(u.releaseDir(failedVersion)) {
		return fmt.Errorf(
			"current does not point at failed target %s (points at %q); refusing to restore a possibly stale .previous",
			failedVersion, cur,
		)
	}
	data, err := os.ReadFile(u.previousFile())
	if err != nil {
		return fmt.Errorf("read previous updater target: %w", err)
	}
	target := strings.TrimSpace(string(data))
	if target == "" {
		return fmt.Errorf("no previous updater target recorded")
	}
	if filepath.Clean(target) == filepath.Clean(u.releaseDir(failedVersion)) {
		return fmt.Errorf("recorded previous target is the failed version itself; refusing to restore")
	}
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("previous updater target missing: %w", err)
	}
	return u.swapCurrent(target)
}

// atomicWriteFile writes data to a temp file in the destination directory, syncs
// it, and renames it into place, so a crash never leaves a partial file.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Prune removes stale release directories, keeping the current target, the
// recorded previous target, and the keep most-recent other releases (by mtime).
// Best-effort and crash-safe: it never touches current or previous, so a
// concurrent restore always has its target. Errors are joined but do not stop
// the sweep.
func (u *SelfUpdater) Prune(keep int) error {
	if u.Dir == "" {
		return nil
	}
	releasesDir := filepath.Join(u.Dir, "releases")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	protected := map[string]bool{}
	if cur, e := os.Readlink(u.currentLink()); e == nil && cur != "" {
		protected[filepath.Clean(cur)] = true
	}
	if data, e := os.ReadFile(u.previousFile()); e == nil {
		if prev := strings.TrimSpace(string(data)); prev != "" {
			protected[filepath.Clean(prev)] = true
		}
	}

	type releaseEntry struct {
		path string
		mod  time.Time
	}
	var candidates []releaseEntry
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Clean(filepath.Join(releasesDir, ent.Name()))
		if protected[path] {
			continue
		}
		info, e := ent.Info()
		if e != nil {
			continue
		}
		candidates = append(candidates, releaseEntry{path: path, mod: info.ModTime()})
	}
	// Newest first; keep the first `keep`, remove the rest.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].mod.After(candidates[j].mod) })
	var errs []error
	for i, c := range candidates {
		if i < keep {
			continue
		}
		if e := os.RemoveAll(c.path); e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
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

// verifyAnyKey accepts the release signature if it verifies under any trusted
// key. This is the key-rollover path: during a signing-key rotation both the
// old and new keys are trusted, so a node never fails to verify an artifact
// signed by whichever key the server currently uses.
func verifyAnyKey(keys []ed25519.PublicKey, product, version, checksum, signature string) error {
	var lastErr error
	for _, key := range keys {
		if err := signing.Verify(key, product, version, checksum, signature); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no trusted signing keys")
	}
	return lastErr
}

// maybeSelfUpdate checks the parent for a newer updater binary and, when one
// is offered, verifies, stages, validates, and activates it, then returns
// ErrRestartPending so the process exits for the service manager to relaunch.
func (s *Simulator) maybeSelfUpdate(ctx context.Context, mode Mode, client *Client) error {
	if s.config.SelfUpdate.Disabled {
		return nil
	}
	if s.binaryVersion == "" {
		// Unstamped (dev) builds have no version to compare against.
		return nil
	}

	product := SelfUpdateProduct()
	operatingSystem, architecture := s.platform()
	offer, err := client.CheckUpdate(ctx, product, UpdateCheckRequest{
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

	// Quarantine: a version that previously failed to apply or come up is skipped
	// until the server offers a different one, so a bad release does not flap
	// (restore, re-download, re-activate) every cycle. A different offer clears
	// the quarantine so a corrected build is taken.
	if q := s.state.QuarantinedSelfUpdateVersion; q != "" {
		if offer.LatestVersion == q {
			s.logger.Warn(
				"skipping self-update: target is quarantined after a prior failure",
				"target_version", offer.LatestVersion,
			)
			return nil
		}
		s.state.QuarantinedSelfUpdateVersion = ""
		if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			return err
		}
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

	// Check the managed-layout precondition BEFORE downloading. A legacy install
	// (binary in /usr/local/bin, not launched through the managed symlink) can
	// never apply a swap, so fetching the artifact every cycle is pure waste.
	// A kit reinstall with the self-updatable layout is required.
	updater := &SelfUpdater{Dir: s.config.SelfUpdate.Dir}
	if exePath, exeErr := resolvedExecutable(); exeErr == nil && !updater.Manages(exePath) {
		s.logger.Warn(
			"self-update offered but this install cannot apply it: binary is outside the managed layout; skipping download",
			"exe", exePath,
			"managed_dir", s.config.SelfUpdate.Dir,
			"target_version", offer.LatestVersion,
		)
		return nil
	}

	result, err := s.verifyAndDownload(ctx, offer, client)
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

	exePath, err := resolvedExecutable()
	if err != nil {
		return fmt.Errorf("resolve running executable: %w", err)
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
		// Quarantine this target so the next cycle does not immediately retry the
		// same bad version; a different offer clears it.
		s.state.QuarantinedSelfUpdateVersion = offer.LatestVersion
		s.state.LastSelfUpdate = &platformtypes.UpdateAttempt{
			FromVersion:   s.binaryVersion,
			TargetVersion: offer.LatestVersion,
			Success:       false,
			Error:         fmt.Sprintf("self-update %s: %v", stage, cause),
			Timestamp:     time.Now().UTC(),
		}
		stateErr := SaveState(s.config.Simulation.StateFile, s.state)
		// Report on a detached context: a self-update phase that hit its own
		// deadline must still report the failure so fleet visibility is correct.
		repCtx, repCancel := s.recoveryContext()
		defer repCancel()
		reportErr := s.client.ReportUpdate(repCtx, offer.Product, UpdateReportRequest{
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
	// Prove the candidate can boot on THIS host (parse the real config and
	// state) before the swap. Skipped only when the config was not loaded from
	// a file (in-process construction), since there is no file to hand it.
	if cfgPath := s.config.SourcePath(); cfgPath != "" {
		if err := updater.Preflight(ctx, staged, cfgPath, s.config.Simulation.StateFile); err != nil {
			return fail("preflight", err)
		}
	}
	if err := updater.Activate(offer.LatestVersion); err != nil {
		return fail("activate", err)
	}
	// The compressed download is now redundant (the binary is staged under
	// releases/ and validated); drop it so it does not linger in the artifact
	// dir. Best-effort.
	_ = os.Remove(artifactPath)

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
		// The new binary is running. Do NOT declare success yet: a binary that
		// boots but cannot reach its parent must not be marked good. The pending
		// marker is left in place and finalized by the first successful heartbeat
		// (finalizeSelfUpdateOnHeartbeat). Record the live version so heartbeats
		// report what is actually executing in the meantime.
		s.state.UpdaterVersion = pending.ToVersion
		s.config.Instance.UpdaterVersion = pending.ToVersion
		s.logger.Info(
			"self-update binary is live; awaiting first successful heartbeat to finalize",
			"from", pending.FromVersion,
			"to", pending.ToVersion,
		)
		return
	}

	// Watchdog path: a different version came up (activation did not stick, or a
	// broken binary was replaced). Restore the previous binary, but only if the
	// swap actually took effect; quarantine the target so it is not retried.
	updater := &SelfUpdater{Dir: s.config.SelfUpdate.Dir}
	restoreErr := updater.RestorePrevious(pending.ToVersion)
	message := fmt.Sprintf(
		"self-update to %s did not take effect (running %s)",
		pending.ToVersion, s.binaryVersion,
	)
	if restoreErr != nil {
		message += "; restore previous: " + restoreErr.Error()
	} else {
		message += "; previous updater restored"
	}
	attempt := &platformtypes.UpdateAttempt{
		FromVersion:   pending.FromVersion,
		TargetVersion: pending.ToVersion,
		Success:       false,
		Error:         message,
		Timestamp:     time.Now().UTC(),
	}
	s.state.LastUpdateAttempt = attempt
	s.state.LastSelfUpdate = attempt
	s.state.PendingSelfUpdate = nil
	s.state.QuarantinedSelfUpdateVersion = pending.ToVersion
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		s.logger.Warn("failed to persist self-update watchdog result", "error", err)
	}
	s.logger.Error("self-update watchdog triggered", "detail", message)
}

// finalizeSelfUpdateOnHeartbeat marks a pending self-update successful once the
// new binary has proven it can reach its parent (a heartbeat succeeded). This is
// the deferred half of ResolveSelfUpdate: booting is not enough, a node that
// cannot heartbeat its parent is not a good self-update.
func (s *Simulator) finalizeSelfUpdateOnHeartbeat() {
	pending := s.state.PendingSelfUpdate
	if pending == nil {
		return
	}
	if s.binaryVersion == "" || s.binaryVersion != pending.ToVersion {
		return
	}
	attempt := &platformtypes.UpdateAttempt{
		FromVersion:   pending.FromVersion,
		TargetVersion: pending.ToVersion,
		Success:       true,
		Timestamp:     time.Now().UTC(),
	}
	s.state.UpdaterVersion = pending.ToVersion
	s.config.Instance.UpdaterVersion = pending.ToVersion
	s.state.LastSelfUpdate = attempt
	s.state.PendingSelfUpdate = nil
	s.state.QuarantinedSelfUpdateVersion = ""
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		s.logger.Warn("failed to persist finalized self-update", "error", err)
	}
	// Now that the new binary is confirmed good, prune stale release directories
	// so the self-update layout does not grow without bound. Best-effort: a prune
	// failure must never affect the running updater.
	updater := &SelfUpdater{Dir: s.config.SelfUpdate.Dir}
	if err := updater.Prune(selfUpdateReleasesKeep); err != nil {
		s.logger.Warn("self-update release prune incomplete", "error", err)
	}
	s.logger.Info(
		"self-update finalized: new updater reached its parent",
		"from", pending.FromVersion,
		"to", pending.ToVersion,
	)
}

// verifyAndDownload enforces the release signature policy and downloads the
// artifact with checksum verification. Shared by product updates and
// self-updates so both paths have identical supply-chain guarantees.
func (s *Simulator) verifyAndDownload(ctx context.Context, offer *UpdateOffer, client *Client) (*DownloadResult, error) {
	if offer.DownloadURL == "" {
		return nil, fmt.Errorf("update %s %s has no download URL", offer.Product, offer.LatestVersion)
	}
	if len(s.publicKeys) > 0 {
		if err := verifyAnyKey(s.publicKeys, offer.Product, offer.LatestVersion, offer.Checksum, offer.Signature); err != nil {
			return nil, fmt.Errorf("release signature for %s %s: %w", offer.Product, offer.LatestVersion, err)
		}
	} else if s.config.Signing.Require {
		return nil, fmt.Errorf("signing.require is set but no signing key is configured")
	}

	// Bound the download by its own deadline (download_timeout), separate from
	// server.timeout which bounds only metadata. A deadline already present on
	// ctx (e.g. the self-update or product phase budget) still applies.
	dlCtx, cancel := context.WithTimeout(ctx, s.config.downloadTimeout())
	defer cancel()
	result, err := client.DownloadArtifact(
		dlCtx,
		offer.DownloadURL,
		offer.Checksum,
		s.config.Simulation.ArtifactDir,
		fmt.Sprintf("%s-%s.artifact", offer.Product, offer.LatestVersion),
		s.config.Simulation.MaxDownloadBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("download %s %s: %w", offer.Product, offer.LatestVersion, err)
	}
	if offer.SelectedArtifactKind != "" {
		for _, a := range offer.Artifacts {
			if a.Kind == offer.SelectedArtifactKind && result.Size != a.Size {
				_ = os.Remove(result.Path)
				return nil, fmt.Errorf("signed artifact size mismatch")
			}
		}
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
