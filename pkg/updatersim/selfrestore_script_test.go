package updatersim

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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

// TestSelfRestoreScriptSkipsNonSelfUpdateFailure proves the gate (h8): a
// start-limit failure that is NOT a self-update (no pending marker in state.json
// and a stale .previous) must not roll the layout back to a possibly weeks-old
// binary. The current symlink must be left untouched.
func TestSelfRestoreScriptSkipsNonSelfUpdateFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script; not applicable on Windows")
	}
	cases := []struct {
		name   string
		script string
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
			layout := t.TempDir()
			rel := func(v string) string { return filepath.Join(layout, "releases", v) }
			for _, v := range []string{"1.0.0", "2.0.0"} {
				if err := os.MkdirAll(rel(v), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(rel(v), tc.binary), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			current := filepath.Join(layout, "current")
			if err := os.Symlink(rel("2.0.0"), current); err != nil {
				t.Fatal(err)
			}
			prev := filepath.Join(layout, ".previous")
			if err := os.WriteFile(prev, []byte(rel("1.0.0")), 0o644); err != nil {
				t.Fatal(err)
			}
			// Make .previous stale (older than the 1 h window) so it is not taken
			// as evidence of a recent activation.
			old := time.Now().Add(-48 * time.Hour)
			if err := os.Chtimes(prev, old, old); err != nil {
				t.Fatal(err)
			}
			// No state.json => no pending_self_update marker.
			stateFile := filepath.Join(t.TempDir(), "state.json")

			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(),
				"SELF_RESTORE_LAYOUT="+layout,
				"SELF_RESTORE_STATE="+stateFile,
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("restore script failed: %v\n%s", err, out)
			}

			got, err := os.Readlink(current)
			if err != nil {
				t.Fatal(err)
			}
			if got != rel("2.0.0") {
				t.Fatalf("current = %s; a non-self-update failure must not roll back (want %s)", got, rel("2.0.0"))
			}
		})
	}
}

// TestSelfRestoreScriptRollsBackOnPendingMarker proves the pending_self_update
// marker in state.json is sufficient evidence to roll back, even when .previous
// is stale.
func TestSelfRestoreScriptRollsBackOnPendingMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script; not applicable on Windows")
	}
	cases := []struct {
		name   string
		script string
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
			layout := t.TempDir()
			rel := func(v string) string { return filepath.Join(layout, "releases", v) }
			for _, v := range []string{"1.0.0", "2.0.0"} {
				if err := os.MkdirAll(rel(v), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(rel(v), tc.binary), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			current := filepath.Join(layout, "current")
			if err := os.Symlink(rel("2.0.0"), current); err != nil {
				t.Fatal(err)
			}
			prev := filepath.Join(layout, ".previous")
			if err := os.WriteFile(prev, []byte(rel("1.0.0")), 0o644); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-48 * time.Hour)
			if err := os.Chtimes(prev, old, old); err != nil {
				t.Fatal(err)
			}
			stateFile := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(stateFile, []byte(`{"pending_self_update":{"to_version":"2.0.0"}}`), 0o644); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(),
				"SELF_RESTORE_LAYOUT="+layout,
				"SELF_RESTORE_STATE="+stateFile,
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("restore script failed: %v\n%s", err, out)
			}

			got, err := os.Readlink(current)
			if err != nil {
				t.Fatal(err)
			}
			if got != rel("1.0.0") {
				t.Fatalf("current = %s; a pending-marker failure must roll back (want %s)", got, rel("1.0.0"))
			}
		})
	}
}
