package domain_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/audit/internal/domain"
)

func sample() domain.Record {
	env := uuid.MustParse("01920000-0000-7000-8000-000000000002")
	return domain.Record{
		TenantID:      uuid.MustParse("01920000-0000-7000-8000-000000000001"),
		Seq:           7,
		ID:            uuid.MustParse("01920000-0000-7000-8000-000000000003"),
		EnvironmentID: &env,
		OccurredAt:    time.Date(2026, 9, 30, 12, 0, 0, 123456000, time.UTC),
		ActorType:     "api_key",
		ActorID:       "key_abc",
		Action:        "api_key.created",
		ResourceType:  "api_key",
		ResourceID:    "key_def",
		Outcome:       domain.OutcomeSuccess,
		RequestID:     "req_1",
		Metadata:      []byte(`{"name":"ci"}`),
		PrevHash:      domain.GenesisHash(),
	}
}

func TestComputeHashIsDeterministic(t *testing.T) {
	assert.Equal(t, domain.ComputeHash(sample()), domain.ComputeHash(sample()))
	assert.Len(t, domain.ComputeHash(sample()), 32)
}

// Every field is covered by the hash: changing any one of them changes the result.
func TestComputeHashCoversEveryField(t *testing.T) {
	base := domain.ComputeHash(sample())
	otherID := uuid.MustParse("01920000-0000-7000-8000-0000000000ff")
	mutations := map[string]func(r *domain.Record){
		"tenant":        func(r *domain.Record) { r.TenantID = otherID },
		"seq":           func(r *domain.Record) { r.Seq++ },
		"id":            func(r *domain.Record) { r.ID = otherID },
		"environment":   func(r *domain.Record) { r.EnvironmentID = &otherID },
		"tenant level":  func(r *domain.Record) { r.EnvironmentID = nil },
		"time":          func(r *domain.Record) { r.OccurredAt = r.OccurredAt.Add(time.Microsecond) },
		"actor type":    func(r *domain.Record) { r.ActorType = "system" },
		"actor id":      func(r *domain.Record) { r.ActorID = "key_xyz" },
		"action":        func(r *domain.Record) { r.Action = "api_key.revoked" },
		"resource type": func(r *domain.Record) { r.ResourceType = "tenant" },
		"resource id":   func(r *domain.Record) { r.ResourceID = "key_ghi" },
		"outcome":       func(r *domain.Record) { r.Outcome = domain.OutcomeDenied },
		"request id":    func(r *domain.Record) { r.RequestID = "req_2" },
		"metadata":      func(r *domain.Record) { r.Metadata = []byte(`{"name":"cj"}`) },
		"prev hash":     func(r *domain.Record) { r.PrevHash = bytes.Repeat([]byte{1}, 32) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := sample()
			mutate(&r)
			assert.NotEqual(t, base, domain.ComputeHash(r))
		})
	}
}

// Length prefixes prevent moving bytes between adjacent string fields without changing the hash.
func TestComputeHashResistsFieldBoundaryShifts(t *testing.T) {
	a, b := sample(), sample()
	a.ActorType, a.ActorID = "api_key", "x"
	b.ActorType, b.ActorID = "api_ke", "yx"
	assert.NotEqual(t, domain.ComputeHash(a), domain.ComputeHash(b))
}

func TestComputeHashIgnoresStoredHash(t *testing.T) {
	r := sample()
	h := domain.ComputeHash(r)
	r.Hash = h
	assert.Equal(t, h, domain.ComputeHash(r))
}
