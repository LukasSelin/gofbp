// Package bearing holds the one piece of angle arithmetic both package spread
// and package fbp need: folding a bearing into [0, 360).
package bearing

import (
	"math"
)

// Normalize folds a bearing into [0, 360).
//
// The second guard is not redundant with the first. A tiny negative input — which
// atan2 produces routinely, since sin(2π) evaluates to -2.4e-16 rather than 0 —
// plus 360 rounds to exactly 360.0 in float64, and 360 is not a bearing in
// [0, 360). Callers that compare against 0 would see a 360° error from a 1e-14
// input.
func Normalize(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	if d >= 360 {
		return 0
	}
	return d
}
