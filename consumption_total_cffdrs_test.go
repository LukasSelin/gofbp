package fbp

import (
	"sort"
	"testing"
)

// Crown and total fuel consumption against the oracle.
//
// Oracle case (b): cfc and tfc are in fbp()'s output contract but were not
// emitted, so this port changed gen_cffdrs_reference.R and moved the fixture
// digest. What made the change more than two extra columns is the CFL problem.
//
// # Why the generator grew a block rather than just two columns
//
// Eq. 66a is CFL·CFB, and every crown-block row in the fixture sends CFL = 1.0.
// A sweep at one value of a multiplicative factor cannot tell CFL·CFB from CFB:
// an implementation that dropped the factor entirely would have agreed with the
// oracle across all 10752 of those rows. That is the fmc trap in /migration-port
// exactly — the column existing is not the claim that matters — so the generator
// grew a 512-row block that sweeps CFL over four values in (0, 2] and PC and PDF
// across the two mixedwood weightings.
//
// The (0, 2] bound is not decoration either. crown_fuel_load substitutes the
// per-fuel table whenever CFL is non-positive, above 2, or NA, and the fixture
// records what was SENT — so a row outside the range would be an assertion about
// a number fbp() never used. usableForConsumption is what holds the line.
//
// # What the surface blocks can still say
//
// Most of the fixture sends the CFL sentinel, and those rows cannot be read as
// assertions about CFL. They are not useless: wherever the oracle's own cfb is
// 0, CFC is 0 whatever CFL was, so tfc must equal sfc exactly. That is the
// fourth pass below, it needs no knowledge of the per-fuel table, and it is
// deliberately not a back-door recovery of one — the CBH/CFL tables are a
// separate ledger row whose exposure is an open design decision.
//
// ledger: total_fuel_consumption.r
func TestCFFDRSTotalFuelConsumption(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9

	perFuel := map[string]int{}
	cflSeen := map[float64]int{}
	var worstCFC, worstTFC, worstOurSFC, worstEndToEnd float64
	nDirect, badDirect := 0, 0
	nOurSFC, badOurSFC := 0, 0
	nEndToEnd, badEndToEnd := 0, 0
	weighted, crowning := 0, 0

	for i, c := range f.Cases {
		if !c.usableForConsumption() {
			continue
		}
		fuel := ourFuel(c.Fuel)
		nDirect++
		perFuel[fuel]++
		cflSeen[c.CFL]++
		if c.CFB > 0 {
			crowning++
		}
		switch fuel {
		case "M1", "M2", "M3", "M4":
			weighted++
		}

		// Pass one: eqs. 66 and 67 alone. CFB and SFC are the oracle's own, so a
		// failure here is the two multiplications and the addition, and nothing
		// upstream of them.
		cfc := CrownFuelConsumption(fuel, c.CFL, c.CFB, c.PC, c.PDF)
		tfc := TotalFuelConsumption(fuel, c.CFL, c.CFB, c.SFC, c.PC, c.PDF)
		if rel := relErr(cfc, c.CFC); rel > worstCFC {
			worstCFC = rel
		}
		if rel := relErr(tfc, c.TFC); rel > worstTFC {
			worstTFC = rel
		}
		if !closeEnough(cfc, c.CFC, tol) || !closeEnough(tfc, c.TFC, tol) {
			if badDirect++; badDirect <= 10 {
				t.Errorf("case %d %s cfl=%v cfb=%v pc=%v pdf=%v sfc=%v: CFC = %v / TFC = %v, cffdrs = %v / %v",
					i, c.Fuel, c.CFL, c.CFB, c.PC, c.PDF, c.SFC, cfc, tfc, c.CFC, c.TFC)
			}
		}

		// Pass two: the same, with SFC computed here instead of read. This is
		// where eq. 67 stops being an addition of two fixture columns and starts
		// composing the two ported halves.
		nOurSFC++
		ourSFC := SurfaceFuelConsumption(fuel, c.FFMC, c.BUI, c.PC, sweepGrassFuelLoad)
		tfcOurSFC := TotalFuelConsumption(fuel, c.CFL, c.CFB, ourSFC, c.PC, c.PDF)
		if rel := relErr(tfcOurSFC, c.TFC); rel > worstOurSFC {
			worstOurSFC = rel
		}
		if !closeEnough(tfcOurSFC, c.TFC, tol) {
			if badOurSFC++; badOurSFC <= 10 {
				t.Errorf("case %d %s ffmc=%v bui=%v: TFC from our SFC = %v, cffdrs = %v (our SFC %v vs %v)",
					i, c.Fuel, c.FFMC, c.BUI, tfcOurSFC, c.TFC, ourSFC, c.SFC)
			}
		}

		// Pass three: end to end from FFMC and wind on flat rows — our SFC, our
		// surface rate, our CFB, then eqs. 66 and 67 on top.
		//
		// FMC is taken from the fixture rather than computed, and the reason is
		// D1. fbp() zeroes FMC for D1/S1/S2/S3/O1A/O1B AFTER computing it and
		// BEFORE the crown chain reads it, so a D1 row's threshold was built on
		// FMC = 0 and recomputing the real value here would disagree with the
		// oracle for a reason that has nothing to do with eqs. 66 or 67.
		// TestCFFDRSFoliarMoistureContent is what asserts that column.
		if c.GS != 0 {
			continue
		}
		nEndToEnd++
		crown := Crown{FMC: c.FMC, SFC: ourSFC, CBH: c.CBH, CFL: c.CFL}
		var ourCFB float64
		if fuel == "C6" {
			// C6's CFB comes off the C6 path, gated on RSC > RSS — see c6.go.
			wsv, _ := NetEffectiveWind(SlopeWind{
				Code: fuel, FFMC: c.FFMC, SlopePct: c.GS, WindKmh: c.WS,
				WindAzimuthDeg:    c.WD + 180,
				UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect, made explicit
				PC:                c.PC, PDF: c.PDF, CuringPct: c.CC,
			})
			ourISI := ISI(c.FFMC, wsv)
			crown.SurfaceROS = RSI(fuel, ourISI, c.PC, c.PDF, c.CC) * BuildupEffect(fuel, c.BUI)
			_, ourCFB = C6CrownFire(crown, ourISI)
		} else {
			rss, _ := slopeAdjustedROS(SlopeWind{
				Code: fuel, FFMC: c.FFMC, SlopePct: c.GS, WindKmh: c.WS,
				WindAzimuthDeg:    c.WD + 180,
				UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect, made explicit
				PC:                c.PC, PDF: c.PDF, CuringPct: c.CC,
			}, c.BUI)
			crown.SurfaceROS = rss
			ourCFB = CrownFractionBurned(crown)
		}
		e2e := TotalFuelConsumption(fuel, c.CFL, ourCFB, ourSFC, c.PC, c.PDF)
		if rel := relErr(e2e, c.TFC); rel > worstEndToEnd {
			worstEndToEnd = rel
		}
		if !closeEnough(e2e, c.TFC, tol) {
			if badEndToEnd++; badEndToEnd <= 10 {
				t.Errorf("case %d %s ffmc=%v ws=%v cbh=%v cfl=%v: end-to-end TFC = %v, cffdrs = %v (our CFB %v vs %v)",
					i, c.Fuel, c.FFMC, c.WS, c.CBH, c.CFL, e2e, c.TFC, ourCFB, c.CFB)
			}
		}
	}
	if badDirect > 10 {
		t.Errorf("... and %d more eq. 66/67 mismatches", badDirect-10)
	}
	if badOurSFC > 10 {
		t.Errorf("... and %d more mismatches with our own SFC", badOurSFC-10)
	}
	if badEndToEnd > 10 {
		t.Errorf("... and %d more end-to-end mismatches", badEndToEnd-10)
	}

	// Pass four: every case whose crown fraction burned is zero, including the
	// thousands that sent the CFL sentinel. CFC is CFL·CFB weighted, so a zero
	// CFB puts it at zero regardless of the CFL fbp() substituted — which makes
	// this the one assertion the surface blocks can carry, and it carries it
	// without knowing a single entry of the table.
	nZero, badZero := 0, 0
	for i, c := range f.Cases {
		if c.CFB != 0 {
			continue
		}
		nZero++
		// The CFL here is deliberately the one the fixture recorded, sentinel and
		// all: with CFB = 0 the product is 0 for any finite CFL, and that is the
		// property being asserted.
		got := TotalFuelConsumption(ourFuel(c.Fuel), c.CFL, c.CFB, c.SFC, c.PC, c.PDF)
		if !closeEnough(c.CFC, 0, tol) || !closeEnough(got, c.TFC, tol) {
			if badZero++; badZero <= 10 {
				t.Errorf("case %d %s cfb=0: cffdrs cfc = %v (want 0), our TFC = %v, cffdrs tfc = %v, sfc = %v",
					i, c.Fuel, c.CFC, got, c.TFC, c.SFC)
			}
		}
	}
	if badZero > 10 {
		t.Errorf("... and %d more zero-CFB mismatches", badZero-10)
	}

	t.Logf("TFC  %5d cases with a CFL fbp() honoured verbatim: eqs. 66/67 from the oracle's CFB and SFC, "+
		"%d mismatched, worst relative error CFC %.3g TFC %.3g", nDirect, badDirect, worstCFC, worstTFC)
	t.Logf("TFC  %5d of those with SFC computed here, %d mismatched, worst relative error %.3g",
		nOurSFC, badOurSFC, worstOurSFC)
	t.Logf("TFC  %5d flat ones end to end from FFMC and wind, %d mismatched, worst relative error %.3g",
		nEndToEnd, badEndToEnd, worstEndToEnd)
	t.Logf("TFC  %5d cases with CFB = 0 (the surface blocks included): cfc = 0 and tfc = sfc, %d mismatched",
		nZero, badZero)

	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, k := range fuels {
		t.Logf("  %-4s %5d cases", k, perFuel[k])
	}
	cfls := make([]float64, 0, len(cflSeen))
	for k := range cflSeen {
		cfls = append(cfls, k)
	}
	sort.Float64s(cfls)
	t.Logf("CFL values honoured verbatim: %v", cfls)

	// Anti-no-op floors. Each one names the thing that would silently stop being
	// asserted if the generator's block were narrowed.
	if nDirect < 5000 {
		t.Fatalf("only %d cases carry a usable CFL — eqs. 66/67 are effectively unasserted", nDirect)
	}
	if nEndToEnd < 200 {
		t.Fatalf("only %d flat usable cases — nothing here asserts the chain end to end", nEndToEnd)
	}
	if nZero < 1000 {
		t.Fatalf("only %d cases with CFB = 0 — the tfc = sfc identity is effectively unasserted", nZero)
	}
	if crowning == 0 {
		t.Fatal("no crowning cases with a usable CFL — CFC is zero throughout and eq. 66a is unasserted")
	}
	if weighted == 0 {
		t.Fatal("no M1/M2/M3/M4 cases with a usable CFL — eqs. 66b and 66c are unasserted")
	}
	// The one the whole generator block exists for. Two distinct CFL values is
	// the minimum that can tell CFL·CFB from CFB; the block sends four, on top of
	// the crown block's 1.0.
	if len(cfls) < 2 {
		t.Fatalf("only %d distinct CFL value(s) honoured verbatim — an implementation ignoring "+
			"the CFL factor of eq. 66a would pass this test. The generator's CFC block has been "+
			"narrowed; widen it back rather than deleting this check.", len(cfls))
	}
}
