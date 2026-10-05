package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/interp"
	"github.com/tobioyinlola/RiskOS/backend/internal/rules/internal/pack"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

// Calendar windows run from the start of the local day or month in the governing pack's time
// zone, from the activation's day boundary (ADR-0007).
func TestCalendarWindows(t *testing.T) {
	accra := &TenantPack{Location: zone(t, "Africa/Accra")}
	kigali := &TenantPack{Location: zone(t, "Africa/Kigali")}
	kigaliSix := &TenantPack{Location: zone(t, "Africa/Kigali"), DayStart: 6}
	day := interp.AggRef{Fn: "sum", Window: 86400, Calendar: pack.CalendarDay}
	month := interp.AggRef{Fn: "sum", Window: 31 * 86400, Calendar: pack.CalendarMonth}
	utc := func(s string) time.Time {
		at, err := time.Parse(time.RFC3339, s)
		require.NoError(t, err)
		return at
	}
	for _, c := range []struct {
		name string
		p    *TenantPack
		ref  interp.AggRef
		at   string
		want time.Duration
	}{
		{"Accra afternoon: since local midnight", accra, day, "2026-10-04T13:30:00Z", 13*time.Hour + 30*time.Minute},
		{"Accra at midnight: a new day", accra, day, "2026-10-05T00:00:00Z", 0},
		{"Accra a second before midnight", accra, day, "2026-10-04T23:59:59Z", 24*time.Hour - time.Second},
		{"Kigali 01:30 local is 23:30 UTC the day before", kigali, day, "2026-10-04T23:30:00Z", 90 * time.Minute},
		{"Kigali 23:30 local", kigali, day, "2026-10-04T21:30:00Z", 23*time.Hour + 30*time.Minute},
		{"a day starting at 06:00, at 03:00 local", kigaliSix, day, "2026-10-05T01:00:00Z", 21 * time.Hour},
		{"a day starting at 06:00, at 06:00 local", kigaliSix, day, "2026-10-05T04:00:00Z", 0},
		{"Accra month: since the 1st", accra, month, "2026-10-04T12:00:00Z", 3*24*time.Hour + 12*time.Hour},
		{"Kigali month: the 1st at 01:00 local is still October", kigali, month, "2026-09-30T23:00:00Z", time.Hour},
		{"a month starting at 06:00 on the 1st, at 03:00 local", kigaliSix, month, "2026-10-01T01:00:00Z", 30*24*time.Hour - 3*time.Hour},
		{"rolling windows are unchanged", kigali, interp.AggRef{Fn: "sum", Window: 3600}, "2026-10-04T23:30:00Z", time.Hour},
	} {
		t.Run(c.name, func(t *testing.T) {
			fn, window := c.p.Query(c.ref, utc(c.at))
			assert.Equal(t, "sum", fn)
			assert.Equal(t, c.want, window)
		})
	}
}

// days_over carries its threshold and the shift from UTC to the local business day.
func TestDaysOverQuery(t *testing.T) {
	ref := interp.AggRef{Fn: "days_over", Window: 604800, Over: 240_000, OverCurrency: "GHS"}
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		p    *TenantPack
		want string
	}{
		{&TenantPack{Location: zone(t, "Africa/Accra")}, "days_over:240000:0"},
		{&TenantPack{Location: zone(t, "Africa/Kigali")}, "days_over:240000:7200000"},
		{&TenantPack{Location: zone(t, "Africa/Kigali"), DayStart: 6}, "days_over:240000:-14400000"},
	} {
		fn, window := c.p.Query(ref, at)
		assert.Equal(t, c.want, fn)
		assert.Equal(t, 7*24*time.Hour, window)
	}
}
