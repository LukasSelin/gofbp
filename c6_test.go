package fbp

import (
	"math"
	"testing"
)

// The unconditional half of c6.go's coverage: identities, bounds, monotonicity
// and the branches the oracle sweep cannot reach. These run on a fresh clone,
// where TestCFFDRSC6RateOfSpread skips for want of a fixture.

func TestFMEAvgIsTheFMEOfARealFoliarMoisture(t *testing.T) {
	// FMEAvg is eq. 64's normaliser, so RSC is "the fitted rate scaled by how
	// this stand compares to the fitted one". That reading is only true if 0.778
	// is an FME the equation can actually produce at a plausible moisture — if it
	// were an arbitrary constant, RSC would be a scaled curve and nothing more.
	// Bisect for the moisture, and check it lands where foliage lives.
	lo, hi := 0.0, FMETurningPointFMC
	for range 200 {
		mid := (lo + hi) / 2
		if FoliarMoistureEffect(mid) > FMEAvg {
			lo = mid
		} else {
			hi = mid
		}
	}
	const want = 97.017271
	if math.Abs(lo-want) > 1e-4 {
		t.Errorf("FME = FMEAvg at FMC %.6f, expected %.6f — the 0.778 or eq. 61 has moved", lo, want)
	}
	// Eqs. 1-8 produce FMC in [85, 120]; a normaliser outside that would not be
	// the average stand it claims to be.
	if lo < 85 || lo > 120 {
		t.Errorf("FMEAvg corresponds to FMC %.2f, outside the [85, 120] range eqs. 1-8 produce", lo)
	}
	if got := FoliarMoistureEffect(lo); !closeEnough(got, FMEAvg, 1e-9) {
		t.Errorf("FoliarMoistureEffect(%.6f) = %v, want %v", lo, got, FMEAvg)
	}
}

// FMETurningPointFMC is where eq. 61's numerator reaches zero: 1.5/0.00275. Not
// exported from the package because it is a property of the equation rather than
// a published constant, but the tests below need it in two places.
const FMETurningPointFMC = 1.5 / 0.00275

func TestFoliarMoistureEffectTurningPoint(t *testing.T) {
	// Eq. 61's fourth power is even, so FME bottoms out at 545.45 % and rises
	// again. c6.go documents that rather than clamping it; this is what makes the
	// documentation checkable, and what would fail if someone "fixed" it by
	// clamping without saying so.
	if got := FoliarMoistureEffect(FMETurningPointFMC); got != 0 {
		t.Errorf("FME at the turning point FMC %.6f = %v, want exactly 0", FMETurningPointFMC, got)
	}
	below := FoliarMoistureEffect(FMETurningPointFMC - 10)
	above := FoliarMoistureEffect(FMETurningPointFMC + 10)
	if below <= 0 || above <= 0 {
		t.Fatalf("FME should be positive on both sides of its root, got %v below and %v above", below, above)
	}
	// Strictly decreasing below the turning point, strictly increasing above it.
	prev := math.Inf(1)
	for fmc := 0.0; fmc < FMETurningPointFMC; fmc += 5 {
		got := FoliarMoistureEffect(fmc)
		if got >= prev {
			t.Fatalf("FME is not decreasing at FMC %v: %v after %v", fmc, got, prev)
		}
		prev = got
	}
	prev = 0
	for fmc := FMETurningPointFMC + 1; fmc < 900; fmc += 5 {
		got := FoliarMoistureEffect(fmc)
		if got <= prev {
			t.Fatalf("FME is not increasing above the turning point at FMC %v: %v after %v", fmc, got, prev)
		}
		prev = got
	}
}

func TestFoliarMoistureEffectScreensImpossibleInput(t *testing.T) {
	// A negative FMC is the dangerous one: the even power keeps the numerator
	// positive while the denominator heads for zero at −17.76, so without the
	// screen eq. 61 returns a LARGE FME — a fast crown fire — for input that
	// cannot exist. Check across the pole, not just near zero.
	for _, fmc := range []float64{-0.001, -1, -17.76, -17.760617760617763, -20, -1000,
		math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := FoliarMoistureEffect(fmc); got != 0 {
			t.Errorf("FoliarMoistureEffect(%v) = %v, want 0", fmc, got)
		}
	}
	if got := FoliarMoistureEffect(0); !(got > 0 && !math.IsInf(got, 0)) {
		t.Errorf("FoliarMoistureEffect(0) = %v, want a finite positive value", got)
	}
}

