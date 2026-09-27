package history

import (
	"testing"
	"time"
)

func TestPercentInWindow_AllUp(t *testing.T) {
	now := time.Unix(1000, 0)
	records := []Record{
		{Check: "a", Time: now.Add(-1 * time.Hour), Success: true},
		{Check: "a", Time: now.Add(-2 * time.Hour), Success: true},
	}
	u := PercentInWindow(records, "a", now.Add(-24*time.Hour), now)
	if !u.OK {
		t.Fatal("expected OK=true")
	}
	if u.Percent != 100 {
		t.Errorf("expected 100%%, got %v", u.Percent)
	}
}

func TestPercentInWindow_Mixed(t *testing.T) {
	now := time.Unix(1000, 0)
	records := []Record{
		{Check: "a", Time: now.Add(-1 * time.Hour), Success: true},
		{Check: "a", Time: now.Add(-2 * time.Hour), Success: false},
		{Check: "a", Time: now.Add(-3 * time.Hour), Success: true},
		{Check: "a", Time: now.Add(-4 * time.Hour), Success: true},
	}
	u := PercentInWindow(records, "a", now.Add(-24*time.Hour), now)
	if !u.OK {
		t.Fatal("expected OK=true")
	}
	if u.Total != 4 || u.Up != 3 {
		t.Fatalf("expected 3/4, got %d/%d", u.Up, u.Total)
	}
	if u.Percent != 75 {
		t.Errorf("expected 75%%, got %v", u.Percent)
	}
}

func TestPercentInWindow_NoData(t *testing.T) {
	now := time.Unix(1000, 0)
	u := PercentInWindow(nil, "a", now.Add(-24*time.Hour), now)
	if u.OK {
		t.Fatal("expected OK=false when there is no data")
	}
}

func TestPercentInWindow_IgnoresOtherChecksAndOutOfWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	records := []Record{
		{Check: "other", Time: now.Add(-1 * time.Hour), Success: false},
		{Check: "a", Time: now.Add(-100 * time.Hour), Success: false}, // out of window
		{Check: "a", Time: now.Add(-1 * time.Hour), Success: true},
	}
	u := PercentInWindow(records, "a", now.Add(-24*time.Hour), now)
	if u.Total != 1 {
		t.Fatalf("expected exactly 1 record in window for check a, got %d", u.Total)
	}
	if u.Percent != 100 {
		t.Errorf("expected 100%%, got %v", u.Percent)
	}
}

func TestPercentInWindow_BoundaryExclusiveAtNow(t *testing.T) {
	now := time.Unix(1000, 0)
	records := []Record{
		{Check: "a", Time: now, Success: false}, // exactly at "now": excluded
	}
	u := PercentInWindow(records, "a", now.Add(-24*time.Hour), now)
	if u.OK {
		t.Fatal("expected no data: the only record is exactly at the exclusive upper bound")
	}
}

func TestPercentInWindow_BoundaryInclusiveAtSince(t *testing.T) {
	now := time.Unix(1000, 0)
	since := now.Add(-24 * time.Hour)
	records := []Record{
		{Check: "a", Time: since, Success: true}, // exactly at "since": included
	}
	u := PercentInWindow(records, "a", since, now)
	if !u.OK || u.Total != 1 {
		t.Fatalf("expected the record exactly at since to be included, got %+v", u)
	}
}
