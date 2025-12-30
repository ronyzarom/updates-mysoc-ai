package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Reporter sends heartbeats to the update server
type Reporter struct {
	config *config.Config
	client *http.Client
}

// NewReporter creates a new heartbeat reporter
func NewReporter(cfg *config.Config) *Reporter {
	return &Reporter{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Heartbeat.Timeout,
		},
	}
}

// Start begins the heartbeat reporting loop
func (r *Reporter) Start(ctx context.Context) {
	ticker := time.NewTicker(r.config.Heartbeat.Interval)
	defer ticker.Stop()

	// Send initial heartbeat
	r.sendHeartbeat()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sendHeartbeat()
		}
	}
}

// sendHeartbeat sends a single heartbeat to the server
func (r *Reporter) sendHeartbeat() {
	heartbeat := r.collectHeartbeat()

	body, err := json.Marshal(heartbeat)
	if err != nil {
		fmt.Printf("Failed to marshal heartbeat: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", r.config.Server.URL+"/api/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Failed to create heartbeat request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", r.config.Server.APIKey)

	resp, err := r.client.Do(req)
	if err != nil {
		fmt.Printf("Failed to send heartbeat: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Heartbeat returned status %d\n", resp.StatusCode)
	}
}

// collectHeartbeat gathers all heartbeat data
func (r *Reporter) collectHeartbeat() types.Heartbeat {
	hostname, _ := os.Hostname()

	return types.Heartbeat{
		InstanceID:     r.config.Instance.ID,
		InstanceType:   r.config.Instance.Type,
		Hostname:       hostname,
		UpdaterVersion: "1.0.0", // TODO: Get from version
		ConfigHash:     r.getConfigHash(),
		License:        r.getLicenseStatus(),
		Products:       r.getProductStatuses(),
		System:         r.getSystemMetrics(),
		Security:       r.getSecurityStatus(),
		Timestamp:      time.Now(),
	}
}

func (r *Reporter) getConfigHash() string {
	// Simple hash of config for change detection
	return fmt.Sprintf("%x", time.Now().Unix()/3600) // Changes hourly
}

func (r *Reporter) getLicenseStatus() types.LicenseStatus {
	return types.LicenseStatus{
		Key:       r.config.Instance.LicenseKey,
		Valid:     true, // Assume valid, server will validate
		LastCheck: time.Now(),
	}
}

func (r *Reporter) getProductStatuses() []types.ProductStatus {
	var statuses []types.ProductStatus

	for _, product := range r.config.Products {
		status := types.ProductStatus{
			Name:    product.Name,
			Version: r.getProductVersion(product.Name),
			Channel: r.config.Update.Channel,
			Status:  r.getServiceStatus(product.Service),
		}

		// Get PID if running
		if status.Status == "running" {
			status.PID = r.getServicePID(product.Service)
		}

		// Check health endpoint if available
		if product.HealthEndpoint != "" {
			status.HealthEndpoint = product.HealthEndpoint
			status.HealthStatus = r.checkHealthEndpoint(product.HealthEndpoint)
		}

		statuses = append(statuses, status)
	}

	return statuses
}

func (r *Reporter) getProductVersion(productName string) string {
	baseDir := config.BaseDir(r.config.Instance.Type)
	versionFile := filepath.Join(baseDir, "updater", "versions", productName+".version")

	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

func (r *Reporter) getServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "stopped"
	}

	status := strings.TrimSpace(string(output))
	switch status {
	case "active":
		return "running"
	case "inactive":
		return "stopped"
	case "failed":
		return "crashed"
	default:
		return status
	}
}

func (r *Reporter) getServicePID(serviceName string) int {
	cmd := exec.Command("systemctl", "show", serviceName, "--property=MainPID", "--value")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	var pid int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid)
	return pid
}

func (r *Reporter) checkHealthEndpoint(endpoint string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return "unhealthy"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return "healthy"
	}
	return "unhealthy"
}

func (r *Reporter) getSystemMetrics() types.SystemMetrics {
	metrics := types.SystemMetrics{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// CPU usage (simplified)
	metrics.CPUUsage = r.getCPUUsage()

	// Memory
	metrics.MemoryTotal, metrics.MemoryUsed = r.getMemoryInfo()

	// Disk
	metrics.DiskTotal, metrics.DiskUsed = r.getDiskInfo()

	// Load average
	metrics.LoadAverage = r.getLoadAverage()

	// System uptime
	metrics.Uptime = r.getSystemUptime()

	return metrics
}

func (r *Reporter) getCPUUsage() float64 {
	// Read from /proc/stat
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 1 {
		return 0
	}

	// Parse first line (cpu aggregate)
	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return 0
	}

	// This is a simplified calculation
	var total, idle int64
	for i := 1; i < len(fields); i++ {
		var val int64
		fmt.Sscanf(fields[i], "%d", &val)
		total += val
		if i == 4 { // idle is the 4th value
			idle = val
		}
	}

	if total == 0 {
		return 0
	}
	return float64(total-idle) / float64(total) * 100
}

func (r *Reporter) getMemoryInfo() (total, used int64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(data), "\n")
	var memTotal, memAvailable int64

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		var val int64
		fmt.Sscanf(fields[1], "%d", &val)
		val *= 1024 // Convert from KB to bytes

		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
	}

	return memTotal, memTotal - memAvailable
}

func (r *Reporter) getDiskInfo() (total, used int64) {
	// Use df command for simplicity
	cmd := exec.Command("df", "-B1", "/")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, 0
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0
	}

	fmt.Sscanf(fields[1], "%d", &total)
	fmt.Sscanf(fields[2], "%d", &used)

	return total, used
}

func (r *Reporter) getLoadAverage() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}

	var load float64
	fmt.Sscanf(fields[0], "%f", &load)
	return load
}

func (r *Reporter) getSystemUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}

	var uptime float64
	fmt.Sscanf(fields[0], "%f", &uptime)
	return int64(uptime)
}

func (r *Reporter) getSecurityStatus() types.SecurityStatus {
	status := types.SecurityStatus{
		FirewallEnabled: r.config.Security.Firewall.Enabled,
		SSHHardened:     r.config.Security.SSH.Enabled,
		LastScan:        time.Now(),
	}

	// Check for pending updates
	status.PendingUpdates, status.SecurityUpdates = r.checkPendingUpdates()

	// Check if reboot required
	status.RebootRequired = r.checkRebootRequired()

	return status
}

func (r *Reporter) checkPendingUpdates() (pending, security int) {
	// Check for apt updates (Debian/Ubuntu)
	cmd := exec.Command("apt-get", "-s", "upgrade")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "upgraded,") {
			fmt.Sscanf(line, "%d upgraded,", &pending)
		}
	}

	// Check for security updates
	cmd = exec.Command("apt-get", "-s", "upgrade", "-o", "Dir::Etc::sourcelist=/etc/apt/sources.list.d/security.sources")
	output, err = cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, "upgraded,") {
				fmt.Sscanf(line, "%d upgraded,", &security)
			}
		}
	}

	return pending, security
}

func (r *Reporter) checkRebootRequired() bool {
	_, err := os.Stat("/var/run/reboot-required")
	return err == nil
}

