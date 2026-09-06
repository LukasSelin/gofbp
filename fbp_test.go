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

// pdfPctForTest is the same for percent dead balsam fir, which weights the M3/M4
// blends and is ignored by every other fuel.
const pdfPctForTest = 35.0

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
			cur := ROS(code, isi, 60, 50, pdfPctForTest, curingPctForTest, 0)
			if cur < prev {
				t.Fatalf("%s not monotonic at ISI %v: %v < %v", code, isi, cur, prev)
			}
			prev = cur
		}
	}
}

func TestROSZeroAtZeroISI(t *testing.T) {
	for code := range Fuels {
		if got := ROS(code, 0, 60, 50, pdfPctForTest, curingPctForTest, 30); got != 0 {
			t.Errorf("ROS(%s, isi=0) = %v, want 0", code, got)
		}
	}
}

func TestConiferSpreadsFasterThanDeciduous(t *testing.T) {
	for _, isi := range []float64{3, 8, 15, 25} {
		c2 := ROS("C2", isi, 60, 100, pdfPctForTest, curingPctForTest, 0)
		d1 := ROS("D1", isi, 60, 100, pdfPctForTest, curingPctForTest, 0)
		if c2 <= d1 {
			t.Errorf("at ISI %v: C2 %v not > D1 %v", isi, c2, d1)
		}
	}
}

func TestMixedwoodBracketedByEndmembers(t *testing.T) {
	for _, isi := range []float64{5, 12, 20} {
		c2 := RSI("C2", isi, 100, pdfPctForTest, curingPctForTest)
		d1 := RSI("D1", isi, 100, pdfPctForTest, curingPctForTest)
		mid := RSI("M1", isi, 50, pdfPctForTest, curingPctForTest)
		if mid < math.Min(c2, d1) || mid > math.Max(c2, d1) {
			t.Errorf("M1 %v not bracketed by C2 %v / D1 %v at ISI %v", mid, c2, d1, isi)
		}
	}
	if got, want := RSI("M1", 10, 100, pdfPctForTest, curingPctForTest), RSI("C2", 10, 100, pdfPctForTest, curingPctForTest); !almostEqual(got, want, 1e-12) {
		t.Errorf("M1 at pc=100 = %v, want C2 %v", got, want)
	}
	if got, want := RSI("M1", 10, 0, pdfPctForTest, curingPctForTest), RSI("D1", 10, 100, pdfPctForTest, curingPctForTest); !almostEqual(got, want, 1e-12) {
		t.Errorf("M1 at pc=0 = %v, want D1 %v", got, want)
	}
}

// TestDeadFirMixedwoodEndpoints pins eqs. 29 and 33 at the two PDF values where
// the blend collapses to something nameable.
//
// At PDF 100 the deciduous half drops out and the answer must be the fuel's own
// eq. 30 curve — which is the quantity cffdrs reaches by forcing PDF to 100 in
// its slope path, so getting this endpoint wrong would break the back-solve in
// the same stroke. At PDF 0 the fuel's own curve drops out and only D1 remains,
// at full weight for M3 and at the 0.2 dead-fir weight for M4. That 0.2 is the
// single most transposable digit in either equation: M3 and M4 are otherwise the
// same shape, and dropping it makes M4 read five times high with no shape change
// to notice.
func TestDeadFirMixedwoodEndpoints(t *testing.T) {
	for _, isi := range []float64{3, 8, 15, 25} {
		d1 := RSI("D1", isi, 0, 0, 0)
		for _, code := range []string{"M3", "M4"} {
			own := rsiBase(Fuels[code], isi)
			if got := RSI(code, isi, 0, 100, 0); !almostEqual(got, own, 1e-12) {
				t.Errorf("RSI(%s, ISI %v) at PDF=100 = %v, want its own eq. 30 curve %v",
					code, isi, got, own)
			}
		}
		if got := RSI("M3", isi, 0, 0, 0); !almostEqual(got, d1, 1e-12) {
			t.Errorf("RSI(M3, ISI %v) at PDF=0 = %v, want D1 %v", isi, got, d1)
		}
		if got := RSI("M4", isi, 0, 0, 0); !almostEqual(got, 0.2*d1, 1e-12) {
			t.Errorf("RSI(M4, ISI %v) at PDF=0 = %v, want 0.2·D1 %v", isi, got, 0.2*d1)
		}
	}
}

