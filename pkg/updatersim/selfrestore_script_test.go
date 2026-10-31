package updatersim

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSelfRestoreScriptRollsBackToPrevious proves the Phase 2 root backstop
// (survivability invariant 4): when the updater has flapped and the restore unit
// runs the self-restore script, it points the self-update "current" symlink back
// at the recorded ".previous" target. It execs the real kit script against a
// temp layout via SELF_RESTORE_LAYOUT, so the shipped backstop is tested, not a
// reimplementation.
func TestSelfRestoreScriptRollsBackToPrevious(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script; not applicable on Windows")
	}
	cases := []struct {
		name   string
		script string // relative to this package dir
		binary string
	}{
		{"mysoc", "../../kits/mysoc/mysoc-updater-self-restore.sh", "mysoc-updater"},
		{"siemcore", "../../kits/siemcore/siemcore-cascade-updater-self-restore.sh", "siemcore-cascade-updater"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script, err := filepath.Abs(tc.script)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(script); err != nil {
				t.Fatalf("kit restore script missing: %v", err)
			}

			layout := t.TempDir()
			rel := func(v string) string { return filepath.Join(layout, "releases", v) }
			for _, v := range []string{"1.0.0", "2.0.0"} {
				if err := os.MkdirAll(rel(v), 0o755); err != nil {
					t.Fatal(err)
				}
				// The script requires <dir>/<name> to exist to treat a target as valid.
				if err := os.WriteFile(filepath.Join(rel(v), tc.binary), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			current := filepath.Join(layout, "current")
			if err := os.Symlink(rel("2.0.0"), current); err != nil {
				t.Fatal(err)
			}
			// .previous records the pre-self-update target (1.0.0).
			if err := os.WriteFile(filepath.Join(layout, ".previous"), []byte(rel("1.0.0")), 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "SELF_RESTORE_LAYOUT="+layout)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("restore script failed: %v\n%s", err, out)
			}

			got, err := os.Readlink(current)
			if err != nil {
				t.Fatal(err)
			}
			if got != rel("1.0.0") {
				t.Fatalf("current = %s after restore, want the previous target %s", got, rel("1.0.0"))
			}
		})
	}
}
