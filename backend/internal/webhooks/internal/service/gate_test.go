package service

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestTenantGateSharesSlotsFairly(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	g := newTenantGate(2)

	assert.True(t, g.tryAcquire(a))
	assert.True(t, g.tryAcquire(a))
	assert.False(t, g.tryAcquire(a), "a tenant holds at most its share")
	assert.True(t, g.tryAcquire(b), "and another tenant is not affected")

	g.release(a)
	assert.True(t, g.tryAcquire(a), "a released slot can be taken again")
	g.release(a)
	g.release(a)
	g.release(b)
	assert.Empty(t, g.inUse, "nothing is leaked")
}

func TestTenantGateIsOptional(t *testing.T) {
	g := newTenantGate(0)
	assert.Nil(t, g)
	id := uuid.New()
	for range 100 {
		assert.True(t, g.tryAcquire(id))
	}
	g.release(id) // harmless on a nil gate
}

func TestTenantGateUnderConcurrency(t *testing.T) {
	g := newTenantGate(3)
	id := uuid.New()
	var wg sync.WaitGroup
	var mu sync.Mutex
	held, peak := 0, 0
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !g.tryAcquire(id) {
				return
			}
			mu.Lock()
			held++
			peak = max(peak, held)
			mu.Unlock()
			mu.Lock()
			held--
			mu.Unlock()
			g.release(id)
		}()
	}
	wg.Wait()
	assert.LessOrEqual(t, peak, 3)
	assert.Empty(t, g.inUse)
}
