// Package workdays counts a jurisdiction's working days (ADR-0007): Monday to Friday in the
// jurisdiction's time zone, less the public holidays the institution entered. RiskOS ships no
// holiday dates of its own: they are gazetted year by year and some move with the moon, so a
// wrong built-in date would silently shorten a legal hold.
package workdays

import "time"

// DateLayout is how holiday dates are written (2026-12-25).
const DateLayout = time.DateOnly

// Calendar is one jurisdiction's working days.
type Calendar struct {
	// Location is the jurisdiction's time zone: days start and end at its midnight.
	Location *time.Location
	// Holidays are public holidays by date (DateLayout).
	Holidays map[string]bool
}

// Working reports whether a local date is a working day.
func (c Calendar) Working(day time.Time) bool {
	switch day.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	return !c.Holidays[day.Format(DateLayout)]
}

// End returns when a period of n working days from at ends: the close of the nth working day
// after the local day of at (the day itself is not counted, as legal periods are counted). n
// below 1 returns at.
func (c Calendar) End(at time.Time, n int) time.Time {
	if n < 1 {
		return at
	}
	loc := c.Location
	if loc == nil {
		loc = time.UTC
	}
	local := at.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	for counted := 0; counted < n; {
		day = day.AddDate(0, 0, 1)
		if c.Working(day) {
			counted++
		}
	}
	return day.AddDate(0, 0, 1)
}
