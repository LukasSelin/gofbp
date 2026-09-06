package fbp

import (
	"math"
	"testing"
)

// allFuels is every code in the published table, for the sweeps below.
func allFuels() []string {
	codes := make([]string, 0, len(Fuels))
	for code := range Fuels {
		codes = append(codes, code)
	}
	return codes
}

// sameBearing compares two azimuths the way bearings must be compared: 0 and 360
// are the same direction, and atan2 routinely lands a hair either side of north.
func sameBearing(a, b, tolDeg float64) bool {
	return math.Abs(normalizeDeg(a-b+180)-180) <= tolDeg
}

// The convention test, and the one the cffdrs fixture structurally cannot
// provide: every row in it has Aspect 0.
//
// Both azimuths in SlopeWind are "pushes towards", so a caller holding a
// meteorological wd (wind FROM) and an aspect raster (the DOWNSLOPE aspect) must
// add 180 to each. This pins that arithmetic with cases worked out
// by hand rather than derived from the same formula being tested.
//
// A failure here is the dangerous kind. Getting one of the two offsets wrong does
// not produce a NaN or an obviously silly number — it produces a plausible spread
// rate that is wrong by up to a factor of 60, and it is wrong in a way that
// varies with terrain, so no single spot-check would reveal it.
func TestUpslopeConventionIsAspectPlus180(t *testing.T) {
	// Ground rising to the north: the downslope face points south, so
	// aspect_deg = 180 and the upslope azimuth is 0.
	const aspectDeg = 180.0
	const upslope = aspectDeg + 180 // 360 ≡ 0, north

	base := SlopeWind{
		Code: "C2", FFMC: 90, SlopePct: 30, PC: 100,
		CuringPct:         curingPctForTest,
		UpslopeAzimuthDeg: upslope,
	}
	wse := EquivalentWind(base)
	if wse <= 0 {
		t.Fatalf("EquivalentWind = %v, want > 0 — the rest of this test is vacuous", wse)
	}
	const ws = 10.0

	// Wind FROM the south (wd 180) blows towards the north: straight upslope.
	aligned := base
	aligned.WindKmh = ws
	aligned.WindAzimuthDeg = 180 + 180
	wsv, raz := NetEffectiveWind(aligned)
	if !almostEqual(wsv, ws+wse, 1e-9) {
		t.Errorf("wind driving upslope: WSV = %v, want WS+WSE = %v", wsv, ws+wse)
	}
	if !sameBearing(raz, 0, 1e-9) {
		t.Errorf("wind driving upslope: RAZ = %v, want 0 (due north)", raz)
	}

	// Wind FROM the north (wd 0) blows towards the south: straight downslope.
	opposed := base
	opposed.WindKmh = ws
	opposed.WindAzimuthDeg = 0 + 180
	wsv, raz = NetEffectiveWind(opposed)
	if !almostEqual(wsv, math.Abs(wse-ws), 1e-9) {
		t.Errorf("wind opposing slope: WSV = %v, want |WSE-WS| = %v", wsv, math.Abs(wse-ws))
	}
	// WSE exceeds WS here, so the slope still wins and the fire runs north.
	wantRAZ := 0.0
	if ws > wse {
		wantRAZ = 180
	}
	if !sameBearing(raz, wantRAZ, 1e-9) {
		t.Errorf("wind opposing slope: RAZ = %v, want %v", raz, wantRAZ)
	}

	// And the whole point: opposing must be slower than aligned.
	alignedROS, _ := slopeAdjustedROS(aligned, 60)
	opposedROS, _ := slopeAdjustedROS(opposed, 60)
	if !(opposedROS < alignedROS) {
		t.Errorf("opposing ROS %v is not below aligned ROS %v", opposedROS, alignedROS)
	}
}

// Rotating the whole world must not change how fast the fire moves, only which
// way it goes. This is what stands in for the fixture's missing aspect sweep, and
// for the invariance question it is strictly stronger: the fixture pins Aspect at
// 0 and could never test it at all.
//
// A failure means a sin/cos swap or a quadrant error in eq. 51.
func TestNetEffectiveWindRotationalInvariance(t *testing.T) {
	for _, code := range allFuels() {
		for _, slope := range []float64{5, 30, 69.9, 70, 150} {
			for _, ws := range []float64{0, 5, 20, 45} {
				base := SlopeWind{
					Code: code, FFMC: 88, SlopePct: slope, WindKmh: ws,
					WindAzimuthDeg: 37, UpslopeAzimuthDeg: 214,
					PC: 50, CuringPct: curingPctForTest,
				}
				wsv0, raz0 := NetEffectiveWind(base)
				for theta := 0.0; theta < 360; theta += 10 {
					r := base
					r.WindAzimuthDeg += theta
					r.UpslopeAzimuthDeg += theta
					wsv, raz := NetEffectiveWind(r)
					if !almostEqual(wsv, wsv0, 1e-12) {
						t.Fatalf("%s slope %v ws %v rot %v: WSV = %v, want %v",
							code, slope, ws, theta, wsv, wsv0)
					}
					want := normalizeDeg(raz0 + theta)
					if !sameBearing(raz, want, 1e-9) {
						t.Fatalf("%s slope %v ws %v rot %v: RAZ = %v, want %v",
							code, slope, ws, theta, raz, want)
					}
				}
			}
		}
	}
}

