package spread

import (
	"math"

	"github.com/LukasSelin/gofbp/fuel"
)

// THE ACCELERATION MODEL: a fire growing from a point towards equilibrium.
//
// Everything else in this package is an EQUILIBRIUM prediction — the rate a fire
// reaches once it has burned long enough for its front to stop speeding up. A
// fire that has just been ignited from a point (a lightning strike, a spot fire
// across a road) is not there yet: its head starts at zero and approaches the
// equilibrium rate exponentially, and its shape starts as a circle and stretches
// towards the equilibrium length-to-breadth ratio over the same time scale.
//
// ST-X-3 models that with one rate constant, alpha (eq. 72), and three
// consequences of it:
//
//	ROS(t)  = ROSeq · (1 − e^(−α·t))                     eq. 70
//	D(t)    = ROSeq · (t + e^(−α·t)/α − 1/α)             eq. 71, its integral
//	LB(t)   = (LBeq − 1) · (1 − e^(−α·t)) + 1            Wotton et al. 2009, eq. 81
//
// with t in MINUTES since ignition. cffdrs' fbp() takes hours and multiplies by
// 60 before calling these; this package takes minutes, which is the unit every
// rate here is already in (m/min). A caller holding hours multiplies, once.
//
// # Alpha
//
// For the "open" fuels — C1, D1, S1, S2, S3 and both grasses — α is the
// constant 0.115 per minute: about 90 % of equilibrium after 20 minutes. For
// every other fuel it depends on how much of the crown is burning:
//
//	α = 0.115 − 18.8 · CFB^2.5 · e^(−8·CFB)               eq. 72
//
// which is NOT monotone in CFB. It falls from 0.115 at CFB 0 to its minimum,
// about 0.0307, at CFB = 2.5/8 = 0.3125, and climbs back towards 0.115 as the
// crown fire becomes continuous. An intermittent crown fire is the slowest to
// reach equilibrium, which is the published model's statement, not an artefact
// of the fit. α stays positive for every CFB, so eq. 71's 1/α is always finite.
//
// The open-fuel list is cffdrs'. An unimplemented code takes the closed-canopy
// form, as cffdrs' ifelse does and as LengthToBreadth's fallback does.
//
// # Which CFB, and which rates
//
// fbp() uses the HEAD fire's CFB for every rate it accelerates, including the
// back rate BROS — not the back fire's own CFB. That is reproduced by taking CFB
// as a parameter: pass the head CFB, as the oracle test does, to match fbp().
// The accelerated flank rate is then FlankROS(ROSAtTime(head), ROSAtTime(back),
// LengthToBreadthAtTime(…)), which is how fbp() builds its FROSt.
//
// # What is not here
//
// fbp()'s flank distance DF = (DH + DB) / (2·LBt) and its time to crown fire
// initiation TI are computed inline in the driver, not in these three upstream
// files, and stay on MIGRATION.md's driver row.

// OpenFuelAcceleration is eq. 72's alpha for the open fuel types, per minute.
const OpenFuelAcceleration = 0.115

// isOpenFuel is cffdrs' list: C1, O1A, O1B, S1, S2, S3, D1.
func isOpenFuel(canonical string) bool {
	switch canonical {
	case "C1", "O1A", "O1B", "S1", "S2", "S3", "D1":
		return true
	}
	return false
}

// AccelerationAlpha is ST-X-3 eq. 72: the point-ignition acceleration rate
// constant per minute, for a fuel type and the head fire's crown fraction burned.
// See the file comment for the shape of the closed-canopy form.
func AccelerationAlpha(code string, cfb float64) float64 {
	canonical, _ := fuel.CanonicalFuelCode(code)
	if isOpenFuel(canonical) {
		return OpenFuelAcceleration
	}
	return OpenFuelAcceleration - 18.8*math.Pow(cfb, 2.5)*math.Exp(-8*cfb)
}

// growth is 1 − e^(−α·t), the fraction of equilibrium reached after t minutes.
// A negative or NaN elapsed time returns NaN: before ignition is not a time this
// model describes, and e^(−α·t) for t < 0 would otherwise report a rate above
// equilibrium with nothing to say it is wrong. (fbp() folds a negative HR to its
// absolute value; that is the driver tidying input, and a caller wanting it can
// do it.)
func growth(alpha, elapsedMin float64) float64 {
	if !(elapsedMin >= 0) {
		return math.NaN()
	}
	return 1 - math.Exp(-alpha*elapsedMin)
}

// ROSAtTime is ST-X-3 eq. 70: the rate of spread in m/min elapsedMin minutes
// after a point ignition, for a fire whose equilibrium rate is rosEq. It is 0 at
// ignition and approaches rosEq from below.
//
// It applies to any rate along the ellipse's axis — fbp() calls it for the head
// (HROSt) and the back (BROSt) — and cfb is the HEAD fire's crown fraction burned
// in both cases. See the file comment.
func ROSAtTime(code string, rosEq, elapsedMin, cfb float64) float64 {
	return rosEq * growth(AccelerationAlpha(code, cfb), elapsedMin)
}

// DistanceAtTime is ST-X-3 eq. 71: the distance in metres a fire front has
// travelled elapsedMin minutes after a point ignition, at equilibrium rate rosEq
// and with CFB the head fire's crown fraction burned. It is eq. 70 integrated
// from ignition, so it is always less than rosEq·t and approaches
// rosEq·(t − 1/α) once the fire has reached equilibrium — a constant lag of
// rosEq/α metres behind a fire that started at full speed.
//
// fbp() calls it for the head (DH) and the back (DB).
func DistanceAtTime(code string, rosEq, elapsedMin, cfb float64) float64 {
	alpha := AccelerationAlpha(code, cfb)
	if !(elapsedMin >= 0) {
		return math.NaN()
	}
	return rosEq * (elapsedMin + math.Exp(-alpha*elapsedMin)/alpha - 1/alpha) // eq. 71
}

// LengthToBreadthAtTime is the length-to-breadth ratio elapsedMin minutes after
// a point ignition (Wotton, Alexander & Taylor 2009, GLC-X-10, eq. 81): 1 — a
// circle — at ignition, approaching the equilibrium lbEq from LengthToBreadth at
// the same rate eq. 70 approaches the equilibrium spread rate.
//
// The equation number is GLC-X-10's, as cffdrs cites it. It is not this
// package's LengthToBreadth "eq. 81", which is ST-X-3's grass form; the two
// publications number independently.
func LengthToBreadthAtTime(code string, lbEq, elapsedMin, cfb float64) float64 {
	return (lbEq-1)*growth(AccelerationAlpha(code, cfb), elapsedMin) + 1
}
