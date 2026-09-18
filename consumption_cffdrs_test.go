package fbp

import (
	"math"
	"sort"
	"testing"
)

// Surface fuel consumption against the oracle.
//
// This is the cheapest kind of oracle column there is: SFC was already in the
// fixture before this was ported, carried as an INPUT because the Go side
// deliberately did not compute it — see the note in gen_cffdrs_reference.R. The
// port did not change the generator and did not move the fixture digest. The
// column simply stopped being something the tests read and started being
// something they check.
//
// Every one of the fixture's cases is usable. SFC does not depend on wind,
// slope, curing or the crown inputs, so none of the exclusions the other oracle
// tests carry apply — not even C6, whose ROS is a different quantity from this
// package's but whose surface fuel consumption is plain eq. 12.

// sweepGrassFuelLoad is the GFL that gen_cffdrs_reference.R sends on every row.
//
// GFL is the one SFC driver the fixture does NOT carry a column for, because it
// is a constant in the generator rather than a swept input — see base_row(). So
// this has to be written down twice, and the two copies are joined by nothing
// but this comment. That is tolerable only because the join fails loudly: change
// GFL in the generator and TestCFFDRSSurfaceFuelConsumption's O1A/O1B cases go
// red on the next regeneration, naming both numbers.
//
// It is NOT a default. SurfaceFuelConsumption has no default grass fuel load and
// deliberately will not grow one; this is the value this particular sweep was
// generated with, nothing more.
const sweepGrassFuelLoad = 0.35

// Eleven equations at once — 9a/9b, 10, 11, 12, 13-15, 16, 17, 18, 19-25 — and
// 28 transcribed coefficients between them.
//
// The two branch points are what this is really for, because they are the two
// places cffdrs departs from FCFDG 1992 and a transcription from the paper alone
// would land somewhere else entirely:
//
//   - C1 at FFMC 84. The paper's eq. 9 is a single exponential in (FFMC - 81);
//     cffdrs uses the GLC-X-10 revision. At FFMC 60 the paper's form is deeply
//     negative and clamps to the 1e-6 floor while the revision gives ~0.0015, so
//     the sweep's FFMC 60 and 75 rows separate them by three orders of magnitude.
//   - C7 at FFMC 70. Eq. 13 as published has no branch and goes negative below
//     70, taking the eq. 15 sum with it; cffdrs substitutes 0 for that term and
//     keeps eq. 14's woody contribution. Again FFMC 60 and 75 separate them.
//
// Both are asserted rather than assumed: the counts below fail the test if the
// sweep ever stops straddling either hinge.
//
// ledger: surface_fuel_consumption.r
func TestCFFDRSSurfaceFuelConsumption(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	perFuel := map[string]int{}
	var worst float64
	var worstCase string
	shown := 0

	for i, c := range f.Cases {
		fuel := ourFuel(c.Fuel)
		perFuel[fuel]++

		got := SurfaceFuelConsumption(c.Fuel, c.FFMC, c.BUI, c.PC, sweepGrassFuelLoad)
		if rel := relErr(got, c.SFC); rel > worst {
			worst, worstCase = rel, c.Fuel
		}
		if !closeEnough(got, c.SFC, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s ffmc=%v bui=%v pc=%v: SFC = %v, cffdrs = %v (ratio %.9f)",
					i, c.Fuel, c.FFMC, c.BUI, c.PC, got, c.SFC, got/c.SFC)
			}
		}
	}
	if shown > 15 {
		t.Errorf("... and %d more SFC mismatches", shown-15)
	}

	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, fuel := range fuels {
		t.Logf("%-4s %6d cases", fuel, perFuel[fuel])
	}
	t.Logf("worst relative error: %.3g (%s)", worst, worstCase)

	// Every fuel this package implements has to be in there. A sweep that
	// quietly stopped emitting S3 would leave eqs. 23/24/25 untested while this
	// test stayed green.
	for code := range Fuels {
		if perFuel[code] == 0 {
			t.Errorf("no fixture case for %s — its SFC equation is unasserted", code)
		}
	}

	// The two hinges. See the doc comment: these are the cffdrs-vs-paper
	// departures, and a sweep that sat entirely on one side of either would pass
	// this test while saying nothing about the branch that matters.
	var c1Below, c1Above, c7Below, c7Above int
	for _, c := range f.Cases {
		switch ourFuel(c.Fuel) {
		case "C1":
			if c.FFMC > 84 {
				c1Above++
			} else {
				c1Below++
			}
		case "C7":
			if c.FFMC > 70 {
				c7Above++
			} else {
				c7Below++
			}
		}
	}
	if c1Below == 0 || c1Above == 0 {
		t.Errorf("C1 cases do not straddle the eq. 9a/9b hinge at FFMC 84: %d below, %d above. "+
			"The GLC-X-10 revision is then unasserted on one side.", c1Below, c1Above)
	}
	if c7Below == 0 || c7Above == 0 {
		t.Errorf("C7 cases do not straddle the eq. 13 clamp at FFMC 70: %d below, %d above. "+
			"cffdrs' undocumented departure from the paper is then unasserted.", c7Below, c7Above)
	}
	t.Logf("C1 straddles FFMC 84: %d below, %d above; C7 straddles FFMC 70: %d below, %d above",
		c1Below, c1Above, c7Below, c7Above)

	// SFC is a function of FFMC, BUI, PC and fuel, so a sweep that collapsed to
	// one buildup would reproduce eleven equations at a single point and check
	// almost none of their shape.
	distinct := map[float64]bool{}
	for _, c := range f.Cases {
		distinct[c.SFC] = true
	}
	// 50, against the 103 the sweep currently produces: low enough not to break on
	// an ordinary widening of BUI_VALUES, high enough that a sweep collapsed to one
	// buildup (~17, one per fuel group) fails outright.
	if len(distinct) < 50 {
		t.Errorf("only %d distinct SFC values across %d cases — the equations are pinned at "+
			"points rather than oracled. Has BUI_VALUES been narrowed?", len(distinct), len(f.Cases))
	}
	t.Logf("%d distinct SFC values across %d cases", len(distinct), len(f.Cases))

	// Grass is the one fuel family whose "equation" is an identity on an input
	// the fixture does not carry (eq. 18, SFC = GFL). Its rows therefore assert
	// the generator's constant rather than a coefficient, and saying so here is
	// cheaper than leaving a reader to work out why O1A/O1B can never fail.
	grass := 0
	for i, c := range f.Cases {
		if fuel := ourFuel(c.Fuel); fuel != "O1A" && fuel != "O1B" {
			continue
		}
		grass++
		if c.SFC != sweepGrassFuelLoad {
			t.Errorf("case %d %s: fixture SFC = %v, but the generator sends GFL = %v. "+
				"Eq. 18 is SFC = GFL; either base_row() changed or fbp() stopped honouring it.",
				i, c.Fuel, c.SFC, sweepGrassFuelLoad)
		}
	}
	t.Logf("%d grass cases assert eq. 18 as an identity on GFL = %v, not a coefficient",
		grass, sweepGrassFuelLoad)
}

