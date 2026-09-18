package fbp

import (
	"math"
	"testing"
)

// The oracle tests in foliar_cffdrs_test.go skip on a fresh clone. These are what
// CI runs on every push, so they carry the shape of the model rather than its
// coefficients: the branch points, the range, the sentinels and the rounding.

func TestFoliarMoistureContentBranchPoints(t *testing.T) {
	// D0 supplied so the site drops out and ND is exactly DJ - D0.
	const d0 = 200

	tests := []struct {
		nd   float64
		want float64
		eq   string
	}{
		{0, 85, "eq. 6 at the minimum"},
		{10, 85 + 0.0189*100, "eq. 6"},
		{29, 85 + 0.0189*841, "eq. 6, last day before the hinge"},
		{30, 32.9 + 3.17*30 - 0.0288*900, "eq. 7, first day at the hinge"},
		{40, 32.9 + 3.17*40 - 0.0288*1600, "eq. 7"},
		{49, 32.9 + 3.17*49 - 0.0288*2401, "eq. 7, last day before the plateau"},
		{50, 120, "eq. 8, first day of the plateau"},
		{200, 120, "eq. 8"},
	}
	for _, tc := range tests {
		for _, sign := range []float64{1, -1} {
			got := FoliarMoistureContent(50, 100, 0, d0+sign*tc.nd, d0)
			if math.Abs(got-tc.want) > 1e-12 {
				t.Errorf("ND = %v (%s, dj %s d0): FMC = %v, want %v",
					tc.nd, tc.eq, map[float64]string{1: "after", -1: "before"}[sign], got, tc.want)
			}
		}
	}
}

// The curve is NOT continuous, and that is the published system rather than a
// transcription error. Eq. 6 arrives at the hinge from below at 102.01 and eq. 7
// leaves it at 102.08; eq. 7 reaches day 50 at 119.4 and eq. 8 starts at 120.
//
// Worth pinning rather than smoothing. A reader who assumes continuity and
// "fixes" one of the coefficients to get it would move every FMC in the band.
func TestFoliarMoistureContentIsDiscontinuousAtBothHinges(t *testing.T) {
	const d0 = 200
	at := func(nd float64) float64 { return FoliarMoistureContent(50, 100, 0, d0+nd, d0) }

	below30, at30 := at(29.999999), at(30)
	if gap := at30 - below30; math.Abs(gap-0.07) > 0.01 {
		t.Errorf("the eq. 6/7 hinge steps by %v; the published coefficients give ~0.07 "+
			"(102.01 to 102.08). A zero gap means someone made the curve continuous.", gap)
	}
	below50, at50 := at(49.999999), at(50)
	if gap := at50 - below50; math.Abs(gap-0.6) > 0.01 {
		t.Errorf("the eq. 7/8 hinge steps by %v; the published coefficients give ~0.6 "+
			"(119.4 to 120)", gap)
	}
	t.Logf("published discontinuities: %.4f at ND 30, %.4f at ND 50", at30-below30, at50-below50)
}

func TestFoliarMoistureContentIsBoundedAndRises(t *testing.T) {
	const d0 = 180
	prev := math.Inf(-1)
	for nd := 0.0; nd <= 120; nd += 0.25 {
		got := FoliarMoistureContent(55, 110, 0, d0+nd, d0)
		if got < fmcMinimumPct || got > MaxFoliarMoisturePct {
			t.Fatalf("ND = %v: FMC = %v, outside [%v, %v]",
				nd, got, fmcMinimumPct, MaxFoliarMoisturePct)
		}
		// Monotone in days from the minimum: foliage only recovers moisture as
		// the season moves away from its driest day. Both hinges step UP, so this
		// holds across them too.
		if got < prev {
			t.Fatalf("ND = %v: FMC fell from %v to %v", nd, prev, got)
		}
		prev = got
	}
}

