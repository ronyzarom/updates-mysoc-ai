package updatersim

import (
	"net"
	"net/http"
	"sync"
	"time"

	platformtypes "github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// relayGuard is the relay listener's self-protection layer. The port is open
// to the internet by design (customers never configure firewalls), so the
// guard has to make the listener boring to abuse with zero configuration:
//
//  1. Auto-learned allowlist: source IPs that authenticate successfully get
//     full service. Unknown IPs can only reach the heartbeat/enrollment path
//     (and only at a strict rate) — never the expensive download endpoints.
//     Learning is in-memory and self-heals after a restart because children
//     re-authenticate on their next heartbeat.
//  2. Per-IP token-bucket rate limits: generous for learned IPs, strict for
//     unknown ones, so cheap floods die at the first line.
//  3. Auth-failure temp-ban ladder: repeated failures from one IP earn
//     escalating bans (5m, 30m, 6h) answered before any request parsing.
//  4. Bounded state: per-IP entries are capped and expired so scanners
//     cannot balloon relay memory; the child map is capped separately.
//
// Everything is process-local. No persistence, no knobs, no UI.
const (
	guardLearnedRate  = 10.0 // requests/second for learned IPs
	guardLearnedBurst = 20.0
	guardUnknownRate  = 5.0 / 60.0 // 5/minute for unknown IPs
	guardUnknownBurst = 5.0

	guardFailWindow    = 10 * time.Minute // auth failures counted within this window
	guardFailThreshold = 5                // failures within the window that trigger a ban

	guardLearnedTTL  = 24 * time.Hour // learned IPs expire without re-auth
	guardMaxIPState  = 10000          // cap on tracked per-IP entries
	guardMaxChildren = 500            // cap on enrolled children (anti instance_id flood)
)

// guardBanLadder is the escalating ban duration per prior ban count.
var guardBanLadder = []time.Duration{5 * time.Minute, 30 * time.Minute, 6 * time.Hour}

type guardIPState struct {
	// token bucket
	tokens     float64
	lastRefill time.Time

	// auth-failure tracking
	failures     int
	failWindowAt time.Time
	banUntil     time.Time
	banCount     int

	// allowlist
	learnedAt time.Time

	lastSeen time.Time
}

type relayGuard struct {
	mu  sync.Mutex
	ips map[string]*guardIPState

	blocked     uint64
	rateLimited uint64
	banned      uint64

	now func() time.Time // test seam
}

func newRelayGuard() *relayGuard {
	return &relayGuard{ips: map[string]*guardIPState{}, now: time.Now}
}

// remoteIP extracts the bare IP from an http.Request RemoteAddr.
func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// state returns (creating if needed) the tracked entry for ip, enforcing the
// bounded-memory cap. Callers hold g.mu.
func (g *relayGuard) state(ip string, now time.Time) *guardIPState {
	st, ok := g.ips[ip]
	if ok {
		return st
	}
	if len(g.ips) >= guardMaxIPState {
		g.evictLocked(now)
	}
	// A fresh source starts with a full unknown-tier bucket so its first
	// requests are judged by path policy, not by an empty limiter.
	st = &guardIPState{lastRefill: now, tokens: guardUnknownBurst}
	g.ips[ip] = st
	return st
}

// evictLocked drops expired and stale entries; if still over cap it removes
// the least-recently-seen non-learned, non-banned entries. Callers hold g.mu.
func (g *relayGuard) evictLocked(now time.Time) {
	for ip, st := range g.ips {
		expiredBan := !st.banUntil.IsZero() && now.After(st.banUntil)
		staleLearn := !st.learnedAt.IsZero() && now.Sub(st.learnedAt) > guardLearnedTTL
		idle := now.Sub(st.lastSeen) > guardFailWindow
		if (st.learnedAt.IsZero() || staleLearn) && (st.banUntil.IsZero() || expiredBan) && idle {
			delete(g.ips, ip)
		}
	}
	// Hard fallback: still full means an active flood from many sources;
	// drop arbitrary non-learned, non-banned entries to stay bounded.
	if len(g.ips) >= guardMaxIPState {
		for ip, st := range g.ips {
			if st.learnedAt.IsZero() && (st.banUntil.IsZero() || now.After(st.banUntil)) {
				delete(g.ips, ip)
				if len(g.ips) < guardMaxIPState {
					break
				}
			}
		}
	}
}

// noteAuthSuccess marks ip as a learned child source.
func (g *relayGuard) noteAuthSuccess(remoteAddr string) {
	ip := remoteIP(remoteAddr)
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.state(ip, now)
	st.learnedAt = now
	st.failures = 0
	st.lastSeen = now
	// Fresh trust comes with the full learned-tier bucket.
	if st.tokens < guardLearnedBurst {
		st.tokens = guardLearnedBurst
	}
}

// noteAuthFailure counts a failed authentication and applies the ban ladder.
func (g *relayGuard) noteAuthFailure(remoteAddr string) {
	ip := remoteIP(remoteAddr)
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.state(ip, now)
	st.lastSeen = now
	if now.Sub(st.failWindowAt) > guardFailWindow {
		st.failures = 0
		st.failWindowAt = now
	}
	st.failures++
	if st.failures >= guardFailThreshold {
		step := st.banCount
		if step >= len(guardBanLadder) {
			step = len(guardBanLadder) - 1
		}
		st.banUntil = now.Add(guardBanLadder[step])
		st.banCount++
		st.failures = 0
		// A banned IP is no longer trusted regardless of prior learning.
		st.learnedAt = time.Time{}
	}
}

type guardVerdict int

const (
	guardAllow guardVerdict = iota
	guardDenyBanned
	guardDenyRateLimited
	guardDenyUnknown
)

// check evaluates one request. enrollPath marks the heartbeat/enrollment
// endpoint, the only path unknown IPs may reach.
func (g *relayGuard) check(remoteAddr string, enrollPath bool) guardVerdict {
	ip := remoteIP(remoteAddr)
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()

	st := g.state(ip, now)
	st.lastSeen = now

	if !st.banUntil.IsZero() {
		if now.Before(st.banUntil) {
			g.banned++
			return guardDenyBanned
		}
		st.banUntil = time.Time{}
	}

	learned := !st.learnedAt.IsZero() && now.Sub(st.learnedAt) <= guardLearnedTTL

	rate, burst := guardUnknownRate, guardUnknownBurst
	if learned {
		rate, burst = guardLearnedRate, guardLearnedBurst
	}
	elapsed := now.Sub(st.lastRefill).Seconds()
	st.tokens += elapsed * rate
	if st.tokens > burst {
		st.tokens = burst
	}
	st.lastRefill = now
	if st.tokens < 1 {
		g.rateLimited++
		return guardDenyRateLimited
	}
	st.tokens--

	if !learned && !enrollPath {
		g.blocked++
		return guardDenyUnknown
	}
	return guardAllow
}

// Stats snapshots the guard counters for the relay's upward heartbeat.
func (g *relayGuard) Stats() *platformtypes.RelayGuardStats {
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	stats := &platformtypes.RelayGuardStats{
		Blocked:     g.blocked,
		RateLimited: g.rateLimited,
		Banned:      g.banned,
	}
	for _, st := range g.ips {
		if !st.banUntil.IsZero() && now.Before(st.banUntil) {
			stats.ActiveBans++
		}
		if !st.learnedAt.IsZero() && now.Sub(st.learnedAt) <= guardLearnedTTL {
			stats.LearnedIPs++
		}
	}
	return stats
}

// middleware wraps the relay mux with the guard checks. The enrollment path
// (child heartbeat) is the only route unknown IPs may reach; everything else
// requires a learned source. Denials are answered before any body parsing.
func (g *relayGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		enrollPath := req.Method == http.MethodPost && req.URL.Path == "/api/v1/heartbeat"
		switch g.check(req.RemoteAddr, enrollPath) {
		case guardDenyBanned:
			relayError(w, http.StatusForbidden, "source temporarily banned")
			return
		case guardDenyRateLimited:
			relayError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		case guardDenyUnknown:
			relayError(w, http.StatusForbidden, "unknown source: enroll via heartbeat first")
			return
		}
		next.ServeHTTP(w, req)
	})
}
