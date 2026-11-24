package updatersim

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWideCoreKitConfigAndPins proves that splicing WideCore's pinned
// deploy/widecore-cascade/updater-executor.yaml into kits/widecore/config.yaml
// produces a valid updater configuration that loads cleanly through LoadConfig,
// and that all four bundled host files match widecore-host-files.sha256.
func TestWideCoreKitConfigAndPins(t *testing.T) {
	kitConfigPath, err := filepath.Abs("../../kits/widecore/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	hostDir, err := filepath.Abs("../../deploy/widecore-cascade")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Verify all four pinned host files match widecore-host-files.sha256.
	sumsRaw, err := os.ReadFile(filepath.Join(hostDir, "widecore-host-files.sha256"))
	if err != nil {
		t.Fatalf("read widecore-host-files.sha256: %v", err)
	}
	pinned := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(sumsRaw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed sha256 line: %q", line)
		}
		pinned[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	for _, name := range []string{
		"widecore-apply-update",
		"sudoers-widecore-updater",
		"updater-override.conf",
		"updater-executor.yaml",
	} {
		want, ok := pinned[name]
		if !ok {
			t.Fatalf("missing pin for %s", name)
		}
		data, err := os.ReadFile(filepath.Join(hostDir, name))
		if err != nil {
			t.Fatalf("read host file %s: %v", name, err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("pin mismatch for %s: got %s, want %s", name, got, want)
		}
	}

	// 2. Render config.yaml as install.sh does and splice updater-executor.yaml verbatim.
	cfgRaw, err := os.ReadFile(kitConfigPath)
	if err != nil {
		t.Fatalf("read kit config: %v", err)
	}
	blockRaw, err := os.ReadFile(filepath.Join(hostDir, "updater-executor.yaml"))
	if err != nil {
		t.Fatalf("read executor block: %v", err)
	}

	rendered := string(cfgRaw)
	rendered = strings.ReplaceAll(rendered, "@VERSION@", "1.21.0.1")
	rendered = strings.Replace(rendered, "  ca_file: mysoc-relay-ca.pem", "  # ca_file: omitted", 1)
	rendered = strings.Replace(rendered, `license_key: "CHANGE-ME-WIDECORE-CREDENTIAL"`, `license_key: "test-cred"`, 1)
	rendered = strings.Replace(rendered, "id: widecore-CHANGE-ME", "id: widecore-test-1", 1)
	rendered = strings.Replace(rendered, "parent_id: mysoc-CHANGE-ME", "parent_id: mysoc-testing-mysoc-ai", 1)
	rendered = strings.Replace(rendered, "customer_id: customer-CHANGE-ME", "customer_id: test-cust", 1)
	rendered = strings.Replace(rendered, `public_key: "PASTE-HEX-PUBLIC-KEY"`, `public_key: "`+strings.Repeat("a", 64)+`"`, 1)
	rendered = strings.Replace(rendered, "  # update_group: stable", "  update_group: alpha", 1)

	begin := "#BEGIN-FS-EXECUTOR\n"
	end := "#END-FS-EXECUTOR"
	i := strings.Index(rendered, begin)
	j := strings.Index(rendered, end)
	if i < 0 || j <= i {
		t.Fatal("missing #BEGIN-FS-EXECUTOR / #END-FS-EXECUTOR markers in kits/widecore/config.yaml")
	}
	spliced := rendered[:i+len(begin)] + strings.TrimRight(string(blockRaw), "\n") + "\n" + rendered[j:]

	tmpCfg := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmpCfg, []byte(spliced), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpCfg)
	if err != nil {
		t.Fatalf("LoadConfig(spliced widecore config) failed: %v\n--- rendered ---\n%s", err, spliced)
	}
	if cfg.Instance.ProductTier != TierWideCore {
		t.Fatalf("product_tier = %q, want %q", cfg.Instance.ProductTier, TierWideCore)
	}
	if cfg.Instance.UpdateGroup != "alpha" {
		t.Fatalf("update_group = %q, want alpha", cfg.Instance.UpdateGroup)
	}
	if cfg.Relay.Enabled {
		t.Fatal("widecore must be a leaf (relay.enabled must be false)")
	}
	if cfg.Simulation.Executor != "filesystem" {
		t.Fatalf("executor = %q, want filesystem", cfg.Simulation.Executor)
	}
	if cfg.Simulation.Filesystem.InstallRoot != "/opt/widecore-cascade" {
		t.Fatalf("install_root = %q, want /opt/widecore-cascade", cfg.Simulation.Filesystem.InstallRoot)
	}
	if cfg.Simulation.Filesystem.AvailabilityProbeURL != "http://127.0.0.1:9180/livez" {
		t.Fatalf("availability_probe_url = %q, want http://127.0.0.1:9180/livez", cfg.Simulation.Filesystem.AvailabilityProbeURL)
	}
	if cfg.Simulation.Filesystem.CommandTimeout.Duration != 15*time.Minute {
		t.Fatalf("command_timeout = %v, want 15m", cfg.Simulation.Filesystem.CommandTimeout.Duration)
	}
	if cfg.Simulation.Filesystem.FirstInstallTimeout.Duration != 30*time.Minute {
		t.Fatalf("first_install_timeout = %v, want 30m", cfg.Simulation.Filesystem.FirstInstallTimeout.Duration)
	}
	if cfg.Simulation.Filesystem.KeepReleases != 3 {
		t.Fatalf("keep_releases = %d, want 3", cfg.Simulation.Filesystem.KeepReleases)
	}
	if len(cfg.Products) != 1 || cfg.Products[0].Name != "widecore" {
		t.Fatalf("products = %+v, want single widecore entry", cfg.Products)
	}
}
