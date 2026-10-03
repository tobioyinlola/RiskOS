package pack_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

const meta = `{"id":"t","jurisdiction":"NG","name":"Test","version":"1",
 "groups":[{"id":"velocity","label":"Velocity"},{"id":"mandatory_controls","label":"Mandatory"}],
 "sources":[{"id":"S1","title":"Source","links":[]}],"clocks":[],"gaps":[],"glossary":[],
 "licence_types":[{"id":"bank","label":"Bank"}],"capabilities":[]}`

const text = `{"VEL-01":{"name":"Burst","description":"Five or more transfers out in 10 minutes","points":"15","engineering_logic":"RULE VEL-01"}}`

// signal is a valid VEL-01; tests mutate it.
func signal() map[string]any {
	var r map[string]any
	_ = json.Unmarshal([]byte(`{
	 "id":"VEL-01","group":"velocity","kind":"signal","status":"recommended","regulators":["NIBSS"],
	 "sources":["S1"],"effective":{"text":"Not regulation"},"verification":"secondary_source_only",
	 "defaults":{"enabled":true,"mode":"shadow"},"inputs":["events","history"],
	 "trigger":{"events":["transfer_outbound"]},
	 "condition":{"left":{"agg":{"fn":"count","event":"transfer_outbound","window":"window","by":"account"}},
	              "op":"gte","right":{"param":"transfers"}},
	 "outcome":{"weight":"weight","severity":"medium","reason":"burst"},
	 "parameters":[
	  {"name":"weight","label":"Points","type":"points","default":15,"min":0,"max":50},
	  {"name":"transfers","label":"Transfers","type":"count","default":5,"min":2,"max":100},
	  {"name":"window","label":"Window","type":"duration","default":600,"min":60,"max":3600}]}`), &r)
	return r
}

func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	require.True(t, ok)
	return m
}

func list(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	require.True(t, ok)
	return l
}

func load(t *testing.T, rule map[string]any) error {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"rules": []any{rule}})
	require.NoError(t, err)
	_, err = pack.Load(fstest.MapFS{
		"pack.json":  {Data: []byte(meta)},
		"text.json":  {Data: []byte(text)},
		"rules.json": {Data: raw},
	})
	return err
}

func TestValidRuleLoads(t *testing.T) {
	require.NoError(t, load(t, signal()))
}

func TestValidatorRejects(t *testing.T) {
	cases := map[string]struct {
		mutate func(t *testing.T, r map[string]any)
		want   string
	}{
		"a literal threshold": {func(t *testing.T, r map[string]any) {
			obj(t, r["condition"])["right"] = map[string]any{"literal": 5}
		}, "compare against a parameter, not a literal number"},
		"an unknown fact": {func(t *testing.T, r map[string]any) {
			r["condition"] = map[string]any{"left": map[string]any{"fact": "txn.secret"}, "op": "gt", "right": map[string]any{"param": "transfers"}}
		}, `unknown fact "txn.secret"`},
		"a fact from an undeclared input": {func(t *testing.T, r map[string]any) {
			r["condition"] = map[string]any{"left": map[string]any{"fact": "customer.is_pep"}, "op": "eq", "right": map[string]any{"literal": true}}
		}, `comes from input "pep_list", which the rule does not list`},
		"mismatched units": {func(t *testing.T, r map[string]any) {
			obj(t, r["condition"])["right"] = map[string]any{"param": "window"}
		}, "a count cannot be compared with a duration parameter"},
		"an unused parameter": {func(t *testing.T, r map[string]any) {
			r["parameters"] = append(list(t, r["parameters"]), map[string]any{"name": "spare", "label": "Spare", "type": "count", "default": 1, "min": 1, "max": 2})
		}, `parameter "spare" is never used`},
		"a default outside its bounds": {func(t *testing.T, r map[string]any) {
			obj(t, list(t, r["parameters"])[1])["default"] = 500
		}, "the default is outside its bounds"},
		"an unknown field (no code can hide in a pack)": {func(t *testing.T, r map[string]any) {
			r["script"] = "os.Exit(1)"
		}, `unknown field "script"`},
		"an awaited rule switched on": {func(t *testing.T, r map[string]any) {
			r["verification"], r["caveat"] = "awaiting_regulator", "Awaited."
		}, "must be off by default"},
		"a conflicting rule without a caveat": {func(t *testing.T, r map[string]any) {
			r["verification"] = "conflicting"
		}, "a conflicting rule needs a caveat"},
		"a signal that sets a decision": {func(t *testing.T, r map[string]any) {
			obj(t, r["outcome"])["decision"] = "block"
		}, "signals add weight; they do not set a decision"},
		"no legal source": {func(t *testing.T, r map[string]any) {
			r["sources"] = []any{}
		}, "at least one legal or evidence source is required"},
		"a wrong kind for the prefix": {func(t *testing.T, r map[string]any) {
			r["kind"] = "control"
		}, "kind and status must be signal and recommended"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := signal()
			tc.mutate(t, r)
			err := load(t, r)
			require.Error(t, err)
			assert.ErrorIs(t, err, pack.ErrInvalid)
			assert.True(t, strings.Contains(err.Error(), tc.want), "got %v", err)
		})
	}
}

func TestEveryRuleNeedsWording(t *testing.T) {
	r := signal()
	r["id"] = "VEL-02"
	err := load(t, r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VEL-02: no wording")
	assert.Contains(t, err.Error(), "VEL-01 has wording but no rule")
}
