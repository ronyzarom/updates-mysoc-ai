package licensing

import (
	"testing"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func child(id string, kids ...types.ChildReport) types.ChildReport {
	return types.ChildReport{InstanceID: id, Children: kids}
}

func TestFlattenReportedChildren_DepthFirstSkipsReporter(t *testing.T) {
	tree := []types.ChildReport{
		child("relay-a", child("leaf-a1"), child("leaf-a2")),
		child("relay-b", child("leaf-b1")),
		child(""),         // empty id skipped
		child("reporter"), // reporter self-reference skipped
	}

	nodes, truncated := flattenReportedChildren("reporter", tree, 100)
	if truncated {
		t.Fatalf("did not expect truncation")
	}

	var ids []string
	for _, n := range nodes {
		ids = append(ids, n.child.InstanceID)
	}
	want := []string{"relay-a", "leaf-a1", "leaf-a2", "relay-b", "leaf-b1"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order mismatch at %d: got %v, want %v", i, ids, want)
		}
	}

	// Parent linkage is preserved for the flattened rows.
	byID := map[string]reportedNode{}
	for _, n := range nodes {
		byID[n.child.InstanceID] = n
	}
	if byID["relay-a"].parentID != "reporter" {
		t.Fatalf("relay-a parent = %q, want reporter", byID["relay-a"].parentID)
	}
	if byID["leaf-a1"].parentID != "relay-a" {
		t.Fatalf("leaf-a1 parent = %q, want relay-a", byID["leaf-a1"].parentID)
	}
}

func TestFlattenReportedChildren_TruncatesNeverRejects(t *testing.T) {
	var big []types.ChildReport
	for i := 0; i < 50; i++ {
		big = append(big, child(string(rune('a'+i%26))+string(rune('0'+i/26))))
	}

	nodes, truncated := flattenReportedChildren("reporter", big, 10)
	if !truncated {
		t.Fatalf("expected truncation at budget 10")
	}
	if len(nodes) != 10 {
		t.Fatalf("got %d nodes, want exactly the budget of 10", len(nodes))
	}
}
