package fbp

import (
	"math"
	"testing"
)

// Surface fuel consumption, without the fixture.
//
// These run on a fresh clone, which is what makes them the tests CI actually
// executes on every push — the TestCFFDRS* ones skip until somebody generates
// the oracle. They cannot tell a mistranscribed coefficient from a correct one;
// what they hold is the shape of the equations, the two branch points, the floor
// the fixture never reaches, and the behaviour on input a sweep of physically
// plausible values never produces.

// nonBUIFuels are the fuels whose SFC does not read BUI at all: C1 reads FFMC
// alone, and the grass fuels read neither driver (eq. 18 returns GFL unchanged).
// Several tests below sweep BUI and have to skip these rather than assert a
// monotonicity they do not have.
var nonBUIFuels = map[string]bool{"C1": true, "O1A": true, "O1B": true}

// Every fuel in Fuels must reach a real branch of the switch. A fuel added to
// the table without a line here would fall to the default and return 0, which
// composes as an infinite crowning threshold and looks exactly like a cell that
// will not burn.
func TestSurfaceFuelConsumptionCoversEveryFuel(t *testing.T) {
	for code := range Fuels {
		got := SurfaceFuelConsumption(code, 90, 60, 50, 0.35)
		if !(got > 0) || math.IsInf(got, 0) {
			t.Errorf("%s: SFC = %v at FFMC 90, BUI 60 — every implemented fuel has a "+
				"positive finite surface fuel consumption there", code, got)
		}
	}
}

// Eq. 18 is an identity, and it is the only one. Whatever grass fuel load goes
// in comes back out, with no FFMC or BUI dependence at all.
func TestSurfaceFuelConsumptionGrassIsTheGrassFuelLoad(t *testing.T) {
	for _, code := range []string{"O1A", "O1B", "O1a", "o1b"} {
		for _, gfl := range []float64{0.1, 0.35, 1.0, 5.0} {
			for _, ffmc := range []float64{60, 85, 95} {
				for _, bui := range []float64{1, 50, 200} {
					got := SurfaceFuelConsumption(code, ffmc, bui, 50, gfl)
					if got != gfl {
						t.Errorf("%s ffmc=%v bui=%v gfl=%v: SFC = %v, want %v (eq. 18 is an identity)",
							code, ffmc, bui, gfl, got, gfl)
					}
				}
			}
		}
	}
	// A non-positive grass fuel load is not an error, it is a cell with no grass
	// in it, and it lands on the floor like every other zero load.
	if got := SurfaceFuelConsumption("O1A", 90, 60, 50, 0); got != MinSurfaceFuelConsumptionKgM2 {
		t.Errorf("O1A with GFL 0: SFC = %v, want the floor %v", got, MinSurfaceFuelConsumptionKgM2)
	}
}

// The floor, which the oracle sweep never reaches — see
// TestCFFDRSSurfaceFuelConsumptionFloorIsUnreached. Every BUI-driven equation
// passes through 0 at BUI 0 and is negative below it.
func TestSurfaceFuelConsumptionFloor(t *testing.T) {
	for code := range Fuels {
		if nonBUIFuels[code] {
			continue
		}
		for _, bui := range []float64{0, -1, -1000} {
			got := SurfaceFuelConsumption(code, 60, bui, 50, 0.35)
			// C7 is the exception at BUI 0: its eq. 13 forest-floor term is
			// driven by FFMC and is still positive above 70, so the sum does not
			// reach the floor. Sweeping FFMC 60 here keeps that term clamped to
			// 0 so the woody term alone decides, which is the case being made.
			if math.IsNaN(got) {
				continue // negative base under eqs. 11/12's power; see the NaN test
			}
			if got != MinSurfaceFuelConsumptionKgM2 {
				t.Errorf("%s at BUI %v: SFC = %v, want the floor %v — a non-positive fuel load "+
					"must not reach eq. 57 as a divisor", code, bui, got, MinSurfaceFuelConsumptionKgM2)
			}
		}
	}
	// And C7 above the FFMC clamp does NOT floor at BUI 0, because its other
	// term is still contributing. Stated positively so the exception above reads
	// as a property rather than as an excuse.
	if got := SurfaceFuelConsumption("C7", 95, 0, 50, 0.35); !(got > MinSurfaceFuelConsumptionKgM2) {
		t.Errorf("C7 at FFMC 95, BUI 0: SFC = %v, want the eq. 13 forest-floor term alone", got)
	}
}

