package alert

import (
	"fmt"
	"time"
)

// summary renders a short, human-readable one-liner for an event, shared by
// every notifier so alert text is consistent across channels.
func summary(ev Event) string {
	switch ev.Kind {
	case Down:
		return fmt.Sprintf("%s is DOWN (%s) — %d consecutive failures", ev.CheckName, ev.URL, ev.ConsecutiveFails)
	case Up:
		return fmt.Sprintf("%s is back UP (%s) — was down for %s", ev.CheckName, ev.URL, formatDuration(ev.Downtime))
	case SSLExpiry:
		return fmt.Sprintf("%s (%s) SSL certificate expires in %d day(s) (%s)", ev.CheckName, ev.URL, ev.SSLDaysLeft, ev.SSLExpiry.Format("2006-01-02"))
	default:
		return fmt.Sprintf("%s: unknown event", ev.CheckName)
	}
}

// detail renders the secondary line/field shown by richer channels
// (Discord/Slack embeds, email body).
func detail(ev Event) string {
	switch ev.Kind {
	case Down:
		if ev.Error != "" {
			return "Last error: " + ev.Error
		}
		return ""
	case Up:
		return fmt.Sprintf("Recovered at %s", ev.Time.Format("2006-01-02 15:04:05 MST"))
	case SSLExpiry:
		return "Renew the certificate soon to avoid an outage."
	default:
		return ""
	}
}

// formatDuration renders a duration rounded to the second, since
// millisecond-level precision in a downtime notification is just noise.
func formatDuration(d time.Duration) string {
	return d.Round(time.Second).String()
}
