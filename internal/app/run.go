package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Ricky1800/sitewatch/internal/alert"
	"github.com/Ricky1800/sitewatch/internal/checker"
	"github.com/Ricky1800/sitewatch/internal/clock"
	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/history"
	"github.com/Ricky1800/sitewatch/internal/state"
)

// compactInterval controls how often the daemon compacts the history file.
const compactInterval = 24 * time.Hour

// RunDaemon implements `sitewatch run`: it starts one scheduler goroutine
// per configured check (each with its own jittered ticker so checks don't
// all fire in lockstep), evaluates results through the state machine,
// dispatches alerts only on state changes, appends every result to history,
// and shuts down cleanly on SIGINT/SIGTERM.
func RunDaemon(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "sitewatch.yaml", "path to sitewatch.yaml")
	metricsAddr := fs.String("metrics-addr", "", "address for the Prometheus /metrics endpoint (disabled by default)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(cfg.Checks) == 0 {
		fmt.Fprintln(stderr, "no checks configured")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d := &daemon{
		cfg:     cfg,
		checker: &checker.Checker{Client: &http.Client{}, Clock: clock.Real{}},
		clock:   clock.Real{},
		alerts:  alert.NewManager(cfg.Alerts),
		states:  make(map[string]*state.CheckState),
		metrics: make(map[string]metricSample),
		log:     stdout,
	}
	// Alert delivery errors are reported from many check goroutines at once.
	var stderrMu sync.Mutex
	d.alerts.Logf = func(format string, args ...any) {
		stderrMu.Lock()
		defer stderrMu.Unlock()
		fmt.Fprintf(stderr, format+"\n", args...)
	}

	var metricsServer *http.Server
	var metricsDone chan error
	if *metricsAddr != "" {
		server, listener, done, startErr := startMetricsServer(d, *metricsAddr)
		if startErr != nil {
			fmt.Fprintf(stderr, "starting metrics endpoint: %v\n", startErr)
			return 2
		}
		metricsServer, metricsDone = server, done
		d.logf("metrics endpoint listening on %s\n", listener.Addr())
	}

	fmt.Fprintf(stdout, "sitewatch %s starting: %d check(s), history=%s\n", Version, len(cfg.Checks), cfg.History.Path)
	d.run(ctx)
	if metricsServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			d.logf("metrics endpoint shutdown: %v\n", err)
			_ = metricsServer.Close()
		}
		cancel()
		if err := <-metricsDone; err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.logf("metrics endpoint: %v\n", err)
		}
	}
	fmt.Fprintln(stdout, "sitewatch: shutdown complete")
	return 0
}

// daemon holds everything the running scheduler needs, so run() and
// runCheckLoop() are simple methods rather than a pile of parameters.
type daemon struct {
	cfg     *config.Config
	checker *checker.Checker
	clock   clock.Clock
	alerts  *alert.Manager
	log     io.Writer

	// logMu serializes writes to log: every check goroutine logs through
	// logf, and most io.Writers (bytes.Buffer, bufio.Writer) are not safe
	// for concurrent use.
	logMu sync.Mutex

	mu      sync.Mutex
	states  map[string]*state.CheckState
	metrics map[string]metricSample // protected by mu; latest completed result per check
}

// logf writes one log line to d.log, safe for concurrent use.
func (d *daemon) logf(format string, args ...any) {
	d.logMu.Lock()
	defer d.logMu.Unlock()
	fmt.Fprintf(d.log, format, args...)
}

func (d *daemon) stateFor(name string) *state.CheckState {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.states[name]
	if !ok {
		s = &state.CheckState{}
		d.states[name] = s
	}
	return s
}

// run starts one goroutine per check plus a periodic history-compaction
// goroutine, and blocks until ctx is canceled (SIGINT/SIGTERM), then waits
// for every in-flight check to finish before returning.
func (d *daemon) run(ctx context.Context) {
	var wg sync.WaitGroup

	for _, chk := range d.cfg.Checks {
		wg.Add(1)
		go func(chk config.Check) {
			defer wg.Done()
			d.scheduleCheck(ctx, chk)
		}(chk)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		d.scheduleCompaction(ctx)
	}()

	<-ctx.Done()
	d.logf("sitewatch: shutdown signal received, waiting for in-flight checks...\n")
	wg.Wait()
}

