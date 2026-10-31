package updatersim

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/artifactprotocol"
	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var ErrCycleInProgress = errors.New("simulator cycle already in progress")

// Simulator runs safe updater protocol cycles.
type Simulator struct {
	config *Config
	client *Client
	// fallbackClient targets server.fallback_url and is used ONLY for the
	// self-update check/download, ONLY after the parent fails. nil when no
	// fallback is configured. It never carries the relay token (that is
	// parent-specific); it authenticates to the origin with the node's own
	// credential, which only tier-1 nodes hold.
	fallbackClient *Client
	executor       Executor
	logger         *slog.Logger
	state          *State
	started        time.Time
	cycleMu        sync.Mutex
	random         *rand.Rand
	publicKey      ed25519.PublicKey // primary release-signing key; nil disables verification
	// publicKeys is the full set of trusted release-signing keys
	// (signing.public_key plus signing.public_keys). A release signature valid
	// under any of them is accepted — the key-rollover path, so a signing-key
	// rotation never strands a node that cannot verify its own next self-update.
	publicKeys []ed25519.PublicKey

	// binaryVersion is the ldflags-stamped version of the running executable,
	// set via SetBinaryVersion. It anchors self-update comparisons; empty for
	// unstamped dev builds, which never self-update.
	binaryVersion string

	// childrenFn supplies the cascade rollup for relay-mode heartbeats.
	childrenFn func() []platformtypes.ChildReport

	// relayListenerFn reports whether the relay's child-facing listener passed
	// its health self-probe this cycle. Only set in relay mode.
	relayListenerFn func() bool
	// cycleListenerOK caches this cycle's relay listener probe (nil for
	// non-relays), so the watchdog gate and the heartbeat agree and the probe
	// runs once per cycle.
	cycleListenerOK *bool

	// parentRejectedCredential records that the parent rejected this node's
	// credential (401/403) during the current cycle, distinct from "parent
	// unreachable". Reset at the start of each cycle; read by buildHeartbeat.
	parentRejectedCredential bool

	// relayCredProbe reports whether the relay last had its own credential
	// rejected by the parent while forwarding a child check. Only set in relay
	// mode; ORed into the heartbeat's parent_rejected_credential signal.
	relayCredProbe func() bool

	// guardStatsFn supplies the relay listener's port-protection counters.
	guardStatsFn func() *platformtypes.RelayGuardStats

	// deltaFn drains the relay's change-only upward batch (Fleet Scalability
	// 1.12); deltaAckFn prunes that queue once the parent acks the cursor.
	deltaFn    func() *platformtypes.DeltaEnvelope
	deltaAckFn func(uint64)
}

// SetChildrenProvider wires the relay's rollup into this node's heartbeats.
func (s *Simulator) SetChildrenProvider(provider func() []platformtypes.ChildReport) {
	s.childrenFn = provider
}

// SetGuardStatsProvider wires the relay guard's counters into heartbeats.
func (s *Simulator) SetGuardStatsProvider(provider func() *platformtypes.RelayGuardStats) {
	s.guardStatsFn = provider
}

// SetRelayListenerProbe wires the relay's child-facing listener health check
// into heartbeats, so a live process with a dead listener is visible upstream.
func (s *Simulator) SetRelayListenerProbe(probe func() bool) {
	s.relayListenerFn = probe
}

// SetParentCredentialProbe wires the relay's observation that the parent
// rejected this node's own credential (while forwarding a child check) into the
// heartbeat, so an expired or rotated license surfaces cascade-wide.
func (s *Simulator) SetParentCredentialProbe(probe func() bool) {
	s.relayCredProbe = probe
}

// SetDeltaProvider wires the relay's change-only delta stream into this node's
// upward heartbeats, plus the ack callback that prunes the queue.
func (s *Simulator) SetDeltaProvider(
	drain func() *platformtypes.DeltaEnvelope,
	ack func(uint64),
) {
	s.deltaFn = drain
	s.deltaAckFn = ack
}

