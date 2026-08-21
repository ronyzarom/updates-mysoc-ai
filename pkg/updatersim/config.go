package updatersim

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultServerURL        = "https://updates.mysoc.ai"
	defaultTimeout          = 30 * time.Second
	defaultHeartbeat        = 60 * time.Second
	defaultJitter           = 5 * time.Second
	defaultDrainTimeout     = 30 * time.Second
	defaultMaxResponseBytes = int64(1 << 20)
	defaultMaxDownloadBytes = int64(1 << 30)
	defaultCommandTimeout   = 60 * time.Second
)

// Executor kinds selectable via simulation.executor.
const (
	// ExecutorNoop simulates lifecycle latency without changing the machine.
	ExecutorNoop = "noop"
	// ExecutorFilesystem performs a real versioned install with an atomic
	// current-symlink swap and rollback.
	ExecutorFilesystem = "filesystem"
)

var productNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Canonical product tiers (mysoc > siemcore > swf). A customer owns one license
// that spans the whole tree; every node presents the same license_key and
// self-reports its tier plus its parent node's instance id.
const (
	TierMySoc    = "mysoc"
	TierSiemCore = "siemcore"
	TierSWF      = "swf"
)

// tierRequiresParent maps each canonical tier to whether it must declare a
// parent (the root, mysoc, must not).
var tierRequiresParent = map[string]bool{
	TierMySoc:    false,
	TierSiemCore: true,
	TierSWF:      true,
}

func validTier(t string) bool {
	_, ok := tierRequiresParent[t]
	return ok
}

// Mode controls how far the simulator proceeds after an update is offered.
type Mode string

const (
	// ModeObserve only sends heartbeats and checks update policy.
	ModeObserve Mode = "observe"
	// ModeDownload downloads and verifies an artifact without reporting success.
	ModeDownload Mode = "download"
	// ModeSimulate downloads, verifies, runs the configured Executor, and reports.
	ModeSimulate Mode = "simulate"
)

// Duration is a YAML duration such as "30s" or "5m".
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration string.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return err
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML formats the duration as a string.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// Config configures one simulator process.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Instance   InstanceConfig   `yaml:"instance"`
	Heartbeat  HeartbeatConfig  `yaml:"heartbeat"`
	Simulation SimulationConfig `yaml:"simulation"`
	Signing    SigningConfig    `yaml:"signing,omitempty"`
	Relay      RelayConfig      `yaml:"relay,omitempty"`
	SelfUpdate SelfUpdateConfig `yaml:"self_update,omitempty"`
	Products   []ProductConfig  `yaml:"products"`
}

// SelfUpdateConfig controls how the updater updates its own binary. It is on
// by default: every cycle the updater also checks its parent for the platform
// product "updater-<os>-<arch>" using the running binary's stamped version,
// and applies offers via a staged, validated, atomic symlink swap followed by
// a service-manager restart. It only takes effect when the binary is launched
// through the managed <dir>/current symlink (the kit installs this layout).
type SelfUpdateConfig struct {
	// Disabled turns self-update checks off entirely.
	Disabled bool `yaml:"disabled,omitempty"`
	// Channel selects the release channel to follow (default "stable").
	Channel string `yaml:"channel,omitempty"`
	// Dir is the managed layout root. Default: <state_file dir>/self-update.
	Dir string `yaml:"dir,omitempty"`
}

// SigningConfig pins the release-signing public key. When Require is true the
// updater refuses any artifact whose release signature is missing or invalid.
type SigningConfig struct {
	// PublicKey is the hex-encoded ed25519 public key published by the updates
	// server (GET /api/v1/signing-key). Pin it at provisioning time.
	PublicKey string `yaml:"public_key,omitempty"`
	Require   bool   `yaml:"require,omitempty"`
}

// RelayConfig turns the updater into a cascade relay: it keeps being a normal
// updater toward its parent, and additionally serves the updater API subset
// (heartbeat, update check, artifact download) to its own children.
type RelayConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
	// Listen is the address the relay listener binds, e.g. ":8443" or
	// "127.0.0.1:8090".
	Listen string `yaml:"listen,omitempty"`
	// CacheDir holds verified pull-through artifact copies.
	CacheDir string `yaml:"cache_dir,omitempty"`
	// MaxArtifactBytes bounds one cached artifact (default 2 GiB).
	MaxArtifactBytes int64 `yaml:"max_artifact_bytes,omitempty"`
	// ChildOfflineAfter marks a child offline in rollups when it has not
	// heartbeated for this long (default 5m).
	ChildOfflineAfter Duration `yaml:"child_offline_after,omitempty"`
	// TLS configures the child-facing listener's certificate. The listener is
	// always TLS; there is no plaintext mode.
	TLS RelayTLSConfig `yaml:"tls,omitempty"`
}

