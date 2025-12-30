package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/heartbeat"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/service"
	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/update"
)

var daemonConfigPath string

var DaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the updater as a daemon",
	Long: `Run the updater as a background daemon.

The daemon will:
  - Send regular heartbeats to the update server
  - Check for updates and apply them
  - Monitor service health and restart crashed services
  - Apply security hardening and report status`,
	RunE: runDaemon,
}

func init() {
	DaemonCmd.Flags().StringVarP(&daemonConfigPath, "config", "c", "", "Path to config file")
}

func runDaemon(cmd *cobra.Command, args []string) error {
	fmt.Printf("MySoc Updater Daemon v%s starting...\n", Version)

	// Find config file
	configPath := daemonConfigPath
	if configPath == "" {
		// Try default paths
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

	fmt.Printf("Loaded config from %s\n", configPath)
	fmt.Printf("Instance: %s (%s)\n", cfg.Instance.ID, cfg.Instance.Type)
	fmt.Printf("Server: %s\n", cfg.Server.URL)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start heartbeat reporter
	heartbeatReporter := heartbeat.NewReporter(cfg)
	go heartbeatReporter.Start(ctx)
	fmt.Println("Heartbeat reporter started")

	// Start update checker
	updateChecker := update.NewChecker(cfg)
	go updateChecker.Start(ctx)
	fmt.Println("Update checker started")

	// Start service monitor
	serviceMonitor := service.NewMonitor(cfg)
	go serviceMonitor.Start(ctx)
	fmt.Println("Service monitor started")

	fmt.Println("Daemon running. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	sig := <-sigChan
	fmt.Printf("\nReceived signal %v, shutting down...\n", sig)

	// Cancel context to stop all goroutines
	cancel()

	// Give goroutines time to clean up
	time.Sleep(2 * time.Second)

	fmt.Println("Daemon stopped")
	return nil
}