// NewSimulator loads durable state and constructs a simulator.
func NewSimulator(
	cfg *Config,
	executor Executor,
	logger *slog.Logger,
) (*Simulator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("simulator config is required")
	}
	if executor == nil {
		executor = NoopExecutor{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Simulation.LegacySimulateMode {
		logger.Warn(`config uses the retired mode "simulate"; running as mode "real" — update the config (simulate is a deprecated alias and will be removed)`)
	}

	state, err := LoadState(cfg.Simulation.StateFile)
	if err != nil {
		return nil, err
	}
	ApplyState(cfg, state)
	if cfg.Instance.ID == "" {
		return nil, fmt.Errorf("instance id is required; configure one or run enroll")
	}

	client, err := NewClient(cfg.Server)
	if err != nil {
		return nil, err
	}
	if state.RelayToken != "" {
		client.SetRelayToken(state.RelayToken)
	}

	var publicKeys []ed25519.PublicKey
	for _, pubHex := range cfg.TrustedSigningKeys() {
		key, err := signing.ParsePublicKeyHex(pubHex)
		if err != nil {
			return nil, fmt.Errorf("signing key %q: %w", pubHex, err)
		}
		publicKeys = append(publicKeys, key)
	}
	var publicKey ed25519.PublicKey
	if len(publicKeys) > 0 {
		publicKey = publicKeys[0]
	}

	sim := &Simulator{
		config:     cfg,
		client:     client,
		executor:   executor,
		logger:     logger,
		state:      state,
		started:    time.Now(),
		random:     rand.New(rand.NewSource(time.Now().UnixNano())),
		publicKey:  publicKey,
		publicKeys: publicKeys,
	}
	if fb := strings.TrimSpace(cfg.Server.FallbackURL); fb != "" {
		fbCfg := cfg.Server
		fbCfg.URL = fb
		// The fallback is the origin; it uses system roots, not the relay's
		// pinned CA, and never the parent-issued relay token.
		fbCfg.CAFile = ""
		fallbackClient, err := NewClient(fbCfg)
		if err != nil {
			return nil, fmt.Errorf("server.fallback_url client: %w", err)
		}
		sim.fallbackClient = fallbackClient
		if cfg.Relay.Enabled {
			logger.Warn(
				"server.fallback_url is set on a relay node; it only helps nodes that hold an origin credential — a relay's children reach the origin via the relay, not a fallback URL",
				"fallback_url", fb,
			)
		}
	}
	return sim, nil
}

// SendHeartbeat sends one heartbeat and returns server update hints.
func (s *Simulator) SendHeartbeat(ctx context.Context) (*HeartbeatResponse, error) {
	response, err := s.client.SendHeartbeat(ctx, s.buildHeartbeat())
	if err != nil {
		return nil, fmt.Errorf("send heartbeat: %w", err)
	}
	// Only prune the delta queue once the parent confirms it durably ingested
	// the batch; a failed send above leaves everything queued for retry.
	if s.deltaAckFn != nil && response.AckCursor > 0 {
		s.deltaAckFn(response.AckCursor)
	}
	if response.RelayToken != "" && response.RelayToken != s.state.RelayToken {
		// A relay parent issued (or rotated) this node's token; persist it so
		// restarts keep the same identity binding.
		s.state.RelayToken = response.RelayToken
		s.client.SetRelayToken(response.RelayToken)
		if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
			s.logger.Warn("failed to persist relay token", "error", err)
		}
	}
	s.logger.Info(
		"heartbeat accepted",
		"instance_id", s.config.Instance.ID,
		"update_hints", len(response.Updates),
	)
	for _, update := range response.Updates {
		s.logger.Info(
			"heartbeat update hint",
			"product", update.Product,
			"current_version", update.CurrentVersion,
			"target_version", update.LatestVersion,
		)
	}
	return response, nil
}

