package features_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
)

func parse(t *testing.T, normalized string) features.Event {
	t.Helper()
	e, err := features.ParseEvent([]byte(normalized))
	require.NoError(t, err)
	return e
}

const debit = `{"type":"transaction","occurred_at":"2026-09-30T12:00:00Z",
 "actor":{"user_id":"u","account_id":"a","device_id":"d"},"context":{"country":"NG"},
 "data":{"transaction_id":"t","amount":5000000,"currency":"NGN","direction":"debit","beneficiary_id":"b"}}`

func codes(signals []domain.Signal) []string {
	out := []string{}
	for _, s := range signals {
		out = append(out, s.Code)
	}
	return out
}

func TestSpecScenario(t *testing.T) {
	// The spec's example (§8): new device, new beneficiary, unusual amount, velocity, geo.
	e := parse(t, debit)
	v := features.Values{
		NewDevice: true, NewBeneficiary: true, NewCountry: true,
		AmountCount: 10, AmountMean: 200000, AmountStd: 50000,
		AccountTxn1m: 4,
	}
	signals, degraded, _ := features.Signals(e, v, domain.DefaultSettings())
	assert.Empty(t, degraded)
	assert.ElementsMatch(t, []string{"new_device", "new_beneficiary", "amount_anomaly", "velocity_anomaly", "geo_anomaly"}, codes(signals))
	score := domain.Score(signals)
	assert.Equal(t, 84, score, "20+15+24+15+10, exactly the spec's example")
	assert.Equal(t, domain.LevelHigh, domain.LevelForScore(score, domain.DefaultThresholds()), "84 is High on the default 20/50/70/90 bands")
}

func TestColdStart(t *testing.T) {
	e := parse(t, debit)
	v := features.Values{NewUser: true, NewDevice: true, NewBeneficiary: true, NewCountry: true}
	signals, degraded, _ := features.Signals(e, v, domain.DefaultSettings())
	assert.Empty(t, degraded)
	assert.Equal(t, []string{"new_user"}, codes(signals), "no anomaly signals without history")
}

func TestAmountAnomalyNeedsHistoryAndMagnitude(t *testing.T) {
	e := parse(t, debit) // 5,000,000
	cases := []struct {
		name string
		v    features.Values
		want bool
	}{
		{"too little history", features.Values{AmountCount: 4, AmountMean: 100000, AmountStd: 1}, false},
		{"within 3 sigma", features.Values{AmountCount: 10, AmountMean: 3000000, AmountStd: 1000000}, false},
		{"beyond 3 sigma but under 2x mean", features.Values{AmountCount: 10, AmountMean: 4000000, AmountStd: 100}, false},
		{"far outside", features.Values{AmountCount: 10, AmountMean: 100000, AmountStd: 50000}, true},
	}
	for _, c := range cases {
		signals, _, _ := features.Signals(e, c.v, domain.DefaultSettings())
		assert.Equal(t, c.want, assert.ObjectsAreEqual(true, contains(codes(signals), "amount_anomaly")), c.name)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestAccountTakeoverSignals(t *testing.T) {
	login := parse(t, `{"type":"login","occurred_at":"2026-09-30T12:00:00Z","actor":{"user_id":"u","device_id":"d"},"context":{},"data":{"outcome":"failure","method":"password"}}`)
	signals, _, _ := features.Signals(login, features.Values{FailedLogins15m: 5, DeviceUsers: 7}, domain.DefaultSettings())
	assert.ElementsMatch(t, []string{"failed_login_burst", "shared_device"}, codes(signals))

	e := parse(t, debit)
	signals, _, _ = features.Signals(e, features.Values{RecentCredentialChange: true, BeneficiarySenders24h: 12}, domain.DefaultSettings())
	assert.ElementsMatch(t, []string{"recent_credential_change", "beneficiary_fan_in"}, codes(signals))
}

func TestDegradedSourcesAreReportedNotGuessed(t *testing.T) {
	e := parse(t, debit)
	v := features.Values{Degraded: []string{domain.SourceHistory, domain.SourceVelocity, domain.SourceNetwork}, NewDevice: true, AccountTxn1m: 99}
	signals, degraded, _ := features.Signals(e, v, domain.DefaultSettings())
	assert.Empty(t, signals, "no signal is computed from an unavailable source")
	assert.ElementsMatch(t, []string{
		"new_user", "new_device", "new_beneficiary", "geo_anomaly", "amount_anomaly", "recent_credential_change",
		"velocity_anomaly", "shared_device", "beneficiary_fan_in",
	}, degraded)

	f := features.Facts(e, v)
	assert.NotContains(t, f, "feature.new_device", "facts of degraded sources are omitted")
	assert.Equal(t, 5000000.0, f["transaction.amount"], "event facts are always present")
}

func TestZeroWeightDisablesASignal(t *testing.T) {
	s := domain.DefaultSettings()
	s.Weights[domain.SignalNewDevice] = 0
	signals, _, _ := features.Signals(parse(t, debit), features.Values{NewDevice: true}, s)
	assert.NotContains(t, codes(signals), "new_device")
}

func TestFacts(t *testing.T) {
	f := features.Facts(parse(t, debit), features.Values{NewBeneficiary: true, AccountTxn1h: 3, AmountCount: 5, AmountMean: 2500000})
	assert.Equal(t, "transaction", f["event.type"])
	assert.Equal(t, "NGN", f["transaction.currency"])
	assert.Equal(t, true, f["feature.new_beneficiary"])
	assert.Equal(t, 3.0, f["feature.account_txn_1h"])
	assert.Equal(t, 2.0, f["feature.amount_to_mean_ratio"])
	assert.NotContains(t, f, "login.outcome")
}

func TestWarmingHoldsBackHistorySignals(t *testing.T) {
	e := parse(t, debit)
	v := features.Values{NewDevice: true, NewCountry: true, RecentCredentialChange: true, Warming: true}
	signals, degraded, warming := features.Signals(e, v, domain.DefaultSettings())
	assert.Empty(t, degraded)
	assert.ElementsMatch(t, []string{"new_device", "new_beneficiary", "geo_anomaly", "amount_anomaly", "new_user"}, warming)
	codes := make([]string, 0, len(signals))
	for _, s := range signals {
		codes = append(codes, s.Code)
	}
	assert.Equal(t, []string{"recent_credential_change"}, codes,
		"a signal that needs a fresh observation still fires")
}
