package history

import "time"

// Uptime is the result of computing an uptime percentage over a window: OK
// is false when there is no data at all in the window, in which case
// Percent is meaningless and callers should render "no data" instead of a
// number.
type Uptime struct {
	Percent float64
	OK      bool
	Total   int
	Up      int
}

// PercentInWindow computes the fraction of records for one check that were
// successful within [since, now), as a percentage in [0, 100].
func PercentInWindow(records []Record, checkName string, since, now time.Time) Uptime {
	var total, up int
	for _, r := range records {
		if r.Check != checkName {
			continue
		}
		if r.Time.Before(since) || !r.Time.Before(now) {
			continue
		}
		total++
		if r.Success {
			up++
		}
	}
	if total == 0 {
		return Uptime{OK: false}
	}
	return Uptime{
		Percent: 100 * float64(up) / float64(total),
		OK:      true,
		Total:   total,
		Up:      up,
	}
}
