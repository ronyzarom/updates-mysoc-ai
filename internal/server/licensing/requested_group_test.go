package licensing

import "testing"

// normalizeRequestedUpdateGroup maps a node's self-requested enrolment ring to
// a valid group or falls back to stable. Pure; runs without a database.
func TestNormalizeRequestedUpdateGroup(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"alpha", "alpha"},
		{"beta", "beta"},
		{"stable", "stable"},
		{"production", "production"},
		{"  Alpha ", "alpha"}, // trimmed + lowercased
		{"ALPHA", "alpha"},
		{"", "stable"},        // unset -> stable
		{"nightly", "stable"}, // unknown -> stable (never errors on the hot path)
		{"canary", "stable"},
	}
	for _, c := range cases {
		if got := normalizeRequestedUpdateGroup(c.in); got != c.want {
			t.Fatalf("normalizeRequestedUpdateGroup(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
