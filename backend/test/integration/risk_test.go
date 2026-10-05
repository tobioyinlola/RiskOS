//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/apigen"
	"github.com/tobioyinlola/RiskOS/backend/internal/platform/ids"
)

// actor identifies who an event is about; tests vary it to build behavioral history.
type actor struct{ user, account, device string }

func (a actor) json() map[string]any {
	return map[string]any{"user_id": a.user, "account_id": a.account, "device_id": a.device}
}

var seq int //nolint:gochecknoglobals // unique event keys across a test run

func nextKey() string {
	seq++
	// Base 36, so keys never contain a digit run that looks like a card number.
	return "k-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.Itoa(seq)
}

// debit returns a transaction event.
func debit(a actor, beneficiary, country, currency string, amount int64) map[string]any {
	key := nextKey()
	return map[string]any{
		"event_id": key, "type": "transaction",
		"occurred_at": time.Now().Add(-time.Second).Format(time.RFC3339Nano),
		"actor":       a.json(),
		"context":     map[string]any{"ip": "203.0.113.7", "country": country},
		"data": map[string]any{
			"transaction_id": "t_" + key, "amount": amount, "currency": currency,
			"direction": "debit", "beneficiary_id": beneficiary, "channel": "bank_transfer",
		},
	}
}

func login(a actor, outcome string) map[string]any {
	return map[string]any{
		"event_id": nextKey(), "type": "login",
		"occurred_at": time.Now().Add(-time.Second).Format(time.RFC3339Nano),
		"actor":       a.json(),
		"context":     map[string]any{"ip": "203.0.113.7", "country": "NG"},
		"data":        map[string]any{"outcome": outcome, "method": "password"},
	}
}

func profileChange(a actor, fields ...string) map[string]any {
	return map[string]any{
		"event_id": nextKey(), "type": "profile_change",
		"occurred_at": time.Now().Add(-time.Second).Format(time.RFC3339Nano),
		"actor":       a.json(),
		"context":     map[string]any{"ip": "203.0.113.7", "country": "NG"},
		"data":        map[string]any{"fields": fields},
	}
}

func (s stack) decide(t *testing.T, key string, event map[string]any) apigen.RiskDecision {
	t.Helper()
	r := s.ingest(t, key, event)
	require.Equal(t, http.StatusOK, r.status, r.String())
	return decode[apigen.RiskDecision](t, r)
}

func signalCodes(d apigen.RiskDecision) []string {
	out := []string{}
	for _, s := range d.Signals {
		out = append(out, s.Code)
	}
	return out
}

// stored reads the reproducibility record of an evaluation as the schema owner.
type stored struct {
	PolicyReasons []string
	RuleVersions  []string
	Shadow        []apigen.Signal
	Features      struct {
		Event json.RawMessage `json:"event"`
		Facts map[string]any  `json:"facts"`
	}
}

func storedEvaluation(t *testing.T, evaluationID string) stored {
	t.Helper()
	id, err := ids.Parse(ids.PrefixEvaluation, evaluationID)
	require.NoError(t, err)
	var out stored
	var shadow, feats []byte
	require.NoError(t, shared.owner.QueryRow(t.Context(),
		`SELECT policy_reasons, rule_versions, shadow_signals, features FROM evaluations WHERE id = $1`, id).
		Scan(&out.PolicyReasons, &out.RuleVersions, &shadow, &feats))
	require.NoError(t, json.Unmarshal(shadow, &out.Shadow))
	require.NoError(t, json.Unmarshal(feats, &out.Features))
	return out
}

