package status

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/history"
)

func testConfig() *config.Config {
	return &config.Config{
		Checks: []config.Check{
			{Name: "Homepage", URL: "https://example.com"},
			{Name: "API", URL: "https://api.example.com"},
			{Name: "NeverChecked", URL: "https://new.example.com"},
		},
	}
}

func TestBuildPageData_StateFromLatestRecord(t *testing.T) {
	now := time.Unix(1_000_000, 0).UTC()
	records := []history.Record{
		{Check: "Homepage", Time: now.Add(-2 * time.Hour), Success: true, ResponseTimeMS: 50},
		{Check: "Homepage", Time: now.Add(-1 * time.Hour), Success: true, ResponseTimeMS: 60},
		{Check: "API", Time: now.Add(-30 * time.Minute), Success: false, Error: "timeout"},
	}
	data := BuildPageData(testConfig(), records, now)

	byName := map[string]CheckStatus{}
	for _, c := range data.Checks {
		byName[c.Name] = c
	}

	if got := byName["Homepage"].State; got != "UP" {
		t.Errorf("expected Homepage UP, got %s", got)
	}
	if got := byName["API"].State; got != "DOWN" {
		t.Errorf("expected API DOWN, got %s", got)
	}
	if got := byName["API"].LastError; got != "timeout" {
		t.Errorf("expected API last error 'timeout', got %q", got)
	}
	if got := byName["NeverChecked"].State; got != "UNKNOWN" {
		t.Errorf("expected NeverChecked UNKNOWN, got %s", got)
	}
	if got := byName["NeverChecked"].LastCheckedDisplay; got != "never" {
		t.Errorf("expected 'never', got %q", got)
	}
}

func TestBuildPageData_UptimeWindows(t *testing.T) {
	now := time.Unix(2_000_000, 0).UTC()
	var records []history.Record
	// 10 records strictly within the last 24h (i=1..10, never exactly at
	// "now" which is the window's exclusive upper bound), 8 successes.
	for i := 1; i <= 10; i++ {
		records = append(records, history.Record{
			Check:   "Homepage",
			Time:    now.Add(-time.Duration(i) * time.Hour),
			Success: i <= 8,
		})
	}
	data := BuildPageData(testConfig(), records, now)
	var hp CheckStatus
	for _, c := range data.Checks {
		if c.Name == "Homepage" {
			hp = c
		}
	}
	if hp.Uptime24h != "80.00%" {
		t.Errorf("expected 80.00%% uptime24h, got %s", hp.Uptime24h)
	}
}

func TestBuildPageData_NoDataUptime(t *testing.T) {
	now := time.Now()
	data := BuildPageData(testConfig(), nil, now)
	for _, c := range data.Checks {
		if c.Uptime24h != "no data" {
			t.Errorf("expected 'no data' for %s, got %s", c.Name, c.Uptime24h)
		}
	}
}

func TestGenerate_ProducesValidHTMLWithExpectedContent(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	records := []history.Record{
		{Check: "Homepage", Time: now.Add(-1 * time.Hour), Success: true, ResponseTimeMS: 42},
	}
	data := BuildPageData(testConfig(), records, now)

	var buf bytes.Buffer
	if err := Generate(&buf, data); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"<html", "</html>", "Homepage", "https://example.com", "<svg", "UP",
		"Last updated", "90-day uptime", "day-segment", "Incident history",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<script src=\"http") || strings.Contains(out, "<link rel=\"stylesheet\" href=\"http") {
		t.Error("expected no external script/stylesheet references")
	}
}

func TestGenerate_NoChecksConfigured(t *testing.T) {
	data := BuildPageData(&config.Config{}, nil, time.Now())
	if data.OverallState != "No Checks Configured" {
		t.Errorf("expected 'No Checks Configured', got %q", data.OverallState)
	}
	var buf bytes.Buffer
	if err := Generate(&buf, data); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(buf.String(), "No checks configured") {
		t.Error("expected placeholder text when no checks are configured")
	}
}

func TestBuildPageData_OverallState(t *testing.T) {
	now := time.Unix(3_000_000, 0).UTC()

	allUp := []history.Record{
		{Check: "Homepage", Time: now.Add(-time.Minute), Success: true},
		{Check: "API", Time: now.Add(-time.Minute), Success: true},
		{Check: "NeverChecked", Time: now.Add(-time.Minute), Success: true},
	}
	if got := BuildPageData(testConfig(), allUp, now).OverallState; got != "All Systems Operational" {
		t.Errorf("expected All Systems Operational, got %q", got)
	}

	allDown := []history.Record{
		{Check: "Homepage", Time: now.Add(-time.Minute), Success: false},
		{Check: "API", Time: now.Add(-time.Minute), Success: false},
		{Check: "NeverChecked", Time: now.Add(-time.Minute), Success: false},
	}
	if got := BuildPageData(testConfig(), allDown, now).OverallState; got != "Major Outage" {
		t.Errorf("expected Major Outage, got %q", got)
	}

	mixed := []history.Record{
		{Check: "Homepage", Time: now.Add(-time.Minute), Success: true},
		{Check: "API", Time: now.Add(-time.Minute), Success: false},
	}
	if got := BuildPageData(testConfig(), mixed, now).OverallState; got != "Partial Outage" {
		t.Errorf("expected Partial Outage, got %q", got)
	}
}

