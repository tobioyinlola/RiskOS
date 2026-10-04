package rules_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/rules"
)

// The spec's example rule (§9): IF transaction.amount > 1000000 AND beneficiary is new.
const specExample = `{"all":[
  {"field":"transaction.amount","op":"gt","value":1000000},
  {"field":"feature.new_beneficiary","op":"eq","value":true}
]}`

func TestSpecExampleRule(t *testing.T) {
	c, err := rules.Compile([]byte(specExample))
	require.NoError(t, err)
	assert.Equal(t, []string{"feature.new_beneficiary", "transaction.amount"}, c.Fields())

	assert.True(t, c.Match(rules.Facts{"transaction.amount": 1500000.0, "feature.new_beneficiary": true}))
	assert.False(t, c.Match(rules.Facts{"transaction.amount": 1500000.0, "feature.new_beneficiary": false}))
	assert.False(t, c.Match(rules.Facts{"transaction.amount": 1000000.0, "feature.new_beneficiary": true}), "gt is strict")
	assert.False(t, c.Match(rules.Facts{"feature.new_beneficiary": true}), "missing facts never match")
}

func TestOperators(t *testing.T) {
	facts := rules.Facts{"transaction.amount": 500.0, "transaction.currency": "NGN", "feature.new_device": true, "context.country": "GH"}
	tests := map[string]bool{
		`{"field":"transaction.amount","op":"gte","value":500}`:                                                               true,
		`{"field":"transaction.amount","op":"lt","value":500}`:                                                                false,
		`{"field":"transaction.amount","op":"lte","value":500}`:                                                               true,
		`{"field":"transaction.amount","op":"ne","value":1}`:                                                                  true,
		`{"field":"transaction.currency","op":"in","value":["USD","NGN"]}`:                                                    true,
		`{"field":"transaction.currency","op":"not_in","value":["USD","NGN"]}`:                                                false,
		`{"field":"context.country","op":"ne","value":"NG"}`:                                                                  true,
		`{"field":"feature.new_device","op":"ne","value":true}`:                                                               false,
		`{"any":[{"field":"feature.new_device","op":"eq","value":false},{"field":"context.country","op":"eq","value":"GH"}]}`: true,
		`{"not":{"field":"context.country","op":"eq","value":"GH"}}`:                                                          false,
		`{"not":{"field":"login.outcome","op":"eq","value":"failure"}}`:                                                       true,
	}
	for cond, want := range tests {
		c, err := rules.Compile([]byte(cond))
		require.NoError(t, err, cond)
		assert.Equal(t, want, c.Match(facts), cond)
	}
}

func TestCompileRejects(t *testing.T) {
	deep := strings.Repeat(`{"not":`, rules.MaxDepth) + `{"field":"feature.new_device","op":"eq","value":true}` + strings.Repeat(`}`, rules.MaxDepth)
	var wide strings.Builder
	wide.WriteString(`{"any":[`)
	for i := range rules.MaxNodes {
		if i > 0 {
			wide.WriteString(",")
		}
		wide.WriteString(`{"field":"feature.new_device","op":"eq","value":true}`)
	}
	wide.WriteString(`]}`)
	manyItems := `{"field":"transaction.currency","op":"in","value":[` + strings.Repeat(`"X",`, rules.MaxListItems) + `"Y"]}`

	tests := map[string]string{
		"unknown field":            `{"field":"user.email","op":"eq","value":"a"}`,
		"unknown operator":         `{"field":"transaction.amount","op":"matches","value":".*"}`,
		"operator not for strings": `{"field":"transaction.currency","op":"gt","value":"A"}`,
		"operator not for bools":   `{"field":"feature.new_device","op":"in","value":[true]}`,
		"wrong value type":         `{"field":"transaction.amount","op":"gt","value":"1000"}`,
		"missing value":            `{"field":"transaction.amount","op":"gt"}`,
		"two kinds in one node":    `{"field":"transaction.amount","op":"gt","value":1,"all":[]}`,
		"empty group":              `{"all":[]}`,
		"empty node":               `{}`,
		"unknown key":              `{"field":"transaction.amount","op":"gt","value":1,"script":"x"}`,
		"too deep":                 deep,
		"too many nodes":           wide.String(),
		"list too long":            manyItems,
		"empty list":               `{"field":"transaction.currency","op":"in","value":[]}`,
		"not JSON":                 `{"field":`,
		"trailing data":            `{"field":"feature.new_device","op":"eq","value":true} {}`,
	}
	for name, cond := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := rules.Compile([]byte(cond))
			assert.ErrorIs(t, err, rules.ErrInvalid)
		})
	}
}

func TestErrorsNameTheProblem(t *testing.T) {
	_, err := rules.Compile([]byte(`{"all":[{"field":"feature.new_device","op":"eq","value":true},{"field":"nope","op":"eq","value":1}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "condition.all[1]")
	assert.Contains(t, err.Error(), `unknown field "nope"`)
}

func TestEvaluationIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"all":[`)
	for i := range rules.MaxNodes - 1 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"field":"transaction.currency","op":"not_in","value":[` + strings.Repeat(`"AAA",`, rules.MaxListItems-1) + `"ZZZ"]}`)
	}
	b.WriteString(`]}`)
	c, err := rules.Compile([]byte(b.String()))
	require.NoError(t, err)
	start := time.Now()
	for range 1000 {
		c.Match(rules.Facts{"transaction.currency": "NGN"})
	}
	assert.Less(t, time.Since(start), 500*time.Millisecond, "the largest allowed rule evaluates in well under a millisecond")
}

// Every fact a rule can use is described for people who build rules without code, and every
// listed value is one the event contract allows.
func TestEveryFieldIsDescribed(t *testing.T) {
	fields, info := rules.Fields(), rules.Info()
	require.Len(t, info, len(fields))
	for name, typ := range fields {
		fi, ok := info[name]
		require.True(t, ok, "%s has no description", name)
		assert.NotEmpty(t, fi.Label, name)
		assert.NotEmpty(t, fi.Group, name)
		if typ != rules.Number {
			assert.Empty(t, fi.Unit, "%s: only numbers have units", name)
		}
		if len(fi.Values) > 0 || fi.Kind != "" {
			assert.Equal(t, rules.String, typ, "%s: values and kinds are for strings", name)
		}
	}
}
