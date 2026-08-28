package updatersim

import (
	"testing"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func node(id, version, status string) platformtypes.ChildReport {
	return platformtypes.ChildReport{
		InstanceID: id,
		Status:     status,
		Products:   []platformtypes.ProductStatus{{Name: "swf", Version: version}},
	}
}

func TestDeltaTrackerEmptyIsNil(t *testing.T) {
	t.Parallel()
	d := newDeltaTracker(0)
	if env := d.envelope(); env != nil {
		t.Fatalf("empty tracker should produce nil envelope, got %+v", env)
	}
}

func TestDeltaTrackerCoalescesPerNode(t *testing.T) {
	t.Parallel()
	d := newDeltaTracker(0)
	d.recordNode(node("swf-1", "1.0.0", "online"))
	d.recordNode(node("swf-1", "1.1.0", "online")) // same node changes again
	if got := d.pending(); got != 1 {
		t.Fatalf("expected 1 coalesced entry, got %d", got)
	}
	env := d.envelope()
	if len(env.Inventory) != 1 {
		t.Fatalf("expected 1 inventory change, got %d", len(env.Inventory))
	}
	if v := env.Inventory[0].Node.Products[0].Version; v != "1.1.0" {
		t.Fatalf("newest change should win, got version %s", v)
	}
}

func TestDeltaTrackerAckPrunesDeliveredOnly(t *testing.T) {
	t.Parallel()
	d := newDeltaTracker(0)
	d.recordNode(node("swf-1", "1.0.0", "online"))
	d.recordNode(node("swf-2", "1.0.0", "online"))

	env := d.envelope()
	if env.Cursor == 0 {
		t.Fatal("expected non-zero cursor")
	}
	// A new change arrives after the batch was produced but before ack.
	d.recordNode(node("swf-3", "1.0.0", "online"))

	d.ack(env.Cursor)
	if got := d.pending(); got != 1 {
		t.Fatalf("ack should leave only the post-batch change, got %d pending", got)
	}
	next := d.envelope()
	if len(next.Inventory) != 1 || next.Inventory[0].Node.InstanceID != "swf-3" {
		t.Fatalf("expected only swf-3 to remain, got %+v", next.Inventory)
	}
}

func TestDeltaTrackerChangeAfterSendSurvivesAck(t *testing.T) {
	t.Parallel()
	d := newDeltaTracker(0)
	d.recordNode(node("swf-1", "1.0.0", "online"))
	env := d.envelope()

	// swf-1 changes again before the parent acks the first batch.
	d.recordNode(node("swf-1", "2.0.0", "online"))
	d.ack(env.Cursor) // acks the stale cursor

	if got := d.pending(); got != 1 {
		t.Fatalf("re-changed node must survive stale ack, pending=%d", got)
	}
	next := d.envelope()
	if next.Inventory[0].Node.Products[0].Version != "2.0.0" {
		t.Fatalf("expected re-queued newer version, got %+v", next.Inventory[0].Node)
	}
}

func TestDeltaTrackerBoundedBatchCursorCoversPrefix(t *testing.T) {
	t.Parallel()
	d := newDeltaTracker(2)
	d.recordNode(node("a", "1", "online"))
	d.recordNode(node("b", "1", "online"))
	d.recordNode(node("c", "1", "online"))

	env := d.envelope()
	if len(env.Inventory) != 2 {
		t.Fatalf("batch should be capped at 2, got %d", len(env.Inventory))
	}
	// Acking the batch cursor must prune exactly the two delivered entries,
	// never the untransmitted third (its seq is above the cursor).
	d.ack(env.Cursor)
	if got := d.pending(); got != 1 {
		t.Fatalf("expected 1 remaining after bounded ack, got %d", got)
	}
}

func TestDeltaTrackerIngestForwards(t *testing.T) {
	t.Parallel()
	child := &platformtypes.DeltaEnvelope{
		Inventory: []platformtypes.InventoryChange{{Seq: 7, Node: node("leaf-1", "1.0.0", "online")}},
		Summaries: []platformtypes.FleetSummary{{CustomerID: "cust-1", Total: 3, Online: 3}},
		Cursor:    7,
	}
	d := newDeltaTracker(0)
	d.ingest(child)
	if got := d.pending(); got != 2 {
		t.Fatalf("ingest should queue both streams, got %d", got)
	}
	env := d.envelope()
	if len(env.Inventory) != 1 || len(env.Summaries) != 1 {
		t.Fatalf("forwarded envelope missing streams: %+v", env)
	}
	// Re-stamped with local sequence, not the child's.
	if env.Inventory[0].Seq == 7 {
		t.Fatal("ingest must restamp with local sequence")
	}
}

func TestDeltaTrackerSummaryCoalesces(t *testing.T) {
	t.Parallel()
	d := newDeltaTracker(0)
	d.recordSummary(platformtypes.FleetSummary{CustomerID: "c1", Total: 1})
	d.recordSummary(platformtypes.FleetSummary{CustomerID: "c1", Total: 2})
	if got := d.pending(); got != 1 {
		t.Fatalf("summary should coalesce per customer, got %d", got)
	}
	env := d.envelope()
	if env.Summaries[0].Total != 2 {
		t.Fatalf("newest summary should win, got %+v", env.Summaries[0])
	}
}
