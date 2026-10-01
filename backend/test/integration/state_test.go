//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
)

// TestFeatureStoreReset covers reset detection (DISASTER_RECOVERY.md): when Redis loses its
// state, or the hash key changes, history signals are held back and reported as warming instead
// of firing for every returning user.
func TestFeatureStoreReset(t *testing.T) {
	// A Redis database of its own, so flushing it does not disturb other tests.
	rdb := redis.NewClient(&redis.Options{Addr: shared.redisCfg.Addr, Password: shared.redisCfg.Password, DB: 7})
	t.Cleanup(func() { _ = rdb.Close() })
	require.NoError(t, rdb.FlushDB(t.Context()).Err())
	_, err := shared.owner.Exec(t.Context(), `DELETE FROM feature_store_state`)
	require.NoError(t, err)

	s := newStackWith(t, time.Second, generousLimits(), rdb, withHistoryWarmup(time.Hour))
	a := s.createTenant(t, "Reset")
	resetAt := func() *time.Time {
		var at *time.Time
		require.NoError(t, shared.owner.QueryRow(t.Context(), `SELECT reset_at FROM feature_store_state`).Scan(&at))
		return at
	}
	codes := func(d apigen.RiskDecision) []string {
		out := make([]string, 0, len(d.Signals))
		for _, s := range d.Signals {
			out = append(out, s.Code)
		}
		return out
	}
	ingest := func(key, device string) apigen.RiskDecision {
		r := s.ingest(t, a.testKey, actorTxn(key, "u_1", "acc_1", device, 1000))
		require.Equal(t, http.StatusOK, r.status, r.String())
		return decode[apigen.RiskDecision](t, r)
	}

	require.NoError(t, s.risk.CheckFeatureState(t.Context()))
	assert.Nil(t, resetAt(), "a first start is not a reset")
	ingest("first", "dev_1")
	d := ingest("second", "dev_2")
	assert.Contains(t, codes(d), "new_device")
	assert.Empty(t, d.WarmingSignals)

	// Redis loses its data: the next check records a reset.
	require.NoError(t, rdb.FlushDB(t.Context()).Err())
	require.NoError(t, s.risk.CheckFeatureState(t.Context()))
	first := resetAt()
	require.NotNil(t, first)

	ingest("after-1", "dev_1")
	d = ingest("after-2", "dev_3")
	assert.NotContains(t, codes(d), "new_device", "a returning user's device is not called new")
	assert.Contains(t, d.WarmingSignals, "new_device")
	assert.Empty(t, d.DegradedSignals, "warming is not degradation: fail policies do not apply")

	// Later checks keep the reset time; a changed hash key is another reset.
	require.NoError(t, s.risk.CheckFeatureState(t.Context()))
	assert.Equal(t, first.UnixMicro(), resetAt().UnixMicro())
	_, err = shared.owner.Exec(t.Context(), `UPDATE feature_store_state SET key_fingerprint = 'previous-key'`)
	require.NoError(t, err)
	require.NoError(t, s.risk.CheckFeatureState(t.Context()))
	assert.True(t, resetAt().After(*first))
}
