package api

import (
	"sync"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

// The check plane is O(fleet): every leaf polls for updates on a jittered
// cycle, and every poll is forwarded up the cascade to this server. At 100k+
// nodes that is hundreds of requests per second, each of which previously did
// a release lookup and a DB upsert. These two structures make the hot path
// read-mostly:
//
//   - releaseLookupCache memoizes "highest release for (product, channel,
//     group)" for a short TTL, so a burst of identical checks costs one DB
//     query, not one per request. Version comparison against the caller's
//     current version stays per-request (cheap, in-memory).
//   - checkWriteThrottle collapses the per-check TouchFromCheck upsert to at
//     most one write per instance per interval, unless the reported version
//     changed. Liveness is preserved because the interval is far below the
//     derived-offline threshold.

const (
	releaseCacheTTL   = 10 * time.Second
	checkWriteMinGap  = 60 * time.Second
	checkThrottleCap  = 200000 // bounded memory; evicted opportunistically
	checkThrottleKeep = 2 * checkWriteMinGap
)

type releaseCacheEntry struct {
	release *types.Release
	at      time.Time
}

type releaseLookupCache struct {
	mu  sync.Mutex
	m   map[string]releaseCacheEntry
	ttl time.Duration
	now func() time.Time
}

func newReleaseLookupCache() *releaseLookupCache {
	return &releaseLookupCache{m: map[string]releaseCacheEntry{}, ttl: releaseCacheTTL, now: time.Now}
}

// getOrLoad returns the cached highest release for key, or loads and caches it.
// A nil release (no release for the group) is cached too, so repeated checks
// for a product with no release don't hammer the DB. The loader error is
// returned without caching.
func (c *releaseLookupCache) getOrLoad(key string, load func() (*types.Release, error)) (*types.Release, error) {
	now := c.now()
	c.mu.Lock()
	if e, ok := c.m[key]; ok && now.Sub(e.at) < c.ttl {
		c.mu.Unlock()
		return e.release, nil
	}
	c.mu.Unlock()

	rel, err := load()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.m[key] = releaseCacheEntry{release: rel, at: now}
	c.mu.Unlock()
	return rel, nil
}

type checkThrottleEntry struct {
	at      time.Time
	version string
}

type checkWriteThrottle struct {
	mu     sync.Mutex
	m      map[string]checkThrottleEntry
	minGap time.Duration
	now    func() time.Time
}

func newCheckWriteThrottle() *checkWriteThrottle {
	return &checkWriteThrottle{m: map[string]checkThrottleEntry{}, minGap: checkWriteMinGap, now: time.Now}
}

// shouldWrite reports whether a TouchFromCheck write should happen now for this
// instance. It returns true (and records the decision) when the instance is
// unseen, its reported version changed, or the throttle interval has elapsed —
// so steady-state checks between writes touch nothing, while first contact and
// real version changes always persist promptly.
func (t *checkWriteThrottle) shouldWrite(instanceID, version string) bool {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	if e, ok := t.m[instanceID]; ok {
		if e.version == version && now.Sub(e.at) < t.minGap {
			return false
		}
	} else if len(t.m) >= checkThrottleCap {
		t.evictLocked(now)
	}
	t.m[instanceID] = checkThrottleEntry{at: now, version: version}
	return true
}

// evictLocked drops entries idle beyond the retention window; callers hold mu.
func (t *checkWriteThrottle) evictLocked(now time.Time) {
	for id, e := range t.m {
		if now.Sub(e.at) > checkThrottleKeep {
			delete(t.m, id)
		}
	}
}