// The formal version of "only the difference of the two azimuths reaches the
// speed". It is what makes the +180-on-both reasoning in SlopeWind's doc safe:
// a caller that gets both conventions wrong the same way still gets the right
// WSV, and this is the property that guarantees it rather than assumes it.
func TestNetEffectiveWindDependsOnlyOnRelativeAngle(t *testing.T) {
	base := SlopeWind{
		Code: "C3", FFMC: 92, SlopePct: 40, WindKmh: 18,
		WindAzimuthDeg: 100, UpslopeAzimuthDeg: 25,
		PC: 100, CuringPct: curingPctForTest,
	}
	want, _ := NetEffectiveWind(base)
	for _, k := range []float64{-1234.5, -180, 0.25, 73.1, 999} {
		s := base
		s.WindAzimuthDeg += k
		s.UpslopeAzimuthDeg += k
		if got, _ := NetEffectiveWind(s); !almostEqual(got, want, 1e-12) {
			t.Errorf("offset %v: WSV = %v, want %v", k, got, want)
		}
	}
}

// The identity the whole back-solve rests on: with no wind to vector-add, the
// equivalent wind is all there is, and the published path must reproduce the
// multiplicative one exactly.
//
// Three documented exceptions, all findings rather than conveniences, and all
// asserted here as inequalities rather than skipped:
//
// All four mixedwoods. Eq. 42 blends the ISF of C2 and D1, and blending two
// inverted ISFs is not the inverse of blending two RSIs, so for mixedwood the
// identity genuinely does not hold — RSI · BE · SF reads up to 6.5 % high (M2 at
// 60 % rise, pc 50). This is not our deviation: the fixture's sloped mixedwood
// rows show cffdrs breaking the identity in exactly the same way, which is what
// confirmed that slopeEquivalentISF should blend ISF rather than RSF.
//
// M3/M4 break it harder, and for a second reason on top of the first. Eqs.
// 42b/42c reach the pure dead-fir component by forcing PDF to 100, so the ISF
// being blended is the fuel's own eq. 30 curve at full weight — while the RSI
// side blends that same curve at the caller's actual PDF, and for M4 also
// carries a 0.2 on its deciduous half that the slope path drops. The two sides
// are weighting different things, so the gap is a factor rather than a few per
// cent. The inequality still holds, which is what this branch asserts.
//
// The isfClampMin clamp, where the slope demands more spread than the fuel's RSI
// curve can deliver.
//
// The EquivalentWindCapKmh saturation, where the implied equivalent wind runs off
// the end of eq. 47's inverse. S1 at FFMC 95 on a 60 % slope wants an ISF of
// 148.7, and 112.45 km/h only buys 103.7 — a 4.4 % shortfall.
//
// Neither bound is the rare corner it first looks like: at FFMC 95 one or the
// other fires for most fuels above about 60 % rise, which is ordinary steep dry
// ground rather than an edge case. They are what keep the answer a
// bounded number rather than a NaN or a runaway, and both cost accuracy in the
// same, safe direction — the back-solve must still never come out ABOVE
// RSI · BE · SF, which is what makes ROS's "upper bound, never an
// under-estimate" contract true at zero wind as well as under wind.
func TestZeroWindRecoversRSIxBExSF(t *testing.T) {
	const bui = 60
	for _, code := range allFuels() {
		mixedwood := code == "M1" || code == "M2" || code == "M3" || code == "M4"
		for _, ffmc := range []float64{60, 75, 85, 90, 95} {
			for _, slope := range []float64{5, 15, 30, 45, 60, 69.9, 70, 100, 200} {
				s := SlopeWind{
					Code: code, FFMC: ffmc, SlopePct: slope, WindKmh: 0,
					PC: 100, PDF: pdfPctForTest, CuringPct: curingPctForTest,
				}
				got, _ := slopeAdjustedROS(s, bui)
				want := ROS(code, ISI(ffmc, 0), bui, 100, pdfPctForTest, curingPctForTest, slope)
				_, clamped := slopeEquivalentISF(s)
				saturated := EquivalentWind(s) >= EquivalentWindCapKmh

				if clamped || saturated || mixedwood {
					if got > want*(1+1e-9) {
						t.Errorf("%s FFMC %v slope %v%%: back-solve %v exceeds RSI·BE·SF %v",
							code, ffmc, slope, got, want)
					}
					continue
				}
				if !closeEnough(got, want, 1e-9) {
					t.Errorf("%s FFMC %v slope %v%%: back-solve %v, RSI·BE·SF %v (ratio %.6f)",
						code, ffmc, slope, got, want, got/want)
				}
			}
		}
	}
}

