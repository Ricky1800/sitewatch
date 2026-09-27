package clock

import (
	"sync"
	"time"
)

// Fake is a Clock whose value is controlled entirely by test code. It never
// advances on its own, which lets tests exercise time-dependent logic (state
// machines, daily SSL-warning windows, uptime windows) without sleeping.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake clock initialized to t.
func NewFake(t time.Time) *Fake {
	return &Fake{now: t}
}

// Now returns the current fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the fake clock forward by d (d may be negative).
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

// Set moves the fake clock to an absolute time.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	f.now = t
	f.mu.Unlock()
}
