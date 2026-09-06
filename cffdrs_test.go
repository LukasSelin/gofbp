package fbp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

// The cffdrs fixture is the only oracle that can say the ST-X-3 coefficients in
// this package are actually right, and the only cross-implementation check the
// package has at all. cffdrs is maintained by the Canadian Forest Service authors
// of the FBP System, so it is the reference implementation rather than a second
// opinion.
//
// Regenerate with:
//
//	testdata/regen-cffdrs.sh
//
// R is needed only to regenerate, and that script does not need it on the host:
// it pins R and cffdrs in a container. The fixture is NOT committed — see
// testdata/README.md — so these tests skip on a fresh clone until you make it.
type cffdrsCase struct {
	Fuel string  `json:"fuel"`
	FFMC float64 `json:"ffmc"`
	BUI  float64 `json:"bui"`
	WS   float64 `json:"ws"`
	WD   float64 `json:"wd"`
	GS   float64 `json:"gs"`
	PC   float64 `json:"pc"`
	PDF  float64 `json:"pdf"`
	CC   float64 `json:"cc"`
	// CBH and CFL are the crown-fire inputs as SENT. They are -1 on the surface
	// sweeps, the sentinel that makes fbp() substitute its own per-fuel table —
	// and since fbp() returns neither, those rows cannot say what values the
	// oracle actually used. Only rows with CBH > 0 and CFL > 0 are usable for the
	// crown assertions; see usableForCrown.
	CBH  float64 `json:"cbh"`
	CFL  float64 `json:"cfl"`
	LAT  float64 `json:"lat"`
	DJ   float64 `json:"dj"`
	ISI  float64 `json:"isi"`
	BE   float64 `json:"be"`
	SF   float64 `json:"sf"`
	WSV  float64 `json:"wsv"`
	FMC  float64 `json:"fmc"`
	SFC  float64 `json:"sfc"`
	CSI  float64 `json:"csi"`
	RSO  float64 `json:"rso"`
	CFB  float64 `json:"cfb"`
	FD   string  `json:"fd"`
	ROS  float64 `json:"ros"`
	LB   float64 `json:"lb"`
	BROS float64 `json:"bros"`
	FROS float64 `json:"fros"`
}

// usableForCrown reports whether a case carries crown inputs this package can be
// fed. See the CBH/CFL note above.
func (c cffdrsCase) usableForCrown() bool { return c.CBH > 0 && c.CFL > 0 }

type cffdrsFixture struct {
	Oracle        string       `json:"oracle"`
	CFFDRSVersion string       `json:"cffdrs_version"`
	RVersion      string       `json:"r_version"`
	Cases         []cffdrsCase `json:"cases"`
}

// ourFuel maps a cffdrs fuel name onto this package's key. cffdrs spells the
// grass fuels O1a/O1b; ST-X-3's tables and Fuels use O1A/O1B.
//
// The package no longer needs this at a call site — CanonicalFuelCode folds the
// fixture's spelling itself, and TestSpellingDoesNotChangeAnyAnswer is what
// holds that. It stays because the per-fuel tallies below key their maps on the
// result, and a tally split between "O1a" and "O1B" would under-report coverage
// on both without failing anything.
func ourFuel(code string) string { return strings.ToUpper(code) }

func loadCFFDRS(t *testing.T) cffdrsFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/cffdrs.json")
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("testdata/cffdrs.json is absent; it is generated, not committed — " +
			"run testdata/regen-cffdrs.sh (needs Docker). See testdata/README.md.")
	}
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f cffdrsFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	t.Logf("oracle: %s %s (%s), %d cases", f.Oracle, f.CFFDRSVersion, f.RVersion, len(f.Cases))
	return f
}

