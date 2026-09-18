package fbp

import (
	"math"
	"sort"
	"testing"
)

// Foliar moisture content against the oracle.
//
// Unlike SFC, this port could not be made on the fixture that was already there.
// The `fmc` column existed and was read as an input, but LONG, ELV and D0 were
// constants in gen_cffdrs_reference.R — 15, 0 and 0 on every one of its 23,532
// rows — so the fixture reached exactly one of the model's three paths. Eqs. 3/4
// (the elevation branch) and the caller-supplied-D0 branch had no coverage at
// all, and eqs. 1/2's longitude exponential was pinned at a single point. A port
// asserted only against that would have looked complete while leaving seven of
// the model's twelve coefficients unchecked.
//
// So this is an oracle case (b): the generator grew an FMC block and three input
// columns, and the fixture digest moved. See MIGRATION.md's Log for what
// fixture-diff said about whether anything ELSE moved with it.

// fbpZeroesFMC reports whether cffdrs' fbp() overwrites this fuel's FMC with 0
// before returning it.
//
// It is the driver's doing, not foliar_moisture_content()'s: fbp() runs
//
//	FMC <- ifelse(FUELTYPE %in% c("D1","S1","S2","S3","O1A","O1B"), 0, FMC)
//
// after computing FMC, on the reasoning that a fuel with no crown has no foliage
// whose moisture matters. The fixture's `fmc` column for those fuels is therefore
// a statement about fbp(), not about eqs. 1-8, and a test that compared it
// against this package would be asserting that FoliarMoistureContent knows about
// fuel types. It does not, deliberately — see foliar.go.
//
// This is a list of fuel codes and not a property of the Fuels table because it
// is a reading of the reference implementation's source. If it is ever wrong, the
// count logged below drops and the exclusions become visible rather than silent.
func fbpZeroesFMC(fuel string) bool {
	switch ourFuel(fuel) {
	case "D1", "S1", "S2", "S3", "O1A", "O1B":
		return true
	}
	return false
}

// Eight equations and twelve transcribed coefficients: the two LATN forms, the
// two date scalings, the elevation term, and the three pieces of the ND curve.
//
// The fixture's LONG is signed as the generator sent it and fbp() folds the sign
// itself, so math.Abs is applied here rather than inside FoliarMoistureContent —
// see that function on why the package has no driver to do it. The sweep carries
// one deliberately negative longitude, so this line is asserted rather than
// assumed: drop the math.Abs and that site fails.
//
// ledger: foliar_moisture_content.r
func TestCFFDRSFoliarMoistureContent(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	perFuel := map[string]int{}
	var worst float64
	var worstCase string
	n, skipped, shown := 0, 0, 0

	for i, c := range f.Cases {
		if fbpZeroesFMC(c.Fuel) {
			skipped++
			if c.FMC != 0 {
				t.Errorf("case %d %s: fixture FMC = %v, but fbp() zeroes the crownless "+
					"fuels. Either that list changed upstream or this exclusion is wrong; "+
					"do not widen it to make this pass.", i, c.Fuel, c.FMC)
			}
			continue
		}
		n++
		perFuel[ourFuel(c.Fuel)]++

		got := FoliarMoistureContent(c.LAT, math.Abs(c.LONG), c.ELV, c.DJ, c.D0)
		if rel := relErr(got, c.FMC); rel > worst {
			worst, worstCase = rel, c.Fuel
		}
		if !closeEnough(got, c.FMC, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s lat=%v long=%v elv=%v dj=%v d0=%v: FMC = %v, cffdrs = %v",
					i, c.Fuel, c.LAT, c.LONG, c.ELV, c.DJ, c.D0, got, c.FMC)
			}
		}
	}
	if shown > 15 {
		t.Errorf("... and %d more FMC mismatches", shown-15)
	}

	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, fuel := range fuels {
		t.Logf("%-4s %6d cases", fuel, perFuel[fuel])
	}
	t.Logf("%d cases asserted, %d skipped as fuels fbp() zeroes FMC for", n, skipped)
	t.Logf("worst relative error: %.3g (%s)", worst, worstCase)
	if n == 0 {
		t.Fatal("no fixture case asserts FMC")
	}
}

