package api

import "testing"

// TestWideningBeyondAlpha proves the promotion gate only triggers when a release
// is widened to a ring beyond alpha it does not already have (invariant 7). It
// must not trigger on narrowing, re-asserting the same groups, or staying
// alpha-only.
func TestWideningBeyondAlpha(t *testing.T) {
	cases := []struct {
		name      string
		current   []string
		requested []string
		widening  bool
	}{
		{"alpha to beta", []string{"alpha"}, []string{"alpha", "beta"}, true},
		{"alpha to stable", []string{"alpha"}, []string{"stable"}, true},
		{"alpha to production", []string{"alpha"}, []string{"alpha", "production"}, true},
		{"reassert alpha", []string{"alpha"}, []string{"alpha"}, false},
		{"narrow beta to alpha", []string{"alpha", "beta"}, []string{"alpha"}, false},
		{"beta already present", []string{"alpha", "beta"}, []string{"alpha", "beta"}, false},
		{"empty requested", []string{"alpha"}, nil, false},
		{"beta to stable is widening", []string{"alpha", "beta"}, []string{"alpha", "beta", "stable"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wideningBeyondAlpha(tc.current, tc.requested); got != tc.widening {
				t.Fatalf("wideningBeyondAlpha(%v, %v) = %v, want %v",
					tc.current, tc.requested, got, tc.widening)
			}
		})
	}
}