// Both branch points are continuous, and both are places a transcription slip
// would show up as a step.
//
// They are not continuous in the same way, and the difference is a real property
// of the 2009 revision rather than a testing detail. C7's eq. 13 term leaves the
// hinge with a finite slope (0.208 kg/m² per FFMC point), so the function is
// smooth there. C1's eqs. 9a/9b leave theirs under a square root, which gives a
// VERTICAL TANGENT at FFMC 84: the gap across the hinge shrinks like √ε, not
// like ε, so a nanometre in FFMC is still 10⁻⁵ kg/m² in SFC. A caller
// interpolating FFMC near 84 should expect that sensitivity; it is in the
// published revision, not in this transcription.
func TestSurfaceFuelConsumptionBranchesAreContinuous(t *testing.T) {
	// C7: finite slope, so an ordinary tolerance is the right instrument.
	for _, eps := range []float64{1e-6, 1e-9, 1e-12} {
		below := SurfaceFuelConsumption("C7", 70-eps, 60, 50, 0.35)
		at := SurfaceFuelConsumption("C7", 70, 60, 50, 0.35)
		above := SurfaceFuelConsumption("C7", 70+eps, 60, 50, 0.35)
		if math.Abs(below-at) > 1e-5 || math.Abs(above-at) > 1e-5 {
			t.Errorf("C7 across FFMC 70 at eps %v: %v / %v / %v is a step, not a hinge "+
				"(eq. 13's forest-floor term is 0 at FFMC 70 from both sides)", eps, below, at, above)
		}
	}

	// C1's hinge value is a number in its own right: 0.75 is the midpoint both
	// GLC-X-10 branches are built around, and both reach it exactly.
	if got := SurfaceFuelConsumption("C1", 84, 0, 50, 0.35); got != 0.75 {
		t.Errorf("C1 at FFMC 84: SFC = %v, want exactly 0.75", got)
	}

	// C1: assert the √ε convergence rather than a fixed tolerance, which is what
	// says the branches meet without pretending the slope is finite.
	//
	// The tolerances here are loose on purpose and the reason is arithmetic, not
	// slack: 1 - exp(-0.23ε) is catastrophic cancellation for small ε, so the
	// gap's own last digits are noise long before the assertion is. 1e-3
	// relative still separates a mistyped 0.23 or 0.75 from a correct one by
	// orders of magnitude, which is all this is for — the fixture is what checks
	// the coefficients.
	prev := math.Inf(1)
	for _, eps := range []float64{1e-4, 1e-6, 1e-8, 1e-10} {
		below := 0.75 - SurfaceFuelConsumption("C1", 84-eps, 60, 50, 0.35)
		above := SurfaceFuelConsumption("C1", 84+eps, 60, 50, 0.35) - 0.75
		if math.Abs(below-above) > 1e-12 {
			t.Errorf("C1 at eps %v: the branches are asymmetric about the hinge, %v below "+
				"against %v above — eqs. 9a and 9b are mirror images", eps, below, above)
		}
		want := 0.75 * math.Sqrt(1-math.Exp(-0.23*eps))
		if math.Abs(above-want) > 1e-3*want {
			t.Errorf("C1 at eps %v: gap %v, want %v from eq. 9a", eps, above, want)
		}
		// Two decades of ε is one decade of gap — that IS the square root, and
		// it is what distinguishes this hinge from a smooth one.
		if prev != math.Inf(1) {
			if ratio := prev / above; math.Abs(ratio-10) > 0.1 {
				t.Errorf("C1 at eps %v: the gap shrank by %.4f per two decades, want 10 "+
					"(√ε scaling is the signature of eqs. 9a/9b's square root)", eps, ratio)
			}
		}
		prev = above
	}
	if !(prev < 1e-5) {
		t.Errorf("C1's gap across the hinge bottomed out at %v rather than vanishing", prev)
	}
}

