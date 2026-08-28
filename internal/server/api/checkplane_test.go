package api

import (
	"errors"
	"testing"
	"time"

	"github.com/cyfox-labs/updates-mysoc-ai/pkg/types"
)

func TestReleaseLookupCache_MemoizesWithinTTL(t *testing.T) {
	now := time.Now()
	c := newReleaseLookupCache()
	c.now = func() time.Time { return now }

	calls := 0
	load := func() (*types.Release, error) {
		calls++
		return &types.Release{Version: "1.0.0.1"}, nil
	}

	for i := 0; i < 5; i++ {
		if _, err := c.getOrLoad("k", load); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("within TTL: loaded %d times, want 1", calls)
	}

	// After the TTL elapses, the loader runs again.
	now = now.Add(releaseCacheTTL + time.Second)
	if _, err := c.getOrLoad("k", load); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("after TTL: loaded %d times, want 2", calls)
	}
}

func TestReleaseLookupCache_CachesNilAndPropagatesError(t *testing.T) {
	c := newReleaseLookupCache()
	calls := 0
	nilLoad := func() (*types.Release, error) { calls++; return nil, nil }
	if r, _ := c.getOrLoad("empty", nilLoad); r != nil {
		t.Fatalf("expected nil release")
	}
	if r, _ := c.getOrLoad("empty", nilLoad); r != nil {
		t.Fatalf("expected nil release (cached)")
	}
	if calls != 1 {
		t.Fatalf("nil result should be cached: loaded %d times, want 1", calls)
	}

	// Errors are not cached and are surfaced.
	boom := errors.New("db down")
	_, err := c.getOrLoad("err", func() (*types.Release, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("expected error surfaced, got %v", err)
	}
}

func TestCheckWriteThrottle_FirstVersionChangeAndInterval(t *testing.T) {
	now := time.Now()
	tr := newCheckWriteThrottle()
	tr.now = func() time.Time { return now }

	// First contact always writes.
	if !tr.shouldWrite("i-1", "1.0.0.1") {
		t.Fatal("first check should write")
	}
	// Same version within the interval is throttled.
	if tr.shouldWrite("i-1", "1.0.0.1") {
		t.Fatal("repeat within interval should not write")
	}
	// A version change writes immediately, regardless of interval.
	if !tr.shouldWrite("i-1", "1.0.0.2") {
		t.Fatal("version change should write")
	}
	// After the interval elapses, an unchanged version writes again (liveness).
	now = now.Add(checkWriteMinGap + time.Second)
	if !tr.shouldWrite("i-1", "1.0.0.2") {
		t.Fatal("after interval should write")
	}
}
