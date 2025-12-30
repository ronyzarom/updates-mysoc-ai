package service

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
)

// Monitor watches services and restarts them if they crash
type Monitor struct {
	config       *config.Config
	restartCount map[string]int
	lastRestart  map[string]time.Time
}

// NewMonitor creates a new service monitor
func NewMonitor(cfg *config.Config) *Monitor {
	return &Monitor{
		config:       cfg,
		restartCount: make(map[string]int),
		lastRestart:  make(map[string]time.Time),
	}
}

// Start begins the service monitoring loop
func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAllServices()
		}
	}
}

// checkAllServices checks all managed services
func (m *Monitor) checkAllServices() {
	for _, product := range m.config.Products {
		status := m.getServiceStatus(product.Service)

		switch status {
		case "active":
			// Service is running, check health if endpoint available
			if product.HealthEndpoint != "" {
				if !m.checkHealth(product.HealthEndpoint) {
					fmt.Printf("Service %s is running but unhealthy\n", product.Service)
					m.restartService(product)
				}
			}
			// Reset restart count on healthy service
			m.restartCount[product.Service] = 0

		case "failed", "inactive":
			fmt.Printf("Service %s is %s, attempting restart\n", product.Service, status)
			m.restartService(product)
		}
	}
}

// getServiceStatus gets the status of a systemd service
func (m *Monitor) getServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// checkHealth checks a health endpoint
func (m *Monitor) checkHealth(endpoint string) bool {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// restartService attempts to restart a service
func (m *Monitor) restartService(product config.ProductConfig) {
	// Check restart cooldown (don't restart too frequently)
	if lastRestart, ok := m.lastRestart[product.Service]; ok {
		if time.Since(lastRestart) < 30*time.Second {
			fmt.Printf("Skipping restart of %s (cooldown period)\n", product.Service)
			return
		}
	}

	// Check restart count (don't restart infinitely)
	if count, ok := m.restartCount[product.Service]; ok && count >= 5 {
		fmt.Printf("Service %s has restarted too many times, giving up\n", product.Service)
		return
	}

	// Attempt restart
	cmd := exec.Command("systemctl", "restart", product.Service)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to restart %s: %v\n", product.Service, err)
		return
	}

	m.restartCount[product.Service]++
	m.lastRestart[product.Service] = time.Now()

	fmt.Printf("Restarted service %s (attempt %d)\n", product.Service, m.restartCount[product.Service])

	// Wait a moment and verify
	time.Sleep(5 * time.Second)
	if m.getServiceStatus(product.Service) != "active" {
		fmt.Printf("Service %s failed to start after restart\n", product.Service)
	}
}

// GetStatus returns the status of all managed services
func (m *Monitor) GetStatus() []ServiceStatus {
	var statuses []ServiceStatus

	for _, product := range m.config.Products {
		status := ServiceStatus{
			Name:   product.Service,
			Status: m.getServiceStatus(product.Service),
		}

		if product.HealthEndpoint != "" {
			status.Healthy = m.checkHealth(product.HealthEndpoint)
		} else {
			status.Healthy = status.Status == "active"
		}

		if count, ok := m.restartCount[product.Service]; ok {
			status.RestartCount = count
		}

		if lastRestart, ok := m.lastRestart[product.Service]; ok {
			status.LastRestart = lastRestart
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name         string
	Status       string
	Healthy      bool
	RestartCount int
	LastRestart  time.Time
}