// Every equation rises with its driver. This is the cheapest check that a
// coefficient has not had its sign flipped, and it is one the fixture would also
// catch — the point of having it here is that it runs without one.
func TestSurfaceFuelConsumptionIsMonotone(t *testing.T) {
	for code := range Fuels {
		if nonBUIFuels[code] {
			continue
		}
		prev := math.Inf(-1)
		for bui := 0.0; bui <= 300; bui += 5 {
			got := SurfaceFuelConsumption(code, 90, bui, 50, 0.35)
			if got < prev {
				t.Errorf("%s: SFC fell from %v to %v between BUI %v and %v — more buildup "+
					"cannot mean less fuel consumed", code, prev, got, bui-5, bui)
				break
			}
			prev = got
		}
	}
	for _, code := range []string{"C1", "C7"} {
		prev := math.Inf(-1)
		for ffmc := 0.0; ffmc <= 101; ffmc++ {
			got := SurfaceFuelConsumption(code, ffmc, 60, 50, 0.35)
			if got < prev {
				t.Errorf("%s: SFC fell from %v to %v approaching FFMC %v — drier fine fuel "+
					"cannot mean less consumption", code, prev, got, ffmc)
				break
			}
			prev = got
		}
	}
}

// Eq. 17's endpoints. The blend is the whole content of M1/M2's SFC, and at
// either end it has to collapse exactly onto the curve it is blending.
func TestSurfaceFuelConsumptionMixedwoodBlendCollapses(t *testing.T) {
	for _, bui := range []float64{1, 20, 64, 150} {
		c2 := SurfaceFuelConsumption("C2", 90, bui, 50, 0.35)
		d1 := SurfaceFuelConsumption("D1", 90, bui, 50, 0.35)
		for _, code := range []string{"M1", "M2"} {
			if got := SurfaceFuelConsumption(code, 90, bui, 100, 0.35); math.Abs(got-c2) > 1e-12 {
				t.Errorf("%s at PC 100, BUI %v: SFC = %v, want C2's %v", code, bui, got, c2)
			}
			if got := SurfaceFuelConsumption(code, 90, bui, 0, 0.35); math.Abs(got-d1) > 1e-12 {
				t.Errorf("%s at PC 0, BUI %v: SFC = %v, want D1's %v", code, bui, got, d1)
			}
			// And the middle is genuinely between them, which a blend written
			// with its weights transposed would not be.
			mid := SurfaceFuelConsumption(code, 90, bui, 50, 0.35)
			if !(mid > d1 && mid < c2) {
				t.Errorf("%s at PC 50, BUI %v: SFC = %v is not between D1's %v and C2's %v",
					code, bui, mid, d1, c2)
			}
		}
	}
}

// M3 and M4 take eq. 10 unchanged — the same spruce-moss floor as C2 — and PDF
// does not enter SFC at all even though it weights their RSI. A reader coming
// from RSI would reasonably expect otherwise, so it is asserted rather than left
// to the switch's grouping.
func TestSurfaceFuelConsumptionDeadFirFuelsTakeTheC2Equation(t *testing.T) {
	for _, bui := range []float64{1, 20, 64, 150} {
		c2 := SurfaceFuelConsumption("C2", 90, bui, 50, 0.35)
		for _, code := range []string{"M3", "M4"} {
			if got := SurfaceFuelConsumption(code, 90, bui, 50, 0.35); got != c2 {
				t.Errorf("%s at BUI %v: SFC = %v, want C2's %v (eq. 10 covers all three)",
					code, bui, got, c2)
			}
		}
	}
}

