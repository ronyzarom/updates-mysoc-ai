package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/security"
)

var securityConfigPath string

var SecurityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security management commands",
	Long:  `Manage security hardening and compliance.`,
}

var securityScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Run security scan",
	RunE:  runSecurityScan,
}

var securityApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply security hardening",
	RunE:  runSecurityApply,
}

var securityStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show security status",
	RunE:  runSecurityStatus,
}

func init() {
	SecurityCmd.PersistentFlags().StringVarP(&securityConfigPath, "config", "c", "", "Path to config file")

	SecurityCmd.AddCommand(securityScanCmd)
	SecurityCmd.AddCommand(securityApplyCmd)
	SecurityCmd.AddCommand(securityStatusCmd)
}

func loadSecurityConfig() (*config.Config, error) {
	configPath := securityConfigPath
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
		return nil, fmt.Errorf("no config file found")
	}

	return config.Load(configPath)
}

func runSecurityScan(cmd *cobra.Command, args []string) error {
	cfg, err := loadSecurityConfig()
	if err != nil {
		return err
	}

	if !cfg.Security.Enabled {
		fmt.Println("Security module is disabled in configuration.")
		return nil
	}

	fmt.Println("Running security scan...")
	fmt.Println()

	scanner := security.NewScanner(cfg)
	results := scanner.Scan()

	// Print results
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   Security Scan Results                        ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")

	for _, result := range results.Checks {
		status := "✅"
		if !result.Passed {
			status = "❌"
		}
		fmt.Printf("║  %s %-56s ║\n", status, result.Name)
		if !result.Passed && result.Details != "" {
			fmt.Printf("║     └─ %-54s ║\n", truncate(result.Details, 54))
		}
	}

	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Security Score: %d/100                                        ║\n", results.Score)
	fmt.Printf("║  Passed: %d/%d checks                                           ║\n", results.PassedCount, results.TotalCount)
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")

	return nil
}

func runSecurityApply(cmd *cobra.Command, args []string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	cfg, err := loadSecurityConfig()
	if err != nil {
		return err
	}

	if !cfg.Security.Enabled {
		fmt.Println("Security module is disabled in configuration.")
		return nil
	}

	fmt.Println("Applying security hardening...")
	fmt.Println()

	hardener := security.NewHardener(cfg)
	results := hardener.Apply()

	for _, result := range results {
		status := "✅"
		if !result.Success {
			status = "❌"
		}
		fmt.Printf("%s %s\n", status, result.Name)
		if !result.Success && result.Error != "" {
			fmt.Printf("   └─ %s\n", result.Error)
		}
	}

	fmt.Println()
	fmt.Println("Security hardening complete.")
	return nil
}

func runSecurityStatus(cmd *cobra.Command, args []string) error {
	cfg, err := loadSecurityConfig()
	if err != nil {
		return err
	}

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   Security Status                              ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")

	if !cfg.Security.Enabled {
		fmt.Println("║  Status: DISABLED                                              ║")
		fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
		return nil
	}

	// Module status
	modules := []struct {
		name    string
		enabled bool
	}{
		{"Firewall", cfg.Security.Firewall.Enabled},
		{"SSH Hardening", cfg.Security.SSH.Enabled},
		{"TLS Certificates", cfg.Security.TLS.Enabled},
		{"OS Updates", cfg.Security.OSUpdates.Enabled},
		{"File Integrity", cfg.Security.FileIntegrity.Enabled},
		{"Port Scanning", cfg.Security.PortScan.Enabled},
		{"Compliance", cfg.Security.Compliance.Enabled},
	}

	fmt.Println("║  Modules:                                                       ║")
	for _, m := range modules {
		status := "✅ enabled"
		if !m.enabled {
			status = "⚪ disabled"
		}
		fmt.Printf("║    %-20s %s                              ║\n", m.name, status)
	}

	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Baseline: %-50s ║\n", cfg.Security.Compliance.Baseline)
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")

	return nil
}