// The clamp is documented as reachable in ordinary conditions. If a
// change to the coefficients or to SlopeFactor ever made it unreachable, the
// inequality branch of TestZeroWindRecoversRSIxBExSF would quietly stop covering
// anything and its comment would become false. This is what notices.
func TestISFClampIsReachable(t *testing.T) {
	s := SlopeWind{Code: "C2", FFMC: 95, SlopePct: 70, PC: 100, CuringPct: curingPctForTest}
	if _, clamped := slopeEquivalentISF(s); !clamped {
		t.Errorf("isfClampMin no longer fires for C2 at FFMC 95 on a 70%% slope — "+
			"if that is intended, the clamp's doc comment and %s need updating",
			"TestZeroWindRecoversRSIxBExSF")
	}
}

// The absence of this ordering IS the bug the back-solve exists to fix: RSI·BE·SF
// applies the full slope factor whether the wind is helping or fighting it, so it
// returns the same number for all three. A failure means the sign of the vector
// addition is inverted.
func TestOpposingWindNeverExceedsAlignedWind(t *testing.T) {
	const bui = 60
	for _, code := range allFuels() {
		for _, slope := range []float64{5, 30, 70, 150} {
			for _, ws := range []float64{1, 10, 35, 60} {
				mk := func(rel float64) SlopeWind {
					return SlopeWind{
						Code: code, FFMC: 90, SlopePct: slope, WindKmh: ws,
						WindAzimuthDeg: rel, UpslopeAzimuthDeg: 0,
						PC: 50, CuringPct: curingPctForTest,
					}
				}
				aligned, cross, opposed := mk(0), mk(90), mk(180)
				wsvA, _ := NetEffectiveWind(aligned)
				wsvC, _ := NetEffectiveWind(cross)
				wsvO, _ := NetEffectiveWind(opposed)
				if !(wsvO <= wsvC+1e-12 && wsvC <= wsvA+1e-12) {
					t.Errorf("%s slope %v ws %v: WSV opposed %v, cross %v, aligned %v — not ordered",
						code, slope, ws, wsvO, wsvC, wsvA)
				}
				rosA, _ := slopeAdjustedROS(aligned, bui)
				rosC, _ := slopeAdjustedROS(cross, bui)
				rosO, _ := slopeAdjustedROS(opposed, bui)
				if !(rosO <= rosC+1e-12 && rosC <= rosA+1e-12) {
					t.Errorf("%s slope %v ws %v: ROS opposed %v, cross %v, aligned %v — not ordered",
						code, slope, ws, rosO, rosC, rosA)
				}
			}
		}
	}
}

// EquivalentWind inverts ISI, so composing the two must be the identity. This is
// the ONLY coverage the high-wind branch above HighWindKmh and the
// EquivalentWindCapKmh saturation have: the fixture's sloped rows are all at FFMC
// 85 and top out at WSE 38.743, below the branch. Adding 95 to the sloped sweep's
// FFMC in gen_cffdrs_reference.R would replace this with oracle backing.
//
// True by construction on both branches, so the tolerance is tight below the
// branch point. Around 40 km/h the two fW branches do not perfectly meet — the
// low branch's inverse at 40 returns 39.9953 — and that 0.024 % step is in the
// published system, not in this port.
func TestEquivalentWindRoundTrip(t *testing.T) {
	for ffmc := 60.0; ffmc <= 99; ffmc += 3 {
		ff := fineFuelMoistureFunction(ffmc)
		for ws := 0.0; ws <= 120; ws += 2.5 {
			isf := ISI(ffmc, ws)
			// Invert by the same branches EquivalentWind uses.
			wse := math.Log(isf/(0.208*ff)) / 0.05039
			if wse > HighWindKmh {
				if isf >= 0.999*2.496*ff {
					continue // the saturation, deliberately not invertible
				}
				wse = 28 - math.Log(1-isf/(2.496*ff))/0.0818
			}
			tol := 1e-12
			if ws >= HighWindKmh-1 {
				tol = 1e-3
			}
			if got := ISI(ffmc, wse); !closeEnough(got, isf, tol) {
				t.Errorf("FFMC %v ws %v: round trip gave ISI %v from WSE %v, want %v",
					ffmc, ws, got, wse, isf)
			}
		}
	}
}

