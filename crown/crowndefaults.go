package crown

import (
	"github.com/LukasSelin/gofbp/fuel"
)

// PER-FUEL CROWN DEFAULTS: the published crown base height and crown fuel load
// for each fuel type, as a lookup the caller opts into.
//
// Crown still takes CBH and CFL as fields and still applies no default of its
// own. That is the design decision this file was written under, and it is worth
// stating what it protects: a default inside Crown would decide, silently and
// for every caller, what a stand's canopy is made of. A caller with measured
// crown base heights — from lidar, an inventory plot, a fuel map with its own
// attribution — must be able to use them without this package second-guessing
// the value. So nothing here is called by anything else in the package. A
// caller who wants the published values asks for them by name:
//
//	cbh, _ := fbp.DefaultCrownBaseHeight("C3") // 8 m
//	cfl, _ := fbp.DefaultCrownFuelLoad("C3")   // 1.15 kg/m²
//	cfb := fbp.CrownFractionBurned(fbp.Crown{FMC: fmc, SFC: sfc, CBH: cbh, CFL: cfl, SurfaceROS: rss})
//
// and a caller who wants cffdrs' fbp() exactly — a supplied value where it is in
// range, the published one where it is not — calls CrownBaseHeight and
// CrownFuelLoad, which are crown_base_height() and crown_fuel_load() ported.
//
// # The zeros are the point of the CFL table
//
// Six fuels — D1, S1, S2, S3, O1A, O1B — have no crown, and the published table
// says so with a crown fuel load of 0 (and a crown base height of 0, which is
// not a height but the same statement). CrownFractionBurned's CFL > 0 gate is
// what turns that 0 into "this fire never crowns", exactly as fbp()'s
// CFB <- ifelse(CFL > 0, CFB, 0) does. A caller using this lookup gets that
// behaviour for free; a caller supplying their own CFL for those fuels is
// overriding it, and may mean to.
//
// # The C6 stand equation
//
// For C6 — conifer plantation — cffdrs replaces the table entry with a linear
// function of stand density SD (stems/ha) and stand height SH (m) whenever both
// are positive: CBH = -11.2 + 1.06·SH + 0.0017·SD, floored at 1e-7 m when it
// goes negative. It is GLC-X-10's eq. 91 (Wotton, Alexander & Taylor 2009),
// which takes it from McAlpine & Hobbs (1994): height to live crown base in RED
// PINE plantations from stand height and density. GLC-X-10 prints 0.00170 for
// the density coefficient, which is cffdrs' 0.0017. The red-pine provenance is
// worth knowing before applying it to a C6 stand of another species. See
// C6StandCrownBaseHeight.

// crownBaseHeightTable is cffdrs' CBHs vector (crown_base_height.r), in metres.
var crownBaseHeightTable = map[string]float64{
	"C1": 2, "C2": 3, "C3": 8, "C4": 4, "C5": 18, "C6": 7, "C7": 10,
	"D1": 0, "M1": 6, "M2": 6, "M3": 6, "M4": 6,
	"S1": 0, "S2": 0, "S3": 0, "O1A": 0, "O1B": 0,
}

// crownFuelLoadTable is cffdrs' CFLs vector (crown_fuel_load.r), in kg/m².
var crownFuelLoadTable = map[string]float64{
	"C1": 0.75, "C2": 0.8, "C3": 1.15, "C4": 1.2, "C5": 1.2, "C6": 1.8, "C7": 0.5,
	"D1": 0, "M1": 0.8, "M2": 0.8, "M3": 0.8, "M4": 0.8,
	"S1": 0, "S2": 0, "S3": 0, "O1A": 0, "O1B": 0,
}

// The ranges cffdrs honours a supplied value in. Outside them fbp() discards the
// value and substitutes — these are the bounds CrownBaseHeight and CrownFuelLoad
// reproduce, and the reason the fixture's sentinel rows send -1.
const (
	MaxSuppliedCrownBaseHeightM  = 50.0 // crown_base_height: CBH in (0, 50]
	MaxSuppliedCrownFuelLoadKgM2 = 2.0  // crown_fuel_load: CFL in (0, 2]

	// fbp()'s screens on the C6 stand inputs: values outside these become -999
	// before crown_base_height sees them, which sends C6 back to the table.
	MaxStandDensityStemsHa = 1e5
	MaxStandHeightM        = 100.0
)

