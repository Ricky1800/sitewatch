// Package alert builds and sends notifications for check state changes
// (down, recovered, SSL expiry) across Discord, Slack, generic webhook, and
// SMTP email channels.
package alert

import (
	"context"
	"time"
)

// Kind identifies what happened.
type Kind int

const (
	// Down fires once when a check crosses its fail_threshold.
	Down Kind = iota
	// Up fires once when a down check recovers.
	Up
	// SSLExpiry fires at most once per day per check while its certificate
	// is inside the configured warning window.
	SSLExpiry
)

func (k Kind) String() string {
	switch k {
	case Down:
		return "DOWN"
	case Up:
		return "UP"
	case SSLExpiry:
		return "SSL_EXPIRY"
	default:
		return "UNKNOWN"
	}
}

// Event describes one alert-worthy occurrence for a single check. Only the
// fields relevant to Kind are populated by callers, but every notifier reads
// defensively.
type Event struct {
	Kind      Kind
	CheckName string
	URL       string
	Time      time.Time

	// Down
	ConsecutiveFails int
	Error            string

	// Up
	Downtime time.Duration

	// SSLExpiry
	SSLExpiry   time.Time
	SSLDaysLeft int
}

// Notifier sends a single alert Event. Implementations must be safe to call
// from multiple goroutines.
type Notifier interface {
	Notify(ctx context.Context, ev Event) error
	// Name returns a short identifier used in logs ("discord", "slack", ...).
	Name() string
}
