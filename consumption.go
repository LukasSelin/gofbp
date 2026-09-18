package fbp

import "math"

// SURFACE FUEL CONSUMPTION: how much fuel per square metre the surface fire
// actually burns.
//
// This is the quantity that turns a rate of spread into an intensity. Two fires
// spreading at the same metres per minute through a jack pine stand and through
// a cured grass sward are not the same event: one is consuming several kilograms
// of forest floor and woody debris per square metre, the other a third of a
// kilogram of grass. SFC is where a stand's accumulated history — duff depth,
// slash, how long since the last fire — enters the FBP System, and it is the
// divisor in eq. 57, so it sets the crown-fire threshold as much as the crown's
// own geometry does.
//
// # Shape of the published system
//
// There is no single SFC equation. There are eleven, one per fuel family, and
// they do not even share their drivers: C2/C3/C4/C5/C6/D1/M1/M2/S1/S2/S3 read
// BUI alone, C1 reads FFMC alone, C7 reads both, and the grass fuels read
// neither — their SFC is the grass fuel load the caller supplies, unchanged.
// That is eq. 18, and it is an identity rather than a model.
//
// So the "coefficient table" for this file is the equations themselves, which is
// why they are named constants grouped by equation number below rather than rows
// of a Fuel struct.
//
// # What this file does NOT do
//
// It does not supply a grass fuel load. GFL is a required parameter, not a
// defaulted one, for the same reason CBH and CFL are the caller's in crown.go:
// the published default of 0.35 kg/m² is a number about a particular kind of
// ground, and choosing it on the caller's behalf is exactly the local judgement
// this package refuses to make. cffdrs' fbp() does substitute 0.35; that is a
// driver's decision, and this package does not have a driver.
//
// It does not compute total fuel consumption or fireline intensity. Those are
// eqs. 59-66 and are not ported yet — see MIGRATION.md.

// The SFC coefficients, by equation number.
//
// Grouped rather than tabulated because the equations have different shapes: a
// bare exponential rise (eqs. 10, 16), one raised to a power (eqs. 11, 12), a sum
// of a forest-floor and a woody term (eqs. 13-15, 19-25), a two-branch square
// root (eqs. 9a, 9b) and a linear blend of two others (eq. 17). A table with a
// column per coefficient would have to leave most of them empty.
const (
	// Eqs. 9a, 9b (Wotton, Alexander & Taylor 2009, GLC-X-10) — C1.
	//
	// These REPLACE ST-X-3's own eq. 9. See the disagreement note on
	// SurfaceFuelConsumption; the hinge at FFMC 84 and the square root are the
	// 2009 revision, not the 1992 publication.
	c1HingeFFMC = 84.0
	c1Half      = 0.75
	c1Rate      = 0.23

	// Eq. 10 (FCFDG 1992) — C2, and M3/M4, whose surface fuel is the same
	// spruce-moss floor with the dead balsam fir standing in it.
	c2Max  = 5.0
	c2Rate = 0.0115

	// Eq. 11 (FCFDG 1992) — C3, C4.
	c3Max   = 5.0
	c3Rate  = 0.0164
	c3Power = 2.24

	// Eq. 12 (FCFDG 1992) — C5, C6.
	c5Max   = 5.0
	c5Rate  = 0.0149
	c5Power = 2.48

	// Eqs. 13, 14, 15 (FCFDG 1992) — C7, the only fuel whose SFC reads both
	// drivers: a forest-floor term in FFMC plus a woody term in BUI.
	c7FloorMax  = 2.0
	c7FloorRate = 0.104
	c7FloorFFMC = 70.0
	c7WoodyMax  = 1.5
	c7WoodyRate = 0.0201

	// Eq. 16 (FCFDG 1992) — D1, and the deciduous half of eq. 17's blend.
	d1Max  = 1.5
	d1Rate = 0.0183

	// Eqs. 19, 20, 25 (FCFDG 1992) — S1.
	s1FloorMax  = 4.0
	s1FloorRate = 0.025
	s1WoodyMax  = 4.0
	s1WoodyRate = 0.034

	// Eqs. 21, 22, 25 (FCFDG 1992) — S2.
	s2FloorMax  = 10.0
	s2FloorRate = 0.013
	s2WoodyMax  = 6.0
	s2WoodyRate = 0.060

	// Eqs. 23, 24, 25 (FCFDG 1992) — S3.
	s3FloorMax  = 12.0
	s3FloorRate = 0.0166
	s3WoodyMax  = 20.0
	s3WoodyRate = 0.0210
)

