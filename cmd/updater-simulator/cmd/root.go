package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/updatersim"
)

type options struct {
	configPath string
	verbose    bool
	version    string
	gitCommit  string
	buildTime  string
}

// Execute runs the updater-simulator CLI.
func Execute(version, gitCommit, buildTime string) error {
	opts := &options{
		version:   version,
		gitCommit: gitCommit,
		buildTime: buildTime,
	}
	return newRootCommand(opts).Execute()
}

func newRootCommand(opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:           "updater-simulator",
		Short:         "Safely simulate an Updates Server updater agent",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVarP(
		&opts.configPath,
		"config",
		"c",
		"updater-simulator.yaml",
		"Path to simulator YAML configuration",
	)
	root.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "Enable debug logging")

	root.AddCommand(newVersionCommand(opts))
	root.AddCommand(newEnrollCommand(opts))
	root.AddCommand(newHeartbeatCommand(opts))
	root.AddCommand(newCheckCommand(opts))
	root.AddCommand(newOnceCommand(opts))
	root.AddCommand(newRunCommand(opts))
	return root
}

func newVersionCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("updater-simulator %s\n", opts.version)
			fmt.Printf("  Git Commit: %s\n", opts.gitCommit)
			fmt.Printf("  Build Time: %s\n", opts.buildTime)
		},
	}
}

func newEnrollCommand(opts *options) *cobra.Command {
	var confirmed bool
	command := &cobra.Command{
		Use:   "enroll",
		Short: "Activate a dedicated test license and save device credentials",
		Long: `Enroll mutates Updates Server state, binds the license to machine_id,
and may rotate an existing instance API key. Use only a dedicated test license.`,
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !confirmed {
				return fmt.Errorf("enroll requires --confirm-license-binding")
			}

			cfg, err := updatersim.LoadConfig(opts.configPath)
			if err != nil {
				return err
			}
			if cfg.Server.LicenseKey == "" {
				return fmt.Errorf(
					"license key is required; set UPDATER_SIM_LICENSE_KEY or server.license_key",
				)
			}
			if cfg.Instance.MachineID == "" {
				return fmt.Errorf("instance.machine_id is required for enrollment")
			}

			client, err := updatersim.NewClient(cfg.Server)
			if err != nil {
				return err
			}
			response, err := client.ActivateLicense(command.Context(), platformtypes.LicenseActivationRequest{
				LicenseKey: cfg.Server.LicenseKey,
				Hostname:   cfg.Instance.Hostname,
				MachineID:  cfg.Instance.MachineID,
			})
			if err != nil {
				return err
			}
			if response.Instance == nil || response.Instance.Name == "" || response.Instance.APIKey == "" {
				return fmt.Errorf("activation response did not contain instance credentials")
			}

			state, err := updatersim.LoadState(cfg.Simulation.StateFile)
			if err != nil {
				return err
			}
			state.InstanceID = response.Instance.Name
			state.APIKey = response.Instance.APIKey
			if err := updatersim.SaveState(cfg.Simulation.StateFile, state); err != nil {
				return err
			}

			logger(opts).Info(
				"enrollment completed",
				"instance_id", response.Instance.Name,
				"state_file", cfg.Simulation.StateFile,
			)
			return nil
		},
	}
	command.Flags().BoolVar(
		&confirmed,
		"confirm-license-binding",
		false,
		"Confirm that the dedicated test license may be bound or rotated",
	)
	return command
}

func newHeartbeatCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "heartbeat",
		Short: "Send one heartbeat and display safe update hints",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			simulator, _, err := loadSimulator(opts)
			if err != nil {
				return err
			}
			_, err = simulator.SendHeartbeat(command.Context())
			return err
		},
	}
}

func newCheckCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "check [product]",
		Short: "Check group-aware update policy without downloading",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			simulator, cfg, err := loadSimulator(opts)
			if err != nil {
				return err
			}

			products := cfg.Products
			if len(args) == 1 {
				product, ok := cfg.Product(args[0])
				if !ok {
					return fmt.Errorf("product %q is not configured", args[0])
				}
				products = []updatersim.ProductConfig{*product}
			}
			for _, product := range products {
				offer, err := simulator.Check(command.Context(), product.Name)
				if err != nil {
					return err
				}
				logger(opts).Info(
					"update policy checked",
					"product", product.Name,
					"current_version", product.CurrentVersion,
					"target_version", offer.LatestVersion,
					"update_available", offer.UpdateAvailable,
					"update_group", offer.UpdateGroup,
					"source", offer.Source,
				)
			}
			return nil
		},
	}
}

func newOnceCommand(opts *options) *cobra.Command {
	var download bool
	var simulate bool
	command := &cobra.Command{
		Use:   "once",
		Short: "Run one heartbeat and update-policy cycle",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			simulator, cfg, err := loadSimulator(opts)
			if err != nil {
				return err
			}
			mode, err := selectedMode(cfg.Simulation.Mode, download, simulate)
			if err != nil {
				return err
			}
			return simulator.RunCycle(command.Context(), mode)
		},
	}
	addModeFlags(command, &download, &simulate)
	return command
}

func newRunCommand(opts *options) *cobra.Command {
	var download bool
	var simulate bool
	command := &cobra.Command{
		Use:   "run",
		Short: "Run continuous jittered simulator cycles",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			simulator, cfg, err := loadSimulator(opts)
			if err != nil {
				return err
			}
			mode, err := selectedMode(cfg.Simulation.Mode, download, simulate)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(
				context.Background(),
				os.Interrupt,
				syscall.SIGTERM,
			)
			defer stop()
			logger(opts).Info(
				"simulator started",
				"instance_id", cfg.Instance.ID,
				"mode", mode,
				"interval", cfg.Heartbeat.Interval.String(),
			)
			return simulator.Run(ctx, mode)
		},
	}
	addModeFlags(command, &download, &simulate)
	return command
}

func addModeFlags(command *cobra.Command, download, simulate *bool) {
	command.Flags().BoolVar(
		download,
		"download",
		false,
		"Download and verify offered artifacts without reporting success",
	)
	command.Flags().BoolVar(
		simulate,
		"simulate",
		false,
		"Run the no-op executor, report success, and advance simulator state",
	)
}

func selectedMode(configured updatersim.Mode, download, simulate bool) (updatersim.Mode, error) {
	if download && simulate {
		return "", fmt.Errorf("--download and --simulate are mutually exclusive")
	}
	if download {
		return updatersim.ModeDownload, nil
	}
	if simulate {
		return updatersim.ModeSimulate, nil
	}
	return configured, nil
}

func loadSimulator(opts *options) (*updatersim.Simulator, *updatersim.Config, error) {
	cfg, err := updatersim.LoadConfig(opts.configPath)
	if err != nil {
		return nil, nil, err
	}
	simulator, err := updatersim.NewSimulator(cfg, updatersim.NoopExecutor{}, logger(opts))
	if err != nil {
		return nil, nil, err
	}
	return simulator, cfg, nil
}

func logger(opts *options) *slog.Logger {
	level := slog.LevelInfo
	if opts.verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
