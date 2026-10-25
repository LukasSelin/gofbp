package spread

import (
	"testing"

	"github.com/LukasSelin/gofbp/crown"
	"github.com/LukasSelin/gofbp/fuel"
	"github.com/LukasSelin/gofbp/internal/cffdrstest"
)

// The acceleration model against the oracle.
//
// Oracle case (b): fbp() gates every time-dependent column on ACCEL, and until
// the generator's acceleration block every row sent ACCEL = 0, so HROSt, BROSt,
// FROSt, LBt, DH and DB all carried equilibrium values. The block sends ACCEL = 1
// across all 17 fuels, seven elapsed times from ignition to 6 h, and weather and
// crown base heights that put CFB at 0, intermediate and near 1 — which is what
// reaches both arms of eq. 72 and the non-monotone middle of the closed one.
//
// Each test reads the oracle's own equilibrium quantities (ros, bros, lb, cfb)
// and hr, so a failure is in eqs. 70/71/72/81 and nothing upstream of them. The
// HEAD fire's CFB drives alpha for every rate, back included, because that is
// what fbp() does — see acceleration.go.

// accelCases are the rows that exercise the model, with fbp()'s hours converted
// to the minutes the model runs in.
func accelCases(t *testing.T) ([]cffdrstest.Case, []float64) {
	t.Helper()
	f := cffdrstest.Load(t)
	var cs []cffdrstest.Case
	var mins []float64
	for _, c := range f.Cases {
		if c.Accel == 1 {
			cs = append(cs, c)
			mins = append(mins, c.HR*60)
		}
	}
	if len(cs) == 0 {
		t.Fatal("the fixture has no accel = 1 rows: it predates the acceleration block " +
			"(testdata/README.md lists it among the earlier digests). Regenerate it.")
	}
	return cs, mins
}

// accelCoverage fails unless every branch the model has is reached: both alpha
// forms, CFB at 0, intermediate and continuous, and the ignition instant.
func accelCoverage(t *testing.T, cs []cffdrstest.Case) {
	t.Helper()
	var open, closedZero, closedMid, closedHigh, ignition int
	fuels := map[string]bool{}
	for _, c := range cs {
		fuelCode := cffdrstest.Fuel(c.Fuel)
		fuels[fuelCode] = true
		canonical, _ := fuel.CanonicalFuelCode(fuelCode)
		switch {
		case isOpenFuel(canonical):
			open++
		case c.CFB == 0:
			closedZero++
		case c.CFB < crown.ContinuousCrownCFB:
			closedMid++
		default:
			closedHigh++
		}
		if c.HR == 0 {
			ignition++
		}
	}
	t.Logf("  %d rows: %d open-fuel, closed-canopy %d at CFB 0 / %d intermediate / %d continuous, %d at ignition, %d fuels",
		len(cs), open, closedZero, closedMid, closedHigh, ignition, len(fuels))
	if open == 0 || closedZero == 0 || closedMid == 0 || closedHigh == 0 || ignition == 0 {
		t.Error("a branch of eq. 72 or the ignition instant is unreached — the acceleration block has been narrowed")
	}
	if len(fuels) < len(fuel.Fuels) {
		t.Errorf("only %d of %d fuels reached", len(fuels), len(fuel.Fuels))
	}
}

// ledger: rate_of_spread_at_time.r
func TestCFFDRSRateOfSpreadAtTime(t *testing.T) {
	cs, mins := accelCases(t)
	accelCoverage(t, cs)
	const tol = 1e-9
	bad := 0
	var worstH, worstB, worstF float64
	for i, c := range cs {
		fuelCode := cffdrstest.Fuel(c.Fuel)
		h := ROSAtTime(fuelCode, c.ROS, mins[i], c.CFB)
		b := ROSAtTime(fuelCode, c.BROS, mins[i], c.CFB)
		// FROSt is fbp()'s composition, FlankROS of the two accelerated rates on
		// the accelerated ellipse — asserted because it is the one place the
		// three functions meet, and a caller building it needs the head CFB in all
		// three.
		fr := FlankROS(h, b, LengthToBreadthAtTime(fuelCode, c.LB, mins[i], c.CFB))
		worstH = cffdrstest.MaxF(worstH, cffdrstest.RelErr(h, c.HROSt))
		worstB = cffdrstest.MaxF(worstB, cffdrstest.RelErr(b, c.BROSt))
		worstF = cffdrstest.MaxF(worstF, cffdrstest.RelErr(fr, c.FROSt))
		if !cffdrstest.CloseEnough(h, c.HROSt, tol) || !cffdrstest.CloseEnough(b, c.BROSt, tol) || !cffdrstest.CloseEnough(fr, c.FROSt, tol) {
			if bad++; bad <= 10 {
				t.Errorf("%s hr=%v cfb=%v ros=%v bros=%v: HROSt/BROSt/FROSt = %v / %v / %v, cffdrs %v / %v / %v",
					c.Fuel, c.HR, c.CFB, c.ROS, c.BROS, h, b, fr, c.HROSt, c.BROSt, c.FROSt)
			}
		}
	}
	t.Logf("ROSt %5d rows, %d mismatched, worst relative error head %.3g back %.3g flank %.3g",
		len(cs), bad, worstH, worstB, worstF)
}

// ledger: distance_at_time.r
func TestCFFDRSDistanceAtTime(t *testing.T) {
	cs, mins := accelCases(t)
	accelCoverage(t, cs)
	const tol = 1e-9
	bad := 0
	var worstH, worstB float64
	for i, c := range cs {
		fuelCode := cffdrstest.Fuel(c.Fuel)
		dh := DistanceAtTime(fuelCode, c.ROS, mins[i], c.CFB)
		db := DistanceAtTime(fuelCode, c.BROS, mins[i], c.CFB)
		worstH = cffdrstest.MaxF(worstH, cffdrstest.RelErr(dh, c.DH))
		worstB = cffdrstest.MaxF(worstB, cffdrstest.RelErr(db, c.DB))
		if !cffdrstest.CloseEnough(dh, c.DH, tol) || !cffdrstest.CloseEnough(db, c.DB, tol) {
			if bad++; bad <= 10 {
				t.Errorf("%s hr=%v cfb=%v ros=%v bros=%v: DH/DB = %v / %v, cffdrs %v / %v",
					c.Fuel, c.HR, c.CFB, c.ROS, c.BROS, dh, db, c.DH, c.DB)
			}
		}
	}
	t.Logf("D(t) %5d rows, %d mismatched, worst relative error head %.3g back %.3g", len(cs), bad, worstH, worstB)
}

// ledger: length_to_breadth_at_time.r
func TestCFFDRSLengthToBreadthAtTime(t *testing.T) {
	cs, mins := accelCases(t)
	accelCoverage(t, cs)
	const tol = 1e-9
	bad := 0
	var worst float64
	for i, c := range cs {
		got := LengthToBreadthAtTime(cffdrstest.Fuel(c.Fuel), c.LB, mins[i], c.CFB)
		worst = cffdrstest.MaxF(worst, cffdrstest.RelErr(got, c.LBt))
		if !cffdrstest.CloseEnough(got, c.LBt, tol) {
			if bad++; bad <= 10 {
				t.Errorf("%s hr=%v cfb=%v lb=%v: LBt = %v, cffdrs %v", c.Fuel, c.HR, c.CFB, c.LB, got, c.LBt)
			}
		}
	}
	t.Logf("LBt  %5d rows, %d mismatched, worst relative error %.3g", len(cs), bad, worst)
}