// RelayTLSConfig selects the relay listener's certificate. When cert_file and
// key_file are both set the operator-provided material is used as-is.
// Otherwise the relay self-provisions a long-lived self-signed certificate
// under dir; its cert.pem doubles as the CA file child updaters pin via
// server.ca_file.
type RelayTLSConfig struct {
	// CertFile and KeyFile point at operator-provided PEM material. Set both
	// or neither.
	CertFile string `yaml:"cert_file,omitempty"`
	KeyFile  string `yaml:"key_file,omitempty"`
	// Dir holds self-provisioned material (cert.pem, key.pem). Default:
	// "relay-tls" beside the configuration file.
	Dir string `yaml:"dir,omitempty"`
	// Hosts lists extra DNS names or IP addresses children use to reach this
	// relay (the OS hostname, localhost, 127.0.0.1, and ::1 are always
	// included). Changing this list regenerates self-provisioned material.
	Hosts []string `yaml:"hosts,omitempty"`
}

// ServerConfig configures the Updates Server client.
type ServerConfig struct {
	URL                    string   `yaml:"url"`
	LicenseKey             string   `yaml:"license_key,omitempty"`
	APIKey                 string   `yaml:"api_key,omitempty"`
	Timeout                Duration `yaml:"timeout"`
	MaxResponseBytes       int64    `yaml:"max_response_bytes"`
	AllowExternalDownloads bool     `yaml:"allow_external_downloads"`
	// CAFile is a PEM bundle to trust for the server URL instead of the
	// system roots. Required when the parent is a cascade relay with a
	// self-provisioned certificate: pin the relay's cert.pem here.
	CAFile string `yaml:"ca_file,omitempty"`
}

// InstanceConfig identifies the simulated machine and its place in the product
// hierarchy (mysoc > siemcore > swf).
type InstanceConfig struct {
	ID             string `yaml:"id"`
	Type           string `yaml:"type"`
	Hostname       string `yaml:"hostname"`
	MachineID      string `yaml:"machine_id"`
	UpdaterVersion string `yaml:"updater_version"`
	OS             string `yaml:"os,omitempty"`
	Arch           string `yaml:"arch,omitempty"`
	// ProductTier is the canonical tier this node reports: mysoc, siemcore, or
	// swf. Optional; when set it is validated and sent on every heartbeat.
	ProductTier string `yaml:"product_tier,omitempty"`
	// ParentID is the instance id of this node's parent (a siemcore for an swf,
	// a mysoc for a siemcore). Required for siemcore/swf, forbidden for mysoc.
	ParentID string `yaml:"parent_id,omitempty"`
	// CustomerID identifies the end customer this node serves; it travels up
	// the cascade so the updates server can group the fleet per customer.
	CustomerID string `yaml:"customer_id,omitempty"`
	// CustomerName is the human-friendly customer label.
	CustomerName string `yaml:"customer_name,omitempty"`
}

// HeartbeatConfig controls the simulator loop.
type HeartbeatConfig struct {
	Interval Duration `yaml:"interval"`
	Jitter   Duration `yaml:"jitter"`
}

// SimulationConfig controls safe simulator behavior.
type SimulationConfig struct {
	Mode             Mode             `yaml:"mode"`
	Executor         string           `yaml:"executor,omitempty"`
	ArtifactDir      string           `yaml:"artifact_dir"`
	StateFile        string           `yaml:"state_file"`
	ManifestFile     string           `yaml:"manifest_file,omitempty"`
	MaxDownloadBytes int64            `yaml:"max_download_bytes"`
	LegacyFallback   bool             `yaml:"legacy_fallback"`
	DrainTimeout     Duration         `yaml:"drain_timeout"`
	Filesystem       FilesystemConfig `yaml:"filesystem,omitempty"`
}

