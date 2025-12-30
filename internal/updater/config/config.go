package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the updater configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Instance  InstanceConfig  `yaml:"instance"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	Update    UpdateConfig    `yaml:"update"`
	Products  []ProductConfig `yaml:"products"`
	Security  SecurityConfig  `yaml:"security"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// ServerConfig holds update server connection settings
type ServerConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

// InstanceConfig holds instance identification
type InstanceConfig struct {
	ID         string `yaml:"id"`
	Type       string `yaml:"type"` // mysoc, siemcore
	LicenseKey string `yaml:"license_key"`
}

// HeartbeatConfig holds heartbeat settings
type HeartbeatConfig struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

// UpdateConfig holds update settings
type UpdateConfig struct {
	CheckInterval     time.Duration      `yaml:"check_interval"`
	Channel           string             `yaml:"channel"`
	AutoUpdate        bool               `yaml:"auto_update"`
	MaintenanceWindow *MaintenanceWindow `yaml:"maintenance_window,omitempty"`
}

// MaintenanceWindow defines when updates can be applied
type MaintenanceWindow struct {
	Start    string `yaml:"start"`    // HH:MM
	End      string `yaml:"end"`      // HH:MM
	Timezone string `yaml:"timezone"` // e.g., "UTC"
}

// ProductConfig holds configuration for a managed product
type ProductConfig struct {
	Name           string `yaml:"name"`
	Service        string `yaml:"service"`        // systemd service name
	Binary         string `yaml:"binary"`         // path to binary
	Config         string `yaml:"config"`         // path to config file
	Type           string `yaml:"type"`           // binary, data
	HealthEndpoint string `yaml:"health_endpoint"` // HTTP health check URL
	HotReload      bool   `yaml:"hot_reload"`     // can reload without restart
}

// SecurityConfig holds security hardening settings
type SecurityConfig struct {
	Enabled       bool                   `yaml:"enabled"`
	ScanInterval  time.Duration          `yaml:"scan_interval"`
	Firewall      FirewallConfig         `yaml:"firewall"`
	SSH           SSHConfig              `yaml:"ssh"`
	TLS           TLSConfig              `yaml:"tls"`
	OSUpdates     OSUpdatesConfig        `yaml:"os_updates"`
	FileIntegrity FileIntegrityConfig    `yaml:"file_integrity"`
	PortScan      PortScanConfig         `yaml:"port_scan"`
	UserAudit     UserAuditConfig        `yaml:"user_audit"`
	Compliance    ComplianceConfig       `yaml:"compliance"`
}

// FirewallConfig holds firewall settings
type FirewallConfig struct {
	Enabled         bool           `yaml:"enabled"`
	DefaultPolicy   string         `yaml:"default_policy"`
	AllowedInbound  []FirewallRule `yaml:"allowed_inbound"`
	AllowedOutbound []FirewallRule `yaml:"allowed_outbound"`
}

// FirewallRule defines a firewall rule
type FirewallRule struct {
	Port     int    `yaml:"port"`
	Source   string `yaml:"source"`
	Dest     string `yaml:"dest"`
	Protocol string `yaml:"protocol"`
}

// SSHConfig holds SSH hardening settings
type SSHConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Enforce        map[string]string `yaml:"enforce"`
	AllowedUsers   []string          `yaml:"allowed_users"`
	AuthorizedKeysSource string      `yaml:"authorized_keys_source"`
}

// TLSConfig holds TLS certificate settings
type TLSConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Certificates []CertConfig      `yaml:"certificates"`
	Settings     TLSSettings       `yaml:"settings"`
}

// CertConfig holds certificate configuration
type CertConfig struct {
	Domain         string `yaml:"domain"`
	CertPath       string `yaml:"cert_path"`
	KeyPath        string `yaml:"key_path"`
	Provider       string `yaml:"provider"` // letsencrypt, managed
	RenewBeforeDays int   `yaml:"renew_before_days"`
}

