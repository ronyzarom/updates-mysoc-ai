//go:build unix

package updatersim

import (
	"os/exec"
	"syscall"
	"time"
)

// procGroupSigkillGrace is how long the group gets to honour SIGTERM before the
// whole group is SIGKILLed on cancellation.
const procGroupSigkillGrace = 2 * time.Second

// setProcessGroup puts the command in its own process group and installs a
// Cancel that signals the entire group. Without this, cmd's default Cancel
// kills only the immediate child (sudo), leaving a root wrapper it spawned
// running — which then races a rollback that starts a second copy.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// The child is the leader of its own group (Setpgid), so its pgid equals
		// its pid. Negative pid signals the whole group.
		pgid := cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		go func() {
			time.Sleep(procGroupSigkillGrace)
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}()
		return nil
	}
}
