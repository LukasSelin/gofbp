package fbp

import (
	"math"
	"sort"
	"testing"
)

// LB is two coefficients per fuel FAMILY (one wooded form, one grass form), so
// this checks the eq. 79/80/81 branch as well as the numbers.
func TestCFFDRSLengthToBreadth(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	perFuel, bad := map[string]int{}, map[string]int{}
	shown := 0
	for i, c := range f.Cases {
		fuel := ourFuel(c.Fuel)
		perFuel[fuel]++
		got := LengthToBreadth(fuel, c.WSV)
		if !closeEnough(got, c.LB, tol) {
			bad[fuel]++
			if shown++; shown <= 15 {
				t.Errorf("case %d %s WSV=%.4f: LB = %v, cffdrs = %v", i, c.Fuel, c.WSV, got, c.LB)
			}
		}
	}
	logPerFuel(t, perFuel, bad)
	if shown > 15 {
		t.Errorf("... and %d more LB mismatches", shown-15)
	}
}

// The backing rate, via the ratio form the serving path actually uses.
//
// BROS is not a new spread equation — it is ROS at the BACKING ISI. Asserting it
// through BackISIRatio rather than BackISI is deliberate: the ratio is what
// internal/surface calls, because it is anchored on SMHI's published ISI and
// never forms an ISI from FFMC. Testing the form nothing calls would leave the
// served number unoracled.
//
// Restricted to flat rows for the same reason TestCFFDRSSurfaceROS is: on sloped
// ground cffdrs' reported ISI already carries the slope-equivalent wind, and the
// back-solve is asserted separately.
func TestCFFDRSBackROS(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-8
	perFuel, bad := map[string]int{}, map[string]int{}
	shown := 0
	for i, c := range f.Cases {
		if c.GS != 0 || c.CFB != 0 {
			continue
		}
		fuel := ourFuel(c.Fuel)
		perFuel[fuel]++
		bisi := c.ISI * BackISIRatio(c.WSV)
		got := ROS(fuel, bisi, c.BUI, c.PC, c.CC, 0)
		if !closeEnough(got, c.BROS, tol) {
			bad[fuel]++
			if shown++; shown <= 15 {
				t.Errorf("case %d %s ISI=%.4f WSV=%.4f BUI=%v: BROS = %v, cffdrs = %v (ratio %.4f)",
					i, c.Fuel, c.ISI, c.WSV, c.BUI, got, c.BROS, got/c.BROS)
			}
		}
	}
	logPerFuel(t, perFuel, bad)
	if shown > 15 {
		t.Errorf("... and %d more BROS mismatches", shown-15)
	}
}

func TestCFFDRSFlankROS(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	bad, shown := 0, 0
	for i, c := range f.Cases {
		got := FlankROS(c.ROS, c.BROS, c.LB)
		if !closeEnough(got, c.FROS, tol) {
			bad++
			if shown++; shown <= 15 {
				t.Errorf("case %d %s: FROS = %v, cffdrs = %v", i, c.Fuel, got, c.FROS)
			}
		}
	}
	t.Logf("%d cases, %d mismatched", len(f.Cases), bad)
}

// The two exact identities that pin the ellipse geometry. cffdrs' fbp() returns
// the ellipse's parameters but no rate at an arbitrary bearing, so these — not
// the oracle — are what says ROSAtAngle is the right curve through them.
func TestROSAtAngleReproducesHeadAndBackExactly(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	badHead, badBack, shown := 0, 0, 0
	for i, c := range f.Cases {
		if c.ROS <= 0 || c.FROS <= 0 {
			continue
		}
		if head := ROSAtAngle(c.ROS, c.FROS, c.BROS, 0); !closeEnough(head, c.ROS, tol) {
			badHead++
			if shown++; shown <= 10 {
				t.Errorf("case %d %s: ROSAtAngle(0) = %v, want the head rate %v", i, c.Fuel, head, c.ROS)
			}
		}
		if back := ROSAtAngle(c.ROS, c.FROS, c.BROS, 180); !closeEnough(back, c.BROS, tol) {
			badBack++
			if shown++; shown <= 10 {
				t.Errorf("case %d %s: ROSAtAngle(180) = %v, want the back rate %v", i, c.Fuel, back, c.BROS)
			}
		}
	}
	t.Logf("%d cases: %d head mismatches, %d back mismatches", len(f.Cases), badHead, badBack)
}

