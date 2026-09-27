// Package app implements sitewatch's subcommands (run, check, status,
// version) as testable functions over explicit stdout/stderr writers,
// separate from main.go's argv parsing.
package app

import (
	"fmt"
	"io"
)

// Version is sitewatch's release version. Bump alongside CHANGELOG.md.
const Version = "0.2.0"

// RunVersion prints the version string.
func RunVersion(stdout io.Writer) int {
	fmt.Fprintf(stdout, "sitewatch %s\n", Version)
	return 0
}