// An unimplemented fuel returns 0, not the near-zero load cffdrs' -999 sentinel
// decays into. See the doc comment: 0 composes into an infinite crowning
// threshold, where 1e-6 composes into a fabricated one.
func TestSurfaceFuelConsumptionUnknownFuel(t *testing.T) {
	for _, code := range []string{"D2", "WA", "NF", "", "not a fuel", "C8"} {
		if got := SurfaceFuelConsumption(code, 90, 60, 50, 0.35); got != 0 {
			t.Errorf("%q: SFC = %v, want 0 — an unimplemented fuel must not come back "+
				"as a plausible fuel load", code, got)
		}
	}
	// The fold is CanonicalFuelCode's, so spelling latitude carries here too.
	want := SurfaceFuelConsumption("C2", 90, 60, 50, 0.35)
	for _, code := range []string{"c2", "C-2", " C2 ", "c_2"} {
		if got := SurfaceFuelConsumption(code, 90, 60, 50, 0.35); got != want {
			t.Errorf("%q: SFC = %v, want C2's %v", code, got, want)
		}
	}
}

// Non-finite input, which is what a no-data cell in a raster looks like.
//
// NaN propagates, matching cffdrs' NA. An infinity does NOT reach the floor —
// see the doc comment, a -Inf BUI coming back as 1e-6 would read as a real stand
// with almost nothing in it.
func TestSurfaceFuelConsumptionNonFiniteInput(t *testing.T) {
	nan, posInf, negInf := math.NaN(), math.Inf(1), math.Inf(-1)

	for code := range Fuels {
		if nonBUIFuels[code] {
			continue
		}
		if got := SurfaceFuelConsumption(code, 90, nan, 50, 0.35); !math.IsNaN(got) {
			t.Errorf("%s with NaN BUI: SFC = %v, want NaN", code, got)
		}
		if got := SurfaceFuelConsumption(code, 90, negInf, 50, 0.35); !math.IsNaN(got) {
			t.Errorf("%s with -Inf BUI: SFC = %v, want NaN — an infinity must not arrive "+
				"at the floor and read as almost no fuel", code, got)
		}
	}
	for _, code := range []string{"C1", "C7"} {
		if got := SurfaceFuelConsumption(code, nan, 60, 50, 0.35); !math.IsNaN(got) {
			t.Errorf("%s with NaN FFMC: SFC = %v, want NaN", code, got)
		}
	}
	if got := SurfaceFuelConsumption("O1A", 90, 60, 50, posInf); !math.IsNaN(got) {
		t.Errorf("O1A with +Inf GFL: SFC = %v, want NaN", got)
	}
	if got := SurfaceFuelConsumption("O1A", 90, 60, 50, nan); !math.IsNaN(got) {
		t.Errorf("O1A with NaN GFL: SFC = %v, want NaN", got)
	}
	// A driver a fuel does not read is not a driver. A conifer stand with no
	// mixedwood or grass columns must not be poisoned by their absence.
	if got := SurfaceFuelConsumption("C2", 90, 60, nan, nan); math.IsNaN(got) {
		t.Error("C2 with NaN PC and GFL: SFC is NaN, but eq. 10 reads neither")
	}
	if got := SurfaceFuelConsumption("D1", 90, 60, nan, nan); math.IsNaN(got) {
		t.Error("D1 with NaN PC and GFL: SFC is NaN, but eq. 16 reads neither")
	}
	// An unknown fuel is decided before any driver is read.
	if got := SurfaceFuelConsumption("D2", nan, nan, nan, nan); got != 0 {
		t.Errorf("D2 with every driver NaN: SFC = %v, want 0", got)
	}
}