// TestDeadFirMixedwoodBracketedByEndmembers is the shape between those
// endpoints: a weighted average of two curves cannot leave the interval they
// span. It is what would catch a blend written with the weights the wrong way
// round, which both endpoints above would still satisfy if they were swapped in
// the same direction.
func TestDeadFirMixedwoodBracketedByEndmembers(t *testing.T) {
	for _, isi := range []float64{5, 12, 20} {
		for _, code := range []string{"M3", "M4"} {
			lo := RSI(code, isi, 0, 0, 0)
			hi := RSI(code, isi, 0, 100, 0)
			for _, pdf := range []float64{10, 35, 50, 90} {
				mid := RSI(code, isi, 0, pdf, 0)
				if mid < math.Min(lo, hi) || mid > math.Max(lo, hi) {
					t.Errorf("%s at PDF %v = %v, not bracketed by %v / %v at ISI %v",
						code, pdf, mid, lo, hi, isi)
				}
			}
		}
	}
}

// TestBlendInputsDoNotCrossFamilies is the test the two-parameter signature
// exists for.
//
// pc and pdf sit next to each other, are both percentages, and are both "how
// much of this mixedwood is the flammable component" — so the failure mode is
// not a wrong formula but a wrong variable, and it produces a number in the
// right range with the right units. Nothing else here would see it: every other
// mixedwood test holds one of the two fixed. M1/M2 must ignore pdf entirely and
// M3/M4 must ignore pc entirely, and that is checkable independently of what
// either blend computes.
func TestBlendInputsDoNotCrossFamilies(t *testing.T) {
	const isi = 12
	for _, code := range []string{"M1", "M2"} {
		want := RSI(code, isi, 50, 0, 0)
		for _, pdf := range []float64{0, 35, 100} {
			if got := RSI(code, isi, 50, pdf, 0); got != want {
				t.Errorf("RSI(%s) moved with pdf: %v at pdf=%v, %v at pdf=0", code, got, pdf, want)
			}
		}
	}
	for _, code := range []string{"M3", "M4"} {
		want := RSI(code, isi, 0, 50, 0)
		for _, pc := range []float64{0, 35, 100} {
			if got := RSI(code, isi, pc, 50, 0); got != want {
				t.Errorf("RSI(%s) moved with pc: %v at pc=%v, %v at pc=0", code, got, pc, want)
			}
		}
	}
	// The same separation in the slope path, where the two blends live in
	// different switch branches and could diverge independently.
	base := SlopeWind{FFMC: 92, SlopePct: 30, PC: 50, PDF: 50}
	for _, tc := range []struct {
		code  string
		mutet func(*SlopeWind)
		field string
	}{
		{"M1", func(s *SlopeWind) { s.PDF = 0 }, "PDF"},
		{"M2", func(s *SlopeWind) { s.PDF = 0 }, "PDF"},
		{"M3", func(s *SlopeWind) { s.PC = 0 }, "PC"},
		{"M4", func(s *SlopeWind) { s.PC = 0 }, "PC"},
	} {
		s := base
		s.Code = tc.code
		want := EquivalentWind(s)
		tc.mutet(&s)
		if got := EquivalentWind(s); got != want {
			t.Errorf("EquivalentWind(%s) moved with %s: %v, want %v", tc.code, tc.field, got, want)
		}
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
					if got := ROS(code, isi, bui, 50, pdfPctForTest, curingPctForTest, slope); math.IsNaN(got) || math.IsInf(got, 0) {
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
//
// This is the numeric half of the contract and it is deliberately not the whole
// of it. 0 is the safe value to return but it is not an informative one — it is
// what an implemented fuel with no spread returns too. CanonicalFuelCode is what
// separates the two, and TestUnimplementedFuelIsReportedRatherThanInferred is
// where that half is asserted.
func TestUnknownFuelIsInert(t *testing.T) {
	if got := ROS("nonsense", 10, 60, 100, pdfPctForTest, curingPctForTest, 20); got != 0 {
		t.Errorf("ROS(unknown fuel) = %v, want 0", got)
	}
}
