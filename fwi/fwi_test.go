package fwi

import (
	"math"
	"testing"
)

// These run on a fresh clone with no fixture. They cannot say a coefficient is
// right — only cffdrs_test.go can — but they forbid whole classes of wrong:
// bounds a code must stay inside, directions rain and wind must push, and
// identities between the functions and Step that hold for any coefficients.
// Metamorphic where possible, like the rest of the repository: each asserts a
// relation between two outputs rather than a number.

// A grid of valid weather, wide enough to reach every branch of every code.
var (
	gridTemp   = []float64{-40, -10, -2.8, -1.1, 0, 5, 15, 25, 35, 45}
	gridRH     = []float64{0, 1, 10, 30, 50, 70, 90, 99, 100}
	gridWind   = []float64{0, 1, 10, 30, 40, 60, 120}
	gridPrecip = []float64{0, 0.3, 0.5, 0.51, 1.5, 1.51, 2.8, 2.81, 5, 20, 80, 250}
	gridFFMC   = []float64{0, 5, 20, 50, 70, 85, 90, 95, 99, 101}
	gridDMC    = []float64{0, 0.5, 5, 20, 33, 50, 65, 100, 300}
	gridDC     = []float64{0, 5, 15, 100, 300, 600, 1000}
	gridLat    = []float64{-90, -45, -30, -20, -10, 0, 10, 20, 30, 46, 55, 62, 69, 90}
)