// SFC's reason for existing is that it is eq. 57's divisor, so the composition
// with crown.go is worth one assertion of its own: a real fuel at a real buildup
// must produce a finite, usable crowning threshold, and the floor must keep the
// degenerate case finite too rather than handing back the +Inf that
// CriticalSurfaceROS reserves for a genuinely fuel-free cell.
func TestSurfaceFuelConsumptionFeedsTheCrownThreshold(t *testing.T) {
	csi := CriticalSurfaceIntensity(120, 3)
	for code := range Fuels {
		sfc := SurfaceFuelConsumption(code, 90, 60, 50, 0.35)
		rso := CriticalSurfaceROS(csi, sfc)
		if math.IsInf(rso, 0) || math.IsNaN(rso) || !(rso > 0) {
			t.Errorf("%s: SFC %v gives RSO %v — a real stand must have a finite crowning "+
				"threshold", code, sfc, rso)
		}
	}
	// At BUI 0 the floor keeps it finite. Enormous, because almost nothing is
	// burning, but finite and therefore printable.
	sfc := SurfaceFuelConsumption("C2", 90, 0, 50, 0.35)
	if rso := CriticalSurfaceROS(csi, sfc); math.IsInf(rso, 0) {
		t.Errorf("C2 at BUI 0: SFC %v gives an infinite RSO — the floor exists to prevent "+
			"exactly this", sfc)
	}
}

// Total and crown fuel consumption, unconditionally.
//
// The oracle test above skips on a fresh clone; everything below is what CI runs
// on every push, so the properties that matter most — the CFL factor is really
// there, the two mixedwood weightings apply to the right families, and nothing
// non-finite escapes — are asserted here rather than only against the fixture.

// The identity eq. 67 is: with nothing in the crown, the total IS the surface.
//
// Worth its own test because it is the property the fixture's ~11000
// sentinel-CFL rows carry and nothing else can: a CFB of zero has to zero CFC
// for every fuel, including the four that carry a weighting.
func TestTotalFuelConsumptionWithNoCrownFireIsSurfaceFuelConsumption(t *testing.T) {
	for code := range Fuels {
		sfc := SurfaceFuelConsumption(code, 90, 60, 50, 0.35)
		for _, cfl := range []float64{0, 0.5, 1, 2, 1e6} {
			if got := TotalFuelConsumption(code, cfl, 0, sfc, 50, 35); got != sfc {
				t.Errorf("%s cfl=%v cfb=0: TFC = %v, want SFC %v", code, cfl, got, sfc)
			}
			if got := CrownFuelConsumption(code, cfl, 0, 50, 35); got != 0 {
				t.Errorf("%s cfl=%v cfb=0: CFC = %v, want 0", code, cfl, got)
			}
		}
	}
}

// Eq. 66a is a product, so CFC has to be linear in each factor separately. This
// is the assertion a fixture at one CFL cannot make — see the oracle test's
// header for why the generator grew a block to be able to make it there too.
func TestCrownFuelConsumptionIsLinearInCrownFuelLoad(t *testing.T) {
	const cfb = 0.4
	for code := range Fuels {
		base := CrownFuelConsumption(code, 1, cfb, 50, 35)
		for _, k := range []float64{0.25, 0.5, 2, 10} {
			got := CrownFuelConsumption(code, k, cfb, 50, 35)
			want := k * base
			if math.Abs(got-want) > 1e-12*math.Max(1, math.Abs(want)) {
				t.Errorf("%s: CFC at CFL %v = %v, want %v × CFC at CFL 1 (%v)", code, k, got, k, base)
			}
		}
	}
	// And in CFB, which is the other factor of the same product.
	for code := range Fuels {
		base := CrownFuelConsumption(code, 1.2, 1, 50, 35)
		for _, cfb := range []float64{0.1, 0.5, 0.9} {
			got := CrownFuelConsumption(code, 1.2, cfb, 50, 35)
			want := cfb * base
			if math.Abs(got-want) > 1e-12*math.Max(1, math.Abs(want)) {
				t.Errorf("%s: CFC at CFB %v = %v, want %v × CFC at CFB 1 (%v)", code, cfb, got, cfb, base)
			}
		}
	}
}

