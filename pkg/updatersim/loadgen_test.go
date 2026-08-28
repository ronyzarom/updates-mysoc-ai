package updatersim

import (
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	t.Parallel()
	s := make([]time.Duration, 100)
	for i := range s {
		s[i] = time.Duration(i+1) * time.Millisecond
	}
	p50, p95, p99, max := percentiles(s)
	if p50 != 50*time.Millisecond {
		t.Fatalf("p50=%s", p50)
	}
	if p95 != 95*time.Millisecond {
		t.Fatalf("p95=%s", p95)
	}
	if p99 != 99*time.Millisecond {
		t.Fatalf("p99=%s", p99)
	}
	if max != 100*time.Millisecond {
		t.Fatalf("max=%s", max)
	}
}

func TestPercentilesEmpty(t *testing.T) {
	t.Parallel()
	p50, p95, p99, max := percentiles(nil)
	if p50 != 0 || p95 != 0 || p99 != 0 || max != 0 {
		t.Fatal("empty sample should be all zero")
	}
}

func TestLoadgenEnrollmentCarriesFullInventory(t *testing.T) {
	t.Parallel()
	o := LoadOptions{Customers: 10, LeavesPerCustomer: 5, ChurnPercent: 20}
	o.applyDefaults()

	hb := o.buildCustomerHeartbeat(1, 3)
	if hb.Delta == nil || len(hb.Delta.Inventory) != 5 {
		t.Fatalf("enrollment cycle should carry all 5 leaves, got %+v", hb.Delta)
	}
	if hb.ProductTier != TierSiemCore || hb.CustomerID != "loadgen-cust-000003" {
		t.Fatalf("unexpected heartbeat identity: %+v", hb)
	}
}

func TestLoadgenSteadyCycleCarriesOnlyChurn(t *testing.T) {
	t.Parallel()
	o := LoadOptions{Customers: 10, LeavesPerCustomer: 100, ChurnPercent: 3}
	o.applyDefaults()

	hb := o.buildCustomerHeartbeat(2, 0)
	if got := len(hb.Delta.Inventory); got != 3 {
		t.Fatalf("steady cycle should carry 3%% of 100 leaves = 3, got %d", got)
	}
	// The summary still reports the full customer size, not just the churn.
	if hb.Delta.Summaries[0].Total != 100 {
		t.Fatalf("summary total should be full fleet size, got %d", hb.Delta.Summaries[0].Total)
	}
	if hb.Delta.Cursor == 0 {
		t.Fatal("cursor must be set")
	}
}