// Check checks the group-aware policy endpoint for one configured product.
func (s *Simulator) Check(ctx context.Context, productName string) (*UpdateOffer, error) {
	product, ok := s.config.Product(productName)
	if !ok {
		return nil, fmt.Errorf("product %q is not configured", productName)
	}

	operatingSystem, architecture := s.platform()
	request := UpdateCheckRequest{
		InstanceID:       s.config.Instance.ID,
		CurrentVersion:   product.CurrentVersion,
		UpdaterVersion:   s.config.Instance.UpdaterVersion,
		OS:               operatingSystem,
		Arch:             architecture,
		Hostname:         s.config.Instance.Hostname,
		Channel:          product.Channel,
		ProductTier:      s.config.Instance.ProductTier,
		ParentInstanceID: s.config.Instance.ParentID,
	}
	if len(product.PrerequisiteVerifier) > 0 {
		request.ProtocolVersion = artifactprotocol.Version
		request.Capabilities = []string{artifactprotocol.Capability, artifactprotocol.IndependentCapability}
		request.Lifecycle = "empty"
		evidence, probeErr := s.prerequisiteEvidence(ctx, product.Name)
		if probeErr != nil {
			return nil, fmt.Errorf("prerequisite inspection: %w", probeErr)
		}
		request.Lifecycle = evidence.Lifecycle
		request.InstalledVersion = evidence.InstalledVersion
		request.CachedDependencies = evidence.Dependencies
	}
	offer, err := s.client.CheckUpdate(ctx, product.Name, request)
	if err == nil {
		return offer, nil
	}

	var apiErr *APIError
	if !s.config.Simulation.LegacyFallback ||
		!errors.As(err, &apiErr) ||
		(apiErr.StatusCode != 404 && apiErr.StatusCode != 405) {
		return nil, fmt.Errorf("check update policy for %s: %w", product.Name, err)
	}

	s.logger.Warn(
		"policy endpoint unavailable; using group-unaware legacy check",
		"product", product.Name,
		"status", apiErr.StatusCode,
	)
	offer, err = s.client.CheckLatest(
		ctx,
		product.Name,
		product.Channel,
		product.CurrentVersion,
	)
	if err == nil {
		return offer, nil
	}
	if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
		return &UpdateOffer{
			Product:         product.Name,
			CurrentVersion:  product.CurrentVersion,
			UpdateAvailable: false,
			Channel:         product.Channel,
			Source:          "legacy",
		}, nil
	}
	return nil, fmt.Errorf("legacy update check for %s: %w", product.Name, err)
}

// RunCycle sends a heartbeat and evaluates every configured product.
func (s *Simulator) RunCycle(ctx context.Context, mode Mode) error {
	if !s.cycleMu.TryLock() {
		return ErrCycleInProgress
	}
	defer s.cycleMu.Unlock()

	if mode == "" {
		mode = s.config.Simulation.Mode
	}
	switch mode {
	case ModeObserve, ModeDownload, ModeReal:
	default:
		return fmt.Errorf("invalid cycle mode %q", mode)
	}

	// Whole-cycle deadline: a stuck cycle self-cancels here so the next tick is
	// never starved (ErrCycleInProgress forever was a confirmed deadlock).
	ctx, cancelCycle := context.WithTimeout(ctx, s.config.cycleTimeout())
	defer cancelCycle()

	// Fresh credential-rejection signal per cycle.
	s.parentRejectedCredential = false

	// Probe the relay listener once this cycle (nil for non-relays) for the
	// heartbeat's relay_listener_ok. The systemd watchdog is fed by a separate
	// background pinger (startWatchdog), not here, so a long-but-progressing
	// cycle (e.g. a fresh-host first_install) never trips WatchdogSec while a
	// dead relay listener still lapses it.
	s.cycleListenerOK = nil
	if s.relayListenerFn != nil {
		ok := s.relayListenerFn()
		s.cycleListenerOK = &ok
	}

	var cycleErrors []error

	// 1. Self-update FIRST, in isolation, with its own deadline. Its outcome
	//    never depends on heartbeat or product results, so a rejected heartbeat
	//    or a wedged product hook can never stop the updater from fetching the
	//    release that fixes the host. A successful activation returns
	//    ErrRestartPending immediately so the process can hand off.
	if err := s.selfUpdatePhase(ctx, mode); err != nil {
		if errors.Is(err, ErrRestartPending) {
			return err
		}
		cycleErrors = append(cycleErrors, err)
	}

	// 2. Heartbeat. A heartbeat error is recorded but NEVER fatal: a
	//    server-side rejection must not block product work or (above)
	//    self-update.
	if _, err := s.SendHeartbeat(ctx); err != nil {
		s.noteParentAuth(err)
		cycleErrors = append(cycleErrors, fmt.Errorf("heartbeat: %w", err))
	}

	// 3. Products, each bounded by its own deadline so one hung hook cannot
	//    consume the whole cycle budget.
	for _, configuredProduct := range s.config.Products {
		if err := s.runProductCycle(ctx, mode, configuredProduct.Name); err != nil {
			cycleErrors = append(cycleErrors, err)
		}
	}
	return errors.Join(cycleErrors...)
}

