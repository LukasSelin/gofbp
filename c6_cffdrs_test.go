package fbp

import (
	"math"
	"testing"
)

// C-6's crown path against the oracle.
//
// This is the row the ledger called the only 🔴 one whose oracle was already in
// the fixture on disk, and it is: C-6's ros and cfb columns ARE the blended
// quantity, so no generator change and no regeneration were needed to assert it.
// That also means these rows were in the fixture all along, excluded by name —
// see crownChangesROS, which stays exactly as it is. The exclusion is correct for
// what it guards (this package's ROS is the surface rate, and for C-6 the
// fixture's ros is not); what it was withholding was coverage of the C-6 crown
// path, and this test is where that coverage arrives instead.
//
// Three passes, each narrowing what a failure can mean — the reason
// TestCFFDRSCrownFractionBurned has two.
//
// Pass one isolates eq. 65 by feeding it the oracle's own CFB, and reaches all
// 948 C-6 rows because the blend needs no CBH.
//
// Pass two runs the whole chain — eqs. 61, 64, 58 and 65 composed — from the
// oracle's ISI, over the 768 crown-block rows. The other 180 sent the CBH/CFL
// sentinel and the fixture records the −1 rather than the table value fbp()
// substituted, so they cannot say what threshold the oracle used.
//
// Pass three is the only end-to-end one, and it is restricted to FLAT rows.
// # Why pass three cannot run on the sloped C6 rows
//
// Found while writing this test, and it is a gap in a different row rather than
// in this one. cffdrs' slope back-solve calls rate_of_spread(FUELTYPE, ISZ, …)
// for the zero-wind rate it inverts — and rate_of_spread for C6 returns
// rate_of_spread_extended's ROS, which is the BLENDED rate. So cffdrs takes a
// blended zero-wind rate, multiplies it by SF, and inverts C-6's SURFACE curve
// (a = 30, b = 0.08, c = 3) to recover an equivalent wind. The consequence is
// visible in the fixture: at GS 30 and WS 0, C-6's isi column moves with FMC and
// CBH — 15.93 at FMC 85.17 against 11.33 at FMC 120, same weather — which no
// surface-only back-solve can reproduce, and gofbp's NetEffectiveWind is
// surface-only for every fuel.
//
// That is why TestCFFDRSSlopeBackSolve excludes C6 through crownChangesROS, and
// the exclusion is doing more work than its comment claimed. Porting C-6's crown
// rate of spread does not lift it: the missing piece is in the back-solve, which
// is Slopecalc.r's row, not this one. MIGRATION.md carries the evidence and the
// status call it implies.
//
// ledger: C6calc.r
func TestCFFDRSC6RateOfSpread(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9

	// Pass one: eq. 65 alone. RSS and RSC are ours; CFB is the oracle's, which is
	// what makes a failure here eq. 65's and nothing else's.
	nBlend, badBlend, blended := 0, 0, 0
	var worstBlend float64
	minROS := math.Inf(1)
	for i, c := range f.Cases {
		if ourFuel(c.Fuel) != "C6" {
			continue
		}
		nBlend++
		rss := RSI("C6", c.ISI, c.PC, c.PDF, c.CC) * BuildupEffect("C6", c.BUI)
		rsc := C6CrownROS(c.ISI, c.FMC)
		if rsc > rss {
			blended++
		}
		got := C6ROS(rss, rsc, c.CFB)
		if rel := relErr(got, c.ROS); rel > worstBlend {
			worstBlend = rel
		}
		if !closeEnough(got, c.ROS, tol) {
			if badBlend++; badBlend <= 10 {
				t.Errorf("case %d C6 isi=%.6f bui=%v fmc=%v cfb=%v: blended ROS = %v, cffdrs = %v (RSS %v, RSC %v)",
					i, c.ISI, c.BUI, c.FMC, c.CFB, got, c.ROS, rss, rsc)
			}
		}
		minROS = math.Min(minROS, c.ROS)
	}
	if badBlend > 10 {
		t.Errorf("... and %d more eq. 65 mismatches", badBlend-10)
	}

	// Pass two: eqs. 61, 64, 58 and 65 composed, from the oracle's own ISI. That
	// is the ISI fbp() actually used, so a failure here is this chain's and
	// nothing else's.
	//
	// Pass three, in the same loop: the same chain from OUR ISI, back-solved from
	// FFMC and wind, on flat rows only. See the header for why the sloped ones
	// cannot be included.
	nChain, badChain, crowning := 0, 0, 0
	nEndToEnd, badEndToEnd := 0, 0
	var worstChainROS, worstChainCFB, worstEndToEnd float64
	for i, c := range f.Cases {
		if ourFuel(c.Fuel) != "C6" || !c.usableForCrown() {
			continue
		}
		nChain++
		rss := RSI("C6", c.ISI, c.PC, c.PDF, c.CC) * BuildupEffect("C6", c.BUI)
		crown := Crown{FMC: c.FMC, SFC: c.SFC, CBH: c.CBH, CFL: c.CFL, SurfaceROS: rss}
		ros, cfb := C6CrownFire(crown, c.ISI)
		if c.CFB > 0 {
			crowning++
		}
		if rel := relErr(ros, c.ROS); rel > worstChainROS {
			worstChainROS = rel
		}
		if rel := relErr(cfb, c.CFB); rel > worstChainCFB {
			worstChainCFB = rel
		}
		if !closeEnough(ros, c.ROS, tol) || !closeEnough(cfb, c.CFB, tol) {
			if badChain++; badChain <= 10 {
				t.Errorf("case %d C6 isi=%.6f bui=%v fmc=%v cbh=%v gs=%v%% ws=%v: ROS = %v / CFB = %v, cffdrs = %v / %v (RSS %v)",
					i, c.ISI, c.BUI, c.FMC, c.CBH, c.GS, c.WS, ros, cfb, c.ROS, c.CFB, rss)
			}
		}

		if c.GS != 0 {
			continue
		}
		nEndToEnd++
		// Flat ground, so NetEffectiveWind reduces to the wind itself and this is
		// the whole prediction from raw weather: FFMC and WS in, blended C-6 rate
		// out, with nothing taken from the fixture but the inputs and the crown
		// data a caller supplies.
		wsv, _ := NetEffectiveWind(SlopeWind{
			Code: "C6", FFMC: c.FFMC, SlopePct: c.GS, WindKmh: c.WS,
			WindAzimuthDeg:    c.WD + 180,
			UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect, made explicit
			PC:                c.PC, PDF: c.PDF, CuringPct: c.CC,
		})
		ourISI := ISI(c.FFMC, wsv)
		ourRSS := RSI("C6", ourISI, c.PC, c.PDF, c.CC) * BuildupEffect("C6", c.BUI)
		crown.SurfaceROS = ourRSS
		e2e, _ := C6CrownFire(crown, ourISI)
		if rel := relErr(e2e, c.ROS); rel > worstEndToEnd {
			worstEndToEnd = rel
		}
		if !closeEnough(e2e, c.ROS, tol) {
			if badEndToEnd++; badEndToEnd <= 10 {
				t.Errorf("case %d C6 ffmc=%v ws=%v bui=%v fmc=%v cbh=%v: end-to-end ROS = %v, cffdrs = %v (our ISI %v vs %v)",
					i, c.FFMC, c.WS, c.BUI, c.FMC, c.CBH, e2e, c.ROS, ourISI, c.ISI)
			}
		}
	}
	if badChain > 10 {
		t.Errorf("... and %d more C6 chain mismatches", badChain-10)
	}
	if badEndToEnd > 10 {
		t.Errorf("... and %d more C6 end-to-end mismatches", badEndToEnd-10)
	}

	t.Logf("C6   %5d cases: eq. 65 fed the oracle's CFB, %d mismatched, worst relative error %.3g",
		nBlend, badBlend, worstBlend)
	t.Logf("C6   %5d crown-block cases: the chain from the oracle's ISI, %d mismatched, worst relative error ROS %.3g CFB %.3g",
		nChain, badChain, worstChainROS, worstChainCFB)
	t.Logf("C6   %5d flat crown-block cases: end to end from FFMC and wind, %d mismatched, worst relative error %.3g",
		nEndToEnd, badEndToEnd, worstEndToEnd)

	// Anti-no-op floors. Both loops skip silently if the fixture stops carrying
	// C6, which would leave this test green having asserted nothing — the failure
	// mode TestCFFDRSSlopeBackSolve and countCrownCases both guard the same way.
	if nBlend < 500 {
		t.Fatalf("only %d C6 cases in the fixture — eq. 65 is effectively unasserted", nBlend)
	}
	if nChain < 500 {
		t.Fatalf("only %d C6 cases carry an explicit CBH and CFL — the chain is effectively unasserted", nChain)
	}
	if nEndToEnd < 200 {
		t.Fatalf("only %d flat C6 crown-block cases — nothing here asserts the chain end to end", nEndToEnd)
	}
	// Without cases that actually crown, eq. 65 reduces to returning its first
	// argument and eq. 64's coefficients are unchecked.
	if crowning == 0 {
		t.Fatal("no crowning C6 cases — eq. 65's blend term is unasserted")
	}
	t.Logf("%d of %d crown-block C6 cases crown; RSC exceeded RSS in %d of %d cases",
		crowning, nChain, blended, nBlend)

	// Two things this sweep does NOT reach, said out loud rather than left for a
	// reader to assume are covered.
	//
	// The RSC <= RSS branch of eqs. 58/65 is the first: RSC exceeds RSS in every
	// C6 row here, so neither the CFB gate nor eq. 65's else-branch is oracled.
	// That is not a thin sweep, it is a property of the equations inside the FMC
	// range the FBP System can produce — TestC6CrownGateIsUnreachableWithinTheFMCRange
	// carries the bound, and the branch itself is asserted unconditionally in
	// c6_test.go.
	if blended != nBlend {
		t.Logf("NOTE: %d C6 cases have RSC <= RSS, so the gate IS oracled now — "+
			"TestC6CrownGateIsUnreachableWithinTheFMCRange says that needs FMC above 127.26, "+
			"which eq. 8 cannot produce. Read the fixture's FMC range before believing this.",
			nBlend-blended)
	}
	// And the 1e-6 floor cffdrs puts under the final rate. It fires only where the
	// rate is non-positive; the smallest C6 rate in the sweep is positive, so the
	// floor is unreached and C6ROS's not reproducing it changes no number here.
	if minROS <= 0 {
		t.Errorf("a C6 case reports ROS %v — at or below zero, where cffdrs substitutes 1e-6 "+
			"and C6ROS does not. The floor is now reachable and C6ROS's doc is wrong about it.", minROS)
	}
	t.Logf("smallest C6 ros in the sweep: %.6g (positive, so cffdrs' 1e-6 floor never fires)", minROS)
}