// The ellipse's SHAPE, asserted rather than its endpoints alone: the head is the
// global maximum and the curve falls to a single minimum on the flank and rises
// again to the back. Together with the two endpoint identities this forbids the
// plausible transcription errors — a sign flip, swapped semi-axes, a centre
// offset in the wrong direction — each of which shows up here as a second
// interior maximum or a rate above the head even when the endpoints land.
//
// Note what is deliberately NOT asserted: that the rate decreases all the way
// from head to back. It does not, and that is real FBP geometry rather than a
// defect. On an elongated fire the minimum is the FLANK: a long thin ellipse
// spreads faster backwards along its own axis than sideways across it, so the
// curve turns back up somewhere before 180 degrees. About 9 % of the fixture's
// cases do this. A test asserting monotone decrease fails on exactly those, and
// "fixing" ROSAtAngle to satisfy it would break the back-rate identity.
func TestROSAtAngleIsUnimodalWithTheHeadAsMaximum(t *testing.T) {
	f := loadCFFDRS(t)
	bad, shown, turned := 0, 0, 0
	for i, c := range f.Cases {
		if c.ROS <= 0 || c.FROS <= 0 || c.BROS >= c.ROS {
			continue
		}
		head := ROSAtAngle(c.ROS, c.FROS, c.BROS, 0)
		prev := head
		rising := false
		fell := false
		for theta := 5.0; theta <= 180; theta += 5 {
			got := ROSAtAngle(c.ROS, c.FROS, c.BROS, theta)
			fail := ""
			switch {
			case math.IsNaN(got) || got <= 0:
				fail = "not a positive rate"
			case got > head*(1+1e-9):
				fail = "exceeds the head rate"
			case got > prev*(1+1e-9)+1e-12:
				// Rising. Legal once — the flank minimum — and never again
				// after a subsequent fall.
				if fell {
					fail = "fell again after rising: two interior maxima"
				}
				if !rising {
					rising = true
					turned++
				}
			case got < prev*(1-1e-9):
				if rising {
					fell = true
				}
			}
			if fail != "" {
				bad++
				if shown++; shown <= 10 {
					t.Errorf("case %d %s theta=%v: ROSAtAngle = %v (%s; head %v, prev %v)",
						i, c.Fuel, theta, got, fail, head, prev)
				}
				break
			}
			prev = got
		}
	}
	if turned == 0 {
		t.Error("no case turned back up before 180 degrees — the fixture no longer covers elongated ellipses, so the unimodality branch checked nothing")
	}
	t.Logf("%d cases, %d malformed, %d with the minimum on the flank rather than the back", len(f.Cases), bad, turned)
}

// The ellipse is symmetric about its head axis, so a bearing left of the head
// spreads at the same rate as the same bearing right of it. AngleBetweenDeg
// relies on this to hand ROSAtAngle an unsigned separation.
func TestROSAtAngleIsSymmetricAboutTheHeadAxis(t *testing.T) {
	const rosHead, rosFlank, rosBack = 12.0, 3.0, 1.5
	for theta := 0.0; theta <= 180; theta += 15 {
		want := ROSAtAngle(rosHead, rosFlank, rosBack, theta)
		for _, mirrored := range []float64{-theta, 360 - theta, theta + 360} {
			if got := ROSAtAngle(rosHead, rosFlank, rosBack, mirrored); !closeEnough(got, want, 1e-12) {
				t.Errorf("theta=%v vs %v: %v != %v", theta, mirrored, got, want)
			}
		}
	}
}

// The 90-degree gap, asserted rather than left as a comment. The flank rate is
// the ellipse's half-width at its OWN centre; ROSAtAngle measures from the
// ignition point, which sits behind that centre, so it must come out smaller
// wherever the fire is not a circle. Someone who "fixes" ROSAtAngle to return
// FROS at 90 degrees breaks this.
func TestROSAtAngle90IsBelowTheFlankRate(t *testing.T) {
	f := loadCFFDRS(t)
	checked := 0
	for i, c := range f.Cases {
		if c.LB <= 1.05 || c.ROS <= 0 || c.FROS <= 0 {
			continue
		}
		checked++
		if got := ROSAtAngle(c.ROS, c.FROS, c.BROS, 90); got >= c.FROS {
			t.Fatalf("case %d %s LB=%.3f: ROSAtAngle(90) = %v, want < FROS = %v", i, c.Fuel, c.LB, got, c.FROS)
		}
	}
	if checked == 0 {
		t.Fatal("no elongated cases in the fixture — the assertion checked nothing")
	}
	t.Logf("%d elongated cases checked", checked)
}

// A circle is the degenerate ellipse: with no wind the fire spreads at the same
// rate in every direction, and the geometry must not invent a bearing dependence
// out of float noise.
func TestROSAtAngleIsIsotropicWithoutWind(t *testing.T) {
	const r = 4.0
	for theta := 0.0; theta <= 360; theta += 15 {
		if got := ROSAtAngle(r, r, r, theta); !closeEnough(got, r, 1e-12) {
			t.Errorf("theta=%v: ROSAtAngle = %v, want %v everywhere on a circle", theta, got, r)
		}
	}
}

func TestAngleBetweenDeg(t *testing.T) {
	cases := []struct{ a, b, want float64 }{
		{0, 0, 0},
		{10, 350, 20},  // straddling north — the case a naive |a-b| gets wrong
		{350, 10, 20},  // and symmetric in its arguments
		{0, 180, 180},  // opposite
		{90, 271, 179}, // just under opposite, from the other side
		{45, 225, 180},
		{370, 10, 0},  // out-of-range input folds
		{-10, 350, 0}, // negative input folds
	}
	for _, c := range cases {
		if got := AngleBetweenDeg(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("AngleBetweenDeg(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func logPerFuel(t *testing.T, perFuel, bad map[string]int) {
	t.Helper()
	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, fuel := range fuels {
		t.Logf("%-4s %5d cases, %d mismatched", fuel, perFuel[fuel], bad[fuel])
	}
}
