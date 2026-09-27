package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ricky1800/sitewatch/internal/checker"
)

func writeConfig(t *testing.T, dir string, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "sitewatch.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRunCheck_AllPass_ExitsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, "checks:\n  - name: Test\n    url: "+srv.URL+"\n")

	var stdout, stderr bytes.Buffer
	code := RunCheck([]string{"--config", cfgPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Test") || !strings.Contains(stdout.String(), "OK") {
		t.Errorf("expected table output with check name and OK, got:\n%s", stdout.String())
	}
}

func TestRunCheck_Failure_ExitsOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, "checks:\n  - name: Broken\n    url: "+srv.URL+"\n")

	var stdout, stderr bytes.Buffer
	code := RunCheck([]string{"--config", cfgPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 on failing check, got %d", code)
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in table output, got:\n%s", stdout.String())
	}
}

func TestRunCheck_JSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, "checks:\n  - name: Test\n    url: "+srv.URL+"\n")

	var stdout, stderr bytes.Buffer
	code := RunCheck([]string{"--config", cfgPath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	var results []checker.Result
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("expected valid JSON output, got error %v:\n%s", err, stdout.String())
	}
	if len(results) != 1 || results[0].Name != "Test" || !results[0].Success {
		t.Errorf("unexpected JSON results: %+v", results)
	}
}

func TestRunCheck_MissingConfig_ExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCheck([]string{"--config", "/nonexistent/sitewatch.yaml"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for missing config, got %d", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected an error message on stderr")
	}
}

func TestRunCheck_InvalidConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, "checks: []\n")

	var stdout, stderr bytes.Buffer
	code := RunCheck([]string{"--config", cfgPath}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for invalid config, got %d", code)
	}
}