// runProductCycle checks and (in non-observe modes) applies one product's
// offer, bounded by the per-product deadline.
func (s *Simulator) runProductCycle(ctx context.Context, mode Mode, productName string) error {
	ctx, cancel := context.WithTimeout(ctx, s.config.productTimeout())
	defer cancel()

	offer, err := s.Check(ctx, productName)
	if err != nil {
		return err
	}
	if !offer.UpdateAvailable {
		current := ""
		if p, ok := s.config.Product(productName); ok {
			current = p.CurrentVersion
		}
		s.logger.Info(
			"product is up to date",
			"product", productName,
			"current_version", current,
			"source", offer.Source,
		)
		return nil
	}
	return s.processOffer(ctx, mode, offer)
}

// selfUpdatePhase runs the self-update check/apply with its own deadline, and
// (for nodes that hold an origin credential) falls back to server.fallback_url
// if the parent is unreachable. A successful activation returns
// ErrRestartPending; all other errors are returned for the cycle to record.
func (s *Simulator) selfUpdatePhase(ctx context.Context, mode Mode) error {
	ctx, cancel := context.WithTimeout(ctx, s.config.selfUpdateTimeout())
	defer cancel()

	err := s.maybeSelfUpdate(ctx, mode, s.client)
	if err == nil || errors.Is(err, ErrRestartPending) {
		return err
	}
	s.noteParentAuth(err)
	if s.fallbackClient == nil {
		return err
	}
	// The parent failed; try the origin fallback. Only effective for nodes that
	// hold an origin credential (tier 1); for a relay child this simply fails
	// again, which is fine — the error was already going to be recorded.
	s.logger.Warn(
		"self-update via parent failed; retrying via fallback origin",
		"fallback_url", s.config.Server.FallbackURL,
		"parent_error", err.Error(),
	)
	fbErr := s.maybeSelfUpdate(ctx, mode, s.fallbackClient)
	if fbErr == nil || errors.Is(fbErr, ErrRestartPending) {
		return fbErr
	}
	return errors.Join(err, fmt.Errorf("fallback: %w", fbErr))
}

// startWatchdog launches a background systemd watchdog pinger when launched
// under a notify unit with WatchdogSec. It pings at half the watchdog interval,
// decoupled from cycle duration so a long first_install never trips it. A relay
// pings only while its child-facing listener self-probe passes, so a live
// process with a dead cascade listener is restarted rather than kept alive.
// Inert (no goroutine) when not under a watchdog unit.
func (s *Simulator) startWatchdog(ctx context.Context) {
	if !sdWatchdogEnabled() {
		return
	}
	interval := sdWatchdogInterval()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.relayListenerFn == nil || s.relayListenerFn() {
					sdNotifyWatchdog()
				}
			}
		}
	}()
}

// Run starts an immediate cycle followed by jittered heartbeat cycles. It
// returns ErrRestartPending when a self-update was activated, so the caller
// can exit the process for the service manager to relaunch the new binary.
func (s *Simulator) Run(ctx context.Context, mode Mode) error {
	s.startWatchdog(ctx)
	if err := s.RunCycle(ctx, mode); err != nil && !errors.Is(err, context.Canceled) {
		if errors.Is(err, ErrRestartPending) {
			return err
		}
		s.logger.Error("simulator cycle failed", "error", err)
	}

	for {
		timer := time.NewTimer(s.nextInterval())
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
			if err := s.RunCycle(ctx, mode); err != nil && !errors.Is(err, context.Canceled) {
				if errors.Is(err, ErrRestartPending) {
					return err
				}
				s.logger.Error("simulator cycle failed", "error", err)
			}
		}
	}
}

