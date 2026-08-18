// Package catalog defines the canonical product hierarchy shared by the update
// server and its clients: a customer owns one license that spans a three-tier
// tree of mysoc (root) -> siemcore -> swf.
package catalog

import "strings"

// Tier describes one level of the product hierarchy.
type Tier struct {
	Name        string `json:"name"`                  // canonical id used on the wire (mysoc|siemcore|swf)
	DisplayName string `json:"display_name"`          // human-friendly label for the dashboard
	Rank        int    `json:"rank"`                  // 0 = root; larger = deeper
	ParentTier  string `json:"parent_tier,omitempty"` // required parent tier ("" for the root)
}

// tiers is the ordered, canonical list of product tiers. Order is root-first.
var tiers = []Tier{
	{Name: "mysoc", DisplayName: "MySoc", Rank: 0, ParentTier: ""},
	{Name: "siemcore", DisplayName: "SiemCore", Rank: 1, ParentTier: "mysoc"},
	{Name: "swf", DisplayName: "SWF Windows Forwarder", Rank: 2, ParentTier: "siemcore"},
}

// Tiers returns a copy of the canonical tier catalog (root-first).
func Tiers() []Tier {
	out := make([]Tier, len(tiers))
	copy(out, tiers)
	return out
}

// Lookup returns the tier definition for a canonical name.
func Lookup(name string) (Tier, bool) {
	for _, t := range tiers {
		if t.Name == name {
			return t, true
		}
	}
	return Tier{}, false
}

// IsValidTier reports whether name is one of the canonical tiers.
func IsValidTier(name string) bool {
	_, ok := Lookup(name)
	return ok
}

// Normalize trims and lower-cases a tier name for tolerant comparison.
func Normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// RequiresParent reports whether a node of the given tier must declare a parent.
// The root (mysoc) does not; every deeper tier does.
func RequiresParent(name string) bool {
	t, ok := Lookup(name)
	return ok && t.ParentTier != ""
}

// ExpectedParentTier returns the tier that a parent of `name` must have.
// ok is false when `name` is not a canonical tier; parent is "" for the root.
func ExpectedParentTier(name string) (parent string, ok bool) {
	t, ok := Lookup(name)
	if !ok {
		return "", false
	}
	return t.ParentTier, true
}

// ParentTierMatches reports whether parentTier is the exact tier expected as the
// parent of childTier (i.e. one rank above). Both must be canonical tiers.
func ParentTierMatches(childTier, parentTier string) bool {
	expected, ok := ExpectedParentTier(childTier)
	if !ok || expected == "" {
		return false
	}
	return expected == parentTier
}
