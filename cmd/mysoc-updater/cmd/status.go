package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
)

var statusConfigPath string

var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current status",
	Long:  `Display the current status of the updater, managed services, and security posture.`,
	RunE:  runStatus,
}

func init() {
	StatusCmd.Flags().StringVarP(&statusConfigPath, "config", "c", "", "Path to config file")
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Find config file
	configPath := statusConfigPath
	if configPath == "" {
		paths := []string{
			"/opt/siemcore/updater/config.yaml",
			"/opt/mysoc/updater/config.yaml",
			"./config.yaml",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	if configPath == "" {
		fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
		fmt.Println("║                  MySoc Updater Status                          ║")
		fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
		fmt.Println("║  Status: NOT INITIALIZED                                       ║")
		fmt.Println("║                                                                ║")
		fmt.Println("║  Run 'sudo mysoc-updater init --license YOUR-KEY' to start    ║")
		fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
		return nil
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Print status
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                  MySoc Updater Status                          ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	
	// Instance info
	fmt.Printf("║  Instance:     %-46s ║\n", cfg.Instance.ID)
	fmt.Printf("║  Type:         %-46s ║\n", cfg.Instance.Type)
	fmt.Printf("║  Server:       %-46s ║\n", truncate(cfg.Server.URL, 46))
	fmt.Printf("║  Channel:      %-46s ║\n", cfg.Update.Channel)
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")

	// License status
	licenseStatus := checkLicenseStatus(cfg)
	fmt.Printf("║  License:      %-46s ║\n", licenseStatus)
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")

	// Product status
	fmt.Println("║  Products:                                                     ║")
	for _, product := range cfg.Products {
		status := getServiceStatus(product.Service)
		version := getProductVersion(cfg, product.Name)
		line := fmt.Sprintf("%s v%s %s", product.Name, version, status)
		fmt.Printf("║    %-58s ║\n", line)
	}
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")

	// Security status
	securityScore := getSecurityScore(cfg)
	fmt.Printf("║  Security Score: %-44s ║\n", securityScore)
	
	// Updater daemon status
	updaterStatus := getServiceStatus("mysoc-updater.service")
	fmt.Printf("║  Updater Daemon: %-44s ║\n", updaterStatus)

	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")

	return nil
}

func checkLicenseStatus(cfg *config.Config) string {
	// Try to validate license with server
	if cfg.Server.URL == "" || cfg.Instance.LicenseKey == "" {
		return "⚠️  Not configured"
	}

	req := map[string]string{"license_key": cfg.Instance.LicenseKey}
	body, _ := json.Marshal(req)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(cfg.Server.URL+"/api/v1/license/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "⚠️  Unable to verify (offline?)"
	}
	defer resp.Body.Close()

	var result struct {
		Valid     bool      `json:"valid"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "⚠️  Invalid response"
	}

	if !result.Valid {
		return "❌ Invalid or expired"
	}

	daysLeft := int(time.Until(result.ExpiresAt).Hours() / 24)
	if daysLeft < 30 {
		return fmt.Sprintf("⚠️  Expires in %d days", daysLeft)
	}
	return fmt.Sprintf("✅ Valid (expires %s)", result.ExpiresAt.Format("2006-01-02"))
}

func getServiceStatus(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "❌ stopped"
	}

	status := strings.TrimSpace(string(output))
	switch status {
	case "active":
		return "✅ running"
	case "inactive":
		return "⚪ stopped"
	case "failed":
		return "❌ failed"
	default:
		return "⚠️  " + status
	}
}

func getProductVersion(cfg *config.Config, productName string) string {
	baseDir := config.BaseDir(cfg.Instance.Type)
	versionFile := filepath.Join(baseDir, "updater", "versions", productName+".version")
	
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "?.?.?"
	}
	return strings.TrimSpace(string(data))
}

func getSecurityScore(cfg *config.Config) string {
	if !cfg.Security.Enabled {
		return "⚪ Disabled"
	}

	// Basic security checks
	score := 0
	total := 5

	// Check firewall
	if cfg.Security.Firewall.Enabled {
		score++
	}

	// Check SSH
	if cfg.Security.SSH.Enabled {
		score++
	}

	// Check TLS
	if cfg.Security.TLS.Enabled {
		score++
	}

	// Check file integrity
	if cfg.Security.FileIntegrity.Enabled {
		score++
	}

	// Check compliance
	if cfg.Security.Compliance.Enabled {
		score++
	}

	percentage := (score * 100) / total
	if percentage >= 80 {
		return fmt.Sprintf("✅ %d/100", percentage)
	} else if percentage >= 50 {
		return fmt.Sprintf("⚠️  %d/100", percentage)
	}
	return fmt.Sprintf("❌ %d/100", percentage)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

