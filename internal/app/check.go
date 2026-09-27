package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/Ricky1800/sitewatch/internal/checker"
	"github.com/Ricky1800/sitewatch/internal/config"
)

// RunCheck implements `sitewatch check`: a one-shot run over every
// configured check, suitable for CI or cron. It exits 1 if any check
// failed, so a cron job or CI step can alert on a non-zero exit code.
func RunCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "sitewatch.yaml", "path to sitewatch.yaml")
	jsonOut := fs.Bool("json", false, "output results as JSON instead of a table")
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

	results := runAllChecks(cfg)

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintln(stderr, "encoding results:", err)
			return 2
		}
	} else {
		printCheckTable(stdout, results)
	}

	for _, r := range results {
		if !r.Success {
			return 1
		}
	}
	return 0
}

// runAllChecks runs every configured check concurrently and returns results
// in the same order as cfg.Checks.
func runAllChecks(cfg *config.Config) []checker.Result {
	c := &checker.Checker{Client: &http.Client{}}
	results := make([]checker.Result, len(cfg.Checks))

	var wg sync.WaitGroup
	for i, chk := range cfg.Checks {
		wg.Add(1)
		go func(i int, chk config.Check) {
			defer wg.Done()
			results[i] = c.Check(context.Background(), chk)
		}(i, chk)
	}
	wg.Wait()
	return results
}

func printCheckTable(w io.Writer, results []checker.Result) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS\tCODE\tRESPONSE\tERROR")
	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAIL"
		}
		errMsg := r.Error
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
			r.Name, status, r.StatusCode, r.ResponseTime.Round(time.Millisecond), errMsg)
	}
	tw.Flush()
}
