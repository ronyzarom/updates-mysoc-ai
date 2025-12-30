package types

import (
	"time"
)

// License represents a customer license
type License struct {
	ID           string         `json:"id"`
	LicenseKey   string         `json:"license_key"`
	CustomerID   string         `json:"customer_id"`
	CustomerName string         `json:"customer_name"`
	Type         string         `json:"type"` // mysoc-cloud, siemcore, siemcore-lite
	Products     []string       `json:"products"`
	Features     []string       `json:"features,omitempty"`
	Limits       LicenseLimits  `json:"limits"`
	IssuedAt     time.Time      `json:"issued_at"`
	ExpiresAt    time.Time      `json:"expires_at"`
	BoundTo      string         `json:"bound_to,omitempty"`
	IsActive     bool           `json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// LicenseLimits defines the limits for a license
type LicenseLimits struct {
	MaxEventsPerDay  int64 `json:"max_events_per_day"`
	MaxUsers         int   `json:"max_users"`
	MaxDataSources   int   `json:"max_data_sources"`
	MaxRetentionDays int   `json:"max_retention_days"`
}

// Instance represents a registered server instance
type Instance struct {
	ID                string          `json:"id"`
	InstanceID        string          `json:"instance_id"`
	InstanceType      string          `json:"instance_type"` // mysoc, siemcore
	Hostname          string          `json:"hostname"`
	LicenseID         string          `json:"license_id,omitempty"`
	APIKeyHash        string          `json:"-"`
	LastHeartbeat     *time.Time      `json:"last_heartbeat,omitempty"`
	LastHeartbeatData *Heartbeat      `json:"last_heartbeat_data,omitempty"`
	Status            string          `json:"status"` // online, offline, degraded
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// Release represents a product release
type Release struct {
	ID                string    `json:"id"`
	ProductName       string    `json:"product_name"`
	Version           string    `json:"version"`
	Channel           string    `json:"channel"` // stable, beta, nightly
	Manifest          Manifest  `json:"manifest"`
	ArtifactPath      string    `json:"artifact_path,omitempty"`
	ArtifactSize      int64     `json:"artifact_size"`
	Checksum          string    `json:"checksum"`
	Signature         string    `json:"signature,omitempty"`
	ReleaseNotes      string    `json:"release_notes,omitempty"`
	MinUpdaterVersion string    `json:"min_updater_version,omitempty"`
	ReleasedAt        time.Time `json:"released_at"`
	CreatedAt         time.Time `json:"created_at"`
}

// Manifest contains release metadata
type Manifest struct {
	Product      string     `json:"product"`
	Version      string     `json:"version"`
	Channel      string     `json:"channel"`
	Artifacts    []Artifact `json:"artifacts"`
	Dependencies []string   `json:"dependencies,omitempty"`
	Changelog    string     `json:"changelog,omitempty"`
}

// Artifact represents a downloadable file in a release
type Artifact struct {
	Name     string `json:"name"`
	Arch     string `json:"arch"` // linux/amd64, linux/arm64
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// Deployment tracks what's installed on an instance
type Deployment struct {
	ID              string     `json:"id"`
	InstanceID      string     `json:"instance_id"`
	ReleaseID       string     `json:"release_id"`
	Status          string     `json:"status"` // pending, downloading, installing, success, failed, rolled_back
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	PreviousVersion string     `json:"previous_version,omitempty"`
}

// Heartbeat is the payload sent by updaters
type Heartbeat struct {
	InstanceID     string          `json:"instance_id"`
	InstanceType   string          `json:"instance_type"`
	Hostname       string          `json:"hostname"`
	UpdaterVersion string          `json:"updater_version"`
	ConfigHash     string          `json:"config_hash"`
	License        LicenseStatus   `json:"license"`
	Products       []ProductStatus `json:"products"`
	System         SystemMetrics   `json:"system"`
	Security       SecurityStatus  `json:"security,omitempty"`
	Timestamp      time.Time       `json:"timestamp"`
}

// LicenseStatus reports license state
type LicenseStatus struct {
	Key       string    `json:"key"`
	Valid     bool      `json:"valid"`
	ExpiresAt time.Time `json:"expires_at"`
	LastCheck time.Time `json:"last_check"`
}

// ProductStatus reports product state
type ProductStatus struct {
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Channel        string    `json:"channel"`
	Status         string    `json:"status"` // running, stopped, crashed, updating
	Uptime         int64     `json:"uptime"`
	LastRestart    time.Time `json:"last_restart"`
	PID            int       `json:"pid,omitempty"`
	HealthEndpoint string    `json:"health_endpoint,omitempty"`
	HealthStatus   string    `json:"health_status,omitempty"`
}

// SystemMetrics reports system resource usage
type SystemMetrics struct {
	OS          string  `json:"os"`
	Arch        string  `json:"arch"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryTotal int64   `json:"memory_total"`
	MemoryUsed  int64   `json:"memory_used"`
	DiskTotal   int64   `json:"disk_total"`
	DiskUsed    int64   `json:"disk_used"`
	LoadAverage float64 `json:"load_average"`
	Uptime      int64   `json:"uptime"`
}

