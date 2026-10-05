//go:build integration

package store_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/store"
)

const redisImage = "redis:7.4.11-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499"

func newRedis(t *testing.T) *redis.Client {
	t.Helper()
	rc, err := tcredis.Run(t.Context(), redisImage)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Terminate(context.Background()) })
	uri, err := rc.ConnectionString(t.Context())
	require.NoError(t, err)
	opts, err := redis.ParseURL(uri)
	require.NoError(t, err)
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

// xorshift is a small deterministic generator: the test replays the same events every run.
type xorshift struct{ s uint64 }

// IntN returns a number in [0, n).
func (x *xorshift) IntN(n int) int {
	x.s ^= x.s << 13
	x.s ^= x.s >> 7
	x.s ^= x.s << 17
	return int(x.s % uint64(n)) //nolint:gosec // n is a small positive bound
}

type entry struct {
	op features.WindowOp
	r  features.Replay
}

// expected computes a query the way backtests do (features.MemoryWindows), over entries and,
// for a dry run, the event itself.
func expected(entries []entry, q features.WindowQuery, at time.Time, currency string) features.WindowValue {
	m := features.NewMemoryWindows()
	for _, e := range entries {
		m.Record("k", "e", e.op, e.r)
	}
	return m.Value("k", "e", q.Fn, q.Window, at, currency)
}