// FilesystemConfig configures the real filesystem installer used when
// simulation.executor is "filesystem".
type FilesystemConfig struct {
	// InstallRoot is the base directory that holds per-product install trees.
	InstallRoot string `yaml:"install_root"`
	// RestartCommand runs after the atomic symlink swap (and after rollback).
	RestartCommand []string `yaml:"restart_command,omitempty"`
	// HealthCommand runs during validation; a non-zero exit triggers rollback.
	HealthCommand []string `yaml:"health_command,omitempty"`
	// KeepReleases bounds retained version directories (0 = keep all).
	KeepReleases int `yaml:"keep_releases,omitempty"`
	// CommandTimeout bounds restart/health command execution.
	CommandTimeout Duration `yaml:"command_timeout,omitempty"`
}

// ProductConfig identifies one simulated managed product.
type ProductConfig struct {
	Name           string `yaml:"name"`
	CurrentVersion string `yaml:"current_version"`
	Channel        string `yaml:"channel"`
}

// LoadConfig reads, defaults, validates, and resolves a YAML configuration.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read simulator config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse simulator config: %w", err)
	}

	cfg.setDefaults()
	cfg.applyEnvironment()
	if err := cfg.normalizeCredentials(); err != nil {
		return nil, err
	}
	cfg.resolvePaths(filepath.Dir(path))
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) setDefaults() {
	if c.Server.URL == "" {
		c.Server.URL = defaultServerURL
	}
	if c.Server.Timeout.Duration == 0 {
		c.Server.Timeout.Duration = defaultTimeout
	}
	if c.Server.MaxResponseBytes == 0 {
		c.Server.MaxResponseBytes = defaultMaxResponseBytes
	}
	if c.Heartbeat.Interval.Duration == 0 {
		c.Heartbeat.Interval.Duration = defaultHeartbeat
	}
	if c.Heartbeat.Jitter.Duration == 0 {
		c.Heartbeat.Jitter.Duration = defaultJitter
	}
	if c.Simulation.Mode == "" {
		c.Simulation.Mode = ModeObserve
	}
	if c.Simulation.Executor == "" {
		c.Simulation.Executor = ExecutorNoop
	}
	if c.Simulation.Filesystem.CommandTimeout.Duration == 0 {
		c.Simulation.Filesystem.CommandTimeout.Duration = defaultCommandTimeout
	}
	if c.Simulation.ArtifactDir == "" {
		c.Simulation.ArtifactDir = "simulator-artifacts"
	}
	if c.Simulation.StateFile == "" {
		c.Simulation.StateFile = ".updater-simulator-state.json"
	}
	if c.Simulation.MaxDownloadBytes == 0 {
		c.Simulation.MaxDownloadBytes = defaultMaxDownloadBytes
	}
	if c.Simulation.DrainTimeout.Duration == 0 {
		c.Simulation.DrainTimeout.Duration = defaultDrainTimeout
	}
	if c.Instance.Hostname == "" {
		c.Instance.Hostname, _ = os.Hostname()
	}
	if c.Instance.UpdaterVersion == "" {
		c.Instance.UpdaterVersion = "updater-simulator/dev"
	}
	if c.Instance.Type == "" {
		c.Instance.Type = "simulator"
	}
	if c.Relay.Enabled {
		if c.Relay.CacheDir == "" {
			c.Relay.CacheDir = "relay-cache"
		}
		if c.Relay.MaxArtifactBytes == 0 {
			c.Relay.MaxArtifactBytes = 2 << 30 // 2 GiB
		}
		if c.Relay.ChildOfflineAfter.Duration == 0 {
			c.Relay.ChildOfflineAfter.Duration = 5 * time.Minute
		}
		if c.Relay.TLS.Dir == "" {
			c.Relay.TLS.Dir = "relay-tls"
		}
	}
	for i := range c.Products {
		if c.Products[i].CurrentVersion == "" {
			c.Products[i].CurrentVersion = "0.0.0"
		}
		if c.Products[i].Channel == "" {
			c.Products[i].Channel = "stable"
		}
	}
}

func (c *Config) applyEnvironment() {
	if value := os.Getenv("UPDATER_SIM_LICENSE_KEY"); value != "" {
		c.Server.LicenseKey = value
	}
	if value := os.Getenv("UPDATER_SIM_API_KEY"); value != "" {
		c.Server.APIKey = value
	}
}

