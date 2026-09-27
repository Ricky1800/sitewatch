// Package history persists check results to an append-only JSONL file and
// compacts it by dropping records older than a configured age.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Record is one line of history: the outcome of a single check at a point
// in time.
type Record struct {
	Time           time.Time `json:"time"`
	Check          string    `json:"check"`
	Success        bool      `json:"success"`
	StatusCode     int       `json:"status_code,omitempty"`
	ResponseTimeMS int64     `json:"response_time_ms"`
	Error          string    `json:"error,omitempty"`
}

// Append writes one record as a JSON line to the file at path, creating it
// (and any parent directory) if needed.
func Append(path string, rec Record) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("history: create dir %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("history: open %s: %w", path, err)
	}
	defer f.Close()

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("history: marshal record: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("history: write %s: %w", path, err)
	}
	return nil
}

// ReadAll reads every well-formed record from the file at path. A missing
// file is treated as an empty history, not an error, since a fresh install
// won't have one yet.
func ReadAll(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("history: open %s: %w", path, err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			// Skip a corrupt line rather than failing the whole read; a
			// partially-written line from a crash shouldn't take down the
			// status page or the daemon.
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("history: scan %s: %w", path, err)
	}
	return records, nil
}

// ForCheck filters records to a single check name.
func ForCheck(records []Record, name string) []Record {
	var out []Record
	for _, r := range records {
		if r.Check == name {
			out = append(out, r)
		}
	}
	return out
}

// Since filters records to those at or after t.
func Since(records []Record, t time.Time) []Record {
	var out []Record
	for _, r := range records {
		if !r.Time.Before(t) {
			out = append(out, r)
		}
	}
	return out
}

// Compact rewrites the history file at path, keeping only records newer
// than now.Add(-maxAge) (maxAge <= 0 disables compaction entirely). It
// writes to a temporary file in the same directory and renames it into
// place, so a crash mid-compaction can't corrupt or truncate the original.
func Compact(path string, maxAge time.Duration, now time.Time) error {
	if maxAge <= 0 {
		return nil
	}
	records, err := ReadAll(path)
	if err != nil {
		return err
	}
	cutoff := now.Add(-maxAge)
	kept := Since(records, cutoff)

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sitewatch-history-*.tmp")
	if err != nil {
		return fmt.Errorf("history: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	w := bufio.NewWriter(tmp)
	for _, rec := range kept {
		line, err := json.Marshal(rec)
		if err != nil {
			tmp.Close()
			return fmt.Errorf("history: marshal record during compaction: %w", err)
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			tmp.Close()
			return fmt.Errorf("history: write temp file: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return fmt.Errorf("history: flush temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("history: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("history: rename temp file into place: %w", err)
	}
	return nil
}
