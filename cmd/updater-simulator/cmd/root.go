package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	root.AddCommand(newPreflightCommand(opts))
	root.AddCommand(newEnrollCommand(opts))
	root.AddCommand(newHeartbeatCommand(opts))
	root.AddCommand(newCheckCommand(opts))
	root.AddCommand(newOnceCommand(opts))
	root.AddCommand(newReconcileCommand(opts))
	root.AddCommand(newRunCommand(opts))
	root.AddCommand(newRelayCommand(opts))
	root.AddCommand(newLoadgenCommand(opts))
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

// newPreflightCommand implements the read-only boot check a staged self-update
// candidate must pass before activation. It loads and validates the host's real
// config and state and prints the binary version, touching nothing. If the new
// binary cannot parse this host's config/state it exits non-zero here, before
// any symlink swap — the crash-loop a bare `version` check would miss.
func newPreflightCommand(opts *options) *cobra.Command {
	var statePath string
	command := &cobra.Command{
		Use:   "preflight",
		Short: "Read-only boot check: load and validate config and state, print version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := updatersim.LoadConfig(opts.configPath)
			if err != nil {
				return fmt.Errorf("preflight: config: %w", err)
			}
			sp := statePath
			if sp == "" {
				sp = cfg.Simulation.StateFile
			}
			if _, err := updatersim.LoadState(sp); err != nil {
				return fmt.Errorf("preflight: state: %w", err)
			}
			fmt.Printf("updater-simulator preflight ok %s\n", opts.version)
			return nil
		},
	}
	command.Flags().StringVar(&statePath, "state", "", "Path to the state file (default: simulation.state_file from config)")
	return command
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
			return finishOnRestart(logger(opts), simulator.RunCycle(command.Context(), mode))
		},
	}
	addModeFlags(command, &download, &simulate)
	return command
}

func newReconcileCommand(opts *options) *cobra.Command {
	var download bool
	var simulate bool
	command := &cobra.Command{
		Use:   "reconcile",
		Short: "Reconcile the machine toward a system-template.json manifest",
		Long: `Reconcile loads the desired-state manifest configured by
simulation.manifest_file and drives the staged, health-gated pipeline (prepare,
migrate, containers, config, self-update, health, security). All work is
delegated to the safe no-op executor unless a product executor is supplied.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			simulator, cfg, err := loadSimulator(opts)
			if err != nil {
				return err
			}
			if cfg.Simulation.ManifestFile == "" {
				return fmt.Errorf("simulation.manifest_file is required for reconcile")
			}
			manifest, err := updatersim.LoadManifest(cfg.Simulation.ManifestFile)
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
				"reconcile started",
				"instance_id", cfg.Instance.ID,
				"manifest", cfg.Simulation.ManifestFile,
				"mode", mode,
			)
			return simulator.RunReconcile(ctx, mode, manifest)
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
			log := logger(opts)
			log.Info(
				"simulator started",
				"instance_id", cfg.Instance.ID,
				"mode", mode,
				"interval", cfg.Heartbeat.Interval.String(),
			)
			// A leaf has no listener to bind; startup is complete once the loop is
			// about to run. Inert when not under a systemd notify unit.
			simulator.NotifyReady()

			// Graceful shutdown: on a signal, stop starting new update work and
			// give any in-flight cycle a bounded window to drain before forcing
			// exit so a stuck stage cannot hang shutdown indefinitely.
			runErr := make(chan error, 1)
			go func() {
				runErr <- simulator.Run(ctx, mode)
			}()

			select {
			case err := <-runErr:
				return finishOnRestart(log, err)
			case <-ctx.Done():
				// Tell systemd we are stopping so TimeoutStopSec is measured from
				// the drain, not from the signal.
				simulator.NotifyStopping()
				drain := cfg.Simulation.DrainTimeout.Duration
				log.Info(
					"shutdown signal received; pausing updates and draining",
					"drain_timeout", drain.String(),
				)
				select {
				case err := <-runErr:
					log.Info("simulator drained cleanly")
					return finishOnRestart(log, err)
				case <-time.After(drain):
					log.Warn("drain timeout exceeded; forcing shutdown", "drain_timeout", drain.String())
					return nil
				}
			}
		},
	}
	addModeFlags(command, &download, &simulate)
	return command
}

func newRelayCommand(opts *options) *cobra.Command {
	var download bool
	var simulate bool
	command := &cobra.Command{
		Use:   "relay",
		Short: "Run as a cascade relay: normal updater plus a child-facing listener",
		Long: `Relay mode keeps this node a normal updater toward its parent (heartbeats,
update checks, installs) and additionally serves the updater API subset to its
own children: their heartbeats are aggregated into this node's upward rollup,
their update checks are forwarded with this node's credential, and artifacts
are served from a signature-verified pull-through cache. Requires
relay.enabled: true (and relay.listen) in the configuration.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			simulator, cfg, err := loadSimulator(opts)
			if err != nil {
				return err
			}
			if !cfg.Relay.Enabled {
				return fmt.Errorf("relay.enabled must be true in %s", opts.configPath)
			}
			mode, err := selectedMode(cfg.Simulation.Mode, download, simulate)
			if err != nil {
				return err
			}

			upstream, err := updatersim.NewClient(cfg.Server)
			if err != nil {
				return err
			}
			relay, err := updatersim.NewRelay(cfg, upstream, logger(opts))
			if err != nil {
				return err
			}
			// Steady-state reporting is either the legacy full rollup or the
			// change-only delta stream — one path, never both, so the server
			// never double-counts a fleet that is transitioning.
			if cfg.Relay.DeltaReporting {
				simulator.SetDeltaProvider(relay.DeltaEnvelope, relay.AckUpstream)
			} else {
				simulator.SetChildrenProvider(relay.ChildrenReport)
			}
			simulator.SetGuardStatsProvider(relay.GuardStats)
			// The relay's own health feeds the node's survivability signals: a
			// dead child-facing listener must lapse the systemd watchdog
			// (restart, not keep-alive), and a parent that rejects the relay's
			// credential must surface cascade-wide in the upward heartbeat.
			simulator.SetRelayListenerProbe(relay.ListenerHealthy)
			simulator.SetParentCredentialProbe(relay.ParentCredentialRejected)

			ctx, stop := signal.NotifyContext(
				context.Background(),
				os.Interrupt,
				syscall.SIGTERM,
			)
			defer stop()
			log := logger(opts)
			log.Info(
				"relay updater started",
				"instance_id", cfg.Instance.ID,
				"tier", cfg.Instance.ProductTier,
				"listen", cfg.Relay.Listen,
				"parent", cfg.Server.URL,
				"mode", mode,
			)

			serveErr := make(chan error, 1)
			go func() { serveErr <- relay.Serve(ctx) }()

			// Signal systemd readiness only after the child-facing listener is
			// actually accepting connections, so a relay that fails to bind is not
			// reported as started. Bounded poll; inert when not under a notify unit.
			go func() {
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					if relay.ListenerHealthy() {
						simulator.NotifyReady()
						return
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(200 * time.Millisecond):
					}
				}
				// Bind did not confirm in time; still signal so a slow-but-live
				// node is not killed by the notify unit's TimeoutStartSec.
				simulator.NotifyReady()
			}()

			runErr := make(chan error, 1)
			go func() { runErr <- simulator.Run(ctx, mode) }()

			select {
			case err := <-serveErr:
				stop()
				<-runErr
				return err
			case err := <-runErr:
				stop()
				<-serveErr
				return finishOnRestart(log, err)
			case <-ctx.Done():
				// Graceful shutdown: tell systemd we are stopping (so
				// TimeoutStopSec is measured from here) and give the serve loop and
				// in-flight cycle a bounded window to drain before forcing exit, so
				// a stuck stage or a child connection that will not close cannot
				// hang shutdown indefinitely.
				simulator.NotifyStopping()
				drain := cfg.Simulation.DrainTimeout.Duration
				log.Info(
					"shutdown signal received; pausing updates and draining",
					"drain_timeout", drain.String(),
				)
				timer := time.NewTimer(drain)
				defer timer.Stop()
				drained := 0
				for drained < 2 {
					select {
					case <-serveErr:
						drained++
					case <-runErr:
						drained++
					case <-timer.C:
						log.Warn("drain timeout exceeded; forcing shutdown", "drain_timeout", drain.String())
						return nil
					}
				}
				log.Info("relay drained cleanly")
				return nil
			}
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
		"real",
		false,
		"Run the full pipeline: verify, download, run the configured executor, report",
	)
	command.Flags().BoolVar(
		simulate,
		"simulate",
		false,
		"Deprecated alias of --real (the retired name could silently report fake success)",
	)
	_ = command.Flags().MarkDeprecated("simulate", "use --real")
}

