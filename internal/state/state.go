// Package state implements the per-check state machine that turns a stream
// of pass/fail probe results into up/down transitions, suppressing flapping
// alerts, plus a once-per-day SSL expiry warning gate.
package state

import "time"

// Status is the current up/down/unknown state of a check.
type Status int

const (
	// StatusUnknown is the initial state before any result has been recorded.
	StatusUnknown Status = iota
	StatusUp
	StatusDown
)

// String implements fmt.Stringer.
func (s Status) String() string {
	switch s {
	case StatusUp:
		return "UP"
	case StatusDown:
		return "DOWN"
	default:
		return "UNKNOWN"
	}
}

// TransitionKind describes what, if anything, changed after recording a
// result.
type TransitionKind int

const (
	// NoTransition means the check stayed in the same externally-visible
	// state (including "still failing but hasn't crossed fail_threshold yet").
	NoTransition TransitionKind = iota
	// TransitionToDown fires exactly once, the moment consecutive failures
	// reach the configured fail_threshold.
	TransitionToDown
	// TransitionToUp fires exactly once, on the first success after being
	// down.
	TransitionToUp
)

// Transition is returned by Record whenever recording a probe result caused
// (or didn't cause) a state change worth alerting on.
type Transition struct {
	Kind             TransitionKind
	ConsecutiveFails int           // populated on TransitionToDown
	Downtime         time.Duration // populated on TransitionToUp
}

// CheckState tracks the running state of a single monitored check across
// repeated probes. The zero value is a valid starting state (StatusUnknown).
type CheckState struct {
	Current              Status
	ConsecutiveFails     int
	ConsecutiveSuccesses int
	DownSince            time.Time
	LastChangeTime       time.Time
	lastSSLWarnDateSet   bool
	lastSSLWarnDate      string
}

// Record records the outcome of one probe (success or failure) and returns
// the Transition it produced, if any. now is supplied by the caller (via an
// injected clock) so the state machine has no dependency on real time.
//
// Alerting semantics:
//   - Up -> Down only fires once consecutive failures reach failThreshold.
//     Further failures while already Down produce NoTransition (no repeated
//     alerts / no flapping).
//   - Down -> Up fires on the very next success, reporting how long the
//     check was down.
//   - A single failure that doesn't reach failThreshold does not change
//     Current and does not alert, but does increment ConsecutiveFails so a
//     later failure can still cross the threshold.
func (s *CheckState) Record(success bool, failThreshold int, now time.Time) Transition {
	if failThreshold < 1 {
		failThreshold = 1
	}

	if success {
		s.ConsecutiveFails = 0
		s.ConsecutiveSuccesses++

		if s.Current == StatusDown {
			downtime := now.Sub(s.DownSince)
			s.Current = StatusUp
			s.LastChangeTime = now
			return Transition{Kind: TransitionToUp, Downtime: downtime}
		}
		if s.Current == StatusUnknown {
			s.Current = StatusUp
			s.LastChangeTime = now
		}
		return Transition{Kind: NoTransition}
	}

	// Failure.
	s.ConsecutiveSuccesses = 0
	s.ConsecutiveFails++

	if s.Current == StatusDown {
		// Already down: no repeated alert.
		return Transition{Kind: NoTransition}
	}

	if s.ConsecutiveFails >= failThreshold {
		s.Current = StatusDown
		s.DownSince = now
		s.LastChangeTime = now
		return Transition{Kind: TransitionToDown, ConsecutiveFails: s.ConsecutiveFails}
	}

	// Failing, but hasn't crossed the threshold yet.
	return Transition{Kind: NoTransition}
}

// ShouldWarnSSL reports whether an SSL-expiry warning should fire right now
// for a certificate expiring at expiry, given warnDays and the current time
// now, and if so marks today as warned so a second call on the same day
// returns false. The "day" is computed from now in UTC, so it is stable
// under repeated calls within the same calendar day regardless of exact
// time-of-day.
func (s *CheckState) ShouldWarnSSL(expiry time.Time, warnDays int, now time.Time) bool {
	if warnDays <= 0 {
		return false
	}
	daysLeft := expiry.Sub(now)
	if daysLeft > time.Duration(warnDays)*24*time.Hour {
		return false
	}
	if expiry.Before(now) {
		// Already expired; still worth a once-a-day nudge rather than
		// silence, using the same gate.
	}

	today := now.UTC().Format("2006-01-02")
	if s.lastSSLWarnDateSet && s.lastSSLWarnDate == today {
		return false
	}
	s.lastSSLWarnDate = today
	s.lastSSLWarnDateSet = true
	return true
}