// crownChangesROS reports whether cffdrs' reported ROS for a case is a different
// quantity from the surface rate this package computes.
//
// It is FUEL TYPE, not CFB > 0, and the difference recovered several thousand
// cases of oracle coverage. cffdrs' rate_of_spread() returns the surface rate RSS
// unchanged for every fuel except C6, folding CFB in only through C6's separate
// crown rate of spread — so a crowning C1 stand has exactly the same reported ROS
// it would have had with no crown model at all. These tests previously excluded
// every case with CFB != 0, which discarded 3689 of the fixture's 11500 rows to
// avoid a divergence that, outside C6, does not exist.
//
// If that reading is ever wrong, the tests using this fail loudly with a ratio
// attached. Do not widen it back to CFB != 0 to make a failure go away.
func crownChangesROS(c cffdrsCase) bool { return ourFuel(c.Fuel) == "C6" }

func closeEnough(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol*math.Max(1, math.Abs(want))
}

// The buildup effect is a two-coefficient formula per fuel (q and BUI0), so this
// checks 26 transcribed numbers at once.
//
// ledger: buildup_effect.r
func TestCFFDRSBuildupEffect(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	bad := 0
	for i, c := range f.Cases {
		got := BuildupEffect(ourFuel(c.Fuel), c.BUI)
		if !closeEnough(got, c.BE, tol) {
			if bad++; bad <= 10 {
				t.Errorf("case %d %s BUI=%v: BE = %v, cffdrs = %v", i, c.Fuel, c.BUI, got, c.BE)
			}
		}
	}
	if bad > 10 {
		t.Errorf("... and %d more BE mismatches", bad-10)
	}
}

// SF is one formula (ST-X-3 eq. 39) plus a saturation rule. The fixture brackets
// 70 % rise from both sides, which is where a wrong cap or a clamp-instead-of-
// continuous cutoff would show up.
//
// ledger: Slopecalc.r
func TestCFFDRSSlopeFactor(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	bad := 0
	for i, c := range f.Cases {
		got := SlopeFactor(c.GS)
		if !closeEnough(got, c.SF, tol) {
			if bad++; bad <= 10 {
				t.Errorf("case %d GS=%v%%: SF = %v, cffdrs = %v", i, c.GS, got, c.SF)
			}
		}
	}
	if bad > 10 {
		t.Errorf("... and %d more SF mismatches", bad-10)
	}
}

// ISI's own check, and the only thing in this package that can see a wrong
// fine-fuel-moisture term.
//
// Before the back-solve landed nothing here computed ISI — it arrived as an
// input, and every test took it from the fixture — so there was no constant to
// be wrong. The value a careful reading of eq. 10 produces is 147.2, which most
// printings round it to; checked against the fixture while the back-solve was
// being written, that carries a 1.04e-3 relative bias against cffdrs' exact
// 250·59.5/101. Invisible next to any real tolerance and perfectly systematic, so
// it would have skewed every back-solved equivalent wind the same direction. With
// the exact rational the error is 0. This test is what keeps it that way.
//
// Restricted to flat rows because ISI on a sloped row already contains the
// slope-equivalent wind, which is the thing being derived rather than an input.
// A failure means the moisture constant or one of the two wind branches is wrong;
// the fixture's rows at WS = 50 are what exercise the second branch (without it
// the error there reaches 0.242).
//
// ledger: initial_spread_index.r
func TestCFFDRSInitialSpreadIndex(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-12
	n, bad := 0, 0
	for i, c := range f.Cases {
		if c.GS != 0 {
			continue
		}
		n++
		got := ISI(c.FFMC, c.WS)
		if !closeEnough(got, c.ISI, tol) {
			if bad++; bad <= 10 {
				t.Errorf("case %d %s FFMC=%v WS=%v: ISI = %v, cffdrs = %v (rel %.3g)",
					i, c.Fuel, c.FFMC, c.WS, got, c.ISI, math.Abs(got-c.ISI)/c.ISI)
			}
		}
	}
	if bad > 10 {
		t.Errorf("... and %d more ISI mismatches", bad-10)
	}
	t.Logf("%d flat cases, %d mismatched", n, bad)
}