// normalizeCredentials cleans the credential fields sent to the server. The
// server matches credentials exactly (by design — see the license and admin-key
// contracts), so a quoted or padded value silently fails to authenticate. We
// normalize on the client instead of the server:
//   - surrounding whitespace (including a trailing newline from `$(cat key)`)
//     is trimmed;
//   - a value wrapped in literal quotes is rejected with a clear error rather
//     than silently stripped, so the operator fixes the source of truth.
func (c *Config) normalizeCredentials() error {
	licenseKey, err := normalizeCredential("license_key", c.Server.LicenseKey)
	if err != nil {
		return err
	}
	c.Server.LicenseKey = licenseKey

	apiKey, err := normalizeCredential("api_key", c.Server.APIKey)
	if err != nil {
		return err
	}
	c.Server.APIKey = apiKey
	return nil
}

// normalizeCredential trims surrounding whitespace and rejects a value that is
// wrapped in literal quote characters. Our credentials (license keys, msk_/admin
// API keys) never contain quotes, so a leading or trailing quote is always a
// copy/paste mistake.
func normalizeCredential(field, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if isQuoteByte(trimmed[0]) || isQuoteByte(trimmed[len(trimmed)-1]) {
		return "", fmt.Errorf(
			"%s must not be wrapped in quotes: remove the surrounding quote characters and provide the raw value",
			field)
	}
	return trimmed, nil
}

func isQuoteByte(b byte) bool {
	return b == '"' || b == '\'' || b == '`'
}

func (c *Config) resolvePaths(configDir string) {
	if configDir == "" {
		configDir = "."
	}
	if !filepath.IsAbs(c.Simulation.ArtifactDir) {
		c.Simulation.ArtifactDir = filepath.Join(configDir, c.Simulation.ArtifactDir)
	}
	if !filepath.IsAbs(c.Simulation.StateFile) {
		c.Simulation.StateFile = filepath.Join(configDir, c.Simulation.StateFile)
	}
	if c.Simulation.ManifestFile != "" && !filepath.IsAbs(c.Simulation.ManifestFile) {
		c.Simulation.ManifestFile = filepath.Join(configDir, c.Simulation.ManifestFile)
	}
	if c.Simulation.Filesystem.InstallRoot != "" && !filepath.IsAbs(c.Simulation.Filesystem.InstallRoot) {
		c.Simulation.Filesystem.InstallRoot = filepath.Join(configDir, c.Simulation.Filesystem.InstallRoot)
	}
	if c.Relay.CacheDir != "" && !filepath.IsAbs(c.Relay.CacheDir) {
		c.Relay.CacheDir = filepath.Join(configDir, c.Relay.CacheDir)
	}
	if c.Relay.TLS.Dir != "" && !filepath.IsAbs(c.Relay.TLS.Dir) {
		c.Relay.TLS.Dir = filepath.Join(configDir, c.Relay.TLS.Dir)
	}
	if c.Relay.TLS.CertFile != "" && !filepath.IsAbs(c.Relay.TLS.CertFile) {
		c.Relay.TLS.CertFile = filepath.Join(configDir, c.Relay.TLS.CertFile)
	}
	if c.Relay.TLS.KeyFile != "" && !filepath.IsAbs(c.Relay.TLS.KeyFile) {
		c.Relay.TLS.KeyFile = filepath.Join(configDir, c.Relay.TLS.KeyFile)
	}
	if c.Server.CAFile != "" && !filepath.IsAbs(c.Server.CAFile) {
		c.Server.CAFile = filepath.Join(configDir, c.Server.CAFile)
	}
	if c.SelfUpdate.Dir == "" {
		// Default beside the state file so the layout lives in the service's
		// writable data directory (e.g. /var/lib/<name>/self-update).
		c.SelfUpdate.Dir = filepath.Join(filepath.Dir(c.Simulation.StateFile), "self-update")
	} else if !filepath.IsAbs(c.SelfUpdate.Dir) {
		c.SelfUpdate.Dir = filepath.Join(configDir, c.SelfUpdate.Dir)
	}
	if c.SelfUpdate.Channel == "" {
		c.SelfUpdate.Channel = "stable"
	}
}

