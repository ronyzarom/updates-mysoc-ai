package updatersim

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FilesystemExecutor is a real (non-simulated) Executor that installs a verified
// artifact onto the local machine. Unlike NoopExecutor it changes the disk, so
// it is opt-in via simulation.executor: filesystem.
//
// Layout under InstallRoot for a product "siemcore":
//
//	<root>/siemcore/releases/<version>/   extracted artifact for each version
//	<root>/siemcore/current               symlink -> releases/<version>
//	<root>/siemcore/.previous             records the prior symlink target
//
// Apply is atomic and reversible: it stages the new version into its own
// directory, then flips the "current" symlink with an atomic rename, then runs
// the optional restart command. Rollback flips "current" back to the previous
// target and restarts. A fresh install (no previous) rolls back by removing the
// symlink.
type FilesystemExecutor struct {
	// InstallRoot is the base directory that holds per-product install trees.
	InstallRoot string
	// RestartCommand, when set, is executed after the symlink swap (and after a
	// rollback). It receives PRODUCT, VERSION, CURRENT_DIR, and INSTALL_ROOT in
	// its environment.
	RestartCommand []string
	// HealthCommand, when set, is executed by Validate to confirm the new
	// version is healthy. A non-zero exit fails validation and triggers rollback.
	HealthCommand []string
	// KeepReleases bounds retained version directories (0 = keep all). The
	// current and previous targets are always kept.
	KeepReleases int
	// CommandTimeout bounds restart/health command execution.
	CommandTimeout time.Duration

	logger *slog.Logger
}

// NewFilesystemExecutor constructs a filesystem installer.
func NewFilesystemExecutor(cfg FilesystemConfig, logger *slog.Logger) *FilesystemExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.CommandTimeout.Duration
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	// Resolve the install root to an absolute path so that the "current" symlink
	// target and the CURRENT_DIR passed to restart/health commands are stable
	// regardless of the process working directory.
	installRoot := cfg.InstallRoot
	if abs, err := filepath.Abs(installRoot); err == nil {
		installRoot = abs
	}
	return &FilesystemExecutor{
		InstallRoot:    installRoot,
		RestartCommand: cfg.RestartCommand,
		HealthCommand:  cfg.HealthCommand,
		KeepReleases:   cfg.KeepReleases,
		CommandTimeout: timeout,
		logger:         logger,
	}
}

var versionDirPattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// releaseMetadata is written alongside each installed version for auditing and
// for Validate to confirm the expected version is live.
type releaseMetadata struct {
	Product     string    `json:"product"`
	Version     string    `json:"version"`
	SHA256      string    `json:"sha256"`
	InstalledAt time.Time `json:"installed_at"`
}

func (e *FilesystemExecutor) productRoot(product string) string {
	return filepath.Join(e.InstallRoot, sanitizeSegment(product))
}

func (e *FilesystemExecutor) versionDir(product, version string) string {
	return filepath.Join(e.productRoot(product), "releases", sanitizeSegment(version))
}

func (e *FilesystemExecutor) currentLink(product string) string {
	return filepath.Join(e.productRoot(product), "current")
}

func (e *FilesystemExecutor) previousFile(product string) string {
	return filepath.Join(e.productRoot(product), ".previous")
}

// Apply installs the artifact for update and atomically activates it.
func (e *FilesystemExecutor) Apply(ctx context.Context, update Update) error {
	if e.InstallRoot == "" {
		return fmt.Errorf("filesystem executor requires an install_root")
	}
	if update.ArtifactPath == "" {
		return fmt.Errorf("no artifact to install for %s %s", update.Product, update.ToVersion)
	}

	versionDir := e.versionDir(update.Product, update.ToVersion)
	// Stage into a clean directory so re-running Apply is idempotent.
	if err := os.RemoveAll(versionDir); err != nil {
		return fmt.Errorf("clear staging dir: %w", err)
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	if err := installArtifact(update.ArtifactPath, versionDir, update.Product); err != nil {
		return fmt.Errorf("install artifact: %w", err)
	}

	meta := releaseMetadata{
		Product:     update.Product,
		Version:     update.ToVersion,
		SHA256:      update.ArtifactSHA256,
		InstalledAt: time.Now().UTC(),
	}
	if err := writeJSONFile(filepath.Join(versionDir, ".updater-release.json"), meta); err != nil {
		return fmt.Errorf("write release metadata: %w", err)
	}

	// Record the current target (if any) so Rollback can restore it.
	prevTarget, _ := os.Readlink(e.currentLink(update.Product))
	if err := e.recordPrevious(update.Product, prevTarget); err != nil {
		return fmt.Errorf("record previous target: %w", err)
	}

	if err := e.swapCurrent(update.Product, versionDir); err != nil {
		return fmt.Errorf("activate version: %w", err)
	}
	e.logger.Info(
		"filesystem install activated",
		"product", update.Product,
		"version", update.ToVersion,
		"current", e.currentLink(update.Product),
		"target", versionDir,
	)

	if err := e.runCommand(ctx, "restart", e.RestartCommand, update); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}

	e.pruneOldReleases(update.Product, versionDir, prevTarget)
	return nil
}

