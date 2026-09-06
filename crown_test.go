package fbp

import (
	"math"
	"testing"
)

// A stand that crowns readily: a low crown base over dry-ish foliage with enough
// surface fuel that the threshold is a few m/min rather than a few hundred.
// Every test below perturbs one field of this and looks at which way CFB moves.
func crownableStand() Crown {
	return Crown{FMC: 97, SFC: 2.5, CBH: 3, CFL: 0.8, SurfaceROS: 10}
}

func TestCrownFractionBurnedIsZeroAtAndBelowThreshold(t *testing.T) {
	c := crownableStand()
	rso := CriticalSurfaceROS(CriticalSurfaceIntensity(c.FMC, c.CBH), c.SFC)
	if !(rso > 0) || math.IsInf(rso, 0) {
		t.Fatalf("test stand has a degenerate threshold: RSO = %v", rso)
	}

	for _, ros := range []float64{0, rso / 2, math.Nextafter(rso, 0), rso} {
		c.SurfaceROS = ros
		if got := CrownFractionBurned(c); got != 0 {
			t.Errorf("CFB at SurfaceROS %v (RSO %v) = %v, want 0", ros, rso, got)
		}
	}
	// Strictly above, and it must be strictly positive — a threshold that is
	// never crossed classifies everything as a surface fire. The margin is 1e-6
	// rather than one ULP because eq. 58 is 1 − exp(−x): at x below about 1e-16
	// the exponential rounds to exactly 1 and the difference is 0. That is
	// float64, not the model, and asserting against it would pin an artefact.
	c.SurfaceROS = rso + 1e-6
	if got := CrownFractionBurned(c); !(got > 0) {
		t.Errorf("CFB just above RSO %v = %v, want > 0", rso, got)
	}
}

// Eq. 58 is 1 − exp(−0.23·(ROS − RSO)), which is 0 at the threshold, so the
// piecewise definition joins continuously and approaches it with slope 0.23. A
// clamp or an offset bolted on here would show up as a step.
//
// The tolerance is 1e-3 relative and both sources of slack are real: (rso + eps)
// − rso loses digits when eps is far below rso's magnitude, and the linearisation
// itself drops a (0.23·eps)²/2 term that reaches 1.1e-4 relative at eps = 1e-3.
// Tightening this would be pinning float64, not the equation.
func TestCrownFractionBurnedIsContinuousAtThreshold(t *testing.T) {
	c := crownableStand()
	rso := CriticalSurfaceROS(CriticalSurfaceIntensity(c.FMC, c.CBH), c.SFC)
	for _, eps := range []float64{1e-6, 1e-4, 1e-3} {
		c.SurfaceROS = rso + eps
		got := CrownFractionBurned(c)
		if want := CrownDecayCoefficient * eps; math.Abs(got-want) > 1e-3*want {
			t.Errorf("CFB at RSO+%v = %v, want ~%v (the linearisation of eq. 58)", eps, got, want)
		}
	}
}

func TestCrownFractionBurnedIsBoundedAndFinite(t *testing.T) {
	for _, fmc := range []float64{0, 85, 97, 120} {
		for _, sfc := range []float64{0, 1e-6, 0.35, 2.5, 10} {
			for _, cbh := range []float64{0, MinCrownBaseHeightM, 2, 7, 18, 50} {
				for _, cfl := range []float64{0, 0.5, 1.8} {
					for _, ros := range []float64{0, 0.1, 5, 100, 5000} {
						c := Crown{FMC: fmc, SFC: sfc, CBH: cbh, CFL: cfl, SurfaceROS: ros}
						got := CrownFractionBurned(c)
						if math.IsNaN(got) || got < 0 || got > 1 {
							t.Fatalf("CFB(%+v) = %v, want a finite fraction in [0, 1]", c, got)
						}
					}
				}
			}
		}
	}
}

// Which way each driver runs. Getting one of these backwards is the failure mode
// that survives a code review — the sign is buried in a reciprocal or an
// exponent, and the output stays in range either way.
func TestCrownFractionBurnedMonotonicity(t *testing.T) {
	t.Run("rises with surface spread", func(t *testing.T) {
		c, prev := crownableStand(), -1.0
		for ros := 0.0; ros <= 60; ros += 0.25 {
			c.SurfaceROS = ros
			if cur := CrownFractionBurned(c); cur < prev {
				t.Fatalf("CFB fell at SurfaceROS %v: %v < %v", ros, cur, prev)
			} else {
				prev = cur
			}
		}
	})
	// A higher crown is further from the flames, and wetter foliage is harder to
	// light. Both raise CSI, so both must lower CFB.
	t.Run("falls with crown base height", func(t *testing.T) {
		c, prev := crownableStand(), math.Inf(1)
		for cbh := 0.5; cbh <= 25; cbh += 0.5 {
			c.CBH = cbh
			if cur := CrownFractionBurned(c); cur > prev {
				t.Fatalf("CFB rose at CBH %v: %v > %v", cbh, cur, prev)
			} else {
				prev = cur
			}
		}
	})
	t.Run("falls with foliar moisture", func(t *testing.T) {
		c, prev := crownableStand(), math.Inf(1)
		for fmc := 0.0; fmc <= 120; fmc += 2 {
			c.FMC = fmc
			if cur := CrownFractionBurned(c); cur > prev {
				t.Fatalf("CFB rose at FMC %v: %v > %v", fmc, cur, prev)
			} else {
				prev = cur
			}
		}
	})
	// More surface fuel means more intensity at the same spread rate, so the
	// threshold falls and CFB rises. This is the one that reads backwards.
	t.Run("rises with surface fuel consumption", func(t *testing.T) {
		c, prev := crownableStand(), -1.0
		for sfc := 0.25; sfc <= 12; sfc += 0.25 {
			c.SFC = sfc
			if cur := CrownFractionBurned(c); cur < prev {
				t.Fatalf("CFB fell at SFC %v: %v < %v", sfc, cur, prev)
			} else {
				prev = cur
			}
		}
	})
}

