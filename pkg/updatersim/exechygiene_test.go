package updatersim

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestRunCommandBackgroundedChildDoesNotWedge is the regression test for the
// deadlock where a hook backgrounds a daemon that inherits (holds) stdout. With
// the old CombinedOutput()/no-WaitDelay path, cmd.Wait blocked until the
// backgrounded child exited (here, 30s). Setpgid + Cancel(-pgid) + WaitDelay
// must bound it: on the command timeout the whole group is signaled and Wait
// returns promptly, well under the child's lifetime.
func TestRunCommandBackgroundedChildDoesNotWedge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group semantics; shell-based hook")
	}
	root := t.TempDir()
	e := NewFilesystemExecutor(FilesystemConfig{
		InstallRoot:    root,
		CommandTimeout: Duration{Duration: time.Second},
	}, discardLogger())

	// The main shell exits immediately but backgrounds a long sleep that
	// inherits stdout. Without the fix, Wait would block ~30s.
	argv := []string{"sh", "-c", "sleep 30 & echo started; exit 0"}
	update := Update{Product: "p", FromVersion: "1", ToVersion: "2"}

	start := time.Now()
	err := e.runCommand(context.Background(), "restart", "apply", argv, update)
	elapsed := time.Since(start)

	// It must return long before the backgrounded child's 30s lifetime. Allow a
	// generous bound covering CommandTimeout(1s) + WaitDelay(5s) + scheduling.
	if elapsed > 15*time.Second {
		t.Fatalf("runCommand wedged on backgrounded pipe-holder: took %s", elapsed)
	}
	// The command itself exited 0; whether Run reports a WaitDelay/timeout error
	// is acceptable either way. We only assert it did not hang.
	_ = err
}
