package fbp

import "math"

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

// Fuels is the ST-X-3 coefficient table. C = conifer, D = deciduous,
// M = mixedwood, S = slash, O = open/grass. M1/M2 carry no RSI coefficients of
// their own — their RSI is a percent-conifer blend of C2 and D1 (see RSI).
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
}

// Slope saturation, ST-X-3 eq. 39: SF reaches 10 at 70% rise and is held there.
// The formula evaluates to 10.0024 at exactly 70%, so the cap is continuous.
const (
	SlopeCapPct    = 70.0
	SlopeCapFactor = 10.0
)

func rsiBase(f Fuel, isi float64) float64 {
	return f.A * math.Pow(1-math.Exp(-f.B*isi), f.C)
}

// Curing-factor breakpoint. Below it CF is ST-X-3 eq. 35's exponential; at and
// above it CF is linear, the revision in Wotton, Alexander & Taylor (2009). The
// two branches meet continuously: eq. 35 evaluates to 0.17561 at 58.8, against
// the linear branch's 0.176.
const (
	CuringBreakpointPct = 58.8
	CuringBreakpointCF  = 0.176
	CuringLinearSlope   = 0.02
)

// CuringFactor is the grass curing coefficient CF for the O1 fuels.
//
// The linear branch is not optional decoration: with only eq. 35 this returns
// 2.224 at 100 % curing where the reference implementation returns 1.000, so a
// fully cured grass cell spread 2.2x too fast. TestCFFDRSSurfaceROS found that,
// and nothing short of an authoritative oracle could have: the error came from
// the 1992 source itself, so any second transcription of it agreed precisely in
// being wrong.
func CuringFactor(curingPct float64) float64 {
	if curingPct < CuringBreakpointPct {
		return 0.005 * (math.Exp(0.061*curingPct) - 1)
	}
	return CuringBreakpointCF + CuringLinearSlope*(curingPct-CuringBreakpointPct)
}

// RSI is the initial spread rate for a fuel type at a given ISI, in m/min.
// pc is percent conifer, used only by the M1/M2 blends; curingPct applies only
// to the O1 grass fuels.
func RSI(code string, isi, pc, curingPct float64) float64 {
	if code == "M1" || code == "M2" {
		c2 := rsiBase(Fuels["C2"], isi)
		d1 := rsiBase(Fuels["D1"], isi)
		w := pc / 100
		if code == "M1" {
			return w*c2 + (1-w)*d1
		}
		// M2's dead-fir component contributes at 20%.
		return w*c2 + 0.2*(1-w)*d1
	}
	f, ok := Fuels[code]
	if !ok {
		return 0
	}
	base := rsiBase(f, isi)
	if code == "O1A" || code == "O1B" {
		return base * CuringFactor(curingPct)
	}
	return base
}

// BuildupEffect is BE = exp(50·ln(q)·(1/BUI − 1/BUI0)) (ST-X-3 eq. 54). It is
// exactly 1.0 at BUI == BUI0, and 1.0 for the grass fuels (q = 1) and BUI <= 0.
func BuildupEffect(code string, bui float64) float64 {
	f, ok := Fuels[code]
	if !ok || bui <= 0 || f.BUI0 <= 0 || f.Q >= 1.0 {
		return 1
	}
	return math.Exp(50 * math.Log(f.Q) * (1/bui - 1/f.BUI0))
}

// SlopePercentFromDegrees converts a slope in degrees to percent rise, the unit
// FBP's equations expect. 45° is 100%, not 45% — the surface package stores
// degrees, so this conversion is where that mismatch is handled.
func SlopePercentFromDegrees(slopeDeg float64) float64 {
	return 100 * math.Tan(slopeDeg*math.Pi/180)
}

// SlopeFactor is SF = exp(3.533·(GS/100)^1.2), capped at 10 for slopes at or
// above 70% rise (ST-X-3 eq. 39). slopePct is percent rise. Downslope is not
// modelled: a head fire runs uphill, so negative slope is treated as flat.
func SlopeFactor(slopePct float64) float64 {
	if slopePct <= 0 {
		return 1
	}
	if slopePct >= SlopeCapPct {
		return SlopeCapFactor
	}
	return math.Exp(3.533 * math.Pow(slopePct/100, 1.2))
}

// ROS is head-fire rate of spread in m/min: RSI(fuel, ISI) · BE(fuel, BUI) ·
// SF(slope).
//
// This is NOT the published slope path. Slope enters here as a multiplier, so it
// applies whole regardless of wind direction, and the result is an upper bound
// wherever wind is not blowing uphill (median 2.98x, worst 99x with wind opposing
// the slope; see the package doc and TestCFFDRSSlopeDivergence). It never
// under-estimates.
//
// Use it only when FFMC or wind is unavailable and the back-solve therefore
// cannot run. A caller with all four inputs should take WSV from
// NetEffectiveWind, feed ISI(FFMC, WSV) in here, and pass slopePct = 0 so the
// slope is not counted twice — it is already inside WSV.
func ROS(code string, isi, bui, pc, curingPct, slopePct float64) float64 {
	return RSI(code, isi, pc, curingPct) * BuildupEffect(code, bui) * SlopeFactor(slopePct)
}