// SecurityStatus reports security posture
type SecurityStatus struct {
	FirewallEnabled  bool           `json:"firewall_enabled"`
	FirewallStatus   string         `json:"firewall_status"`
	SSHHardened      bool           `json:"ssh_hardened"`
	TLSCertificates  []CertStatus   `json:"tls_certificates,omitempty"`
	PendingUpdates   int            `json:"pending_updates"`
	SecurityUpdates  int            `json:"security_updates"`
	RebootRequired   bool           `json:"reboot_required"`
	ComplianceScore  float64        `json:"compliance_score"`
	FailedChecks     int            `json:"failed_checks"`
	SecurityScore    int            `json:"security_score"`
	SecurityAlerts   []SecurityAlert `json:"security_alerts,omitempty"`
	LastScan         time.Time      `json:"last_scan"`
}

// CertStatus reports TLS certificate state
type CertStatus struct {
	Domain    string    `json:"domain"`
	ExpiresAt time.Time `json:"expires_at"`
	DaysLeft  int       `json:"days_left"`
	Status    string    `json:"status"` // valid, expiring, expired
}

// SecurityAlert represents a security issue
type SecurityAlert struct {
	Type     string    `json:"type"`
	Severity string    `json:"severity"` // critical, high, medium, low
	Message  string    `json:"message"`
	Details  string    `json:"details,omitempty"`
	Time     time.Time `json:"time"`
}

// LicenseActivationRequest is the request to activate a license
type LicenseActivationRequest struct {
	LicenseKey string `json:"license_key"`
	Hostname   string `json:"hostname"`
	MachineID  string `json:"machine_id"`
}

// LicenseActivationResponse is the response from license activation
type LicenseActivationResponse struct {
	Success  bool            `json:"success"`
	License  *License        `json:"license,omitempty"`
	Instance *InstanceInfo   `json:"instance,omitempty"`
	Install  *InstallManifest `json:"install,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// InstanceInfo contains instance credentials
type InstanceInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"api_key"`
}

// InstallManifest tells the updater what to install
type InstallManifest struct {
	Products         []ProductInstall `json:"products"`
	ConfigTemplate   string           `json:"config_template"`
	SecurityBaseline string           `json:"security_baseline"`
}

// ProductInstall specifies a product to install
type ProductInstall struct {
	Name    string `json:"name"`
	Version string `json:"version"` // "latest" or specific version
	Channel string `json:"channel"`
}

// ReleaseInfo is the response for release queries
type ReleaseInfo struct {
	Product         string    `json:"product"`
	CurrentVersion  string    `json:"current_version,omitempty"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	Channel         string    `json:"channel"`
	DownloadURL     string    `json:"download_url"`
	Checksum        string    `json:"checksum"`
	Size            int64     `json:"size"`
	ReleaseNotes    string    `json:"release_notes,omitempty"`
	ReleasedAt      time.Time `json:"released_at"`
}

