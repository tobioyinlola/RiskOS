//go:build integration

package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
)

func debitOf(t *testing.T, user string, amount int64) features.Event {
	t.Helper()
	e, err := features.ParseEvent([]byte(fmt.Sprintf(`{"type":"transaction","occurred_at":"2026-10-01T12:00:00Z",
 "actor":{"user_id":%q,"account_id":"a","device_id":"d"},"context":{"country":"NG"},
 "data":{"transaction_id":"t","amount":%d,"currency":"NGN","direction":"debit","beneficiary_id":"b"}}`, user, amount)))
	require.NoError(t, err)
	return e
}

// One user's amount baseline takes at most the configured number of observations an hour, and
// another user's baseline is unaffected.
func TestBaselineUpdatesAreRateCapped(t *testing.T) {
	rdb := newRedis(t)
	ctx := tenancy.WithScope(t.Context(), tenancy.Scope{TenantID: uuid.New(), EnvironmentID: uuid.New()})

	capped := store.NewFeatureStore(rdb, []byte("baseline-test"), 5*time.Second).WithBaselineCap(5)
	for range 40 {
		require.NoError(t, capped.RecordAmount(ctx, debitOf(t, "usr-a", 100_000)))
	}
	require.NoError(t, capped.RecordAmount(ctx, debitOf(t, "usr-b", 100_000)))

	count := func(user string) int64 {
		v := capped.Observe(ctx, debitOf(t, user, 100_000), false)
		require.Empty(t, v.Degraded)
		return v.AmountCount
	}
	assert.EqualValues(t, 5, count("usr-a"), "a burst teaches the baseline at most the cap an hour")
	assert.EqualValues(t, 1, count("usr-b"), "other users have their own allowance")

	uncapped := store.NewFeatureStore(rdb, []byte("baseline-test"), 5*time.Second)
	for range 10 {
		require.NoError(t, uncapped.RecordAmount(ctx, debitOf(t, "usr-c", 100_000)))
	}
	assert.EqualValues(t, 10, uncapped.Observe(ctx, debitOf(t, "usr-c", 100_000), false).AmountCount, "zero removes the cap")
}
