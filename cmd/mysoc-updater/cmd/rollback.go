package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
)

var rollbackConfigPath string

var RollbackCmd = &cobra.Command{
	Use:   "rollback <product>",
	Short: "Rollback a product to previous version",
	Long: `Rollback a product to its previous version.

This command restores the previous binary from backup and restarts the service.`,
	Args: cobra.ExactArgs(1),
	RunE: runRollback,
}

func init() {
	RollbackCmd.Flags().StringVarP(&rollbackConfigPath, "config", "c", "", "Path to config file")
}

func runRollback(cmd *cobra.Command, args []string) error {
	productName := args[0]

	// Check root
	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	// Find config file
	configPath := rollbackConfigPath
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
		return fmt.Errorf("no config file found. Run 'mysoc-updater init' first")
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find product config
	var productCfg *config.ProductConfig
	for i := range cfg.Products {
		if cfg.Products[i].Name == productName {
			productCfg = &cfg.Products[i]
			break
		}
	}

	if productCfg == nil {
		return fmt.Errorf("product '%s' not found in configuration", productName)
	}

	baseDir := config.BaseDir(cfg.Instance.Type)
	backupDir := filepath.Join(baseDir, "updater", "backups")

	// Find latest backup
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	var latestBackup string
	var latestVersion string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), productName+".") && strings.HasSuffix(entry.Name(), ".bak") {
			// Extract version from filename (product.version.bak)
			parts := strings.Split(entry.Name(), ".")
			if len(parts) >= 3 {
				version := parts[1]
				if latestBackup == "" || version > latestVersion {
					latestBackup = entry.Name()
					latestVersion = version
				}
			}
		}
	}

	if latestBackup == "" {
		return fmt.Errorf("no backup found for %s", productName)
	}

	backupPath := filepath.Join(backupDir, latestBackup)
	binaryPath := productCfg.Binary

	fmt.Printf("Rolling back %s to version %s...\n", productName, latestVersion)

	// Stop service
	fmt.Printf("→ Stopping service %s...\n", productCfg.Service)
	if err := exec.Command("systemctl", "stop", productCfg.Service).Run(); err != nil {
		fmt.Printf("  ⚠ Warning: failed to stop service: %v\n", err)
	}

	// Backup current binary
	currentVersion := getCurrentVersion(cfg, productName)
	if currentVersion != "" {
		currentBackup := filepath.Join(backupDir, fmt.Sprintf("%s.%s.current.bak", productName, currentVersion))
		if err := exec.Command("cp", binaryPath, currentBackup).Run(); err != nil {
			fmt.Printf("  ⚠ Warning: failed to backup current binary: %v\n", err)
		}
	}

	// Restore backup
	fmt.Printf("→ Restoring backup...\n")
	if err := exec.Command("cp", backupPath, binaryPath).Run(); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	// Set permissions
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Update version file
	versionFile := filepath.Join(baseDir, "updater", "versions", productName+".version")
	if err := os.WriteFile(versionFile, []byte(latestVersion), 0644); err != nil {
		fmt.Printf("  ⚠ Warning: failed to update version file: %v\n", err)
	}

	// Start service
	fmt.Printf("→ Starting service %s...\n", productCfg.Service)
	if err := exec.Command("systemctl", "start", productCfg.Service).Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Printf("✓ Rolled back %s to version %s\n", productName, latestVersion)
	return nil
}

func getCurrentVersion(cfg *config.Config, productName string) string {
	baseDir := config.BaseDir(cfg.Instance.Type)
	versionFile := filepath.Join(baseDir, "updater", "versions", productName+".version")

	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

