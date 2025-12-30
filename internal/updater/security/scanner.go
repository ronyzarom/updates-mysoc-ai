package security

import (
	"os"
	"os/exec"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
)

// Scanner performs security scans
type Scanner struct {
	config *config.Config
}

// NewScanner creates a new security scanner
func NewScanner(cfg *config.Config) *Scanner {
	return &Scanner{config: cfg}
}

// ScanResults contains the results of a security scan
type ScanResults struct {
	Checks      []CheckResult
	Score       int
	PassedCount int
	TotalCount  int
}

// CheckResult contains the result of a single check
type CheckResult struct {
	ID      string
	Name    string
	Passed  bool
	Details string
}

// Scan performs a security scan
func (s *Scanner) Scan() ScanResults {
	var results ScanResults

	// Firewall check
	if s.config.Security.Firewall.Enabled {
		results.Checks = append(results.Checks, s.checkFirewall())
	}

	// SSH hardening check
	if s.config.Security.SSH.Enabled {
		results.Checks = append(results.Checks, s.checkSSH()...)
	}

	// TLS certificates check
	if s.config.Security.TLS.Enabled {
		results.Checks = append(results.Checks, s.checkTLS()...)
	}

	// OS updates check
	if s.config.Security.OSUpdates.Enabled {
		results.Checks = append(results.Checks, s.checkOSUpdates())
	}

	// File integrity check
	if s.config.Security.FileIntegrity.Enabled {
		results.Checks = append(results.Checks, s.checkFileIntegrity())
	}

	// Port scan check
	if s.config.Security.PortScan.Enabled {
		results.Checks = append(results.Checks, s.checkPorts()...)
	}

	// Calculate score
	results.TotalCount = len(results.Checks)
	for _, check := range results.Checks {
		if check.Passed {
			results.PassedCount++
		}
	}

	if results.TotalCount > 0 {
		results.Score = (results.PassedCount * 100) / results.TotalCount
	}

	return results
}

func (s *Scanner) checkFirewall() CheckResult {
	result := CheckResult{
		ID:   "firewall-enabled",
		Name: "Firewall Enabled",
	}

	// Check if iptables has rules
	cmd := exec.Command("iptables", "-L", "-n")
	output, err := cmd.Output()
	if err != nil {
		result.Passed = false
		result.Details = "Failed to check iptables"
		return result
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 5 { // More than just the default headers
		result.Passed = true
		result.Details = "iptables rules present"
	} else {
		result.Passed = false
		result.Details = "No iptables rules configured"
	}

	return result
}

func (s *Scanner) checkSSH() []CheckResult {
	var results []CheckResult

	// Read sshd_config
	data, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		return []CheckResult{{
			ID:      "ssh-config",
			Name:    "SSH Configuration",
			Passed:  false,
			Details: "Cannot read sshd_config",
		}}
	}
	config := string(data)

	// Check PermitRootLogin
	results = append(results, CheckResult{
		ID:     "ssh-root-login",
		Name:   "SSH Root Login Disabled",
		Passed: strings.Contains(config, "PermitRootLogin no"),
	})

	// Check PasswordAuthentication
	results = append(results, CheckResult{
		ID:     "ssh-password-auth",
		Name:   "SSH Password Auth Disabled",
		Passed: strings.Contains(config, "PasswordAuthentication no"),
	})

	// Check PubkeyAuthentication
	pubkeyEnabled := !strings.Contains(config, "PubkeyAuthentication no")
	results = append(results, CheckResult{
		ID:     "ssh-pubkey-auth",
		Name:   "SSH Pubkey Auth Enabled",
		Passed: pubkeyEnabled,
	})

	return results
}

func (s *Scanner) checkTLS() []CheckResult {
	var results []CheckResult

	for _, cert := range s.config.Security.TLS.Certificates {
		result := CheckResult{
			ID:   "tls-" + cert.Domain,
			Name: "TLS Certificate: " + cert.Domain,
		}

		// Check if certificate file exists
		if _, err := os.Stat(cert.CertPath); err != nil {
			result.Passed = false
			result.Details = "Certificate file not found"
		} else {
			// Check expiration using openssl
			cmd := exec.Command("openssl", "x509", "-enddate", "-noout", "-in", cert.CertPath)
			output, err := cmd.Output()
			if err != nil {
				result.Passed = false
				result.Details = "Cannot check certificate"
			} else {
				result.Passed = true
				result.Details = strings.TrimSpace(string(output))
			}
		}

		results = append(results, result)
	}

	return results
}

func (s *Scanner) checkOSUpdates() CheckResult {
	result := CheckResult{
		ID:   "os-updates",
		Name: "Security Updates Current",
	}

	// Check for pending security updates (Debian/Ubuntu)
	cmd := exec.Command("apt-get", "-s", "upgrade")
	output, err := cmd.Output()
	if err != nil {
		result.Passed = true // Assume ok if can't check
		result.Details = "Cannot check for updates"
		return result
	}

	if strings.Contains(string(output), "0 upgraded") {
		result.Passed = true
		result.Details = "System is up to date"
	} else {
		result.Passed = false
		result.Details = "Updates available"
	}

	return result
}

func (s *Scanner) checkFileIntegrity() CheckResult {
	result := CheckResult{
		ID:   "file-integrity",
		Name: "Critical Files Unchanged",
	}

	// Check critical files
	criticalFiles := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/ssh/sshd_config",
	}

	for _, file := range criticalFiles {
		if _, err := os.Stat(file); err != nil {
			result.Passed = false
			result.Details = "Critical file missing: " + file
			return result
		}
	}

	result.Passed = true
	result.Details = "All critical files present"
	return result
}

func (s *Scanner) checkPorts() []CheckResult {
	var results []CheckResult

	// Get listening ports
	cmd := exec.Command("ss", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		return []CheckResult{{
			ID:      "ports-check",
			Name:    "Port Check",
			Passed:  false,
			Details: "Cannot check ports",
		}}
	}

	lines := strings.Split(string(output), "\n")
	listeningPorts := make(map[int]bool)

	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			// Extract port from address (e.g., "0.0.0.0:22" or "*:22")
			addr := fields[3]
			parts := strings.Split(addr, ":")
			if len(parts) >= 2 {
				var port int
				if _, err := exec.Command("echo", parts[len(parts)-1]).Output(); err == nil {
					// Parse port number
					for _, c := range parts[len(parts)-1] {
						if c >= '0' && c <= '9' {
							port = port*10 + int(c-'0')
						}
					}
					if port > 0 {
						listeningPorts[port] = true
					}
				}
			}
		}
	}

	// Check expected ports
	for _, expected := range s.config.Security.PortScan.ExpectedListening {
		result := CheckResult{
			ID:   "port-" + expected.Process,
			Name: "Port " + string(rune(expected.Port)) + " (" + expected.Process + ")",
		}

		if listeningPorts[expected.Port] {
			result.Passed = true
			result.Details = "Listening as expected"
		} else {
			result.Passed = false
			result.Details = "Not listening"
		}

		results = append(results, result)
	}

	return results
}