// The floor is the one part of SurfaceFuelConsumption the fixture cannot reach.
//
// BUI_VALUES starts at 1, not 0, so no swept row lands on MinSurfaceFuelConsumptionKgM2
// — every BUI-driven equation is comfortably positive there. That is worth
// stating in an oracle test rather than only in an unconditional one, because it
// is a statement about the FIXTURE's coverage: the clamp in the reference
// implementations is reproduced on the strength of reading them, and
// TestSurfaceFuelConsumptionFloor is the only thing holding it.
//
// ledger: surface_fuel_consumption.r
func TestCFFDRSSurfaceFuelConsumptionFloorIsUnreached(t *testing.T) {
	f := loadCFFDRS(t)
	for i, c := range f.Cases {
		if c.SFC <= MinSurfaceFuelConsumptionKgM2 {
			t.Fatalf("case %d %s bui=%v: fixture SFC = %v is at or below the floor. The sweep now "+
				"reaches it, so the clamp IS oracle-backed — say so here and in MIGRATION.md "+
				"rather than leaving this test claiming otherwise.", i, c.Fuel, c.BUI, c.SFC)
		}
		if math.IsNaN(c.SFC) {
			t.Fatalf("case %d %s: fixture SFC is NaN", i, c.Fuel)
		}
	}
	t.Logf("all %d fixture SFC values are strictly above the %v floor; the clamp is "+
		"asserted only by TestSurfaceFuelConsumptionFloor", len(f.Cases), MinSurfaceFuelConsumptionKgM2)
}
