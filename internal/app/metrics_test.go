package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/state"
)

func TestWriteMetrics_Golden(t *testing.T) {
	var got bytes.Buffer
	samples := map[string]metricSample{
		"Acme \"Plumbing\"\n": {
			status:       state.StatusUp,
			responseTime: 182 * time.Millisecond,
		},
		"Unknown": {
			status:       state.StatusUnknown,
			responseTime: 0,
		},
	}

	if err := writeMetrics(&got, samples); err != nil {
		t.Fatalf("writeMetrics: %v", err)
	}

	want := "# HELP sitewatch_up Whether the check is currently up (1) or down (0).\n" +
		"# TYPE sitewatch_up gauge\n" +
		"# HELP sitewatch_response_time_ms Most recent response time in milliseconds.\n" +
		"# TYPE sitewatch_response_time_ms gauge\n" +
		"sitewatch_up{check=\"Acme \\\"Plumbing\\\"\\n\"} 1\n" +
		"sitewatch_response_time_ms{check=\"Acme \\\"Plumbing\\\"\\n\"} 182\n" +
		"sitewatch_response_time_ms{check=\"Unknown\"} 0\n"
	if got.String() != want {
		t.Fatalf("metrics output mismatch\ngot:\n%s\nwant:\n%s", got.String(), want)
	}
}

func TestMetricsHandlerUsesLiveInMemoryState(t *testing.T) {
	var statusCode atomic.Int32
	statusCode.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(statusCode.Load()))
	}))
	defer server.Close()

	historyPath := filepath.Join(t.TempDir(), "history.jsonl")
	d, _ := newTestDaemon(t, server, historyPath, nil, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	check := config.Check{
		Name:          "Homepage",
		URL:           server.URL,
		Timeout:       config.Duration(time.Second),
		ExpectStatus:  http.StatusOK,
		FailThreshold: 1,
	}

	d.runOnce(context.Background(), check)
	if err := os.Remove(historyPath); err != nil {
		t.Fatalf("remove history to verify scrape uses memory: %v", err)
	}

	first := httptest.NewRecorder()
	d.metricsHandler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", first.Code)
	}
	if first.Header().Get("Content-Type") != metricsContentType {
		t.Fatalf("unexpected content type: %q", first.Header().Get("Content-Type"))
	}
	if !strings.Contains(first.Body.String(), "sitewatch_up{check=\"Homepage\"} 1\n") {
		t.Fatalf("expected live UP metric, got:\n%s", first.Body.String())
	}
	if !strings.Contains(first.Body.String(), "sitewatch_response_time_ms{check=\"Homepage\"} ") {
		t.Fatalf("expected response-time metric, got:\n%s", first.Body.String())
	}

	statusCode.Store(http.StatusInternalServerError)
	d.runOnce(context.Background(), check)
	second := httptest.NewRecorder()
	d.metricsHandler().ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(second.Body.String(), "sitewatch_up{check=\"Homepage\"} 0\n") {
		t.Fatalf("expected live DOWN metric after a failed check, got:\n%s", second.Body.String())
	}
}

func TestStartMetricsServerServesAndShutsDown(t *testing.T) {
	d := &daemon{
		metrics: map[string]metricSample{
			"Homepage": {status: state.StatusUp, responseTime: 42 * time.Millisecond},
		},
		log: &bytes.Buffer{},
	}
	server, listener, done, err := startMetricsServer(d, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startMetricsServer: %v", err)
	}
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.StatusCode)
	}
	if !strings.Contains(body.String(), "sitewatch_up{check=\"Homepage\"} 1") {
		t.Fatalf("metrics endpoint response missing UP metric:\n%s", body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown metrics server: %v", err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("expected ErrServerClosed, got %v", err)
	}
}
