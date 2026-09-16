package updatersim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The privileged host shim delegates through current, so product maintenance
// compensation must not assume that the failed bundle handles rollback.
func TestPodDelegationRestoresPreviousEntrypointBeforeRollback(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls")
	shim := filepath.Join(t.TempDir(), "shim")
	body := "#!/bin/sh\nset -eu\nprintf '%s:%s:%s\\n' \"$(cat \"$CURRENT_DIR/VERSION\")\" \"$UPDATER_PHASE\" \"$1\" >> '" + log + "'\nif [ \"$UPDATER_PHASE\" = health ] && [ \"$(cat \"$CURRENT_DIR/VERSION\")\" = 3.3.152.32 ]; then exit 1; fi\n"
	if err := os.WriteFile(shim, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	e := NewFilesystemExecutor(FilesystemConfig{InstallRoot: root, RestartCommand: []string{shim}, HealthCommand: []string{shim}, CommandTimeout: Duration{Duration: defaultCommandTimeout}}, discardLogger())
	old := filepath.Join(t.TempDir(), "old.tar.gz")
	next := filepath.Join(t.TempDir(), "next.tar.gz")
	makeTarGz(t, old, map[string]string{"VERSION": "3.3.152.30"})
	makeTarGz(t, next, map[string]string{"VERSION": "3.3.152.32"})
	initial := Update{Product: "siemcore", ToVersion: "3.3.152.30", ArtifactPath: old}
	if err := e.Apply(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	candidate := Update{Product: "siemcore", FromVersion: "3.3.152.30", ToVersion: "3.3.152.32", ArtifactPath: next}
	if err := e.Apply(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if err := e.Validate(context.Background(), candidate); err == nil {
		t.Fatal("expected unhealthy candidate")
	}
	if err := e.Rollback(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "3.3.152.30:apply:apply\n3.3.152.32:apply:apply\n3.3.152.32:health:health\n3.3.152.30:rollback:rollback\n"
	if string(data) != want {
		t.Fatalf("delegation order: %q", data)
	}
	if !strings.HasSuffix(resolveCurrent(t, e, "siemcore"), "3.3.152.30") {
		t.Fatal("old release not restored")
	}
}
