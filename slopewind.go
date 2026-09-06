package fbp

import "math"

// SlopeWind is the input bundle for the equivalent-wind back-solve: everything
// the FBP System needs to turn "steep ground plus real wind" into one net
// effective wind.
//
// It is a struct rather than eight positional arguments because six of the eight
// are floats and four of those are angles. Swapping two angles is precisely the
// bug this package cannot afford — it does not blow up, it returns plausible
// numbers that are wrong by a factor of 60 (see
// TestNetEffectiveWindRotationalInvariance for the measurement).
//
// All angles are degrees clockwise from true north, and all are the azimuth the
// thing PUSHES TOWARDS, not the azimuth it comes from:
//
//   - WindAzimuthDeg is where the wind is blowing to. A meteorological
//     "wind from" bearing — the near-universal convention in weather data — is
//     this minus 180.
//   - UpslopeAzimuthDeg is where the ground rises. A downslope aspect — what a
//     terrain aspect raster normally stores — is this minus 180.
//
// Only the DIFFERENCE of the two reaches the net wind speed, so a caller that
// gets both conventions wrong in the same direction still gets the right speed.
// It gets the wrong spread azimuth. Do not rely on the cancellation.
type SlopeWind struct {
	Code              string
	FFMC              float64
	SlopePct          float64 // percent rise; see SlopePercentFromDegrees
	WindKmh           float64
	WindAzimuthDeg    float64
	UpslopeAzimuthDeg float64
	PC                float64 // percent conifer, M1/M2 only
	CuringPct         float64 // percent cured, O1A/O1B only
}

// EquivalentWind is WSE (ST-X-3 eqs. 40-44, 47): the wind speed in km/h that,
// blowing over level ground, would drive a head fire as fast as this slope does
// with no wind at all.
//
// This is the whole point of the FBP slope treatment and the reason RSI · BE · SF
// is wrong. SF is a multiplier on a spread rate, so it applies whole no matter
// which way the wind blows; WSE is a VECTOR, so wind blowing downhill can cancel
// it. See NetEffectiveWind, which does the cancelling.
//
// The path: take the zero-wind, level ISI (ISZ), get the zero-wind spread rate
// RSZ from it, scale that by SF to get the spread rate the slope alone produces
// (RSF), then invert RSI to recover the ISF that would have produced RSF on the
// level — and invert ISI once more to recover the wind behind that ISF.
//
// Returns 0 for a slope at or below 0, for an unknown fuel code, for an FFMC
// outside (0, 101], and wherever the back-solve is otherwise undefined. It is
// never negative: SF >= 1 forces ISF >= ISZ.
//
// Above 70 % rise SF saturates at 10 (eq. 39), so WSE saturates too — at about
// 32.9 km/h for C2 at FFMC 85. The saturation is now expressed as a bounded
// equivalent WIND rather than a bounded MULTIPLIER, and that is the behaviour
// change that matters: on a 70 % slope under a 40 km/h wind blowing downhill, the
// old cap still multiplied spread by 10, and this cancels it.
//
// Both the high-wind inverse above HighWindKmh and the EquivalentWindCapKmh
// saturation are reachable only on dry ground: an FFMC-85 sweep tops out at a WSE
// of 38.7, just under the branch. The fixture's sloped rows therefore sweep FFMC
// 95 as well, which crosses both. TestEquivalentWindRoundTrip holds them
// independently, asserting WSE is ISI's inverse.
func EquivalentWind(s SlopeWind) float64 {
	if s.SlopePct <= 0 || s.FFMC <= 0 || s.FFMC > 101 {
		return 0
	}
	isf, _ := slopeEquivalentISF(s)
	if isf <= 0 {
		return 0
	}
	ff := fineFuelMoistureFunction(s.FFMC)
	if ff <= 0 {
		return 0
	}
	wse := math.Log(isf/(0.208*ff)) / 0.05039
	if wse > HighWindKmh {
		// Eq. 47, the exact inverse of the high-wind fW: 2.496 = 0.208·12, and
		// the inverse asymptotes as ISF approaches it. cffdrs takes the cap at
		// 0.999 of the asymptote rather than at it, so the log stays finite.
		if isf < 0.999*2.496*ff {
			wse = 28 - math.Log(1-isf/(2.496*ff))/0.0818
		} else {
			wse = EquivalentWindCapKmh
		}
	}
	if wse < 0 {
		return 0
	}
	return wse
}