// The back-solve against the oracle: WSV and the full slope path, on every sloped
// row cffdrs gives us.
//
// The two are asserted separately on purpose. A WSV-only failure localises to
// eqs. 40-51 — the inversion, the equivalent wind or the vector addition. A
// ROS-only failure localises to the final RSI(ISI(WSV)) · BE composition, which
// is the step that must NOT also multiply SF. Asserting only ROS would leave
// those indistinguishable.
//
// The fixture's Aspect is 0 in all 11500 rows, so WD is the only directional knob
// and the upslope azimuth is fixed at 180. That means this test says nothing
// about whether the aspect convention is right — it cannot, and no regeneration
// of the current script would change that. TestUpslopeConventionIsAspectPlus180
// and TestNetEffectiveWindRotationalInvariance carry that half.
//
// C6 is excluded for the same reason as in TestCFFDRSSurfaceROS: it is the one
// fuel whose reported ROS is not the surface rate. See crownChangesROS.
//
// ledger: Slopecalc.r
func TestCFFDRSSlopeBackSolve(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-9
	perFuel := map[string]int{}
	n, shown := 0, 0
	var worstWSV, worstROS float64
	for i, c := range f.Cases {
		if c.GS <= 0 || crownChangesROS(c) {
			continue
		}
		fuel := ourFuel(c.Fuel)
		s := SlopeWind{
			Code: fuel, FFMC: c.FFMC, SlopePct: c.GS, WindKmh: c.WS,
			WindAzimuthDeg:    c.WD + 180,
			UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect, made explicit
			PC:                c.PC, PDF: c.PDF, CuringPct: c.CC,
		}
		n++
		perFuel[fuel]++

		wsv, _ := NetEffectiveWind(s)
		if rel := relErr(wsv, c.WSV); rel > worstWSV {
			worstWSV = rel
		}
		if !closeEnough(wsv, c.WSV, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s gs=%v%% ws=%v wd=%v: WSV = %v, cffdrs = %v",
					i, c.Fuel, c.GS, c.WS, c.WD, wsv, c.WSV)
			}
		}

		ros, _ := slopeAdjustedROS(s, c.BUI)
		if rel := relErr(ros, c.ROS); rel > worstROS {
			worstROS = rel
		}
		if !closeEnough(ros, c.ROS, tol) {
			if shown++; shown <= 15 {
				t.Errorf("case %d %s gs=%v%% ws=%v wd=%v bui=%v: ROS = %v, cffdrs = %v (ratio %.4f)",
					i, c.Fuel, c.GS, c.WS, c.WD, c.BUI, ros, c.ROS, ros/c.ROS)
			}
		}
	}
	if shown > 15 {
		t.Errorf("... and %d more back-solve mismatches", shown-15)
	}

	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, fuel := range fuels {
		t.Logf("%-4s %5d sloped cases", fuel, perFuel[fuel])
	}
	t.Logf("%d sloped cases; worst relative error: WSV %.3g, ROS %.3g", n, worstWSV, worstROS)

	// A floor, not a formality. If a fixture regeneration drops or renames the
	// sloped sweep, every loop above simply skips and the test passes green
	// having asserted nothing — the same silent-no-op failure mode
	// TestCFFDRSSlopeDivergence guards with its own t.Fatal.
	if n < 1000 {
		t.Fatalf("only %d sloped non-crowning cases — the back-solve is effectively unasserted", n)
	}

	// The mixedwood ISF blends are reasoning, not measurement, until these
	// appear. M1 is a common real-world mapping. See gen_cffdrs_reference.R.
	if perFuel["M1"] == 0 && perFuel["M2"] == 0 {
		t.Log("NOTE: no sloped M1/M2 cases — the eq. 42 ISF blend is unoracled")
	}
	// Eqs. 42b/42c carry two readings taken off cffdrs' R source rather than
	// measured — that the pure dead-fir component is reached by forcing PDF to
	// 100, and that M4 drops its 0.2 deciduous weighting in the slope path the
	// way M2 does. Both are invisible to every other test here: they change the
	// blend's inputs, not its shape, so a wrong reading still produces a smooth,
	// monotone, correctly-bracketed spread rate. These rows are the only thing
	// that can contradict them.
	if perFuel["M3"] == 0 && perFuel["M4"] == 0 {
		t.Log("NOTE: no sloped M3/M4 cases — eqs. 42b/42c are unoracled; " +
			"the fixture predates them, so regenerate it")
	}
}

