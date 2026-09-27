package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Ricky1800/sitewatch/internal/config"
	"github.com/Ricky1800/sitewatch/internal/history"
	"github.com/Ricky1800/sitewatch/internal/status"
)

// RunStatus implements `sitewatch status --out status.html`: it reads the
// history file and renders a static, self-contained HTML status page.
func RunStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "sitewatch.yaml", "path to sitewatch.yaml")
	out := fs.String("out", "status.html", "path to write the generated status page")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	records, err := history.ReadAll(cfg.History.Path)
	if err != nil {
		fmt.Fprintln(stderr, "reading history:", err)
		return 2
	}

	data := status.BuildPageData(cfg, records, time.Now())

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(stderr, "creating output file:", err)
		return 2
	}
	defer f.Close()

	if err := status.Generate(f, data); err != nil {
		fmt.Fprintln(stderr, "generating status page:", err)
		return 2
	}

	fmt.Fprintf(stdout, "wrote %s (%d checks)\n", *out, len(data.Checks))
	return 0
}