// scheduleCheck runs chk on its configured interval, starting after a
// random jitter (0..interval) so many checks configured with the same
// interval don't all fire in the same instant.
func (d *daemon) scheduleCheck(ctx context.Context, chk config.Check) {
	jitter := time.Duration(rand.Int63n(int64(chk.Interval.Std())))
	timer := time.NewTimer(jitter)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			d.runOnce(ctx, chk)
			timer.Reset(chk.Interval.Std())
		}
	}
}

// runOnce performs a single probe, feeds it through the state machine, and
// dispatches any resulting alert, then appends the result to history.
func (d *daemon) runOnce(ctx context.Context, chk config.Check) {
	res := d.checker.Check(ctx, chk)
	now := d.clock.Now()
	s := d.stateFor(chk.Name)

	d.mu.Lock()
	tr := s.Record(res.Success, chk.FailThreshold, now)
	shouldWarnSSL := false
	sslDaysLeft := 0
	if res.TLSExpiry != nil {
		sslDaysLeft = int(res.TLSExpiry.Sub(now).Hours() / 24)
		shouldWarnSSL = s.ShouldWarnSSL(*res.TLSExpiry, chk.SSLWarnDays, now)
	}
	if d.metrics == nil {
		d.metrics = make(map[string]metricSample)
	}
	d.metrics[chk.Name] = metricSample{status: s.Current, responseTime: res.ResponseTime}
	d.mu.Unlock()

	switch tr.Kind {
	case state.TransitionToDown:
		d.logf("%s DOWN: %s\n", chk.Name, res.Error)
		d.alerts.Send(ctx, alert.Event{
			Kind:             alert.Down,
			CheckName:        chk.Name,
			URL:              chk.URL,
			Time:             now,
			ConsecutiveFails: tr.ConsecutiveFails,
			Error:            res.Error,
		})
	case state.TransitionToUp:
		d.logf("%s UP (was down %s)\n", chk.Name, tr.Downtime.Round(time.Second))
		d.alerts.Send(ctx, alert.Event{
			Kind:      alert.Up,
			CheckName: chk.Name,
			URL:       chk.URL,
			Time:      now,
			Downtime:  tr.Downtime,
		})
	}

	if res.TLSExpiry != nil && shouldWarnSSL {
		d.logf("%s SSL certificate expires in %d day(s)\n", chk.Name, sslDaysLeft)
		d.alerts.Send(ctx, alert.Event{
			Kind:        alert.SSLExpiry,
			CheckName:   chk.Name,
			URL:         chk.URL,
			Time:        now,
			SSLExpiry:   *res.TLSExpiry,
			SSLDaysLeft: sslDaysLeft,
		})
	}

	rec := history.Record{
		Time:           now,
		Check:          chk.Name,
		Success:        res.Success,
		StatusCode:     res.StatusCode,
		ResponseTimeMS: res.ResponseTime.Milliseconds(),
		Error:          res.Error,
	}
	if err := history.Append(d.cfg.History.Path, rec); err != nil {
		d.logf("history: %v\n", err)
	}
}

// scheduleCompaction periodically compacts the history file so it doesn't
// grow forever. It runs once immediately (cheap no-op if the file is
// small) and then every compactInterval.
func (d *daemon) scheduleCompaction(ctx context.Context) {
	maxAge := time.Duration(d.cfg.History.MaxAgeDays) * 24 * time.Hour
	d.compactOnce(maxAge)

	ticker := time.NewTicker(compactInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.compactOnce(maxAge)
		}
	}
}

func (d *daemon) compactOnce(maxAge time.Duration) {
	if err := history.Compact(d.cfg.History.Path, maxAge, d.clock.Now()); err != nil {
		d.logf("history compaction: %v\n", err)
	}
}
