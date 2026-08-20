//go:build linux

package updatersim

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// collectHostTelemetry gathers real host measurements from /proc and statfs.
// It returns ok=false when nothing could be measured (the caller then omits
// telemetry rather than reporting zeros as facts). Dependency-free on purpose:
// the updater runs on minimal hosts and must stay a single static binary.
func collectHostTelemetry() (metrics hostTelemetry, ok bool) {
	if uptime, err := readProcUptime(); err == nil {
		metrics.Uptime = uptime
		ok = true
	}
	if total, available, err := readProcMeminfo(); err == nil {
		metrics.MemoryTotal = total
		metrics.MemoryUsed = total - available
		ok = true
	}
	if total, used, err := statfsRoot(); err == nil {
		metrics.DiskTotal = total
		metrics.DiskUsed = used
		ok = true
	}
	if load, err := readLoadAverage(); err == nil {
		metrics.LoadAverage = load
		ok = true
	}
	if cpu, err := sampleCPUUsage(200 * time.Millisecond); err == nil {
		metrics.CPUUsage = cpu
		ok = true
	}
	return metrics, ok
}

// readProcUptime returns whole seconds since boot.
func readProcUptime() (int64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errInvalidProcFormat
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return int64(seconds), nil
}

// readProcMeminfo returns MemTotal and MemAvailable in bytes.
func readProcMeminfo() (total, available int64, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			available = value * 1024
		}
	}
	if total == 0 {
		return 0, 0, errInvalidProcFormat
	}
	return total, available, nil
}

// statfsRoot returns total and used bytes of the root filesystem.
func statfsRoot() (total, used int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, err
	}
	blockSize := int64(stat.Bsize)
	total = int64(stat.Blocks) * blockSize
	free := int64(stat.Bfree) * blockSize
	return total, total - free, nil
}

// readLoadAverage returns the 1-minute load average.
func readLoadAverage() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errInvalidProcFormat
	}
	return strconv.ParseFloat(fields[0], 64)
}

// sampleCPUUsage measures aggregate CPU utilization (percent) across a short
// sampling window from /proc/stat deltas. The window blocks heartbeat
// assembly briefly, which is negligible at heartbeat intervals.
func sampleCPUUsage(window time.Duration) (float64, error) {
	idle1, total1, err := readProcStatCPU()
	if err != nil {
		return 0, err
	}
	time.Sleep(window)
	idle2, total2, err := readProcStatCPU()
	if err != nil {
		return 0, err
	}
	totalDelta := total2 - total1
	if totalDelta <= 0 {
		return 0, nil
	}
	idleDelta := idle2 - idle1
	usage := 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage, nil
}

// readProcStatCPU parses the aggregate "cpu" line: idle = idle + iowait,
// total = sum of all fields.
func readProcStatCPU() (idle, total uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		for i, field := range fields {
			value, parseErr := strconv.ParseUint(field, 10, 64)
			if parseErr != nil {
				continue
			}
			total += value
			// Fields 3 (idle) and 4 (iowait) count as idle time.
			if i == 3 || i == 4 {
				idle += value
			}
		}
		return idle, total, nil
	}
	return 0, 0, errInvalidProcFormat
}
