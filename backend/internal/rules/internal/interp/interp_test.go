package interp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
)

func TestMissingDataIsNeverNoRisk(t *testing.T) {
	defs := loadPack(t)
	// ATO-04 fires on any one integrity finding: one finding is enough even if the others are
	// unknown, but "all clean" needs every check.
	r := interp.Compile(defs["ATO-04"], nil)
	env := func(f interp.Facts) interp.Env { return interp.Env{Kinds: []string{"login"}, Facts: f} }
	assert.Equal(t, interp.StatusFired, interp.Evaluate(r, env(interp.Facts{"device.emulator": true})).Status)
	res := interp.Evaluate(r, env(interp.Facts{"device.rooted": false, "device.jailbroken": false}))
	assert.Equal(t, interp.StatusNotEvaluated, res.Status)
	assert.Equal(t, []string{"fact:device.emulator", "fact:device.hooking_framework"}, res.Missing)
}

func TestAnUntriggeredRuleIsNotApplicable(t *testing.T) {
	defs := loadPack(t)
	res := interp.Evaluate(interp.Compile(defs["REG-01"], nil), interp.Env{Kinds: []string{"login"}})
	assert.Equal(t, interp.StatusNotApplicable, res.Status)
	assert.Empty(t, res.Missing)
}

func TestSettingsOverrideDefaults(t *testing.T) {
	defs := loadPack(t)
	facts := interp.Facts{"txn.amount": ngn(15_000), "device.bound_age": 60.0, "account.age": 400 * day}
	env := interp.Env{Kinds: []string{"transfer_outbound"}, Facts: facts}
	assert.Equal(t, interp.StatusClear, interp.Evaluate(interp.Compile(defs["REG-01"], nil), env).Status)
	assert.Equal(t, interp.StatusFired,
		interp.Evaluate(interp.Compile(defs["REG-01"], map[string]float64{"new_device_cap": 1_000_000}), env).Status,
		"an institution may set a stricter limit")
}

func TestAggregateKeys(t *testing.T) {
	defs := loadPack(t)
	keyOf := func(id, fn, event, by string, overrides map[string]float64) string {
		for _, a := range interp.Compile(defs[id], overrides).Aggregates() {
			if a.Fn == fn && a.Event == event && a.By == by {
				return a.Key
			}
		}
		t.Fatalf("%s has no %s %s by %s", id, fn, event, by)
		return ""
	}
	// SEC-03 and VEL-01 count different events: different windows.
	assert.NotEqual(t, keyOf("SEC-03", "count", "auth_failure", "account", nil), keyOf("VEL-01", "count", "transfer_outbound", "account", nil))
	// Unfiltered outbound transfers by account are one stored window, whatever rule reads them.
	assert.Equal(t, keyOf("VEL-01", "count", "transfer_outbound", "account", nil), keyOf("MUL-01", "sum", "transfer_outbound", "account", nil))
	// A filter's settings are part of the key: a changed setting starts a fresh window.
	assert.NotEqual(t, keyOf("CNP-01", "count", "card_authorization", "card", nil),
		keyOf("CNP-01", "count", "card_authorization", "card", map[string]float64{"tiny": 20_000}))
	for _, a := range interp.Compile(defs["VEL-01"], nil).Aggregates() {
		assert.Equal(t, int64(600), a.Window)
		assert.Equal(t, int64(3600), a.Retention, "kept for the longest window the setting allows")
	}
}

func TestWhereFilters(t *testing.T) {
	defs := loadPack(t)
	var phone interp.AggRef
	for _, a := range interp.Compile(defs["SIM-02"], nil).Aggregates() {
		phone = a
	}
	require.NotNil(t, phone.Where)
	assert.Equal(t, interp.True, interp.Where(phone, interp.Facts{"profile.field": []string{"email", "phone"}}))
	assert.Equal(t, interp.False, interp.Where(phone, interp.Facts{"profile.field": []string{"email"}}))
	assert.Equal(t, interp.Unknown, interp.Where(phone, interp.Facts{}))
}
