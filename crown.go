package fbp

import "math"

// The CROWN-FIRE THRESHOLD: whether the fire is still in the surface fuels.
//
// Everything else in this package answers "how fast". This file answers a prior
// question — "how fast through WHAT" — and the two are not the same prediction.
// A surface fire and a crown fire in the same stand under the same weather are
// different events: different intensity by an order of magnitude, different
// suppression options, different spotting behaviour. A rate of spread that does
// not say which one it describes is missing the part a reader needs first.
//
// The published chain is three equations and a classification. Critical surface
// intensity CSI is the fireline intensity needed to reach the base of the crown
// (eq. 56); RSO is the surface spread rate that produces it (eq. 57); crown
// fraction burned CFB is how much of the crown is consumed once the surface fire
// exceeds it (eq. 58); and FD folds CFB into the three classes the FBP System
// reports, surface / intermittent / continuous.
//
// # What this file does NOT do
//
// It does not change ROS. That is worth stating plainly because the opposite is
// the natural assumption: in the reference implementation the final rate of
// spread is the SURFACE rate for every fuel type except C6 — cffdrs'
// rate_of_spread() returns RSS unchanged wherever the fuel is not C6, and folds
// CFB in only through C6's separate crown path. So for fourteen of the fifteen
// fuels here, CFB is a classification of a spread rate this package already
// computes correctly, not a correction to it. C6's crown path is not implemented
// (see the package doc), so C6's ROS remains surface-only.
//
// It also does not produce its own inputs. FMC (from latitude, longitude,
// elevation and date) and SFC (from FFMC and BUI per fuel) are the caller's, as
// are CBH and CFL. The published system has per-fuel default tables for the
// latter two; they are deliberately absent here — see the package doc.

// FireDescription is the FBP System's FD: which of the three fire types the
// crown fraction burned puts this fire in. The single-letter values are the
// published system's own, and are what cffdrs' FD column contains.
type FireDescription string

const (
	SurfaceFire       FireDescription = "S"
	IntermittentCrown FireDescription = "I"
	ContinuousCrown   FireDescription = "C"
)

// The FD class boundaries, and the decay coefficient in eq. 58.
//
// Note the asymmetry in how the boundaries are applied: DescribeFire treats
// exactly 0.1 as intermittent and exactly 0.9 as continuous. That is not a
// choice made here — it is cffdrs', which starts every case at "I" and then
// overwrites with "S" where CFB < 0.1 and with "C" where CFB >= 0.9. Making the
// two ends symmetric would put a whole class boundary in the wrong place for
// inputs that land on it, which grid-derived CFB does more often than a
// continuous quantity suggests.
const (
	CrownDecayCoefficient = 0.23
	IntermittentCrownCFB  = 0.1
	ContinuousCrownCFB    = 0.9
)

// MinCrownBaseHeightM is the floor cffdrs puts under CBH before eq. 56, and the
// value this package substitutes for a non-positive crown base height.
//
// It is not padding. CBH is 0 in the published table for the fuels that have no
// crown at all — D1, S1, S2, S3 and both grass types — so a caller reading that
// table hands a 0 straight in. At exactly 0, CSI is 0, RSO is 0, and every fire
// with any spread at all reports full crowning: the most alarming possible
// answer, produced for the fuels that cannot crown. The floor does not fix that
// on its own — the CFL gate in CrownFractionBurned is what does — but it keeps
// this function's own output continuous and finite as CBH approaches 0 from
// above, which is where a caller interpolating stand data will sit.
const MinCrownBaseHeightM = 1e-7

// Crown is the input bundle for the crown-fire threshold.
//
// A struct rather than five positional arguments for the reason SlopeWind is
// one: every field is a float64, none of them carries a unit the compiler can
// see, and transposing two returns a plausible number rather than an error. SFC
// and CFL are both fuel loads in kg/m², and swapping those two in particular is
// silent — it moves the threshold and flips the gate at once.
//
// SurfaceROS is the SURFACE head rate, RSI(ISI(FFMC, WSV)) · BE, with no slope
// factor multiplied in. The slope is already inside WSV; ROS's simplified
// RSI · BE · SF product is an upper bound (see its doc) and feeding it here
// over-predicts crowning by that same factor. On a 70 % slope with the wind
// against it that is a tenfold error in the input to an exponential.
type Crown struct {
	FMC        float64 // foliar moisture content, percent
	SFC        float64 // surface fuel consumption, kg/m²
	CBH        float64 // crown base height, m; see MinCrownBaseHeightM
	CFL        float64 // crown fuel load, kg/m²; 0 means the fuel cannot crown
	SurfaceROS float64 // surface head rate, m/min — RSI·BE, NOT RSI·BE·SF
}

