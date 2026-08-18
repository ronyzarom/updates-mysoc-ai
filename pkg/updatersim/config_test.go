package updatersim

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleConfigsAreSafeObserveOnlyDefaults(t *testing.T) {
	t.Setenv("UPDATER_SIM_LICENSE_KEY", "")
	t.Setenv("UPDATER_SIM_API_KEY", "")

	for _, name := range []string{"mysoc.yaml", "siemcore.yaml", "swf.yaml"} {
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

func TestExampleConfigsDeclareHierarchy(t *testing.T) {
	t.Setenv("UPDATER_SIM_LICENSE_KEY", "")
	t.Setenv("UPDATER_SIM_API_KEY", "")

	cases := map[string]struct {
		tier   string
		parent string
	}{
		"mysoc.yaml":    {tier: TierMySoc, parent: ""},
		"siemcore.yaml": {tier: TierSiemCore, parent: "sim-mysoc-dev-01"},
		"swf.yaml":      {tier: TierSWF, parent: "sim-siemcore-dev-01"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "examples", "updater-simulator", name)
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("load %s: %v", name, err)
			}
			if cfg.Instance.ProductTier != want.tier {
				t.Fatalf("%s product_tier = %q, want %q", name, cfg.Instance.ProductTier, want.tier)
			}
			if cfg.Instance.ParentID != want.parent {
				t.Fatalf("%s parent_id = %q, want %q", name, cfg.Instance.ParentID, want.parent)
			}
		})
	}
}

func TestHierarchyValidation(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:    ServerConfig{URL: "https://updates.mysoc.ai"},
			Instance:  InstanceConfig{ID: "sim-x", Type: "sim"},
			Heartbeat: HeartbeatConfig{Interval: Duration{60_000_000_000}, Jitter: Duration{5_000_000_000}},
		}
	}

	t.Run("mysoc rejects parent", func(t *testing.T) {
		c := base()
		c.setDefaults()
		c.Instance.ProductTier = "mysoc"
		c.Instance.ParentID = "sim-parent"
		if err := c.Validate(); err == nil {
			t.Fatal("expected error: mysoc must not declare a parent")
		}
	})

	t.Run("siemcore requires parent", func(t *testing.T) {
		c := base()
		c.setDefaults()
		c.Instance.ProductTier = "siemcore"
		if err := c.Validate(); err == nil {
			t.Fatal("expected error: siemcore requires a parent")
		}
	})

	t.Run("swf with parent is valid and normalized", func(t *testing.T) {
		c := base()
		c.setDefaults()
		c.Instance.ProductTier = "SWF"
		c.Instance.ParentID = " sim-siemcore "
		if err := c.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Instance.ProductTier != "swf" {
			t.Fatalf("tier not normalized: %q", c.Instance.ProductTier)
		}
		if c.Instance.ParentID != "sim-siemcore" {
			t.Fatalf("parent not trimmed: %q", c.Instance.ParentID)
		}
	})

	t.Run("unknown tier rejected", func(t *testing.T) {
		c := base()
		c.setDefaults()
		c.Instance.ProductTier = "gateway"
		if err := c.Validate(); err == nil {
			t.Fatal("expected error: unknown tier")
		}
	})
}
