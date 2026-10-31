package updatersim

import (
	"net"
	"os"
)

// sd_notify (systemd) support, implemented with the standard library only so
// the updater keeps zero external dependencies. Every function is inert when
// NOTIFY_SOCKET is unset (i.e. not launched by systemd with a notify unit), so
// it is safe to call unconditionally and ships now, ahead of the Phase 2 unit
// that sets Type=notify/WatchdogSec.
//
// Two signals matter for survivability:
//   - READY=1 after the updater has resolved any pending self-update and is
//     serving: this is what lets the Phase 2 unit use Type=notify so a binary
//     that never reaches readiness is treated as a failed start (and the
//     OnFailure restore unit can roll back to .previous).
//   - WATCHDOG=1 once per cycle while healthy: with WatchdogSec set, a process
//     that stops pinging (a wedged cycle, a dead relay listener) is killed and
//     restarted by systemd. The relay only pings after its listener self-probe
//     passes, so a live process with a dead cascade listener does not keep the
//     watchdog satisfied.

// sdNotify sends one datagram to the systemd notify socket. It is a no-op when
// NOTIFY_SOCKET is unset or cannot be reached; notification is best-effort and
// must never affect the updater's own behaviour.
func sdNotify(state string) {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return
	}
	// Abstract-namespace sockets start with '@'; the kernel expects a leading
	// NUL for those.
	addr := socket
	if len(addr) > 0 && addr[0] == '@' {
		addr = "\x00" + addr[1:]
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(state))
}

// sdNotifyReady signals that startup is complete and the updater is serving.
func sdNotifyReady() { sdNotify("READY=1") }

// sdNotifyWatchdog pings the systemd watchdog. Call only when the updater is
// genuinely healthy this cycle.
func sdNotifyWatchdog() { sdNotify("WATCHDOG=1") }

// sdWatchdogEnabled reports whether systemd asked for watchdog pings
// (WATCHDOG_USEC set and this is the main pid). Used to decide whether to ping.
func sdWatchdogEnabled() bool {
	return os.Getenv("NOTIFY_SOCKET") != "" && os.Getenv("WATCHDOG_USEC") != ""
}

// NotifyReady tells the service manager (systemd Type=notify) that the updater
// has finished startup, settled any pending self-update, and is serving. Inert
// unless launched under a notify unit.
func (s *Simulator) NotifyReady() { sdNotifyReady() }
