//go:build linux

package updatersim

import "testing"

// Runs only on a real Linux host (CI or a container): the collector must
// return sane, nonzero measurements there.
func TestCollectHostTelemetryLinux(t *testing.T) {
	metrics, ok := collectHostTelemetry()
	if !ok {
		t.Fatal("expected telemetry collection to succeed on linux")
	}
	if metrics.MemoryTotal <= 0 {
		t.Errorf("memory_total not measured: %d", metrics.MemoryTotal)
	}
	if metrics.MemoryUsed < 0 || metrics.MemoryUsed > metrics.MemoryTotal {
		t.Errorf("memory_used out of range: %d of %d", metrics.MemoryUsed, metrics.MemoryTotal)
	}
	if metrics.DiskTotal <= 0 || metrics.DiskUsed < 0 || metrics.DiskUsed > metrics.DiskTotal {
		t.Errorf("disk measurements out of range: used %d of %d", metrics.DiskUsed, metrics.DiskTotal)
	}
	if metrics.Uptime <= 0 {
		t.Errorf("uptime not measured: %d", metrics.Uptime)
	}
	if metrics.CPUUsage < 0 || metrics.CPUUsage > 100 {
		t.Errorf("cpu usage out of range: %f", metrics.CPUUsage)
	}
}
