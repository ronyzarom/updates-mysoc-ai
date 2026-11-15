package updatersim

import (
	"testing"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// A relay must forward a child's requested enrolment ring up the cascade so the
// server can honor it when it first creates the child's row. Pure; no network.
func TestChildReportForwardsRequestedUpdateGroup(t *testing.T) {
	hb := platformtypes.Heartbeat{
		InstanceID:           "pod-a",
		ProductTier:          "siemcore",
		RequestedUpdateGroup: "alpha",
	}
	got := childReport(hb, "online", time.Now(), "203.0.113.7")
	if got.RequestedUpdateGroup != "alpha" {
		t.Fatalf("childReport did not forward requested ring: got %q, want alpha", got.RequestedUpdateGroup)
	}
	// Absent request forwards empty (server then defaults to stable).
	if r := childReport(platformtypes.Heartbeat{InstanceID: "x"}, "online", time.Now(), ""); r.RequestedUpdateGroup != "" {
		t.Fatalf("expected empty requested ring, got %q", r.RequestedUpdateGroup)
	}
}
