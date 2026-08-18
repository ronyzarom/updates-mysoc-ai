package updatersim

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeCredential(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"clean", "SIEM-FE94-A129-BF44-A9C9", "SIEM-FE94-A129-BF44-A9C9", false},
		{"trailing space", "SIEM-FE94-A129-BF44-A9C9 ", "SIEM-FE94-A129-BF44-A9C9", false},
		{"leading and trailing whitespace", "  msk_abc  ", "msk_abc", false},
		{"trailing newline", "msk_abc\n", "msk_abc", false},
		{"double quoted", `"SIEM-FE94-A129-BF44-A9C9"`, "", true},
		{"single quoted", `'msk_abc'`, "", true},
		{"backtick quoted", "`msk_abc`", "", true},
		{"leading quote only", `"SIEM-FE94`, "", true},
		{"trailing quote only", `SIEM-FE94"`, "", true},
		{"quoted with padding", `  "msk_abc"  `, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeCredential("license_key", c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", c.in)
				}
				if !strings.Contains(err.Error(), "license_key") {
					t.Fatalf("error should name the field, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("normalizeCredential(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestLoadConfigRejectsQuotedCredential ensures the quote guard runs through the
// real load path (env-sourced credential on an example config).
func TestLoadConfigRejectsQuotedCredential(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "updater-simulator", "swf.yaml")

	t.Run("quoted license key rejected", func(t *testing.T) {
		t.Setenv("UPDATER_SIM_LICENSE_KEY", `"SIEM-FE94-A129-BF44-A9C9"`)
		t.Setenv("UPDATER_SIM_API_KEY", "")
		if _, err := LoadConfig(path); err == nil {
			t.Fatal("expected LoadConfig to reject a quoted license key")
		}
	})

	t.Run("padded license key trimmed", func(t *testing.T) {
		t.Setenv("UPDATER_SIM_LICENSE_KEY", "  SIEM-FE94-A129-BF44-A9C9\n")
		t.Setenv("UPDATER_SIM_API_KEY", "")
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if cfg.Server.LicenseKey != "SIEM-FE94-A129-BF44-A9C9" {
			t.Fatalf("license key = %q, want trimmed value", cfg.Server.LicenseKey)
		}
	})
}
