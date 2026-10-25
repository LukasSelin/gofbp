package fbp

import (
	"math"
	"testing"
)

// The acceleration model, without the fixture: eq. 72's two arms and its
// minimum, the boundary values of eqs. 70, 71 and 81, the integral relation
// between them, and the input eq. 70 has no meaning for.

func TestAccelerationAlpha(t *testing.T) {
	for _, code := range []string{"C1", "D1", "S1", "S2", "S3", "O1A", "O1B", "o1a"} {
		for _, cfb := range []float64{0, 0.3125, 1} {
			if got := AccelerationAlpha(code, cfb); got != OpenFuelAcceleration {
				t.Errorf("%s cfb=%v: alpha = %v, want the open-fuel constant 0.115", code, cfb, got)
			}
		}
	}
	// Closed canopy: 0.115 at CFB 0, minimum at CFB 2.5/8, back up towards 0.115.
	if got := AccelerationAlpha("C2", 0); got != OpenFuelAcceleration {
		t.Errorf("C2 cfb=0: alpha = %v, want 0.115", got)
	}
	min := AccelerationAlpha("C2", 0.3125)
	if !closeEnough(min, 0.115-18.8*math.Pow(0.3125, 2.5)*math.Exp(-2.5), 1e-15) || !(min > 0.0307 && min < 0.0308) {
		t.Errorf("C2 alpha at CFB 0.3125 = %v, want ≈ 0.03075", min)
	}
	for _, cfb := range []float64{0.05, 0.2, 0.3, 0.32, 0.5, 0.9, 1} {
		a := AccelerationAlpha("C2", cfb)
		if !(a >= min && a <= OpenFuelAcceleration) {
			t.Errorf("C2 alpha at CFB %v = %v, outside [%v, 0.115]", cfb, a, min)
		}
	}
	// Unknown fuels take the closed-canopy form, as cffdrs' ifelse does.
	if got, want := AccelerationAlpha("D2", 0.5), AccelerationAlpha("C2", 0.5); got != want {
		t.Errorf("D2 alpha = %v, want the closed form %v", got, want)
	}
}

func TestAccelerationAtIgnitionAndEquilibrium(t *testing.T) {
	for _, code := range []string{"C1", "C2", "M3"} {
		for _, cfb := range []float64{0, 0.3125, 0.95} {
			if got := ROSAtTime(code, 20, 0, cfb); got != 0 {
				t.Errorf("%s: ROS at ignition = %v, want 0", code, got)
			}
			if got := DistanceAtTime(code, 20, 0, cfb); got != 0 {
				t.Errorf("%s: distance at ignition = %v, want 0", code, got)
			}
			if got := LengthToBreadthAtTime(code, 3.5, 0, cfb); got != 1 {
				t.Errorf("%s: LB at ignition = %v, want 1 (a circle)", code, got)
			}
			// Equilibrium: after 10 h every alpha has decayed to nothing.
			if got := ROSAtTime(code, 20, 600, cfb); !closeEnough(got, 20, 1e-6) {
				t.Errorf("%s: ROS after 10 h = %v, want 20", code, got)
			}
			if got := LengthToBreadthAtTime(code, 3.5, 600, cfb); !closeEnough(got, 3.5, 1e-6) {
				t.Errorf("%s: LB after 10 h = %v, want 3.5", code, got)
			}
			// ...and the distance lags a full-speed start by rosEq/alpha.
			a := AccelerationAlpha(code, cfb)
			if got, want := DistanceAtTime(code, 20, 600, cfb), 20*(600-1/a); !closeEnough(got, want, 1e-9) {
				t.Errorf("%s: distance after 10 h = %v, want %v", code, got, want)
			}
		}
	}
	// 0.115 per minute is ~90 % of equilibrium after 20 minutes.
	if got := ROSAtTime("C1", 1, 20, 0); !(got > 0.89 && got < 0.91) {
		t.Errorf("C1 after 20 min: %v of equilibrium, want ≈ 0.90", got)
	}
}

// Eq. 71 is eq. 70 integrated: the distance's derivative is the rate.
func TestDistanceIsTheIntegralOfRate(t *testing.T) {
	const h = 1e-4
	for _, cfb := range []float64{0, 0.3, 0.9} {
		for _, tMin := range []float64{1, 5, 30, 120} {
			slope := (DistanceAtTime("C3", 12, tMin+h, cfb) - DistanceAtTime("C3", 12, tMin-h, cfb)) / (2 * h)
			if rate := ROSAtTime("C3", 12, tMin, cfb); !closeEnough(slope, rate, 1e-6) {
				t.Errorf("cfb=%v t=%v: dD/dt = %v, ROS(t) = %v", cfb, tMin, slope, rate)
			}
			if d := DistanceAtTime("C3", 12, tMin, cfb); !(d < 12*tMin) {
				t.Errorf("cfb=%v t=%v: distance %v is not behind a full-speed start %v", cfb, tMin, d, 12*tMin)
			}
		}
	}
}

// A slower alpha means a slower approach: the intermittent crown fire lags the
// surface fire and the continuous one at every time.
func TestIntermittentCrownFireAcceleratesSlowest(t *testing.T) {
	for _, tMin := range []float64{2, 10, 30, 60} {
		surface := ROSAtTime("C2", 1, tMin, 0)
		intermittent := ROSAtTime("C2", 1, tMin, 0.3125)
		continuous := ROSAtTime("C2", 1, tMin, 1)
		if !(intermittent < surface && intermittent < continuous) {
			t.Errorf("t=%v: fraction of equilibrium surface %v, intermittent %v, continuous %v",
				tMin, surface, intermittent, continuous)
		}
	}
}

func TestAccelerationRejectsNegativeTime(t *testing.T) {
	for _, tMin := range []float64{-1, math.NaN()} {
		for name, got := range map[string]float64{
			"ROSAtTime":             ROSAtTime("C2", 10, tMin, 0.5),
			"DistanceAtTime":        DistanceAtTime("C2", 10, tMin, 0.5),
			"LengthToBreadthAtTime": LengthToBreadthAtTime("C2", 2, tMin, 0.5),
		} {
			if !math.IsNaN(got) {
				t.Errorf("%s at t=%v = %v, want NaN", name, tMin, got)
			}
		}
	}
}
