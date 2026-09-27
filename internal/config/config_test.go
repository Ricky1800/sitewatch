package config

import (
	"strings"
	"testing"
	"time"
)

func TestParse_Minimal(t *testing.T) {
	data := []byte(`
checks:
  - name: Example
    url: https://example.com
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(cfg.Checks))
	}
	c := cfg.Checks[0]
	if c.Interval.Std() != 60*time.Second {
		t.Errorf("expected default interval 60s, got %s", c.Interval)
	}
	if c.Timeout.Std() != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %s", c.Timeout)
	}
	if c.FailThreshold != 3 {
		t.Errorf("expected default fail_threshold 3, got %d", c.FailThreshold)
	}
	if c.SSLWarnDays != 14 {
		t.Errorf("expected default ssl_warn_days 14, got %d", c.SSLWarnDays)
	}
	if c.ExpectStatus != 200 {
		t.Errorf("expected default expected_status 200, got %d", c.ExpectStatus)
	}
	if cfg.History.Path != "sitewatch_history.jsonl" {
		t.Errorf("expected default history path, got %q", cfg.History.Path)
	}
	if cfg.StatusPage.Title != "Status" {
		t.Errorf("expected default status_page.title 'Status', got %q", cfg.StatusPage.Title)
	}
	if cfg.StatusPage.AccentColor != "#2563eb" {
		t.Errorf("expected default status_page.accent_color, got %q", cfg.StatusPage.AccentColor)
	}
}

func TestParse_StatusPageOverrides(t *testing.T) {
	data := []byte(`
status_page:
  title: "Acme Plumbing Status"
  logo_url: "https://acmeplumbing.example.com/logo.png"
  accent_color: "#15803d"
  description: "Live status for our public-facing sites."
checks:
  - name: Example
    url: https://example.com
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.StatusPage.Title != "Acme Plumbing Status" {
		t.Errorf("unexpected title: %q", cfg.StatusPage.Title)
	}
	if cfg.StatusPage.LogoURL != "https://acmeplumbing.example.com/logo.png" {
		t.Errorf("unexpected logo_url: %q", cfg.StatusPage.LogoURL)
	}
	if cfg.StatusPage.AccentColor != "#15803d" {
		t.Errorf("unexpected accent_color: %q", cfg.StatusPage.AccentColor)
	}
	if cfg.StatusPage.Description != "Live status for our public-facing sites." {
		t.Errorf("unexpected description: %q", cfg.StatusPage.Description)
	}
}

func TestParse_OverridesDefaults(t *testing.T) {
	data := []byte(`
defaults:
  interval: 30s
  timeout: 5s
  fail_threshold: 5
  ssl_warn_days: 7
checks:
  - name: Example
    url: https://example.com
    interval: 10s
    timeout: 2s
    expected_status: 201
    fail_threshold: 1
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	c := cfg.Checks[0]
	if c.Interval.Std() != 10*time.Second {
		t.Errorf("expected explicit interval 10s, got %s", c.Interval)
	}
	if c.Timeout.Std() != 2*time.Second {
		t.Errorf("expected explicit timeout 2s, got %s", c.Timeout)
	}
	if c.ExpectStatus != 201 {
		t.Errorf("expected explicit expected_status 201, got %d", c.ExpectStatus)
	}
	if c.FailThreshold != 1 {
		t.Errorf("expected explicit fail_threshold 1, got %d", c.FailThreshold)
	}

	// A second check that doesn't override anything should pick up the
	// custom defaults section, not the built-in defaults.
	data2 := append(data, []byte(`
  - name: Second
    url: https://example.org
`)...)
	cfg2, err := Parse(data2)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	second := cfg2.Checks[1]
	if second.Interval.Std() != 30*time.Second {
		t.Errorf("expected second check to inherit custom default interval 30s, got %s", second.Interval)
	}
	if second.SSLWarnDays != 7 {
		t.Errorf("expected second check to inherit custom default ssl_warn_days 7, got %d", second.SSLWarnDays)
	}
}

func TestParse_NoChecks(t *testing.T) {
	_, err := Parse([]byte(`checks: []`))
	if err == nil {
		t.Fatal("expected error for empty checks list")
	}
	if !strings.Contains(err.Error(), "at least one check is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_MissingURLReportsLine(t *testing.T) {
	data := []byte(`checks:
  - name: NoURL
    interval: 30s
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	ve, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	found := false
	for _, fe := range ve {
		if fe.Path == "checks[0].url" {
			found = true
			if fe.Line != 2 {
				t.Errorf("expected url error on line 2, got line %d", fe.Line)
			}
		}
	}
	if !found {
		t.Fatalf("expected a checks[0].url error, got: %v", ve)
	}
}

func TestParse_InvalidURL(t *testing.T) {
	data := []byte(`checks:
  - name: Bad
    url: "not a url"
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "checks[0].url") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_NonHTTPScheme(t *testing.T) {
	data := []byte(`checks:
  - name: Bad
    url: "ftp://example.com/file"
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "scheme must be http or https") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_DuplicateNames(t *testing.T) {
	data := []byte(`checks:
  - name: Dup
    url: https://example.com
  - name: Dup
    url: https://example.org
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "duplicate check name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_TimeoutGreaterThanInterval(t *testing.T) {
	data := []byte(`checks:
  - name: Slow
    url: https://example.com
    interval: 5s
    timeout: 10s
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "must not be greater than interval") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_InvalidExpectedStatus(t *testing.T) {
	data := []byte(`checks:
  - name: Bad
    url: https://example.com
    expected_status: 999
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "expected_status") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_NegativeMaxResponseMS(t *testing.T) {
	data := []byte(`checks:
  - name: Bad
    url: https://example.com
    max_response_ms: -1
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "max_response_ms") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_EmailAlertRequiresPasswordEnvNotPassword(t *testing.T) {
	data := []byte(`checks:
  - name: Example
    url: https://example.com
alerts:
  email:
    smtp_host: smtp.example.com
    smtp_port: 587
    from: alerts@example.com
    to: ["me@example.com"]
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error for missing password_env")
	}
	if !strings.Contains(err.Error(), "password_env") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_EmailAlertValid(t *testing.T) {
	data := []byte(`checks:
  - name: Example
    url: https://example.com
alerts:
  email:
    smtp_host: smtp.example.com
    smtp_port: 587
    from: alerts@example.com
    to: ["me@example.com"]
    password_env: SITEWATCH_SMTP_PASSWORD
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Alerts.Email == nil {
		t.Fatal("expected email alert to be set")
	}
	if cfg.Alerts.Email.PasswordEnv != "SITEWATCH_SMTP_PASSWORD" {
		t.Errorf("unexpected password env var name: %q", cfg.Alerts.Email.PasswordEnv)
	}
}

func TestParse_DiscordRequiresWebhookURL(t *testing.T) {
	data := []byte(`checks:
  - name: Example
    url: https://example.com
alerts:
  discord: {}
`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "alerts.discord.webhook_url") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_InvalidYAMLSyntax(t *testing.T) {
	_, err := Parse([]byte("checks: [\n"))
	if err == nil {
		t.Fatal("expected a yaml parse error")
	}
}

func TestParse_MultipleErrorsAggregated(t *testing.T) {
	data := []byte(`checks:
  - name: ""
    url: ""
  - name: ""
    url: ""
`)
	_, err := Parse(data)
	ve, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T (%v)", err, err)
	}
	if len(ve) < 4 {
		t.Errorf("expected at least 4 aggregated errors, got %d: %v", len(ve), ve)
	}
}

func TestEmailAlert_PasswordFromEnv(t *testing.T) {
	t.Setenv("SITEWATCH_TEST_SMTP_PW", "hunter2")
	e := EmailAlert{PasswordEnv: "SITEWATCH_TEST_SMTP_PW"}
	if got := e.Password(); got != "hunter2" {
		t.Errorf("expected password from env, got %q", got)
	}

	e2 := EmailAlert{}
	if got := e2.Password(); got != "" {
		t.Errorf("expected empty password when password_env unset, got %q", got)
	}
}
