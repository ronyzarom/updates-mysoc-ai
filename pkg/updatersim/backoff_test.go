package updatersim

import (
	"testing"
	"time"
)

// TestRetryBackoffSchedule proves the back-off escalates through the schedule,
// caps at the last step, reports remaining time, and clears on success.
func TestRetryBackoffSchedule(t *testing.T) {
	s := &Simulator{state: &State{}}

	if d := s.retryBackoffRemaining("mysoc", "2.0.0"); d != 0 {
		t.Fatalf("expected no back-off initially, got %s", d)
	}

	// Each bump returns the next schedule step; the last caps.
	want := append(append([]time.Duration{}, firstInstallBackoffSchedule...),
		firstInstallBackoffSchedule[len(firstInstallBackoffSchedule)-1])
	for i, expect := range want {
		got := s.bumpRetryBackoff("mysoc", "2.0.0")
		if got != expect {
			t.Fatalf("bump %d = %s, want %s", i, got, expect)
		}
	}

	if remaining := s.retryBackoffRemaining("mysoc", "2.0.0"); remaining <= 0 {
		t.Fatalf("expected a positive back-off window, got %s", remaining)
	}
	// A different product is unaffected.
	if d := s.retryBackoffRemaining("siemcore", "2.0.0"); d != 0 {
		t.Fatalf("unrelated product should have no back-off, got %s", d)
	}

	if !s.clearRetryBackoff("mysoc") {
		t.Fatal("clear should report it removed an entry")
	}
	if d := s.retryBackoffRemaining("mysoc", "2.0.0"); d != 0 {
		t.Fatalf("back-off should be cleared after success, got %s", d)
	}
}

// TestRetryBackoffRemainingExpired proves an elapsed window reports 0 (retry
// allowed) without clearing the failure count.
func TestRetryBackoffRemainingExpired(t *testing.T) {
	s := &Simulator{state: &State{RetryBackoff: map[string]*RetryBackoff{
		backoffKey("mysoc", "2.0.0"): {Count: 2, NextAt: time.Now().Add(-time.Minute)},
	}}}
	if d := s.retryBackoffRemaining("mysoc", "2.0.0"); d != 0 {
		t.Fatalf("expired window should report 0, got %s", d)
	}
	// Next bump continues the escalation (Count was 2 -> index 2 -> cap).
	if got := s.bumpRetryBackoff("mysoc", "2.0.0"); got != firstInstallBackoffSchedule[len(firstInstallBackoffSchedule)-1] {
		t.Fatalf("bump after expiry = %s, want cap", got)
	}
}

// TestRetryBackoffKeyedByVersion proves a new target version starts with a clean
// schedule instead of inheriting the failed version's cool-down, so a fix
// shipped as a new version is not throttled.
func TestRetryBackoffKeyedByVersion(t *testing.T) {
	s := &Simulator{state: &State{}}

	// Exhaust the schedule for the bad 2.0.0.
	for range firstInstallBackoffSchedule {
		s.bumpRetryBackoff("mysoc", "2.0.0")
	}
	if s.retryBackoffRemaining("mysoc", "2.0.0") <= 0 {
		t.Fatal("2.0.0 should be backing off")
	}
	// The fixed 2.0.1 is a different key: no throttle, and its first bump starts
	// at the first (shortest) schedule step.
	if d := s.retryBackoffRemaining("mysoc", "2.0.1"); d != 0 {
		t.Fatalf("new target version must not inherit back-off, got %s", d)
	}
	if got := s.bumpRetryBackoff("mysoc", "2.0.1"); got != firstInstallBackoffSchedule[0] {
		t.Fatalf("new version first bump = %s, want shortest step", got)
	}
}

// TestRetryBackoffClampsClockJump proves a NextAt persisted from a faster/future
// clock (or a backward host clock jump) never defers longer than the cap, so a
// bad clock cannot wedge updates indefinitely.
func TestRetryBackoffClampsClockJump(t *testing.T) {
	s := &Simulator{state: &State{RetryBackoff: map[string]*RetryBackoff{
		backoffKey("mysoc", "2.0.0"): {Count: 1, NextAt: time.Now().Add(1000 * time.Hour)},
	}}}
	got := s.retryBackoffRemaining("mysoc", "2.0.0")
	if got > maxRetryBackoff() {
		t.Fatalf("remaining %s exceeds clamp %s", got, maxRetryBackoff())
	}
	if got <= 0 {
		t.Fatalf("clamped remaining should still be positive, got %s", got)
	}
}

// TestClearRetryBackoffRemovesAllVersionsAndLegacy proves clearing a product
// drops every version-keyed entry and any legacy product-only key.
func TestClearRetryBackoffRemovesAllVersionsAndLegacy(t *testing.T) {
	s := &Simulator{state: &State{RetryBackoff: map[string]*RetryBackoff{
		backoffKey("mysoc", "2.0.0"): {Count: 1, NextAt: time.Now().Add(time.Hour)},
		backoffKey("mysoc", "2.0.1"): {Count: 1, NextAt: time.Now().Add(time.Hour)},
		"mysoc":                      {Count: 1, NextAt: time.Now().Add(time.Hour)}, // legacy product-only key
		backoffKey("siemcore", "9"):  {Count: 1, NextAt: time.Now().Add(time.Hour)},
	}}}
	if !s.clearRetryBackoff("mysoc") {
		t.Fatal("clear should report removal")
	}
	if len(s.state.RetryBackoff) != 1 {
		t.Fatalf("expected only the siemcore entry to remain, got %d entries", len(s.state.RetryBackoff))
	}
	if _, ok := s.state.RetryBackoff[backoffKey("siemcore", "9")]; !ok {
		t.Fatal("unrelated product entry must survive")
	}
	if s.clearRetryBackoff("absent") {
		t.Fatal("clearing an absent product should report no removal")
	}
}