// Which input weights which family. The names invite the wrong guess — cffdrs'
// M2 takes PC, not PDF, despite reading like a dead-fir fuel — and getting it
// backwards is silent: both are percentages, both are swept, and a transposed
// pair still returns a plausible kilogram per square metre.
func TestCrownFuelConsumptionWeightsTheRightMixedwoodInput(t *testing.T) {
	// Deliberately float64 variables rather than untyped constants: Go folds
	// constant arithmetic exactly, so a const product would be 0.9 where the
	// function's own 1.5 × 0.6 is 0.8999999999999999, and the test would be
	// checking Go's constant evaluator instead of eq. 66.
	cfl, cfb := 1.5, 0.6
	product := cfl * cfb

	for _, code := range []string{"M1", "M2"} {
		// Eq. 66b: PC scales it, PDF does not reach it.
		if got := CrownFuelConsumption(code, cfl, cfb, 40, 35); math.Abs(got-0.40*product) > 1e-12 {
			t.Errorf("%s: CFC at PC 40 = %v, want %v", code, got, 0.40*product)
		}
		a := CrownFuelConsumption(code, cfl, cfb, 40, 0)
		b := CrownFuelConsumption(code, cfl, cfb, 40, 100)
		if a != b {
			t.Errorf("%s: PDF changed CFC (%v at PDF 0, %v at PDF 100) — eq. 66b reads PC", code, a, b)
		}
	}
	for _, code := range []string{"M3", "M4"} {
		// Eq. 66c: PDF scales it, PC does not reach it.
		if got := CrownFuelConsumption(code, cfl, cfb, 50, 40); math.Abs(got-0.40*product) > 1e-12 {
			t.Errorf("%s: CFC at PDF 40 = %v, want %v", code, got, 0.40*product)
		}
		a := CrownFuelConsumption(code, cfl, cfb, 0, 40)
		b := CrownFuelConsumption(code, cfl, cfb, 100, 40)
		if a != b {
			t.Errorf("%s: PC changed CFC (%v at PC 0, %v at PC 100) — eq. 66c reads PDF", code, a, b)
		}
	}
	// And every other fuel is the bare product of eq. 66a, indifferent to both.
	for code := range Fuels {
		switch canonical, _ := CanonicalFuelCode(code); canonical {
		case "M1", "M2", "M3", "M4":
			continue
		}
		for _, pc := range []float64{0, 50, 100} {
			for _, pdf := range []float64{0, 50, 100} {
				if got := CrownFuelConsumption(code, cfl, cfb, pc, pdf); got != product {
					t.Errorf("%s at pc=%v pdf=%v: CFC = %v, want the unweighted %v",
						code, pc, pdf, got, product)
				}
			}
		}
	}
}

// The published crown fuel load for D1, S1, S2, S3, O1A and O1B is 0, and this
// package does not carry that table — CFL is the caller's. So the zero has to
// arrive through the arithmetic, and it does: eq. 66a has no per-fuel gate.
//
// The other half of the same point is that a caller who supplies a positive CFL
// for one of those fuels gets a positive CFC back. That is not this package
// second-guessing the caller; it is the same behaviour fbp() has once CFL is in
// (0, 2], and the fixture's D1 rows assert it.
func TestCrownFuelConsumptionHasNoPerFuelGate(t *testing.T) {
	for _, code := range []string{"D1", "S1", "S2", "S3", "O1A", "O1B"} {
		if got := CrownFuelConsumption(code, 0, 0.5, 50, 35); got != 0 {
			t.Errorf("%s at the published CFL of 0: CFC = %v, want 0", code, got)
		}
		if got := CrownFuelConsumption(code, 0.8, 0.5, 50, 35); got != 0.4 {
			t.Errorf("%s at a caller-supplied CFL of 0.8: CFC = %v, want 0.4 — eq. 66a "+
				"applies no fuel test of its own", code, got)
		}
	}
}

