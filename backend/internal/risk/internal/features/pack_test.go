package features_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tobioyinlola/RiskOS/backend/internal/risk/internal/features"
)

func TestPackHistoryFacts(t *testing.T) {
	// 12:00 UTC is 13:00 in Lagos.
	e := parse(t, debit)
	v := features.Values{}
	thirteen, nine := int64(1)<<13, int64(1)<<9
	f := features.PackFacts(e, v, features.Activity{Hours: &thirteen})
	assert.Equal(t, true, f["history.hour_usual"], "active at 13:00 Lagos before")
	f = features.PackFacts(e, v, features.Activity{Hours: &nine})
	assert.Equal(t, false, f["history.hour_usual"])
	none := int64(0)
	assert.NotContains(t, features.PackFacts(e, v, features.Activity{Hours: &none}), "history.hour_usual", "no hours yet: unknown, not unusual")
	assert.Equal(t, 13.0, f["txn.local_hour"])

	first := e.OccurredAt.Add(-40 * 24 * time.Hour)
	f = features.PackFacts(e, v, features.Activity{UserFirst: &first})
	assert.InDelta(t, 40*86400, f["customer.history_age"], 0)
	assert.NotContains(t, f, "account.inactive_for", "no earlier activity known: unknown, not active")

	f = features.PackFacts(e, features.Values{Warming: true}, features.Activity{Hours: &thirteen, UserFirst: &first})
	assert.NotContains(t, f, "history.hour_usual", "history is held back while it warms up")
	assert.NotContains(t, f, "customer.history_age")
}

func TestExportBaselines(t *testing.T) {
	e := parse(t, `{"type":"activity","occurred_at":"2026-09-30T12:00:00Z","actor":{"user_id":"u1"},"data":{},
		"compliance":{"kind":"data_export","facts":{"export.rows":600}}}`)
	f := features.PackFacts(e, features.Values{}, features.Activity{
		ExportUser:   features.Baseline{Count: 5, Mean: 100},
		ExportTenant: features.Baseline{Count: 4, Mean: 100},
	})
	assert.InDelta(t, 6.0, f["export.rows_to_user_baseline"], 0)
	assert.NotContains(t, f, "export.rows_to_tenant_baseline", "four exports are too few for a baseline")
}
