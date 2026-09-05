package fbp

import (
	"math"
	"testing"
)

func almostEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// curingPctForTest is an arbitrary in-range curing value. FBP takes curing as
// an input; choosing what to assume for a given landscape is the caller's
// business and deliberately not this package's.
const curingPctForTest = 80.0

func TestBuildupEffectIsOneAtBUI0(t *testing.T) {
	for code, f := range Fuels {
		if f.Q >= 1.0 {
			continue
		}
		if got := BuildupEffect(code, f.BUI0); !almostEqual(got, 1.0, 1e-12) {
			t.Errorf("BuildupEffect(%s, BUI0=%v) = %v, want 1", code, f.BUI0, got)
		}
	}
}

func TestGrassFuelsHaveNoBuildupEffect(t *testing.T) {
	for _, code := range []string{"O1A", "O1B"} {
		for _, bui := range []float64{0, 10, 50, 150} {
			if got := BuildupEffect(code, bui); got != 1.0 {
				t.Errorf("BuildupEffect(%s, %v) = %v, want 1", code, bui, got)
			}
		}
	}
}

func TestROSMonotonicInISI(t *testing.T) {
	for _, code := range []string{"C1", "C2", "C3", "D1", "S1", "M1"} {
		prev := -1.0
		for isi := 0.0; isi <= 60; isi++ {
			cur := ROS(code, isi, 60, 50, curingPctForTest, 0)
			if cur < prev {
				t.Fatalf("%s not monotonic at ISI %v: %v < %v", code, isi, cur, prev)
			}
			prev = cur
		}
	}
}

func TestROSZeroAtZeroISI(t *testing.T) {
	for code := range Fuels {
		if got := ROS(code, 0, 60, 50, curingPctForTest, 30); got != 0 {
			t.Errorf("ROS(%s, isi=0) = %v, want 0", code, got)
		}
	}
}

func TestConiferSpreadsFasterThanDeciduous(t *testing.T) {
	for _, isi := range []float64{3, 8, 15, 25} {
		c2 := ROS("C2", isi, 60, 100, curingPctForTest, 0)
		d1 := ROS("D1", isi, 60, 100, curingPctForTest, 0)
		if c2 <= d1 {
			t.Errorf("at ISI %v: C2 %v not > D1 %v", isi, c2, d1)
		}
	}
}

func TestMixedwoodBracketedByEndmembers(t *testing.T) {
	for _, isi := range []float64{5, 12, 20} {
		c2 := RSI("C2", isi, 100, curingPctForTest)
		d1 := RSI("D1", isi, 100, curingPctForTest)
		mid := RSI("M1", isi, 50, curingPctForTest)
		if mid < math.Min(c2, d1) || mid > math.Max(c2, d1) {
			t.Errorf("M1 %v not bracketed by C2 %v / D1 %v at ISI %v", mid, c2, d1, isi)
		}
	}
	if got, want := RSI("M1", 10, 100, curingPctForTest), RSI("C2", 10, 100, curingPctForTest); !almostEqual(got, want, 1e-12) {
		t.Errorf("M1 at pc=100 = %v, want C2 %v", got, want)
	}
	if got, want := RSI("M1", 10, 0, curingPctForTest), RSI("D1", 10, 100, curingPctForTest); !almostEqual(got, want, 1e-12) {
		t.Errorf("M1 at pc=0 = %v, want D1 %v", got, want)
	}
}

// TestSlopeCapIsContinuous is the sharpest available check that eq. 39's
// coefficient and exponent are transcribed correctly without cffdrs: the formula
// must itself reach ~10 exactly where the published cap takes over.
func TestSlopeCapIsContinuous(t *testing.T) {
	if got := SlopeFactor(69.9999); !almostEqual(got, SlopeCapFactor, 0.01) {
		t.Errorf("SlopeFactor(69.9999) = %v, want ~%v", got, SlopeCapFactor)
	}
	if got := SlopeFactor(70); got != SlopeCapFactor {
		t.Errorf("SlopeFactor(70) = %v, want %v", got, SlopeCapFactor)
	}
	if got := SlopeFactor(200); got != SlopeCapFactor {
		t.Errorf("SlopeFactor(200) = %v, want %v", got, SlopeCapFactor)
	}
}

func TestSlopeFactorFlatAndDownhill(t *testing.T) {
	if got := SlopeFactor(0); got != 1.0 {
		t.Errorf("SlopeFactor(0) = %v, want 1", got)
	}
	if got := SlopeFactor(-30); got != 1.0 {
		t.Errorf("SlopeFactor(-30) = %v, want 1 (downslope not modelled)", got)
	}
}

func TestSlopePercentFromDegrees(t *testing.T) {
	// 45° is 100 % rise, not 45 % — the unit trap this function exists for.
	if got := SlopePercentFromDegrees(45); !almostEqual(got, 100, 1e-9) {
		t.Errorf("SlopePercentFromDegrees(45) = %v, want 100", got)
	}
	if got := SlopePercentFromDegrees(0); got != 0 {
		t.Errorf("SlopePercentFromDegrees(0) = %v, want 0", got)
	}
}

func TestROSFiniteAcrossOperatingRange(t *testing.T) {
	for code := range Fuels {
		for _, isi := range []float64{0, 1, 10, 50, 100} {
			for _, bui := range []float64{0, 1, 60, 200} {
				for _, slope := range []float64{0, 25, 70, 150} {
					if got := ROS(code, isi, bui, 50, curingPctForTest, slope); math.IsNaN(got) || math.IsInf(got, 0) {
						t.Errorf("ROS(%s, %v, %v, slope %v) = %v", code, isi, bui, slope, got)
					}
				}
			}
		}
	}
}

// TestUnknownFuelIsInert guards the API path: a cell whose fuel cannot be
// determined must yield 0, never NaN, since NaN is a caller's usual no-data
// sentinel.
func TestUnknownFuelIsInert(t *testing.T) {
	if got := ROS("nonsense", 10, 60, 100, curingPctForTest, 20); got != 0 {
		t.Errorf("ROS(unknown fuel) = %v, want 0", got)
	}
}
