// Package config loads and validates sitewatch.yaml.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Defaults holds global values applied to any Check field left unset.
type Defaults struct {
	Interval      Duration `yaml:"interval"`
	Timeout       Duration `yaml:"timeout"`
	FailThreshold int      `yaml:"fail_threshold"`
	SSLWarnDays   int      `yaml:"ssl_warn_days"`
}

// DiscordAlert configures a Discord incoming-webhook alert channel.
type DiscordAlert struct {
	WebhookURL string `yaml:"webhook_url"`
}

// SlackAlert configures a Slack incoming-webhook alert channel.
type SlackAlert struct {
	WebhookURL string `yaml:"webhook_url"`
}

// WebhookAlert configures a generic JSON POST webhook alert channel.
type WebhookAlert struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
}

// EmailAlert configures SMTP email alerts. The password is never read from
// the config file itself: PasswordEnv names an environment variable that
// holds it at runtime.
type EmailAlert struct {
	SMTPHost    string   `yaml:"smtp_host"`
	SMTPPort    int      `yaml:"smtp_port"`
	Username    string   `yaml:"username"`
	PasswordEnv string   `yaml:"password_env"`
	From        string   `yaml:"from"`
	To          []string `yaml:"to"`
}

// Password reads the SMTP password from the environment variable named by
// PasswordEnv. It returns "" if PasswordEnv is empty or unset.
func (e EmailAlert) Password() string {
	if e.PasswordEnv == "" {
		return ""
	}
	return os.Getenv(e.PasswordEnv)
}

// Alerts groups every configured alert channel. Channels left as their zero
// value are simply not used.
type Alerts struct {
	Discord *DiscordAlert `yaml:"discord"`
	Slack   *SlackAlert   `yaml:"slack"`
	Webhook *WebhookAlert `yaml:"webhook"`
	Email   *EmailAlert   `yaml:"email"`
}

// History configures the append-only JSONL history file.
type History struct {
	Path       string `yaml:"path"`
	MaxAgeDays int    `yaml:"max_age_days"`
}

// StatusPage configures cosmetic details of the generated static status
// page. Every field is optional; unset fields fall back to sensible
// defaults in applyDefaults so `sitewatch status` works with zero config.
type StatusPage struct {
	Title       string `yaml:"title"`
	LogoURL     string `yaml:"logo_url"`
	AccentColor string `yaml:"accent_color"`
	Description string `yaml:"description"`
}

// Check describes one monitored endpoint. Zero-valued fields fall back to
// Config.Defaults at load time (see applyDefaults).
type Check struct {
	Name          string   `yaml:"name"`
	URL           string   `yaml:"url"`
	Interval      Duration `yaml:"interval"`
	Timeout       Duration `yaml:"timeout"`
	ExpectStatus  int      `yaml:"expected_status"`
	BodyContains  string   `yaml:"body_contains"`
	MaxResponseMS int      `yaml:"max_response_ms"`
	SSLWarnDays   int      `yaml:"ssl_warn_days"`
	FailThreshold int      `yaml:"fail_threshold"`
}

// Config is the fully parsed and defaulted contents of sitewatch.yaml.
type Config struct {
	Defaults   Defaults   `yaml:"defaults"`
	Alerts     Alerts     `yaml:"alerts"`
	Checks     []Check    `yaml:"checks"`
	History    History    `yaml:"history"`
	StatusPage StatusPage `yaml:"status_page"`
}

// defaultConfig returns the built-in defaults applied before the YAML
// document's own "defaults:" section is merged in.
func defaultConfig() Config {
	return Config{
		Defaults: Defaults{
			Interval:      Duration(60 * time.Second),
			Timeout:       Duration(10 * time.Second),
			FailThreshold: 3,
			SSLWarnDays:   14,
		},
		History: History{
			Path:       "sitewatch_history.jsonl",
			MaxAgeDays: 90,
		},
	}
}

// Load reads, parses, defaults, and validates the config file at path. On any
// validation failure it returns a *ValidationErrors describing every problem
// found, each annotated with the offending YAML line number when available.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses raw YAML bytes into a validated Config. It is separated from
// Load so tests can exercise it without touching the filesystem.
func Parse(data []byte) (*Config, error) {
	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Already parsed above; this should never fail if the first pass
		// succeeded, but guard anyway.
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	applyDefaults(&cfg)

	if errs := validate(&cfg, &doc); len(errs) > 0 {
		return nil, errs
	}
	return &cfg, nil
}

