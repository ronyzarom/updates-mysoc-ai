package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/update"
)

var (
	updateConfigPath string
	updateForce      bool
)

var UpdateCmd = &cobra.Command{
	Use:   "update [product]",
	Short: "Check for and apply updates",
	Long: `Check for available updates and apply them.

If a product name is specified, only that product will be updated.
Otherwise, all products will be checked for updates.`,
	RunE: runUpdate,
}

func init() {
	UpdateCmd.Flags().StringVarP(&updateConfigPath, "config", "c", "", "Path to config file")
	UpdateCmd.Flags().BoolVarP(&updateForce, "force", "f", false, "Force update even if current version")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Check root
	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	// Find config file
	configPath := updateConfigPath
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

	// Create updater
	updater := update.NewUpdater(cfg)

	// Determine which products to update
	var products []string
	if len(args) > 0 {
		products = args
	} else {
		for _, p := range cfg.Products {
			products = append(products, p.Name)
		}
	}

	fmt.Println("Checking for updates...")

	for _, productName := range products {
		fmt.Printf("\n→ Checking %s...\n", productName)

		// Check for update
		hasUpdate, releaseInfo, err := updater.CheckUpdate(productName)
		if err != nil {
			fmt.Printf("  ⚠ Error checking for updates: %v\n", err)
			continue
		}

		if !hasUpdate && !updateForce {
			fmt.Printf("  ✓ Already up to date\n")
			continue
		}

		if releaseInfo != nil {
			fmt.Printf("  Update available: %s → %s\n", releaseInfo.CurrentVersion, releaseInfo.LatestVersion)
		}

		// Apply update
		fmt.Printf("  → Downloading...\n")
		if err := updater.ApplyUpdate(productName, releaseInfo); err != nil {
			fmt.Printf("  ❌ Update failed: %v\n", err)
			continue
		}

		fmt.Printf("  ✓ Updated to %s\n", releaseInfo.LatestVersion)
	}

	fmt.Println("\nUpdate check complete.")
	return nil
}

