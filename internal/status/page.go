// Package status generates a self-contained static HTML status page (no
// external CSS/JS/fonts) from a config and history records.
package status

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"sort"
	"time"

	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/history"
)

//go:embed templates/status.html.tmpl
var templateFS embed.FS

var pageTemplate = template.Must(template.New("status.html.tmpl").ParseFS(templateFS, "templates/status.html.tmpl"))

// sparklineSamples caps how many recent response-time points are drawn.
const sparklineSamples = 40

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
}

// PageData is the full view model passed to the HTML template.
type PageData struct {
	GeneratedAtDisplay string
	Checks             []CheckStatus
}

// BuildPageData derives the page view model from config (for the set of
// checks and their display order) and the full history record set, as of
// now. "Current state" is inferred from each check's most recent history
// record: no persisted daemon state is required to generate a status page,
// which keeps sitewatch a single append-only file with no database.
func BuildPageData(cfg *config.Config, records []history.Record, now time.Time) PageData {
	data := PageData{
		GeneratedAtDisplay: now.UTC().Format("2006-01-02 15:04:05 UTC"),
	}

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
			} else {
				cs.State = "DOWN"
				cs.StateClass = "down"
				cs.LastError = latest.Error
			}
		}

		cs.Uptime24h = formatUptime(history.PercentInWindow(records, chk.Name, now.Add(-24*time.Hour), now))
		cs.Uptime7d = formatUptime(history.PercentInWindow(records, chk.Name, now.Add(-7*24*time.Hour), now))
		cs.Uptime30d = formatUptime(history.PercentInWindow(records, chk.Name, now.Add(-30*24*time.Hour), now))

		cs.SparklineSVG = template.HTML(Sparkline(responseTimes(checkRecords, sparklineSamples), 160, 32))

		data.Checks = append(data.Checks, cs)
	}

	return data
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
