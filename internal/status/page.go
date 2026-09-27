// Package status generates a self-contained static HTML status page (no
// external CSS/JS/fonts) from a config and history records.
package status

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/history"
)

//go:embed templates/status.html.tmpl
var templateFS embed.FS

var pageTemplate = template.Must(template.New("status.html.tmpl").ParseFS(templateFS, "templates/status.html.tmpl"))

const (
	// sparklineSamples caps how many recent response-time points are drawn.
	sparklineSamples = 40
	// uptimeBarDays is the number of daily segments in each check's uptime bar.
	uptimeBarDays = 90
	// maxIncidents caps how many past incidents are listed per check (most
	// recent first) so a long-lived, flaky check doesn't produce an
	// unbounded page.
	maxIncidents = 10
)

// DayUptime is one segment (one calendar day, UTC) of a 90-day uptime bar.
type DayUptime struct {
	Date        string // YYYY-MM-DD
	Percent     float64
	HasData     bool
	Class       string // "good", "warn", "bad", or "unknown" — text/tooltip always carries the same info, so color is never the only signal.
	TooltipText string // e.g. "2026-09-01: 100.00% uptime" or "2026-09-01: no data"
}

// Incident is one down -> up (or down -> still down) span derived purely
// from consecutive success/failure transitions in the history file.
type Incident struct {
	StartDisplay string
	EndDisplay   string // "" when Ongoing
	DurationText string
	Error        string
	Ongoing      bool
}

// CheckStatus is the per-check view model rendered on the status page.
type CheckStatus struct {
	Name               string
	URL                string
	State              string // "UP", "DOWN", or "UNKNOWN"
	StateClass         string // css class suffix: "up", "down", "unknown"
	LastCheckedDisplay string
	LastError          string
	Uptime24h          string
	Uptime7d           string
	Uptime30d          string
	SparklineSVG       template.HTML
	UptimeBar          []DayUptime
	Incidents          []Incident
}

// PageData is the full view model passed to the HTML template.
type PageData struct {
	Title              string
	Description        string
	LogoURL            string
	AccentColor        string
	GeneratedAtDisplay string
	OverallState       string // e.g. "All Systems Operational", "Partial Outage", "Major Outage"
	OverallStateClass  string // "up", "partial", "down", or "unknown"
	Checks             []CheckStatus
}

// BuildPageData derives the page view model from config (for the set of
// checks, their display order, and status_page cosmetics) and the full
// history record set, as of now. "Current state" is inferred from each
// check's most recent history record: no persisted daemon state is
// required to generate a status page, which keeps sitewatch a single
// append-only file with no database.
func BuildPageData(cfg *config.Config, records []history.Record, now time.Time) PageData {
	data := PageData{
		Title:              firstNonEmpty(cfg.StatusPage.Title, "Status"),
		Description:        cfg.StatusPage.Description,
		LogoURL:            cfg.StatusPage.LogoURL,
		AccentColor:        firstNonEmpty(cfg.StatusPage.AccentColor, "#2563eb"),
		GeneratedAtDisplay: now.UTC().Format("2006-01-02 15:04:05 UTC"),
	}

	upCount, downCount := 0, 0
	for _, chk := range cfg.Checks {
		checkRecords := history.ForCheck(records, chk.Name)
		sort.Slice(checkRecords, func(i, j int) bool {
			return checkRecords[i].Time.Before(checkRecords[j].Time)
		})

		cs := CheckStatus{
			Name:               chk.Name,
			URL:                chk.URL,
			State:              "UNKNOWN",
			StateClass:         "unknown",
			LastCheckedDisplay: "never",
		}

		if n := len(checkRecords); n > 0 {
			latest := checkRecords[n-1]
			cs.LastCheckedDisplay = latest.Time.UTC().Format("2006-01-02 15:04:05 UTC")
			if latest.Success {
				cs.State = "UP"
				cs.StateClass = "up"
				upCount++
			} else {
				cs.State = "DOWN"
				cs.StateClass = "down"
				cs.LastError = latest.Error
				downCount++
			}
		}

		cs.Uptime24h = formatUptime(history.PercentInWindow(records, chk.Name, now.Add(-24*time.Hour), now))
		cs.Uptime7d = formatUptime(history.PercentInWindow(records, chk.Name, now.Add(-7*24*time.Hour), now))
		cs.Uptime30d = formatUptime(history.PercentInWindow(records, chk.Name, now.Add(-30*24*time.Hour), now))

		cs.SparklineSVG = template.HTML(Sparkline(responseTimes(checkRecords, sparklineSamples), 160, 32))
		cs.UptimeBar = buildUptimeBar(records, chk.Name, now, uptimeBarDays)
		cs.Incidents = buildIncidents(checkRecords, now, maxIncidents)

		data.Checks = append(data.Checks, cs)
	}

	switch {
	case len(cfg.Checks) == 0:
		data.OverallState = "No Checks Configured"
		data.OverallStateClass = "unknown"
	case downCount == 0 && upCount == len(cfg.Checks):
		data.OverallState = "All Systems Operational"
		data.OverallStateClass = "up"
	case upCount == 0:
		data.OverallState = "Major Outage"
		data.OverallStateClass = "down"
	default:
		data.OverallState = "Partial Outage"
		data.OverallStateClass = "partial"
	}

	return data
}