// Validate checks configuration that is common to all commands.
func (c *Config) Validate() error {
	baseURL, err := url.Parse(c.Server.URL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return fmt.Errorf("server URL must use http or https")
	}
	if baseURL.Host == "" || baseURL.User != nil {
		return fmt.Errorf("server URL must contain a host and no embedded credentials")
	}
	c.Server.URL = strings.TrimRight(c.Server.URL, "/")

	if c.Server.Timeout.Duration <= 0 {
		return fmt.Errorf("server timeout must be positive")
	}
	if c.Server.MaxResponseBytes <= 0 {
		return fmt.Errorf("max_response_bytes must be positive")
	}
	if c.Heartbeat.Interval.Duration <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}
	if c.Heartbeat.Jitter.Duration < 0 {
		return fmt.Errorf("heartbeat jitter cannot be negative")
	}
	if c.Heartbeat.Jitter.Duration >= c.Heartbeat.Interval.Duration {
		return fmt.Errorf("heartbeat jitter must be shorter than the interval")
	}
	if c.Simulation.MaxDownloadBytes <= 0 {
		return fmt.Errorf("max_download_bytes must be positive")
	}
	if c.Simulation.DrainTimeout.Duration <= 0 {
		return fmt.Errorf("drain_timeout must be positive")
	}
	switch c.Simulation.Mode {
	case ModeObserve, ModeDownload, ModeSimulate:
	default:
		return fmt.Errorf("invalid simulation mode %q", c.Simulation.Mode)
	}

	switch c.Simulation.Executor {
	case ExecutorNoop:
	case ExecutorFilesystem:
		if c.Simulation.Filesystem.InstallRoot == "" {
			return fmt.Errorf("simulation.filesystem.install_root is required for the filesystem executor")
		}
		if c.Simulation.Filesystem.KeepReleases < 0 {
			return fmt.Errorf("simulation.filesystem.keep_releases cannot be negative")
		}
		if c.Simulation.Filesystem.CommandTimeout.Duration <= 0 {
			return fmt.Errorf("simulation.filesystem.command_timeout must be positive")
		}
	default:
		return fmt.Errorf("invalid simulation executor %q (must be noop or filesystem)", c.Simulation.Executor)
	}

	if c.Instance.ProductTier != "" {
		tier := strings.ToLower(strings.TrimSpace(c.Instance.ProductTier))
		if !validTier(tier) {
			return fmt.Errorf("invalid instance.product_tier %q (must be mysoc, siemcore, or swf)", c.Instance.ProductTier)
		}
		parent := strings.TrimSpace(c.Instance.ParentID)
		if tierRequiresParent[tier] && parent == "" {
			return fmt.Errorf("instance.product_tier %q requires instance.parent_id (the parent node's instance id)", tier)
		}
		if !tierRequiresParent[tier] && parent != "" {
			return fmt.Errorf("instance.product_tier %q is a root and must not set instance.parent_id", tier)
		}
		c.Instance.ProductTier = tier
		c.Instance.ParentID = parent
	}

	if c.Relay.Enabled {
		if strings.TrimSpace(c.Relay.Listen) == "" {
			return fmt.Errorf("relay.listen is required when relay.enabled is true")
		}
		if c.Relay.MaxArtifactBytes <= 0 {
			return fmt.Errorf("relay.max_artifact_bytes must be positive")
		}
		if (c.Relay.TLS.CertFile == "") != (c.Relay.TLS.KeyFile == "") {
			return fmt.Errorf("relay.tls.cert_file and relay.tls.key_file must be set together")
		}
	}
	if c.Signing.Require && strings.TrimSpace(c.Signing.PublicKey) == "" {
		return fmt.Errorf("signing.require is true but signing.public_key is not set")
	}

	seen := make(map[string]struct{}, len(c.Products))
	for _, product := range c.Products {
		if !productNamePattern.MatchString(product.Name) {
			return fmt.Errorf("invalid product name %q", product.Name)
		}
		if _, ok := seen[product.Name]; ok {
			return fmt.Errorf("duplicate product %q", product.Name)
		}
		seen[product.Name] = struct{}{}
	}
	return nil
}

// Product returns the named product configuration.
func (c *Config) Product(name string) (*ProductConfig, bool) {
	for i := range c.Products {
		if c.Products[i].Name == name {
			return &c.Products[i], true
		}
	}
	return nil, false
}
