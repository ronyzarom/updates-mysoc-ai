package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/cmd/mysoc-updater/cmd"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "mysoc-updater",
	Short: "MySoc Updater Agent",
	Long: `MySoc Updater Agent - Bootstrap, update, monitor, and secure MySoc/SIEMCore instances.

This agent is responsible for:
  - Installing and updating MySoc/SIEMCore products
  - Monitoring service health and auto-restarting
  - Applying security hardening
  - Reporting status via heartbeat`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(c *cobra.Command, args []string) {
		fmt.Printf("mysoc-updater %s\n", Version)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
		fmt.Printf("  Build Time: %s\n", BuildTime)
	},
}

func init() {
	// Set version info for subcommands
	cmd.Version = Version
	cmd.GitCommit = GitCommit
	cmd.BuildTime = BuildTime

	// Add commands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(cmd.InitCmd)
	rootCmd.AddCommand(cmd.DaemonCmd)
	rootCmd.AddCommand(cmd.StatusCmd)
	rootCmd.AddCommand(cmd.UpdateCmd)
	rootCmd.AddCommand(cmd.RollbackCmd)
	rootCmd.AddCommand(cmd.ServiceCmd)
	rootCmd.AddCommand(cmd.SecurityCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