func TestSpecScenarioScoresHigh(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Spec scenario")
	withoutCompliance(t, a.id)
	u := actor{"u_spec", "acc_spec", "dev_home"}

	// History: six ordinary transfers from the usual device to the usual beneficiary.
	first := s.decide(t, a.testKey, debit(u, "ben_usual", "NG", "NGN", 200000))
	assert.Equal(t, []string{"new_user"}, signalCodes(first), "cold start: only new_user")
	for range 5 {
		d := s.decide(t, a.testKey, debit(u, "ben_usual", "NG", "NGN", 200000))
		assert.NotContains(t, signalCodes(d), "new_device", "the device is known after the first event")
		assert.NotContains(t, signalCodes(d), "new_beneficiary")
	}

	// The spec's example (§8): new device, new beneficiary, 25x the usual amount, from a new
	// country, in a burst of transfers.
	u.device = "dev_new"
	d := s.decide(t, a.testKey, debit(u, "ben_new", "GH", "NGN", 5000000))
	assert.ElementsMatch(t, []string{"new_device", "new_beneficiary", "amount_anomaly", "velocity_anomaly", "geo_anomaly"}, signalCodes(d))
	assert.Equal(t, 84, d.RiskScore)
	assert.Equal(t, apigen.RiskLevel("high"), d.RiskLevel, "84 is High on the default 20/50/70/90 bands")
	assert.Equal(t, apigen.Decision("step_up"), d.Decision, "high steps up a transaction; BLOCK is never produced")
	assert.Empty(t, d.DegradedSignals)

	rec := storedEvaluation(t, d.EvaluationId)
	assert.Equal(t, true, rec.Features.Facts["feature.new_device"])
	assert.InDelta(t, 25.0, rec.Features.Facts["feature.amount_to_mean_ratio"], 0.001)
	assert.Empty(t, rec.RuleVersions)
	assert.Empty(t, rec.Features.Event, "an ingested event is not duplicated into its evaluation")
}

func TestVelocity(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Velocity")
	u := actor{"u_vel", "acc_vel", "dev_vel"}
	for i := 1; i <= 5; i++ {
		d := s.decide(t, a.testKey, debit(u, "ben_vel", "NG", "NGN", 1000))
		// More than three transactions from an account in a minute.
		assert.Equal(t, i > 3, slices.Contains(signalCodes(d), "velocity_anomaly"), "transaction %d", i)
	}
}

func TestCredentialChangeThenNewDeviceStepsUp(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Takeover")
	u := actor{"u_ato", "acc_ato", "dev_ato"}
	s.decide(t, a.testKey, debit(u, "ben_ato", "NG", "NGN", 5000))

	changed := s.decide(t, a.testKey, profileChange(u, "password"))
	assert.Equal(t, apigen.Decision("allow"), changed.Decision)

	u.device = "dev_attacker"
	d := s.decide(t, a.testKey, debit(u, "ben_ato", "NG", "NGN", 5000))
	assert.ElementsMatch(t, []string{"new_device", "recent_credential_change"}, signalCodes(d))
	assert.Equal(t, apigen.RiskLevel("low"), d.RiskLevel)
	assert.Equal(t, apigen.Decision("step_up"), d.Decision, "low with a signal step-up addresses")
	assert.Equal(t, []string{"strengthenable_signal"}, storedEvaluation(t, d.EvaluationId).PolicyReasons)
}

func TestNetworkAndLoginSignals(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Network")

	t.Run("beneficiary fan-in", func(t *testing.T) {
		var last apigen.RiskDecision
		for i := range 10 {
			last = s.decide(t, a.testKey, debit(actor{fmt.Sprintf("u_fan_%d", i), fmt.Sprintf("acc_fan_%d", i), fmt.Sprintf("dev_fan_%d", i)}, "ben_mule", "NG", "NGN", 5000))
			assert.Equal(t, i == 9, slices.Contains(signalCodes(last), "beneficiary_fan_in"), "sender %d", i+1)
		}
	})
	t.Run("shared device", func(t *testing.T) {
		for i := range 5 {
			d := s.decide(t, a.testKey, login(actor{fmt.Sprintf("u_dev_%d", i), fmt.Sprintf("acc_dev_%d", i), "dev_farm"}, "success"))
			assert.Equal(t, i == 4, slices.Contains(signalCodes(d), "shared_device"), "user %d", i+1)
		}
	})
	t.Run("failed login burst", func(t *testing.T) {
		u := actor{"u_brute", "acc_brute", "dev_brute"}
		for i := 1; i <= 5; i++ {
			d := s.decide(t, a.testKey, login(u, "failure"))
			assert.Equal(t, i == 5, slices.Contains(signalCodes(d), "failed_login_burst"), "attempt %d", i)
		}
	})
	t.Run("identifiers are pseudonymized in redis", func(t *testing.T) {
		keys, err := shared.rdb.Keys(t.Context(), "rk:*").Result()
		require.NoError(t, err)
		require.NotEmpty(t, keys)
		for _, k := range keys {
			assert.NotContains(t, k, "ben_mule")
			assert.NotContains(t, k, "dev_farm")
			assert.NotContains(t, k, "u_brute")
		}
	})
}

