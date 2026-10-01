package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/domain"
)

func TestDecide(t *testing.T) {
	defaults := domain.DefaultSettings()
	newDevice := []domain.Signal{{Code: domain.SignalNewDevice, Weight: 20}}
	velocity := []domain.Signal{{Code: domain.SignalVelocityAnomaly, Weight: 15}}
	tests := []struct {
		name    string
		in      domain.PolicyInput
		want    domain.Decision
		reasons []string
	}{
		{"minimal allows", domain.PolicyInput{EventType: "transaction", Level: domain.LevelMinimal, Signals: newDevice}, domain.DecisionAllow, nil},
		{"low without strengthenable allows", domain.PolicyInput{EventType: "transaction", Level: domain.LevelLow, Signals: velocity}, domain.DecisionAllow, nil},
		{"low with new device steps up", domain.PolicyInput{EventType: "transaction", Level: domain.LevelLow, Signals: newDevice}, domain.DecisionStepUp, []string{"strengthenable_signal"}},
		{"elevated reviews", domain.PolicyInput{EventType: "transaction", Level: domain.LevelElevated}, domain.DecisionReview, nil},
		{"high steps up a transaction", domain.PolicyInput{EventType: "transaction", Level: domain.LevelHigh}, domain.DecisionStepUp, nil},
		{"high reviews a profile change", domain.PolicyInput{EventType: "profile_change", Level: domain.LevelHigh}, domain.DecisionReview, []string{"step_up_not_available"}},
		{"critical reviews, never blocks", domain.PolicyInput{EventType: "transaction", Level: domain.LevelCritical}, domain.DecisionReview, nil},
		{"degraded transaction fails closed", domain.PolicyInput{EventType: "transaction", Level: domain.LevelMinimal, Degraded: []string{"velocity"}}, domain.DecisionReview, []string{"fail_closed"}},
		{"degraded login fails open", domain.PolicyInput{EventType: "login", Level: domain.LevelMinimal, Degraded: []string{"velocity"}}, domain.DecisionAllow, nil},
		{"degraded high-value floor overrides fail-open", domain.PolicyInput{
			EventType: "transaction", Level: domain.LevelLow, Degraded: []string{"history"}, HighValue: true,
			Settings: domain.Settings{FailPolicies: map[string]domain.FailPolicy{"transaction": domain.FailOpen}},
		}, domain.DecisionReview, []string{"degraded_high_value_floor"}},
		{"high value without degradation is scored normally", domain.PolicyInput{EventType: "transaction", Level: domain.LevelLow, HighValue: true}, domain.DecisionAllow, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.in.Settings.FailPolicies == nil {
				tt.in.Settings = defaults
			}
			got := domain.Decide(tt.in)
			assert.Equal(t, tt.want, got.Decision)
			assert.Equal(t, tt.reasons, got.Reasons)
		})
	}
}

func TestAutoBlock(t *testing.T) {
	in := func(mode domain.AutoBlockMode, l domain.Level) domain.PolicyInput {
		st := domain.DefaultSettings()
		st.AutoBlock = mode
		return domain.PolicyInput{EventType: "transaction", Level: l, Settings: st}
	}
	shadow := domain.Decide(in(domain.AutoBlockShadow, domain.LevelCritical))
	assert.Equal(t, domain.DecisionReview, shadow.Decision, "shadow mode changes no decision")
	assert.Equal(t, []string{"would_block"}, shadow.Reasons)

	on := domain.Decide(in(domain.AutoBlockOn, domain.LevelCritical))
	assert.Equal(t, domain.DecisionBlock, on.Decision)
	assert.Equal(t, []string{"auto_block"}, on.Reasons)

	for _, l := range []domain.Level{domain.LevelMinimal, domain.LevelLow, domain.LevelElevated, domain.LevelHigh} {
		assert.NotEqual(t, domain.DecisionBlock, domain.Decide(in(domain.AutoBlockOn, l)).Decision, "only Critical blocks")
	}
	st := domain.DefaultSettings()
	assert.Equal(t, domain.DecisionReview, st.DecisionFor(domain.LevelCritical))
	st.AutoBlock = domain.AutoBlockOn
	assert.Equal(t, domain.DecisionBlock, st.DecisionFor(domain.LevelCritical))
}

// TestDecideNeverBlocks: without automatic blocking switched on, no input produces BLOCK.
func TestDecideNeverBlocks(t *testing.T) {
	for _, l := range []domain.Level{domain.LevelMinimal, domain.LevelLow, domain.LevelElevated, domain.LevelHigh, domain.LevelCritical} {
		for _, typ := range []string{"login", "transaction", "profile_change"} {
			d := domain.Decide(domain.PolicyInput{EventType: typ, Level: l, Degraded: []string{"x"}, HighValue: true, Settings: domain.DefaultSettings()})
			assert.NotEqual(t, domain.DecisionBlock, d.Decision)
		}
	}
}

func TestSettings(t *testing.T) {
	s := domain.DefaultSettings()
	spec, ok := domain.BuiltinSignal(domain.SignalNewDevice)
	assert.True(t, ok)
	assert.Equal(t, 20, s.Weight(spec))
	s.Weights[domain.SignalNewDevice] = 35
	assert.Equal(t, 35, s.Weight(spec))
	assert.Equal(t, int64(1_000_000), s.HighValueThreshold("NGN"))
	s.HighValueByCurrency["USD"] = 500_000
	assert.Equal(t, int64(500_000), s.HighValueThreshold("USD"))
	assert.Equal(t, domain.FailClosed, s.FailPolicyFor("beneficiary_added"), "unknown types fail closed")
}

func TestSpecWeightsAreTheDefaults(t *testing.T) {
	want := map[string]int{"new_device": 20, "new_beneficiary": 15, "amount_anomaly": 24, "velocity_anomaly": 15, "geo_anomaly": 10}
	for code, w := range want {
		spec, ok := domain.BuiltinSignal(code)
		assert.True(t, ok, code)
		assert.Equal(t, w, spec.DefaultWeight, code)
	}
	for _, s := range domain.BuiltinSignals() {
		assert.LessOrEqual(t, s.DefaultWeight, domain.MaxSignalWeight, s.Code)
	}
}

func TestRaisesAlert(t *testing.T) {
	s := domain.DefaultSettings()
	assert.False(t, s.RaisesAlert(domain.DecisionAllow))
	assert.False(t, s.RaisesAlert(domain.DecisionStepUp), "default threshold is REVIEW")
	assert.True(t, s.RaisesAlert(domain.DecisionReview))
	s.AlertMinDecision = domain.DecisionStepUp
	assert.True(t, s.RaisesAlert(domain.DecisionStepUp))
	assert.False(t, s.RaisesAlert(domain.DecisionAllow))
	assert.True(t, domain.Settings{}.RaisesAlert(domain.DecisionReview), "unset means REVIEW")
	assert.False(t, domain.Settings{}.RaisesAlert(domain.DecisionStepUp))
}
