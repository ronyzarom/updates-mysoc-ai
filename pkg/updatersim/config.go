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
	Products   []ProductConfig  `yaml:"products"`
}

// ServerConfig configures the Updates Server client.
type ServerConfig struct {
	URL                    string   `yaml:"url"`
	LicenseKey             string   `yaml:"license_key,omitempty"`
	APIKey                 string   `yaml:"api_key,omitempty"`
	Timeout                Duration `yaml:"timeout"`
	MaxResponseBytes       int64    `yaml:"max_response_bytes"`
	AllowExternalDownloads bool     `yaml:"allow_external_downloads"`
}

// InstanceConfig identifies the simulated machine.
type InstanceConfig struct {
	ID             string `yaml:"id"`
	Type           string `yaml:"type"`
	Hostname       string `yaml:"hostname"`
	MachineID      string `yaml:"machine_id"`
	UpdaterVersion string `yaml:"updater_version"`
	OS             string `yaml:"os,omitempty"`
	Arch           string `yaml:"arch,omitempty"`
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
