package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
)

var serviceConfigPath string

var ServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage services",
	Long:  `Manage services for managed products.`,
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all managed services",
	RunE:  runServiceList,
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart <service>",
	Short: "Restart a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceRestart,
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop <service>",
	Short: "Stop a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceStop,
}

var serviceStartCmd = &cobra.Command{
	Use:   "start <service>",
	Short: "Start a service",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceStart,
}

var serviceLogsCmd = &cobra.Command{
	Use:   "logs <service>",
	Short: "View service logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceLogs,
}

func init() {
	ServiceCmd.PersistentFlags().StringVarP(&serviceConfigPath, "config", "c", "", "Path to config file")
	
	ServiceCmd.AddCommand(serviceListCmd)
	ServiceCmd.AddCommand(serviceRestartCmd)
	ServiceCmd.AddCommand(serviceStopCmd)
	ServiceCmd.AddCommand(serviceStartCmd)
	ServiceCmd.AddCommand(serviceLogsCmd)
}

func loadServiceConfig() (*config.Config, error) {
	configPath := serviceConfigPath
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

func runServiceList(cmd *cobra.Command, args []string) error {
	cfg, err := loadServiceConfig()
	if err != nil {
		return err
	}

	fmt.Println("Managed Services:")
	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Printf("%-25s %-15s %-15s\n", "SERVICE", "STATUS", "PRODUCT")
	fmt.Println("─────────────────────────────────────────────────────")

	for _, product := range cfg.Products {
		status := getServiceStatusSimple(product.Service)
		fmt.Printf("%-25s %-15s %-15s\n", product.Service, status, product.Name)
	}

	// Add updater service
	updaterStatus := getServiceStatusSimple("mysoc-updater.service")
	fmt.Printf("%-25s %-15s %-15s\n", "mysoc-updater.service", updaterStatus, "(updater)")

	return nil
}

func runServiceRestart(cmd *cobra.Command, args []string) error {
	serviceName := normalizeServiceName(args[0])

	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	fmt.Printf("Restarting %s...\n", serviceName)
	if err := exec.Command("systemctl", "restart", serviceName).Run(); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	fmt.Printf("✓ Service %s restarted\n", serviceName)
	return nil
}

func runServiceStop(cmd *cobra.Command, args []string) error {
	serviceName := normalizeServiceName(args[0])

	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	fmt.Printf("Stopping %s...\n", serviceName)
	if err := exec.Command("systemctl", "stop", serviceName).Run(); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	fmt.Printf("✓ Service %s stopped\n", serviceName)
	return nil
}

func runServiceStart(cmd *cobra.Command, args []string) error {
	serviceName := normalizeServiceName(args[0])

	if os.Getuid() != 0 {
		return fmt.Errorf("this command must be run as root (use sudo)")
	}

	fmt.Printf("Starting %s...\n", serviceName)
	if err := exec.Command("systemctl", "start", serviceName).Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Printf("✓ Service %s started\n", serviceName)
	return nil
}

func runServiceLogs(cmd *cobra.Command, args []string) error {
	serviceName := normalizeServiceName(args[0])

	// Use exec to replace current process with journalctl
	c := exec.Command("journalctl", "-u", serviceName, "-f", "--no-pager")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func normalizeServiceName(name string) string {
	if !strings.HasSuffix(name, ".service") {
		return name + ".service"
	}
	return name
}

func getServiceStatusSimple(serviceName string) string {
	cmd := exec.Command("systemctl", "is-active", serviceName)
	output, err := cmd.Output()
	if err != nil {
		return "stopped"
	}
	return strings.TrimSpace(string(output))
}