// An unknown fuel code returns 0 from both, which is a departure from cffdrs —
// its ifelse chain falls through to the unweighted product for any name it does
// not recognise. See the doc comments for why 0 is the safer answer here, and
// note that TFC returns 0 rather than SFC: a surface-only total for a fuel that
// could not be classified is the number that would look ordinary downstream.
func TestTotalFuelConsumptionUnknownFuel(t *testing.T) {
	for _, code := range []string{"", "C8", "X1", "NoSuchFuel", "M5"} {
		if got := CrownFuelConsumption(code, 1.2, 0.5, 50, 35); got != 0 {
			t.Errorf("CrownFuelConsumption(%q) = %v, want 0", code, got)
		}
		if got := TotalFuelConsumption(code, 1.2, 0.5, 3.0, 50, 35); got != 0 {
			t.Errorf("TotalFuelConsumption(%q) = %v, want 0 (not the surface value)", code, got)
		}
	}
	// The codes that ARE known keep working through the same fold, separators and
	// case included — CanonicalFuelCode is the only spelling authority here.
	for _, code := range []string{"c2", "C-2", "o1a", "M 3"} {
		if got := CrownFuelConsumption(code, 1, 0.5, 50, 35); got == 0 && code != "o1a" {
			t.Errorf("CrownFuelConsumption(%q) = 0 — the code was not recognised", code)
		}
	}
}

// Nothing non-finite escapes as an infinity. A NaN driver a fuel actually reads
// propagates, which is cffdrs' behaviour; an infinite load comes back as NaN
// rather than +Inf, for the reason SurfaceFuelConsumption gives — it would reach
// eq. 69 as an infinite fireline intensity and read as a number.
func TestTotalFuelConsumptionNonFiniteInput(t *testing.T) {
	inf, ninf := math.Inf(1), math.Inf(-1)

	for _, tc := range []struct {
		name          string
		code          string
		cfl, cfb, sfc float64
		pc, pdf       float64
	}{
		{"infinite CFL", "C2", inf, 0.5, 3, 50, 35},
		{"negative infinite CFL", "C2", ninf, 0.5, 3, 50, 35},
		{"infinite CFB", "C2", 1, inf, 3, 50, 35},
		{"infinite CFL, M1", "M1", inf, 0.5, 3, 50, 35},
		{"infinite PC on M1", "M1", 1, 0.5, 3, inf, 35},
		{"infinite PDF on M3", "M3", 1, 0.5, 3, 50, inf},
	} {
		if got := CrownFuelConsumption(tc.code, tc.cfl, tc.cfb, tc.pc, tc.pdf); math.IsInf(got, 0) {
			t.Errorf("%s: CFC = %v — an infinity escaped", tc.name, got)
		}
		if got := TotalFuelConsumption(tc.code, tc.cfl, tc.cfb, tc.sfc, tc.pc, tc.pdf); math.IsInf(got, 0) {
			t.Errorf("%s: TFC = %v — an infinity escaped", tc.name, got)
		}
	}
	// An infinite SFC is the caller's parameter rather than anything computed
	// here, and it is caught in the same place for the same reason.
	if got := TotalFuelConsumption("C2", 1, 0.5, inf, 50, 35); math.IsInf(got, 0) {
		t.Errorf("infinite SFC: TFC = %v — an infinity escaped", got)
	}

	// NaN propagates rather than being swallowed into a plausible number.
	nan := math.NaN()
	for _, tc := range []struct {
		name          string
		code          string
		cfl, cfb, sfc float64
		pc, pdf       float64
	}{
		{"NaN CFL", "C2", nan, 0.5, 3, 50, 35},
		{"NaN CFB", "C2", 1, nan, 3, 50, 35},
		{"NaN PC on M1", "M1", 1, 0.5, 3, nan, 35},
		{"NaN PDF on M4", "M4", 1, 0.5, 3, 50, nan},
	} {
		if got := CrownFuelConsumption(tc.code, tc.cfl, tc.cfb, tc.pc, tc.pdf); !math.IsNaN(got) {
			t.Errorf("%s: CFC = %v, want NaN", tc.name, got)
		}
	}
	if got := TotalFuelConsumption("C2", 1, 0.5, nan, 50, 35); !math.IsNaN(got) {
		t.Errorf("NaN SFC: TFC = %v, want NaN", got)
	}
	// PC does not reach a conifer, so a NaN in it must NOT contaminate the
	// answer — the same "passing a zero for a driver a fuel does not read is
	// harmless" contract SurfaceFuelConsumption states.
	if got := CrownFuelConsumption("C2", 1, 0.5, nan, nan); math.IsNaN(got) {
		t.Error("C2: a NaN in PC/PDF reached eq. 66a, which reads neither")
	}
}

