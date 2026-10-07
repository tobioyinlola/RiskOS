package workdays

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnd(t *testing.T) {
	accra, err := time.LoadLocation("Africa/Accra")
	require.NoError(t, err)
	kigali, err := time.LoadLocation("Africa/Kigali")
	require.NoError(t, err)
	at := func(loc *time.Location, s string) time.Time {
		t.Helper()
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		require.NoError(t, err)
		return v
	}
	for _, c := range []struct {
		name     string
		cal      Calendar
		from     time.Time
		n        int
		want     time.Time
		describe string
	}{
		{
			name: "seven working days from a Monday end at the close of the next Wednesday",
			cal:  Calendar{Location: accra}, from: at(accra, "2026-10-05 10:00"), n: 7,
			want: at(accra, "2026-10-15 00:00"),
		},
		{
			name: "a Friday order skips the weekend before counting",
			cal:  Calendar{Location: accra}, from: at(accra, "2026-10-09 16:30"), n: 1,
			want: at(accra, "2026-10-13 00:00"),
		},
		{
			name: "an order on a Saturday counts from Monday",
			cal:  Calendar{Location: accra}, from: at(accra, "2026-10-10 09:00"), n: 2,
			want: at(accra, "2026-10-14 00:00"),
		},
		{
			name: "public holidays the institution entered are skipped",
			cal:  Calendar{Location: accra, Holidays: map[string]bool{"2026-12-25": true, "2026-12-26": true, "2027-01-01": true}},
			from: at(accra, "2026-12-23 12:00"), n: 3,
			// 24 (Thu) is day 1; 25 and 26 are holidays, 26-27 the weekend; 28 (Mon) day 2; 29 day 3.
			want: at(accra, "2026-12-30 00:00"),
		},
		{
			name: "the day is the jurisdiction's: 23:30 in Accra is already the next day in Kigali",
			cal:  Calendar{Location: kigali}, from: at(accra, "2026-10-08 23:30"), n: 1,
			// Friday 01:30 in Kigali; the next working day is Monday 12 October.
			want: at(kigali, "2026-10-13 00:00"),
		},
		{
			name: "zero days is the start itself",
			cal:  Calendar{Location: accra}, from: at(accra, "2026-10-05 10:00"), n: 0,
			want: at(accra, "2026-10-05 10:00"),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.cal.End(c.from, c.n)
			assert.True(t, c.want.Equal(got), "want %s, got %s", c.want, got.In(c.cal.Location))
		})
	}
}

func TestWorking(t *testing.T) {
	cal := Calendar{Location: time.UTC, Holidays: map[string]bool{"2026-03-06": true}}
	assert.True(t, cal.Working(time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)))
	assert.False(t, cal.Working(time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)), "a holiday")
	assert.False(t, cal.Working(time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)), "a Saturday")
	assert.False(t, cal.Working(time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)), "a Sunday")
}