func (s *Simulator) processOffer(
	ctx context.Context,
	mode Mode,
	offer *UpdateOffer,
) error {
	s.logger.Info(
		"update available",
		"product", offer.Product,
		"current_version", offer.CurrentVersion,
		"target_version", offer.LatestVersion,
		"update_group", offer.UpdateGroup,
		"source", offer.Source,
		"mode", mode,
	)
	if mode == ModeObserve {
		return nil
	}

	// After a first_install failure a fresh host backs off instead of
	// re-provisioning every heartbeat; skip this cycle until the window passes.
	if remaining := s.retryBackoffRemaining(offer.Product); remaining > 0 {
		s.logger.Info(
			"deferring install retry (back-off after a prior failure)",
			"product", offer.Product,
			"retry_in", remaining.Round(time.Second).String(),
		)
		return nil
	}

	// verifyAndDownload enforces the origin release signature before touching
	// the artifact. In the cascade this is what prevents any intermediate hop
	// from substituting a payload: the signature is minted only by the updates
	// server.
	if err := s.verifyDualOffer(ctx, offer); err != nil {
		return s.recordDualFailure(offer, err)
	}
	result, err := s.verifyAndDownload(ctx, offer, s.client)
	if err != nil {
		if offer.SelectedArtifactKind != "" {
			return s.recordDualFailure(offer, err)
		}
		return err
	}
	if mode == ModeDownload {
		return nil
	}

	update := Update{
		Product:              offer.Product,
		FromVersion:          offer.CurrentVersion,
		ToVersion:            offer.LatestVersion,
		Channel:              offer.Channel,
		UpdateGroup:          offer.UpdateGroup,
		ReleaseNotes:         offer.ReleaseNotes,
		ArtifactPath:         result.Path,
		ArtifactSHA256:       result.Checksum,
		ArtifactSignature:    offer.Signature,
		SelectedArtifactKind: offer.SelectedArtifactKind, DependencyValidation: offer.DependencyValidation,
	}

	// In real mode an install must actually happen. With no executor
	// configured, reporting success would be a lie — the silent failure
	// the retired "simulate" mode allowed. Fail loudly instead so the
	// gap is visible on the dashboard and in reports.
	if _, noop := s.executor.(NoopExecutor); noop {
		return s.failAndRollback(ctx, update, fmt.Errorf(
			"no executor configured: refusing to report a successful install that did not happen (set simulation.executor: filesystem)"))
	}

	if err := s.verifyDualOffer(ctx, offer); err != nil {
		return s.recordDualFailure(offer, err)
	}
	// A fresh host (from 0.0.0) runs the artifact's first_install, which
	// provisions the whole host and is idempotent/resumable. A failure there is
	// NOT rolled back (there is nothing to roll back to, and the per-cycle
	// stage/fail/rollback loop is exactly the churn we want to avoid): we clean
	// the staged release, report the error, and back off so a later retry
	// resumes once the admin fixes the host.
	fresh := isFreshInstall(update.FromVersion)
	if err := s.executor.Apply(ctx, update); err != nil {
		// A low-disk refusal is a clean skip, not a failed install: nothing was
		// staged, so there is nothing to roll back. Back off and retry once the
		// host has room, rather than churning stage/fail/rollback every cycle.
		if errors.Is(err, ErrInsufficientDisk) {
			return s.recordSkip(ctx, update, "insufficient_disk", err)
		}
		if fresh {
			return s.recordFirstInstallFailure(ctx, update, fmt.Errorf("first_install: %w", err))
		}
		return s.failAndRollback(ctx, update, fmt.Errorf("apply: %w", err))
	}
	if err := s.executor.Validate(ctx, update); err != nil {
		if fresh {
			return s.recordFirstInstallFailure(ctx, update, fmt.Errorf("first_install validation: %w", err))
		}
		return s.failAndRollback(ctx, update, fmt.Errorf("validation: %w", err))
	}

	s.clearRetryBackoff(update.Product)
	s.recordAttempt(update, true, "")
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		return err
	}
	unavailable, violation := s.availability()
	if err := s.client.ReportUpdate(ctx, update.Product, UpdateReportRequest{
		InstanceID:           s.config.Instance.ID,
		FromVersion:          update.FromVersion,
		ToVersion:            update.ToVersion,
		SelectedArtifactKind: update.SelectedArtifactKind, DependencyValidation: update.DependencyValidation, ArtifactDigest: update.ArtifactSHA256,
		Success:               true,
		UnavailableSeconds:    unavailable,
		AvailabilityViolation: violation,
	}); err != nil {
		return fmt.Errorf("report update: %w", err)
	}

	s.logger.Info(
		"update applied and verified",
		"product", update.Product,
		"from_version", update.FromVersion,
		"to_version", update.ToVersion,
	)
	return nil
}

