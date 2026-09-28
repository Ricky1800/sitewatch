package app

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ricky1800/sitewatch/internal/state"
)

const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

type metricSample struct {
	status       state.Status
	responseTime time.Duration
}

func startMetricsServer(d *daemon, addr string) (*http.Server, net.Listener, chan error, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, nil, err
	}

	server := &http.Server{
		Handler:           d.metricsHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()
	return server, listener, done, nil
}

func (d *daemon) metricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", metricsContentType)
		d.mu.Lock()
		samples := make(map[string]metricSample, len(d.metrics))
		for name, sample := range d.metrics {
			samples[name] = sample
		}
		d.mu.Unlock()

		if err := writeMetrics(w, samples); err != nil {
			d.logf("metrics endpoint write: %v\n", err)
		}
	})
}

func writeMetrics(w io.Writer, samples map[string]metricSample) error {
	for _, line := range []string{
		"# HELP sitewatch_up Whether the check is currently up (1) or down (0).",
		"# TYPE sitewatch_up gauge",
		"# HELP sitewatch_response_time_ms Most recent response time in milliseconds.",
		"# TYPE sitewatch_response_time_ms gauge",
	} {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	names := make([]string, 0, len(samples))
	for name := range samples {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		sample := samples[name]
		label := escapeMetricLabel(name)
		if sample.status != state.StatusUnknown {
			up := 0
			if sample.status == state.StatusUp {
				up = 1
			}
			if _, err := fmt.Fprintf(w, "sitewatch_up{check=\"%s\"} %d\n", label, up); err != nil {
				return err
			}
		}

		milliseconds := float64(sample.responseTime) / float64(time.Millisecond)
		value := strconv.FormatFloat(milliseconds, 'f', -1, 64)
		if _, err := fmt.Fprintf(w, "sitewatch_response_time_ms{check=\"%s\"} %s\n", label, value); err != nil {
			return err
		}
	}
	return nil
}

func escapeMetricLabel(value string) string {
	var escaped strings.Builder
	for _, r := range value {
		switch r {
		case '\\':
			escaped.WriteString("\\\\")
		case '"':
			escaped.WriteString("\\\"")
		case '\n':
			escaped.WriteString("\\n")
		default:
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}
