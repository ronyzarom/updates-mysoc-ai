package updatersim

import (
	"sort"
	"sync"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// deltaTracker is a relay's change-only forwarding queue (Fleet Scalability
// 1.12). It coalesces per-key state (one entry per node instance_id, one per
// customer summary) and hands out bounded, sequence-ordered batches that the
// parent acknowledges with a cursor. Coalescing bounds memory to the number
// of children/customers regardless of churn, and the seq-ordered prefix makes
// the ack cursor exact: acking cursor C prunes precisely the entries that were
// delivered and have not changed since.
//
// Everything is O(changes) in steady state: an idle fleet produces empty
// envelopes.
type deltaTracker struct {
	mu        sync.Mutex
	seq       uint64
	inventory map[string]invEntry     // by instance_id
	summaries map[string]summaryEntry // by customer_id
	maxBatch  int
}

type invEntry struct {
	seq  uint64
	node platformtypes.ChildReport
}

type summaryEntry struct {
	seq     uint64
	summary platformtypes.FleetSummary
}

const defaultDeltaBatch = 500

func newDeltaTracker(maxBatch int) *deltaTracker {
	if maxBatch <= 0 {
		maxBatch = defaultDeltaBatch
	}
	return &deltaTracker{
		inventory: map[string]invEntry{},
		summaries: map[string]summaryEntry{},
		maxBatch:  maxBatch,
	}
}

// recordNode marks one node changed (enroll, version/status change, update
// attempt, decommission/revive). The newest change wins and takes a fresh
// sequence, so a node that changes again after being sent is re-queued.
func (d *deltaTracker) recordNode(node platformtypes.ChildReport) {
	id := node.InstanceID
	if id == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	d.inventory[id] = invEntry{seq: d.seq, node: node}
}

// recordSummary marks one customer summary changed.
func (d *deltaTracker) recordSummary(s platformtypes.FleetSummary) {
	if s.CustomerID == "" {
		// The empty-customer bucket (operator platform nodes) is summarized
		// under a fixed key so it still propagates.
		s.CustomerID = ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	d.summaries[s.CustomerID] = summaryEntry{seq: d.seq, summary: s}
}

// ingest folds a child's delta envelope into this tracker, restamping each
// entry with a local sequence so it forwards upstream (store-and-forward
// across a cascade hop). The caller acks the child with the child's own
// cursor once ingest returns.
func (d *deltaTracker) ingest(env *platformtypes.DeltaEnvelope) {
	if env == nil {
		return
	}
	for _, s := range env.Summaries {
		d.recordSummary(s)
	}
	for _, c := range env.Inventory {
		d.recordNode(c.Node)
	}
}

// envelope builds the next outbound batch: the lowest-sequence pending entries
// (a seq-ordered prefix up to maxBatch) so the returned Cursor covers exactly
// the included entries. An empty tracker yields a nil envelope.
func (d *deltaTracker) envelope() *platformtypes.DeltaEnvelope {
	d.mu.Lock()
	defer d.mu.Unlock()

	type item struct {
		seq  uint64
		inv  *platformtypes.InventoryChange
		summ *platformtypes.FleetSummary
	}
	items := make([]item, 0, len(d.inventory)+len(d.summaries))
	for _, e := range d.inventory {
		ic := platformtypes.InventoryChange{Seq: e.seq, Node: e.node}
		items = append(items, item{seq: e.seq, inv: &ic})
	}
	for _, e := range d.summaries {
		s := e.summary
		items = append(items, item{seq: e.seq, summ: &s})
	}
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].seq < items[j].seq })
	if len(items) > d.maxBatch {
		items = items[:d.maxBatch]
	}

	env := &platformtypes.DeltaEnvelope{}
	for _, it := range items {
		if it.inv != nil {
			env.Inventory = append(env.Inventory, *it.inv)
		} else if it.summ != nil {
			env.Summaries = append(env.Summaries, *it.summ)
		}
		if it.seq > env.Cursor {
			env.Cursor = it.seq
		}
	}
	return env
}

// ack prunes entries delivered at or below cursor that have not changed since
// (a later change bumped their seq above cursor and is retained).
func (d *deltaTracker) ack(cursor uint64) {
	if cursor == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, e := range d.inventory {
		if e.seq <= cursor {
			delete(d.inventory, id)
		}
	}
	for id, e := range d.summaries {
		if e.seq <= cursor {
			delete(d.summaries, id)
		}
	}
}

// pending reports how many entries are queued (test/backpressure visibility).
func (d *deltaTracker) pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.inventory) + len(d.summaries)
}
