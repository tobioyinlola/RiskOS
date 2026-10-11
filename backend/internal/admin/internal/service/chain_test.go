package service_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/admin/internal/service"
)

func entry() service.ChainEntry {
	tenant := uuid.MustParse("11111111-1111-7111-8111-111111111111")
	return service.ChainEntry{
		Seq: 7, ID: uuid.MustParse("22222222-2222-7222-8222-222222222222"),
		OccurredAt: time.Date(2026, 10, 10, 9, 0, 0, 123456000, time.UTC),
		Actor:      "stf_1", Action: "staff.signed_in", TenantID: &tenant, ResourceID: "stf_1", Outcome: "success",
		MetadataText: `{"ip":"203.0.113.9"}`, PrevHash: service.GenesisHash(),
	}
}

func TestChainHashCoversEveryField(t *testing.T) {
	base := service.ComputeChainHash(entry())
	assert.Equal(t, base, service.ComputeChainHash(entry()), "deterministic")
	assert.Len(t, base, 32)

	other := uuid.MustParse("33333333-3333-7333-8333-333333333333")
	changes := map[string]func(*service.ChainEntry){
		"seq":       func(e *service.ChainEntry) { e.Seq++ },
		"id":        func(e *service.ChainEntry) { e.ID = other },
		"time":      func(e *service.ChainEntry) { e.OccurredAt = e.OccurredAt.Add(time.Microsecond) },
		"actor":     func(e *service.ChainEntry) { e.Actor = "stf_2" },
		"action":    func(e *service.ChainEntry) { e.Action = "staff.signed_out" },
		"tenant":    func(e *service.ChainEntry) { e.TenantID = &other },
		"no tenant": func(e *service.ChainEntry) { e.TenantID = nil },
		"resource":  func(e *service.ChainEntry) { e.ResourceID = "stf_2" },
		"outcome":   func(e *service.ChainEntry) { e.Outcome = "failure" },
		"metadata":  func(e *service.ChainEntry) { e.MetadataText = `{}` },
		"prev":      func(e *service.ChainEntry) { e.PrevHash = service.ComputeChainHash(entry()) },
	}
	for name, change := range changes {
		e := entry()
		change(&e)
		assert.NotEqual(t, base, service.ComputeChainHash(e), "%s is covered", name)
	}
}

func TestChainHashResistsFieldBoundaryShifts(t *testing.T) {
	a, b := entry(), entry()
	a.Actor, a.Action = "ab", "c"
	b.Actor, b.Action = "a", "bc"
	assert.NotEqual(t, service.ComputeChainHash(a), service.ComputeChainHash(b), "length-prefixed")
}
