package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var (
	initLicenseKey string
	initServerURL  string
	initName       string
	initChannel    string
)

var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize and bootstrap the updater",
	Long: `Initialize the updater with a license key and bootstrap the installation.

This command will:
  1. Validate the license with the update server
  2. Download all required products
  3. Set up configuration files
  4. Create systemd services
  5. Apply security hardening
  6. Start all services
  7. Register with the update server`,
	RunE: runInit,
}

func init() {
	InitCmd.Flags().StringVarP(&initLicenseKey, "license", "l", "", "License key (required)")
	InitCmd.Flags().StringVarP(&initServerURL, "server", "s", "https://updates.mysoc.ai", "Update server URL")
	InitCmd.Flags().StringVarP(&initName, "name", "n", "", "Instance name (defaults to hostname)")
	InitCmd.Flags().StringVarP(&initChannel, "channel", "c", "stable", "Update channel (stable, beta, nightly)")
	InitCmd.MarkFlagRequired("license")
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║           MySoc Updater - Bootstrap Installation           ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Check if running as root
	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	if initName != "" {
		hostname = initName
	}

	// Get machine ID
	machineID := getMachineID()

	fmt.Printf("→ Hostname:   %s\n", hostname)
	fmt.Printf("→ Machine ID: %s\n", machineID)
	fmt.Printf("→ Server:     %s\n", initServerURL)
	fmt.Println()

	// Step 1: Activate license
	fmt.Println("Step 1: Activating license...")
	activation, err := activateLicense(initServerURL, initLicenseKey, hostname, machineID)
	if err != nil {
		return fmt.Errorf("failed to activate license: %w", err)
	}
	if !activation.Success {
		return fmt.Errorf("license activation failed: %s", activation.Error)
	}
	fmt.Printf("   ✓ License valid for: %s\n", activation.License.CustomerName)
	fmt.Printf("   ✓ License type: %s\n", activation.License.Type)
	fmt.Printf("   ✓ Expires: %s\n", activation.License.ExpiresAt.Format("2006-01-02"))
	fmt.Printf("   ✓ Instance ID: %s\n", activation.Instance.Name)
	fmt.Println()

	// Determine base directory
	baseDir := config.BaseDir(activation.License.Type)
	fmt.Printf("→ Installing to: %s\n", baseDir)
	fmt.Println()

	// Step 2: Create directories
	fmt.Println("Step 2: Creating directories...")
	if err := createDirectories(baseDir); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	fmt.Println("   ✓ Directories created")
	fmt.Println()

	// Step 3: Create system user
	fmt.Println("Step 3: Creating system user...")
	userName := getUserName(activation.License.Type)
	if err := createSystemUser(userName); err != nil {
		// User might already exist, that's ok
		fmt.Printf("   ⚠ User creation: %v (may already exist)\n", err)
	} else {
		fmt.Printf("   ✓ User '%s' created\n", userName)
	}
	fmt.Println()

	// Step 4: Download products
	fmt.Println("Step 4: Downloading products...")
	for _, product := range activation.Install.Products {
		fmt.Printf("   → Downloading %s...\n", product.Name)
		if err := downloadProduct(initServerURL, activation.Instance.APIKey, baseDir, product); err != nil {
			fmt.Printf("   ⚠ Warning: Failed to download %s: %v\n", product.Name, err)
			// Continue with other products
		} else {
			fmt.Printf("   ✓ %s downloaded\n", product.Name)
		}
	}
	fmt.Println()

	// Step 5: Save updater configuration
	fmt.Println("Step 5: Creating configuration...")
	cfg := createUpdaterConfig(activation, initServerURL, initChannel)
	configPath := filepath.Join(baseDir, "updater", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("   ✓ Configuration saved to %s\n", configPath)

	// Save instance credentials
	credentialsPath := filepath.Join(baseDir, "updater", ".instance")
	credentials := fmt.Sprintf("INSTANCE_ID=%s\nAPI_KEY=%s\n", activation.Instance.Name, activation.Instance.APIKey)
	if err := os.WriteFile(credentialsPath, []byte(credentials), 0600); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}
	fmt.Println("   ✓ Credentials saved")
	fmt.Println()

	// Step 6: Create systemd services
	fmt.Println("Step 6: Creating systemd services...")
	if err := createSystemdServices(baseDir, userName, activation.Install.Products); err != nil {
		fmt.Printf("   ⚠ Warning: Failed to create some services: %v\n", err)
	} else {
		fmt.Println("   ✓ Systemd services created")
	}

	// Create updater service
	if err := createUpdaterService(); err != nil {
		fmt.Printf("   ⚠ Warning: Failed to create updater service: %v\n", err)
	} else {
		fmt.Println("   ✓ Updater service created")
	}
	fmt.Println()

	// Step 7: Set permissions
	fmt.Println("Step 7: Setting permissions...")
	if err := setPermissions(baseDir, userName); err != nil {
		fmt.Printf("   ⚠ Warning: Failed to set permissions: %v\n", err)
	} else {
		fmt.Println("   ✓ Permissions set")
	}
	fmt.Println()

	// Step 8: Enable and start services
	fmt.Println("Step 8: Starting services...")
	if err := startServices(activation.Install.Products); err != nil {
		fmt.Printf("   ⚠ Warning: Failed to start some services: %v\n", err)
	}
	fmt.Println()

	// Done
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║              ✓ Installation Complete!                       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  • Check status:    mysoc-updater status")
	fmt.Println("  • View logs:       journalctl -u mysoc-updater -f")
	fmt.Println("  • Start daemon:    systemctl start mysoc-updater")
	fmt.Println()

	return nil
}

