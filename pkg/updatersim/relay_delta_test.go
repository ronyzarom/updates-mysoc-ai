package updatersim

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// heartbeatChildFull posts a full heartbeat (products, optional forwarded
// delta) and returns the decoded response.
func heartbeatChildFull(t *testing.T, relay *Relay, hb platformtypes.Heartbeat, token string) map[string]interface{} {
	t.Helper()
	body, _ := json.Marshal(hb)
	req := httptest.NewRequest("POST", "/api/v1/heartbeat", bytes.NewReader(body))
	req.Header.Set("X-License-Key", "device-secret")
	if token != "" {
		req.Header.Set("X-Relay-Token", token)
	}
	rec := httptest.NewRecorder()
	relay.handleChildHeartbeat(rec, req)
	if rec.Code != 200 {
		t.Fatalf("heartbeat returned %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRelayEnrollmentQueuesInventoryDelta(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)

	heartbeatChildFull(t, relay, platformtypes.Heartbeat{
		InstanceID:  "swf-1",
		ProductTier: "swf",
		CustomerID:  "acme",
		Products:    []platformtypes.ProductStatus{{Name: "swf", Version: "1.0.0"}},
	}, "")

	env := relay.DeltaEnvelope()
	if env == nil {
		t.Fatal("enrollment should queue a delta")
	}
	if len(env.Inventory) != 1 || env.Inventory[0].Node.InstanceID != "swf-1" {
		t.Fatalf("expected swf-1 inventory change, got %+v", env.Inventory)
	}
	// The relay's own customer summary rides the same stream.
	if len(env.Summaries) == 0 {
		t.Fatal("expected a customer summary in the delta")
	}
}

func TestRelaySteadyHeartbeatIsNoChange(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	hb := platformtypes.Heartbeat{
		InstanceID:  "swf-1",
		ProductTier: "swf",
		CustomerID:  "acme",
		Products:    []platformtypes.ProductStatus{{Name: "swf", Version: "1.0.0"}},
	}
	enroll := heartbeatChildFull(t, relay, hb, "")
	token, _ := enroll["relay_token"].(string)

	// Drain and ack the enrollment delta.
	env := relay.DeltaEnvelope()
	relay.AckUpstream(env.Cursor)
	if p := relay.delta.pending(); p != 0 {
		t.Fatalf("ack should empty the queue, %d pending", p)
	}

	// An identical heartbeat is not a material change: no new inventory delta.
	heartbeatChildFull(t, relay, hb, token)
	env = relay.DeltaEnvelope()
	if env != nil {
		for _, ic := range env.Inventory {
			if ic.Node.InstanceID == "swf-1" {
				t.Fatalf("steady heartbeat re-queued inventory: %+v", env.Inventory)
			}
		}
	}
}

func TestRelayVersionChangeRequeues(t *testing.T) {
	relay := newDecommissionTestRelay(t, t.TempDir(), true)
	hb := platformtypes.Heartbeat{
		InstanceID:  "swf-1",
		ProductTier: "swf",
		CustomerID:  "acme",
		Products:    []platformtypes.ProductStatus{{Name: "swf", Version: "1.0.0"}},
	}
	enroll := heartbeatChildFull(t, relay, hb, "")
	token, _ := enroll["relay_token"].(string)
	relay.AckUpstream(relay.DeltaEnvelope().Cursor)

	hb.Products[0].Version = "1.1.0"
	heartbeatChildFull(t, relay, hb, token)

	env := relay.DeltaEnvelope()
	if env == nil {
		t.Fatal("version change should re-queue a delta")
	}
	found := false
	for _, ic := range env.Inventory {
		if ic.Node.InstanceID == "swf-1" && ic.Node.Products[0].Version == "1.1.0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected swf-1 at 1.1.0 in the delta, got %+v", env.Inventory)
	}
}

func TestRelayForwardsChildEnvelope(t *testing.T) {
	// A siemcore relay heartbeats to a mysoc relay carrying its own delta
	// envelope; the mysoc relay must fold it in and forward it one hop up.
	relay := newDecommissionTestRelay(t, t.TempDir(), false)

	forwarded := &platformtypes.DeltaEnvelope{
		Inventory: []platformtypes.InventoryChange{
			{Seq: 3, Node: platformtypes.ChildReport{InstanceID: "leaf-9", ProductTier: "swf", CustomerID: "acme"}},
		},
		Summaries: []platformtypes.FleetSummary{
			{CustomerID: "acme", ReporterID: "siemcore-a", Total: 4, Online: 4},
		},
		Cursor: 3,
	}
	resp := heartbeatChildFull(t, relay, platformtypes.Heartbeat{
		InstanceID:  "siemcore-a",
		ProductTier: "siemcore",
		CustomerID:  "acme",
		Products:    []platformtypes.ProductStatus{{Name: "siemcore", Version: "2.0.0"}},
		Delta:       forwarded,
	}, "")

	// The relay must ack the child's forwarded cursor so the child can prune.
	if ack, ok := resp["ack_cursor"]; !ok || ack.(float64) != 3 {
		t.Fatalf("expected ack_cursor 3, got %v", resp["ack_cursor"])
	}

	env := relay.DeltaEnvelope()
	if env == nil {
		t.Fatal("forwarded envelope should be queued upstream")
	}
	var sawLeaf, sawAcme bool
	for _, ic := range env.Inventory {
		if ic.Node.InstanceID == "leaf-9" {
			sawLeaf = true
		}
	}
	for _, s := range env.Summaries {
		if s.CustomerID == "acme" && s.ReporterID == "siemcore-a" {
			sawAcme = true
		}
	}
	if !sawLeaf {
		t.Fatalf("forwarded leaf not present upstream: %+v", env.Inventory)
	}
	if !sawAcme {
		t.Fatalf("forwarded customer summary lost its reporter: %+v", env.Summaries)
	}
}
