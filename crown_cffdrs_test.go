package fbp

import (
	"math"
	"sort"
	"testing"
)

// The crown-fire threshold against the oracle.
//
// These use the fixture's crown sweep — the rows that carry an explicit CBH and
// CFL, see cffdrsCase.usableForCrown. The surface sweeps send the -1 sentinel
// that makes fbp() substitute its own per-fuel table, and since fbp() returns
// neither value back, those rows cannot say what threshold the oracle used.
//
// The chain is asserted in pieces rather than end to end, and the reason is the
// same one behind splitting WSV from ROS in TestCFFDRSSlopeBackSolve: CSI, RSO
// and CFB are three transcribed equations, and a single composed assertion would
// say one of them is wrong without saying which.

// countCrownCases is the anti-no-op floor these tests share. A fixture
// regeneration that drops or renames the crown sweep would otherwise leave every
// loop below skipping and every test passing green having asserted nothing —
// the failure mode TestCFFDRSSlopeBackSolve guards the same way.
func countCrownCases(t *testing.T, f cffdrsFixture) int {
	t.Helper()
	n := 0
	for _, c := range f.Cases {
		if c.usableForCrown() {
			n++
		}
	}
	if n < 500 {
		t.Fatalf("only %d cases carry an explicit CBH and CFL — the crown threshold is "+
			"effectively unasserted. Has the crown sweep in gen_cffdrs_reference.R been dropped?", n)
	}
	return n
}

// Eqs. 56 and 57: the threshold itself, fed the oracle's own FMC and SFC.
//
// Those two are caller-supplied inputs to this package by design — it implements
// neither the foliar-moisture model nor surface fuel consumption — so taking them
// from the fixture is not a shortcut, it is the actual contract. What is being
// checked is the four transcribed numbers in eq. 56 (0.001, 1.5, 460, 25.9) and
// the 300 in eq. 57.
//
// C6 is included here. Its ROS and CFB are a different quantity (see
// crownChangesROS), but CSI and RSO are the same equations for every fuel.
func TestCFFDRSCrownThreshold(t *testing.T) {
	f := loadCFFDRS(t)
	countCrownCases(t, f)
	const tol = 1e-9
	perFuel := map[string]int{}
	var worstCSI, worstRSO float64
	shown := 0
	for i, c := range f.Cases {
		if !c.usableForCrown() {
			continue
		}
		fuel := ourFuel(c.Fuel)
		perFuel[fuel]++

		csi := CriticalSurfaceIntensity(c.FMC, c.CBH)
		if rel := relErr(csi, c.CSI); rel > worstCSI {
			worstCSI = rel
		}
		if !closeEnough(csi, c.CSI, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s fmc=%v cbh=%v: CSI = %v, cffdrs = %v (ratio %.6f)",
					i, c.Fuel, c.FMC, c.CBH, csi, c.CSI, csi/c.CSI)
			}
		}

		rso := CriticalSurfaceROS(c.CSI, c.SFC)
		if rel := relErr(rso, c.RSO); rel > worstRSO {
			worstRSO = rel
		}
		if !closeEnough(rso, c.RSO, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s csi=%v sfc=%v: RSO = %v, cffdrs = %v (ratio %.6f)",
					i, c.Fuel, c.CSI, c.SFC, rso, c.RSO, rso/c.RSO)
			}
		}
	}
	if shown > 15 {
		t.Errorf("... and %d more threshold mismatches", shown-15)
	}
	logCrownPerFuel(t, perFuel)
	t.Logf("worst relative error: CSI %.3g, RSO %.3g", worstCSI, worstRSO)

	// FMC is only ever reached through latitude and day of year, so a sweep that
	// collapsed to one date would reproduce eq. 56 at a single foliar moisture and
	// say nothing about the 25.9. Note that cffdrs forces FMC to 0 for the fuels
	// with no crown, so the distinct values are counted over conifers only.
	fmcs := map[float64]bool{}
	for _, c := range f.Cases {
		if c.usableForCrown() && c.FMC > 0 {
			fmcs[c.FMC] = true
		}
	}
	if len(fmcs) < 5 {
		t.Errorf("only %d distinct non-zero FMC values in the crown sweep — eq. 56 is "+
			"pinned at a point, not oracled. Widen LAT_VALUES/DJ_VALUES.", len(fmcs))
	}
	t.Logf("%d distinct non-zero FMC values across the crown sweep", len(fmcs))
}

