package updatersim

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"
)

// commandWaitDelay bounds how long cmd.Wait blocks after the process exits (or
// its context is cancelled) while inherited I/O pipes are still open. A hook
// that backgrounds a daemon holding stdout would otherwise block Wait forever;
// WaitDelay closes the pipes and forces completion.
const commandWaitDelay = 5 * time.Second

// maxCommandOutputBytes caps captured hook/validation output so a chatty or
// runaway command cannot balloon updater memory. Output past the cap is dropped
// (the command is not failed for being verbose).
const maxCommandOutputBytes = 64 << 10

// hardenCommand applies the process-execution hygiene every external command in
// the updater needs:
//
//   - WaitDelay bounds Wait so a backgrounded grandchild holding stdout cannot
//     wedge the cycle past its deadline.
//   - setProcessGroup (platform-specific) puts the child in its own process
//     group and, on cancellation, signals the whole group (SIGTERM then
//     SIGKILL) so a root wrapper launched via sudo is terminated too, not just
//     sudo itself.
func hardenCommand(cmd *exec.Cmd, waitDelay time.Duration) {
	cmd.WaitDelay = waitDelay
	setProcessGroup(cmd)
}

// boundedOutput is an io.Writer that captures up to maxCommandOutputBytes and
// silently truncates the rest. It never returns a short write or error, so the
// os/exec output-copy goroutine is never blocked or made to kill the process
// merely because output exceeded the cap.
type boundedOutput struct {
	buf       bytes.Buffer
	truncated bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if room := maxCommandOutputBytes - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
			b.truncated = true
		} else {
			b.buf.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedOutput) String() string {
	if b.truncated {
		return b.buf.String() + fmt.Sprintf("\n[output truncated at %d bytes]", maxCommandOutputBytes)
	}
	return b.buf.String()
}
