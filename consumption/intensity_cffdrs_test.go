package consumption

import (
	"sort"
	"testing"

	"github.com/LukasSelin/gofbp/crown"
	"github.com/LukasSelin/gofbp/internal/cffdrstest"
	"github.com/LukasSelin/gofbp/spread"
)

// Head fire intensity against the oracle.
//
// Oracle case (b): HFI is in fbp()'s output contract and was not emitted, so
// this port added the hfi column to gen_cffdrs_reference.R and moved the fixture
// digest — and nothing else. Unlike cfc/tfc it needed no new block: eq. 69 has
// no coefficient of its own to under-sweep, and its two factors are already
// swept as widely as the fixture sweeps anything (TFC across four CFLs and both
// mixedwood weightings, ROS across every fuel, FFMC, wind and slope).
//
// The passes narrow what a failure can mean, the same way the TFC test's do:
//
//  1. eq. 69 alone, from the oracle's own tfc and ros, over every case. This is
//     the only pass that reaches sloped rows and the sentinel-CFL rows, because
//     it reads both factors rather than computing them.
//  2. end to end on flat rows whose CFL fbp() honoured verbatim: our SFC, our
//     surface or C6-blended rate, our CFB, our TFC, then eq. 69.
//  3. end to end on flat rows the oracle says do not crown: our SFC and our
//     surface rate alone. CFB = 0 makes the CFL fbp() substituted irrelevant,
//     so this is the surface blocks' share, and it reads no table.
//
// Pass 3 takes CFB = 0 from the oracle rather than computing it, because the
// sentinel rows sent no CBH this package could compute it from. It is therefore
// an assertion about eqs. 69, 67 and the surface rate, not about eq. 58.
//
// ledger: fire_intensity.r
func TestCFFDRSHeadFireIntensity(t *testing.T) {
	f := cffdrstest.Load(t)
	const tol = 1e-9

	// A fixture older than this port has no hfi key, which decodes as 0 on every
	// row and would make pass 1 compare 300·tfc·ros against zero thousands of
	// times. Say what is actually wrong instead.
	emitted := false
	for _, c := range f.Cases {
		if c.HFI != 0 {
			emitted = true
			break
		}
	}
	if !emitted {
		t.Fatal("the fixture has no hfi column: it predates the fire_intensity.r port " +
			"(testdata/README.md lists it among the earlier digests). Regenerate it.")
	}

	perFuel := map[string]int{}
	var worstDirect, worstE2E, worstSurface float64
	nDirect, badDirect := 0, 0
	nE2E, badE2E, crowningE2E, c6E2E := 0, 0, 0, 0
	nSurface, badSurface := 0, 0

	for i, c := range f.Cases {
		fuel := cffdrstest.Fuel(c.Fuel)

		// Pass one.
		nDirect++
		perFuel[fuel]++
		got := FireIntensity(c.TFC, c.ROS)
		if rel := cffdrstest.RelErr(got, c.HFI); rel > worstDirect {
			worstDirect = rel
		}
		if !cffdrstest.CloseEnough(got, c.HFI, tol) {
			if badDirect++; badDirect <= 10 {
				t.Errorf("case %d %s tfc=%v ros=%v: HFI = %v, cffdrs = %v", i, c.Fuel, c.TFC, c.ROS, got, c.HFI)
			}
		}

		if c.GS != 0 {
			continue
		}
		ourSFC := SurfaceFuelConsumption(fuel, c.FFMC, c.BUI, c.PC, cffdrstest.SweepGrassFuelLoad)
		rss := ourFlatSurfaceROS(c, fuel)

		// Pass three.
		if c.CFB == 0 {
			nSurface++
			hfi := FireIntensity(TotalFuelConsumption(fuel, c.CFL, 0, ourSFC, c.PC, c.PDF), rss)
			if rel := cffdrstest.RelErr(hfi, c.HFI); rel > worstSurface {
				worstSurface = rel
			}
			if !cffdrstest.CloseEnough(hfi, c.HFI, tol) {
				if badSurface++; badSurface <= 10 {
					t.Errorf("case %d %s ffmc=%v ws=%v bui=%v cfb=0: surface HFI = %v, cffdrs = %v (our SFC %v vs %v, our ROS %v vs %v)",
						i, c.Fuel, c.FFMC, c.WS, c.BUI, hfi, c.HFI, ourSFC, c.SFC, rss, c.ROS)
				}
			}
		}

		// Pass two.
		if !c.UsableForConsumption() {
			continue
		}
		nE2E++
		stand := crown.Crown{FMC: c.FMC, SFC: ourSFC, CBH: c.CBH, CFL: c.CFL, SurfaceROS: rss}
		ros, cfb := rss, 0.0
		if fuel == "C6" {
			c6E2E++
			ros, cfb = crown.C6CrownFire(stand, spread.ISI(c.FFMC, c.WS))
		} else {
			cfb = crown.CrownFractionBurned(stand)
		}
		if cfb > 0 {
			crowningE2E++
		}
		hfi := FireIntensity(TotalFuelConsumption(fuel, c.CFL, cfb, ourSFC, c.PC, c.PDF), ros)
		if rel := cffdrstest.RelErr(hfi, c.HFI); rel > worstE2E {
			worstE2E = rel
		}
		if !cffdrstest.CloseEnough(hfi, c.HFI, tol) {
			if badE2E++; badE2E <= 10 {
				t.Errorf("case %d %s ffmc=%v ws=%v cbh=%v cfl=%v: end-to-end HFI = %v, cffdrs = %v (our ROS %v vs %v, our CFB %v vs %v)",
					i, c.Fuel, c.FFMC, c.WS, c.CBH, c.CFL, hfi, c.HFI, ros, c.ROS, cfb, c.CFB)
			}
		}
	}
	for _, more := range []struct {
		n    int
		what string
	}{{badDirect, "eq. 69"}, {badE2E, "end-to-end"}, {badSurface, "surface-only"}} {
		if more.n > 10 {
			t.Errorf("... and %d more %s mismatches", more.n-10, more.what)
		}
	}

	t.Logf("HFI  %5d cases, eq. 69 from the oracle's tfc and ros: %d mismatched, worst relative error %.3g",
		nDirect, badDirect, worstDirect)
	t.Logf("HFI  %5d flat cases with a CFL honoured verbatim, end to end from FFMC and wind "+
		"(%d crowning, %d C6): %d mismatched, worst relative error %.3g",
		nE2E, crowningE2E, c6E2E, badE2E, worstE2E)
	t.Logf("HFI  %5d flat cases with CFB = 0, from our SFC and surface rate alone: %d mismatched, worst relative error %.3g",
		nSurface, badSurface, worstSurface)
	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, k := range fuels {
		t.Logf("  %-4s %5d cases", k, perFuel[k])
	}

	// Anti-no-op floors, as in the TFC test.
	if len(perFuel) < 17 {
		t.Fatalf("only %d fuel types reached — HFI is unasserted for the rest", len(perFuel))
	}
	if nE2E < 200 {
		t.Fatalf("only %d flat usable cases — nothing asserts HFI end to end", nE2E)
	}
	if crowningE2E == 0 {
		t.Fatal("no crowning end-to-end cases — the crown half of HFI's fuel consumption is unasserted")
	}
	if c6E2E == 0 {
		t.Fatal("no C6 end-to-end cases — HFI on the crown-blended rate is unasserted")
	}
	if nSurface < 1000 {
		t.Fatalf("only %d flat CFB = 0 cases — the surface-only HFI is effectively unasserted", nSurface)
	}
}

// ourFlatSurfaceROS is the surface head rate RSS this package computes for a
// flat fixture row: the slope-and-wind path for sixteen fuels, and RSI·BE for
// C6, whose sloped path is the gap recorded on MIGRATION.md's Slopecalc.r row.
// On flat ground the two agree; C6 takes the direct form only so that this
// helper does not lean on the path the ledger says is incomplete.
func ourFlatSurfaceROS(c cffdrstest.Case, fuel string) float64 {
	if fuel == "C6" {
		return spread.RSI(fuel, spread.ISI(c.FFMC, c.WS), c.PC, c.PDF, c.CC) * spread.BuildupEffect(fuel, c.BUI)
	}
	rss, _ := slopeAdjustedROS(spread.SlopeWind{
		Code: fuel, FFMC: c.FFMC, SlopePct: c.GS, WindKmh: c.WS,
		WindAzimuthDeg:    c.WD + 180,
		UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect, made explicit
		PC:                c.PC, PDF: c.PDF, CuringPct: c.CC,
	}, c.BUI)
	return rss
}
