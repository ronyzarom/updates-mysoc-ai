package updatersim

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// makeTarGz builds an in-memory .tar.gz containing the given files and writes it
// to path.
func makeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
}

func newFSExecutor(t *testing.T, root string) *FilesystemExecutor {
	t.Helper()
	return NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:    root,
		CommandTimeout: Duration{Duration: defaultCommandTimeout},
	}, nil)
}

func resolveCurrent(t *testing.T, e *FilesystemExecutor, product string) string {
	t.Helper()
	target, err := os.Readlink(e.currentLink(product))
	if err != nil {
		t.Fatalf("read current symlink: %v", err)
	}
	return target
}

func TestFilesystemExecutorApplyInstallsAndActivates(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{
		"bin/app":    "#!/bin/sh\necho v1\n",
		"VERSION":    "v1.0.0",
		"config.yml": "mode: prod\n",
	})

	e := newFSExecutor(t, root)
	update := Update{Product: "siemcore", FromVersion: "v0.0.0", ToVersion: "v1.0.0", ArtifactPath: artifact, ArtifactSHA256: "abc"}

	if err := e.Apply(context.Background(), update); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := e.Validate(context.Background(), update); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// current -> releases/v1.0.0
	target := resolveCurrent(t, e, "siemcore")
	if filepath.Base(target) != "v1.0.0" {
		t.Fatalf("current points to %q, want release v1.0.0", target)
	}
	// Extracted files exist under the version dir.
	for _, f := range []string{"bin/app", "VERSION", "config.yml"} {
		if _, err := os.Stat(filepath.Join(target, f)); err != nil {
			t.Fatalf("expected extracted file %q: %v", f, err)
		}
	}
}

func TestFilesystemExecutorUpgradeThenRollbackRestoresPrevious(t *testing.T) {
	root := t.TempDir()
	artDir := t.TempDir()
	e := newFSExecutor(t, root)

	v1 := filepath.Join(artDir, "siemcore-v1.0.0.artifact")
	makeTarGz(t, v1, map[string]string{"VERSION": "v1.0.0"})
	up1 := Update{Product: "siemcore", FromVersion: "v0.0.0", ToVersion: "v1.0.0", ArtifactPath: v1}
	if err := e.Apply(context.Background(), up1); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	v1Target := resolveCurrent(t, e, "siemcore")

	// Upgrade to v2.
	v2 := filepath.Join(artDir, "siemcore-v2.0.0.artifact")
	makeTarGz(t, v2, map[string]string{"VERSION": "v2.0.0"})
	up2 := Update{Product: "siemcore", FromVersion: "v1.0.0", ToVersion: "v2.0.0", ArtifactPath: v2}
	if err := e.Apply(context.Background(), up2); err != nil {
		t.Fatalf("apply v2: %v", err)
	}
	if got := filepath.Base(resolveCurrent(t, e, "siemcore")); got != "v2.0.0" {
		t.Fatalf("after upgrade current=%q, want v2.0.0", got)
	}

	// Rollback should restore the v1 target.
	if err := e.Rollback(context.Background(), up2); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if got := resolveCurrent(t, e, "siemcore"); got != v1Target {
		t.Fatalf("after rollback current=%q, want %q", got, v1Target)
	}
}

func TestFilesystemExecutorFreshInstallRollbackRemovesSymlink(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.0.0"})

	e := newFSExecutor(t, root)
	update := Update{Product: "siemcore", ToVersion: "v1.0.0", ArtifactPath: artifact}
	if err := e.Apply(context.Background(), update); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := e.Rollback(context.Background(), update); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err := os.Lstat(e.currentLink("siemcore")); !os.IsNotExist(err) {
		t.Fatalf("expected current symlink removed after fresh-install rollback, err=%v", err)
	}
}

func TestFilesystemExecutorValidateFailsOnVersionMismatch(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.0.0"})
	e := newFSExecutor(t, root)

	if err := e.Apply(context.Background(), Update{Product: "siemcore", ToVersion: "v1.0.0", ArtifactPath: artifact}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Validate against a different expected version must fail.
	err := e.Validate(context.Background(), Update{Product: "siemcore", ToVersion: "v9.9.9"})
	if err == nil {
		t.Fatal("expected validation to fail on version mismatch")
	}
}

func TestFilesystemExecutorRestartCommandRunsWithEnv(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "restart-marker")
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.0.0"})

	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:    root,
		RestartCommand: []string{"sh", "-c", "printf '%s' \"$PRODUCT-$VERSION\" > " + marker},
		CommandTimeout: Duration{Duration: defaultCommandTimeout},
	}, nil)

	if err := e.Apply(context.Background(), Update{Product: "siemcore", ToVersion: "v1.0.0", ArtifactPath: artifact}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(data) != "siemcore-v1.0.0" {
		t.Fatalf("restart marker = %q, want siemcore-v1.0.0", string(data))
	}
}

// TestFilesystemExecutorPhaseArgumentAndEnv proves the entrypoint contract:
// every lifecycle command receives the phase as its trailing positional
// argument AND as UPDATER_PHASE in the environment.
func TestFilesystemExecutorPhaseArgumentAndEnv(t *testing.T) {
	root := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "phases.log")
	script := filepath.Join(t.TempDir(), "entrypoint.sh")
	if err := os.WriteFile(script,
		[]byte("#!/bin/sh\necho \"$UPDATER_PHASE:$1\" >> "+logFile+"\n"), 0o755); err != nil {
		t.Fatalf("write entrypoint script: %v", err)
	}
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	makeTarGz(t, artifact, map[string]string{"VERSION": "v1.0.0"})

	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:    root,
		RestartCommand: []string{script},
		HealthCommand:  []string{script},
		CommandTimeout: Duration{Duration: defaultCommandTimeout},
	}, nil)
	update := Update{Product: "siemcore", ToVersion: "v1.0.0", ArtifactPath: artifact}

	if err := e.Apply(context.Background(), update); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := e.Validate(context.Background(), update); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := e.Rollback(context.Background(), update); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read phase log: %v", err)
	}
	want := "apply:apply\nhealth:health\nrollback:rollback\n"
	if string(data) != want {
		t.Fatalf("phase log = %q, want %q", string(data), want)
	}
}

func TestFilesystemExecutorRawArtifactCopiedAsFile(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(t.TempDir(), "siemcore-v1.0.0.artifact")
	if err := os.WriteFile(artifact, []byte("raw-binary-bytes"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	e := newFSExecutor(t, root)
	if err := e.Apply(context.Background(), Update{Product: "siemcore", ToVersion: "v1.0.0", ArtifactPath: artifact}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	target := resolveCurrent(t, e, "siemcore")
	got, err := os.ReadFile(filepath.Join(target, "siemcore-v1.0.0.artifact"))
	if err != nil {
		t.Fatalf("read installed raw file: %v", err)
	}
	if string(got) != "raw-binary-bytes" {
		t.Fatalf("installed raw file = %q", string(got))
	}
}
