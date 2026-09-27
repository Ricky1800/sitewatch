package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration so it can be written in sitewatch.yaml as a
// human string ("30s", "5m", "1h30m") the way the rest of the Go ecosystem
// expects, since yaml.v3 has no built-in notion of time.Duration.
type Duration time.Duration

// Std returns the underlying time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// String implements fmt.Stringer.
func (d Duration) String() string { return time.Duration(d).String() }

// UnmarshalYAML accepts either a duration string ("30s") or a bare number,
// which is interpreted as whole seconds.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err == nil {
		if s == "" {
			*d = 0
			return nil
		}
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q (line %d): %w", s, node.Line, err)
		}
		*d = Duration(parsed)
		return nil
	}

	var secs float64
	if err := node.Decode(&secs); err == nil {
		*d = Duration(time.Duration(secs * float64(time.Second)))
		return nil
	}

	return fmt.Errorf("invalid duration value at line %d: expected a string like \"30s\" or a number of seconds", node.Line)
}

// MarshalYAML renders the duration back to its string form.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}