// ffmcMoistureScale converts FFMC to fine-fuel moisture content (Van Wagner
// 1987, FWI System eq. 10). It is 250·59.5/101 exactly, NOT the 147.2 that most
// printings of the equation round it to.
//
// The difference is not cosmetic. At 147.2 this package's ISI is off from cffdrs
// by up to 1.0e-3 relative — small enough to read as rounding, and systematic
// per FFMC, which is the signature of a wrong fF rather than noise. Every
// back-solved equivalent wind would inherit that bias in the same direction.
// Nothing here computed ISI before the back-solve landed — it arrived as an
// input — so
// no test could have seen it; TestCFFDRSInitialSpreadIndex is what pins it, and
// it reproduces the fixture's isi column to 0 relative error with this constant
// against 1.04e-3 with 147.2.
const ffmcMoistureScale = 250 * 59.5 / 101

// HighWindKmh is where FBP leaves the FWI System's exponential wind function for
// its own bounded one (cffdrs' fbpMod branch). Below it ISI is the published FWI
// ISI; at and above it FBP substitutes fW = 12·(1 − exp(−0.0818·(ws − 28))),
// which saturates instead of growing without bound.
//
// The two branches nearly but do not exactly meet: inverting the low branch at
// 40 km/h returns 39.9953, a 0.024 % step. That discontinuity is in the published
// system. Do not smooth it.
const HighWindKmh = 40.0

// EquivalentWindCapKmh is where the inverse of the high-wind branch asymptotes
// (ST-X-3 eq. 47). A slope steep and dry enough to imply a larger equivalent wind
// is reported as this.
const EquivalentWindCapKmh = 112.45

// isfClampMin is cffdrs' guard on inverting RSI: the term 1 − (RSF/a)^(1/c) is
// held at this floor rather than allowed to go non-positive.
//
// It looks like defensive padding and is not. RSF exceeds the RSI asymptote a
// whenever the slope is steep and the fuel dry, and that is not a rare corner:
// at FFMC 95 it fires for most fuels above roughly 60 % rise (about 31°), which
// is an ordinary August afternoon on a steep dry ridge. Without the floor the
// term goes negative, math.Log returns NaN, and that NaN propagates to the
// caller. A caller that reads NaN as no-data gets a hole in its output sited
// exactly on the steepest, driest ground — where the answer matters most. TestEquivalentWindIsNonNegativeAndFiniteEverywhere is the
// guard, and TestISFClampIsReachable is what keeps this paragraph honest.
//
// Because it binds, the back-solve returns LESS than RSI · BE · SF there — the
// zero-wind identity holds only where neither this nor EquivalentWindCapKmh is
// active. TestZeroWindRecoversRSIxBExSF carries the exceptions.
//
// A consequence worth stating before someone files it as a bug: above the clamp
// ISF no longer depends on FFMC, so the equivalent wind DECREASES as FFMC rises
// (112.45 km/h at FFMC 95 against 59.04 at 99, for C2). That is cffdrs'
// behaviour. Reproducing it is the point of this package.
const isfClampMin = 0.01

// ISI is the Initial Spread Index for a given FFMC and 10-m open wind speed in
// km/h (note the unit) — FWI System eqs. 24-26, with the FBP System's high-wind wind
// function (cffdrs .ISIcalc with fbpMod = TRUE, which fbp() always uses).
//
// Wind is km/h because ST-X-3 is. Converting from m/s is the caller's job: this
// package speaks the published system's units and nothing else.
//
// It is here for two reasons the package did not previously have. First, the
// equivalent-wind back-solve is defined as this function's inverse, so the two
// must live together or they drift. Second, it lets a caller anchored on somebody
// else's published ISI express "the same ISI, but at a different wind" as the
// ratio ISI(ffmc, w2)/ISI(ffmc, w1) — in which fF cancels exactly, so the ratio
// carries none of this function's fine-fuel-moisture error at all.
//
// Returns 0 for ffmc outside (0, 101]: above 101 the moisture content goes
// negative and m^5.31 is NaN, and a NaN escaping here would propagate silently
// into the caller's output.
func ISI(ffmc, windKmh float64) float64 {
	if ffmc <= 0 || ffmc > 101 {
		return 0
	}
	return 0.208 * windFunction(windKmh) * fineFuelMoistureFunction(ffmc)
}

// fineFuelMoistureFunction is fF (FWI System eq. 25), the fuel-moisture half of
// ISI. Split out because the back-solve needs it on its own: WSE is derived by
// dividing ISF through by 0.208·fF.
func fineFuelMoistureFunction(ffmc float64) float64 {
	m := ffmcMoistureScale * (101 - ffmc) / (59.5 + ffmc)
	return 91.9 * math.Exp(-0.1386*m) * (1 + math.Pow(m, 5.31)/4.93e7)
}

// windFunction is fW (FWI System eq. 24 with FBP's high-wind extension), the
// wind half of ISI. See HighWindKmh for the branch.
func windFunction(windKmh float64) float64 {
	if windKmh < HighWindKmh {
		return math.Exp(0.05039 * windKmh)
	}
	return 12 * (1 - math.Exp(-0.0818*(windKmh-28)))
}