func (s *Simulator) failAndRollback(
	ctx context.Context,
	update Update,
	updateErr error,
) error {
	rollbackErr := s.executor.Rollback(ctx, update)
	errorMessage := updateErr.Error()
	if rollbackErr != nil {
		errorMessage += "; rollback: " + rollbackErr.Error()
	}

	s.recordAttempt(update, false, errorMessage)
	stateErr := SaveState(s.config.Simulation.StateFile, s.state)
	unavailable, violation := s.availability()
	reportErr := s.client.ReportUpdate(ctx, update.Product, UpdateReportRequest{
		InstanceID:           s.config.Instance.ID,
		FromVersion:          update.FromVersion,
		ToVersion:            update.ToVersion,
		SelectedArtifactKind: update.SelectedArtifactKind, DependencyValidation: update.DependencyValidation, ArtifactDigest: update.ArtifactSHA256,
		Success:               false,
		Error:                 errorMessage,
		UnavailableSeconds:    unavailable,
		AvailabilityViolation: violation,
	})
	return errors.Join(updateErr, rollbackErr, stateErr, reportErr)
}

// availability reads the most recent restart-window measurement from the
// executor (invariant 7), if it reports one. Zero/false when the executor does
// not measure availability (e.g. NoopExecutor).
func (s *Simulator) availability() (int, bool) {
	if reporter, ok := s.executor.(AvailabilityReporter); ok {
		return reporter.LastUnavailable()
	}
	return 0, false
}

// recordSkip reports a non-failure skip (e.g. insufficient disk) without a
// rollback and backs off before the next retry, so a transient host condition
// does not churn stage/fail/rollback every cycle.
func (s *Simulator) recordSkip(ctx context.Context, update Update, stage string, cause error) error {
	message := cause.Error()
	delay := s.bumpRetryBackoff(update.Product)
	s.recordAttempt(update, false, message)
	stateErr := SaveState(s.config.Simulation.StateFile, s.state)
	reportErr := s.client.ReportUpdate(ctx, update.Product, UpdateReportRequest{
		InstanceID:           s.config.Instance.ID,
		FromVersion:          update.FromVersion,
		ToVersion:            update.ToVersion,
		SelectedArtifactKind: update.SelectedArtifactKind, DependencyValidation: update.DependencyValidation, ArtifactDigest: update.ArtifactSHA256,
		Success: false,
		Error:   message,
		Stage:   stage,
	})
	s.logger.Warn("update skipped; backing off before retry",
		"product", update.Product,
		"stage", stage,
		"retry_in", delay.Round(time.Second).String(),
		"error", message,
	)
	return errors.Join(cause, stateErr, reportErr)
}

// firstInstallBackoffSchedule is the escalating wait between fresh-host install
// retries. The last entry is the steady-state cap.
var firstInstallBackoffSchedule = []time.Duration{
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
}

// recordFirstInstallFailure handles a failed fresh-host install without a
// rollback: it clears the staged (incomplete) release, records and reports the
// failure, and schedules an escalating back-off so the next heartbeat does not
// immediately re-provision. The host stays resumable (the artifact owns
// /opt/mysoc and is idempotent), so a later retry continues from where it
// stopped.
func (s *Simulator) recordFirstInstallFailure(ctx context.Context, update Update, cause error) error {
	message := cause.Error()
	if cleaner, ok := s.executor.(interface{ CleanupStaged(Update) error }); ok {
		if cleanErr := cleaner.CleanupStaged(update); cleanErr != nil {
			s.logger.Warn("cleanup of staged first_install failed", "product", update.Product, "error", cleanErr)
		}
	}
	delay := s.bumpRetryBackoff(update.Product)
	s.recordAttempt(update, false, message)
	stateErr := SaveState(s.config.Simulation.StateFile, s.state)
	reportErr := s.client.ReportUpdate(ctx, update.Product, UpdateReportRequest{
		InstanceID:           s.config.Instance.ID,
		FromVersion:          update.FromVersion,
		ToVersion:            update.ToVersion,
		SelectedArtifactKind: update.SelectedArtifactKind, DependencyValidation: update.DependencyValidation, ArtifactDigest: update.ArtifactSHA256,
		Success: false,
		Error:   message,
	})
	s.logger.Warn(
		"first_install failed; backing off before retry",
		"product", update.Product,
		"to_version", update.ToVersion,
		"retry_in", delay.Round(time.Second).String(),
		"error", message,
	)
	return errors.Join(cause, stateErr, reportErr)
}