// Eq. 5 is |DJ - D0|, so a date is only ever as far from the minimum as the
// calendar makes it — the model does not distinguish spring from autumn.
func TestFoliarMoistureContentIsSymmetricAboutTheMinimum(t *testing.T) {
	for _, nd := range []float64{0, 1, 15, 29.5, 30, 45, 50, 90} {
		before := FoliarMoistureContent(50, 100, 0, 200-nd, 200)
		after := FoliarMoistureContent(50, 100, 0, 200+nd, 200)
		if before != after {
			t.Errorf("ND = %v: %v days before the minimum gives %v, after gives %v",
				nd, nd, before, after)
		}
	}
}

// The branch cffdrs selects on ELV <= 0 rather than on "elevation is known" —
// see DateOfMinimumFoliarMoisture. At exactly 0 it is eqs. 1/2, and the two
// branches give genuinely different dates, so this is not a distinction without
// a difference.
func TestDateOfMinimumFoliarMoistureBranchesOnElevationSign(t *testing.T) {
	const lat, long = 50.0, 100.0

	flat := DateOfMinimumFoliarMoisture(lat, long, 0)
	alsoFlat := DateOfMinimumFoliarMoisture(lat, long, -500)
	if flat != alsoFlat {
		t.Errorf("ELV 0 gives D0 %v and ELV -500 gives %v; cffdrs puts both on eqs. 1/2",
			flat, alsoFlat)
	}

	// Eqs. 1/2 at this site: LATN = 46 + 23.4·exp(-0.0360·50), D0 = 151·LAT/LATN.
	latn := 46 + 23.4*math.Exp(-0.0360*50)
	if want := math.RoundToEven(151 * lat / latn); flat != want {
		t.Errorf("D0 = %v, eqs. 1/2 give %v", flat, want)
	}

	// Just above sea level is a different branch and a different date — the
	// discontinuity that reading ELV as a sentinel creates.
	elev := DateOfMinimumFoliarMoisture(lat, long, 1e-9)
	if elev == flat {
		t.Errorf("ELV 1e-9 gives the same D0 as ELV 0 (%v); eqs. 3/4 should apply "+
			"above zero and they have different coefficients", flat)
	}

	// Elevation delays the minimum by 0.0172 days per metre, on top of a date
	// that is already later than eqs. 1/2's at this longitude.
	low := d0ElevScale * lat / (43 + 33.7*math.Exp(-0.0351*50))
	for _, m := range []float64{100, 1000, 3000} {
		got := DateOfMinimumFoliarMoisture(lat, long, m)
		if want := math.RoundToEven(low + 0.0172*m); got != want {
			t.Errorf("ELV %v m: D0 = %v, eqs. 3/4 give %v", m, got, want)
		}
	}
}

// R's round() goes to the even digit on an exact half; Go's math.Round does not.
// The oracle test proves the fixture agrees with the first rule; this one proves
// this package implements it, and runs without a fixture.
func TestFoliarMoistureRoundsASuppliedD0HalfToEven(t *testing.T) {
	tests := []struct{ d0, rounded float64 }{
		{150.5, 150}, // half, even below — the case that separates the two rules
		{151.5, 152}, // half, odd below — both rules agree
		{150.7, 151},
		{150.2, 150},
		{200, 200},
	}
	for _, tc := range tests {
		// A Dj inside eq. 6's window, where a day of D0 changes the answer.
		const dj = 160
		got := FoliarMoistureContent(50, 100, 0, dj, tc.d0)
		want := FoliarMoistureContent(50, 100, 0, dj, tc.rounded)
		if got != want {
			t.Errorf("d0 = %v: FMC = %v, but rounding to %v gives %v", tc.d0, got, tc.rounded, want)
		}
	}
	// And the one that would pass under either rule must not be the only evidence:
	// 150.5 and 151.5 have to land on different days.
	if a, b := math.RoundToEven(150.5), math.RoundToEven(151.5); a != 150 || b != 152 {
		t.Fatalf("math.RoundToEven(150.5) = %v, (151.5) = %v — not half-to-even", a, b)
	}
}

