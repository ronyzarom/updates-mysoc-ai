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
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/signing"
	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var ErrCycleInProgress = errors.New("simulator cycle already in progress")

// Simulator runs safe updater protocol cycles.
type Simulator struct {
	config    *Config
	client    *Client
	executor  Executor
	logger    *slog.Logger
	state     *State
	started   time.Time
	cycleMu   sync.Mutex
	random    *rand.Rand
	publicKey ed25519.PublicKey // release-signing key; nil disables verification

	// binaryVersion is the ldflags-stamped version of the running executable,
	// set via SetBinaryVersion. It anchors self-update comparisons; empty for
	// unstamped dev builds, which never self-update.
	binaryVersion string

	// childrenFn supplies the cascade rollup for relay-mode heartbeats.
	childrenFn func() []platformtypes.ChildReport

	// guardStatsFn supplies the relay listener's port-protection counters.
	guardStatsFn func() *platformtypes.RelayGuardStats
}

// SetChildrenProvider wires the relay's rollup into this node's heartbeats.
func (s *Simulator) SetChildrenProvider(provider func() []platformtypes.ChildReport) {
	s.childrenFn = provider
}

// SetGuardStatsProvider wires the relay guard's counters into heartbeats.
func (s *Simulator) SetGuardStatsProvider(provider func() *platformtypes.RelayGuardStats) {
	s.guardStatsFn = provider
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

	var publicKey ed25519.PublicKey
	if pubHex := strings.TrimSpace(cfg.Signing.PublicKey); pubHex != "" {
		publicKey, err = signing.ParsePublicKeyHex(pubHex)
		if err != nil {
			return nil, fmt.Errorf("signing.public_key: %w", err)
		}
	}

	return &Simulator{
		config:    cfg,
		client:    client,
		executor:  executor,
		logger:    logger,
		state:     state,
		started:   time.Now(),
		random:    rand.New(rand.NewSource(time.Now().UnixNano())),
		publicKey: publicKey,
	}, nil
}

// SendHeartbeat sends one heartbeat and returns server update hints.
func (s *Simulator) SendHeartbeat(ctx context.Context) (*HeartbeatResponse, error) {
	response, err := s.client.SendHeartbeat(ctx, s.buildHeartbeat())
	if err != nil {
		return nil, fmt.Errorf("send heartbeat: %w", err)
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
	offer, err := s.client.CheckUpdate(ctx, product.Name, UpdateCheckRequest{
		InstanceID:       s.config.Instance.ID,
		CurrentVersion:   product.CurrentVersion,
		UpdaterVersion:   s.config.Instance.UpdaterVersion,
		OS:               operatingSystem,
		Arch:             architecture,
		Hostname:         s.config.Instance.Hostname,
		Channel:          product.Channel,
		ProductTier:      s.config.Instance.ProductTier,
		ParentInstanceID: s.config.Instance.ParentID,
	})
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

	if _, err := s.SendHeartbeat(ctx); err != nil {
		return err
	}

	var cycleErrors []error
	for _, configuredProduct := range s.config.Products {
		offer, err := s.Check(ctx, configuredProduct.Name)
		if err != nil {
			cycleErrors = append(cycleErrors, err)
			continue
		}
		if !offer.UpdateAvailable {
			s.logger.Info(
				"product is up to date",
				"product", configuredProduct.Name,
				"current_version", configuredProduct.CurrentVersion,
				"source", offer.Source,
			)
			continue
		}
		if err := s.processOffer(ctx, mode, offer); err != nil {
			cycleErrors = append(cycleErrors, err)
		}
	}

	// After product work, see whether the updater itself has a newer release.
	// A successful activation aborts the cycle with ErrRestartPending so the
	// process can exit for the service manager to relaunch the new binary.
	if err := s.maybeSelfUpdate(ctx, mode); err != nil {
		if errors.Is(err, ErrRestartPending) {
			return err
		}
		cycleErrors = append(cycleErrors, err)
	}
	return errors.Join(cycleErrors...)
}

// Run starts an immediate cycle followed by jittered heartbeat cycles. It
// returns ErrRestartPending when a self-update was activated, so the caller
// can exit the process for the service manager to relaunch the new binary.
func (s *Simulator) Run(ctx context.Context, mode Mode) error {
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

	// verifyAndDownload enforces the origin release signature before touching
	// the artifact. In the cascade this is what prevents any intermediate hop
	// from substituting a payload: the signature is minted only by the updates
	// server.
	result, err := s.verifyAndDownload(ctx, offer)
	if err != nil {
		return err
	}
	if mode == ModeDownload {
		return nil
	}

	update := Update{
		Product:        offer.Product,
		FromVersion:    offer.CurrentVersion,
		ToVersion:      offer.LatestVersion,
		Channel:        offer.Channel,
		UpdateGroup:    offer.UpdateGroup,
		ReleaseNotes:   offer.ReleaseNotes,
		ArtifactPath:   result.Path,
		ArtifactSHA256: result.Checksum,
	}

	// In real mode an install must actually happen. With no executor
	// configured, reporting success would be a lie — the silent failure
	// the retired "simulate" mode allowed. Fail loudly instead so the
	// gap is visible on the dashboard and in reports.
	if _, noop := s.executor.(NoopExecutor); noop {
		return s.failAndRollback(ctx, update, fmt.Errorf(
			"no executor configured: refusing to report a successful install that did not happen (set simulation.executor: filesystem)"))
	}

	if err := s.executor.Apply(ctx, update); err != nil {
		return s.failAndRollback(ctx, update, fmt.Errorf("apply: %w", err))
	}
	if err := s.executor.Validate(ctx, update); err != nil {
		return s.failAndRollback(ctx, update, fmt.Errorf("validation: %w", err))
	}

	s.recordAttempt(update, true, "")
	if err := SaveState(s.config.Simulation.StateFile, s.state); err != nil {
		return err
	}
	if err := s.client.ReportUpdate(ctx, update.Product, UpdateReportRequest{
		InstanceID:  s.config.Instance.ID,
		FromVersion: update.FromVersion,
		ToVersion:   update.ToVersion,
		Success:     true,
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
	reportErr := s.client.ReportUpdate(ctx, update.Product, UpdateReportRequest{
		InstanceID:  s.config.Instance.ID,
		FromVersion: update.FromVersion,
		ToVersion:   update.ToVersion,
		Success:     false,
		Error:       errorMessage,
	})
	return errors.Join(updateErr, rollbackErr, stateErr, reportErr)
}

func (s *Simulator) recordAttempt(update Update, success bool, message string) {
	attempt := &platformtypes.UpdateAttempt{
		FromVersion:   update.FromVersion,
		TargetVersion: update.ToVersion,
		Success:       success,
		Error:         message,
		Timestamp:     time.Now().UTC(),
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
		Products:          products,
		System:            system,
		Timestamp:         time.Now().UTC(),
		LastUpdateAttempt: s.state.LastUpdateAttempt,
		Children:          children,
		RelayGuard:        guardStats,
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
