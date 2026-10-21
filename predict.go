package fbp

import "math"

// THE DRIVER: one call that composes every piece of this package the way
// cffdrs' fbp() composes them, for one stand under one set of conditions.
//
// Everything Predict computes, the rest of the package already exposes. What
// the driver adds is the composition — the right rate fed to the right
// function, C6 routed through its crown path, the head CFB used for every
// accelerated rate — and the handful of quantities fbp() computes inline rather
// than in a function of its own: the head azimuth RAZ, the flank and back crown
// fractions with the consumptions and intensities built on them, the flank
// spread distance DF, and the times to crown-fire initiation TI/FTI/BTI.
//
// # What it will not do: default anything
//
// fbp() can be called with no input at all and returns a complete prediction,
// because it quietly defaults every input it is missing and replaces every input
// it does not like: FFMC 90, BUI 60, PC 50, a grass fuel load of 0.35 kg/m², a
// curing of 95 % in place of 0, a crown base height and load from a per-fuel
// table, a foliar moisture content from a default latitude. That is the local
// judgement this package exists to refuse, so Predict refuses it too:
//
//   - An input fbp() would silently replace is an error here.
//   - The crown inputs come from exactly one source, and the caller names it.
//     Input.Crown has no usable zero value: CrownAsGiven uses CBH and CFL as
//     written, CrownCFFDRS opts into CrownBaseHeight/CrownFuelLoad — fbp()'s own
//     rule, a supplied value where it is in range and the published table where
//     it is not. Leaving it unset is an error, not a choice.
//   - Foliar moisture is Input.FMC, or — only when the caller hands it a Site —
//     FoliarMoistureContent of that site, which is again fbp()'s rule.
//   - A grass fuel needs Input.GrassFuelLoad. There is no 0.35.
//
// So the only lookups Predict ever applies are the opt-in ones, and only when
// the input asks for them by name.
//
// # What is not here
//
//   - C6 on sloped ground. cffdrs back-solves the slope-equivalent wind from
//     C6's BLENDED rate, which this package does not reproduce (MIGRATION.md's
//     Slopecalc.r row). Predict returns ErrC6OnSlope rather than a number that
//     disagrees with cffdrs for a reason the caller cannot see.
//   - The rate at a bearing (fbp()'s TROS, TROSt, TCFB, TFI, TTFC, TTI). Both
//     cffdrs quantities at a bearing are numerically defective; see
//     ROSAtAngle, which a caller can apply to this prediction's head, flank and
//     back rates.
//   - fbp()'s ISI override (a supplied ISI in place of FFMC and wind), its
//     BUIEFF switch, and its batching (fbp.r's chunking and `cores`).
//
// # Units and directions
//
// The package's own: km/h, percent rise, m/min, metres, kg/m², kW/m, minutes
// since ignition, and azimuths in degrees clockwise from true north that point
// TOWARDS — where the wind blows to, and the upslope direction. fbp() takes the
// opposite for both (WD is where the wind comes from, ASPECT the downslope
// face) and adds 180° internally; a caller porting an fbp() input adds it once,
// at the boundary.

// CrownSource names where Predict takes crown base height and crown fuel load
// from. Its zero value is deliberately not one of the choices.
type CrownSource int

const (
	// CrownUnset is the zero value, and Predict rejects it: the source of a
	// stand's canopy is the caller's decision to make, not this package's.
	CrownUnset CrownSource = iota
	// CrownAsGiven uses Input.CBH and Input.CFL exactly as written. A CFL of 0
	// means the stand cannot crown.
	CrownAsGiven
	// CrownCFFDRS opts into fbp()'s rule: CrownBaseHeight(Fuel, CBH,
	// StandDensity, StandHeight) and CrownFuelLoad(Fuel, CFL) — the supplied
	// value where it is in range, and the published per-fuel table (or C6's
	// stand equation) where it is not. Pass CBH = CFL = -1 for the table.
	CrownCFFDRS
)

// Site is where and when, for computing foliar moisture content. It is used
// only when Input.FMC is not a usable value, and only because the caller set it.
type Site struct {
	Lat, Long float64 // degrees; west longitude may be negative, as fbp() accepts
	ElevM     float64 // metres; 0 or below selects eqs. 1-2 rather than 3-4
	DayOfYear float64 // Julian day, 1-366
	D0        float64 // date of minimum FMC, or 0 to compute it
}

