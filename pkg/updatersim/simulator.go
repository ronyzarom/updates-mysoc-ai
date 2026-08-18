package updatersim

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"runtime"
	"sync"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

var ErrCycleInProgress = errors.New("simulator cycle already in progress")

// Simulator runs safe updater protocol cycles.
type Simulator struct {
	config   *Config
	client   *Client
	executor Executor
	logger   *slog.Logger
	state    *State
	started  time.Time
	cycleMu  sync.Mutex
	random   *rand.Rand
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
	return &Simulator{
		config:   cfg,
		client:   client,
		executor: executor,
		logger:   logger,
		state:    state,
		started:  time.Now(),
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// SendHeartbeat sends one heartbeat and returns server update hints.
func (s *Simulator) SendHeartbeat(ctx context.Context) (*HeartbeatResponse, error) {
	response, err := s.client.SendHeartbeat(ctx, s.buildHeartbeat())
	if err != nil {
		return nil, fmt.Errorf("send heartbeat: %w", err)
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
	case ModeObserve, ModeDownload, ModeSimulate:
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
	return errors.Join(cycleErrors...)
}

// Run starts an immediate cycle followed by jittered heartbeat cycles.
func (s *Simulator) Run(ctx context.Context, mode Mode) error {
	if err := s.RunCycle(ctx, mode); err != nil && !errors.Is(err, context.Canceled) {
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
	if offer.DownloadURL == "" {
		return fmt.Errorf("update %s %s has no download URL", offer.Product, offer.LatestVersion)
	}

	result, err := s.client.DownloadArtifact(
		ctx,
		offer.DownloadURL,
		offer.Checksum,
		s.config.Simulation.ArtifactDir,
		fmt.Sprintf("%s-%s.artifact", offer.Product, offer.LatestVersion),
		s.config.Simulation.MaxDownloadBytes,
	)
	if err != nil {
		return fmt.Errorf("download %s %s: %w", offer.Product, offer.LatestVersion, err)
	}
	s.logger.Info(
		"artifact downloaded and verified",
		"product", offer.Product,
		"target_version", offer.LatestVersion,
		"bytes", result.Size,
		"path", result.Path,
		"sha256", result.Checksum,
	)
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
		SimulationOnly: true,
	}
	if err := s.executor.Apply(ctx, update); err != nil {
		return s.failAndRollback(ctx, update, fmt.Errorf("simulated apply: %w", err))
	}
	if err := s.executor.Validate(ctx, update); err != nil {
		return s.failAndRollback(ctx, update, fmt.Errorf("simulated validation: %w", err))
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
		return fmt.Errorf("report simulated update: %w", err)
	}

	s.logger.Info(
		"simulated update completed",
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
		errorMessage += "; simulated rollback: " + rollbackErr.Error()
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
		products = append(products, platformtypes.ProductStatus{
			Name:         product.Name,
			Version:      product.CurrentVersion,
			Channel:      product.Channel,
			Status:       "running",
			Uptime:       int64(time.Since(s.started).Seconds()),
			HealthStatus: "simulated",
		})
	}

	return platformtypes.Heartbeat{
		InstanceID:       s.config.Instance.ID,
		InstanceType:     s.config.Instance.Type,
		ProductTier:      s.config.Instance.ProductTier,
		ParentInstanceID: s.config.Instance.ParentID,
		Hostname:         s.config.Instance.Hostname,
		UpdaterVersion:   s.config.Instance.UpdaterVersion,
		ConfigHash:       s.configHash(),
		License: platformtypes.LicenseStatus{
			Valid:     s.config.Server.LicenseKey != "",
			LastCheck: time.Now().UTC(),
		},
		Products: products,
		System: platformtypes.SystemMetrics{
			OS:     operatingSystem,
			Arch:   architecture,
			Uptime: int64(time.Since(s.started).Seconds()),
		},
		Timestamp:         time.Now().UTC(),
		LastUpdateAttempt: s.state.LastUpdateAttempt,
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