func TestC6CrownROSSaturatesAtSixtyTimesTheMoistureRatio(t *testing.T) {
	// Eq. 64's 60 is a saturation: RSC -> 60·FME/FMEAvg as ISI grows without
	// bound. That is the statement that C-6's crown rate has a ceiling per
	// foliar moisture, and it is worth an assertion because reading the 60 as a
	// multiplier instead would make RSC unbounded in ISI.
	for _, fmc := range []float64{85, 100, 120} {
		ceiling := 60 * FoliarMoistureEffect(fmc) / FMEAvg
		prev := 0.0
		for _, isi := range []float64{0.1, 1, 5, 20, 100, 1000, 1e6} {
			got := C6CrownROS(isi, fmc)
			if got > ceiling {
				t.Errorf("RSC(isi=%v, fmc=%v) = %v, above the 60·FME/FMEAvg ceiling %v", isi, fmc, got, ceiling)
			}
			// Non-decreasing rather than strictly increasing, and the reason is
			// the saturation itself: exp(-0.0497·ISI) underflows the double's
			// 2^-53 resolution around ISI 720, so RSC is EXACTLY the ceiling from
			// there up. Demanding strict growth past that point would be
			// demanding the ceiling not exist.
			if got < prev {
				t.Errorf("RSC decreases in ISI at isi=%v fmc=%v: %v after %v", isi, fmc, got, prev)
			}
			if isi <= 100 && got <= prev {
				t.Errorf("RSC is not increasing in ISI at isi=%v fmc=%v: %v after %v", isi, fmc, got, prev)
			}
			prev = got
		}
		// Reached exactly, not approached: the ceiling is the value, so a wrong
		// 60 or a wrong FMEAvg shows up here as a ratio rather than a near-miss.
		if got := C6CrownROS(1e6, fmc); got != ceiling {
			t.Errorf("RSC at fmc=%v is %v, want its ceiling %v exactly", fmc, got, ceiling)
		}
	}
	// And RSC falls with foliar moisture, over the range eqs. 1-8 produce.
	prev := math.Inf(1)
	for fmc := 85.0; fmc <= 120; fmc++ {
		got := C6CrownROS(10, fmc)
		if got >= prev {
			t.Fatalf("RSC is not decreasing in FMC at %v: %v after %v", fmc, got, prev)
		}
		prev = got
	}
}

func TestC6CrownROSIsZeroAtZeroISI(t *testing.T) {
	// The guard has to be exact, not merely safe: at ISI 0 eq. 64's own value is
	// 0, so returning 0 is the equation rather than a substitute for it. If that
	// stops being true the guard has started hiding a discontinuity.
	for _, fmc := range []float64{0, 85, 120} {
		if got := C6CrownROS(0, fmc); got != 0 {
			t.Errorf("C6CrownROS(0, %v) = %v, want exactly 0", fmc, got)
		}
		// Approaching from above, with nothing jumping over the guard.
		if got := C6CrownROS(1e-12, fmc); got < 0 || got > 1e-9 {
			t.Errorf("C6CrownROS(1e-12, %v) = %v, expected a value just above 0", fmc, got)
		}
	}
	for _, isi := range []float64{-1e-12, -1, -1e9, math.NaN()} {
		if got := C6CrownROS(isi, 100); got != 0 {
			t.Errorf("C6CrownROS(%v, 100) = %v, want 0", isi, got)
		}
	}
}

func TestC6ROSInterpolatesBetweenTheTwoRates(t *testing.T) {
	// Eq. 65 is a linear interpolation with CFB as the weight, which is the claim
	// a caller reads the number against: the reported rate is never below the
	// surface rate and never above the crown rate.
	const rss, rsc = 4.0, 30.0
	if got := C6ROS(rss, rsc, 0); got != rss {
		t.Errorf("at CFB 0 the blend should be the surface rate: got %v, want %v", got, rss)
	}
	if got := C6ROS(rss, rsc, 1); !closeEnough(got, rsc, 1e-12) {
		t.Errorf("at CFB 1 the blend should be the crown rate: got %v, want %v", got, rsc)
	}
	prev := rss - 1
	for cfb := 0.0; cfb <= 1.0; cfb += 0.01 {
		got := C6ROS(rss, rsc, cfb)
		if got < rss || got > rsc {
			t.Fatalf("C6ROS(%v, %v, %v) = %v, outside [RSS, RSC]", rss, rsc, cfb, got)
		}
		if got < prev {
			t.Fatalf("C6ROS is not increasing in CFB at %v: %v after %v", cfb, got, prev)
		}
		prev = got
	}
	// The continuous-crown boundary, as the doc describes it.
	if got := C6ROS(rss, rsc, ContinuousCrownCFB); got < rsc-0.1*(rsc-rss)-1e-12 {
		t.Errorf("at the continuous-crown boundary the blend should be within a tenth of the crown rate, got %v", got)
	}
}

