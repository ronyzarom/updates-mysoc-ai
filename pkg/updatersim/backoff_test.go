package updatersim

import (
	"testing"
	"time"
)

// TestRetryBackoffSchedule proves the fresh-install back-off escalates through
// the schedule, caps at the last step, reports remaining time, and clears on
// success.
func TestRetryBackoffSchedule(t *testing.T) {
	s := &Simulator{state: &State{}}

	if d := s.retryBackoffRemaining("mysoc"); d != 0 {
		t.Fatalf("expected no back-off initially, got %s", d)
	}

	// Each bump returns the next schedule step; the last caps.
	want := append(append([]time.Duration{}, firstInstallBackoffSchedule...),
		firstInstallBackoffSchedule[len(firstInstallBackoffSchedule)-1])
	for i, expect := range want {
		got := s.bumpRetryBackoff("mysoc")
		if got != expect {
			t.Fatalf("bump %d = %s, want %s", i, got, expect)
		}
	}

	if remaining := s.retryBackoffRemaining("mysoc"); remaining <= 0 {
		t.Fatalf("expected a positive back-off window, got %s", remaining)
	}
	// A different product is unaffected.
	if d := s.retryBackoffRemaining("siemcore"); d != 0 {
		t.Fatalf("unrelated product should have no back-off, got %s", d)
	}

	s.clearRetryBackoff("mysoc")
	if d := s.retryBackoffRemaining("mysoc"); d != 0 {
		t.Fatalf("back-off should be cleared after success, got %s", d)
	}
}

// TestRetryBackoffRemainingExpired proves an elapsed window reports 0 (retry
// allowed) without clearing the failure count.
func TestRetryBackoffRemainingExpired(t *testing.T) {
	s := &Simulator{state: &State{RetryBackoff: map[string]*RetryBackoff{
		"mysoc": {Count: 2, NextAt: time.Now().Add(-time.Minute)},
	}}}
	if d := s.retryBackoffRemaining("mysoc"); d != 0 {
		t.Fatalf("expired window should report 0, got %s", d)
	}
	// Next bump continues the escalation (Count was 2 -> index 2 -> cap).
	if got := s.bumpRetryBackoff("mysoc"); got != firstInstallBackoffSchedule[len(firstInstallBackoffSchedule)-1] {
		t.Fatalf("bump after expiry = %s, want cap", got)
	}
}
