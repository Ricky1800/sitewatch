// Package clock provides a small time abstraction so schedulers and
// state-machine logic can be driven by a deterministic fake clock in tests
// instead of real wall-clock sleeps.
package clock

import "time"

// Clock is the subset of time-related behavior sitewatch depends on.
// Production code uses Real; tests use Fake.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
}

// Real is a Clock backed by the actual system time.
type Real struct{}

// Now returns time.Now().
func (Real) Now() time.Time { return time.Now() }