func TestBuildPageData_UptimeBarHasOneSegmentPerDay(t *testing.T) {
	now := time.Unix(4_000_000, 0).UTC()
	records := []history.Record{
		{Check: "Homepage", Time: now.Add(-time.Hour), Success: true},
	}
	data := BuildPageData(testConfig(), records, now)
	var hp CheckStatus
	for _, c := range data.Checks {
		if c.Name == "Homepage" {
			hp = c
		}
	}
	if len(hp.UptimeBar) != uptimeBarDays {
		t.Fatalf("expected %d uptime-bar segments, got %d", uptimeBarDays, len(hp.UptimeBar))
	}
	today := hp.UptimeBar[len(hp.UptimeBar)-1]
	if !today.HasData {
		t.Errorf("expected today's segment to have data, got %+v", today)
	}
	if today.Class != "good" {
		t.Errorf("expected today's segment class 'good' (100%% uptime), got %q", today.Class)
	}
	if today.TooltipText == "" || !strings.Contains(today.TooltipText, today.Date) {
		t.Errorf("expected tooltip text to mention the date, got %q", today.TooltipText)
	}

	// A day with no records at all should be reported as no-data, not 0%.
	firstDay := hp.UptimeBar[0]
	if firstDay.HasData {
		t.Errorf("expected the earliest day (no records) to have no data, got %+v", firstDay)
	}
	if !strings.Contains(firstDay.TooltipText, "no data") {
		t.Errorf("expected 'no data' tooltip, got %q", firstDay.TooltipText)
	}
}

func TestBuildPageData_IncidentsFromTransitions(t *testing.T) {
	now := time.Unix(5_000_000, 0).UTC()
	records := []history.Record{
		{Check: "Homepage", Time: now.Add(-5 * time.Hour), Success: true},
		{Check: "Homepage", Time: now.Add(-4 * time.Hour), Success: false, Error: "connection refused"},
		{Check: "Homepage", Time: now.Add(-3*time.Hour - 30*time.Minute), Success: false, Error: "connection refused"},
		{Check: "Homepage", Time: now.Add(-3 * time.Hour), Success: true},
		{Check: "Homepage", Time: now.Add(-30 * time.Minute), Success: false, Error: "timeout"},
	}
	data := BuildPageData(testConfig(), records, now)
	var hp CheckStatus
	for _, c := range data.Checks {
		if c.Name == "Homepage" {
			hp = c
		}
	}
	if len(hp.Incidents) != 2 {
		t.Fatalf("expected 2 incidents, got %d: %+v", len(hp.Incidents), hp.Incidents)
	}
	// Most recent first: the still-ongoing incident comes first.
	if !hp.Incidents[0].Ongoing {
		t.Errorf("expected the most recent incident to be ongoing: %+v", hp.Incidents[0])
	}
	if hp.Incidents[0].Error != "timeout" {
		t.Errorf("expected ongoing incident error 'timeout', got %q", hp.Incidents[0].Error)
	}
	if hp.Incidents[1].Ongoing {
		t.Errorf("expected the earlier incident to have resolved: %+v", hp.Incidents[1])
	}
	if hp.Incidents[1].EndDisplay == "" {
		t.Error("expected a resolved incident to have an EndDisplay")
	}
	if hp.Incidents[1].Error != "connection refused" {
		t.Errorf("expected resolved incident error 'connection refused', got %q", hp.Incidents[1].Error)
	}
}

func TestBuildPageData_NoIncidentsWhenAlwaysUp(t *testing.T) {
	now := time.Unix(6_000_000, 0).UTC()
	records := []history.Record{
		{Check: "Homepage", Time: now.Add(-time.Hour), Success: true},
		{Check: "Homepage", Time: now.Add(-30 * time.Minute), Success: true},
	}
	data := BuildPageData(testConfig(), records, now)
	for _, c := range data.Checks {
		if c.Name == "Homepage" && len(c.Incidents) != 0 {
			t.Errorf("expected no incidents for an always-up check, got %+v", c.Incidents)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m"},
		{2*time.Hour + 5*time.Minute, "2h 5m"},
		{3 * time.Hour, "3h"},
		{26 * time.Hour, "1d 2h"},
		{48 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestGenerate_StatusPageConfigAppliesTitleLogoAccentDescription(t *testing.T) {
	cfg := testConfig()
	cfg.StatusPage = config.StatusPage{
		Title:       "Acme Plumbing Status",
		LogoURL:     "https://acmeplumbing.example.com/logo.png",
		AccentColor: "#15803d",
		Description: "Live status for our public-facing sites.",
	}
	data := BuildPageData(cfg, nil, time.Now())

	var buf bytes.Buffer
	if err := Generate(&buf, data); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Acme Plumbing Status",
		"https://acmeplumbing.example.com/logo.png",
		"#15803d",
		"Live status for our public-facing sites.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
}
