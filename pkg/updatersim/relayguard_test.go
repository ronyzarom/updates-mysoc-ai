package updatersim

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// guardClock is a controllable clock for guard tests.
type guardClock struct{ t time.Time }

func (c *guardClock) now() time.Time          { return c.t }
func (c *guardClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestGuard() (*relayGuard, *guardClock) {
	clock := &guardClock{t: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)}
	g := newRelayGuard(0)
	g.now = clock.now
	return g, clock
}

func TestGuardUnknownIPRestrictedToEnrollPath(t *testing.T) {
	g, _ := newTestGuard()

	if v := g.check("203.0.113.10:5000", true); v != guardAllow {
		t.Fatalf("unknown IP on enroll path: got %v, want allow", v)
	}
	if v := g.check("203.0.113.10:5000", false); v != guardDenyUnknown {
		t.Fatalf("unknown IP on restricted path: got %v, want deny-unknown", v)
	}
}

func TestGuardLearnedIPGetsFullService(t *testing.T) {
	g, clock := newTestGuard()

	g.noteAuthSuccess("203.0.113.20:5000")
	if v := g.check("203.0.113.20:5000", false); v != guardAllow {
		t.Fatalf("learned IP on restricted path: got %v, want allow", v)
	}

	// Learning expires without re-auth.
	clock.advance(guardLearnedTTL + time.Minute)
	if v := g.check("203.0.113.20:5000", false); v != guardDenyUnknown {
		t.Fatalf("expired learned IP: got %v, want deny-unknown", v)
	}
}

func TestGuardUnknownRateLimit(t *testing.T) {
	g, _ := newTestGuard()

	allowed := 0
	for i := 0; i < 20; i++ {
		if g.check("203.0.113.30:5000", true) == guardAllow {
			allowed++
		}
	}
	if allowed != int(guardUnknownBurst) {
		t.Fatalf("unknown burst: allowed %d, want %d", allowed, int(guardUnknownBurst))
	}
}

func TestGuardLearnedRateIsGenerous(t *testing.T) {
	g, _ := newTestGuard()
	g.noteAuthSuccess("203.0.113.40:5000")

	allowed := 0
	for i := 0; i < 30; i++ {
		if g.check("203.0.113.40:5000", false) == guardAllow {
			allowed++
		}
	}
	if allowed != int(guardLearnedBurst) {
		t.Fatalf("learned burst: allowed %d, want %d", allowed, int(guardLearnedBurst))
	}
}

func TestGuardNATScalingGivesBusyIPMoreBudget(t *testing.T) {
	g, clock := newTestGuard()
	addr := "203.0.113.45:5000"
	g.noteAuthSuccess(addr)

	// A NATed site with many enrolled leaves earns a proportionally larger
	// learned bucket instead of being starved by the fixed per-IP limit.
	children := 8
	g.setChildren(addr, children)

	// Let the scaled bucket refill to its (larger) ceiling.
	clock.advance(time.Minute)

	allowed := 0
	for i := 0; i < int(guardLearnedBurst)*children*2; i++ {
		if g.check(addr, false) == guardAllow {
			allowed++
		}
	}
	want := int(guardLearnedBurst) * children
	if allowed != want {
		t.Fatalf("NAT-scaled burst: allowed %d, want %d", allowed, want)
	}

	// A single-child (non-NAT) learned IP keeps the base ceiling.
	solo := "203.0.113.46:5000"
	g.noteAuthSuccess(solo)
	g.setChildren(solo, 1)
	clock.advance(time.Minute)
	soloAllowed := 0
	for i := 0; i < int(guardLearnedBurst)*4; i++ {
		if g.check(solo, false) == guardAllow {
			soloAllowed++
		}
	}
	if soloAllowed != int(guardLearnedBurst) {
		t.Fatalf("non-NAT learned burst: allowed %d, want %d", soloAllowed, int(guardLearnedBurst))
	}
}

func TestGuardNATScaleCapped(t *testing.T) {
	if got := natScale(0); got != 1 {
		t.Fatalf("natScale(0) = %v, want 1", got)
	}
	if got := natScale(1); got != 1 {
		t.Fatalf("natScale(1) = %v, want 1", got)
	}
	if got := natScale(guardNATScaleMax + 500); got != float64(guardNATScaleMax) {
		t.Fatalf("natScale over cap = %v, want %d", got, guardNATScaleMax)
	}
}