// Every output of Step stays inside its documented range for every valid day,
// from every valid yesterday.
func TestStepStaysWithinBounds(t *testing.T) {
	n := 0
	for _, f := range gridFFMC {
		for _, d := range []float64{0, 5, 65, 300} {
			for _, c := range []float64{0, 15, 600} {
				for _, temp := range gridTemp {
					for _, rh := range gridRH {
						for _, ws := range []float64{0, 30, 120} {
							for _, p := range []float64{0, 0.51, 1.51, 2.81, 20, 250} {
								for _, m := range []int{1, 7} {
									for _, lat := range []float64{-45, 0, 62} {
										_, ix := Step(State{f, d, c}, Weather{temp, rh, ws, p}, m, lat)
										n++
										if !(ix.FFMC >= 0 && ix.FFMC <= 101) ||
											!(ix.DMC >= 0) || !(ix.DC >= 0) || !(ix.ISI >= 0) ||
											!(ix.BUI >= 0) || !(ix.FWI >= 0) || !(ix.DSR >= 0) {
											t.Fatalf("out of bounds: yesterday %v/%v/%v, weather %v/%v/%v/%v, month %d, lat %v: %+v",
												f, d, c, temp, rh, ws, p, m, lat, ix)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d days, all in bounds", n)
}

// More rain never raises a code. FFMC is the one the task names; DMC and DC
// hold for the same reason — rain only ever adds moisture — and are checked
// alongside. Each pair of adjacent precipitation values is a separate
// metamorphic check, which includes every threshold crossing.
func TestRainNeverRaisesACode(t *testing.T) {
	for _, temp := range gridTemp {
		for _, rh := range gridRH {
			for _, ws := range gridWind {
				for _, f := range gridFFMC {
					prev := FFMC(f, temp, rh, ws, gridPrecip[0])
					for _, p := range gridPrecip[1:] {
						got := FFMC(f, temp, rh, ws, p)
						if got > prev+1e-12 {
							t.Fatalf("FFMC rose with rain: yesterday %v, T %v, RH %v, wind %v: %v at %v mm > %v before",
								f, temp, rh, ws, got, p, prev)
						}
						prev = got
					}
				}
			}
			for _, d := range gridDMC {
				prev := DMC(d, temp, rh, gridPrecip[0], 7, 55)
				for _, p := range gridPrecip[1:] {
					got := DMC(d, temp, rh, p, 7, 55)
					if got > prev+1e-12 {
						t.Fatalf("DMC rose with rain: yesterday %v, T %v, RH %v: %v at %v mm > %v before", d, temp, rh, got, p, prev)
					}
					prev = got
				}
			}
		}
		for _, c := range gridDC {
			prev := DC(c, temp, 50, gridPrecip[0], 7, 55)
			for _, p := range gridPrecip[1:] {
				got := DC(c, temp, 50, p, 7, 55)
				if got > prev+1e-12 {
					t.Fatalf("DC rose with rain: yesterday %v, T %v: %v at %v mm > %v before", c, temp, got, p, prev)
				}
				prev = got
			}
		}
	}
}

// Rain at or below each code's threshold is no rain at all.
func TestRainAtTheThresholdIsNoRain(t *testing.T) {
	for _, temp := range gridTemp {
		for _, rh := range gridRH {
			for _, f := range gridFFMC {
				if FFMC(f, temp, rh, 15, 0.5) != FFMC(f, temp, rh, 15, 0) {
					t.Errorf("FFMC: 0.5 mm is not the same as none (yesterday %v, T %v, RH %v)", f, temp, rh)
				}
			}
			for _, d := range gridDMC {
				if DMC(d, temp, rh, 1.5, 7, 55) != DMC(d, temp, rh, 0, 7, 55) {
					t.Errorf("DMC: 1.5 mm is not the same as none (yesterday %v, T %v, RH %v)", d, temp, rh)
				}
			}
			for _, c := range gridDC {
				if DC(c, temp, rh, 2.8, 7, 55) != DC(c, temp, rh, 0, 7, 55) {
					t.Errorf("DC: 2.8 mm is not the same as none (yesterday %v, T %v)", c, temp)
				}
			}
		}
	}
}

// And rain just above each threshold does something, on a code with room to
// fall. Without this, a threshold transcribed ten times too high passes the
// test above.
func TestRainJustAboveTheThresholdWets(t *testing.T) {
	if !(FFMC(92, 20, 50, 10, 0.6) < FFMC(92, 20, 50, 10, 0.5)) {
		t.Error("FFMC: 0.6 mm did not wet a dry fine fuel")
	}
	if !(DMC(40, 20, 50, 1.6, 7, 55) < DMC(40, 20, 50, 1.5, 7, 55)) {
		t.Error("DMC: 1.6 mm did not wet the duff")
	}
	if !(DC(300, 20, 50, 2.9, 7, 55) < DC(300, 20, 50, 2.8, 7, 55)) {
		t.Error("DC: 2.9 mm did not wet the deep layer")
	}
}

// Below its floor, temperature stops mattering: -1.1 °C for the DMC, -2.8 °C
// for the DC. FFMC has no floor.
func TestTemperatureFloors(t *testing.T) {
	for _, m := range []int{1, 4, 7, 10} {
		for _, lat := range gridLat {
			for _, cold := range []float64{-5, -20, -60} {
				if DMC(20, cold, 50, 0, m, lat) != DMC(20, -1.1, 50, 0, m, lat) {
					t.Errorf("DMC at %v °C differs from the -1.1 floor (month %d, lat %v)", cold, m, lat)
				}
				if DC(200, cold, 50, 0, m, lat) != DC(200, -2.8, 50, 0, m, lat) {
					t.Errorf("DC at %v °C differs from the -2.8 floor (month %d, lat %v)", cold, m, lat)
				}
			}
			if DMC(20, 0, 50, 0, m, lat) == DMC(20, -1.1, 50, 0, m, lat) && DMCDayLength(m, lat) != 0 {
				t.Errorf("DMC does not respond to temperature above the floor (month %d, lat %v)", m, lat)
			}
		}
	}
}

// The DC does not read relative humidity at all.
func TestDCIgnoresRelativeHumidity(t *testing.T) {
	for _, temp := range gridTemp {
		for _, p := range gridPrecip {
			want := DC(300, temp, 50, p, 7, 55)
			for _, rh := range append(gridRH, math.NaN()) {
				if got := DC(300, temp, rh, p, 7, 55); got != want {
					t.Errorf("DC moved with RH %v (T %v, rain %v): %v != %v", rh, temp, p, got, want)
				}
			}
		}
	}
}

// BUI is 0 whenever DMC is — for any DC, including 0, the 0/0 case eq. 27a
// would otherwise hit. And it never reaches twice the DMC: eq. 27a is
// 0.8·DC·DMC/(DMC + 0.4·DC), which is below 2·DMC for every DC, and eq. 27b
// is below DMC. The converse does not hold: eq. 27b's floor makes BUI 0 for a
// DMC below about 0.92 when DC is small.
func TestBUIIsZeroWhenDMCIs(t *testing.T) {
	for _, c := range append(gridDC, 1e-9, 5000) {
		if got := BUI(0, c); got != 0 {
			t.Errorf("BUI(0, %v) = %v, want 0", c, got)
		}
	}
	for _, d := range gridDMC[1:] {
		for _, c := range gridDC {
			got := BUI(d, c)
			if !(got >= 0 && got < 2*d) {
				t.Errorf("BUI(%v, %v) = %v, outside [0, 2·DMC)", d, c, got)
			}
		}
	}
}

// BUI rises with DC at fixed DMC, and with DMC at fixed DC — for any DC of
// about 1.15 or more. Below that it does not, briefly: just past DMC = 0.4·DC,
// where eq. 27b takes over from 27a, its slope is 1 − cc·0.8·DC/(DMC+0.4·DC)²,
// which at DMC = 0.4·DC is 1 − 1.25·cc/DC, and cc ≥ 0.92. A DC that low only
// follows a soaking that floors it, so the grid here starts at 5, and
// TestBUIDipsAtTinyDC pins the exception rather than leaving it to be found.
func TestBUIRisesWithBothCodes(t *testing.T) {
	var dmcs, dcs []float64
	for x := 0.0; x <= 400; x += 0.5 {
		dmcs = append(dmcs, x)
	}
	for x := 0.0; x <= 1200; x += 2 {
		dcs = append(dcs, x)
	}
	for _, c := range gridDC {
		prev := BUI(dmcs[0], c)
		for _, d := range dmcs[1:] {
			got := BUI(d, c)
			if got < prev-1e-12 {
				t.Fatalf("BUI fell as DMC rose: BUI(%v, %v) = %v < %v", d, c, got, prev)
			}
			prev = got
		}
	}
	for _, d := range gridDMC {
		prev := BUI(d, dcs[0])
		for _, c := range dcs[1:] {
			got := BUI(d, c)
			if got < prev-1e-12 {
				t.Fatalf("BUI fell as DC rose: BUI(%v, %v) = %v < %v", d, c, got, prev)
			}
			prev = got
		}
	}
}

// The exception above, as the equations have it: at DC 1, BUI falls as DMC
// rises through 0.4. Eq. 27a gives exactly DMC there; eq. 27b's slope is
// negative.
func TestBUIDipsAtTinyDC(t *testing.T) {
	at := BUI(0.4, 1)
	after := BUI(0.41, 1)
	if !(after < at) {
		t.Errorf("BUI(0.41, 1) = %v is not below BUI(0.4, 1) = %v", after, at)
	}
	if math.Abs(at-0.4) > 1e-15 {
		t.Errorf("BUI(0.4, 1) = %v, want exactly DMC (eq. 27a at DMC = 0.4·DC)", at)
	}
	if BUI(0.51, 1.2) < BUI(0.5, 1.2) {
		t.Error("BUI dips at DC 1.2 too; the stated threshold of about 1.15 is wrong")
	}
}

// ISI rises with FFMC and with wind, and is exponential in wind at every speed
// — the ratio for a fixed step in wind is the same everywhere, which is the
// property FBP's high-wind function does not have.
func TestISIRisesWithFFMCAndIsExponentialInWind(t *testing.T) {
	for _, ws := range gridWind {
		prev := ISI(0, ws)
		for f := 0.5; f <= 101; f += 0.5 {
			got := ISI(f, ws)
			if !(got > prev) {
				t.Fatalf("ISI did not rise with FFMC: ISI(%v, %v) = %v <= %v", f, ws, got, prev)
			}
			prev = got
		}
	}
	want := math.Exp(0.05039 * 10)
	for _, f := range gridFFMC {
		for _, ws := range []float64{0, 20, 35, 40, 50, 80, 150} {
			if r := ISI(f, ws+10) / ISI(f, ws); math.Abs(r-want) > 1e-12*want {
				t.Errorf("ISI(%v, %v+10)/ISI(%v, %v) = %v, want exp(0.5039) = %v", f, ws, f, ws, r, want)
			}
		}
	}
}

// FWI rises with ISI at fixed BUI, and with BUI on each side of 80.
func TestFWIRisesWithISIAndWithBUIOnEachSideOf80(t *testing.T) {
	for _, b := range []float64{0, 5, 40, 80, 81, 200} {
		prev := FWI(0, b)
		for i := 0.1; i <= 100; i += 0.1 {
			got := FWI(i, b)
			if got < prev {
				t.Fatalf("FWI fell as ISI rose: FWI(%v, %v) = %v < %v", i, b, got, prev)
			}
			prev = got
		}
	}
	for _, i := range []float64{0.5, 3, 10, 40} {
		for _, side := range [][2]float64{{0, 80}, {80.01, 500}} {
			prev := FWI(i, side[0])
			for b := side[0] + 0.01; b <= side[1]; b += 0.01 {
				got := FWI(i, b)
				if got < prev {
					t.Fatalf("FWI fell as BUI rose within [%v, %v]: FWI(%v, %v) = %v < %v",
						side[0], side[1], i, b, got, prev)
				}
				prev = got
			}
		}
	}
}

// FWI's two duff-moisture branches do not meet at BUI 80: fD steps down by
// 0.08 % crossing it (23.686 to 23.666), so FWI dips there. This is Van Wagner
// (1987)'s eqs. 28a/28b as published and as cffdrs computes them; the oracle
// grid carries 79.9, 80 and 80.1. Pinned so a "fix" that made FWI continuous is
// caught as the departure from cffdrs it would be.
func TestFWIStepsDownAtBUI80(t *testing.T) {
	at := FWI(10, 80)
	above := FWI(10, math.Nextafter(80, 100))
	if !(above < at) {
		t.Fatalf("FWI(10, 80⁺) = %v is not below FWI(10, 80) = %v — the published step is gone", above, at)
	}
	fdAt := 0.626*math.Pow(80, 0.809) + 2
	fdAbove := 1000 / (25 + 108.64/math.Exp(0.023*80))
	if drop := (fdAt - fdAbove) / fdAt; math.Abs(drop-0.000815) > 1e-5 {
		t.Errorf("fD drops %.4f %% at BUI 80, want 0.0815 %%", 100*drop)
	}
}

// FFMC's clamp to [0, 101]: the doc comment says the top binds only at
// temperatures no weather produces and the bottom not at all. This checks both
// halves against the unclamped arithmetic over a far wider grid than the
// fixture, rather than leaving the comment to be believed.
func TestFFMCIsBoundedWithoutItsClamps(t *testing.T) {
	topBinds := false
	for _, f := range gridFFMC {
		for temp := -80.0; temp <= 70; temp += 2.5 {
			for _, rh := range []float64{0, 0.1, 1, 3, 5, 10, 20, 50, 80, 100} {
				for _, ws := range gridWind {
					for _, p := range gridPrecip {
						raw := ffmcUnclamped(f, temp, rh, ws, p)
						if raw < 0 {
							t.Fatalf("unclamped FFMC %v < 0 at yesterday %v, T %v, RH %v, wind %v, rain %v — the lower clamp is reachable after all",
								raw, f, temp, rh, ws, p)
						}
						// An unchanged FFMC of 101 round-trips through eqs. 1 and 10
						// as 101 plus an ulp; that is rounding, not the clamp doing
						// work, so it is allowed for below.
						if raw > 101+1e-12 {
							if temp <= 45 {
								t.Fatalf("unclamped FFMC %v > 101 at %v °C (yesterday %v, RH %v, wind %v, rain %v) — the upper clamp binds in real weather",
									raw, temp, f, rh, ws, p)
							}
							topBinds = true
						}
					}
				}
			}
		}
	}
	if !topBinds {
		t.Error("the upper clamp never bound, even at 70 °C — then the fixture's clamp rows are not testing it")
	}
}

// DMC's drying term is 1.894·(T+1.1)·(100−RH)·Le·10⁻⁴, so with no rain the day's
// change is exactly proportional to (100 − RH) and to Le, from any yesterday
// above the floors. Checked as ratios between two days, which holds whatever
// the coefficient is.
func TestDMCDryingIsProportionalToDrynessAndDayLength(t *testing.T) {
	for _, m := range []int{1, 5, 7, 11} {
		for _, lat := range gridLat {
			le := DMCDayLength(m, lat)
			a := DMC(20, 20, 60, 0, m, lat) - 20
			b := DMC(20, 20, 20, 0, m, lat) - 20
			if math.Abs(b/a-2) > 1e-12 {
				t.Errorf("month %d lat %v: 80 %% dryness gave %v× the drying of 40 %%, want 2", m, lat, b/a)
			}
			if math.Abs(a/le-1.894*21.1*40*1e-4) > 1e-12 {
				t.Errorf("month %d lat %v: drying per unit Le = %v", m, lat, a/le)
			}
		}
	}
}

// With no rain, RH 100 means no DMC drying at all in the component, and Step's
// clamp to 99.9999 is what makes the day differ.
func TestStepClampsRHAt100(t *testing.T) {
	y := State{FFMC: 85, DMC: 20, DC: 200}
	for _, rh := range []float64{100, 100.5, 150} {
		clamped, ixc := Step(y, Weather{20, rh, 10, 0}, 7, 55)
		ref, ixr := Step(y, Weather{20, rhCeiling, 10, 0}, 7, 55)
		if clamped != ref || ixc != ixr {
			t.Errorf("Step at RH %v differs from Step at RH %v", rh, rhCeiling)
		}
	}
	if got := DMC(20, 20, 100, 0, 7, 55); got != 20 {
		t.Errorf("DMC component at RH 100 = %v, want exactly yesterday's 20 (no drying)", got)
	}
	_, ix := Step(y, Weather{20, 100, 10, 0}, 7, 55)
	if !(ix.DMC > 20) {
		t.Errorf("Step at RH 100 gave DMC %v; fwi()'s clamp to 99.9999 should dry it by a hair", ix.DMC)
	}
}

// Step is exactly the component functions composed, apart from the RH clamp.
func TestStepComposesTheComponents(t *testing.T) {
	for _, f := range gridFFMC {
		for _, temp := range gridTemp {
			for _, rh := range gridRH[:len(gridRH)-1] { // below 100: no clamp
				for _, p := range gridPrecip {
					for _, lat := range gridLat {
						y := State{FFMC: f, DMC: 30, DC: 250}
						s, ix := Step(y, Weather{temp, rh, 25, p}, 6, lat)
						want := Indices{
							FFMC: FFMC(f, temp, rh, 25, p),
							DMC:  DMC(30, temp, rh, p, 6, lat),
							DC:   DC(250, temp, rh, p, 6, lat),
						}
						want.ISI = ISI(want.FFMC, 25)
						want.BUI = BUI(want.DMC, want.DC)
						want.FWI = FWI(want.ISI, want.BUI)
						want.DSR = DSR(want.FWI)
						if ix != want || s != want.State() {
							t.Fatalf("Step disagrees with the components: %+v vs %+v", ix, want)
						}
					}
				}
			}
		}
	}
}

// fwi() refuses negative precipitation, wind and RH with stop(); Step has no
// error to return and returns NaN everywhere instead. NaN weather does the same.
func TestStepRefusesWhatFwiRefuses(t *testing.T) {
	y := StartupState()
	for _, w := range []Weather{
		{20, 40, 10, -0.1},
		{20, 40, -1, 0},
		{20, -5, 10, 0},
		{math.NaN(), 40, 10, 0},
		{20, math.NaN(), 10, 0},
		{20, 40, math.NaN(), 0},
		{20, 40, 10, math.NaN()},
	} {
		s, ix := Step(y, w, 7, 55)
		for _, v := range []float64{s.FFMC, s.DMC, s.DC, ix.FFMC, ix.DMC, ix.DC, ix.ISI, ix.BUI, ix.FWI, ix.DSR} {
			if !math.IsNaN(v) {
				t.Errorf("Step(%+v) = %+v, want NaN throughout", w, ix)
				break
			}
		}
	}
	// And a NaN carried in stays: a chain does not quietly recover from it.
	nan := math.NaN()
	s, _ := Step(State{nan, nan, nan}, Weather{20, 40, 10, 0}, 7, 55)
	if !math.IsNaN(s.FFMC) || !math.IsNaN(s.DMC) || !math.IsNaN(s.DC) {
		t.Errorf("a NaN yesterday recovered: %+v", s)
	}
}

// An invalid month has no day length, and says so as NaN rather than as some
// other month's value.
func TestInvalidMonthIsNaN(t *testing.T) {
	for _, m := range []int{0, -1, 13, 100} {
		if !math.IsNaN(DMCDayLength(m, 55)) || !math.IsNaN(DCDayLength(m, 55)) {
			t.Errorf("month %d has a day length", m)
		}
		if !math.IsNaN(DMC(20, 20, 40, 0, m, 55)) || !math.IsNaN(DC(200, 20, 40, 0, m, 55)) {
			t.Errorf("month %d produced a code", m)
		}
		_, ix := Step(StartupState(), Weather{20, 40, 10, 0}, m, 55)
		if math.IsNaN(ix.FFMC) || math.IsNaN(ix.ISI) {
			t.Errorf("month %d broke FFMC or ISI, which do not depend on it: %+v", m, ix)
		}
		if !math.IsNaN(ix.DMC) || !math.IsNaN(ix.DC) || !math.IsNaN(ix.BUI) || !math.IsNaN(ix.FWI) || !math.IsNaN(ix.DSR) {
			t.Errorf("month %d did not make DMC, DC, BUI, FWI and DSR NaN: %+v", m, ix)
		}
	}
	if !math.IsNaN(DMCDayLength(7, math.NaN())) || !math.IsNaN(DCDayLength(7, math.NaN())) {
		t.Error("a NaN latitude has a day length")
	}
}

// Everywhere the consumer runs — 55 to 69 °N — and anywhere north of 30 °N,
// the latitude adjustment changes nothing: the day lengths are Van Wagner's
// tables and do not vary with latitude inside the band.
func TestDayLengthIsLatitudeFreeNorthOf30(t *testing.T) {
	for m := 1; m <= 12; m++ {
		le, lf := DMCDayLength(m, 46), DCDayLength(m, 46)
		for lat := 30.5; lat <= 90; lat += 0.5 {
			if DMCDayLength(m, lat) != le || DCDayLength(m, lat) != lf {
				t.Fatalf("month %d: day length at %v °N differs from 46 °N", m, lat)
			}
		}
	}
	if DMCDayLength(7, 30) == DMCDayLength(7, 46) {
		t.Error("30 °N is in the 46 °N DMC band; cffdrs puts it in the 20 °N band (lat <= 30)")
	}
}

// The start-up codes are the values fwi() uses with no init, and StartupState
// is exactly them.
func TestStartupState(t *testing.T) {
	if got := StartupState(); got != (State{85, 6, 15}) {
		t.Errorf("StartupState() = %+v, want {85 6 15}", got)
	}
}
