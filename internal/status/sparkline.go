package status

import (
	"fmt"
	"strings"
	"time"
)

// Sparkline renders points (response times, oldest first) as a minimal
// inline SVG polyline sized width x height. It never touches the network or
// the filesystem and is a pure function of its inputs, so it's fully
// deterministic and unit-testable.
func Sparkline(points []time.Duration, width, height int) string {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 24
	}
	if len(points) == 0 {
		return fmt.Sprintf(
			`<svg viewBox="0 0 %d %d" width="%d" height="%d" xmlns="http://www.w3.org/2000/svg" class="sparkline sparkline-empty" role="img" aria-label="no response time data"></svg>`,
			width, height, width, height,
		)
	}

	ms := make([]float64, len(points))
	min, max := float64(points[0].Milliseconds()), float64(points[0].Milliseconds())
	for i, p := range points {
		v := float64(p.Milliseconds())
		ms[i] = v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	spread := max - min
	if spread == 0 {
		spread = 1 // flat line: draw it centered instead of dividing by zero
	}

	n := len(ms)
	denom := n - 1
	if denom < 1 {
		denom = 1
	}

	var pts strings.Builder
	for i, v := range ms {
		x := float64(width) * float64(i) / float64(denom)
		// Higher response time draws lower on the chart (y grows downward
		// in SVG, and "down" reads naturally as "worse/slower" here).
		y := (v - min) / spread * float64(height)
		fmt.Fprintf(&pts, "%.1f,%.1f ", x, y)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" width="%d" height="%d" xmlns="http://www.w3.org/2000/svg" class="sparkline" role="img" aria-label="response time trend, %d samples">`,
		width, height, width, height, n)
	fmt.Fprintf(&b, `<polyline fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" points="%s" />`, strings.TrimSpace(pts.String()))
	b.WriteString(`</svg>`)
	return b.String()
}
