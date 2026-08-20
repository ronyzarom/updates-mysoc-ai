package releases

import "testing"

func TestIsNewerVersionFourPart(t *testing.T) {
	cases := []struct {
		current, next string
		want          bool
	}{
		// The fourth (build) component must participate: this was silently
		// ignored before 1.8.5, so rebuilds were never offered.
		{"2.0.0.1", "2.0.0.2", true},
		{"2.0.0.2", "2.0.0.1", false},
		{"1.8.4.1", "1.8.4.2", true},
		{"1.8.4.1", "1.8.4.1", false},
		// Three-part versions keep working; missing components are zero.
		{"1.2.3", "1.2.4", true},
		{"1.2.3", "1.2.3.1", true},
		{"1.2.3.1", "1.2.3", false},
		{"1.2.3", "1.2.3", false},
		// Higher components still dominate.
		{"1.9.9.9", "2.0.0.0", true},
		{"2.0.0.0", "1.9.9.9", false},
		// Leading v, empty current.
		{"v1.0.0", "v1.0.1", true},
		{"", "0.0.0.1", true},
	}
	for _, c := range cases {
		if got := isNewerVersion(c.current, c.next); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.current, c.next, got, c.want)
		}
	}
}