// TLSSettings holds TLS security settings
type TLSSettings struct {
	MinTLSVersion string   `yaml:"min_tls_version"`
	CipherSuites  []string `yaml:"cipher_suites"`
}

// OSUpdatesConfig holds OS update settings
type OSUpdatesConfig struct {
	Enabled       bool               `yaml:"enabled"`
	SecurityOnly  bool               `yaml:"security_only"`
	Schedule      string             `yaml:"schedule"`
	AutoReboot    bool               `yaml:"auto_reboot"`
	MaintenanceWindow *MaintenanceWindow `yaml:"maintenance_window,omitempty"`
}

// FileIntegrityConfig holds file integrity monitoring settings
type FileIntegrityConfig struct {
	Enabled          bool     `yaml:"enabled"`
	MonitoredPaths   []string `yaml:"monitored_paths"`
	BaselineRefresh  string   `yaml:"baseline_refresh"`
}

// PortScanConfig holds port scanning settings
type PortScanConfig struct {
	Enabled            bool           `yaml:"enabled"`
	Interval           time.Duration  `yaml:"interval"`
	ExpectedListening  []ExpectedPort `yaml:"expected_listening"`
	AlertOnUnexpected  bool           `yaml:"alert_on_unexpected"`
}

// ExpectedPort defines an expected listening port
type ExpectedPort struct {
	Port    int    `yaml:"port"`
	Process string `yaml:"process"`
	Bind    string `yaml:"bind"` // e.g., "127.0.0.1" or "0.0.0.0"
}

// UserAuditConfig holds user audit settings
type UserAuditConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedUsers     []string `yaml:"allowed_users"`
	PrivilegedGroups []string `yaml:"privileged_groups"`
}

// ComplianceConfig holds compliance check settings
type ComplianceConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Baseline string `yaml:"baseline"`
	Schedule string `yaml:"schedule"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSize    string `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			URL: "https://updates.mysoc.ai",
		},
		Heartbeat: HeartbeatConfig{
			Interval: 60 * time.Second,
			Timeout:  10 * time.Second,
		},
		Update: UpdateConfig{
			CheckInterval: 5 * time.Minute,
			Channel:       "stable",
			AutoUpdate:    true,
		},
		Security: SecurityConfig{
			Enabled:      true,
			ScanInterval: time.Hour,
			Firewall: FirewallConfig{
				Enabled:       true,
				DefaultPolicy: "deny",
			},
			SSH: SSHConfig{
				Enabled: true,
				Enforce: map[string]string{
					"PermitRootLogin":        "no",
					"PasswordAuthentication": "no",
					"PubkeyAuthentication":   "yes",
				},
			},
			TLS: TLSConfig{
				Enabled: true,
				Settings: TLSSettings{
					MinTLSVersion: "1.2",
				},
			},
			OSUpdates: OSUpdatesConfig{
				Enabled:      true,
				SecurityOnly: true,
				Schedule:     "daily",
			},
			FileIntegrity: FileIntegrityConfig{
				Enabled:         true,
				BaselineRefresh: "weekly",
			},
			PortScan: PortScanConfig{
				Enabled:           true,
				Interval:          time.Hour,
				AlertOnUnexpected: true,
			},
			Compliance: ComplianceConfig{
				Enabled:  true,
				Baseline: "cis-level1",
				Schedule: "daily",
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "/var/log/mysoc-updater/updater.log",
			MaxSize:    "100MB",
			MaxBackups: 5,
		},
	}
}

// Load loads configuration from a file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// Save saves configuration to a file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ConfigPath returns the default config path based on instance type
func ConfigPath(instanceType string) string {
	switch instanceType {
	case "mysoc", "mysoc-cloud":
		return "/opt/mysoc/updater/config.yaml"
	default:
		return "/opt/siemcore/updater/config.yaml"
	}
}

// BaseDir returns the base directory based on instance type
func BaseDir(instanceType string) string {
	switch instanceType {
	case "mysoc", "mysoc-cloud":
		return "/opt/mysoc"
	default:
		return "/opt/siemcore"
	}
}

