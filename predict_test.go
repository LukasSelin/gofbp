package fbp

import (
	"errors"
	"math"
	"testing"

	"github.com/LukasSelin/gofbp/consumption"
	"github.com/LukasSelin/gofbp/crown"
	"github.com/LukasSelin/gofbp/internal/cffdrstest"
	"github.com/LukasSelin/gofbp/spread"
)

// The driver, without the fixture: every refusal it makes in place of a
// default fbp() would apply, the two opt-ins, and one worked point copied from
// cffdrs so CI checks the composition on every push.

// validC2 is a complete, valid input: the C2 worked point used elsewhere, with
// cffdrs' published numbers in TestPredictMatchesACFFDRSWorkedPoint.
func validC2() Input {
	return Input{
		Fuel: "C2", FFMC: 92, BUI: 40, WindKmh: 30, WindAzimuthDeg: 180,
		PC: 50, PDF: 35, CuringPct: 80,
		FMC: 99.8176, Crown: CrownAsGiven, CBH: 20, CFL: 1,
		ElapsedMin: 60,
	}
}

func TestPredictMatchesACFFDRSWorkedPoint(t *testing.T) {
	p, err := Predict(validC2())
	if err != nil {
		t.Fatal(err)
	}
	const tol = 1e-9
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"CFB", p.CFB, 0.819871907826142},
		{"TFC", p.TFC, 2.6634536802915125},
		{"ROS", p.ROS, 34.62953830166129},
		{"HFI", p.HFI, 27670.2513709067},
	} {
		if !cffdrstest.CloseEnough(c.got, c.want, tol) {
			t.Errorf("%s = %v, cffdrs %v", c.name, c.got, c.want)
		}
	}
	if p.FD != crown.IntermittentCrown {
		t.Errorf("FD = %q, want I", p.FD)
	}
	if p.RAZ != 180 {
		t.Errorf("RAZ = %v on flat ground, want the wind's 180", p.RAZ)
	}
	// Equilibrium without PointIgnition: the t-values are the equilibrium ones
	// and the distance is rate × time.
	if p.HROSt != p.ROS || p.LBt != p.LB || p.DH != p.ROS*60 {
		t.Errorf("no point ignition: HROSt %v ROS %v, LBt %v LB %v, DH %v ROS·t %v",
			p.HROSt, p.ROS, p.LBt, p.LB, p.DH, p.ROS*60)
	}
}

// The composition is the pieces: the driver adds wiring, not arithmetic.
func TestPredictIsTheComposition(t *testing.T) {
	in := validC2()
	in.PointIgnition, in.ElapsedMin = true, 20
	p, err := Predict(in)
	if err != nil {
		t.Fatal(err)
	}
	sfc := consumption.SurfaceFuelConsumption("C2", 92, 40, 50, 0)
	isi := spread.ISI(92, 30)
	rss := spread.RSI("C2", isi, 50, 35, 80) * spread.BuildupEffect("C2", 40)
	cfb := crown.CrownFractionBurned(crown.Crown{FMC: 99.8176, SFC: sfc, CBH: 20, CFL: 1, SurfaceROS: rss})
	bros := spread.RSI("C2", isi*spread.BackISIRatio(30), 50, 35, 80) * spread.BuildupEffect("C2", 40)
	lb := spread.LengthToBreadth("C2", 30)
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"SFC", p.SFC, sfc}, {"ISI", p.ISI, isi}, {"ROS", p.ROS, rss}, {"CFB", p.CFB, cfb},
		{"BROS", p.BROS, bros}, {"LB", p.LB, lb}, {"FROS", p.FROS, spread.FlankROS(rss, bros, lb)},
		{"HROSt", p.HROSt, spread.ROSAtTime("C2", rss, 20, cfb)},
		{"BROSt", p.BROSt, spread.ROSAtTime("C2", bros, 20, cfb)}, // the HEAD cfb, as fbp() uses
		{"LBt", p.LBt, spread.LengthToBreadthAtTime("C2", lb, 20, cfb)},
		{"DH", p.DH, spread.DistanceAtTime("C2", rss, 20, cfb)},
		{"HFI", p.HFI, consumption.FireIntensity(consumption.TotalFuelConsumption("C2", 1, cfb, sfc, 50, 35), rss)},
	} {
		if !cffdrstest.CloseEnough(c.got, c.want, 1e-15) {
			t.Errorf("%s = %v, composed from the pieces %v", c.name, c.got, c.want)
		}
	}
}