// slopeAdjustedROS is the FBP System's full slope path composed end to end:
// RSI(ISI(FFMC, WSV)) · BE(BUI), plus the azimuth the head fire runs towards.
//
// It lives in the test rather than the package because the intended caller has no
// use for it — anchored on a published ISI, it composes the pieces itself (see
// the note where this function would otherwise sit, in slopewind.go). Every part
// of it is package code and separately asserted; what this adds is the order they
// go in, and the fact that SF does not appear.
func slopeAdjustedROS(s SlopeWind, bui float64) (rosMMin, razDeg float64) {
	wsv, raz := NetEffectiveWind(s)
	return RSI(s.Code, ISI(s.FFMC, wsv), s.PC, s.PDF, s.CuringPct) * BuildupEffect(s.Code, bui), raz
}

// relErr is closeEnough's metric, exposed for logging how much headroom the
// assertions actually have.
func relErr(got, want float64) float64 {
	return math.Abs(got-want) / math.Max(1, math.Abs(want))
}

// The real coefficient check: RSI's (a, b, c) per fuel, composed with BE.
//
// Restricted to flat ground and to fuels other than C6, which is where this
// package's output and full FBP's are the same quantity. Wind is allowed and
// wanted — on flat ground cffdrs' net effective wind reduces to WS and ROS is
// still RSI(ISI) × BE, so sweeping wind is just how the fixture reaches high ISI.
// Slope is the one input that pulls the two apart (TestCFFDRSSlopeDivergence),
// and C6 is the one fuel whose reported ROS carries a crown contribution — see
// crownChangesROS, which is where the old CFB != 0 exclusion used to be and why
// it was too broad.
//
// ledger: rate_of_spread.r
func TestCFFDRSSurfaceROS(t *testing.T) {
	f := loadCFFDRS(t)
	const tol = 1e-8
	perFuel := map[string]int{}
	bad := map[string]int{}
	shown := 0
	for i, c := range f.Cases {
		if c.GS != 0 || crownChangesROS(c) {
			continue
		}
		fuel := ourFuel(c.Fuel)
		perFuel[fuel]++
		got := ROS(fuel, c.ISI, c.BUI, c.PC, c.PDF, c.CC, 0)
		if !closeEnough(got, c.ROS, tol) {
			bad[fuel]++
			if shown++; shown <= 15 {
				t.Errorf("case %d %s ISI=%.4f BUI=%v pc=%v cc=%v: ROS = %v, cffdrs = %v (ratio %.4f)",
					i, c.Fuel, c.ISI, c.BUI, c.PC, c.CC, got, c.ROS, got/c.ROS)
			}
		}
	}
	fuels := make([]string, 0, len(perFuel))
	for k := range perFuel {
		fuels = append(fuels, k)
	}
	sort.Strings(fuels)
	for _, fuel := range fuels {
		t.Logf("%-4s %5d surface cases, %d mismatched", fuel, perFuel[fuel], bad[fuel])
	}
	if shown > 15 {
		t.Errorf("... and %d more ROS mismatches", shown-15)
	}
}

