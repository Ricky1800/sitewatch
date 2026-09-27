package status

import (
	"strings"
	"testing"
	"time"
)

func TestSparkline_Empty(t *testing.T) {
	svg := Sparkline(nil, 100, 20)
	if !strings.Contains(svg, "sparkline-empty") {
		t.Errorf("expected empty-state class, got: %s", svg)
	}
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "</svg>") {
		t.Errorf("expected a valid svg wrapper, got: %s", svg)
	}
}

func TestSparkline_PointCount(t *testing.T) {
	points := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		15 * time.Millisecond,
		5 * time.Millisecond,
	}
	svg := Sparkline(points, 100, 20)
	start := strings.Index(svg, `points="`) + len(`points="`)
	end := strings.Index(svg[start:], `"`) + start
	coords := strings.Fields(svg[start:end])
	if len(coords) != len(points) {
		t.Errorf("expected %d coordinate pairs, got %d in points=%q (full svg: %s)", len(points), len(coords), svg[start:end], svg)
	}
}

func TestSparkline_FlatLineDoesNotPanic(t *testing.T) {
	points := []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}
	svg := Sparkline(points, 100, 20)
	if !strings.Contains(svg, "<polyline") {
		t.Errorf("expected a polyline even for a flat line, got: %s", svg)
	}
}

func TestSparkline_SinglePoint(t *testing.T) {
	svg := Sparkline([]time.Duration{5 * time.Millisecond}, 100, 20)
	if !strings.Contains(svg, "<polyline") {
		t.Errorf("expected a polyline for a single point, got: %s", svg)
	}
}

func TestSparkline_DefaultsDimensionsWhenZero(t *testing.T) {
	svg := Sparkline([]time.Duration{time.Millisecond}, 0, 0)
	if !strings.Contains(svg, `viewBox="0 0 120 24"`) {
		t.Errorf("expected default 120x24 viewBox, got: %s", svg)
	}
}

func TestSparkline_Deterministic(t *testing.T) {
	points := []time.Duration{3 * time.Millisecond, 7 * time.Millisecond, 2 * time.Millisecond}
	a := Sparkline(points, 100, 20)
	b := Sparkline(points, 100, 20)
	if a != b {
		t.Errorf("expected deterministic output for identical input:\n%s\nvs\n%s", a, b)
	}
}
