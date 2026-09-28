package checker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ricky1800/sitewatch/internal/clock"
	"github.com/Ricky1800/sitewatch/internal/config"
)

func mustCheck(url string) config.Check {
	return config.Check{
		Name:         "test",
		URL:          url,
		Interval:     config.Duration(30 * time.Second),
		Timeout:      config.Duration(2 * time.Second),
		ExpectStatus: 200,
	}
}

func TestChecker_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Welcome to the site"))
	}))
	defer srv.Close()

	c := &Checker{Client: srv.Client(), Clock: clock.NewFake(time.Unix(0, 0))}
	res := c.Check(context.Background(), mustCheck(srv.URL))
	if !res.Success {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if res.StatusCode != 200 {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}
}

func TestChecker_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Checker{Client: srv.Client()}
	res := c.Check(context.Background(), mustCheck(srv.URL))
	if res.Success {
		t.Fatal("expected failure for 500 response")
	}
	if res.StatusCode != 500 {
		t.Errorf("expected 500, got %d", res.StatusCode)
	}
}

func TestChecker_BodyContains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this page has nothing useful"))
	}))
	defer srv.Close()

	chk := mustCheck(srv.URL)
	chk.BodyContains = "Welcome"
	c := &Checker{Client: srv.Client()}
	res := c.Check(context.Background(), chk)
	if res.Success {
		t.Fatal("expected failure when body_contains text is missing")
	}
}

func TestChecker_ConnectionRefused(t *testing.T) {
	c := &Checker{Client: http.DefaultClient}
	res := c.Check(context.Background(), mustCheck("http://127.0.0.1:1"))
	if res.Success {
		t.Fatal("expected failure connecting to a closed port")
	}
	if res.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestChecker_TLSExpiry(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Checker{Client: srv.Client()}
	res := c.Check(context.Background(), mustCheck(srv.URL))
	if !res.Success {
		t.Fatalf("expected success, got error: %s", res.Error)
	}
	if res.TLSExpiry == nil {
		t.Fatal("expected TLS expiry to be populated for an https check")
	}
	if !res.TLSExpiry.After(time.Now()) {
		t.Errorf("expected certificate expiry in the future, got %s", res.TLSExpiry)
	}
}

func TestChecker_NoTLSExpiryOverPlainHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Checker{Client: srv.Client()}
	res := c.Check(context.Background(), mustCheck(srv.URL))
	if res.TLSExpiry != nil {
		t.Errorf("expected no TLS expiry over plain http, got %s", res.TLSExpiry)
	}
}

// --- Evaluate: pure, deterministic tests (no network, no sleeping) ---

func TestEvaluate_Success(t *testing.T) {
	chk := config.Check{ExpectStatus: 200}
	m := Metrics{StatusCode: 200, ResponseTime: 5 * time.Millisecond}
	res := Evaluate(chk, time.Unix(100, 0), m)
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Error)
	}
	if res.Time.Unix() != 100 {
		t.Errorf("expected injected time to be used, got %s", res.Time)
	}
}

func TestEvaluate_NetworkError(t *testing.T) {
	m := Metrics{Err: errors.New("dial tcp: connection refused")}
	res := Evaluate(config.Check{ExpectStatus: 200}, time.Now(), m)
	if res.Success {
		t.Fatal("expected failure on network error")
	}
	if res.Error != "dial tcp: connection refused" {
		t.Errorf("expected error message passed through, got %q", res.Error)
	}
}

func TestEvaluate_MaxResponseMSExceeded(t *testing.T) {
	chk := config.Check{ExpectStatus: 200, MaxResponseMS: 100}
	m := Metrics{StatusCode: 200, ResponseTime: 5 * time.Second}
	res := Evaluate(chk, time.Now(), m)
	if res.Success {
		t.Fatal("expected failure when response time exceeds max_response_ms")
	}
}

func TestEvaluate_MaxResponseMSWithinLimit(t *testing.T) {
	chk := config.Check{ExpectStatus: 200, MaxResponseMS: 5000}
	m := Metrics{StatusCode: 200, ResponseTime: 5 * time.Millisecond}
	res := Evaluate(chk, time.Now(), m)
	if !res.Success {
		t.Fatalf("expected success within max_response_ms, got: %s", res.Error)
	}
}

func TestEvaluate_MaxResponseMSZeroMeansUnchecked(t *testing.T) {
	chk := config.Check{ExpectStatus: 200, MaxResponseMS: 0}
	m := Metrics{StatusCode: 200, ResponseTime: 999 * time.Second}
	res := Evaluate(chk, time.Now(), m)
	if !res.Success {
		t.Fatalf("expected success when max_response_ms is unset, got: %s", res.Error)
	}
}

func TestEvaluate_DefaultExpectStatus(t *testing.T) {
	chk := config.Check{} // ExpectStatus zero-value means "default to 200"
	m := Metrics{StatusCode: 200}
	res := Evaluate(chk, time.Now(), m)
	if !res.Success {
		t.Fatalf("expected default expected_status of 200 to succeed, got: %s", res.Error)
	}

	m2 := Metrics{StatusCode: 404}
	res2 := Evaluate(chk, time.Now(), m2)
	if res2.Success {
		t.Fatal("expected 404 to fail against default expected_status 200")
	}
}

func TestEvaluate_BodyContainsPure(t *testing.T) {
	chk := config.Check{ExpectStatus: 200, BodyContains: "hello"}
	ok := Evaluate(chk, time.Now(), Metrics{StatusCode: 200, Body: "well hello there"})
	if !ok.Success {
		t.Fatalf("expected match, got: %s", ok.Error)
	}
	bad := Evaluate(chk, time.Now(), Metrics{StatusCode: 200, Body: "goodbye"})
	if bad.Success {
		t.Fatal("expected mismatch to fail")
	}
}

func TestEvaluate_BodyMatchesAndBodyNotContainsPure(t *testing.T) {
	cfg, err := config.Parse([]byte(`checks:
  - name: Price
    url: https://example.com
    body_matches: '^Price: \$[0-9]+\.[0-9]{2}$'
    body_not_contains: "Error 500"
`))
	if err != nil {
		t.Fatalf("config.Parse: %v", err)
	}
	chk := cfg.Checks[0]

	ok := Evaluate(chk, time.Now(), Metrics{StatusCode: 200, Body: "Price: $12.95"})
	if !ok.Success {
		t.Fatalf("expected matching body to pass, got: %s", ok.Error)
	}

	regexpMismatch := Evaluate(chk, time.Now(), Metrics{StatusCode: 200, Body: "Price: unknown"})
	if regexpMismatch.Success {
		t.Fatal("expected non-matching body to fail")
	}

	forbiddenText := Evaluate(chk, time.Now(), Metrics{StatusCode: 200, Body: "Price: $12.95 Error 500"})
	if forbiddenText.Success {
		t.Fatal("expected body containing forbidden text to fail")
	}
}