func TestGuardBanLadder(t *testing.T) {
	g, clock := newTestGuard()
	addr := "203.0.113.50:5000"

	// First ban after the failure threshold.
	for i := 0; i < guardFailThreshold; i++ {
		g.noteAuthFailure(addr)
	}
	if v := g.check(addr, true); v != guardDenyBanned {
		t.Fatalf("after %d failures: got %v, want deny-banned", guardFailThreshold, v)
	}

	// Still banned just before the first rung expires; free afterwards.
	clock.advance(guardBanLadder[0] - time.Second)
	if v := g.check(addr, true); v != guardDenyBanned {
		t.Fatalf("inside first ban window: got %v, want deny-banned", v)
	}
	clock.advance(2 * time.Second)
	if v := g.check(addr, true); v == guardDenyBanned {
		t.Fatalf("after first ban expiry: still banned")
	}

	// Second offense escalates to the next rung.
	for i := 0; i < guardFailThreshold; i++ {
		g.noteAuthFailure(addr)
	}
	clock.advance(guardBanLadder[0] + time.Minute)
	if v := g.check(addr, true); v != guardDenyBanned {
		t.Fatalf("second ban should outlast the first rung")
	}
	clock.advance(guardBanLadder[1])
	if v := g.check(addr, true); v == guardDenyBanned {
		t.Fatalf("after second ban expiry: still banned")
	}
}

func TestGuardBanRevokesLearning(t *testing.T) {
	g, _ := newTestGuard()
	addr := "203.0.113.60:5000"

	g.noteAuthSuccess(addr)
	for i := 0; i < guardFailThreshold; i++ {
		g.noteAuthFailure(addr)
	}
	// Banned now; and once the ban lapses the IP must re-learn.
	if v := g.check(addr, false); v != guardDenyBanned {
		t.Fatalf("banned IP: got %v, want deny-banned", v)
	}
}

func TestGuardFailureWindowResets(t *testing.T) {
	g, clock := newTestGuard()
	addr := "203.0.113.70:5000"

	for i := 0; i < guardFailThreshold-1; i++ {
		g.noteAuthFailure(addr)
	}
	clock.advance(guardFailWindow + time.Minute)
	g.noteAuthFailure(addr) // stale window: counts as 1, not threshold
	if v := g.check(addr, true); v == guardDenyBanned {
		t.Fatalf("failures outside the window must not accumulate into a ban")
	}
}

func TestGuardStateBounded(t *testing.T) {
	g, clock := newTestGuard()

	for i := 0; i < guardDefaultMaxIPState+500; i++ {
		addr := "10.0." + string(rune('0'+(i/250)%10)) + "." + string(rune('0'+i%250)) + ":1"
		_ = g.check(addr, true)
		if i%1000 == 0 {
			clock.advance(time.Second)
		}
	}
	g.mu.Lock()
	n := len(g.ips)
	g.mu.Unlock()
	if n > g.maxIPState {
		t.Fatalf("ip state grew past the cap: %d > %d", n, g.maxIPState)
	}
}

func TestGuardStatsCounters(t *testing.T) {
	g, _ := newTestGuard()

	g.noteAuthSuccess("203.0.113.80:1")
	_ = g.check("203.0.113.81:1", false) // deny-unknown → blocked
	for i := 0; i < guardFailThreshold; i++ {
		g.noteAuthFailure("203.0.113.82:1")
	}
	_ = g.check("203.0.113.82:1", true) // → banned counter

	stats := g.Stats()
	if stats.Blocked != 1 || stats.Banned != 1 || stats.LearnedIPs != 1 || stats.ActiveBans != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestGuardMiddlewareRejectsBeforeHandler(t *testing.T) {
	g, _ := newTestGuard()
	handlerHit := false
	h := g.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerHit = true
		w.WriteHeader(http.StatusOK)
	}))

	// Unknown IP on a download path: rejected, handler never runs.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/releases/siemcore/1.0.0/download", nil)
	req.RemoteAddr = "203.0.113.90:4444"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || handlerHit {
		t.Fatalf("unknown IP download: code=%d handlerHit=%v", rec.Code, handlerHit)
	}

	// Same IP on the enrollment path: allowed through.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat", nil)
	req.RemoteAddr = "203.0.113.90:4444"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !handlerHit {
		t.Fatalf("enroll path: code=%d handlerHit=%v", rec.Code, handlerHit)
	}
}