// CriticalSurfaceIntensity is CSI in kW/m (ST-X-3 eq. 56): the fireline
// intensity a surface fire must reach before it ignites the crown above it.
//
// Both drivers raise the threshold, and it is worth seeing which way each one
// runs: a higher crown base is further from the flames, and wetter foliage takes
// more energy to ignite. Both make crowning harder, so both make CSI larger.
//
// A non-positive CBH is taken as MinCrownBaseHeightM — see that constant.
// Returns 0 for an FMC that is negative or not a number, and for a CBH that is
// not a number: (460 + 25.9·FMC) goes negative below −17.76 % moisture, and
// raising a negative to the power 1.5 is NaN, which would propagate out of here
// into a caller's grid unannounced.
//
// Read that 0 as "there is no usable threshold here", not as "this crowns
// instantly". Downstream a zero CSI gives a zero RSO, which any spreading fire
// exceeds — so CrownFractionBurned screens its inputs for finiteness itself
// rather than letting them arrive through this sentinel.
func CriticalSurfaceIntensity(fmc, cbh float64) float64 {
	if !(fmc >= 0) || math.IsNaN(cbh) { // the first is false for NaN too
		return 0
	}
	if !(cbh > 0) {
		cbh = MinCrownBaseHeightM
	}
	return 0.001 * math.Pow(cbh, 1.5) * math.Pow(460+25.9*fmc, 1.5)
}

// CriticalSurfaceROS is RSO in m/min (ST-X-3 eq. 57): the surface rate of spread
// that produces the critical surface intensity, given how much surface fuel
// there is to burn. Above it the fire crowns; at or below it, it does not.
//
// The division by SFC is the whole content of the equation: the same spread rate
// through twice the fuel is twice the intensity, so twice the fuel halves the
// spread rate needed to crown. Fuel consumption is where a stand's history
// enters the crown-fire answer.
//
// Returns +Inf for a non-positive SFC — with no surface fuel there is no surface
// intensity and nothing crowns, at any spread rate. That is the honest value
// rather than a sentinel: CrownFractionBurned's comparison against it is false,
// which is the intended behaviour, and a caller who prints it sees an infinity
// rather than a plausible finite threshold it never reached.
func CriticalSurfaceROS(csi, sfc float64) float64 {
	if !(sfc > 0) {
		return math.Inf(1)
	}
	return csi / (300 * sfc)
}

// CrownFractionBurned is CFB (ST-X-3 eq. 58): the fraction of the crown
// consumed, 0 where the fire stays on the surface and approaching 1 where it is
// fully in the canopy.
//
// It rises fast. The exponential's coefficient is 0.23 per m/min of excess over
// RSO, so a fire 10 m/min past the threshold is already at 0.90 — the boundary
// of the continuous-crown class. The interesting range is narrow and it sits
// just above RSO, which is why the threshold's inputs deserve more care than
// their apparent precision suggests.
//
// Returns 0 when CFL is not positive. That gate is doing real work, not
// defending against nonsense: the published CFL is exactly 0 for D1, S1, S2, S3,
// O1A and O1B, the fuels with no crown to burn, and it is the ONLY thing that
// keeps them at zero. Their published CBH is 0 too, so without this the
// equations above would hand back a near-1 CFB for a grass fire. cffdrs applies
// the same gate in the same place, after the equations rather than inside them.
//
// Returns 0 for a non-positive SurfaceROS and for any non-finite input, rather
// than letting a NaN through. A caller reading NaN as no-data would get a hole
// in its output; a caller reading it as a number gets an unbounded one.
//
// The screen has to be here rather than only inside the equations, and NaN CBH
// is why. A missing crown base height is exactly what a stand with no inventory
// data looks like, and it would otherwise fall through the non-positive branch
// in CriticalSurfaceIntensity to MinCrownBaseHeightM, putting the threshold at
// effectively zero and reporting a near-total crown fire wherever the data is
// absent. A negative or NaN FMC arrives at the same place by the other route,
// through that function's own zero sentinel. Silence about the most alarming
// answer is the one failure mode this package cannot have.
func CrownFractionBurned(c Crown) float64 {
	if !(c.CFL > 0) {
		return 0
	}
	if !(c.SurfaceROS > 0) || math.IsInf(c.SurfaceROS, 0) {
		return 0
	}
	if !(c.FMC >= 0) || math.IsNaN(c.SFC) || math.IsNaN(c.CBH) {
		return 0
	}
	rso := CriticalSurfaceROS(CriticalSurfaceIntensity(c.FMC, c.CBH), c.SFC)
	if !(c.SurfaceROS > rso) {
		return 0
	}
	return 1 - math.Exp(-CrownDecayCoefficient*(c.SurfaceROS-rso))
}

// DescribeFire is FD: the FBP System's three-way classification of a crown
// fraction burned.
//
// See the constants above for the boundary convention, which is asymmetric and
// deliberately so. A CFB that is not a number returns IntermittentCrown, which
// is cffdrs' behaviour for the same reason — it initialises every case to "I"
// and only overwrites on a comparison, and neither comparison holds against NaN.
func DescribeFire(cfb float64) FireDescription {
	if cfb >= ContinuousCrownCFB {
		return ContinuousCrown
	}
	if cfb < IntermittentCrownCFB {
		return SurfaceFire
	}
	return IntermittentCrown
}