// Input is one stand under one set of conditions.
type Input struct {
	Fuel string

	FFMC, BUI float64

	WindKmh           float64
	WindAzimuthDeg    float64 // TOWARDS — fbp()'s WD + 180
	SlopePct          float64
	UpslopeAzimuthDeg float64 // TOWARDS upslope — fbp()'s ASPECT + 180

	PC, PDF, CuringPct float64 // read only by the fuels they apply to
	GrassFuelLoad      float64 // kg/m², O1A/O1B only, and required for them

	// FMC is foliar moisture content in percent, used when in (0, 120]. Outside
	// that range it is computed from Site if one is given, and is an error if
	// not — for fuels with a crown. The crownless fuels (D1, S1-S3, O1A, O1B)
	// take 0, as fbp() forces.
	FMC  float64
	Site *Site

	Crown                     CrownSource
	CBH, CFL                  float64 // m, kg/m² — see CrownSource
	StandDensity, StandHeight float64 // stems/ha, m — C6 under CrownCFFDRS only

	// ElapsedMin is minutes since ignition. With PointIgnition the acceleration
	// model applies; without it the fire is at equilibrium throughout and the
	// distances are rate × time.
	ElapsedMin    float64
	PointIgnition bool
}

// Prediction is everything Predict computes. Fields are named for fbp()'s
// output columns; the units are the package's.
type Prediction struct {
	// Inputs as resolved: the values the equations actually used.
	FMC, CBH, CFL, SFC float64

	ISI, WSV, RAZ, BE, SF float64 // RAZ in [0, 360), towards

	// The head fire.
	ROS, CFB, CFC, TFC, HFI float64
	FD                      FireDescription
	CSI, RSO                float64

	// The ellipse at equilibrium, with the flank and back fires' own crowning.
	LB, BROS, FROS         float64
	FCFB, BCFB, FTFC, BTFC float64
	FFI, BFI               float64
	TI, FTI, BTI           float64 // minutes to crown-fire initiation

	// At ElapsedMin. Equal to the equilibrium values unless PointIgnition.
	HROSt, BROSt, FROSt, LBt float64
	DH, DB, DF               float64 // metres
}

// PredictError is every error Predict returns. Compare against the Err values
// below with errors.Is; the returned error carries the offending field or code
// as detail, and matches its sentinel.
//
// It is a type of its own rather than errors.New and fmt.Errorf because this
// package imports math and nothing else (TestPackageHasNoDependencies). The
// cost is that the message names the field rather than printing its value.
type PredictError struct {
	kind, detail string
}

func (e *PredictError) Error() string {
	if e.detail == "" {
		return "fbp: " + e.kind
	}
	return "fbp: " + e.kind + ": " + e.detail
}

// Is makes a detailed error match its sentinel under errors.Is.
func (e *PredictError) Is(target error) bool {
	t, ok := target.(*PredictError)
	return ok && t.detail == "" && t.kind == e.kind
}

// Errors Predict returns. Each one names an input fbp() would have replaced or
// a case this package does not reproduce.
var (
	ErrUnknownFuel  = &PredictError{kind: "fuel code is not implemented"}
	ErrC6OnSlope    = &PredictError{kind: "C6 on sloped ground is not reproduced (see MIGRATION.md, Slopecalc.r)"}
	ErrCrownUnset   = &PredictError{kind: "Input.Crown is unset; choose CrownAsGiven or CrownCFFDRS"}
	ErrNoFMC        = &PredictError{kind: "FMC is not in (0, 120] and no Site was given to compute it"}
	ErrNoGrassLoad  = &PredictError{kind: "O1A/O1B need a positive GrassFuelLoad"}
	ErrInvalidInput = &PredictError{kind: "input out of range"}
)

// invalid is ErrInvalidInput naming the field.
func invalid(name string) error {
	return &PredictError{kind: ErrInvalidInput.kind, detail: name}
}

func finiteIn(v, lo, hi float64) bool { return v >= lo && v <= hi && !math.IsInf(v, 0) }

// crownless are the fuels fbp() zeroes FMC for: they have no crown, and their
// published CFL of 0 keeps them from crowning.
func crownless(canonical string) bool {
	switch canonical {
	case "D1", "S1", "S2", "S3", "O1A", "O1B":
		return true
	}
	return false
}

func normAzimuth(deg float64) float64 {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}
	return deg
}

