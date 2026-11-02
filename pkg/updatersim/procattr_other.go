//go:build !unix

package updatersim

import "os/exec"

// setProcessGroup is a no-op on platforms without POSIX process groups. WaitDelay
// (set by hardenCommand) still bounds a wedged Wait; only the group-signal
// escalation is unavailable.
func setProcessGroup(cmd *exec.Cmd) {}