func TestC6ROSFallsBackToTheSurfaceRate(t *testing.T) {
	// Eq. 65's else-branch, and the deviations c6.go documents. The oracle sweep
	// reaches none of this — see TestCFFDRSC6RateOfSpread — so it is asserted
	// here or nowhere.
	const rss = 12.0
	for _, rsc := range []float64{rss, rss - 1e-12, 0, -5, math.NaN()} {
		if got := C6ROS(rss, rsc, 0.5); got != rss {
			t.Errorf("C6ROS(%v, %v, 0.5) = %v, want the surface rate %v", rss, rsc, got, rss)
		}
	}
	// A negative CFB: the published arithmetic would report a rate below the
	// surface rate. There is no such fire.
	for _, cfb := range []float64{-1e-12, -0.5, -1, math.NaN()} {
		if got := C6ROS(rss, 30, cfb); got != rss {
			t.Errorf("C6ROS(%v, 30, %v) = %v, want the surface rate %v", rss, cfb, got, rss)
		}
	}
}

func TestC6CrownFractionBurnedIsGatedOnTheCrownRate(t *testing.T) {
	// C-6's CFB is eq. 58 on the SURFACE rate, exactly as for every other fuel,
	// with RSC only deciding whether it is reported. Both halves of that get an
	// assertion, because the natural misreading — that C-6's CFB is computed from
	// its crown rate — would pass a test of the gate alone.
	c := Crown{FMC: 100, SFC: 2.0, CBH: 3, CFL: 1.0, SurfaceROS: 8}
	plain := CrownFractionBurned(c)
	if !(plain > 0 && plain < 1) {
		t.Fatalf("test case is degenerate: CrownFractionBurned = %v, wanted something strictly inside (0, 1)", plain)
	}
	if got := C6CrownFractionBurned(c, c.SurfaceROS+1e-9); got != plain {
		t.Errorf("with RSC just above RSS, C6's CFB = %v, want eq. 58's own %v", got, plain)
	}
	// Raising RSC must not change CFB at all. If it does, CFB is being computed
	// from the crown rate somewhere.
	for _, rsc := range []float64{c.SurfaceROS * 2, c.SurfaceROS * 10, 1e6} {
		if got := C6CrownFractionBurned(c, rsc); got != plain {
			t.Errorf("C6's CFB moved to %v at RSC %v — it must not depend on the crown rate, only be gated by it",
				got, rsc)
		}
	}
	// The gate, from the other side.
	for _, rsc := range []float64{c.SurfaceROS, c.SurfaceROS - 1e-12, 0, -1, math.NaN()} {
		if got := C6CrownFractionBurned(c, rsc); got != 0 {
			t.Errorf("C6CrownFractionBurned with RSC %v = %v, want 0 — the gate should hold", rsc, got)
		}
	}
	// And CrownFractionBurned's own screens still apply through it. The CFL gate
	// is the load-bearing one: it is what keeps a fuel with no crown at zero.
	noCrown := c
	noCrown.CFL = 0
	if got := C6CrownFractionBurned(noCrown, 1e6); got != 0 {
		t.Errorf("C6CrownFractionBurned with CFL 0 = %v, want 0", got)
	}
	for _, bad := range []Crown{
		{FMC: math.NaN(), SFC: 2, CBH: 3, CFL: 1, SurfaceROS: 8},
		{FMC: 100, SFC: math.NaN(), CBH: 3, CFL: 1, SurfaceROS: 8},
		{FMC: 100, SFC: 2, CBH: math.NaN(), CFL: 1, SurfaceROS: 8},
		{FMC: 100, SFC: 2, CBH: 3, CFL: 1, SurfaceROS: math.NaN()},
	} {
		if got := C6CrownFractionBurned(bad, 1e6); got != 0 {
			t.Errorf("C6CrownFractionBurned(%+v) = %v, want 0 for non-finite input", bad, got)
		}
	}
}