// A supplied D0 bypasses eqs. 1-4 entirely, so the site must stop mattering.
func TestFoliarMoistureContentIgnoresTheSiteWhenD0IsGiven(t *testing.T) {
	want := FoliarMoistureContent(50, 100, 0, 210, 190)
	for _, site := range [][3]float64{{7, 52, 0}, {70, 140, 3000}, {-45, 0, -100}} {
		got := FoliarMoistureContent(site[0], site[1], site[2], 210, 190)
		if got != want {
			t.Errorf("lat=%v long=%v elv=%v with D0 = 190: FMC = %v, want %v (the site "+
				"should not be read at all)", site[0], site[1], site[2], got, want)
		}
	}
}

// This package does not fold the sign of a longitude, and that is a documented
// decision rather than an omission — 120 °E is not 120 °W. fbp() folds it; a
// caller matching fbp() passes math.Abs.
func TestFoliarMoistureContentDoesNotFoldLongitudeSign(t *testing.T) {
	west := DateOfMinimumFoliarMoisture(36, 60, 0)
	east := DateOfMinimumFoliarMoisture(36, -60, 0)
	if west == east {
		t.Fatal("DateOfMinimumFoliarMoisture(-60) equals (+60) — the sign is being folded " +
			"somewhere, which silently maps eastern longitudes onto western ones")
	}
	t.Logf("60 °W gives D0 %v; -60 passed through unfolded gives %v (fbp() would fold it "+
		"and get %v)", west, east, west)
}

func TestFoliarMoistureContentNonFiniteInput(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)

	// Every driver the computed-D0 path reads.
	for i, args := range [][5]float64{
		{nan, 100, 0, 200, 0},
		{50, nan, 0, 200, 0},
		{50, 100, nan, 200, 0},
		{50, 100, 0, nan, 0},
		{50, 100, 0, 200, nan},
		{inf, 100, 0, 200, 0},
		{50, inf, 0, 200, 0},
		{50, 100, inf, 200, 0},
		{50, 100, 0, inf, 0},
		{50, 100, 0, 200, inf},
		{50, 100, 0, -inf, 0},
	} {
		got := FoliarMoistureContent(args[0], args[1], args[2], args[3], args[4])
		if !math.IsNaN(got) {
			t.Errorf("case %d %v: FMC = %v, want NaN. An infinity that reaches eq. 8 comes "+
				"back as an ordinary 120.", i, args, got)
		}
	}

	// A supplied D0 short-circuits eqs. 1-4, so a non-finite SITE is then
	// irrelevant and must not poison the answer.
	if got := FoliarMoistureContent(nan, inf, nan, 210, 190); math.IsNaN(got) {
		t.Errorf("with D0 = 190 supplied the site is never read, so FMC should be %v, not NaN",
			FoliarMoistureContent(50, 100, 0, 210, 190))
	}

	for i, args := range [][3]float64{
		{nan, 100, 0}, {50, nan, 0}, {50, 100, nan},
		{inf, 100, 0}, {50, inf, 0}, {50, 100, inf}, {50, -inf, 0},
	} {
		if got := DateOfMinimumFoliarMoisture(args[0], args[1], args[2]); !math.IsNaN(got) {
			t.Errorf("case %d %v: D0 = %v, want NaN", i, args, got)
		}
	}
}

// FMC is one of the two handles on eq. 56, so its range is what sets the range of
// the crown-fire threshold. Stating that as a test keeps the two files joined.
func TestFoliarMoistureContentMovesTheCrownThreshold(t *testing.T) {
	const cbh = 3.0
	driest := CriticalSurfaceIntensity(FoliarMoistureContent(50, 100, 0, 200, 200), cbh)
	wettest := CriticalSurfaceIntensity(FoliarMoistureContent(50, 100, 0, 300, 200), cbh)
	if driest >= wettest {
		t.Fatalf("CSI at the annual minimum is %v and on the plateau is %v; wetter foliage "+
			"must make crowning harder", driest, wettest)
	}
	t.Logf("across FMC's full range CSI moves %.0f to %.0f kW/m at CBH %v m (%.2fx)",
		driest, wettest, cbh, wettest/driest)
}
