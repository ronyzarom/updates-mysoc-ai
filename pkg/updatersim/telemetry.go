package updatersim

import "errors"

// hostTelemetry is a platform-neutral set of real host measurements. Fields
// left at zero were not measured.
type hostTelemetry struct {
	CPUUsage    float64
	MemoryTotal int64
	MemoryUsed  int64
	DiskTotal   int64
	DiskUsed    int64
	LoadAverage float64
	Uptime      int64
}

var errInvalidProcFormat = errors.New("unexpected /proc format")