// The coverage this port needed the sweep widened for.
//
// Separate from the assertion above because it is a statement about the FIXTURE
// rather than about the code: a green TestCFFDRSFoliarMoistureContent over a
// sweep that only ever reached eqs. 1/2 at one longitude would be exactly the
// "looks complete" failure MIGRATION.md warned about. Each count below is a
// branch that had no coverage before the FMC block existed, and a zero here means
// it has none again.
//
// ledger: foliar_moisture_content_minimum.r
func TestCFFDRSFoliarMoistureContentReachesEveryBranch(t *testing.T) {
	f := loadCFFDRS(t)

	var near, mid, far int         // eqs. 6, 7, 8
	var flatBranch, elevBranch int // eqs. 1/2 against eqs. 3/4
	var suppliedD0, computedD0 int // the caller-supplied minimum date
	var halfD0, negativeLong int   // the rounding rule, and fbp()'s sign fold
	longs := map[float64]bool{}    // distinct longitudes on the eq. 1/2 branch
	elevs := map[float64]bool{}    // distinct elevations on the eq. 3/4 branch
	for _, c := range f.Cases {
		if fbpZeroesFMC(c.Fuel) {
			continue
		}
		if c.D0 > 0 {
			suppliedD0++
			if c.D0 != math.Trunc(c.D0) {
				halfD0++
			}
		} else {
			computedD0++
			if c.ELV > 0 {
				elevBranch++
				elevs[c.ELV] = true
			} else {
				flatBranch++
				longs[math.Abs(c.LONG)] = true
			}
		}
		if c.LONG < 0 {
			negativeLong++
		}

		d0 := c.D0
		if d0 <= 0 {
			d0 = DateOfMinimumFoliarMoisture(c.LAT, math.Abs(c.LONG), c.ELV)
		} else {
			d0 = math.RoundToEven(d0)
		}
		switch nd := math.Abs(c.DJ - d0); {
		case nd < fmcNearDays:
			near++
		case nd < fmcFarDays:
			mid++
		default:
			far++
		}
	}

	for _, b := range []struct {
		n    int
		what string
	}{
		{near, "eq. 6 (ND < 30)"},
		{mid, "eq. 7 (30 <= ND < 50)"},
		{far, "eq. 8 (ND >= 50)"},
		{flatBranch, "eqs. 1, 2 — the ELV <= 0 branch"},
		{elevBranch, "eqs. 3, 4 — the ELV > 0 branch"},
		{computedD0, "D0 computed from the site"},
		{suppliedD0, "D0 supplied by the caller"},
		{halfD0, "a supplied D0 that is not a whole day — the rounding rule"},
		{negativeLong, "a negative longitude — fbp()'s sign fold"},
	} {
		if b.n == 0 {
			t.Errorf("no fixture case reaches %s. It was unasserted before the FMC block "+
				"was added to gen_cffdrs_reference.R; if that block has been narrowed, "+
				"widen it back rather than deleting this check.", b.what)
		}
	}

	// One longitude pins a point on the LATN exponential, not its shape. Same for
	// one elevation and the 0.0172 per metre term.
	if len(longs) < 4 {
		t.Errorf("only %d distinct longitudes on the eq. 1/2 branch — 46, 23.4 and 0.0360 "+
			"are pinned at a point rather than oracled", len(longs))
	}
	if len(elevs) < 4 {
		t.Errorf("only %d distinct elevations on the eq. 3/4 branch — 0.0172 per metre is "+
			"pinned at a point rather than oracled", len(elevs))
	}

	t.Logf("branches: eq.6 %d, eq.7 %d, eq.8 %d; eqs.1/2 %d rows over %d longitudes, "+
		"eqs.3/4 %d rows over %d elevations; D0 supplied %d (of which %d fractional), "+
		"computed %d; negative longitudes %d",
		near, mid, far, flatBranch, len(longs), elevBranch, len(elevs),
		suppliedD0, halfD0, computedD0, negativeLong)
}

// The rounding rule, isolated.
//
// cffdrs rounds D0 with R's round(), which goes to the even digit on an exact
// half; Go's math.Round goes away from zero. The sweep carries a site at
// D0 = 150.5, where the two rules give 150 and 151 and the resulting FMC differs.
// This test does not re-assert FMC — the test above does that — it asserts that
// the fixture can TELL THE TWO APART, because a coefficient check that cannot
// distinguish the rule it is checking is not checking it.
//
// ledger: foliar_moisture_content_minimum.r
func TestCFFDRSFoliarMoistureRoundingIsHalfToEven(t *testing.T) {
	f := loadCFFDRS(t)

	discriminating := 0
	for i, c := range f.Cases {
		if fbpZeroesFMC(c.Fuel) || c.D0 <= 0 {
			continue
		}
		toEven := math.RoundToEven(c.D0)
		awayFromZero := math.Round(c.D0)
		if toEven == awayFromZero {
			continue // 151.5 and every whole day: both rules agree, nothing to see
		}
		withEven := FoliarMoistureContent(c.LAT, math.Abs(c.LONG), c.ELV, c.DJ, toEven)
		withAway := FoliarMoistureContent(c.LAT, math.Abs(c.LONG), c.ELV, c.DJ, awayFromZero)
		if withEven == withAway {
			continue // an exact half, but both land on eq. 8's plateau
		}
		discriminating++
		if !closeEnough(withEven, c.FMC, 1e-9) {
			t.Errorf("case %d d0=%v dj=%v: cffdrs says FMC = %v. Half-to-even (D0 = %v) gives "+
				"%v and half-away-from-zero (D0 = %v) gives %v — the oracle is on the OTHER "+
				"rule, so math.RoundToEven in foliar.go is wrong.",
				i, c.D0, c.DJ, c.FMC, toEven, withEven, awayFromZero, withAway)
		}
	}

	if discriminating == 0 {
		t.Error("no fixture case separates half-to-even from half-away-from-zero. " +
			"FMC_SITES needs a D0 on an exact half (150.5) with a Dj inside eq. 6's " +
			"window, or the rounding rule is reproduced on a reading alone.")
	}
	t.Logf("%d fixture cases separate the two rounding rules, and all agree with "+
		"half-to-even", discriminating)
}
