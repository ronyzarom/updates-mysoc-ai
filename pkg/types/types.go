package types

import (
	"time"
)

// Operator is a SOC operator: the entity a platform license is issued to.
// Everything below an operator (customers, siemcore, swf) is reported by the
// operator's platform through the update cascade, never managed here.
type Operator struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// License represents a customer license
type License struct {
	ID           string        `json:"id"`
	LicenseKey   string        `json:"license_key"`
	CustomerID   string        `json:"customer_id"`
	CustomerName string        `json:"customer_name"`
	Type         string        `json:"type"`                    // mysoc-cloud, siemcore, siemcore-lite
	Product      string        `json:"product,omitempty"`       // tier this key authorizes (mysoc for platform keys); empty on legacy keys
	OperatorRef  string        `json:"operator_ref,omitempty"`  // owning operator entity (operators.id)
	OperatorID   string        `json:"operator_id,omitempty"`   // legacy free-text operator label (pre-1.8.0)
	ResellerID   string        `json:"reseller_id,omitempty"`   // sales channel; empty for direct sales
	ResellerName string        `json:"reseller_name,omitempty"` // human-friendly reseller label
	Products     []string      `json:"products"`
	Features     []string      `json:"features,omitempty"`
	Limits       LicenseLimits `json:"limits"`
	IssuedAt     time.Time     `json:"issued_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	BoundTo      string        `json:"bound_to,omitempty"`
	IsActive     bool          `json:"is_active"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
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
	ID                string     `json:"id"`
	InstanceID        string     `json:"instance_id"`
	InstanceType      string     `json:"instance_type"`                // OS/sub-type, e.g. swf-windows, siemcore-linux
	ProductTier       string     `json:"product_tier,omitempty"`       // canonical tier: mysoc, siemcore, swf
	ParentInstanceID  string     `json:"parent_instance_id,omitempty"` // instance_id of the parent node (siemcore for swf, mysoc for siemcore)
	CustomerID        string     `json:"customer_id,omitempty"`        // end customer this node serves, as reported up the cascade
	CustomerName      string     `json:"customer_name,omitempty"`      // human-friendly customer label from the rollup
	ReportedVia       string     `json:"reported_via,omitempty"`       // instance_id of the relay that reported this node (empty = direct heartbeat)
	ReportedAt        *time.Time `json:"reported_at,omitempty"`        // when the covering rollup was received
	Hostname          string     `json:"hostname"`
	DisplayName       string     `json:"display_name,omitempty"` // Friendly name / domain (e.g., cloud.siemcore.ai)
	LicenseID         string     `json:"license_id,omitempty"`
	APIKeyHash        string     `json:"-"`
	LastHeartbeat     *time.Time `json:"last_heartbeat,omitempty"`
	LastHeartbeatData *Heartbeat `json:"last_heartbeat_data,omitempty"`
	Status            string     `json:"status"` // online, offline, degraded
	AutoUpdateEnabled bool       `json:"auto_update_enabled"`
	UpdateGroup       string     `json:"update_group"` // alpha, beta, stable, production

	// IP address tracking
	LastIPAddress string     `json:"last_ip_address,omitempty"`
	LastIPSeenAt  *time.Time `json:"last_ip_seen_at,omitempty"`

	// Update attempt tracking
	LastUpdateFromVersion   string     `json:"last_update_from_version,omitempty"`
	LastUpdateTargetVersion string     `json:"last_update_target_version,omitempty"`
	LastUpdateSuccess       *bool      `json:"last_update_success,omitempty"`
	LastUpdateError         string     `json:"last_update_error,omitempty"`
	LastUpdateAt            *time.Time `json:"last_update_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	TargetGroups      []string  `json:"target_groups"` // alpha, beta, stable, production
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

// UpdateAttempt tracks the result of an update installation
type UpdateAttempt struct {
	FromVersion   string    `json:"from_version"`
	TargetVersion string    `json:"target_version"`
	Success       bool      `json:"success"`
	Error         string    `json:"error,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// Heartbeat is the payload sent by updaters
type Heartbeat struct {
	InstanceID       string          `json:"instance_id"`
	InstanceType     string          `json:"instance_type"`
	ProductTier      string          `json:"product_tier,omitempty"`       // canonical tier: mysoc, siemcore, swf
	ParentInstanceID string          `json:"parent_instance_id,omitempty"` // parent node's instance_id (self-reported)
	CustomerID       string          `json:"customer_id,omitempty"`        // end customer this node serves
	CustomerName     string          `json:"customer_name,omitempty"`      // human-friendly customer label
	Hostname         string          `json:"hostname"`
	UpdaterVersion   string          `json:"updater_version"`
	ConfigHash       string          `json:"config_hash"`
	License          LicenseStatus   `json:"license"`
	Products         []ProductStatus `json:"products"`
	System           SystemMetrics   `json:"system"`
	Security         SecurityStatus  `json:"security,omitempty"`
	Timestamp        time.Time       `json:"timestamp"`

	// Last update attempt (included in next heartbeat after install)
	LastUpdateAttempt *UpdateAttempt `json:"last_update_attempt,omitempty"`

	// Children carries the cascaded fleet rollup: every node that heartbeats
	// to this updater (recursively). Only relays populate it.
	Children []ChildReport `json:"children,omitempty"`

	// RelayGuard carries the relay listener's self-protection counters
	// (blocked/rate-limited/banned totals). Only relays populate it.
	RelayGuard *RelayGuardStats `json:"relay_guard,omitempty"`
}

// RelayGuardStats summarizes the relay listener's port-protection activity
// since process start. Pure visibility: nothing here is configurable.
type RelayGuardStats struct {
	Blocked     uint64 `json:"blocked"`      // requests rejected (unknown IP on a restricted path)
	RateLimited uint64 `json:"rate_limited"` // requests rejected by per-IP rate limits
	Banned      uint64 `json:"banned"`       // requests rejected because the IP was temp-banned
	ActiveBans  int    `json:"active_bans"`  // IPs currently banned
	LearnedIPs  int    `json:"learned_ips"`  // source IPs learned from authenticated children
}

// ChildReport is one node in a relay's fleet rollup. Parentage is implied by
// nesting: each entry's parent is the node whose Children list contains it.
type ChildReport struct {
	InstanceID     string          `json:"instance_id"`
	InstanceType   string          `json:"instance_type,omitempty"`
	ProductTier    string          `json:"product_tier,omitempty"`
	CustomerID     string          `json:"customer_id,omitempty"`
	CustomerName   string          `json:"customer_name,omitempty"`
	Hostname       string          `json:"hostname,omitempty"`
	UpdaterVersion string          `json:"updater_version,omitempty"`
	Products       []ProductStatus `json:"products,omitempty"`
	// System carries the child's host identity and measurements so cascaded
	// nodes render OS/arch/uptime/metrics on the dashboard like direct ones.
	System            *SystemMetrics `json:"system,omitempty"`
	Status            string         `json:"status,omitempty"` // online, offline, decommissioned (relay's view)
	LastSeen          time.Time      `json:"last_seen"`
	LastUpdateAttempt *UpdateAttempt `json:"last_update_attempt,omitempty"`
	Children          []ChildReport  `json:"children,omitempty"`
	// SourceIP is the address the relay observed this child connecting from;
	// it rolls up so the dashboard can show cascaded nodes' addresses.
	SourceIP string `json:"source_ip,omitempty"`
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
	FirewallEnabled bool            `json:"firewall_enabled"`
	FirewallStatus  string          `json:"firewall_status"`
	SSHHardened     bool            `json:"ssh_hardened"`
	TLSCertificates []CertStatus    `json:"tls_certificates,omitempty"`
	PendingUpdates  int             `json:"pending_updates"`
	SecurityUpdates int             `json:"security_updates"`
	RebootRequired  bool            `json:"reboot_required"`
	ComplianceScore float64         `json:"compliance_score"`
	FailedChecks    int             `json:"failed_checks"`
	SecurityScore   int             `json:"security_score"`
	SecurityAlerts  []SecurityAlert `json:"security_alerts,omitempty"`
	LastScan        time.Time       `json:"last_scan"`
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
	Success  bool             `json:"success"`
	License  *License         `json:"license,omitempty"`
	Instance *InstanceInfo    `json:"instance,omitempty"`
	Install  *InstallManifest `json:"install,omitempty"`
	Error    string           `json:"error,omitempty"`
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
	Signature       string    `json:"signature,omitempty"` // base64 ed25519 signature over the release signing message
	Size            int64     `json:"size"`
	ReleaseNotes    string    `json:"release_notes,omitempty"`
	ReleasedAt      time.Time `json:"released_at"`
}

// ============================================
// Authentication Types
// ============================================

// User represents a dashboard user
type User struct {
	ID                string     `json:"id"`
	Email             string     `json:"email"`
	Name              string     `json:"name"`
	Role              string     `json:"role"` // admin, operator, viewer
	AvatarURL         string     `json:"avatar_url,omitempty"`
	MFAEnabled        bool       `json:"mfa_enabled"`
	IsActive          bool       `json:"is_active"`
	EmailVerified     bool       `json:"email_verified"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// UserWithPassword includes the password hash for internal use
type UserWithPassword struct {
	User
	PasswordHash        string     `json:"-"`
	MFASecret           string     `json:"-"`
	MFABackupCodes      []string   `json:"-"`
	FailedLoginAttempts int        `json:"-"`
	LockedUntil         *time.Time `json:"-"`
}

// Session represents an authenticated session
type Session struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	RefreshTokenHash string     `json:"-"`
	UserAgent        string     `json:"user_agent,omitempty"`
	IPAddress        string     `json:"ip_address,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// AuthAuditLog represents a security audit event
type AuthAuditLog struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id,omitempty"`
	EventType string                 `json:"event_type"`
	IPAddress string                 `json:"ip_address,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// LoginRequest is the initial login request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is returned after successful password verification
type LoginResponse struct {
	RequiresMFA  bool   `json:"requires_mfa"`
	MFAToken     string `json:"mfa_token,omitempty"` // Temporary token to complete MFA
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	User         *User  `json:"user,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"` // seconds
}

// MFAVerifyRequest verifies the TOTP code
type MFAVerifyRequest struct {
	MFAToken string `json:"mfa_token"` // From LoginResponse
	TOTPCode string `json:"totp_code"`
}

// MFASetupResponse contains QR code data for setting up authenticator
type MFASetupResponse struct {
	Secret     string `json:"secret"`
	QRCodeURL  string `json:"qr_code_url"`  // otpauth:// URL
	QRCodeData string `json:"qr_code_data"` // Base64 PNG
}

// MFAEnableRequest enables MFA after verifying the code
type MFAEnableRequest struct {
	TOTPCode string `json:"totp_code"`
}

// MFADisableRequest disables MFA
type MFADisableRequest struct {
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

// MFABackupCodesResponse returns backup codes
type MFABackupCodesResponse struct {
	BackupCodes []string `json:"backup_codes"`
}

// RefreshTokenRequest refreshes the access token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenResponse returns new tokens
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// UpdateProfileRequest updates user profile
type UpdateProfileRequest struct {
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// ChangePasswordRequest changes the user password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// CreateUserRequest creates a new user (admin only)
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// UpdateUserRequest updates a user (admin only)
type UpdateUserRequest struct {
	Name     string `json:"name,omitempty"`
	Role     string `json:"role,omitempty"`
	IsActive *bool  `json:"is_active,omitempty"`
}

// JWTClaims are the claims in the JWT token
type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Type   string `json:"type"` // access, refresh, mfa
}