// MinSurfaceFuelConsumptionKgM2 is the floor the reference implementations put
// under SFC, and it is a load-bearing number rather than a rounding guard.
//
// Every BUI-driven equation here passes through zero at BUI 0 and goes negative
// below it, and a zero or negative fuel load would arrive at eq. 57 as a division
// by zero — CriticalSurfaceROS returns +Inf for a non-positive SFC, which reads
// as "nothing crowns" and is right for a genuinely fuel-free cell but wrong for
// a real stand at the low end of the buildup index. Both NRCan's 1997 C
// implementation and cffdrs clamp to exactly this value in exactly this place,
// after all eleven equations rather than inside any of them.
//
// It is not in ST-X-3. It is an implementation convention of the reference
// implementations, and it is reproduced here because parity with the oracle is
// what this package is for.
const MinSurfaceFuelConsumptionKgM2 = 1e-6

// SurfaceFuelConsumption is SFC in kg/m²: the fuel a surface fire consumes per
// square metre in the given fuel type, at the given fine fuel moisture and
// buildup.
//
// The arguments are in the reference implementation's own order — fuel, FFMC,
// BUI, PC, GFL — so the transcription can be read against it line for line.
// Most fuels ignore most of them: PC reaches only M1 and M2, GFL only O1A and
// O1B, FFMC only C1 and C7. Passing a zero for one a fuel does not read is
// harmless and is what a caller with no mixedwood or grass in its raster should
// do.
//
// The code is folded by CanonicalFuelCode, so case and separators do not matter.
// A fuel this package does not implement returns 0 — see below.
//
// # Where the R disagrees with the published paper
//
// Three places, and the first two are visible in the oracle:
//
//  1. C1 is NOT ST-X-3 eq. 9. The 1992 publication and NRCan's 1997 C reference
//     both have SFC = 1.5·(1 − exp(−0.230·(FFMC − 81))), a single exponential
//     that goes sharply negative below FFMC 81 and is then clamped to the floor.
//     cffdrs uses eqs. 9a/9b from Wotton, Alexander & Taylor (2009), GLC-X-10: a
//     two-branch form hinged at FFMC 84, symmetric about 0.75 kg/m², with a
//     square root on each side. This is a documented revision — cffdrs cites it
//     in the one place it departs from FCFDG 1992 — and the same situation as
//     CuringFactor and GLC-X-10. At FFMC 60 the two differ by more than two
//     orders of magnitude, so the fixture picks the revision out immediately.
//
//  2. C7's forest-floor term is clamped at FFMC 70, and this one is NOT
//     documented. Eq. 13 as published has no branch: 2·(1 − exp(−0.104·(FFMC −
//     70))) simply goes negative below FFMC 70, taking the sum with it and
//     landing on the floor. cffdrs substitutes 0 for that term instead, keeping
//     the woody term of eq. 14 intact, and its comment still cites FCFDG 1992
//     alone. The fixture sweeps FFMC 60 and 75, so it distinguishes the two, and
//     it agrees with cffdrs. Read this as the reference implementation's
//     correction rather than as the paper: below FFMC 70 the forest floor is too
//     wet to carry, which is a floor consumption of nothing, not of a negative
//     amount.
//
//  3. Eq. 17's blend is inlined here and in cffdrs, while the 1997 C reference
//     reaches it by recursively calling itself for C2 and D1 — which floors each
//     component before the blend rather than the blend after it. There is no
//     numerical difference: the two components share a sign at every BUI, so a
//     negative blend implies both were negative, and both routes land on the
//     floor together. Noted only so the next reader does not have to re-derive it.
//
// # Non-finite and unknown input
//
// A NaN in a driver a fuel actually reads propagates to NaN, which is cffdrs'
// behaviour too — its NA falls through the SFC <= 0 comparison untouched. An
// infinity does not: it returns NaN rather than being allowed to reach the
// floor, because a −Inf BUI would otherwise come back as 1e-6 and read as a real
// stand with almost nothing in it. That is a deliberate departure from cffdrs,
// which returns the floor there.
//
// An unimplemented fuel code returns 0, and this is also a departure. cffdrs
// seeds SFC with a −999 sentinel that the final clamp converts to the 1e-6
// floor, so an unknown fuel comes back from it as a plausible near-zero load
// rather than as an error. Returning 0 here at least composes honestly with the
// rest of the package: CriticalSurfaceROS turns a zero SFC into an infinite
// threshold, so nothing crowns and nothing is fabricated. Check the code with
// CanonicalFuelCode where fuel classes enter — once per class, not once per cell
// — rather than relying on either value.
func SurfaceFuelConsumption(code string, ffmc, bui, pc, gfl float64) float64 {
	canonical, known := CanonicalFuelCode(code)
	if !known {
		return 0
	}

	var sfc float64
	switch canonical {
	case "C1":
		// Eqs. 9a, 9b (GLC-X-10). Continuous at the hinge: both branches give
		// exactly 0.75 at FFMC 84, where the square root's argument is 0.
		if ffmc > c1HingeFFMC {
			sfc = c1Half + c1Half*math.Sqrt(1-math.Exp(-c1Rate*(ffmc-c1HingeFFMC)))
		} else {
			sfc = c1Half - c1Half*math.Sqrt(1-math.Exp(-c1Rate*(c1HingeFFMC-ffmc)))
		}
	case "C2", "M3", "M4":
		sfc = c2Max * (1 - math.Exp(-c2Rate*bui)) // eq. 10
	case "C3", "C4":
		sfc = c3Max * math.Pow(1-math.Exp(-c3Rate*bui), c3Power) // eq. 11
	case "C5", "C6":
		sfc = c5Max * math.Pow(1-math.Exp(-c5Rate*bui), c5Power) // eq. 12
	case "C7":
		// Eqs. 13, 14, 15. The FFMC 70 clamp on the forest-floor term is
		// cffdrs', not the paper's — see note 2 above.
		//
		// The NaN guard is not decoration. This is the only branch in the file
		// where a comparison decides between an expression and a LITERAL, so it
		// is the only one where a NaN driver does not propagate: `NaN > 70` is
		// false, the else arm contributes a clean 0, and a no-data FFMC would
		// come back as eq. 14's woody term alone — a plausible kilogram per
		// square metre, from a cell that has no fine fuel moisture reading at
		// all. cffdrs returns NA here (its ifelse propagates), and so does this.
		if math.IsNaN(ffmc) {
			return math.NaN()
		}
		floor := 0.0
		if ffmc > c7FloorFFMC {
			floor = c7FloorMax * (1 - math.Exp(-c7FloorRate*(ffmc-c7FloorFFMC)))
		}
		sfc = floor + c7WoodyMax*(1-math.Exp(-c7WoodyRate*bui))
	case "D1":
		sfc = d1Max * (1 - math.Exp(-d1Rate*bui)) // eq. 16
	case "M1", "M2":
		// Eq. 17: the conifer share burns as C2, the rest as D1. PC is a
		// percentage, and it is the same input that weights RSI's M1/M2 blend.
		conifer := c2Max * (1 - math.Exp(-c2Rate*bui))
		deciduous := d1Max * (1 - math.Exp(-d1Rate*bui))
		sfc = pc/100*conifer + (100-pc)/100*deciduous
	case "O1A", "O1B":
		sfc = gfl // eq. 18
	case "S1":
		sfc = s1FloorMax*(1-math.Exp(-s1FloorRate*bui)) +
			s1WoodyMax*(1-math.Exp(-s1WoodyRate*bui)) // eqs. 19, 20, 25
	case "S2":
		sfc = s2FloorMax*(1-math.Exp(-s2FloorRate*bui)) +
			s2WoodyMax*(1-math.Exp(-s2WoodyRate*bui)) // eqs. 21, 22, 25
	case "S3":
		sfc = s3FloorMax*(1-math.Exp(-s3FloorRate*bui)) +
			s3WoodyMax*(1-math.Exp(-s3WoodyRate*bui)) // eqs. 23, 24, 25
	default:
		// Unreachable while Fuels and this switch agree, and
		// TestSurfaceFuelConsumptionCoversEveryFuel is what holds that.
		return 0
	}

	if math.IsInf(sfc, 0) {
		return math.NaN()
	}
	if sfc <= 0 { // false for NaN, which is the intended pass-through
		return MinSurfaceFuelConsumptionKgM2
	}
	return sfc
}