// applyDefaults fills zero-valued per-check fields from cfg.Defaults.
func applyDefaults(cfg *Config) {
	for i := range cfg.Checks {
		c := &cfg.Checks[i]
		if c.Interval == 0 {
			c.Interval = cfg.Defaults.Interval
		}
		if c.Timeout == 0 {
			c.Timeout = cfg.Defaults.Timeout
		}
		if c.FailThreshold == 0 {
			c.FailThreshold = cfg.Defaults.FailThreshold
		}
		if c.SSLWarnDays == 0 {
			c.SSLWarnDays = cfg.Defaults.SSLWarnDays
		}
		if c.ExpectStatus == 0 {
			c.ExpectStatus = 200
		}
	}
	if cfg.History.Path == "" {
		cfg.History.Path = "sitewatch_history.jsonl"
	}
	if cfg.History.MaxAgeDays == 0 {
		cfg.History.MaxAgeDays = 90
	}
	if strings.TrimSpace(cfg.StatusPage.Title) == "" {
		cfg.StatusPage.Title = "Status"
	}
	if strings.TrimSpace(cfg.StatusPage.AccentColor) == "" {
		cfg.StatusPage.AccentColor = "#2563eb"
	}
}

// FieldError describes a single validation problem, with the YAML line
// number it originated from when one could be resolved (0 otherwise).
type FieldError struct {
	Path    string
	Line    int
	Message string
}

func (e FieldError) String() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s: %s", e.Line, e.Path, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationErrors aggregates every FieldError found while validating a
// Config. It implements error so callers can use errors.As.
type ValidationErrors []FieldError

func (e ValidationErrors) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "config validation failed (%d error(s)):\n", len(e))
	for _, fe := range e {
		fmt.Fprintf(&b, "  - %s\n", fe.String())
	}
	return strings.TrimRight(b.String(), "\n")
}

