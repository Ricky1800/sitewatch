package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var buf bytes.Buffer
	code := RunVersion(&buf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(buf.String(), Version) {
		t.Errorf("expected output to contain version %q, got %q", Version, buf.String())
	}
}
