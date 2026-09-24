package fwi

import "math"

// State is what the FWI System carries from one day to the next: the three
// moisture codes. ISI, BUI, FWI and DSR are recomputed every day from these and
// the day's weather, so they are not state.
type State struct {
	FFMC float64
	DMC  float64
	DC   float64
}

// StartupState is the conventional spring start-up, StartupFFMC, StartupDMC and
// StartupDC — what cffdrs' fwi() uses when it is given no init. It is a
// starting point the caller chooses, not one Step assumes.
func StartupState() State {
	return State{FFMC: StartupFFMC, DMC: StartupDMC, DC: StartupDC}
}

// Weather is one day's input, in the FWI System's units — see the package
// documentation's units table before filling it from model output.
type Weather struct {
	TempC    float64 // noon temperature, °C
	RHPct    float64 // noon relative humidity, %
	WindKmh  float64 // noon 10 m open wind speed, km/h — not m/s
	PrecipMm float64 // precipitation over the 24 h ending at noon, mm — not hourly
}

// Indices is one day's full output: today's three codes and the four indices
// computed from them.
type Indices struct {
	FFMC float64
	DMC  float64
	DC   float64
	ISI  float64
	BUI  float64
	FWI  float64
	DSR  float64
}

// State returns the codes to carry into tomorrow.
func (ix Indices) State() State { return State{FFMC: ix.FFMC, DMC: ix.DMC, DC: ix.DC} }

// rhCeiling is what fwi() replaces a relative humidity of 100 % or more with
// before any code sees it.
const rhCeiling = 99.9999

// Step advances the FWI System by one day: yesterday's codes and today's
// weather, in the given month (1-12) at the given latitude (degrees, north
// positive), give today's codes and indices. It is one iteration of the loop
// inside cffdrs' fwi(), with lat.adjust = TRUE:
//
//  1. RH at or above 100 % becomes 99.9999. The codes on their own take RH as
//     given; this clamp is the driver's, so it is here and not in FFMC or DMC.
//     It matters at exactly 100: the DMC's drying term is (100 − RH), so a
//     saturated day dries the duff by a hair instead of not at all.
//  2. FFMC, DMC and DC from yesterday's values.
//  3. ISI from today's FFMC, BUI from today's DMC and DC, FWI from those, and
//     DSR from FWI.
//
// The returned State is today's three codes, for tomorrow's Step.
//
// fwi() stops with an error on negative precipitation, wind or relative
// humidity. Step has no error to return, so it returns NaN in every field
// instead — a value that cannot be mistaken for a code, and that carries
// through every later day of a chain rather than quietly recovering. NaN in any
// weather field does the same, and a month outside 1-12 makes the DMC, DC, BUI,
// FWI and DSR NaN.
//
// What fwi() does around this loop — sorting stations by date, batching them,
// defaulting a missing latitude to 55 °N and a missing month to July — is not
// here, deliberately. A sequence of days, and what to do across a gap in one,
// is the caller's.
func Step(yesterday State, w Weather, month int, latDeg float64) (State, Indices) {
	if w.PrecipMm < 0 || w.WindKmh < 0 || w.RHPct < 0 ||
		anyNaN(w.TempC, w.RHPct, w.WindKmh, w.PrecipMm) {
		nan := math.NaN()
		return State{nan, nan, nan}, Indices{nan, nan, nan, nan, nan, nan, nan}
	}
	rh := w.RHPct
	if rh >= 100 {
		rh = rhCeiling
	}

	ix := Indices{
		FFMC: FFMC(yesterday.FFMC, w.TempC, rh, w.WindKmh, w.PrecipMm),
		DMC:  DMC(yesterday.DMC, w.TempC, rh, w.PrecipMm, month, latDeg),
		DC:   DC(yesterday.DC, w.TempC, rh, w.PrecipMm, month, latDeg),
	}
	ix.ISI = ISI(ix.FFMC, w.WindKmh)
	ix.BUI = BUI(ix.DMC, ix.DC)
	ix.FWI = FWI(ix.ISI, ix.BUI)
	ix.DSR = DSR(ix.FWI)
	return ix.State(), ix
}