func TestDryRunsDoNotLearn(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Dry learning")
	u := actor{"u_dry", "acc_dry", "dev_dry"}
	s.decide(t, a.testKey, debit(u, "ben_dry", "NG", "NGN", 5000))

	u.device = "dev_other"
	for range 2 {
		r := s.call(t, http.MethodPost, "/api/v1/risk/evaluate", a.testKey, debit(u, "ben_dry", "NG", "NGN", 5000))
		require.Equal(t, http.StatusOK, r.status, r.String())
		dry := decode[apigen.RiskDecision](t, r)
		assert.Contains(t, signalCodes(dry), "new_device")
		assert.NotEmpty(t, storedEvaluation(t, dry.EvaluationId).Features.Event, "a dry run records its event with the evaluation")
	}
	assert.Contains(t, signalCodes(s.decide(t, a.testKey, debit(u, "ben_dry", "NG", "NGN", 5000))), "new_device",
		"the dry runs did not record the device")
	assert.NotContains(t, signalCodes(s.decide(t, a.testKey, debit(u, "ben_dry", "NG", "NGN", 5000))), "new_device",
		"the ingested event did")
}

func TestDegradedFeatureStore(t *testing.T) {
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { _ = dead.Close() })
	s := newStackWith(t, time.Second, generousLimits(), dead)
	a := s.createTenant(t, "Degraded")
	u := actor{"u_deg", "acc_deg", "dev_deg"}

	t.Run("transactions fail closed", func(t *testing.T) {
		d := s.decide(t, a.testKey, debit(u, "ben_deg", "NG", "NGN", 5000))
		assert.NotEmpty(t, d.DegradedSignals)
		assert.Equal(t, 0, d.RiskScore)
		assert.Equal(t, apigen.Decision("review"), d.Decision)
		assert.Equal(t, []string{"fail_closed"}, storedEvaluation(t, d.EvaluationId).PolicyReasons)
	})
	t.Run("logins fail open", func(t *testing.T) {
		d := s.decide(t, a.testKey, login(u, "success"))
		assert.NotEmpty(t, d.DegradedSignals)
		assert.Equal(t, apigen.Decision("allow"), d.Decision)
	})
	t.Run("high-value floor holds when failing open", func(t *testing.T) {
		r := s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, map[string]any{
			"weights": map[string]any{}, "high_value_default": 1000000,
			"high_value_by_currency": map[string]any{"USD": 50000},
			"fail_policies":          map[string]any{"transaction": "open", "login": "open"},
		})
		require.Equal(t, http.StatusOK, r.status, r.String())

		assert.Equal(t, apigen.Decision("allow"), s.decide(t, a.testKey, debit(u, "ben_deg", "NG", "NGN", 999999)).Decision)
		big := s.decide(t, a.testKey, debit(u, "ben_deg", "NG", "NGN", 1000000))
		assert.Equal(t, apigen.Decision("review"), big.Decision)
		assert.Equal(t, []string{"degraded_high_value_floor"}, storedEvaluation(t, big.EvaluationId).PolicyReasons)
		assert.Equal(t, apigen.Decision("review"), s.decide(t, a.testKey, debit(u, "ben_deg", "NG", "USD", 50000)).Decision,
			"per-currency threshold")
	})
}

func TestFeatureStoreRecoversFromScriptCacheLoss(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Script flush")
	u := actor{"u_flush", "acc_flush", "dev_flush"}
	s.decide(t, a.testKey, debit(u, "ben_flush", "NG", "NGN", 5000))

	// A Redis restart empties the script cache. At most one evaluation degrades.
	require.NoError(t, shared.rdb.ScriptFlush(t.Context()).Err())
	s.decide(t, a.testKey, debit(u, "ben_flush", "NG", "NGN", 5000))
	d := s.decide(t, a.testKey, debit(u, "ben_flush", "NG", "NGN", 5000))
	assert.Empty(t, d.DegradedSignals)
}