// DefaultCrownBaseHeight is the published crown base height in metres for a
// fuel type (cffdrs' crown_base_height table). The bool is false for a code
// this package does not implement, and the height is then 0.
//
// The value is 0 for the six fuels with no crown; that is a statement about the
// fuel, not a height, and CriticalSurfaceIntensity floors it at
// MinCrownBaseHeightM. It is the CFL table that keeps those fuels from crowning.
func DefaultCrownBaseHeight(code string) (float64, bool) {
	canonical, ok := fuel.CanonicalFuelCode(code)
	if !ok {
		return 0, false
	}
	return crownBaseHeightTable[canonical], true
}

// DefaultCrownFuelLoad is the published crown fuel load in kg/m² for a fuel type
// (cffdrs' crown_fuel_load table). The bool is false for a code this package
// does not implement, and the load is then 0 — the value that cannot crown.
func DefaultCrownFuelLoad(code string) (float64, bool) {
	canonical, ok := fuel.CanonicalFuelCode(code)
	if !ok {
		return 0, false
	}
	return crownFuelLoadTable[canonical], true
}

// C6StandCrownBaseHeight is GLC-X-10 eq. 91 (McAlpine & Hobbs 1994, fitted to
// red pine plantations): the crown base height of a C6 plantation from its
// stand density (stems/ha) and stand height (m), -11.2 + 1.06·SH + 0.0017·SD,
// floored at MinCrownBaseHeightM when negative as cffdrs floors it.
//
// It applies no range screen of its own; CrownBaseHeight is what decides
// whether a stand's SD and SH are used at all. A NaN input returns NaN.
func C6StandCrownBaseHeight(sdStemsHa, shM float64) float64 {
	cbh := -11.2 + 1.06*shM + 0.0017*sdStemsHa
	if cbh < 0 {
		return MinCrownBaseHeightM
	}
	return cbh
}

// CrownBaseHeight is cffdrs' crown_base_height() as fbp() calls it: the CBH in
// metres that fbp() would actually use.
//
// A supplied cbh in (0, 50] is returned unchanged. Anything else — non-positive,
// above 50, or NaN — is replaced: for C6 with a usable stand density in
// (0, 1e5] and stand height in (0, 100], by C6StandCrownBaseHeight; otherwise by
// DefaultCrownBaseHeight. Pass sd = sh = 0 when there is no stand data, which is
// fbp()'s own default. Every comparison against NaN is false, so a NaN cbh,
// sd or sh falls through exactly as cffdrs' is.na() branches send it. The SD and
// SH screens are fbp()'s (it maps out-of-range
// values to -999 before the call), folded in here so that a stand height of 150
// m falls back to the table as it does upstream rather than producing a 148 m
// crown base.
//
// An unknown fuel code returns the supplied value when it is in range and 0
// otherwise, matching DefaultCrownBaseHeight.
func CrownBaseHeight(code string, cbh, sdStemsHa, shM float64) float64 {
	if cbh > 0 && cbh <= MaxSuppliedCrownBaseHeightM {
		return cbh
	}
	canonical, _ := fuel.CanonicalFuelCode(code)
	if canonical == "C6" &&
		sdStemsHa > 0 && sdStemsHa <= MaxStandDensityStemsHa &&
		shM > 0 && shM <= MaxStandHeightM {
		return C6StandCrownBaseHeight(sdStemsHa, shM)
	}
	table, _ := DefaultCrownBaseHeight(code)
	return table
}

// CrownFuelLoad is cffdrs' crown_fuel_load() as fbp() calls it: the CFL in kg/m²
// that fbp() would actually use. A supplied cfl in (0, 2] is returned unchanged;
// anything else — non-positive, above 2, or NaN — is replaced by
// DefaultCrownFuelLoad.
//
// Note the asymmetry with Crown: Crown takes a CFL of 0 to mean "cannot crown",
// where this function takes it to mean "not supplied". That is cffdrs' reading,
// and it is why this is a function a caller opts into rather than a rule Crown
// applies.
func CrownFuelLoad(code string, cfl float64) float64 {
	if cfl > 0 && cfl <= MaxSuppliedCrownFuelLoadKgM2 {
		return cfl
	}
	table, _ := DefaultCrownFuelLoad(code)
	return table
}
