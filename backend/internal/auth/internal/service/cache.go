package service

import (
	"sync"
	"time"

	"github.com/tobioyinlola/RiskOS/backend/internal/auth/internal/store"
)

// lookupCache keeps recent key lookups (including "no such key") for a short TTL, so the
// synchronous path does not query PostgreSQL on every request. The TTL is the maximum time a
// revoked key keeps working on another instance. Size is bounded: when full, it is cleared, so
// random lookup values cannot grow memory without limit.
type lookupCache struct {
	ttl time.Duration
	max int

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	res      *store.LookupResult
	storedAt time.Time
}

func newLookupCache(ttl time.Duration, maxEntries int) *lookupCache {
	return &lookupCache{ttl: ttl, max: maxEntries, entries: make(map[string]cacheEntry)}
}

func (c *lookupCache) get(lookup string, now time.Time) (*store.LookupResult, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[lookup]
	if !ok || now.Sub(e.storedAt) >= c.ttl {
		return nil, false
	}
	return e.res, true
}

func (c *lookupCache) put(lookup string, res *store.LookupResult, now time.Time) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		clear(c.entries)
	}
	c.entries[lookup] = cacheEntry{res: res, storedAt: now}
}

func (c *lookupCache) delete(lookup string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, lookup)
}