func largeTransferRule() map[string]any {
	return map[string]any{
		"code": "large_ngn_transfer", "name": "Large NGN transfer", "severity": "high", "weight": 30, "mode": "active",
		"condition": map[string]any{"all": []any{
			map[string]any{"field": "transaction.amount", "op": "gte", "value": 1000000},
			map[string]any{"field": "transaction.currency", "op": "eq", "value": "NGN"},
		}},
	}
}

func ruleWeight(d apigen.RiskDecision, code string) (int, bool) {
	for _, s := range d.Signals {
		if s.Code == code {
			return s.Weight, true
		}
	}
	return 0, false
}

func TestRuleLifecycle(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Rules")
	withoutCompliance(t, a.id)
	u := actor{"u_rule", "acc_rule", "dev_rule"}
	big := func() map[string]any { return debit(u, "ben_rule", "NG", "NGN", 2000000) }

	r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, largeTransferRule())
	require.Equal(t, http.StatusCreated, r.status, r.String())
	rule := decode[apigen.Rule](t, r)
	assert.Equal(t, 1, rule.Version)
	assert.True(t, rule.Enabled, "enabled by default")
	path := "/api/v1/rules/" + rule.Id

	d := s.decide(t, a.testKey, big())
	w, ok := ruleWeight(d, "large_ngn_transfer")
	assert.True(t, ok)
	assert.Equal(t, 30, w)
	assert.Equal(t, []string{"large_ngn_transfer@1"}, storedEvaluation(t, d.EvaluationId).RuleVersions)
	_, ok = ruleWeight(s.decide(t, a.testKey, debit(u, "ben_rule", "NG", "NGN", 999999)), "large_ngn_transfer")
	assert.False(t, ok, "below the rule's threshold")

	t.Run("update creates a version", func(t *testing.T) {
		r := s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"weight": 40})
		require.Equal(t, http.StatusOK, r.status, r.String())
		assert.Equal(t, 2, decode[apigen.Rule](t, r).Version)
		w, _ := ruleWeight(s.decide(t, a.testKey, big()), "large_ngn_transfer")
		assert.Equal(t, 40, w, "takes effect immediately in the process that changed it")
	})
	t.Run("rollback restores as a new version", func(t *testing.T) {
		r := s.call(t, http.MethodPost, path+"/rollback", a.testKey, map[string]any{"version": 1})
		require.Equal(t, http.StatusOK, r.status, r.String())
		back := decode[apigen.Rule](t, r)
		assert.Equal(t, 3, back.Version)
		require.NotNil(t, back.RestoredFrom)
		assert.Equal(t, 1, *back.RestoredFrom)
		assert.Equal(t, 30, back.Weight)
		d := s.decide(t, a.testKey, big())
		assert.Equal(t, []string{"large_ngn_transfer@3"}, storedEvaluation(t, d.EvaluationId).RuleVersions)

		versions := decode[apigen.RuleVersionList](t, s.call(t, http.MethodGet, path+"/versions", a.testKey, nil))
		assert.Equal(t, 3, versions.CurrentVersion)
		assert.Len(t, versions.Data, 3)
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodPost, path+"/rollback", a.testKey, map[string]any{"version": 9}).status,
			"the rule exists; the version asked for does not")
	})
	t.Run("shadow rules are recorded, not scored", func(t *testing.T) {
		r := s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"mode": "shadow"})
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := s.decide(t, a.testKey, big())
		_, ok := ruleWeight(d, "large_ngn_transfer")
		assert.False(t, ok)
		rec := storedEvaluation(t, d.EvaluationId)
		require.Len(t, rec.Shadow, 1)
		assert.Equal(t, "large_ngn_transfer", rec.Shadow[0].Code)
		assert.Equal(t, []string{"large_ngn_transfer@4"}, rec.RuleVersions)
	})
	t.Run("disabled rules are not evaluated", func(t *testing.T) {
		r := s.call(t, http.MethodPatch, path, a.testKey, map[string]any{"enabled": false})
		require.Equal(t, http.StatusOK, r.status, r.String())
		d := s.decide(t, a.testKey, big())
		assert.Empty(t, storedEvaluation(t, d.EvaluationId).RuleVersions)
	})
	t.Run("changes are audited", func(t *testing.T) {
		logs := s.call(t, http.MethodGet, "/api/v1/audit-logs", a.testKey, nil)
		actions := auditActions(t, logs)
		assert.Contains(t, actions, "rule.created")
		assert.Contains(t, actions, "rule.updated")
		assert.Contains(t, actions, "rule.rolled_back")
	})
	t.Run("list", func(t *testing.T) {
		list := decode[apigen.RuleList](t, s.call(t, http.MethodGet, "/api/v1/rules", a.testKey, nil))
		require.Len(t, list.Data, 1)
		assert.Equal(t, rule.Id, list.Data[0].Id)
	})
}