// Validate confirms that the live version matches the update and (optionally)
// passes the health command.
func (e *FilesystemExecutor) Validate(ctx context.Context, update Update) error {
	target, err := os.Readlink(e.currentLink(update.Product))
	if err != nil {
		return fmt.Errorf("read current symlink: %w", err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(e.productRoot(update.Product), target)
	}

	var meta releaseMetadata
	if err := readJSONFile(filepath.Join(target, ".updater-release.json"), &meta); err != nil {
		return fmt.Errorf("read live release metadata: %w", err)
	}
	if meta.Version != update.ToVersion {
		return fmt.Errorf("live version %q does not match expected %q", meta.Version, update.ToVersion)
	}

	if err := e.runCommand(ctx, "health", e.HealthCommand, update); err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	e.logger.Info("filesystem install validated", "product", update.Product, "version", update.ToVersion)
	return nil
}

// Rollback restores the previous symlink target (or removes the symlink for a
// fresh install) and restarts.
func (e *FilesystemExecutor) Rollback(ctx context.Context, update Update) error {
	prevTarget, _ := e.readPrevious(update.Product)
	link := e.currentLink(update.Product)

	if prevTarget == "" {
		// Fresh install: there was nothing before. Remove the symlink so the
		// machine returns to its pre-install state.
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove current symlink: %w", err)
		}
		e.logger.Info("filesystem install rolled back (fresh install removed)", "product", update.Product)
		return e.runCommand(ctx, "restart", e.RestartCommand, update)
	}

	if err := e.swapCurrent(update.Product, prevTarget); err != nil {
		return fmt.Errorf("restore previous target: %w", err)
	}
	e.logger.Info(
		"filesystem install rolled back",
		"product", update.Product,
		"restored_target", prevTarget,
	)
	return e.runCommand(ctx, "restart", e.RestartCommand, update)
}

// swapCurrent atomically points <product>/current at target using a temp symlink
// plus rename, which is atomic within the same directory.
func (e *FilesystemExecutor) swapCurrent(product, target string) error {
	link := e.currentLink(product)
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

func (e *FilesystemExecutor) recordPrevious(product, prevTarget string) error {
	return os.WriteFile(e.previousFile(product), []byte(prevTarget), 0o644)
}

func (e *FilesystemExecutor) readPrevious(product string) (string, error) {
	data, err := os.ReadFile(e.previousFile(product))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// pruneOldReleases removes version directories beyond KeepReleases, always
// keeping the current and previous targets.
func (e *FilesystemExecutor) pruneOldReleases(product, keepCurrent, keepPrevious string) {
	if e.KeepReleases <= 0 {
		return
	}
	releasesDir := filepath.Join(e.productRoot(product), "releases")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		return
	}

	type dirInfo struct {
		path    string
		modTime time.Time
	}
	var dirs []dirInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, dirInfo{path: filepath.Join(releasesDir, entry.Name()), modTime: info.ModTime()})
	}
	// Newest first.
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].modTime.After(dirs[j].modTime) })

	kept := 0
	for _, d := range dirs {
		if d.path == keepCurrent || d.path == keepPrevious {
			continue
		}
		kept++
		if kept < e.KeepReleases {
			continue
		}
		_ = os.RemoveAll(d.path)
	}
}

func (e *FilesystemExecutor) runCommand(ctx context.Context, kind string, argv []string, update Update) error {
	if len(argv) == 0 {
		return nil
	}
	runCtx := ctx
	if e.CommandTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, e.CommandTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(),
		"PRODUCT="+update.Product,
		"VERSION="+update.ToVersion,
		"FROM_VERSION="+update.FromVersion,
		"CURRENT_DIR="+e.currentLink(update.Product),
		"INSTALL_ROOT="+e.InstallRoot,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s command failed: %w: %s", kind, err, strings.TrimSpace(string(output)))
	}
	if len(output) > 0 {
		e.logger.Debug("command output", "kind", kind, "output", strings.TrimSpace(string(output)))
	}
	return nil
}

// installArtifact writes the artifact into destDir. A gzip'd tar archive is
// extracted; any other content is copied as a single file. product is used to
// name the file when the artifact is a bare binary.
func installArtifact(artifactPath, destDir, product string) error {
	file, err := os.Open(artifactPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	magic, _ := reader.Peek(2)
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("open gzip: %w", err)
		}
		defer gz.Close()

		isTar, err := extractTar(gz, destDir)
		if err != nil {
			return err
		}
		if isTar {
			return nil
		}
		// Gzip but not a tar: write the decompressed bytes as a single file.
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		gz2, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gz2.Close()
		return writeStream(filepath.Join(destDir, sanitizeSegment(product)), gz2, 0o755)
	}

	// Not gzip: copy the raw artifact as a single named file.
	name := filepath.Base(artifactPath)
	if name == "" || name == "." {
		name = sanitizeSegment(product)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return writeStream(filepath.Join(destDir, name), file, 0o755)
}

// extractTar extracts a tar stream into dest. It reports isTar=false (without
// error) when the stream is not a tar archive, so the caller can fall back.
func extractTar(r io.Reader, dest string) (isTar bool, err error) {
	tr := tar.NewReader(r)
	first := true
	for {
		header, e := tr.Next()
		if e == io.EOF {
			return true, nil
		}
		if e != nil {
			if first {
				return false, nil
			}
			return true, e
		}
		first = false

		target := filepath.Join(dest, header.Name)
		if !withinDir(dest, target) {
			return true, fmt.Errorf("refusing to extract path outside destination: %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return true, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return true, err
			}
			mode := header.FileInfo().Mode()
			if mode == 0 {
				mode = 0o644
			}
			if err := writeStream(target, tr, mode); err != nil {
				return true, err
			}
		default:
			// Skip symlinks, devices, etc. for safety.
		}
	}
}

// withinDir reports whether target stays inside dest (zip-slip guard).
func withinDir(dest, target string) bool {
	rel, err := filepath.Rel(dest, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func writeStream(path string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return out.Chmod(mode)
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readJSONFile(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func sanitizeSegment(name string) string {
	cleaned := versionDirPattern.ReplaceAllString(strings.TrimSpace(name), "_")
	cleaned = strings.Trim(cleaned, "._")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}
