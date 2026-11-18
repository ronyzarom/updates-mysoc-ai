package api

import "testing"

func TestValidateCumulativeGroups(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
		ok     bool
	}{
		{"alpha only", []string{"alpha"}, true},
		{"through beta", []string{"alpha", "beta"}, true},
		{"through stable", []string{"alpha", "beta", "stable"}, true},
		{"all four", []string{"alpha", "beta", "stable", "production"}, true},
		{"unordered all four", []string{"production", "alpha", "stable", "beta"}, true},
		{"empty", nil, false},
		{"production only", []string{"production"}, false},
		{"skips beta", []string{"alpha", "stable"}, false},
		{"stable without beta", []string{"alpha", "beta", "production"}, false},
		{"unknown ring", []string{"alpha", "nightly"}, false},
		{"duplicate", []string{"alpha", "alpha"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCumulativeGroups(tc.groups)
			if (err == nil) != tc.ok {
				t.Fatalf("validateCumulativeGroups(%v) err=%v, want ok=%v", tc.groups, err, tc.ok)
			}
		})
	}
}