func TestRuleValidation(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Rule validation")
	require.Equal(t, http.StatusCreated, s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, largeTransferRule()).status)

	with := func(k string, v any) map[string]any {
		r := largeTransferRule()
		r["code"] = "other_rule"
		r[k] = v
		return r
	}
	cases := map[string]struct {
		body   map[string]any
		status int
	}{
		"duplicate code":      {largeTransferRule(), http.StatusConflict},
		"built-in code":       {with("code", "new_device"), http.StatusBadRequest},
		"unknown field":       {with("condition", map[string]any{"field": "transaction.secret", "op": "eq", "value": 1}), http.StatusBadRequest},
		"wrong operator type": {with("condition", map[string]any{"field": "transaction.currency", "op": "gt", "value": "NGN"}), http.StatusBadRequest},
		"wrong value type":    {with("condition", map[string]any{"field": "transaction.amount", "op": "gt", "value": "big"}), http.StatusBadRequest},
		"regex operator":      {with("condition", map[string]any{"field": "transaction.currency", "op": "matches", "value": ".*"}), http.StatusBadRequest},
		"unknown key":         {with("condition", map[string]any{"field": "transaction.amount", "op": "gt", "value": 1, "eval": "x"}), http.StatusBadRequest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, tc.body)
			assert.Equal(t, tc.status, r.status, r.String())
		})
	}
	t.Run("too deep", func(t *testing.T) {
		var c any = map[string]any{"field": "transaction.amount", "op": "gt", "value": 1}
		for range 8 {
			c = map[string]any{"not": c}
		}
		assert.Equal(t, http.StatusBadRequest, s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, with("condition", c)).status)
	})
	t.Run("rule count is bounded", func(t *testing.T) {
		for i := 1; i < 200; i++ { // one rule exists already
			r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, with("code", fmt.Sprintf("bulk_rule_%03d", i)))
			require.Equal(t, http.StatusCreated, r.status, r.String())
		}
		r := s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, with("code", "one_too_many"))
		assert.Equal(t, http.StatusConflict, r.status, r.String())
	})
}

func TestRuleTesting(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Rule testing")
	rule := decode[apigen.Rule](t, s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, largeTransferRule()))
	path := "/api/v1/rules/" + rule.Id + "/test"
	u := actor{"u_test", "acc_test", "dev_test"}

	r := s.call(t, http.MethodPost, path, a.testKey, map[string]any{"event": debit(u, "ben_t", "NG", "NGN", 2000000)})
	require.Equal(t, http.StatusOK, r.status, r.String())
	res := decode[apigen.TestRuleResult](t, r)
	assert.True(t, res.Matched)
	assert.False(t, res.Draft)
	require.NotNil(t, res.Version)
	assert.Equal(t, 1, *res.Version)
	assert.InDelta(t, 2000000.0, res.Facts["transaction.amount"], 0.1)

	draft := decode[apigen.TestRuleResult](t, s.call(t, http.MethodPost, path, a.testKey, map[string]any{
		"event":     debit(u, "ben_t", "NG", "NGN", 2000000),
		"condition": map[string]any{"field": "transaction.currency", "op": "eq", "value": "USD"},
	}))
	assert.True(t, draft.Draft)
	assert.Nil(t, draft.Version)
	assert.False(t, draft.Matched)

	t.Run("testing stores nothing and learns nothing", func(t *testing.T) {
		assert.Empty(t, decode[apigen.EventList](t, s.call(t, http.MethodGet, "/api/v1/events", a.testKey, nil)).Data)
		d := s.decide(t, a.testKey, debit(u, "ben_t", "NG", "NGN", 5000))
		assert.Equal(t, []string{"new_user"}, signalCodes(d), "the tested events did not create history")
	})
	t.Run("card numbers are rejected", func(t *testing.T) {
		e := debit(u, "ben_t", "NG", "NGN", 5000)
		e["data"].(map[string]any)["transaction_id"] = "4111111111111111" //nolint:errcheck // test fixture
		assert.Equal(t, http.StatusUnprocessableEntity, s.call(t, http.MethodPost, path, a.testKey, map[string]any{"event": e}).status)
	})
}