// retryBackoffRemaining returns how long until the next install retry is allowed
// for a product (0 if none is pending or the window has passed).
func (s *Simulator) retryBackoffRemaining(product string) time.Duration {
	if s.state == nil || s.state.RetryBackoff == nil {
		return 0
	}
	rb := s.state.RetryBackoff[product]
	if rb == nil {
		return 0
	}
	if remaining := time.Until(rb.NextAt); remaining > 0 {
		return remaining
	}
	return 0
}

// bumpRetryBackoff advances the back-off schedule for a product and returns the
// delay applied.
func (s *Simulator) bumpRetryBackoff(product string) time.Duration {
	if s.state.RetryBackoff == nil {
		s.state.RetryBackoff = make(map[string]*RetryBackoff)
	}
	rb := s.state.RetryBackoff[product]
	if rb == nil {
		rb = &RetryBackoff{}
		s.state.RetryBackoff[product] = rb
	}
	idx := rb.Count
	if idx >= len(firstInstallBackoffSchedule) {
		idx = len(firstInstallBackoffSchedule) - 1
	}
	delay := firstInstallBackoffSchedule[idx]
	rb.Count++
	rb.NextAt = time.Now().Add(delay)
	return delay
}

// clearRetryBackoff removes any pending back-off for a product after a success.
func (s *Simulator) clearRetryBackoff(product string) {
	if s.state == nil || s.state.RetryBackoff == nil {
		return
	}
	delete(s.state.RetryBackoff, product)
}

func (s *Simulator) recordAttempt(update Update, success bool, message string) {
	attempt := &platformtypes.UpdateAttempt{
		FromVersion:          update.FromVersion,
		TargetVersion:        update.ToVersion,
		Success:              success,
		Error:                message,
		Timestamp:            time.Now().UTC(),
		SelectedArtifactKind: update.SelectedArtifactKind, DependencyValidation: update.DependencyValidation, ArtifactDigest: update.ArtifactSHA256,
	}
	s.state.LastUpdateAttempt = attempt
	if success {
		if s.state.ProductVersions == nil {
			s.state.ProductVersions = make(map[string]string)
		}
		s.state.ProductVersions[update.Product] = update.ToVersion
		if product, ok := s.config.Product(update.Product); ok {
			product.CurrentVersion = update.ToVersion
		}
	}
}

// noteParentAuth records a parent credential rejection (401/403) so the next
// heartbeat distinguishes "operator deactivated us" from "parent unreachable".
func (s *Simulator) noteParentAuth(err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		s.parentRejectedCredential = true
	}
}

// selfUpdateStatus reports whether this node can replace its own binary and the
// result of its last self-update, for fleet visibility.
func (s *Simulator) selfUpdateStatus() *platformtypes.SelfUpdateStatus {
	status := &platformtypes.SelfUpdateStatus{
		CurrentVersion: s.binaryVersion,
		LastResult:     s.state.LastSelfUpdate,
	}
	if s.config.SelfUpdate.Disabled {
		status.Capable = false
		status.Reason = "self-update disabled in config"
		return status
	}
	exe, err := executablePath()
	if err != nil {
		status.Reason = "cannot resolve running executable: " + err.Error()
		return status
	}
	if resolved, e := filepath.EvalSymlinks(exe); e == nil {
		exe = resolved
	}
	updater := &SelfUpdater{Dir: s.config.SelfUpdate.Dir}
	if updater.Manages(exe) {
		status.Capable = true
	} else {
		status.Reason = "binary is outside the managed self-update layout (needs a kit reinstall to self-update)"
	}
	return status
}

