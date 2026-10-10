package service

import (
	"sync"

	"github.com/google/uuid"
)

// tenantGate bounds how many deliveries of one tenant a worker runs at once, so a tenant whose
// endpoints are slow (each attempt can take the full timeout) cannot occupy every delivery slot and
// starve the others. It is per process: with several workers a tenant can hold up to limit slots in
// each, which still leaves every worker's remaining slots to everyone else. A nil gate allows all.
type tenantGate struct {
	mu    sync.Mutex
	limit int
	inUse map[uuid.UUID]int
}

// newTenantGate returns a gate allowing limit concurrent deliveries per tenant; limit < 1 means
// no limit (nil).
func newTenantGate(limit int) *tenantGate {
	if limit < 1 {
		return nil
	}
	return &tenantGate{limit: limit, inUse: map[uuid.UUID]int{}}
}

// tryAcquire takes a slot for tenant, or reports false when it already holds its share.
func (g *tenantGate) tryAcquire(tenant uuid.UUID) bool {
	if g == nil {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inUse[tenant] >= g.limit {
		return false
	}
	g.inUse[tenant]++
	return true
}

// release returns a slot taken by tryAcquire.
func (g *tenantGate) release(tenant uuid.UUID) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inUse[tenant] <= 1 {
		delete(g.inUse, tenant)
		return
	}
	g.inUse[tenant]--
}