func TestRiskSettings(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Settings")

	defaults := decode[apigen.RiskSettings](t, s.call(t, http.MethodGet, "/api/v1/risk/settings", a.testKey, nil))
	assert.Equal(t, int64(1000000), defaults.HighValueDefault)
	assert.Equal(t, apigen.RiskSettingsFailPolicies("closed"), defaults.FailPolicies["transaction"])
	assert.Equal(t, apigen.RiskSettingsFailPolicies("open"), defaults.FailPolicies["login"])

	put := func(body map[string]any) response {
		return s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, body)
	}
	valid := func() map[string]any {
		return map[string]any{
			"weights": map[string]any{"new_user": 0, "new_device": 35}, "high_value_default": 500000,
			"high_value_by_currency": map[string]any{}, "fail_policies": map[string]any{"transaction": "closed"},
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"weight above the cap": func(b map[string]any) { b["weights"] = map[string]any{"new_device": 51} },
		"unknown signal":       func(b map[string]any) { b["weights"] = map[string]any{"made_up": 5} },
		"bad currency":         func(b map[string]any) { b["high_value_by_currency"] = map[string]any{"naira": 5} },
		"unknown event type":   func(b map[string]any) { b["fail_policies"] = map[string]any{"withdrawal": "open"} },
	} {
		t.Run(name, func(t *testing.T) {
			b := valid()
			mutate(b)
			assert.Equal(t, http.StatusBadRequest, put(b).status)
		})
	}

	r := put(valid())
	require.Equal(t, http.StatusOK, r.status, r.String())
	d := s.decide(t, a.testKey, debit(actor{"u_set", "acc_set", "dev_set"}, "ben_set", "NG", "NGN", 5000))
	assert.Empty(t, d.Signals, "new_user weighted to zero")

	catalog := decode[apigen.RiskCatalog](t, s.call(t, http.MethodGet, "/api/v1/risk/catalog", a.testKey, nil))
	for _, sig := range catalog.Signals {
		if sig.Code == "new_device" {
			assert.Equal(t, 20, sig.DefaultWeight)
			assert.Equal(t, 35, sig.Weight)
		}
	}
	assert.NotEmpty(t, catalog.Fields)
	assert.Contains(t, auditActions(t, s.call(t, http.MethodGet, "/api/v1/audit-logs", a.testKey, nil)), "risk_settings.updated")
}

