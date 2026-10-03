package features_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
)

// The in-memory windows of a backtest follow the Redis windows' rules (store/windows.go).
func TestMemoryWindows(t *testing.T) {
	m := features.NewMemoryWindows()
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	add := func(at time.Time, amount int64, currency, field, per string) {
		m.Record("k", "acc", features.WindowOp{Field: field, Per: per}, features.Replay{At: at, Amount: amount, Currency: currency})
	}
	add(t0, 100, "NGN", "b1", "500")
	add(t0.Add(time.Minute), 200, "NGN", "b2", "500")
	add(t0.Add(2*time.Minute), 300, "USD", "b1", "700")
	at := t0.Add(2 * time.Minute)
	value := func(fn string, window time.Duration) features.WindowValue {
		return m.Value("k", "acc", fn, window, at, "NGN")
	}
	assert.Equal(t, features.WindowValue{Value: 3, OK: true}, value("count", time.Hour))
	assert.Equal(t, features.WindowValue{Value: 300, OK: true}, value("sum", time.Hour), "only naira amounts")
	assert.Equal(t, features.WindowValue{Value: 2, OK: true}, value("distinct_count", time.Hour))
	assert.Equal(t, features.WindowValue{Value: 1, OK: true}, value("distinct_count_once", time.Hour), "b1 twice, b2 once")
	assert.Equal(t, features.WindowValue{Value: 2, OK: true}, value("max_distinct_per_value", time.Hour), "two payees at 500")
	assert.Equal(t, features.WindowValue{Value: 2, OK: true}, value("count", time.Minute), "the window includes its start")
	assert.Equal(t, features.WindowValue{Value: 1, OK: true}, value("exists", time.Second))
	assert.Equal(t, features.WindowValue{Value: 300 / (1.0 / 24), OK: true}, value("daily_avg", time.Hour))
	assert.Equal(t, features.WindowValue{Value: 0, OK: true}, m.Value("k", "other", "count", time.Hour, at, "NGN"), "per entity")
	assert.Equal(t, features.WindowValue{Value: 1, OK: true}, m.Value("k", "acc", "count", time.Hour, t0, "NGN"), "later events are not counted")

	m.Record("k", "acc", features.WindowOp{Unknown: true}, features.Replay{At: at})
	assert.False(t, value("count", time.Hour).OK, "an event whose filter was not checked makes the window unavailable")
	assert.True(t, m.Value("k", "acc", "count", time.Second, t0, "NGN").OK, "outside its window it does not")
}