// The Redis windows give the backtest's results, event after event, for every function: with
// late events, unchecked filters, other currencies and dry runs, and after their indexes are
// rebuilt. A busy entity's reads do not scan its whole window.
func TestWindowsMatchBacktest(t *testing.T) {
	rdb := newRedis(t)
	fs := store.NewFeatureStore(rdb, []byte("windows-test"), 5*time.Second)
	ctx := tenancy.WithScope(t.Context(), tenancy.Scope{TenantID: uuid.New(), EnvironmentID: uuid.New()})
	queries := []features.WindowQuery{
		{Fn: "count", Window: time.Hour}, {Fn: "count", Window: 24 * time.Hour},
		{Fn: "sum", Window: 24 * time.Hour}, {Fn: "sum", Window: 7 * time.Minute},
		{Fn: "exists", Window: 10 * time.Minute}, {Fn: "daily_avg", Window: 24 * time.Hour},
		{Fn: "distinct_count", Window: 24 * time.Hour}, {Fn: "distinct_count", Window: 3 * time.Hour},
		{Fn: "distinct_count_once", Window: 24 * time.Hour}, {Fn: "max_distinct_per_value", Window: 24 * time.Hour},
		// Days whose total reached a threshold, by UTC days, Kigali days and Kigali days starting
		// at 06:00 (ADR-0007).
		{Fn: "days_over:100000000:0", Window: 48 * time.Hour}, {Fn: "days_over:100000000:7200000", Window: 48 * time.Hour},
		{Fn: "days_over:150000000:-14400000", Window: 30 * time.Hour},
	}
	rng := &xorshift{s: 2026}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var stored []entry
	check := func(t *testing.T, i int, got []features.WindowValue, entries []entry, at time.Time, currency string) {
		t.Helper()
		for j, q := range queries {
			want := expected(entries, q, at, currency)
			if !assert.Equal(t, want.OK, got[j].OK, "event %d, %s over %s: known", i, q.Fn, q.Window) {
				continue
			}
			assert.InDelta(t, want.Value, got[j].Value, 1e-6, "event %d, %s over %s", i, q.Fn, q.Window)
		}
	}
	window := func(e entry, record bool) features.WindowOp {
		op := e.op
		op.Key, op.EntityKind, op.EntityID, op.Retention, op.Queries = "k", "beneficiary", "ben-1", 48*time.Hour, queries
		op.Record = record
		return op
	}

	for i := range 1500 {
		at := base.Add(time.Duration(i) * 90 * time.Second)
		if rng.IntN(20) == 0 {
			at = at.Add(-time.Duration(120+rng.IntN(7200)) * time.Second) // a late event
		}
		currency := "NGN"
		if rng.IntN(10) == 0 {
			currency = "USD"
		}
		e := entry{
			op: features.WindowOp{
				Unknown: rng.IntN(200) == 0,
				Field:   fmt.Sprintf("sender-%d", rng.IntN(60)),
				Per:     fmt.Sprintf("%d", 100*rng.IntN(4)),
			},
			r: features.Replay{At: at, Amount: int64(rng.IntN(500_000)), Currency: currency},
		}
		write := rng.IntN(10) != 0
		got, _, ok := fs.Windows(ctx, []features.WindowOp{window(e, true)},
			features.WindowEvent{ID: fmt.Sprintf("evt-%d", i), At: at, Amount: e.r.Amount, Currency: currency, UserID: "u", Hour: -1}, write)
		require.True(t, ok)
		entries := append(append([]entry{}, stored...), e)
		check(t, i, got[0], entries, at, currency)
		if write {
			stored = entries
		}
		if t.Failed() {
			return
		}
	}

	t.Run("indexes are rebuilt from the window", func(t *testing.T) {
		var aux []string
		iter := rdb.Scan(ctx, 0, "*:k:*", 0).Iterator()
		for iter.Next(ctx) {
			if k := iter.Val(); strings.HasSuffix(k, ":u") || strings.HasSuffix(k, ":f") || strings.HasSuffix(k, ":b") {
				aux = append(aux, k)
			}
		}
		require.NoError(t, iter.Err())
		require.GreaterOrEqual(t, len(aux), 2, "an empty index is not kept")
		require.NoError(t, rdb.Del(ctx, aux...).Err())
		at := stored[len(stored)-1].r.At
		e := entry{op: features.WindowOp{Field: "sender-new"}, r: features.Replay{At: at, Amount: 1, Currency: "NGN"}}
		got, _, ok := fs.Windows(ctx, []features.WindowOp{window(e, true)},
			features.WindowEvent{ID: "evt-dry", At: at, Amount: 1, Currency: "NGN", UserID: "u", Hour: -1}, false)
		require.True(t, ok)
		check(t, -1, got[0], append(append([]entry{}, stored...), e), at, "NGN")
	})

	t.Run("an event that only reads a window does not size its buckets", func(t *testing.T) {
		// An event outside a window's filter reads it with no retention. Read first, on a window
		// whose indexes do not exist yet, it must not leave 1-second buckets behind for a 30-day
		// window (millions of buckets per read).
		month := 30 * 24 * time.Hour
		mixed := []features.WindowQuery{{Fn: "count", Window: month}, {Fn: "sum", Window: month}}
		op := func(record bool) features.WindowOp {
			o := features.WindowOp{Key: "mixed", EntityKind: "account", EntityID: "acc-m", Queries: mixed}
			if record {
				o.Record, o.Retention = true, month
			}
			return o
		}
		send := func(id string, at time.Time, record bool) []features.WindowValue {
			t.Helper()
			got, _, ok := fs.Windows(ctx, []features.WindowOp{op(record)},
				features.WindowEvent{ID: id, At: at, Amount: 7, Currency: "NGN", UserID: "u", Hour: -1}, true)
			require.True(t, ok)
			return got[0]
		}
		n := 0
		at := base
		for i := range 300 {
			at = base.Add(time.Duration(i) * time.Hour)
			send(fmt.Sprintf("m-%d", i), at, true)
			n++
		}
		var aux []string
		iter := rdb.Scan(ctx, 0, "*:mixed:*", 0).Iterator()
		for iter.Next(ctx) {
			aux = append(aux, iter.Val())
		}
		require.NoError(t, iter.Err())
		require.NotEmpty(t, aux)
		require.NoError(t, rdb.Del(ctx, aux...).Err(), "as written before the indexes existed")

		for i := range 50 {
			at = at.Add(time.Minute)
			record := i%2 == 1
			if record {
				n++
			}
			got := send(fmt.Sprintf("m-x-%d", i), at, record)
			inWindow := n
			if cut := at.Add(-month); base.Before(cut) {
				inWindow -= int(cut.Sub(base)/time.Hour) + 1
			}
			assert.Equal(t, features.WindowValue{Value: float64(inWindow), OK: true}, got[0], "event %d: count", i)
			assert.Equal(t, features.WindowValue{Value: float64(7 * inWindow), OK: true}, got[1], "event %d: sum", i)
		}
		var keys []string
		iter = rdb.Scan(ctx, 0, "*:mixed:b", 0).Iterator()
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		require.NoError(t, iter.Err())
		require.Len(t, keys, 1)
		size, err := rdb.HGet(ctx, keys[0], "B").Int64()
		require.NoError(t, err)
		assert.Equal(t, month.Milliseconds()/1024, size, "buckets follow the retention writes give")
	})

	t.Run("a slightly late event never counts fewer identifiers", func(t *testing.T) {
		q := []features.WindowQuery{{Fn: "distinct_count", Window: time.Hour}}
		send := func(id, field string, at time.Time) features.WindowValue {
			t.Helper()
			got, _, ok := fs.Windows(ctx, []features.WindowOp{{
				Key: "skew", EntityKind: "ip", EntityID: "nat", Retention: 2 * time.Hour, Record: true, Field: field, Queries: q,
			}}, features.WindowEvent{ID: id, At: at, UserID: "u", Hour: -1}, true)
			require.True(t, ok)
			return got[0][0]
		}
		at := base.Add(10 * time.Hour)
		send("s-1", "acc-1", at)
		send("s-2", "acc-2", at.Add(10*time.Second))
		send("s-3", "acc-3", at.Add(20*time.Second))
		// Arrives last, 15 seconds older than the newest: acc-1, acc-2 and itself (acc-4) were
		// seen by then; acc-3 only after it.
		got := send("s-4", "acc-4", at.Add(5*time.Second))
		assert.True(t, got.OK)
		assert.GreaterOrEqual(t, got.Value, 3.0, "never fewer than the backtest counts")
		assert.LessOrEqual(t, got.Value, 4.0, "at most the identifiers first seen in the minute after it")
		// Two minutes late: counted exactly.
		got = send("s-5", "acc-5", at.Add(-2*time.Minute))
		assert.Equal(t, features.WindowValue{Value: 1, OK: true}, got)
	})

	t.Run("a busy entity's reads stay cheap", func(t *testing.T) {
		hot := func(i int) features.WindowOp {
			return features.WindowOp{
				Key: "hot", EntityKind: "beneficiary", EntityID: "biller", Retention: 7 * 24 * time.Hour, Record: true,
				Field: fmt.Sprintf("payer-%d", i%5000), Queries: []features.WindowQuery{
					{Fn: "count", Window: 24 * time.Hour}, {Fn: "sum", Window: 24 * time.Hour},
					{Fn: "distinct_count", Window: 24 * time.Hour},
				},
			}
		}
		ops := make([]features.WindowOp, 0, 100)
		var at time.Time
		for i := range 20_000 {
			at = base.Add(time.Duration(i) * 4 * time.Second) // 20,000 payments in under a day
			ops = append(ops[:0], hot(i))
			_, _, ok := fs.Windows(ctx, ops, features.WindowEvent{ID: fmt.Sprintf("hot-%d", i), At: at, Amount: 100, Currency: "NGN", UserID: "u", Hour: -1}, true)
			require.True(t, ok)
		}
		require.NoError(t, rdb.ConfigResetStat(ctx).Err())
		var got [][]features.WindowValue
		for i := range 200 {
			at = at.Add(time.Second)
			var ok bool
			got, _, ok = fs.Windows(ctx, []features.WindowOp{hot(20_000 + i)},
				features.WindowEvent{ID: fmt.Sprintf("hot-x-%d", i), At: at, Amount: 100, Currency: "NGN", UserID: "u", Hour: -1}, true)
			require.True(t, ok)
		}
		assert.Equal(t, features.WindowValue{Value: 20_000, OK: true}, got[0][0], "the cap keeps the newest 20,000")
		assert.Equal(t, features.WindowValue{Value: 5000, OK: true}, got[0][2])
		stats, err := rdb.Info(ctx, "commandstats").Result()
		require.NoError(t, err)
		var calls, usec int64
		for line := range strings.SplitSeq(stats, "\n") {
			if strings.HasPrefix(line, "cmdstat_evalsha:") {
				_, err := fmt.Sscanf(strings.TrimSpace(line), "cmdstat_evalsha:calls=%d,usec=%d", &calls, &usec)
				require.NoError(t, err)
			}
		}
		require.Positive(t, calls)
		t.Logf("busy entity (20,000 entries): %d µs of Redis time per window call", usec/calls)
		assert.Less(t, usec/calls, int64(2000), "a read must not scan 20,000 entries")
	})
}