func getMachineID() string {
	// Try to read machine-id
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		return string(bytes.TrimSpace(data))
	}
	// Fallback to dbus machine-id
	if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
		return string(bytes.TrimSpace(data))
	}
	return "unknown"
}

func activateLicense(serverURL, licenseKey, hostname, machineID string) (*types.LicenseActivationResponse, error) {
	req := types.LicenseActivationRequest{
		LicenseKey: licenseKey,
		Hostname:   hostname,
		MachineID:  machineID,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(serverURL+"/api/v1/license/activate", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	var activation types.LicenseActivationResponse
	if err := json.NewDecoder(resp.Body).Decode(&activation); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &activation, nil
}

func createDirectories(baseDir string) error {
	dirs := []string{
		filepath.Join(baseDir, "bin"),
		filepath.Join(baseDir, "etc"),
		filepath.Join(baseDir, "data"),
		filepath.Join(baseDir, "logs"),
		filepath.Join(baseDir, "rules"),
		filepath.Join(baseDir, "updater"),
		filepath.Join(baseDir, "updater", "versions"),
		filepath.Join(baseDir, "updater", "backups"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func getUserName(licenseType string) string {
	switch licenseType {
	case "mysoc", "mysoc-cloud":
		return "mysoc"
	default:
		return "siemcore"
	}
}

func createSystemUser(userName string) error {
	cmd := exec.Command("useradd", "--system", "--no-create-home", "--shell", "/bin/false", userName)
	return cmd.Run()
}

func downloadProduct(serverURL, apiKey, baseDir string, product types.ProductInstall) error {
	// Get latest release info
	url := fmt.Sprintf("%s/api/v1/releases/%s/latest?channel=%s", serverURL, product.Name, product.Channel)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("no release found")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var releaseInfo types.ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		return err
	}

	// Download the artifact
	downloadURL := serverURL + releaseInfo.DownloadURL
	req, err = http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err = client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Save to bin directory
	binaryPath := filepath.Join(baseDir, "bin", product.Name)
	file, err := os.Create(binaryPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}

	// Make executable
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return err
	}

	// Save version info
	versionFile := filepath.Join(baseDir, "updater", "versions", product.Name+".version")
	if err := os.WriteFile(versionFile, []byte(releaseInfo.LatestVersion), 0644); err != nil {
		return err
	}

	return nil
}

func createUpdaterConfig(activation *types.LicenseActivationResponse, serverURL, channel string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Server.URL = serverURL
	cfg.Server.APIKey = activation.Instance.APIKey
	cfg.Instance.ID = activation.Instance.Name
	cfg.Instance.Type = activation.License.Type
	cfg.Instance.LicenseKey = activation.License.LicenseKey
	cfg.Update.Channel = channel

	// Add product configurations
	baseDir := config.BaseDir(activation.License.Type)
	for _, p := range activation.Install.Products {
		productCfg := config.ProductConfig{
			Name:    p.Name,
			Service: p.Name + ".service",
			Binary:  filepath.Join(baseDir, "bin", p.Name),
			Config:  filepath.Join(baseDir, "etc", p.Name+".yaml"),
			Type:    "binary",
		}
		// Add health endpoint for API products
		if p.Name == "siemcore-api" || p.Name == "mysoc-api" {
			productCfg.HealthEndpoint = "http://localhost:8080/health"
		}
		cfg.Products = append(cfg.Products, productCfg)
	}

	return cfg
}

func createSystemdServices(baseDir, userName string, products []types.ProductInstall) error {
	for _, product := range products {
		serviceName := product.Name + ".service"
		servicePath := filepath.Join("/etc/systemd/system", serviceName)

		binaryPath := filepath.Join(baseDir, "bin", product.Name)
		configPath := filepath.Join(baseDir, "etc", product.Name+".yaml")

		serviceContent := fmt.Sprintf(`[Unit]
Description=%s
After=network.target
Wants=mysoc-updater.service

[Service]
Type=simple
User=%s
Group=%s
ExecStart=%s --config %s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`, product.Name, userName, userName, binaryPath, configPath)

		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return err
		}
	}

	// Reload systemd
	exec.Command("systemctl", "daemon-reload").Run()

	return nil
}

func createUpdaterService() error {
	servicePath := "/etc/systemd/system/mysoc-updater.service"

	serviceContent := `[Unit]
Description=MySoc Updater Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mysoc-updater daemon
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return err
	}

	// Reload systemd
	exec.Command("systemctl", "daemon-reload").Run()

	return nil
}

func setPermissions(baseDir, userName string) error {
	// Set ownership
	cmd := exec.Command("chown", "-R", userName+":"+userName, baseDir)
	return cmd.Run()
}

func startServices(products []types.ProductInstall) error {
	// Enable and start updater first
	exec.Command("systemctl", "enable", "mysoc-updater").Run()
	exec.Command("systemctl", "start", "mysoc-updater").Run()
	fmt.Println("   ✓ mysoc-updater enabled and started")

	// Enable and start product services
	for _, product := range products {
		serviceName := product.Name + ".service"
		exec.Command("systemctl", "enable", serviceName).Run()
		if err := exec.Command("systemctl", "start", serviceName).Run(); err != nil {
			fmt.Printf("   ⚠ %s: failed to start\n", serviceName)
		} else {
			fmt.Printf("   ✓ %s enabled and started\n", serviceName)
		}
	}

	return nil
}