// buildUptimeBar computes one DayUptime segment per calendar day (UTC) for
// the last `days` days, oldest first, ending with "today" (the partial day
// containing `now`).
func buildUptimeBar(records []history.Record, checkName string, now time.Time, days int) []DayUptime {
	nowUTC := now.UTC()
	today := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)

	bar := make([]DayUptime, 0, days)
	for i := days - 1; i >= 0; i-- {
		dayStart := today.AddDate(0, 0, -i)
		windowEnd := dayStart.Add(24 * time.Hour)
		if windowEnd.After(nowUTC) {
			windowEnd = nowUTC
		}

		d := DayUptime{Date: dayStart.Format("2006-01-02")}
		u := history.PercentInWindow(records, checkName, dayStart, windowEnd)
		if !u.OK {
			d.Class = "unknown"
			d.TooltipText = fmt.Sprintf("%s: no data", d.Date)
		} else {
			d.HasData = true
			d.Percent = u.Percent
			switch {
			case u.Percent >= 99.9:
				d.Class = "good"
			case u.Percent >= 95:
				d.Class = "warn"
			default:
				d.Class = "bad"
			}
			d.TooltipText = fmt.Sprintf("%s: %.2f%% uptime", d.Date, u.Percent)
		}
		bar = append(bar, d)
	}
	return bar
}

// buildIncidents walks records (already sorted oldest-first, one check)
// and turns every down-streak into an Incident: it starts at the first
// failing record after a success (or at the start of history), and ends at
// the next successful record, or is still Ongoing if the streak runs to
// the end of the records.
func buildIncidents(records []history.Record, now time.Time, max int) []Incident {
	var incidents []Incident
	var down bool
	var start time.Time
	var lastErr string

	for _, r := range records {
		if !r.Success {
			if !down {
				down = true
				start = r.Time
			}
			if r.Error != "" {
				lastErr = r.Error
			}
			continue
		}
		if down {
			incidents = append(incidents, Incident{
				StartDisplay: start.UTC().Format("2006-01-02 15:04:05 UTC"),
				EndDisplay:   r.Time.UTC().Format("2006-01-02 15:04:05 UTC"),
				DurationText: formatDuration(r.Time.Sub(start)),
				Error:        lastErr,
			})
			down = false
			lastErr = ""
		}
	}
	if down {
		incidents = append(incidents, Incident{
			StartDisplay: start.UTC().Format("2006-01-02 15:04:05 UTC"),
			DurationText: formatDuration(now.Sub(start)) + " so far",
			Error:        lastErr,
			Ongoing:      true,
		})
	}

	// Most-recent-first for display.
	for i, j := 0, len(incidents)-1; i < j; i, j = i+1, j-1 {
		incidents[i], incidents[j] = incidents[j], incidents[i]
	}
	if len(incidents) > max {
		incidents = incidents[:max]
	}
	return incidents
}

// formatDuration renders a duration as a short, human-friendly string
// (e.g. "45s", "12m", "3h 5m", "2d 1h") — coarser than time.Duration's own
// String() and without sub-second precision, which isn't useful on a
// status page.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		days := int(d.Hours()) / 24
		h := int(d.Hours()) - days*24
		if h == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, h)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func formatUptime(u history.Uptime) string {
	if !u.OK {
		return "no data"
	}
	return fmt.Sprintf("%.2f%%", u.Percent)
}

// responseTimes returns the response times (successful checks only, so a
// timeout's huge/zero duration doesn't distort the scale) of the last n
// records, oldest first.
func responseTimes(records []history.Record, n int) []time.Duration {
	if len(records) > n {
		records = records[len(records)-n:]
	}
	out := make([]time.Duration, 0, len(records))
	for _, r := range records {
		out = append(out, time.Duration(r.ResponseTimeMS)*time.Millisecond)
	}
	return out
}

// Generate renders data through the embedded template into w.
func Generate(w io.Writer, data PageData) error {
	return pageTemplate.Execute(w, data)
}
