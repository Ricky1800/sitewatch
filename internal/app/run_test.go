package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ricky1800/sitewatch/internal/alert"
	"github.com/Ricky1800/sitewatch/internal/checker"
	"github.com/Ricky1800/sitewatch/internal/clock"
	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/history"
	"github.com/Ricky1800/sitewatch/internal/state"
)

// newTestDaemon builds a daemon wired to an httptest server and a fake
// clock, so runOnce can be driven deterministically without any real
// sleeping or network flakiness.
func newTestDaemon(t *testing.T, srv *httptest.Server, histPath string, alertNotifier alert.Notifier, now time.Time) (*daemon, *clock.Fake) {
	t.Helper()
	fc := clock.NewFake(now)
	var logBuf bytes.Buffer
	d := &daemon{
		cfg: &config.Config{
			History: config.History{Path: histPath},
		},
		checker: &checker.Checker{Client: srv.Client(), Clock: fc},
		clock:   fc,
		alerts:  &alert.Manager{},
		states:  make(map[string]*state.CheckState),
		log:     &logBuf,
	}
	if alertNotifier != nil {
		d.alerts.Notifiers = []alert.Notifier{alertNotifier}
	}
	return d, fc
}

type recordingNotifier struct {
	events []alert.Event
}

func (r *recordingNotifier) Name() string { return "recording" }
func (r *recordingNotifier) Notify(_ context.Context, ev alert.Event) error {
	r.events = append(r.events, ev)
	return nil
}

func TestDaemon_RunOnce_AlertsOnlyOnTransitionToDown(t *testing.T) {
	failing := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	histPath := filepath.Join(dir, "history.jsonl")
	rec := &recordingNotifier{}
	d, fc := newTestDaemon(t, srv, histPath, rec, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	chk := config.Check{
		Name:          "Test",
		URL:           srv.URL,
		Interval:      config.Duration(30 * time.Second),
		Timeout:       config.Duration(2 * time.Second),
		ExpectStatus:  200,
		FailThreshold: 3,
	}

	ctx := context.Background()
	d.runOnce(ctx, chk) // failure 1: no alert yet
	fc.Advance(time.Second)
	d.runOnce(ctx, chk) // failure 2: no alert yet
	fc.Advance(time.Second)
	if len(rec.events) != 0 {
		t.Fatalf("expected no alert before fail_threshold, got %d", len(rec.events))
	}
	d.runOnce(ctx, chk) // failure 3: crosses threshold
	if len(rec.events) != 1 {
		t.Fatalf("expected exactly 1 alert at fail_threshold, got %d", len(rec.events))
	}
	if rec.events[0].Kind != alert.Down {
		t.Errorf("expected a Down event, got %v", rec.events[0].Kind)
	}

	// Further failures while down must not alert again.
	fc.Advance(time.Second)
	d.runOnce(ctx, chk)
	fc.Advance(time.Second)
	d.runOnce(ctx, chk)
	if len(rec.events) != 1 {
		t.Fatalf("expected no repeated alert while still down, got %d total", len(rec.events))
	}

	// Recovery fires an Up alert.
	failing = false
	fc.Advance(time.Minute)
	d.runOnce(ctx, chk)
	if len(rec.events) != 2 {
		t.Fatalf("expected a 2nd alert on recovery, got %d", len(rec.events))
	}
	if rec.events[1].Kind != alert.Up {
		t.Errorf("expected an Up event, got %v", rec.events[1].Kind)
	}

	// History should have one line per runOnce call (6 total).
	records, err := history.ReadAll(histPath)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 6 {
		t.Fatalf("expected 6 history records, got %d", len(records))
	}
}

func TestDaemon_RunOnce_SSLWarningOncePerDay(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	histPath := filepath.Join(dir, "history.jsonl")
	rec := &recordingNotifier{}
	// net/http/httptest's built-in test certificate has a fixed real-world
	// NotAfter of 2084-01-29 (see net/http/internal/testcert). Start the
	// fake clock a few days before that so the certificate is inside the
	// warning window without needing to mint a custom certificate.
	almostExpired := time.Date(2084, 1, 25, 0, 0, 0, 0, time.UTC)
	d, fc := newTestDaemon(t, srv, histPath, rec, almostExpired)

	chk := config.Check{
		Name:          "Secure",
		URL:           srv.URL,
		Interval:      config.Duration(30 * time.Second),
		Timeout:       config.Duration(2 * time.Second),
		ExpectStatus:  200,
		FailThreshold: 1,
		SSLWarnDays:   14,
	}

	ctx := context.Background()
	d.runOnce(ctx, chk)
	sslEvents := countKind(rec.events, alert.SSLExpiry)
	if sslEvents != 1 {
		t.Fatalf("expected exactly 1 SSL warning on first check, got %d", sslEvents)
	}

	// Same day, later: no repeat warning.
	fc.Advance(2 * time.Hour)
	d.runOnce(ctx, chk)
	if got := countKind(rec.events, alert.SSLExpiry); got != 1 {
		t.Fatalf("expected no repeat SSL warning on the same day, got %d total", got)
	}

	// Next day: warns again.
	fc.Advance(24 * time.Hour)
	d.runOnce(ctx, chk)
	if got := countKind(rec.events, alert.SSLExpiry); got != 2 {
		t.Fatalf("expected a 2nd SSL warning on the next day, got %d total", got)
	}
}

func countKind(events []alert.Event, k alert.Kind) int {
	n := 0
	for _, e := range events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

// TestRunDaemon_StartsChecksAndShutsDownCleanly is a small integration smoke
// test of the goroutine scheduler and graceful shutdown. Unlike the
// deterministic runOnce tests above, this necessarily uses a real (short)
// interval and a brief real wait, since jittered per-check scheduling is
// exactly the behavior under test.
func TestRunDaemon_StartsChecksAndShutsDownCleanly(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	histPath := filepath.Join(dir, "history.jsonl")

	d := &daemon{
		cfg: &config.Config{
			Checks: []config.Check{{
				Name:          "Test",
				URL:           srv.URL,
				Interval:      config.Duration(10 * time.Millisecond),
				Timeout:       config.Duration(2 * time.Second),
				ExpectStatus:  200,
				FailThreshold: 1,
			}},
			History: config.History{Path: histPath},
		},
		checker: &checker.Checker{Client: srv.Client(), Clock: clock.Real{}},
		clock:   clock.Real{},
		alerts:  &alert.Manager{},
		states:  make(map[string]*state.CheckState),
		log:     &bytes.Buffer{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.run(ctx)
		close(done)
	}()

	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not shut down within 2s of cancellation")
	}

	records, err := history.ReadAll(histPath)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least one history record to have been written")
	}
}
