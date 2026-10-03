package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/platform/tenancy"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/packs/ng"
)

func ngRule(t *testing.T, id string) *pack.Rule {
	t.Helper()
	p, err := pack.Load(ng.Files)
	require.NoError(t, err)
	for i := range p.Rules {
		if p.Rules[i].ID == id {
			return &p.Rules[i]
		}
	}
	t.Fatalf("no %s", id)
	return nil
}

func TestNeedsApproval(t *testing.T) {
	prod := tenancy.Scope{TenantID: uuid.New(), EnvironmentKind: tenancy.EnvironmentProduction}
	test := tenancy.Scope{TenantID: uuid.New(), EnvironmentKind: tenancy.EnvironmentTest}
	reg, vel := ngRule(t, "REG-01"), ngRule(t, "VEL-01")
	live := RuleConfig{Mode: ModeLive, Settings: map[string]float64{}}
	off := RuleConfig{Mode: ModeOff, Settings: map[string]float64{}}
	stricter := ConfigInput{Mode: ModeLive, Settings: map[string]float64{"new_device_cap": 1_000_000}}
	for name, tc := range map[string]struct {
		scope tenancy.Scope
		def   *pack.Rule
		cur   RuleConfig
		in    ConfigInput
		want  bool
	}{
		"any settings change to a control in production": {prod, reg, live, stricter, true},
		"a control to shadow in production":              {prod, reg, live, ConfigInput{Mode: ModeShadow, Settings: map[string]float64{}}, true},
		"a control switched back on in production":       {prod, reg, off, ConfigInput{Mode: ModeLive, Settings: map[string]float64{}}, false},
		"a control in the test environment":              {test, reg, live, stricter, false},
		"a risk-scoring rule in production":              {prod, vel, live, ConfigInput{Mode: ModeOff, Settings: map[string]float64{}}, false},
	} {
		assert.Equal(t, tc.want, needsApproval(tc.scope, tc.def, tc.cur, tc.in), name)
	}
}

func TestNormalize(t *testing.T) {
	reg := ngRule(t, "REG-01")
	in, err := normalize(reg, ConfigInput{Mode: ModeLive, Reason: " stricter ", Settings: map[string]float64{
		"new_device_cap": 1_000_000, "new_device_window": 86_400, // the second is the default
	}})
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"new_device_cap": 1_000_000}, in.Settings, "defaults are not stored as overrides")
	assert.Equal(t, "stricter", in.Reason)
	for name, settings := range map[string]map[string]float64{
		"above the legal maximum": {"new_device_cap": 2_000_001},
		"below the minimum":       {"new_device_window": 3600},
		"a fraction of a kobo":    {"new_device_cap": 10.5},
		"not a setting":           {"weight": 10},
	} {
		_, err := normalize(reg, ConfigInput{Mode: ModeLive, Reason: "x", Settings: settings})
		assert.ErrorIs(t, err, ErrInvalidInput, name)
	}
	_, err = normalize(reg, ConfigInput{Mode: "on", Reason: "x"})
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestToUSD(t *testing.T) {
	rates := map[string]float64{"NGN": 0.00065, "JPY": 0.0067}
	usd, ok := ToUSD(interp.Money{Minor: 1_600_000_000, Currency: "NGN"}, rates)
	require.True(t, ok)
	assert.Equal(t, interp.Money{Minor: 1_040_000, Currency: "USD"}, usd, "₦16,000,000 is $10,400")
	usd, ok = ToUSD(interp.Money{Minor: 150_000, Currency: "JPY"}, rates)
	require.True(t, ok)
	assert.Equal(t, interp.Money{Minor: 100_500, Currency: "USD"}, usd, "yen have no minor units: ¥150,000 is $1,005")
	usd, ok = ToUSD(interp.Money{Minor: 1234, Currency: "USD"}, nil)
	require.True(t, ok)
	assert.Equal(t, interp.Money{Minor: 1234, Currency: "USD"}, usd)
	_, ok = ToUSD(interp.Money{Minor: 100, Currency: "GHS"}, rates)
	assert.False(t, ok, "no rate, no conversion")
}
