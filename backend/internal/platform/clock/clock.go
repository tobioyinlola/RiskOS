// Package clock abstracts the current time so time-dependent code (latency measurement now;
// replay windows and velocity buckets later) is deterministic under test.
package clock

import "time"

// Clock reports the current time.
type Clock interface {
	Now() time.Time
}

// System is the wall clock.
type System struct{}

// Now returns time.Now().
func (System) Now() time.Time { return time.Now() }
