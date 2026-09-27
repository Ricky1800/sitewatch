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

	for _, want := range []string{"<html", "</html>", "Homepage", "https://example.com", "<svg", "UP"} {
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
	var buf bytes.Buffer
	if err := Generate(&buf, data); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(buf.String(), "No checks configured") {
		t.Error("expected placeholder text when no checks are configured")
	}
}
