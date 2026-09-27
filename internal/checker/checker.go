// Package checker performs a single HTTP probe against a configured check
// and evaluates the result against its expectations (status code, body
// contents, response time, TLS certificate expiry).
package checker

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ricky1800/sitewatch/internal/clock"
	"github.com/Ricky1800/sitewatch/internal/config"
)

// maxBodyRead caps how much of a response body is buffered for the
// body-contains check, so a misbehaving server can't exhaust memory.
const maxBodyRead = 1 << 20 // 1 MiB

// Metrics is the raw, unevaluated outcome of performing an HTTP request. It
// is intentionally separate from Result so that the pass/fail evaluation
// logic (Evaluate) can be unit tested with fabricated metrics and no real
// network call or sleep.
type Metrics struct {
	StatusCode   int
	ResponseTime time.Duration
	Body         string
	Err          error
	TLSExpiry    *time.Time
}

// Result is the outcome of checking one endpoint: whether it passed every
// configured expectation, and enough detail to alert or record history.
type Result struct {
	Name         string
	URL          string
	Time         time.Time
	Success      bool
	StatusCode   int
	ResponseTime time.Duration
	Error        string
	TLSExpiry    *time.Time
}

// Checker performs HTTP checks. The zero value is usable; Client and Clock
// default to http.DefaultClient and clock.Real when nil.
type Checker struct {
	Client *http.Client
	Clock  clock.Clock
}

func (c *Checker) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return http.DefaultClient
}

func (c *Checker) clockOrReal() clock.Clock {
	if c.Clock != nil {
		return c.Clock
	}
	return clock.Real{}
}

// Check performs the HTTP request for chk and evaluates it.
func (c *Checker) Check(ctx context.Context, chk config.Check) Result {
	m := c.do(ctx, chk)
	return Evaluate(chk, c.clockOrReal().Now(), m)
}

// do performs the actual network request and collects raw metrics. It does
// not interpret pass/fail; see Evaluate for that.
func (c *Checker) do(ctx context.Context, chk config.Check) Metrics {
	ctx, cancel := context.WithTimeout(ctx, chk.Timeout.Std())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chk.URL, nil)
	if err != nil {
		return Metrics{Err: err}
	}

	start := time.Now()
	resp, err := c.client().Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return Metrics{ResponseTime: elapsed, Err: err}
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
	m := Metrics{
		StatusCode:   resp.StatusCode,
		ResponseTime: elapsed,
		Body:         string(body),
		Err:          readErr,
	}
	if resp.TLS != nil {
		if exp := certExpiry(resp.TLS); exp != nil {
			m.TLSExpiry = exp
		}
	}
	return m
}

// certExpiry returns the earliest NotAfter among the leaf certificates
// presented by the server, which is the date the connection first stops
// being trusted.
func certExpiry(state *tls.ConnectionState) *time.Time {
	if state == nil || len(state.PeerCertificates) == 0 {
		return nil
	}
	earliest := state.PeerCertificates[0].NotAfter
	for _, cert := range state.PeerCertificates[1:] {
		if cert.NotAfter.Before(earliest) {
			earliest = cert.NotAfter
		}
	}
	return &earliest
}

// Evaluate applies chk's expectations to m and produces a Result. It is a
// pure function of its inputs, which keeps timeout/status/body/max-response
// logic fully unit-testable without performing real network I/O.
func Evaluate(chk config.Check, now time.Time, m Metrics) Result {
	res := Result{
		Name:         chk.Name,
		URL:          chk.URL,
		Time:         now,
		StatusCode:   m.StatusCode,
		ResponseTime: m.ResponseTime,
		TLSExpiry:    m.TLSExpiry,
	}

	if m.Err != nil {
		res.Success = false
		res.Error = m.Err.Error()
		return res
	}

	expect := chk.ExpectStatus
	if expect == 0 {
		expect = 200
	}
	if m.StatusCode != expect {
		res.Success = false
		res.Error = statusMismatchMessage(expect, m.StatusCode)
		return res
	}

	if chk.BodyContains != "" && !strings.Contains(m.Body, chk.BodyContains) {
		res.Success = false
		res.Error = "response body did not contain expected text"
		return res
	}

	if chk.MaxResponseMS > 0 && m.ResponseTime > time.Duration(chk.MaxResponseMS)*time.Millisecond {
		res.Success = false
		res.Error = "response time exceeded max_response_ms"
		return res
	}

	res.Success = true
	return res
}

func statusMismatchMessage(expect, got int) string {
	return "unexpected status code: expected " + strconv.Itoa(expect) + ", got " + strconv.Itoa(got)
}
