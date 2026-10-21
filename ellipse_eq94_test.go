package fbp

import (
	"math"
	"testing"
)

// ROSAtAngle against the PUBLISHED eq. 94 — Wotton, Alexander & Taylor (2009),
// GLC-X-10, p. 20 — rather than against cffdrs, whose transcription of it is
// wrong. Checked against the paper on 2026-10-21.
//
// What the paper prints, with c = cos θ and s = sin θ:
//
//	ROS(θ) = (ROS − BROS)/(2c)
//	       + (ROS + BROS)/(2c) × [ FROS·c·(FROS²c² + ROS·BROS·s²)^½ − ((ROS² − BROS²)/4)·s² ]
//	                             / [ FROS²c² + ((ROS + BROS)/2)²·s² ]
//
// Only the SECOND leading term multiplies the bracket. cffdrs'
// rate_of_spread_at_theta wraps both leading terms in one parenthesis first,
// which collapses them to ROS/c — that one misplaced parenthesis is the whole
// of the defect MIGRATION.md recorded on 2026-09-18 (a pole at 90°, negative
// rates, ROS at 180°). The paper's grouping is not a reading of ambiguous
// typesetting: it is the only one of the two that reproduces the paper's own
// Table 3, and it is the one whose 90° limit is the finite 0/0 the text
// describes ("only undefined at exactly θ = 90°; it displays no asymptotic
// behaviour").
//
// θ is measured from the head direction about the IGNITION POINT (the paper's
// Figure 7: "the line that joins the ignition point or origin of the fire to
// point P"), which is ROSAtAngle's convention.

// eq94Published is the equation as printed, transcribed term for term. It is
// 0/0 at exactly θ = 90° and 270°, as the paper says.
func eq94Published(ros, fros, bros, thetaDeg float64) float64 {
	t := thetaDeg * math.Pi / 180
	c, s := math.Cos(t), math.Sin(t)
	num := fros*c*math.Sqrt(fros*fros*c*c+ros*bros*s*s) - (ros*ros-bros*bros)/4*s*s
	den := fros*fros*c*c + ((ros+bros)/2)*((ros+bros)/2)*s*s
	return (ros-bros)/(2*c) + (ros+bros)/(2*c)*(num/den)
}

// The paper's Table 3: C-2, level ground, FFMC 92, wind 15 km/h, BUI 64, giving
// ROS 17.16, FROS 4.88, BROS 2.16 m/min. The tolerance is the table's own
// precision and nothing more: the three inputs are printed to 0.01 m/min, so
// recomputing from them can move the printed rates by up to about a hundredth.
// The worst row is 75°, 3.7195 against 3.71.
func TestROSAtAngleReproducesGLCX10Table3(t *testing.T) {
	const ros, fros, bros = 17.16, 4.88, 2.16
	table := []struct{ deg, want float64 }{
		{0, 17.16}, {15, 14.29}, {30, 9.72}, {45, 6.6}, {60, 4.78}, {75, 3.71}, {90, 3.07},
		{105, 2.67}, {120, 2.43}, {135, 2.28}, {150, 2.20}, {165, 2.17}, {180, 2.16},
	}
	for _, row := range table {
		got := ROSAtAngle(ros, fros, bros, row.deg)
		if math.Abs(got-row.want) > 0.01 {
			t.Errorf("θ = %v°: ROSAtAngle = %.4f, GLC-X-10 Table 3 prints %.2f", row.deg, got, row.want)
		}
	}
}

// ROSAtAngle solves the ray from the ignition point against the ellipse; eq. 94
// is the closed form of the same intersection. They are the same function
// everywhere eq. 94 is defined, and ROSAtAngle is also defined at 90°, where
// eq. 94's 0/0 is the limit the paper says it is.
func TestROSAtAngleIsPublishedEquation94(t *testing.T) {
	worst, n := 0.0, 0
	for _, ros := range []float64{0.5, 3, 17.16, 60} {
		for _, backRatio := range []float64{0.01, 0.1, 0.3, 0.9} {
			for _, lb := range []float64{1.01, 1.5, 3, 8} {
				bros := ros * backRatio
				fros := (ros + bros) / (2 * lb)
				for deg := 0.0; deg <= 360; deg += 0.5 {
					if math.Mod(deg, 180) == 90 {
						continue
					}
					want := eq94Published(ros, fros, bros, deg)
					got := ROSAtAngle(ros, fros, bros, deg)
					worst = maxF(worst, relErr(got, want))
					if !closeEnough(got, want, 1e-11) {
						if n++; n <= 10 {
							t.Errorf("ros=%v fros=%v bros=%v θ=%v: ROSAtAngle %v, eq. 94 %v", ros, fros, bros, deg, got, want)
						}
					}
				}
			}
		}
	}
	t.Logf("worst relative difference from the published eq. 94: %.3g", worst)

	// At 90° eq. 94 is 0/0; its limit from either side is ROSAtAngle's value.
	const ros, fros, bros = 17.16, 4.88, 2.16
	at90 := ROSAtAngle(ros, fros, bros, 90)
	for _, d := range []float64{89.999, 90.001} {
		if lim := eq94Published(ros, fros, bros, d); math.Abs(lim-at90) > 1e-4 {
			t.Errorf("eq. 94 at %v° = %v, ROSAtAngle at 90° = %v — the 0/0 should have this limit", d, lim, at90)
		}
	}
}