// Every default fbp() would apply is an error here.
func TestPredictRefusesWhatFBPWouldDefault(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
		want error
	}{
		{"zero-value Input", func(in *Input) { *in = Input{} }, ErrUnknownFuel},
		{"crown source unset", func(in *Input) { in.Crown = CrownUnset }, ErrCrownUnset},
		{"no FMC and no site", func(in *Input) { in.FMC = 0 }, ErrNoFMC},
		{"FMC above 120 and no site", func(in *Input) { in.FMC = 150 }, ErrNoFMC},
		{"grass with no fuel load", func(in *Input) { in.Fuel = "O1A" }, ErrNoGrassLoad},
		{"unknown fuel", func(in *Input) { in.Fuel = "D2" }, ErrUnknownFuel},
		{"C6 on a slope", func(in *Input) { in.Fuel, in.SlopePct = "C6", 10 }, ErrC6OnSlope},
		{"FFMC above 101", func(in *Input) { in.FFMC = 102 }, ErrInvalidInput},
		{"negative BUI", func(in *Input) { in.BUI = -1 }, ErrInvalidInput},
		{"NaN wind", func(in *Input) { in.WindKmh = math.NaN() }, ErrInvalidInput},
		{"negative slope", func(in *Input) { in.SlopePct = -5 }, ErrInvalidInput},
		{"PC above 100", func(in *Input) { in.PC = 120 }, ErrInvalidInput},
		{"infinite azimuth", func(in *Input) { in.WindAzimuthDeg = math.Inf(1) }, ErrInvalidInput},
		{"negative time", func(in *Input) { in.ElapsedMin = -1 }, ErrInvalidInput},
		{"as-given NaN CBH", func(in *Input) { in.CBH = math.NaN() }, ErrInvalidInput},
		{"as-given negative CFL", func(in *Input) { in.CFL = -1 }, ErrInvalidInput},
	}
	for _, c := range cases {
		in := validC2()
		c.edit(&in)
		if _, err := Predict(in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// The opt-ins do what fbp() does, and only when asked.
func TestPredictOptIns(t *testing.T) {
	// CrownCFFDRS with the sentinel takes the published table...
	in := validC2()
	in.Crown, in.CBH, in.CFL = CrownCFFDRS, -1, -1
	p, err := Predict(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.CBH != 3 || p.CFL != 0.8 {
		t.Errorf("CrownCFFDRS with sentinels resolved CBH %v CFL %v, want C2's 3 and 0.8", p.CBH, p.CFL)
	}
	// ...and with in-range values agrees with CrownAsGiven exactly.
	a, _ := Predict(validC2())
	in = validC2()
	in.Crown = CrownCFFDRS
	b, _ := Predict(in)
	if a != b {
		t.Errorf("CrownCFFDRS with in-range CBH/CFL differs from CrownAsGiven:\n%+v\n%+v", a, b)
	}
	// CrownAsGiven takes a 0 CFL at its word: the stand cannot crown.
	in = validC2()
	in.CFL = 0
	if p, _ := Predict(in); p.CFB != 0 || p.CFC != 0 {
		t.Errorf("CrownAsGiven CFL 0: CFB %v CFC %v, want 0", p.CFB, p.CFC)
	}
	// A Site is used only when FMC is not usable, and the given FMC wins.
	site := &Site{Lat: 45, Long: -80, DayOfYear: 175}
	in = validC2()
	in.FMC, in.Site = 0, site
	p, _ = Predict(in)
	if want := crown.FoliarMoistureContent(45, 80, 0, 175, 0); p.FMC != want {
		t.Errorf("FMC from site = %v, want %v (west longitude folded)", p.FMC, want)
	}
	in.FMC = 110
	if p, _ := Predict(in); p.FMC != 110 {
		t.Errorf("FMC 110 with a site = %v, want the given 110", p.FMC)
	}
	// The crownless fuels need no FMC and get 0, as fbp() forces.
	in = Input{Fuel: "D1", FFMC: 90, BUI: 60, Crown: CrownCFFDRS, CBH: -1, CFL: -1}
	if p, err := Predict(in); err != nil || p.FMC != 0 || p.CFB != 0 || p.CSI != 0 {
		t.Errorf("D1: err %v FMC %v CFB %v CSI %v, want nil 0 0 0", err, p.FMC, p.CFB, p.CSI)
	}
}

// fbp() zeroes the flank and back crown fractions for C6 rather than running
// C6's crown path at those rates. Reproduced.
func TestPredictC6SideFiresDoNotCrown(t *testing.T) {
	in := validC2()
	in.Fuel, in.BUI, in.CBH = "C6", 100, 2
	p, err := Predict(in)
	if err != nil {
		t.Fatal(err)
	}
	if !(p.CFB > 0) {
		t.Fatalf("this C6 point should crown at the head; CFB = %v", p.CFB)
	}
	if p.FCFB != 0 || p.BCFB != 0 {
		t.Errorf("C6 FCFB %v BCFB %v, want 0 (fbp()'s C6 exclusion)", p.FCFB, p.BCFB)
	}
}
