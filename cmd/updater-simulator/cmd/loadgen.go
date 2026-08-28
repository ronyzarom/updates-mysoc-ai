package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/updatersim"
)

// newLoadgenCommand adds the Fleet Scalability 1.12 load rig: it synthesizes
// tens of thousands of customer subtrees and drives them at a target as
// delta-reporting heartbeats, printing per-cycle acceptance-gate metrics
// (throughput and p50/p95/p99 latency). It needs no simulator config — the
// target, credential, and fleet shape are flags — so it can point at any
// server or mysoc relay.
func newLoadgenCommand(opts *options) *cobra.Command {
	var o updatersim.LoadOptions
	var licenseEnv string
	command := &cobra.Command{
		Use:   "loadgen",
		Short: "Drive N synthetic customer subtrees at a target to measure ingest at scale",
		Long: `Loadgen is the Fleet Scalability 1.12 acceptance rig. It synthesizes
--customers customer subtrees (one siemcore relay + --leaves swf leaves each)
and posts them to --target as delta-reporting heartbeats. Cycle 1 enrolls the
whole fleet (full inventory per customer); later cycles carry only --churn of
each customer's leaves, the O(changes) steady state. Each cycle prints
throughput and latency percentiles so acceptance gates (server heartbeat p99
under 1s at 20k customers) can be verified.

Example — 20k customers, 5 leaves each (120k nodes), against a local server:

  updater-simulator loadgen --target https://127.0.0.1:8443 \
    --customers 20000 --leaves 5 --cycles 3 --concurrency 128 \
    --parent mysoc-op1 --insecure --license-env UPDATER_SIM_LICENSE_KEY`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if o.TargetURL == "" {
				return fmt.Errorf("--target is required")
			}
			if o.LicenseKey == "" && licenseEnv != "" {
				o.LicenseKey = os.Getenv(licenseEnv)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			log := logger(opts)
			log.Info("loadgen starting",
				"target", o.TargetURL,
				"customers", o.Customers,
				"leaves_per_customer", o.LeavesPerCustomer,
				"cycles", o.Cycles,
				"concurrency", o.Concurrency,
			)

			var worstP99 time.Duration
			err := updatersim.RunLoad(ctx, o, func(rep updatersim.CycleReport) {
				fmt.Println(rep.String())
				if rep.P99 > worstP99 {
					worstP99 = rep.P99
				}
			})
			fmt.Printf("worst-cycle p99: %s\n", worstP99.Round(time.Millisecond))
			return err
		},
	}
	f := command.Flags()
	f.StringVar(&o.TargetURL, "target", "", "Base URL of the server or mysoc relay (required)")
	f.StringVar(&o.LicenseKey, "license", "", "License key sent as X-License-Key")
	f.StringVar(&licenseEnv, "license-env", "UPDATER_SIM_LICENSE_KEY", "Environment variable to read the license key from when --license is empty")
	f.IntVar(&o.Customers, "customers", 20000, "Number of synthetic customer subtrees")
	f.IntVar(&o.LeavesPerCustomer, "leaves", 5, "swf leaves per customer subtree")
	f.IntVar(&o.Cycles, "cycles", 3, "Heartbeat cycles to drive (cycle 1 enrolls the fleet)")
	f.IntVar(&o.Concurrency, "concurrency", 128, "Maximum simultaneous in-flight heartbeats")
	f.IntVar(&o.ChurnPercent, "churn", 2, "Percent of each customer's leaves that change per steady cycle")
	f.StringVar(&o.ParentInstanceID, "parent", "", "mysoc relay instance id the synthetic customers report to")
	f.BoolVar(&o.Insecure, "insecure", false, "Skip TLS verification (self-signed relay certs)")
	f.DurationVar(&o.Timeout, "timeout", 30*time.Second, "Per-heartbeat request timeout")
	return command
}