// Eq. 58, twice over, and the two passes answer different questions.
//
// Pass one feeds the oracle's own reported ROS. That isolates the crown equations
// completely: if it fails, eq. 58 or the CFL gate is wrong and nothing else is
// implicated.
//
// Pass two feeds this package's own surface rate, composed the way a caller would
// — the full slope path, RSI(ISI(FFMC, WSV)) · BE with no slope factor. That is
// the end-to-end claim, and it is the one that would catch a caller-visible
// mistake such as feeding CFB the RSI · BE · SF product instead. The two are kept
// apart because pass two failing while pass one passes localises the fault to the
// spread rate rather than the threshold.
//
// C6 is excluded from both. cffdrs computes its CFB from a crown rate of spread
// this package does not implement, so its cfb column is not this quantity.
func TestCFFDRSCrownFractionBurned(t *testing.T) {
	f := loadCFFDRS(t)
	countCrownCases(t, f)
	const tol = 1e-9
	perFuel := map[string]int{}
	crowning := 0
	var worstOracleROS, worstOurROS float64
	shown := 0
	for i, c := range f.Cases {
		if !c.usableForCrown() || crownChangesROS(c) {
			continue
		}
		fuel := ourFuel(c.Fuel)
		perFuel[fuel]++
		if c.CFB > 0 {
			crowning++
		}
		crown := Crown{FMC: c.FMC, SFC: c.SFC, CBH: c.CBH, CFL: c.CFL}

		crown.SurfaceROS = c.ROS
		got := CrownFractionBurned(crown)
		if rel := relErr(got, c.CFB); rel > worstOracleROS {
			worstOracleROS = rel
		}
		if !closeEnough(got, c.CFB, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s cbh=%v fmc=%v sfc=%v ros=%v: CFB = %v, cffdrs = %v",
					i, c.Fuel, c.CBH, c.FMC, c.SFC, c.ROS, got, c.CFB)
			}
		}

		// End to end. The surface rate has to come from the full slope path, not
		// from ROS(..., slopePct): the slope is already inside WSV, and the
		// simplified product would over-predict crowning by the whole slope
		// factor — up to tenfold, straight into an exponential.
		ros, _ := slopeAdjustedROS(SlopeWind{
			Code: fuel, FFMC: c.FFMC, SlopePct: c.GS, WindKmh: c.WS,
			WindAzimuthDeg:    c.WD + 180,
			UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect, made explicit
			PC:                c.PC, PDF: c.PDF, CuringPct: c.CC,
		}, c.BUI)
		crown.SurfaceROS = ros
		got = CrownFractionBurned(crown)
		if rel := relErr(got, c.CFB); rel > worstOurROS {
			worstOurROS = rel
		}
		if !closeEnough(got, c.CFB, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s cbh=%v gs=%v%% ws=%v wd=%v: end-to-end CFB = %v, cffdrs = %v (our ROS %v vs %v)",
					i, c.Fuel, c.CBH, c.GS, c.WS, c.WD, got, c.CFB, ros, c.ROS)
			}
		}
	}
	if shown > 15 {
		t.Errorf("... and %d more CFB mismatches", shown-15)
	}
	logCrownPerFuel(t, perFuel)
	t.Logf("worst relative error: CFB from cffdrs' ROS %.3g, end to end %.3g",
		worstOracleROS, worstOurROS)

	// Without cases on both sides of the threshold this reduces to asserting that
	// zero equals zero, which eq. 58's exponential would pass unimplemented.
	if crowning == 0 {
		t.Fatal("no crowning cases in the crown sweep — eq. 58's exponential is unasserted")
	}
	t.Logf("%d of %d crown-sweep cases actually crown", crowning, sumCounts(perFuel))
}

// FD: the three-way classification, and specifically its boundary convention.
//
// Cases sitting within 1e-9 of a boundary are skipped. The classification is a
// step function of a quantity both implementations compute to about 1e-15, so a
// last-bit difference at exactly 0.1 or 0.9 would flip a class and report a
// disagreement about the equations that is really a disagreement about rounding.
// The exact-boundary behaviour is pinned by TestDescribeFireBoundaries instead,
// where it can be asserted without a float in the way.
func TestCFFDRSFireDescription(t *testing.T) {
	f := loadCFFDRS(t)
	countCrownCases(t, f)
	seen := map[FireDescription]int{}
	skipped, shown := 0, 0
	for i, c := range f.Cases {
		if !c.usableForCrown() || crownChangesROS(c) {
			continue
		}
		if nearBoundary(c.CFB) {
			skipped++
			continue
		}
		want := FireDescription(c.FD)
		got := DescribeFire(CrownFractionBurned(Crown{
			FMC: c.FMC, SFC: c.SFC, CBH: c.CBH, CFL: c.CFL, SurfaceROS: c.ROS,
		}))
		seen[want]++
		if got != want {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s cfb=%v: FD = %q, cffdrs = %q", i, c.Fuel, c.CFB, got, want)
			}
		}
	}
	if shown > 15 {
		t.Errorf("... and %d more FD mismatches", shown-15)
	}
	t.Logf("FD: S %d, I %d, C %d (%d skipped within 1e-9 of a class boundary)",
		seen[SurfaceFire], seen[IntermittentCrown], seen[ContinuousCrown], skipped)

	// All three classes have to appear or the test is asserting one branch. The
	// intermittent band is the narrow one — CFB crosses 0.1 to 0.9 over about
	// 9 m/min of excess spread — so it is the one a thinned sweep loses first.
	for _, fd := range []FireDescription{SurfaceFire, IntermittentCrown, ContinuousCrown} {
		if seen[fd] == 0 {
			t.Errorf("no %q cases in the crown sweep — that branch of FD is unasserted", fd)
		}
	}
}

func nearBoundary(cfb float64) bool {
	const eps = 1e-9
	return math.Abs(cfb-IntermittentCrownCFB) < eps || math.Abs(cfb-ContinuousCrownCFB) < eps
}

func sumCounts(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func logCrownPerFuel(t *testing.T, perFuel map[string]int) {
	t.Helper()
	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, fuel := range fuels {
		t.Logf("%-4s %5d crown cases", fuel, perFuel[fuel])
	}
}
