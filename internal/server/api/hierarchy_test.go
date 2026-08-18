package api

import (
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func inst(instanceID, tier, parent string) types.Instance {
	return types.Instance{
		ID:               "id-" + instanceID,
		InstanceID:       instanceID,
		ProductTier:      tier,
		ParentInstanceID: parent,
		Status:           "online",
	}
}

func TestAssembleTreeNestsByTier(t *testing.T) {
	group := []types.Instance{
		inst("swf-1", "swf", "siem-1"),
		inst("mysoc-1", "mysoc", ""),
		inst("siem-1", "siemcore", "mysoc-1"),
	}

	roots := assembleTree(group)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	root := roots[0]
	if root.InstanceID != "mysoc-1" || root.ProductTier != "mysoc" {
		t.Fatalf("root = %+v, want mysoc-1/mysoc", root)
	}
	if len(root.Children) != 1 || root.Children[0].InstanceID != "siem-1" {
		t.Fatalf("expected siem-1 under mysoc-1, got %+v", root.Children)
	}
	siem := root.Children[0]
	if len(siem.Children) != 1 || siem.Children[0].InstanceID != "swf-1" {
		t.Fatalf("expected swf-1 under siem-1, got %+v", siem.Children)
	}
	if siem.Children[0].Orphan {
		t.Fatal("swf-1 has a resolvable parent and must not be marked orphan")
	}
}

func TestAssembleTreeFlagsOrphans(t *testing.T) {
	group := []types.Instance{
		inst("swf-orphan", "swf", "siem-missing"),
		inst("mysoc-1", "mysoc", ""),
	}
	roots := assembleTree(group)
	// mysoc-1 (rank 0) sorts before the orphan swf (rank 2).
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots (mysoc + orphan), got %d", len(roots))
	}
	if roots[0].InstanceID != "mysoc-1" {
		t.Fatalf("expected mysoc-1 first by rank, got %q", roots[0].InstanceID)
	}
	orphan := roots[1]
	if orphan.InstanceID != "swf-orphan" || !orphan.Orphan {
		t.Fatalf("expected swf-orphan flagged as orphan, got %+v", orphan)
	}
}

func TestAssembleTreeSortsSiblingsByRankThenID(t *testing.T) {
	group := []types.Instance{
		inst("mysoc-1", "mysoc", ""),
		inst("swf-b", "swf", "mysoc-1"), // orphan-ish: wrong parent tier, but parent exists so it nests
		inst("siem-2", "siemcore", "mysoc-1"),
		inst("siem-1", "siemcore", "mysoc-1"),
	}
	roots := assembleTree(group)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	children := roots[0].Children
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}
	// siemcore (rank 1) before swf (rank 2); within siemcore, by instance_id.
	wantOrder := []string{"siem-1", "siem-2", "swf-b"}
	for i, want := range wantOrder {
		if children[i].InstanceID != want {
			t.Fatalf("children[%d] = %q, want %q", i, children[i].InstanceID, want)
		}
	}
}

func TestMaskLicenseKey(t *testing.T) {
	cases := map[string]string{
		"SIEM-FE94-A129-BF44-A9C9": "SIEM…A9C9",
		"short":                    "****",
		"":                         "",
		"  SIEM-FE94-A129  ":       "SIEM…A129",
	}
	for in, want := range cases {
		if got := maskLicenseKey(in); got != want {
			t.Fatalf("maskLicenseKey(%q) = %q, want %q", in, got, want)
		}
	}
}