// slopeEquivalentISF is ISF (ST-X-3 eqs. 41-43): the level-ground ISI that would
// produce the spread rate this slope produces at zero wind.
//
// The three branches are the published system's, not ours. Grass (eq. 43) has the
// curing factor multiplied into RSF, so it must be divided back out of the RSI
// asymptote before inverting or the answer is wrong by CF. Mixedwood (eq. 42)
// blends the ISF of C2 and D1 rather than blending RSF and inverting once — which
// is not the same thing, and is why the zero-wind identity ROS == RSI·BE·SF does
// not hold for M1/M2 — measured against the oracle, RSI·BE·SF reads up to 6.5 %
// high there (M2 at 60 % rise, pc 50); see TestZeroWindRecoversRSIxBExSF, which
// excludes them and says so. Note that M2's 0.2 dead-fir weighting appears in RSI
// but NOT here — both mixedwood types use the plain percent-conifer weight in the
// slope path.
//
// The mixedwood branch was for a while the one part of this file with no oracle
// behind it — the sloped sweep covered C2, C3, D1, S1 and O1b only, while M1 is
// a common real-world mapping. The sweep now includes M1/M2, and
// TestCFFDRSSlopeBackSolve confirms both readings above: cffdrs blends ISF, and
// it does not carry M2's 0.2 into the slope path.
//
// clamped reports whether isfClampMin bound the result — that is, whether the
// slope asked for more spread than the fuel's RSI curve can deliver. It is
// returned rather than swallowed because it is the one condition under which the
// published path stops agreeing with RSI · BE · SF at zero wind, and a test that
// could not see it would have to either skip the region or hard-code which fuels
// reach it. TestZeroWindRecoversRSIxBExSF uses it.
func slopeEquivalentISF(s SlopeWind) (isf float64, clamped bool) {
	isz := ISI(s.FFMC, 0)
	if isz <= 0 {
		return 0, false
	}
	sf := SlopeFactor(s.SlopePct)

	code, ok := CanonicalFuelCode(s.Code)
	if !ok {
		return 0, false
	}

	switch code {
	case "M1", "M2":
		c2, d1 := Fuels["C2"], Fuels["D1"]
		w := s.PC / 100
		isfC2, clampC2 := invertRSI(c2, rsiBase(c2, isz)*sf, 1)
		isfD1, clampD1 := invertRSI(d1, rsiBase(d1, isz)*sf, 1)
		return w*isfC2 + (1-w)*isfD1, clampC2 || clampD1
	case "O1A", "O1B":
		f := Fuels[code]
		cf := CuringFactor(s.CuringPct)
		return invertRSI(f, rsiBase(f, isz)*cf*sf, cf)
	}

	f := Fuels[code]
	return invertRSI(f, rsiBase(f, isz)*sf, 1)
}

// invertRSI solves rsf = (a·scale)·(1 − exp(−b·ISF))^c for ISF, the inverse of
// rsiBase. scale carries the grass curing factor, which multiplies the asymptote
// and so must be inside it rather than applied afterwards.
//
// The isfClampMin floor is load-bearing — see its doc comment. Without it this
// returns NaN on steep dry ground. clamped reports whether it bound the result.
func invertRSI(f Fuel, rsf, scale float64) (isf float64, clamped bool) {
	a := f.A * scale
	if !(a > 0) || !(f.B > 0) || !(f.C > 0) || rsf <= 0 {
		return 0, false
	}
	x := 1 - math.Pow(rsf/a, 1/f.C)
	if !(x > isfClampMin) {
		x, clamped = isfClampMin, true
	}
	return math.Log(x) / -f.B, clamped
}

// NetEffectiveWind is WSV and RAZ (ST-X-3 eqs. 48-51): the slope's equivalent
// wind vector-added to the real wind, giving the wind speed in km/h that actually
// drives the head fire and the azimuth in degrees it runs towards.
//
// This is where wind–slope alignment enters. Wind driving upslope adds to WSE,
// wind opposing it subtracts, and at zero wind WSV is WSE alone.
//
// On flat ground it returns exactly (WindKmh, WindAzimuthDeg) — with nothing to
// vector-add the back-solve is an identity, bit-for-bit, which lets a caller
// short-circuit flat cells and get numbers identical to not calling it at all.
//
// Eq. 51 is computed with math.Atan2 rather than the published acos-plus-quadrant
// form. They are the same function; atan2 has no domain edge to clamp and gets
// the quadrant right by construction.
//
// When WSV is 0 — only when there is neither wind nor slope — the azimuth is
// undefined. cffdrs returns NaN; this returns 0. That is a deliberate deviation:
// a NaN azimuth escaping into a caller's grid would be read as no-data, and the
// direction is meaningless either way.
func NetEffectiveWind(s SlopeWind) (wsvKmh, razDeg float64) {
	if s.SlopePct <= 0 {
		return s.WindKmh, normalizeDeg(s.WindAzimuthDeg)
	}
	wse := EquivalentWind(s)
	waz := s.WindAzimuthDeg * math.Pi / 180
	saz := s.UpslopeAzimuthDeg * math.Pi / 180

	wsx := s.WindKmh*math.Sin(waz) + wse*math.Sin(saz)
	wsy := s.WindKmh*math.Cos(waz) + wse*math.Cos(saz)
	wsv := math.Hypot(wsx, wsy)
	if wsv == 0 {
		return 0, 0
	}
	return wsv, normalizeDeg(math.Atan2(wsx, wsy) * 180 / math.Pi)
}

// The full slope path's final composition — RSI(ISI(FFMC, WSV)) · BE(BUI), where
// slope is NOT multiplied in again because it is already inside WSV — has no
// function here on purpose.
//
// It would have exactly one caller, and that caller is a test. A caller anchored
// on a published ISI rather than deriving ISI from FFMC composes ISI and
// NetEffectiveWind itself and passes slopePct = 0 to ROS. Exporting a second
// composition that nothing outside a test calls would read as an offer, and
// taking it up is the double-count this whole file exists to remove.
//
// The composition is asserted against cffdrs all the same — slopeAdjustedROS in
// cffdrs_test.go, two lines, at the assertion site where it can be read against
// the oracle it is being compared to.

// normalizeDeg folds a bearing into [0, 360).
//
// The second guard is not redundant with the first. A tiny negative input — which
// atan2 produces routinely, since sin(2π) evaluates to -2.4e-16 rather than 0 —
// plus 360 rounds to exactly 360.0 in float64, and 360 is not a bearing in
// [0, 360). Callers that compare against 0 would see a 360° error from a 1e-14
// input.
func normalizeDeg(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	if d >= 360 {
		return 0
	}
	return d
}
