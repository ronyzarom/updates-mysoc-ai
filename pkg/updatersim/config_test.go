package updatersim

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleConfigsAreSafeObserveOnlyDefaults(t *testing.T) {
	t.Setenv("UPDATER_SIM_LICENSE_KEY", "")
	t.Setenv("UPDATER_SIM_API_KEY", "")

	for _, name := range []string{"siemcore.yaml", "swf.yaml"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "examples", "updater-simulator", name)
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("load %s: %v", name, err)
			}
			if cfg.Simulation.Mode != ModeObserve {
				t.Fatalf("%s mode = %q, want observe", name, cfg.Simulation.Mode)
			}
			if cfg.Simulation.LegacyFallback {
				t.Fatalf("%s enables group-unaware legacy fallback", name)
			}
			if cfg.Server.LicenseKey != "" || cfg.Server.APIKey != "" {
				t.Fatalf("%s contains credentials", name)
			}
			if !strings.HasPrefix(cfg.Instance.ID, "sim-") ||
				!strings.HasPrefix(cfg.Instance.MachineID, "sim-") {
				t.Fatalf("%s does not use synthetic simulator identity", name)
			}
			if len(cfg.Products) != 1 {
				t.Fatalf("%s products = %d, want 1", name, len(cfg.Products))
			}
		})
	}
}