// The gate that keeps grass and slash from reporting a crown fire. Their
// published CBH is 0 and their published CFL is 0; the first alone would give
// them a near-1 CFB, so this is load-bearing rather than defensive.
func TestNoCrownFuelMeansNoCrownFire(t *testing.T) {
	for _, cbh := range []float64{0, 3, 18} {
		for _, ros := range []float64{0.1, 10, 1000} {
			c := Crown{FMC: 97, SFC: 2.5, CBH: cbh, CFL: 0, SurfaceROS: ros}
			if got := CrownFractionBurned(c); got != 0 {
				t.Errorf("CFB with CFL=0 (cbh %v, ros %v) = %v, want 0", cbh, ros, got)
			}
		}
	}
	// And the same inputs WITH crown fuel must not be zero, or the test above
	// passes for the wrong reason.
	c := Crown{FMC: 97, SFC: 2.5, CBH: 0, CFL: 0.8, SurfaceROS: 10}
	if got := CrownFractionBurned(c); !(got > 0) {
		t.Errorf("CFB with CFL=0.8 and CBH=0 = %v, want > 0 — the CFL gate is not what is being tested", got)
	}
}

func TestCriticalSurfaceROSIsInfiniteWithoutSurfaceFuel(t *testing.T) {
	csi := CriticalSurfaceIntensity(97, 3)
	if got := CriticalSurfaceROS(csi, 0); !math.IsInf(got, 1) {
		t.Errorf("RSO with SFC=0 = %v, want +Inf", got)
	}
	c := Crown{FMC: 97, SFC: 0, CBH: 3, CFL: 0.8, SurfaceROS: 1e6}
	if got := CrownFractionBurned(c); got != 0 {
		t.Errorf("CFB with SFC=0 = %v, want 0", got)
	}
}

// No NaN may leave this file. A caller reading NaN as no-data gets a hole in its
// grid; a caller reading it as a number gets an unbounded one.
func TestCrownGuardsAgainstNonFiniteInput(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	if got := CriticalSurfaceIntensity(nan, 3); got != 0 {
		t.Errorf("CSI with FMC=NaN = %v, want 0", got)
	}
	if got := CriticalSurfaceIntensity(-20, 3); got != 0 {
		t.Errorf("CSI with FMC=-20 = %v, want 0 (460+25.9·FMC is negative there)", got)
	}
	for _, c := range []Crown{
		{FMC: nan, SFC: 2.5, CBH: 3, CFL: 0.8, SurfaceROS: 10},
		{FMC: 97, SFC: 2.5, CBH: 3, CFL: 0.8, SurfaceROS: nan},
		{FMC: 97, SFC: 2.5, CBH: 3, CFL: 0.8, SurfaceROS: inf},
		{FMC: 97, SFC: nan, CBH: 3, CFL: 0.8, SurfaceROS: 10},
		{FMC: 97, SFC: 2.5, CBH: nan, CFL: 0.8, SurfaceROS: 10},
		{FMC: 97, SFC: 2.5, CBH: 3, CFL: nan, SurfaceROS: 10},
		// Negative FMC reaches the same place by the other route: CSI returns its
		// zero sentinel, which without this screen becomes a zero threshold and a
		// near-total crown fire.
		{FMC: -20, SFC: 2.5, CBH: 3, CFL: 0.8, SurfaceROS: 10},
	} {
		if got := CrownFractionBurned(c); got != 0 {
			t.Errorf("CFB(%+v) = %v, want 0", c, got)
		}
	}
}

// The boundary convention, pinned. cffdrs initialises FD to "I" and overwrites
// with "S" where CFB < 0.1 and "C" where CFB >= 0.9, so the two ends are NOT
// symmetric: 0.1 is intermittent, 0.9 is continuous. Symmetrising it would
// misclassify every value that lands exactly on a boundary.
func TestDescribeFireBoundaries(t *testing.T) {
	for _, tc := range []struct {
		cfb  float64
		want FireDescription
	}{
		{0, SurfaceFire},
		{0.05, SurfaceFire},
		{math.Nextafter(IntermittentCrownCFB, 0), SurfaceFire},
		{IntermittentCrownCFB, IntermittentCrown},
		{0.5, IntermittentCrown},
		{math.Nextafter(ContinuousCrownCFB, 0), IntermittentCrown},
		{ContinuousCrownCFB, ContinuousCrown},
		{1, ContinuousCrown},
		{math.NaN(), IntermittentCrown},
	} {
		if got := DescribeFire(tc.cfb); got != tc.want {
			t.Errorf("DescribeFire(%v) = %q, want %q", tc.cfb, got, tc.want)
		}
	}
}