func selectedMode(configured updatersim.Mode, download, real bool) (updatersim.Mode, error) {
	if download && real {
		return "", fmt.Errorf("--download and --real are mutually exclusive")
	}
	if download {
		return updatersim.ModeDownload, nil
	}
	if real {
		return updatersim.ModeReal, nil
	}
	return configured, nil
}

func loadSimulator(opts *options) (*updatersim.Simulator, *updatersim.Config, error) {
	cfg, err := updatersim.LoadConfig(opts.configPath)
	if err != nil {
		return nil, nil, err
	}
	executor := buildExecutor(cfg, logger(opts))
	simulator, err := updatersim.NewSimulator(cfg, executor, logger(opts))
	if err != nil {
		return nil, nil, err
	}
	// The stamped binary version anchors self-update comparisons and settles
	// any pending self-update marker. ResolveSelfUpdate now only handles the
	// watchdog path (wrong binary -> restore); the happy path is finalized by
	// the first successful heartbeat, so a binary that boots but cannot reach
	// its parent is never declared good.
	simulator.SetBinaryVersion(opts.version)
	simulator.ResolveSelfUpdate()
	// NOTE: systemd readiness (NotifyReady) is signaled by the long-running
	// commands (run, relay) themselves — for relay, only after the child-facing
	// listener is actually accepting connections — not here.
	return simulator, cfg, nil
}

// finishOnRestart converts ErrRestartPending into a clean exit so the service
// manager (systemd Restart=always) relaunches the freshly activated updater.
func finishOnRestart(log *slog.Logger, err error) error {
	if errors.Is(err, updatersim.ErrRestartPending) {
		log.Info("self-update activated; exiting so the service manager restarts the new updater")
		return nil
	}
	return err
}

// buildExecutor selects the Executor implementation from configuration. The
// default no-op executor never touches the machine; the filesystem executor
// performs a real versioned install with an atomic activation and rollback.
func buildExecutor(cfg *updatersim.Config, log *slog.Logger) updatersim.Executor {
	switch cfg.Simulation.Executor {
	case updatersim.ExecutorFilesystem:
		log.Info(
			"using filesystem executor",
			"install_root", cfg.Simulation.Filesystem.InstallRoot,
		)
		return updatersim.NewFilesystemExecutor(cfg.Simulation.Filesystem, log)
	default:
		return updatersim.NoopExecutor{}
	}
}

func logger(opts *options) *slog.Logger {
	level := slog.LevelInfo
	if opts.verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
