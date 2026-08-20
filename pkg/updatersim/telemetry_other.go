//go:build !linux

package updatersim

// collectHostTelemetry has no non-Linux implementation; the heartbeat then
// carries no host measurements and the dashboard renders "Not reported".
func collectHostTelemetry() (hostTelemetry, bool) {
	return hostTelemetry{}, false
}
