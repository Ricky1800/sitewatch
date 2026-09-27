package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStatus_GeneratesPage(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(dir, "history.jsonl")
	cfgPath := writeConfig(t, dir, "checks:\n  - name: Homepage\n    url: https://example.com\nhistory:\n  path: "+histPath+"\n")

	histContents := `{"time":"2026-01-01T00:00:00Z","check":"Homepage","success":true,"status_code":200,"response_time_ms":42}` + "\n"
	if err := os.WriteFile(histPath, []byte(histContents), 0o644); err != nil {
		t.Fatalf("write history: %v", err)
	}

	outPath := filepath.Join(dir, "status.html")
	var stdout, stderr bytes.Buffer
	code := RunStatus([]string{"--config", cfgPath, "--out", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected status page to be written: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "Homepage") {
		t.Error("expected status page to mention the check name")
	}
	if !strings.Contains(html, "UP") {
		t.Error("expected status page to show UP state")
	}
}

func TestRunStatus_MissingConfig_ExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunStatus([]string{"--config", "/nonexistent/sitewatch.yaml"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
}