func validate(cfg *Config, doc *yaml.Node) ValidationErrors {
	var errs ValidationErrors

	if len(cfg.Checks) == 0 {
		errs = append(errs, FieldError{Path: "checks", Line: lineOf(doc, "checks"), Message: "at least one check is required"})
	}

	seen := make(map[string]int) // name -> first index
	for i, c := range cfg.Checks {
		base := fmt.Sprintf("checks[%d]", i)

		if strings.TrimSpace(c.Name) == "" {
			errs = append(errs, FieldError{Path: base + ".name", Line: lineOf(doc, "checks", i), Message: "is required"})
		} else if first, dup := seen[c.Name]; dup {
			errs = append(errs, FieldError{
				Path:    base + ".name",
				Line:    lineOf(doc, "checks", i, "name"),
				Message: fmt.Sprintf("duplicate check name (also used by checks[%d])", first),
			})
		} else {
			seen[c.Name] = i
		}

		if strings.TrimSpace(c.URL) == "" {
			errs = append(errs, FieldError{Path: base + ".url", Line: lineOf(doc, "checks", i), Message: "is required"})
		} else if u, err := url.Parse(c.URL); err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, FieldError{
				Path:    base + ".url",
				Line:    lineOf(doc, "checks", i, "url"),
				Message: fmt.Sprintf("must be an absolute http(s) URL (got %q)", c.URL),
			})
		} else if u.Scheme != "http" && u.Scheme != "https" {
			errs = append(errs, FieldError{
				Path:    base + ".url",
				Line:    lineOf(doc, "checks", i, "url"),
				Message: fmt.Sprintf("scheme must be http or https (got %q)", u.Scheme),
			})
		}

		if c.Interval <= 0 {
			errs = append(errs, FieldError{Path: base + ".interval", Line: lineOf(doc, "checks", i, "interval"), Message: "must be > 0"})
		}
		if c.Timeout <= 0 {
			errs = append(errs, FieldError{Path: base + ".timeout", Line: lineOf(doc, "checks", i, "timeout"), Message: "must be > 0"})
		}
		if c.Timeout > 0 && c.Interval > 0 && c.Timeout > c.Interval {
			errs = append(errs, FieldError{
				Path:    base + ".timeout",
				Line:    lineOf(doc, "checks", i, "timeout"),
				Message: "must not be greater than interval",
			})
		}
		if c.ExpectStatus < 100 || c.ExpectStatus > 599 {
			errs = append(errs, FieldError{
				Path:    base + ".expected_status",
				Line:    lineOf(doc, "checks", i, "expected_status"),
				Message: fmt.Sprintf("must be a valid HTTP status code (got %d)", c.ExpectStatus),
			})
		}
		if c.MaxResponseMS < 0 {
			errs = append(errs, FieldError{Path: base + ".max_response_ms", Line: lineOf(doc, "checks", i, "max_response_ms"), Message: "must be >= 0"})
		}
		if c.SSLWarnDays < 0 {
			errs = append(errs, FieldError{Path: base + ".ssl_warn_days", Line: lineOf(doc, "checks", i, "ssl_warn_days"), Message: "must be >= 0"})
		}
		if c.FailThreshold < 1 {
			errs = append(errs, FieldError{Path: base + ".fail_threshold", Line: lineOf(doc, "checks", i, "fail_threshold"), Message: "must be >= 1"})
		}
	}

	if d := cfg.Alerts.Discord; d != nil && strings.TrimSpace(d.WebhookURL) == "" {
		errs = append(errs, FieldError{Path: "alerts.discord.webhook_url", Line: lineOf(doc, "alerts", "discord"), Message: "is required when alerts.discord is configured"})
	}
	if s := cfg.Alerts.Slack; s != nil && strings.TrimSpace(s.WebhookURL) == "" {
		errs = append(errs, FieldError{Path: "alerts.slack.webhook_url", Line: lineOf(doc, "alerts", "slack"), Message: "is required when alerts.slack is configured"})
	}
	if w := cfg.Alerts.Webhook; w != nil && strings.TrimSpace(w.URL) == "" {
		errs = append(errs, FieldError{Path: "alerts.webhook.url", Line: lineOf(doc, "alerts", "webhook"), Message: "is required when alerts.webhook is configured"})
	}
	if e := cfg.Alerts.Email; e != nil {
		if strings.TrimSpace(e.SMTPHost) == "" {
			errs = append(errs, FieldError{Path: "alerts.email.smtp_host", Line: lineOf(doc, "alerts", "email"), Message: "is required when alerts.email is configured"})
		}
		if e.SMTPPort <= 0 {
			errs = append(errs, FieldError{Path: "alerts.email.smtp_port", Line: lineOf(doc, "alerts", "email", "smtp_port"), Message: "must be > 0"})
		}
		if strings.TrimSpace(e.From) == "" {
			errs = append(errs, FieldError{Path: "alerts.email.from", Line: lineOf(doc, "alerts", "email", "from"), Message: "is required when alerts.email is configured"})
		}
		if len(e.To) == 0 {
			errs = append(errs, FieldError{Path: "alerts.email.to", Line: lineOf(doc, "alerts", "email", "to"), Message: "must list at least one recipient"})
		}
		if strings.TrimSpace(e.PasswordEnv) == "" {
			errs = append(errs, FieldError{
				Path:    "alerts.email.password_env",
				Line:    lineOf(doc, "alerts", "email"),
				Message: "is required; sitewatch never stores SMTP passwords in the config file, so name the environment variable to read it from",
			})
		}
	}

	if cfg.History.MaxAgeDays < 0 {
		errs = append(errs, FieldError{Path: "history.max_age_days", Line: lineOf(doc, "history", "max_age_days"), Message: "must be >= 0"})
	}

	return errs
}

// lineOf walks doc (a *yaml.Node produced by unmarshaling into a yaml.Node)
// following the given sequence of map keys (string) and sequence indices
// (int), returning the 1-based source line of the node found, or 0 if the
// path doesn't resolve (e.g. an optional section that wasn't present).
func lineOf(doc *yaml.Node, path ...any) int {
	node := doc
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, p := range path {
		switch key := p.(type) {
		case string:
			if node.Kind != yaml.MappingNode {
				return 0
			}
			found := false
			for i := 0; i+1 < len(node.Content); i += 2 {
				if node.Content[i].Value == key {
					node = node.Content[i+1]
					found = true
					break
				}
			}
			if !found {
				return 0
			}
		case int:
			if node.Kind != yaml.SequenceNode || key < 0 || key >= len(node.Content) {
				return 0
			}
			node = node.Content[key]
		default:
			return 0
		}
	}
	return node.Line
}
