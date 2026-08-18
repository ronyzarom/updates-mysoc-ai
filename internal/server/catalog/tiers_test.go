package catalog

import "testing"

func TestTiersCanonical(t *testing.T) {
	tiers := Tiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(tiers))
	}
	// Root-first ordering by rank.
	wantOrder := []string{"mysoc", "siemcore", "swf"}
	for i, want := range wantOrder {
		if tiers[i].Name != want {
			t.Fatalf("tier[%d] = %q, want %q", i, tiers[i].Name, want)
		}
		if tiers[i].Rank != i {
			t.Fatalf("tier %q rank = %d, want %d", tiers[i].Name, tiers[i].Rank, i)
		}
	}
	// Returned slice must be a copy (mutating it must not affect the catalog).
	tiers[0].Name = "mutated"
	if Tiers()[0].Name != "mysoc" {
		t.Fatal("Tiers() returned a shared slice; expected a copy")
	}
}

func TestIsValidTier(t *testing.T) {
	for _, ok := range []string{"mysoc", "siemcore", "swf"} {
		if !IsValidTier(ok) {
			t.Fatalf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "MySoc", "gateway", "root"} {
		if IsValidTier(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}

func TestExpectedParentTier(t *testing.T) {
	cases := map[string]struct {
		parent string
		ok     bool
	}{
		"mysoc":    {parent: "", ok: true},
		"siemcore": {parent: "mysoc", ok: true},
		"swf":      {parent: "siemcore", ok: true},
		"unknown":  {parent: "", ok: false},
	}
	for tier, want := range cases {
		parent, ok := ExpectedParentTier(tier)
		if ok != want.ok || parent != want.parent {
			t.Fatalf("ExpectedParentTier(%q) = (%q,%v), want (%q,%v)", tier, parent, ok, want.parent, want.ok)
		}
	}
}

func TestParentTierMatches(t *testing.T) {
	if !ParentTierMatches("swf", "siemcore") {
		t.Fatal("swf's parent should be siemcore")
	}
	if !ParentTierMatches("siemcore", "mysoc") {
		t.Fatal("siemcore's parent should be mysoc")
	}
	if ParentTierMatches("swf", "mysoc") {
		t.Fatal("swf's parent must be exactly one rank above (siemcore), not mysoc")
	}
	if ParentTierMatches("mysoc", "") {
		t.Fatal("root has no parent tier to match")
	}
}

func TestRequiresParent(t *testing.T) {
	if RequiresParent("mysoc") {
		t.Fatal("mysoc (root) must not require a parent")
	}
	if !RequiresParent("siemcore") || !RequiresParent("swf") {
		t.Fatal("siemcore and swf must require a parent")
	}
}