// Predict is the FBP System for one stand: cffdrs' fire_behaviour_prediction()
// with output = "All", less the outputs listed in the file comment, and with
// every default it applies replaced by an error or an opt-in. See the file
// comment, and Input for what each field means.
func Predict(in Input) (Prediction, error) {
	var p Prediction
	fuel, ok := CanonicalFuelCode(in.Fuel)
	if !ok {
		return p, &PredictError{kind: ErrUnknownFuel.kind, detail: `"` + in.Fuel + `"`}
	}

	// The screens fbp() applies by substitution, applied here by refusal.
	switch {
	case !finiteIn(in.FFMC, 0, 101):
		return p, invalid("FFMC")
	case !finiteIn(in.BUI, 0, math.MaxFloat64):
		return p, invalid("BUI")
	case !finiteIn(in.WindKmh, 0, math.MaxFloat64):
		return p, invalid("WindKmh")
	case !finiteIn(in.SlopePct, 0, math.MaxFloat64):
		return p, invalid("SlopePct")
	case !finiteIn(in.WindAzimuthDeg, -math.MaxFloat64, math.MaxFloat64):
		return p, invalid("WindAzimuthDeg")
	case !finiteIn(in.UpslopeAzimuthDeg, -math.MaxFloat64, math.MaxFloat64):
		return p, invalid("UpslopeAzimuthDeg")
	case !finiteIn(in.PC, 0, 100):
		return p, invalid("PC")
	case !finiteIn(in.PDF, 0, 100):
		return p, invalid("PDF")
	case !finiteIn(in.CuringPct, 0, 100):
		return p, invalid("CuringPct")
	case !finiteIn(in.ElapsedMin, 0, math.MaxFloat64):
		return p, invalid("ElapsedMin")
	}
	if fuel == "C6" && in.SlopePct > 0 {
		return p, ErrC6OnSlope
	}
	grass := fuel == "O1A" || fuel == "O1B"
	if grass && !finiteIn(in.GrassFuelLoad, math.SmallestNonzeroFloat64, math.MaxFloat64) {
		return p, ErrNoGrassLoad
	}

	// Crown inputs, from the one source the caller named.
	switch in.Crown {
	case CrownAsGiven:
		if !finiteIn(in.CBH, 0, math.MaxFloat64) {
			return p, invalid("CBH")
		}
		if !finiteIn(in.CFL, 0, math.MaxFloat64) {
			return p, invalid("CFL")
		}
		p.CBH, p.CFL = in.CBH, in.CFL
	case CrownCFFDRS:
		p.CBH = CrownBaseHeight(fuel, in.CBH, in.StandDensity, in.StandHeight)
		p.CFL = CrownFuelLoad(fuel, in.CFL)
	default:
		return p, ErrCrownUnset
	}

	// Foliar moisture: the caller's, or their site's; zero for the crownless.
	switch {
	case crownless(fuel):
		p.FMC = 0
	case in.FMC > 0 && in.FMC <= 120:
		p.FMC = in.FMC
	case in.Site != nil:
		s := in.Site
		p.FMC = FoliarMoistureContent(s.Lat, math.Abs(s.Long), s.ElevM, s.DayOfYear, s.D0)
		if !(p.FMC > 0) {
			return p, ErrNoFMC
		}
	default:
		return p, ErrNoFMC
	}

	p.SFC = SurfaceFuelConsumption(fuel, in.FFMC, in.BUI, in.PC, in.GrassFuelLoad)

	// Net effective wind. fbp() runs the back-solve only on a slope with a
	// positive FFMC, and otherwise takes the wind as it is — so does this.
	p.WSV, p.RAZ = in.WindKmh, in.WindAzimuthDeg
	if in.SlopePct > 0 && in.FFMC > 0 {
		p.WSV, p.RAZ = NetEffectiveWind(SlopeWind{
			Code: fuel, FFMC: in.FFMC, SlopePct: in.SlopePct,
			WindKmh: in.WindKmh, WindAzimuthDeg: in.WindAzimuthDeg,
			UpslopeAzimuthDeg: in.UpslopeAzimuthDeg,
			PC:                in.PC, PDF: in.PDF, CuringPct: in.CuringPct,
		})
	}
	p.RAZ = normAzimuth(p.RAZ)
	p.ISI = ISI(in.FFMC, p.WSV)
	p.BE = BuildupEffect(fuel, in.BUI)
	p.SF = SlopeFactor(in.SlopePct)

	// The head fire: RSS with the slope inside WSV, then C6's blend or the
	// threshold for everyone else.
	crown := Crown{FMC: p.FMC, SFC: p.SFC, CBH: p.CBH, CFL: p.CFL}
	// CSI and RSO are reported as fbp() reports them, which differs from
	// CriticalSurfaceIntensity at exactly one point: a CBH of exactly 0, the
	// published value for the six crownless fuels. fbp() evaluates eq. 56 there
	// and gets 0; CriticalSurfaceIntensity floors CBH at MinCrownBaseHeightM to
	// stay continuous and gets ~3e-10 kW/m. The floor is invisible in CSI, but TI
	// divides RSO by a rate that is itself near zero for those fuels in calm,
	// damp conditions, and there it turns 1e-13 m/min into a TI of minutes. The
	// crown fraction is unaffected either way: CFL is 0 for every one of those
	// fuels under the published table, and the CFL gate holds CFB at 0.
	p.CSI = CriticalSurfaceIntensity(p.FMC, p.CBH)
	if p.CBH == 0 {
		p.CSI = 0
	}
	p.RSO = CriticalSurfaceROS(p.CSI, p.SFC)
	headRate := func(isi float64) (ros, cfb float64) {
		c := crown
		c.SurfaceROS = RSI(fuel, isi, in.PC, in.PDF, in.CuringPct) * p.BE
		if fuel == "C6" {
			return C6CrownFire(c, isi)
		}
		return c.SurfaceROS, CrownFractionBurned(c)
	}
	p.ROS, p.CFB = headRate(p.ISI)
	p.FD = DescribeFire(p.CFB)
	p.CFC = CrownFuelConsumption(fuel, p.CFL, p.CFB, in.PC, in.PDF)
	p.TFC = TotalFuelConsumption(fuel, p.CFL, p.CFB, p.SFC, in.PC, in.PDF)
	p.HFI = FireIntensity(p.TFC, p.ROS)

	// The ellipse. BROS is the same rate function at the backing ISI (eqs.
	// 75-77) — C6's blend included, as in cffdrs' back_rate_of_spread.
	p.LB = LengthToBreadth(fuel, p.WSV)
	p.BROS, _ = headRate(p.ISI * BackISIRatio(p.WSV))
	p.FROS = FlankROS(p.ROS, p.BROS, p.LB)

	// The flank and back fires' own crowning: eq. 58 at their own rates against
	// the head's RSO. fbp() sets both to 0 for C6 rather than running C6's path
	// at those rates; that is reproduced, and is why C6's FFI/BFI carry no crown
	// fuel.
	sideCFB := func(ros float64) float64 {
		if !(p.CFL > 0) || fuel == "C6" {
			return 0
		}
		c := crown
		c.SurfaceROS = ros
		return CrownFractionBurned(c)
	}
	p.FCFB, p.BCFB = sideCFB(p.FROS), sideCFB(p.BROS)
	p.FTFC = TotalFuelConsumption(fuel, p.CFL, p.FCFB, p.SFC, in.PC, in.PDF)
	p.BTFC = TotalFuelConsumption(fuel, p.CFL, p.BCFB, p.SFC, in.PC, in.PDF)
	p.FFI = FireIntensity(p.FTFC, p.FROS)
	p.BFI = FireIntensity(p.BTFC, p.BROS)
	p.TI = timeToCrowning(p.RSO, p.ROS, p.CFB)
	p.FTI = timeToCrowning(p.RSO, p.FROS, p.FCFB)
	p.BTI = timeToCrowning(p.RSO, p.BROS, p.BCFB)

	// Time.
	t := in.ElapsedMin
	if in.PointIgnition {
		p.LBt = LengthToBreadthAtTime(fuel, p.LB, t, p.CFB)
		p.HROSt = ROSAtTime(fuel, p.ROS, t, p.CFB)
		p.BROSt = ROSAtTime(fuel, p.BROS, t, p.CFB)
		p.FROSt = FlankROS(p.HROSt, p.BROSt, p.LBt)
		p.DH = DistanceAtTime(fuel, p.ROS, t, p.CFB)
		p.DB = DistanceAtTime(fuel, p.BROS, t, p.CFB)
		p.DF = (p.DH + p.DB) / (2 * p.LBt)
	} else {
		p.LBt, p.HROSt, p.BROSt, p.FROSt = p.LB, p.ROS, p.BROS, p.FROS
		p.DH, p.DB = p.ROS*t, p.BROS*t
		p.DF = (p.DH + p.DB) / (2 * p.LB)
	}
	return p, nil
}

// timeToCrowning is fbp()'s TI: the minutes after a point ignition at which the
// accelerating rate first reaches RSO, ln(1 − RSO/ROS)/−α, or 0 when the
// equilibrium rate never gets there.
//
// It is the driver's inline code, ported as written, and it departs from eq. 72
// in one way worth knowing: α here is ALWAYS the closed-canopy form
// 0.115 − 18.8·CFB^2.5·e^(−8·CFB), with no open-fuel branch. That differs from
// AccelerationAlpha only for an open fuel with CFB > 0. Five of the seven open
// fuels have a published CFL of 0 and never get there; C1 does (CFL 0.75), so
// a crowning C1 stand's TI is computed with a different α from the one its
// ROSAtTime accelerates with. Reproduced, not corrected: TI is fbp()'s number.
func timeToCrowning(rso, ros, cfb float64) float64 {
	alpha := OpenFuelAcceleration - 18.8*math.Pow(cfb, 2.5)*math.Exp(-8*cfb)
	x := 1 - rso/ros
	if !(x > 0) {
		x = 1
	}
	return math.Log(x) / -alpha
}
