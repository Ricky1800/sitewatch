// Command sitewatch is a single-binary uptime, SSL-expiry, and content
// monitor for small-business websites.
package main

import (
	"fmt"
	"os"

	"github.com/Ricky1800/sitewatch/internal/app"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run":
		return app.RunDaemon(rest, os.Stdout, os.Stderr)
	case "check":
		return app.RunCheck(rest, os.Stdout, os.Stderr)
	case "status":
		return app.RunStatus(rest, os.Stdout, os.Stderr)
	case "version", "-v", "--version":
		return app.RunVersion(os.Stdout)
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "sitewatch: unknown command %q\n\n", cmd)
		printUsage(os.Stderr)
		return 2
	}
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `sitewatch — uptime, SSL-expiry, and content monitor for small-business websites

Usage:
  sitewatch run [--config sitewatch.yaml] [--metrics-addr address]
      Start the monitoring daemon. Runs until SIGINT/SIGTERM.
      Expose Prometheus metrics at /metrics on the given address (off by default).

  sitewatch check [--config sitewatch.yaml] [--json]
      Run every configured check once and print the results.
      Exits 1 if any check failed (useful for CI/cron).

  sitewatch status [--config sitewatch.yaml] [--out status.html]
      Generate a static HTML status page from recorded history.

  sitewatch version
      Print the sitewatch version.
`)
}
