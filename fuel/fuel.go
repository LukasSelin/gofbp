package fuel

import (
	"math"
)

// Fuel holds one FBP fuel type's coefficients: RSI (a, b, c) and the buildup
// effect (q, bui0).
type Fuel struct {
	Code string
	A    float64
	B    float64
	C    float64
	Q    float64
	BUI0 float64
}

// Fuels is the coefficient table: ST-X-3's Tables 6-7, plus M3 and M4 from
// Wotton, Alexander & Taylor (2009). C = conifer, D = deciduous, M = mixedwood,
// S = slash, O = open/grass.
//
// The two mixedwood pairs are not alike, and the NaNs are how that is stated.
// M1/M2 carry no RSI coefficients of their own at all — their RSI is a
// percent-conifer blend of the C2 and D1 curves, so an a, b or c for M1 would be
// a number with nothing behind it, and NaN is what makes reading one an obvious
// failure rather than a plausible one. M3/M4 do have their own curves (eq. 30),
// which the blend then weights against D1 by percent dead balsam fir. So
// rsiBase(Fuels["M3"], isi) is a real quantity — the pure dead-fir component —
// while rsiBase(Fuels["M1"], isi) is NaN by construction. Neither is the fuel's
// RSI; both mixedwood pairs get their RSI from the blends in RSI.
var Fuels = map[string]Fuel{
	"C1":  {"C1", 90, 0.0649, 4.5, 0.90, 72},
	"C2":  {"C2", 110, 0.0282, 1.5, 0.70, 64},
	"C3":  {"C3", 110, 0.0444, 3.0, 0.75, 62},
	"C4":  {"C4", 110, 0.0293, 1.5, 0.80, 66},
	"C5":  {"C5", 30, 0.0697, 4.0, 0.80, 56},
	"C6":  {"C6", 30, 0.0800, 3.0, 0.80, 62},
	"C7":  {"C7", 45, 0.0305, 2.0, 0.85, 106},
	"D1":  {"D1", 30, 0.0232, 1.6, 0.90, 32},
	"S1":  {"S1", 75, 0.0297, 1.3, 0.75, 38},
	"S2":  {"S2", 40, 0.0438, 1.7, 0.75, 63},
	"S3":  {"S3", 55, 0.0829, 3.2, 0.75, 31},
	"O1A": {"O1A", 190, 0.0310, 1.4, 1.00, 1},
	"O1B": {"O1B", 250, 0.0350, 1.7, 1.00, 1},
	"M1":  {"M1", math.NaN(), math.NaN(), math.NaN(), 0.80, 50},
	"M2":  {"M2", math.NaN(), math.NaN(), math.NaN(), 0.80, 50},
	// Wotton, Alexander & Taylor (2009) eq. 30. All four mixedwoods share
	// q = 0.80 and BUI0 = 50.
	"M3": {"M3", 120, 0.0572, 1.40, 0.80, 50},
	"M4": {"M4", 100, 0.0404, 1.48, 0.80, 50},
}

// maxFuelCodeLen bounds the fold buffer in CanonicalFuelCode. The longest key in
// Fuels is three characters; the headroom is for separators a caller's fold has
// to strip before the length is known.
const maxFuelCodeLen = 8

// CanonicalFuelCode folds a fuel code into the spelling Fuels is keyed by, and
// reports whether the result is a fuel this package actually implements.
//
// The fold is case-insensitive and drops spaces, hyphens and underscores, which
// is what cffdrs does to its FuelType column. So "O1a", "o1b", "C-2" and " C2 "
// reach the same coefficients as "O1A", "O1B" and "C2". That spelling latitude
// is not cosmetic: "O1a" and "O1b" are the spellings ST-X-3 itself prints, and a
// fuel raster labelled the way the source document labels it must not read as a
// different fuel from one labelled the way this table is keyed.
//
// The second return is the part worth calling this for. Every other function
// here takes a fuel code and returns a float64, so a code this package does not
// implement can only come back as a spread rate of 0 — which is
// indistinguishable from a cell that genuinely will not carry fire. Two kinds of
// input land there, and both are ordinary:
//
//   - A typo, or a fuel column from a source whose class names are not FBP's.
//   - D2, and cffdrs' non-fuel classes WA and NF. D2 is a real fuel in the
//     published system that this package has no coefficients for, so a stand
//     mapping to it is not a stand that does not burn.
//
// Call this at the boundary where fuel codes enter — once per class, not once
// per cell — and decide there what an unimplemented fuel means for your output.
// Leaving it to the numbers means the answer is 0 and the reason is gone.
func CanonicalFuelCode(code string) (string, bool) {
	// The already-canonical path is the hot one, and it neither folds nor
	// allocates.
	if _, ok := Fuels[code]; ok {
		return code, true
	}
	var buf [maxFuelCodeLen]byte
	n := 0
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == ' ' || c == '\t' || c == '-' || c == '_':
			continue
		case c >= 'a' && c <= 'z':
			c -= 'a' - 'A'
		}
		if n == len(buf) {
			return "", false // longer than any fuel code; it cannot match one
		}
		buf[n] = c
		n++
	}
	canonical := string(buf[:n])
	_, ok := Fuels[canonical]
	return canonical, ok
}