// The NaN sweep, and the reason isfClampMin exists.
//
// Without the clamp, FFMC 95 on 65 %+ rise drives RSF past the RSI asymptote for
// C2, C6, S3 and O1B; the log then takes a negative argument and returns NaN.
// A NaN spread rate is not a loud failure — a caller that reads NaN as no-data
// gets a silent hole, sited exactly on the steepest and driest ground. This also covers FFMC out of range (m^5.31 of a
// negative) and an unknown fuel code (ISF 0, log(0) = -Inf).
func TestEquivalentWindIsNonNegativeAndFiniteEverywhere(t *testing.T) {
	codes := append(allFuels(), "", "nonsense")
	for _, code := range codes {
		for _, ffmc := range []float64{0, 1, 60, 85, 90, 95, 99, 100, 101, 120} {
			for _, slope := range []float64{0, 5, 70, 200, 1e6} {
				for _, pc := range []float64{0, 50, 100} {
					for _, cc := range []float64{0, 50, 100} {
						s := SlopeWind{
							Code: code, FFMC: ffmc, SlopePct: slope,
							PC: pc, CuringPct: cc,
						}
						wse := EquivalentWind(s)
						if math.IsNaN(wse) || math.IsInf(wse, 0) || wse < 0 {
							t.Fatalf("EquivalentWind(%s, FFMC %v, slope %v, pc %v, cc %v) = %v",
								code, ffmc, slope, pc, cc, wse)
						}
						s.WindKmh = 25
						s.WindAzimuthDeg = 210
						s.UpslopeAzimuthDeg = 15
						wsv, raz := NetEffectiveWind(s)
						if math.IsNaN(wsv) || math.IsInf(wsv, 0) || wsv < 0 {
							t.Fatalf("NetEffectiveWind(%s, FFMC %v, slope %v) WSV = %v",
								code, ffmc, slope, wsv)
						}
						if math.IsNaN(raz) || raz < 0 || raz >= 360 {
							t.Fatalf("NetEffectiveWind(%s, FFMC %v, slope %v) RAZ = %v",
								code, ffmc, slope, raz)
						}
						ros, _ := slopeAdjustedROS(s, 60)
						if math.IsNaN(ros) || math.IsInf(ros, 0) || ros < 0 {
							t.Fatalf("slopeAdjustedROS(%s, FFMC %v, slope %v) = %v",
								code, ffmc, slope, ros)
						}
					}
				}
			}
		}
	}
}

// Steeper ground is never worth less equivalent wind, and above SlopeCapPct the
// slope factor saturates so the equivalent wind must too.
func TestEquivalentWindMonotonicInSlope(t *testing.T) {
	for _, code := range allFuels() {
		s := SlopeWind{Code: code, FFMC: 90, PC: 50, CuringPct: curingPctForTest}
		prev := -1.0
		for slope := 0.0; slope <= 120; slope += 2.5 {
			s.SlopePct = slope
			wse := EquivalentWind(s)
			if wse < prev-1e-12 {
				t.Errorf("%s: WSE fell from %v to %v between slope %v and %v",
					code, prev, wse, slope-2.5, slope)
			}
			prev = wse
		}
		s.SlopePct = SlopeCapPct
		atCap := EquivalentWind(s)
		s.SlopePct = 1e6
		if beyond := EquivalentWind(s); !almostEqual(beyond, atCap, 1e-12) {
			t.Errorf("%s: WSE %v beyond the cap, %v at it — SF saturates, so this must too",
				code, beyond, atCap)
		}
	}
}

// Flat ground must be an exact identity, not an approximation. A caller relies
// on this to leave every flat cell bit-for-bit unchanged.
func TestFlatGroundIsAnExactIdentity(t *testing.T) {
	for _, code := range allFuels() {
		for _, ws := range []float64{0, 7, 45} {
			for _, slope := range []float64{0, -1, -90} {
				s := SlopeWind{
					Code: code, FFMC: 88, SlopePct: slope, WindKmh: ws,
					WindAzimuthDeg: 137, UpslopeAzimuthDeg: 0,
					PC: 50, CuringPct: curingPctForTest,
				}
				wsv, raz := NetEffectiveWind(s)
				if wsv != ws {
					t.Errorf("%s slope %v: WSV = %v, want exactly %v", code, slope, wsv, ws)
				}
				if raz != 137 {
					t.Errorf("%s slope %v: RAZ = %v, want exactly 137", code, slope, raz)
				}
			}
		}
	}
}
