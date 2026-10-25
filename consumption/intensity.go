package consumption

import (
	"math"
)

// FIRELINE INTENSITY: the rate of energy release per metre of fire front.
//
// This is the number a fire behaviour prediction is usually read for. Rate of
// spread says where the fire will be; intensity says what can be done about it
// when it gets there — the suppression thresholds in operational use (hand
// tools, heavy equipment, air tankers, nothing) are all stated in kW/m. It is
// also the quantity that ties the two halves of this package together, which is
// why it was the last FBP row to land before the time-dependent ones: it is a
// rate of spread (fbp.go, slopewind.go, c6.go) multiplied by a fuel consumption
// (consumption.go).
//
// Byram's intensity is I = H·w·r, heat of combustion times fuel consumed times
// spread rate. ST-X-3 fixes H at 18,000 kJ/kg for every fuel type and carries w
// in kg/m² and r in m/min, so the units close as
//
//	18000 kJ/kg · kg/m² · m/min / 60 s/min = 300 · FC · ROS  kW/m
//
// and that is eq. 69 in full. There is no per-fuel coefficient anywhere in it.
//
// # Which rate, and which consumption
//
// The head fire intensity HFI is FireIntensity(TFC, ROS), and both arguments
// are exactly what the rest of this package already returns: TFC from
// TotalFuelConsumption, ROS the head rate fbp() reports. That rate is the
// SURFACE rate RSS for sixteen fuels and the crown-blended rate C6CrownFire
// returns for C6 — see crown.go's "What this file does NOT do". A C1 stand at
// CFB 0.95 therefore has its intensity raised by crown fuel consumption and not
// by a faster crown rate, which is the reference implementation's behaviour and
// not a simplification made here.
//
// cffdrs calls the same function for the flank and back intensities (FFI, BFI)
// with flank and back fuel consumptions, which need CFB evaluated at the flank
// and back rates. This package does not compute those consumptions yet, so it
// does not claim those intensities — but FireIntensity is the right call for
// them once it does, since eq. 69 does not know which direction it is facing.

// heatYieldFactor is eq. 69's 300: an 18,000 kJ/kg heat of combustion divided by
// 60 s/min. It is a unit conversion with one physical constant in it, not a
// fitted coefficient, and it is the same for every fuel type.
const heatYieldFactor = 18000.0 / 60

// FireIntensity is Byram's fireline intensity in kW/m (ST-X-3 eq. 69):
// 300 · fuel consumption (kg/m²) · rate of spread (m/min).
//
// For the FBP System's head fire intensity HFI, pass TotalFuelConsumption and
// the head rate — RSS for every fuel but C6, C6CrownFire's blended rate for C6.
// This is how cffdrs' fbp() computes its HFI column, and the oracle test asserts
// exactly that composition.
//
// There is no floor and no clamp, which is cffdrs' behaviour: a zero rate or a
// zero consumption is a zero intensity, and a negative input — which nothing in
// this package produces — passes straight through rather than being silently
// corrected into a plausible number.
//
// A non-finite result returns NaN rather than an infinity, matching the
// consumption functions: an infinite input is a broken driver, not a very
// intense fire, and NaN is what propagates a broken driver honestly. A NaN
// input propagates as NaN.
func FireIntensity(fuelConsumption, rosMMin float64) float64 {
	fi := heatYieldFactor * fuelConsumption * rosMMin // eq. 69
	if math.IsInf(fi, 0) {
		return math.NaN()
	}
	return fi
}
