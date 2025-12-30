package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// Checker periodically checks for updates
type Checker struct {
	config  *config.Config
	client  *http.Client
	updater *Updater
}

// NewChecker creates a new update checker
func NewChecker(cfg *config.Config) *Checker {
	return &Checker{
		config:  cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		updater: NewUpdater(cfg),
	}
}

// Start begins the update checking loop
func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.config.Update.CheckInterval)
	defer ticker.Stop()

	// Initial check after a short delay
	time.Sleep(10 * time.Second)
	c.checkAllUpdates()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if c.isInMaintenanceWindow() || c.config.Update.MaintenanceWindow == nil {
				c.checkAllUpdates()
			}
		}
	}
}

// checkAllUpdates checks and applies updates for all products
func (c *Checker) checkAllUpdates() {
	for _, product := range c.config.Products {
		hasUpdate, releaseInfo, err := c.updater.CheckUpdate(product.Name)
		if err != nil {
			fmt.Printf("Error checking update for %s: %v\n", product.Name, err)
			continue
		}

		if hasUpdate && c.config.Update.AutoUpdate {
			fmt.Printf("Update available for %s: %s -> %s\n",
				product.Name, releaseInfo.CurrentVersion, releaseInfo.LatestVersion)

			if err := c.updater.ApplyUpdate(product.Name, releaseInfo); err != nil {
				fmt.Printf("Error applying update for %s: %v\n", product.Name, err)
			} else {
				fmt.Printf("Successfully updated %s to %s\n", product.Name, releaseInfo.LatestVersion)
			}
		}
	}
}

// isInMaintenanceWindow checks if current time is in maintenance window
func (c *Checker) isInMaintenanceWindow() bool {
	if c.config.Update.MaintenanceWindow == nil {
		return true // No window defined, always allow
	}

	now := time.Now()
	window := c.config.Update.MaintenanceWindow

	// Parse start and end times
	startParts := strings.Split(window.Start, ":")
	endParts := strings.Split(window.End, ":")

	if len(startParts) != 2 || len(endParts) != 2 {
		return true // Invalid format, allow updates
	}

	var startHour, startMin, endHour, endMin int
	fmt.Sscanf(startParts[0], "%d", &startHour)
	fmt.Sscanf(startParts[1], "%d", &startMin)
	fmt.Sscanf(endParts[0], "%d", &endHour)
	fmt.Sscanf(endParts[1], "%d", &endMin)

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin

	if startMinutes < endMinutes {
		// Normal window (e.g., 02:00 - 05:00)
		return currentMinutes >= startMinutes && currentMinutes <= endMinutes
	}
	// Window crosses midnight (e.g., 23:00 - 03:00)
	return currentMinutes >= startMinutes || currentMinutes <= endMinutes
}

// Updater handles downloading and applying updates
type Updater struct {
	config *config.Config
	client *http.Client
}

// NewUpdater creates a new updater
func NewUpdater(cfg *config.Config) *Updater {
	return &Updater{
		config: cfg,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

// CheckUpdate checks if an update is available for a product
func (u *Updater) CheckUpdate(productName string) (bool, *types.ReleaseInfo, error) {
	currentVersion := u.getCurrentVersion(productName)

	url := fmt.Sprintf("%s/api/v1/releases/%s/latest?channel=%s&current_version=%s",
		u.config.Server.URL, productName, u.config.Update.Channel, currentVersion)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("X-API-Key", u.config.Server.APIKey)

	resp, err := u.client.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var releaseInfo types.ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return false, nil, err
	}

	return releaseInfo.UpdateAvailable, &releaseInfo, nil
}

// ApplyUpdate downloads and applies an update
func (u *Updater) ApplyUpdate(productName string, releaseInfo *types.ReleaseInfo) error {
	// Find product config
	var productCfg *config.ProductConfig
	for i := range u.config.Products {
		if u.config.Products[i].Name == productName {
			productCfg = &u.config.Products[i]
			break
		}
	}
	if productCfg == nil {
		return fmt.Errorf("product %s not found in config", productName)
	}

	baseDir := config.BaseDir(u.config.Instance.Type)
	backupDir := filepath.Join(baseDir, "updater", "backups")
	tempDir := filepath.Join(baseDir, "updater", "temp")

	// Ensure directories exist
	os.MkdirAll(backupDir, 0755)
	os.MkdirAll(tempDir, 0755)

	// Download new version
	downloadURL := u.config.Server.URL + releaseInfo.DownloadURL
	tempPath := filepath.Join(tempDir, productName+"-"+releaseInfo.LatestVersion)

	if err := u.downloadFile(downloadURL, tempPath); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Backup current version
	currentVersion := u.getCurrentVersion(productName)
	if currentVersion != "" {
		backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak", productName, currentVersion))
		if _, err := os.Stat(productCfg.Binary); err == nil {
			if err := copyFile(productCfg.Binary, backupPath); err != nil {
				fmt.Printf("Warning: failed to backup current version: %v\n", err)
			}
		}
	}

	// Stop service
	if productCfg.Service != "" {
		if err := runCommand("systemctl", "stop", productCfg.Service); err != nil {
			// Log but continue
			fmt.Printf("Warning: failed to stop service: %v\n", err)
		}
	}

	// Replace binary
	if err := os.Rename(tempPath, productCfg.Binary); err != nil {
		// Try to restore from backup
		backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak", productName, currentVersion))
		copyFile(backupPath, productCfg.Binary)
		return fmt.Errorf("failed to install new version: %w", err)
	}

	// Set permissions
	os.Chmod(productCfg.Binary, 0755)

	// Start service
	if productCfg.Service != "" {
		if err := runCommand("systemctl", "start", productCfg.Service); err != nil {
			// Rollback
			backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak", productName, currentVersion))
			copyFile(backupPath, productCfg.Binary)
			runCommand("systemctl", "start", productCfg.Service)
			return fmt.Errorf("failed to start service after update: %w", err)
		}
	}

	// Update version file
	versionFile := filepath.Join(baseDir, "updater", "versions", productName+".version")
	os.WriteFile(versionFile, []byte(releaseInfo.LatestVersion), 0644)

	return nil
}

func (u *Updater) getCurrentVersion(productName string) string {
	baseDir := config.BaseDir(u.config.Instance.Type)
	versionFile := filepath.Join(baseDir, "updater", "versions", productName+".version")

	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (u *Updater) downloadFile(url, destPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", u.config.Server.APIKey)

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			file.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	return nil
}

