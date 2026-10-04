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
	f := features.PackFacts(e, v, features.Activity{Hours: &thirteen}, wat)
	assert.Equal(t, true, f["history.hour_usual"], "active at 13:00 Lagos before")
	f = features.PackFacts(e, v, features.Activity{Hours: &nine}, wat)
	assert.Equal(t, false, f["history.hour_usual"])
	none := int64(0)
	assert.NotContains(t, features.PackFacts(e, v, features.Activity{Hours: &none}, wat), "history.hour_usual", "no hours yet: unknown, not unusual")
	assert.Equal(t, 13.0, f["txn.local_hour"])

	first := e.OccurredAt.Add(-40 * 24 * time.Hour)
	f = features.PackFacts(e, v, features.Activity{UserFirst: &first}, wat)
	assert.InDelta(t, 40*86400, f["customer.history_age"], 0)
	assert.NotContains(t, f, "account.inactive_for", "no earlier activity known: unknown, not active")

	f = features.PackFacts(e, features.Values{Warming: true}, features.Activity{Hours: &thirteen, UserFirst: &first}, wat)
	assert.NotContains(t, f, "history.hour_usual", "history is held back while it warms up")
	assert.NotContains(t, f, "customer.history_age")
}

func TestExportBaselines(t *testing.T) {
	e := parse(t, `{"type":"activity","occurred_at":"2026-09-30T12:00:00Z","actor":{"user_id":"u1"},"data":{},
		"compliance":{"kind":"data_export","facts":{"export.rows":600}}}`)
	f := features.PackFacts(e, features.Values{}, features.Activity{
		ExportUser:   features.Baseline{Count: 5, Mean: 100},
		ExportTenant: features.Baseline{Count: 4, Mean: 100},
	}, wat)
	assert.InDelta(t, 6.0, f["export.rows_to_user_baseline"], 0)
	assert.NotContains(t, f, "export.rows_to_tenant_baseline", "four exports are too few for a baseline")
}

func TestTravelSpeed(t *testing.T) {
	e := parse(t, `{"type":"login","occurred_at":"2026-09-30T12:00:00Z","actor":{"user_id":"u1"},
		"context":{"geo":{"lat":51.51,"lon":-0.13}},"data":{"outcome":"success","method":"password"}}`)
	lagos := features.Geo{Lat: 6.52, Lon: 3.38}
	hourBefore := e.OccurredAt.Add(-time.Hour)
	f := features.PackFacts(e, features.Values{}, features.Activity{LastGeo: &lagos, LastGeoAt: &hourBefore}, wat)
	speed, ok := f["geo.speed_from_previous"].(float64)
	assert.True(t, ok)
	assert.InDelta(t, 5000, speed, 100, "Lagos to London in an hour")

	london := features.Geo{Lat: 51.51, Lon: -0.13}
	f = features.PackFacts(e, features.Values{}, features.Activity{LastGeo: &london, LastGeoAt: &hourBefore}, wat)
	assert.InDelta(t, 0, f["geo.speed_from_previous"], 0.1)

	later := e.OccurredAt.Add(time.Hour)
	f = features.PackFacts(e, features.Values{}, features.Activity{LastGeo: &lagos, LastGeoAt: &later}, wat)
	assert.NotContains(t, f, "geo.speed_from_previous", "an event older than the last location says nothing")
}

// wat is West Africa Time (Lagos, UTC+1), the time zone of the Nigeria pack.
var wat = time.FixedZone("WAT", 3600) //nolint:gochecknoglobals // a fixed zone, never mutated