func TestC6CrownGateIsUnreachableWithinTheFMCRange(t *testing.T) {
	// Why the oracle sweep never exercises the RSC <= RSS branch, stated as a
	// mechanism rather than as a count of rows.
	//
	// RSS has a supremum over ALL ISI and BUI: eq. 62 saturates at 30 and eq. 63
	// multiplies by BE, which for C-6 (q = 0.8, BUI0 = 62) rises to
	// exp(50·ln(0.8)·(−1/62)) as BUI grows. RSC has one too, 60·FME/FMEAvg. So
	// whether the gate can ever fail is decided by FMC alone, and the crossover
	// is above the 120 eq. 8 caps foliar moisture at.
	rssSup := 30 * math.Exp(50*math.Log(0.8)*(-1.0/62))
	if got := 30 * BuildupEffect("C6", 1e12); got > rssSup+1e-9 {
		t.Errorf("RSS at BUI 1e12 is %v, above the supremum %v this test is built on", got, rssSup)
	}
	lo, hi := 0.0, FMETurningPointFMC
	for range 200 {
		mid := (lo + hi) / 2
		if 60*FoliarMoistureEffect(mid)/FMEAvg > rssSup {
			lo = mid
		} else {
			hi = mid
		}
	}
	const want = 127.261662
	if math.Abs(lo-want) > 1e-4 {
		t.Errorf("RSC's supremum falls below RSS's at FMC %.6f, expected %.6f", lo, want)
	}
	if lo <= MaxFoliarMoisturePct {
		t.Errorf("the crossover is at FMC %.2f, at or below the %v that eqs. 1-8 cap foliar moisture at — "+
			"the RSC <= RSS gate IS reachable from the model's own FMC and the oracle sweep should "+
			"be reaching it. Check TestCFFDRSC6RateOfSpread's count before trusting either.",
			lo, MaxFoliarMoisturePct)
	}
	// Spot-check the conclusion the bound is there to support, at the worst
	// combination the model allows: highest FMC, highest BE, every ISI.
	for _, isi := range []float64{0.5, 1, 3, 10, 30, 100, 1000} {
		rsc := C6CrownROS(isi, MaxFoliarMoisturePct)
		rss := RSI("C6", isi, 0, 0, 0) * BuildupEffect("C6", 1e12)
		if !(rsc > rss) {
			t.Errorf("at FMC %v, BUI -> inf, ISI %v: RSC %v <= RSS %v — the gate is reachable after all",
				MaxFoliarMoisturePct, isi, rsc, rss)
		}
	}
}

func TestC6CrownFireComposesTheSamePieces(t *testing.T) {
	// C6CrownFire is a composition and must stay one: if it ever grows a default
	// or reorders the chain, this is what says so.
	for _, c := range []Crown{
		{FMC: 100, SFC: 2.0, CBH: 3, CFL: 1.0, SurfaceROS: 8},
		{FMC: 85, SFC: 1.0, CBH: 20, CFL: 0.8, SurfaceROS: 1},
		{FMC: 120, SFC: 5.0, CBH: 2, CFL: 1.0, SurfaceROS: 25},
		{FMC: 100, SFC: 2.0, CBH: 7, CFL: 0, SurfaceROS: 8}, // no crown to burn
		{FMC: 100, SFC: 0, CBH: 7, CFL: 1.0, SurfaceROS: 8}, // no surface fuel
	} {
		for _, isi := range []float64{0, 1, 10, 50} {
			rsc := C6CrownROS(isi, c.FMC)
			wantCFB := C6CrownFractionBurned(c, rsc)
			wantROS := C6ROS(c.SurfaceROS, rsc, wantCFB)
			gotROS, gotCFB := C6CrownFire(c, isi)
			if gotROS != wantROS || gotCFB != wantCFB {
				t.Errorf("C6CrownFire(%+v, %v) = (%v, %v), want (%v, %v)",
					c, isi, gotROS, gotCFB, wantROS, wantCFB)
			}
			// The contract a caller relies on: the blend never reads below the
			// surface rate, whatever the crown path decides.
			if gotROS < c.SurfaceROS {
				t.Errorf("C6CrownFire(%+v, %v) reports %v, below the surface rate %v",
					c, isi, gotROS, c.SurfaceROS)
			}
		}
	}
}

func TestC6CrownFireNeedsNoDefaults(t *testing.T) {
	// The package refuses per-fuel CBH and CFL tables, so a zero-valued Crown has
	// to come back as a surface fire rather than as the alarming answer a
	// substituted table would produce. This is the same guarantee crown.go's CFL
	// gate carries, checked through the C-6 path.
	ros, cfb := C6CrownFire(Crown{SurfaceROS: 10}, 30)
	if cfb != 0 {
		t.Errorf("an empty Crown reports CFB %v; with no crown fuel load there is nothing to burn", cfb)
	}
	if ros != 10 {
		t.Errorf("an empty Crown reports ROS %v, want the surface rate 10", ros)
	}
}
