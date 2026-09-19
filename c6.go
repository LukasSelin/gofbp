package fbp

import "math"

// C-6's CROWN RATE OF SPREAD, and the blended rate it produces.
//
// C-6 (conifer plantation) is the one fuel type in the FBP System whose reported
// rate of spread is not the surface rate. Every other fuel's ROS is RSS — the
// surface rate — and CFB only classifies it (see crown.go). C-6 has a crown rate
// of spread RSC of its own, and once the fire is in the crowns the published
// answer is a CFB-weighted blend of the two rates. That blend is what cffdrs
// reports in its ROS column for C-6, and it is why C-6 has to be asked for
// separately here rather than falling out of ROS.
//
// # Where the pieces already are
//
// Two of the five published equations in this chain were already implemented,
// because C-6's surface rate is not special:
//
//	eq. 62  RSI = 30·(1 − exp(−0.08·ISI))³ — the intermediate surface rate.
//	        cffdrs gives this its own function, but it is exactly the generic
//	        a·(1 − exp(−b·ISI))^c curve at C-6's table row (a = 30, b = 0.08,
//	        c = 3), so RSI("C6", isi, …) already returns it.
//	eq. 63  RSS = RSI · BE("C6", BUI). Again the generic composition.
//
// So a caller assembles RSS the ordinary way and this file adds the rest:
//
//	eq. 61  FME, the foliar moisture effect          FoliarMoistureEffect
//	eq. 64  RSC, the crown rate of spread            C6CrownROS
//	eq. 58  CFB, gated on RSC > RSS                  C6CrownFractionBurned
//	eq. 65  ROS = RSS + CFB·(RSC − RSS)              C6ROS
//	        the four composed                        C6CrownFire
//
// # Two equations cffdrs computes and does not use
//
// Upstream's crown_rate_of_spread_c6 evaluates eq. 59, the crown flame
// temperature tt = 1500 − 2.75·FMC, and never reads it — it is dead code at the
// pinned 1.9.2 and at upstream HEAD. It also evaluates eq. 60, the heat of
// ignition H = 460 + 25.9·FMC, and then writes that expression out again inline
// as eq. 61's denominator rather than using H. Neither affects a number, and
// neither is reproduced here; they are recorded because a reader comparing this
// file to the R will otherwise go looking for them. Note also that upstream's
// comment on eq. 61 reads "Average foliar moisture effect", which belongs to
// FMEAvg on the line above it — eq. 61 is the foliar moisture effect itself.
//
// # What this file does not do
//
// It does not change ROS, and it cannot: that function takes a fuel code, ISI,
// BUI and the three blend inputs, and C-6's crown path additionally needs FMC,
// SFC, CBH and CFL. There is nowhere in that signature for them, so ROS remains
// the surface rate for all seventeen fuels and a caller who wants C-6's blended
// rate asks for it here. That is the same caller-supplies convention the crown
// threshold already follows.

// FMEAvg is the average foliar moisture effect that eq. 64 normalises RSC by.
//
// The 0.778 is a published constant, not a derived one: it is the FME of the
// stand the C-6 crown curve was fitted to, so RSC is "the fitted rate, scaled by
// how this stand's foliage compares to that one". FoliarMoistureEffect is 0.778
// at a foliar moisture of 97.02 %, which is the moisture that constant describes
// and is squarely inside the range eqs. 1-8 produce;
// TestFMEAvgIsTheFMEOfARealFoliarMoisture pins it.
const FMEAvg = 0.778

// FoliarMoistureEffect is FME (ST-X-3 eq. 61): how readily this stand's foliage
// carries a crown fire. It is a ratio only once divided by FMEAvg, which is what
// eq. 64 does with it.
//
//	FME = 1000 · (1.5 − 0.00275·FMC)⁴ / (460 + 25.9·FMC)
//
// It falls steeply with foliar moisture, which is the whole point: FME is 11.0 at
// FMC 0 and 0.525 at FMC 120, so the crown rate of spread at the wet end of the
// year is a small fraction of its dry-end value.
//
// Returns 0 for an FMC that is negative, not a number, or infinite. A negative
// FMC is not merely out of range: the numerator's fourth power keeps it positive
// while the denominator is heading for zero at −17.76 %, so the equation would
// hand back a large positive FME for impossible input.
//
// # The turning point at FMC 545
//
// The numerator is an even power, so it reaches zero at FMC = 1.5/0.00275 =
// 545.45 % and RISES again above that — FME is not monotone over all of its
// domain, and above the turning point it claims wetter foliage burns faster.
// Nothing in the FBP System can reach there: eq. 8 caps FoliarMoistureContent at
// 120, and 545 % moisture by dry weight is five times the foliage's own mass in
// water. It is left as the equation reads rather than clamped, because clamping
// would be this package inventing a range the published equation does not state.
// A caller supplying its own FMC from measurement should be screening it long
// before this matters; TestFoliarMoistureEffectTurningPoint pins where it is.
func FoliarMoistureEffect(fmc float64) float64 {
	if !(fmc >= 0) || math.IsInf(fmc, 0) { // the first is false for NaN too
		return 0
	}
	return math.Pow(1.5-0.00275*fmc, 4) / (460 + 25.9*fmc) * 1000
}

