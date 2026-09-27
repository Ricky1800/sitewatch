package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	r1 := Record{Time: time.Unix(100, 0).UTC(), Check: "a", Success: true, StatusCode: 200, ResponseTimeMS: 12}
	r2 := Record{Time: time.Unix(200, 0).UTC(), Check: "a", Success: false, StatusCode: 500, Error: "boom"}

	if err := Append(path, r1); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := Append(path, r2); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if !got[0].Time.Equal(r1.Time) || got[0].Check != "a" || !got[0].Success {
		t.Errorf("unexpected first record: %+v", got[0])
	}
	if got[1].Error != "boom" {
		t.Errorf("unexpected second record error: %q", got[1].Error)
	}
}

func TestReadAll_MissingFileIsEmptyNotError(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadAll(filepath.Join(dir, "does-not-exist.jsonl"))
	if err != nil {
		t.Fatalf("expected no error for a missing file, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d records", len(got))
	}
}

func TestReadAll_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	if err := Append(path, Record{Time: time.Unix(1, 0), Check: "a", Success: true}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	appendRaw(t, path, "{not valid json\n")
	if err := Append(path, Record{Time: time.Unix(2, 0), Check: "a", Success: true}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected corrupt line to be skipped, leaving 2 valid records, got %d", len(got))
	}
}

func appendRaw(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestForCheck(t *testing.T) {
	records := []Record{
		{Check: "a", Success: true},
		{Check: "b", Success: false},
		{Check: "a", Success: false},
	}
	got := ForCheck(records, "a")
	if len(got) != 2 {
		t.Fatalf("expected 2 records for check a, got %d", len(got))
	}
}

func TestSince(t *testing.T) {
	records := []Record{
		{Time: time.Unix(10, 0)},
		{Time: time.Unix(20, 0)},
		{Time: time.Unix(30, 0)},
	}
	got := Since(records, time.Unix(20, 0))
	if len(got) != 2 {
		t.Fatalf("expected 2 records at/after t=20, got %d", len(got))
	}
}

func TestCompact_DropsOldRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	now := time.Unix(1_000_000, 0).UTC()
	old := Record{Time: now.Add(-100 * 24 * time.Hour), Check: "a", Success: true}
	recent := Record{Time: now.Add(-1 * time.Hour), Check: "a", Success: true}

	if err := Append(path, old); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := Append(path, recent); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := Compact(path, 90*24*time.Hour, now); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record to survive compaction, got %d", len(got))
	}
	if !got[0].Time.Equal(recent.Time) {
		t.Errorf("expected the recent record to survive, got %+v", got[0])
	}
}

func TestCompact_DisabledWhenMaxAgeZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	old := Record{Time: time.Unix(0, 0), Check: "a", Success: true}
	if err := Append(path, old); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := Compact(path, 0, time.Now()); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected compaction to be a no-op when maxAge<=0, got %d records", len(got))
	}
}

func TestCompact_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.jsonl")
	if err := Compact(path, time.Hour, time.Now()); err != nil {
		t.Fatalf("expected compacting a missing file to be a no-op, got %v", err)
	}
}