func TestRiskScopesAndIsolation(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Risk scopes A")
	b := s.createTenant(t, "Risk scopes B")
	rule := decode[apigen.Rule](t, s.call(t, http.MethodPost, "/api/v1/rules", a.testKey, largeTransferRule()))
	path := "/api/v1/rules/" + rule.Id

	_, reader := s.createKey(t, a.testKey, "reader", "rules:read")
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, path, reader, nil).status)
	assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/risk/catalog", reader, nil).status)
	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodPatch, path, reader, map[string]any{"weight": 1}).status)
	assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodGet, "/api/v1/risk/settings", reader, nil).status)

	for _, p := range []string{path, path + "/versions"} {
		assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodGet, p, b.testKey, nil).status, p)
	}
	assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPatch, path, b.testKey, map[string]any{"weight": 1}).status)
	assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPost, path+"/rollback", b.testKey, map[string]any{"version": 1}).status)
	assert.Equal(t, http.StatusNotFound, s.call(t, http.MethodPost, path+"/test", b.testKey,
		map[string]any{"event": debit(actor{"u", "a", "d"}, "b", "NG", "NGN", 1)}).status)
	assert.Empty(t, decode[apigen.RuleList](t, s.call(t, http.MethodGet, "/api/v1/rules", b.testKey, nil)).Data)

	// B's rule with the same code is independent, and does not fire on A's events.
	require.Equal(t, http.StatusCreated, s.call(t, http.MethodPost, "/api/v1/rules", b.testKey, largeTransferRule()).status)
	t.Run("rule versions are insert-only", func(t *testing.T) {
		_, err := shared.owner.Exec(t.Context(), `UPDATE rule_versions SET weight = 99`)
		require.Error(t, err)
	})
	t.Run("rls hides rules without a scope", func(t *testing.T) {
		var n int
		require.NoError(t, shared.pool.QueryRow(t.Context(), `SELECT count(*) FROM rules`).Scan(&n))
		assert.Zero(t, n)
	})
}

func TestRiskPolicyAndCalibration(t *testing.T) {
	s := newStack(t, time.Second)
	a := s.createTenant(t, "Calibration")

	policy := decode[apigen.RiskPolicy](t, s.call(t, http.MethodGet, "/api/v1/risk/policy", a.testKey, nil))
	assert.False(t, policy.CustomCalibration)
	assert.Equal(t, apigen.RiskThresholds{Low: 20, Elevated: 50, High: 70, Critical: 90}, policy.Thresholds)
	require.Len(t, policy.Bands, 5)
	want := []struct {
		band     string
		min, max int
		decision string
	}{{"minimal", 0, 19, "allow"}, {"low", 20, 49, "allow"}, {"elevated", 50, 69, "review"}, {"high", 70, 89, "step_up"}, {"critical", 90, 100, "review"}}
	for i, w := range want {
		b := policy.Bands[i]
		assert.Equal(t, w.band, string(b.Band))
		assert.Equal(t, [2]int{w.min, w.max}, [2]int{b.MinScore, b.MaxScore}, w.band)
		assert.Equal(t, w.decision, string(b.DefaultDecision), w.band)
	}

	t.Run("readable by anyone who sees scores", func(t *testing.T) {
		_, alertsReader := s.createKey(t, a.testKey, "alerts reader", "alerts:read")
		assert.Equal(t, http.StatusOK, s.call(t, http.MethodGet, "/api/v1/risk/policy", alertsReader, nil).status)
		_, keysOnly := s.createKey(t, a.testKey, "keys only", "api_keys:read")
		assert.Equal(t, http.StatusForbidden, s.call(t, http.MethodGet, "/api/v1/risk/policy", keysOnly, nil).status)
	})
	t.Run("tenant calibration moves the bands", func(t *testing.T) {
		body := func(th map[string]any) map[string]any {
			return map[string]any{
				"weights": map[string]any{}, "high_value_default": 1000000, "high_value_by_currency": map[string]any{},
				"fail_policies": map[string]any{"transaction": "closed", "login": "open"}, "thresholds": th,
			}
		}
		bad := s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, body(map[string]any{"low": 30, "elevated": 30, "high": 60, "critical": 80}))
		assert.Equal(t, http.StatusBadRequest, bad.status, bad.String())
		r := s.call(t, http.MethodPut, "/api/v1/risk/settings", a.testKey, body(map[string]any{"low": 3, "elevated": 30, "high": 60, "critical": 80}))
		require.Equal(t, http.StatusOK, r.status, r.String())
		p := decode[apigen.RiskPolicy](t, s.call(t, http.MethodGet, "/api/v1/risk/policy", a.testKey, nil))
		assert.True(t, p.CustomCalibration)
		d := s.decide(t, a.testKey, debit(actor{"u_cal", "acc_cal", "dev_cal"}, "ben", "NG", "NGN", 5000))
		assert.Equal(t, 5, d.RiskScore, "new_user only")
		assert.Equal(t, apigen.RiskLevel("low"), d.RiskLevel, "5 is Low when low starts at 3")
	})
}