func (s *Simulator) buildHeartbeat() platformtypes.Heartbeat {
	operatingSystem, architecture := s.platform()
	products := make([]platformtypes.ProductStatus, 0, len(s.config.Products))
	for _, product := range s.config.Products {
		// HealthStatus is deliberately empty: the updater does not probe
		// product health during heartbeats (the health gate runs only
		// inside an apply), and the old "simulated" label misread as
		// "this install is fake" on hosts doing real installs.
		products = append(products, platformtypes.ProductStatus{
			Name:    product.Name,
			Version: product.CurrentVersion,
			Channel: product.Channel,
			Status:  "running",
			Uptime:  int64(time.Since(s.started).Seconds()),
		})
	}

	var children []platformtypes.ChildReport
	if s.childrenFn != nil {
		children = s.childrenFn()
	}
	var guardStats *platformtypes.RelayGuardStats
	if s.guardStatsFn != nil {
		guardStats = s.guardStatsFn()
	}
	var delta *platformtypes.DeltaEnvelope
	if s.deltaFn != nil {
		delta = s.deltaFn()
	}
	relayListenerOK := s.cycleListenerOK

	// Report real host measurements when the platform supports collection.
	// Anything not measured stays zero, which the dashboard renders as
	// "Not reported" — never as a failing 0% or an uptime of the process.
	system := platformtypes.SystemMetrics{
		OS:   operatingSystem,
		Arch: architecture,
	}
	if telemetry, ok := collectHostTelemetry(); ok {
		system.CPUUsage = telemetry.CPUUsage
		system.MemoryTotal = telemetry.MemoryTotal
		system.MemoryUsed = telemetry.MemoryUsed
		system.DiskTotal = telemetry.DiskTotal
		system.DiskUsed = telemetry.DiskUsed
		system.LoadAverage = telemetry.LoadAverage
		system.Uptime = telemetry.Uptime
	}

	return platformtypes.Heartbeat{
		InstanceID:       s.config.Instance.ID,
		InstanceType:     s.config.Instance.Type,
		ProductTier:      s.config.Instance.ProductTier,
		ParentInstanceID: s.config.Instance.ParentID,
		CustomerID:       s.config.Instance.CustomerID,
		CustomerName:     s.config.Instance.CustomerName,
		Hostname:         s.config.Instance.Hostname,
		UpdaterVersion:   s.config.Instance.UpdaterVersion,
		ConfigHash:       s.configHash(),
		License: platformtypes.LicenseStatus{
			Valid:     s.config.Server.LicenseKey != "",
			LastCheck: time.Now().UTC(),
		},
		Products:                 products,
		System:                   system,
		Timestamp:                time.Now().UTC(),
		LastUpdateAttempt:        s.state.LastUpdateAttempt,
		SelfUpdate:               s.selfUpdateStatus(),
		RelayListenerOK:          relayListenerOK,
		ParentRejectedCredential: s.parentRejectedCredential || (s.relayCredProbe != nil && s.relayCredProbe()),
		Children:                 children,
		RelayGuard:               guardStats,
		Delta:                    delta,
	}
}

func (s *Simulator) configHash() string {
	hasher := sha256.New()
	fmt.Fprintf(
		hasher,
		"%s|%s|%s|",
		s.config.Instance.ID,
		s.config.Instance.Type,
		s.config.Instance.UpdaterVersion,
	)
	for _, product := range s.config.Products {
		fmt.Fprintf(hasher, "%s|%s|%s|", product.Name, product.CurrentVersion, product.Channel)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func (s *Simulator) platform() (string, string) {
	operatingSystem := s.config.Instance.OS
	if operatingSystem == "" {
		operatingSystem = runtime.GOOS
	}
	architecture := s.config.Instance.Arch
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	return operatingSystem, architecture
}

func (s *Simulator) nextInterval() time.Duration {
	interval := s.config.Heartbeat.Interval.Duration
	jitter := s.config.Heartbeat.Jitter.Duration
	if jitter <= 0 {
		return interval
	}
	offset := time.Duration(s.random.Int63n(int64(2*jitter)+1)) - jitter
	if interval+offset <= 0 {
		return time.Second
	}
	return interval + offset
}
