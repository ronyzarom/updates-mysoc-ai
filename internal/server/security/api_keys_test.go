package security

import (
	"strings"
	"testing"
)

func TestGenerateAPIKeyFormat(t *testing.T) {
	full, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey error: %v", err)
	}
	if !strings.HasPrefix(full, "msk_") {
		t.Fatalf("full key %q missing msk_ prefix", full)
	}
	if len(prefix) != 12 || full[:12] != prefix {
		t.Fatalf("prefix %q must be first 12 chars of full %q", prefix, full)
	}
	if len(hash) != 64 {
		t.Fatalf("hash %q expected 64 hex chars, got %d", hash, len(hash))
	}
	if hash != HashAPIKey(full) {
		t.Fatalf("returned hash does not match HashAPIKey(full)")
	}
}

func TestGenerateAPIKeyUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		full, _, hash, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey error: %v", err)
		}
		if seen[full] {
			t.Fatalf("duplicate key generated: %q", full)
		}
		seen[full] = true
		if seen[hash] {
			t.Fatalf("duplicate hash generated: %q", hash)
		}
		seen[hash] = true
	}
}

func TestHashAPIKeyDeterministicAndTrims(t *testing.T) {
	if HashAPIKey("msk_abc") != HashAPIKey("  msk_abc  ") {
		t.Fatalf("HashAPIKey should trim surrounding whitespace")
	}
	if HashAPIKey("msk_abc") == HashAPIKey("msk_abd") {
		t.Fatalf("different keys must hash differently")
	}
}

func TestNormalizeScope(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", ScopeReleases, false},
		{"releases", ScopeReleases, false},
		{"  Releases ", ScopeReleases, false},
		{"admin", ScopeAdmin, false},
		{"ADMIN", ScopeAdmin, false},
		{"update-group", ScopeUpdateGroup, false},
		{"  Update-Group ", ScopeUpdateGroup, false},
		{"root", "", true},
		{"write", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeScope(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("NormalizeScope(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeScope(%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("NormalizeScope(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScopeAllows(t *testing.T) {
	cases := []struct {
		keyScope string
		required string
		want     bool
	}{
		{ScopeAdmin, ScopeReleases, true},    // admin covers everything
		{ScopeAdmin, ScopeAdmin, true},       // admin covers admin
		{ScopeAdmin, ScopeUpdateGroup, true}, // admin covers update-group
		{ScopeReleases, ScopeReleases, true},
		{ScopeReleases, ScopeAdmin, false},       // releases cannot do admin
		{ScopeReleases, ScopeUpdateGroup, false}, // releases cannot set rings
		{ScopeUpdateGroup, ScopeUpdateGroup, true},
		{ScopeUpdateGroup, ScopeReleases, false}, // update-group cannot do releases
		{ScopeUpdateGroup, ScopeAdmin, false},    // update-group cannot do admin
	}
	for _, c := range cases {
		if got := ScopeAllows(c.keyScope, c.required); got != c.want {
			t.Fatalf("ScopeAllows(%q, %q) = %v, want %v", c.keyScope, c.required, got, c.want)
		}
	}
}

func TestNormalizeKeyTier(t *testing.T) {
	cases := []struct {
		scope, tier string
		want        string
		wantErr     bool
	}{
		{ScopeUpdateGroup, "", "", false},                   // unbound update-group key
		{ScopeUpdateGroup, " SiemCore ", "siemcore", false}, // normalized
		{ScopeUpdateGroup, " WideCore ", "widecore", false}, // normalized
		{ScopeUpdateGroup, "mysoc", "mysoc", false},
		{ScopeUpdateGroup, "swf", "swf", false},
		{ScopeUpdateGroup, "siem-core", "", true}, // typo: not a catalog tier
		{ScopeReleases, "", "", false},
		{ScopeReleases, "siemcore", "", true}, // tier would restrict nothing
		{ScopeAdmin, "mysoc", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeKeyTier(c.scope, c.tier)
		if (err != nil) != c.wantErr {
			t.Fatalf("NormalizeKeyTier(%q, %q) err = %v, wantErr %v", c.scope, c.tier, err, c.wantErr)
		}
		if got != c.want {
			t.Fatalf("NormalizeKeyTier(%q, %q) = %q, want %q", c.scope, c.tier, got, c.want)
		}
	}
}
