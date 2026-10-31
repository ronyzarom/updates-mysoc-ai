package updatersim

import (
	"path/filepath"
	"testing"
)

// TestLoadsGoldenConfigFromPriorRelease proves invariant 6 as a build-time
// guard: the 1.18.0 updater loads a real config + state written by the prior
// ring fleet (1.17.1.1) without modification. If a future change adds a required
// field or breaks a default, this test fails before any node does. Refresh the
// snapshot (testdata/compat/<version>/) when a new ring build ships; never edit
// it to make this pass.
func TestLoadsGoldenConfigFromPriorRelease(t *testing.T) {
	const snapshot = "1.17.1.1"
	dir := filepath.Join("testdata", "compat", snapshot)

	cfg, err := LoadConfig(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("1.18.0 must load the %s config unchanged: %v", snapshot, err)
	}

	// Spot-check that values survived and defaults filled the new 1.18.0 fields
	// rather than erroring or zeroing them.
	if cfg.Instance.ProductTier != "siemcore" {
		t.Errorf("product_tier = %q, want siemcore", cfg.Instance.ProductTier)
	}
	if !cfg.Relay.Enabled {
		t.Error("relay should be enabled in the golden config")
	}
	if cfg.Relay.DrainTimeout.Duration <= 0 {
		t.Error("relay.drain_timeout must default to a positive value for an old config")
	}
	if cfg.selfUpdateTimeout() <= 0 {
		t.Error("self-update timeout must default to a positive value")
	}
	if cfg.cycleTimeout() <= 0 || cfg.productTimeout() <= 0 {
		t.Error("cycle/product timeouts must default to positive values")
	}
	if cfg.maxUnavailableSeconds() <= 0 {
		t.Error("max_unavailable_seconds must default to a positive value")
	}
	if len(cfg.TrustedSigningKeys()) != 1 {
		t.Errorf("expected the single legacy public_key to be trusted, got %d keys", len(cfg.TrustedSigningKeys()))
	}

	state, err := LoadState(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("1.18.0 must load the %s state unchanged: %v", snapshot, err)
	}
	if state.InstanceID != "siemcore-compat-golden" {
		t.Errorf("state instance_id = %q, want siemcore-compat-golden", state.InstanceID)
	}
	if state.ProductVersions["siemcore"] != "3.3.152.23" {
		t.Errorf("state product version = %q, want 3.3.152.23", state.ProductVersions["siemcore"])
	}
	if state.LastUpdateAttempt == nil || !state.LastUpdateAttempt.Success {
		t.Error("state last_update_attempt should round-trip as a success")
	}
	// A pre-1.18.0 state has no last_self_update; it must load as nil, not error.
	if state.LastSelfUpdate != nil {
		t.Error("golden pre-1.18.0 state unexpectedly carried last_self_update")
	}
}