// There is no floor under either quantity, unlike SFC's 1e-6. That is cffdrs'
// behaviour — the clamp applies to SFC before this addition — and it is worth
// pinning, because adding one here would be the kind of quiet local decision
// this package exists not to make.
func TestTotalFuelConsumptionHasNoFloor(t *testing.T) {
	if got := CrownFuelConsumption("C2", 1e-12, 1e-12, 50, 35); got != 1e-24 {
		t.Errorf("CFC of two tiny factors = %v, want 1e-24 — something clamped it", got)
	}
	// SFC's own floor is the only one in the chain, and it survives the addition
	// intact when nothing crowns.
	sfc := SurfaceFuelConsumption("C2", 90, 0, 50, 0.35)
	if sfc != MinSurfaceFuelConsumptionKgM2 {
		t.Fatalf("C2 at BUI 0: SFC = %v, want the floor %v", sfc, MinSurfaceFuelConsumptionKgM2)
	}
	if got := TotalFuelConsumption("C2", 1, 0, sfc, 50, 35); got != MinSurfaceFuelConsumptionKgM2 {
		t.Errorf("TFC on a floored SFC with no crown fire = %v, want %v", got, MinSurfaceFuelConsumptionKgM2)
	}
}

// TFC is monotone in both halves: more surface fuel or more crown burned can
// only mean more total consumption. Cheap, and it is the shape of eq. 67 that a
// transposed argument would break.
func TestTotalFuelConsumptionIsMonotone(t *testing.T) {
	for code := range Fuels {
		prev := math.Inf(-1)
		for _, cfb := range []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9, 1} {
			got := TotalFuelConsumption(code, 1.2, cfb, 2.5, 50, 35)
			if got < prev {
				t.Errorf("%s: TFC fell from %v to %v as CFB rose to %v", code, prev, got, cfb)
			}
			prev = got
		}
		prev = math.Inf(-1)
		for _, sfc := range []float64{0, 0.5, 1, 3, 10} {
			got := TotalFuelConsumption(code, 1.2, 0.5, sfc, 50, 35)
			if got < prev {
				t.Errorf("%s: TFC fell from %v to %v as SFC rose to %v", code, prev, got, sfc)
			}
			prev = got
		}
	}
}

// Every fuel in Fuels has to reach one of eq. 66's three branches — the guard
// TestSurfaceFuelConsumptionCoversEveryFuel is for the switch above.
func TestCrownFuelConsumptionCoversEveryFuel(t *testing.T) {
	for code := range Fuels {
		if got := CrownFuelConsumption(code, 1, 1, 100, 100); got != 1 {
			t.Errorf("%s: CFC at CFL 1, CFB 1 and both weights at 100%% = %v, want 1", code, got)
		}
	}
}