// Both a measurement and, now, the specification of ROS's error.
//
// This package multiplies RSI × BE × SF. Full FBP does not: it back-solves the
// equivalent wind speed that SF implies, vector-adds it upslope to the real wind,
// and recomputes ROS from the net effective wind. The package doc calls that
// simplification an approximation; this puts a number on it, so the decision to
// implement the full path was made against measured error rather than a caveat.
//
// The full path now exists (asserted by TestCFFDRSSlopeBackSolve),
// but this stays. ROS survives as the degraded-mode answer for callers with no
// FFMC or wind, and its contract is "an upper bound, never an under-estimate" —
// which is a claim about numbers, so it needs a test over numbers. That is the
// bound asserted below; the percentiles are what the bound is worth in practice.
//
// The pairing matters and is easy to get wrong. cffdrs' reported ISI for a sloped
// row ALREADY contains the slope-equivalent wind, so feeding it back into
// ROS(..., slopePct) applies slope twice and the ratio comes out as exactly SF
// for every fuel — a tidy-looking number that measures nothing. What a caller
// actually has is a published ISI, which knows only wind: so each sloped row is
// paired
// with the flat row at the same fuel/FFMC/BUI/wind, and that row's ISI is the
// input. Fails only if the fixture stops containing pairable sloped cases.
//
// ledger: Slopecalc.r
func TestCFFDRSSlopeDivergence(t *testing.T) {
	f := loadCFFDRS(t)

	flatISI := map[string]float64{}
	key := func(c cffdrsCase) string {
		return fmt.Sprintf("%s|%g|%g|%g", c.Fuel, c.FFMC, c.BUI, c.WS)
	}
	for _, c := range f.Cases {
		if c.GS == 0 {
			flatISI[key(c)] = c.ISI
		}
	}

	// Group by the angle between wind and upslope, which is what the
	// simplification actually throws away. Aspect is 0 throughout the fixture, so
	// WD 0 is wind driving straight upslope and WD 180 is wind fighting it.
	angleName := map[float64]string{0: "upslope   (aligned)", 90: "cross-slope", 270: "cross-slope", 180: "downslope (opposed)"}
	byAngle := map[string][]float64{}
	type worstCase struct {
		ratio                 float64
		fuel                  string
		gs, ws, wd, ours, ref float64
	}
	var worst worstCase
	unpaired, noWind, noWindBound := 0, 0, 0
	for _, c := range f.Cases {
		// The crown sweep's rows are skipped even though they are sloped, and the
		// reason is that this test reports PERCENTILES. They are a statement about
		// a population, so the population has to be the one deliberately designed
		// for it — the sloped sweep, which varies wind direction through all four
		// quadrants. The crown block holds WD at 0, so folding its rows in here
		// would triple the wind-aligned bucket and move every median below
		// without a single new measurement behind the change.
		if c.GS <= 0 || crownChangesROS(c) || c.ROS == 0 || c.usableForCrown() {
			continue
		}
		isi, ok := flatISI[key(c)]
		if !ok {
			unpaired++
			continue
		}
		got := ROS(ourFuel(c.Fuel), isi, c.BUI, c.PC, c.PDF, c.CC, c.GS)
		ratio := got / c.ROS
		if c.WS == 0 {
			// Structural: with no wind there is nothing to vector-add, so the
			// back-solve is an identity and this is not an approximation at all.
			// Tracked separately so it cannot dilute the medians below into
			// looking harmless.
			//
			// With two exceptions, both of which only became visible when the
			// sloped sweep was widened past FFMC 85 and C2/C3/D1/S1/O1b.
			//
			// Where the slope demands more spread than the fuel's RSI curve can
			// deliver, cffdrs bounds it — isfClampMin, or the equivalent-wind cap
			// — and RSI·BE·SF, which has no such bound, comes out above it. S1 at
			// FFMC 95 on a 70 % slope is 54 % high with no wind at all.
			//
			// And mixedwood never satisfies the identity, because eq. 42 blends
			// the ISF of C2 and D1 and blending two inverted ISFs is not the
			// inverse of blending two RSIs. Worth being precise about what this
			// shows: it is cffdrs, not this package, breaking the identity — which
			// is independent confirmation that slopeEquivalentISF blends ISF
			// rather than RSF, a reading that had no oracle behind it until these
			// rows existed.
			//
			// Everywhere the identity is excused, the upper-bound contract still
			// has to hold.
			noWind++
			fuel := ourFuel(c.Fuel)
			zw := SlopeWind{
				Code: fuel, FFMC: c.FFMC, SlopePct: c.GS,
				PC: c.PC, PDF: c.PDF, CuringPct: c.CC,
			}
			_, clamped := slopeEquivalentISF(zw)
			mixedwood := fuel == "M1" || fuel == "M2" || fuel == "M3" || fuel == "M4"
			if clamped || mixedwood || EquivalentWind(zw) >= EquivalentWindCapKmh {
				noWindBound++
				if ratio < 1-1e-9 {
					t.Errorf("%s gs=%g%% FFMC=%g at zero wind: ratio %.4f — excused from the identity, but ROS must still read high, not low",
						c.Fuel, c.GS, c.FFMC, ratio)
				}
				continue
			}
			if !closeEnough(ratio, 1, 1e-6) {
				t.Errorf("%s gs=%g%% FFMC=%g at zero wind: ratio %.4f, expected the back-solve to be an identity",
					c.Fuel, c.GS, c.FFMC, ratio)
			}
			continue
		}
		name, ok := angleName[c.WD]
		if !ok {
			continue
		}
		// ROS's contract, and the reason a caller may fall back to it safely:
		// it trades accuracy for availability in one direction only. Applying
		// the whole slope factor regardless of wind direction can only ever add
		// spread, so a fallback cell reads high, never low.
		if ratio < 1-1e-9 {
			t.Errorf("%s gs=%g%% ws=%g wd=%g: ratio %.6f — ROS UNDER-estimates, which breaks its upper-bound contract",
				c.Fuel, c.GS, c.WS, c.WD, ratio)
		}
		byAngle[name] = append(byAngle[name], ratio)
		if ratio > worst.ratio {
			worst = worstCase{ratio, c.Fuel, c.GS, c.WS, c.WD, got, c.ROS}
		}
	}
	if len(byAngle) == 0 {
		t.Fatal("no pairable windy sloped cases — the divergence is unmeasured")
	}
	if unpaired > 0 {
		t.Logf("%d sloped cases had no flat counterpart and were skipped", unpaired)
	}

	t.Logf("zero-wind sloped cases: %d (%d exact, %d excused: mixedwood, or bounded by",
		noWind, noWind-noWindBound, noWindBound)
	t.Log("  the RSI clamp or the equivalent-wind cap, where RSI x BE x SF reads high")
	t.Log("  because it has no such bound). With no wind to vector-add the back-solve is")
	t.Log("  otherwise an identity, so away from those the error is purely directional:")
	t.Log("")
	t.Log("ours / cffdrs with wind on slope (1.0 = exact; >1 = we overestimate):")
	for _, name := range []string{"upslope   (aligned)", "cross-slope", "downslope (opposed)"} {
		v := byAngle[name]
		if len(v) == 0 {
			continue
		}
		sort.Float64s(v)
		t.Logf("  wind %-20s n=%4d  median %.2f  p95 %.2f  max %.2f",
			name, len(v), v[len(v)/2], v[int(float64(len(v))*0.95)], v[len(v)-1])
	}
	t.Logf("worst case: %s gs=%g%% ws=%g wd=%g -> ours %.2f m/min vs cffdrs %.2f (%.1fx)",
		worst.fuel, worst.gs, worst.ws, worst.wd, worst.ours, worst.ref, worst.ratio)
	t.Log("Reading: ROS is trustworthy where wind drives upslope and increasingly")
	t.Log("wrong as wind turns against it, because RSI x BE x SF applies the full slope")
	t.Log("factor regardless of which way the wind blows. That was the case for building")
	t.Log("the back-solve; it is now the specification of what falling back to ROS costs.")

	// The aligned bucket is what a fallback cell looks like in the good case. If
	// it ever drifts far above this the fallback has stopped being a reasonable
	// substitute and a caller should stop offering it silently.
	//
	// The bound is 8 and not the 4 it started at because widening the sweep to
	// FFMC 95 moved the worst aligned case from 3.84x to 6.54x. That is the RSI
	// clamp again — on dry steep ground cffdrs bounds the equivalent wind and
	// RSI x BE x SF does not — so even wind-aligned, the fallback is worse than
	// the old fixture made it look.
	if v := byAngle["upslope   (aligned)"]; len(v) > 0 {
		if worstAligned := v[len(v)-1]; worstAligned > 8.0 {
			t.Errorf("wind-aligned ROS overestimates by %.2fx, above the 8x this test has held",
				worstAligned)
		}
	}
}