// C6CrownROS is RSC in m/min (ST-X-3 eq. 64): the rate at which fire spreads
// through C-6's crowns, once it is up there.
//
//	RSC = 60 · (1 − exp(−0.0497·ISI)) · FME/FMEAvg
//
// Note what it does NOT depend on: BUI, and therefore the buildup effect. The
// crown rate is a function of ISI and foliar moisture alone, which is why a
// drought that raises BUI moves C-6's surface rate and leaves its crown rate
// where it was.
//
// The 60 is a saturation, not a scale factor: RSC approaches 60·FME/FMEAvg as
// ISI grows, so at FMC 100 no ISI produces a crown rate above 56.9 m/min.
//
// Returns 0 for a non-positive or NaN ISI. At exactly ISI 0 that is the
// equation's own value rather than a substitute — (1 − exp(0)) is 0 — and a
// negative ISI is not an input the FWI System can produce.
func C6CrownROS(isi, fmc float64) float64 {
	if !(isi > 0) { // false for NaN too
		return 0
	}
	return 60 * (1 - math.Exp(-0.0497*isi)) * FoliarMoistureEffect(fmc) / FMEAvg
}

// C6CrownFractionBurned is C-6's CFB: eq. 58 on the surface rate, exactly as for
// every other fuel, and then gated on the crown rate exceeding the surface rate.
//
// crownROS is RSC, from C6CrownROS. c.SurfaceROS is RSS — RSI·BE, with no slope
// factor multiplied in, the same quantity Crown.SurfaceROS means everywhere else.
//
// The gate is the part worth reading twice, because it is easy to assume C-6 has
// a crown fraction burned of its own and it does not. crown_fraction_burned_c6
// returns crown_fraction_burned(RSS, RSO) — the ordinary eq. 58, driven by the
// SURFACE rate — and RSC enters only as a condition on whether that value is
// reported at all. So C-6's CFB is not "how much of the crown the crown fire
// consumed"; it is the same threshold quantity as every other fuel's, published
// as zero in the case where a crown fire would be slower than the surface fire
// feeding it.
//
// That extra condition is cffdrs' and is not in eq. 58. It is eq. 65's gate,
// applied to the CFB output as well as to the rate — so the two always agree
// about whether C-6 is crowning, which they would not if CFB ignored it. It is
// also unreachable for any FMC the FBP System itself produces; see
// TestC6CrownGateIsUnreachableWithinTheFMCRange for the bound.
//
// Every screen in CrownFractionBurned applies here unchanged — the CFL gate that
// keeps crownless fuels at zero, and the non-finite input screens. A NaN crownROS
// fails the gate and returns 0 rather than propagating.
func C6CrownFractionBurned(c Crown, crownROS float64) float64 {
	if !(crownROS > c.SurfaceROS) { // false for NaN too
		return 0
	}
	return CrownFractionBurned(c)
}

// C6ROS is C-6's reported rate of spread in m/min (ST-X-3 eq. 65): the surface
// rate, moved towards the crown rate by the fraction of the crown that burns.
//
//	ROS = RSS + CFB·(RSC − RSS)   where RSC > RSS
//	ROS = RSS                     otherwise
//
// So it is a linear interpolation between the two rates, and CFB is the weight.
// At CFB 0 the answer is the surface rate and at CFB 1 it is the crown rate; the
// FBP System's continuous-crown class starts at CFB 0.9, which puts a
// continuously crowning C-6 stand within a tenth of its full crown rate.
//
// cfb should come from C6CrownFractionBurned, whose gate is the same one applied
// here. A non-positive or NaN cfb returns the surface rate: at exactly 0 that is
// the equation's own value, and the deviation is for a negative cfb, where the
// published arithmetic would return a rate BELOW the surface rate. There is no
// such fire, and a caller reading it would under-predict.
//
// Upstream floors the final rate at 1e-6 where it is non-positive. That floor is
// not reproduced here, for the same reason ROS does not reproduce it: it fires
// only where the surface rate is already zero, so it replaces one
// nothing-is-spreading answer with another. TestCFFDRSC6RateOfSpread reports
// that the sweep never reaches it.
func C6ROS(surfaceROS, crownROS, cfb float64) float64 {
	if !(crownROS > surfaceROS) || !(cfb > 0) { // false for NaN too
		return surfaceROS
	}
	return surfaceROS + cfb*(crownROS-surfaceROS)
}

// C6CrownFire composes the whole C-6 crown path: the blended rate of spread and
// the crown fraction burned that produced it.
//
// c.SurfaceROS must be RSS, the surface head rate RSI·BE with the slope already
// inside the ISI rather than multiplied on afterwards — the same contract
// Crown.SurfaceROS carries for the threshold, and misreading it over-predicts
// crowning by the whole slope factor. isi is the ISI that surface rate was
// computed from, because RSC needs it too and taking it from the caller is the
// only way to be sure it is the same one.
//
// Returned together because they are one prediction. A caller that recomputed
// CFB separately could feed C6ROS a fraction from a different RSO, and the two
// halves of the same answer would then disagree about whether the stand crowned.
//
// It is the composition and nothing more: no default CBH, CFL or FMC is
// supplied, and a Crown with CFL 0 comes back as a pure surface fire.
func C6CrownFire(c Crown, isi float64) (rosMMin, cfb float64) {
	rsc := C6CrownROS(isi, c.FMC)
	cfb = C6CrownFractionBurned(c, rsc)
	return C6ROS(c.SurfaceROS, rsc, cfb), cfb
}
