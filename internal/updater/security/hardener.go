package security

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cyfox-labs/updates-mysoc-ai/internal/updater/config"
)

// Hardener applies security hardening
type Hardener struct {
	config *config.Config
}

// NewHardener creates a new security hardener
func NewHardener(cfg *config.Config) *Hardener {
	return &Hardener{config: cfg}
}

// HardenResult contains the result of a hardening action
type HardenResult struct {
	Name    string
	Success bool
	Error   string
}

// Apply applies all security hardening
func (h *Hardener) Apply() []HardenResult {
	var results []HardenResult

	if h.config.Security.Firewall.Enabled {
		results = append(results, h.applyFirewall())
	}

	if h.config.Security.SSH.Enabled {
		results = append(results, h.applySSH())
	}

	if h.config.Security.OSUpdates.Enabled {
		results = append(results, h.applyOSUpdates())
	}

	return results
}

func (h *Hardener) applyFirewall() HardenResult {
	result := HardenResult{Name: "Firewall Configuration"}

	// Flush existing rules
	exec.Command("iptables", "-F").Run()

	// Default policies
	if h.config.Security.Firewall.DefaultPolicy == "deny" {
		exec.Command("iptables", "-P", "INPUT", "DROP").Run()
		exec.Command("iptables", "-P", "FORWARD", "DROP").Run()
		exec.Command("iptables", "-P", "OUTPUT", "ACCEPT").Run()
	}

	// Allow loopback
	exec.Command("iptables", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT").Run()

	// Allow established connections
	exec.Command("iptables", "-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT").Run()

	// Apply inbound rules
	for _, rule := range h.config.Security.Firewall.AllowedInbound {
		args := []string{"-A", "INPUT", "-p", rule.Protocol, "--dport", fmt.Sprintf("%d", rule.Port)}
		if rule.Source != "" && rule.Source != "0.0.0.0/0" {
			args = append(args, "-s", rule.Source)
		}
		args = append(args, "-j", "ACCEPT")

		if err := exec.Command("iptables", args...).Run(); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("Failed to add rule for port %d: %v", rule.Port, err)
			return result
		}
	}

	// Save rules
	if err := exec.Command("sh", "-c", "iptables-save > /etc/iptables/rules.v4").Run(); err != nil {
		// Try alternative location
		exec.Command("sh", "-c", "iptables-save > /etc/iptables.rules").Run()
	}

	result.Success = true
	return result
}

func (h *Hardener) applySSH() HardenResult {
	result := HardenResult{Name: "SSH Hardening"}

	// Read current config
	data, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		result.Success = false
		result.Error = "Cannot read sshd_config"
		return result
	}

	config := string(data)
	modified := false

	// Apply each setting
	for key, value := range h.config.Security.SSH.Enforce {
		setting := fmt.Sprintf("%s %s", key, value)

		// Check if setting exists
		if strings.Contains(config, key) {
			// Replace existing setting
			lines := strings.Split(config, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, key) || strings.HasPrefix(trimmed, "#"+key) {
					lines[i] = setting
					modified = true
				}
			}
			config = strings.Join(lines, "\n")
		} else {
			// Append new setting
			config += "\n" + setting
			modified = true
		}
	}

	if modified {
		// Backup original
		exec.Command("cp", "/etc/ssh/sshd_config", "/etc/ssh/sshd_config.bak").Run()

		// Write new config
		if err := os.WriteFile("/etc/ssh/sshd_config", []byte(config), 0644); err != nil {
			result.Success = false
			result.Error = "Cannot write sshd_config"
			return result
		}

		// Test config
		if err := exec.Command("sshd", "-t").Run(); err != nil {
			// Restore backup
			exec.Command("cp", "/etc/ssh/sshd_config.bak", "/etc/ssh/sshd_config").Run()
			result.Success = false
			result.Error = "Invalid SSH config, restored backup"
			return result
		}

		// Reload SSH
		exec.Command("systemctl", "reload", "sshd").Run()
	}

	result.Success = true
	return result
}

func (h *Hardener) applyOSUpdates() HardenResult {
	result := HardenResult{Name: "Security Updates"}

	// Detect package manager
	var cmd *exec.Cmd
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		// Debian/Ubuntu
		exec.Command("apt-get", "update", "-q").Run()

		if h.config.Security.OSUpdates.SecurityOnly {
			cmd = exec.Command("apt-get", "upgrade", "-y", "-o", "Dir::Etc::sourcelist=/etc/apt/sources.list.d/security.sources")
		} else {
			cmd = exec.Command("apt-get", "upgrade", "-y")
		}
	} else if _, err := os.Stat("/usr/bin/yum"); err == nil {
		// RHEL/CentOS
		if h.config.Security.OSUpdates.SecurityOnly {
			cmd = exec.Command("yum", "update", "-y", "--security")
		} else {
			cmd = exec.Command("yum", "update", "-y")
		}
	} else {
		result.Success = false
		result.Error = "No supported package manager found"
		return result
	}

	if err := cmd.Run(); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Update failed: %v", err)
		return result
	}

	result.Success = true
	return result
}

