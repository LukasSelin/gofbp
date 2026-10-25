package fbp

import (
	"errors"
	"sort"
	"testing"

	"github.com/LukasSelin/gofbp/fuel"
	"github.com/LukasSelin/gofbp/internal/cffdrstest"
)

// The driver against the oracle: Predict, end to end, on every row of the
// fixture.
//
// Oracle case (b). Most of what Predict returns was already a fixture column,
// asserted function by function elsewhere; what this test adds is the claim that
// the COMPOSITION is fbp()'s — every row fed to Predict exactly as it was fed to
// fbp(), from FFMC, BUI, wind, slope, site and the sent CBH/CFL, and every
// output compared. The generator gained only the outputs fbp() computes inline
// (raz, fcfb, bcfb, ftfc, btfc, ffi, bfi, ti, fti, bti), no rows.
//
// Two things make that a fair comparison and both are opt-ins Predict was
// asked for by name, not defaults it applied: Crown is CrownCFFDRS, because the
// fixture sends fbp() CBH/CFL sentinels and out-of-range values on purpose; and
// FMC comes from a Site, because the fixture sends none and fbp() computes it.
// The grass fuel load is passed as the 0.35 the generator sends.
//
// A row testdata/divergences.json covers is held to its entry instead: today
// that is C6 on a slope, which Predict must refuse with the entry's error
// (ErrC6OnSlope, see predict.go). A row no entry covers must be computed and
// must match, so a refusal or a disagreement the register does not know about
// fails here.
//
// ledger: fire_behaviour_prediction.r
func TestCFFDRSPredict(t *testing.T) {
	f := cffdrstest.Load(t)
	reg := cffdrstest.LoadRegister(t)
	const tol = 1e-9

	emitted := false
	for _, c := range f.Cases {
		if c.RAZ != 0 || c.TI != 0 {
			emitted = true
			break
		}
	}
	if !emitted {
		t.Fatal("the fixture has no raz/ti columns: it predates the driver port " +
			"(testdata/README.md lists it among the earlier digests). Regenerate it.")
	}

	type stat struct {
		worst float64
		bad   int
	}
	stats := map[string]*stat{}
	perFuel := map[string]int{}
	refused, shown := 0, 0
	for i, c := range f.Cases {
		fuelCode := cffdrstest.Fuel(c.Fuel)
		p, err := Predict(inputFromCase(c))
		if d := reg.Find("cases", cffdrstest.CaseGetter(c), "*"); d != nil {
			if want := sentinelErrors[d.Error]; !errors.Is(err, want) {
				t.Errorf("case %d %s gs=%v: err = %v, want %v (divergence %s)", i, c.Fuel, c.GS, err, want, d.ID)
			}
			refused++
			continue
		}
		if err != nil {
			t.Errorf("case %d %s: %v, and no entry in %s explains the refusal", i, c.Fuel, err, cffdrstest.RegisterPath)
			continue
		}
		perFuel[fuelCode]++
		if string(p.FD) != c.FD {
			if s := stats["fd"]; s == nil {
				stats["fd"] = &stat{}
			}
			stats["fd"].bad++
		}
		for _, col := range []struct {
			name      string
			got, want float64
		}{
			{"fmc", p.FMC, c.FMC}, {"sfc", p.SFC, c.SFC}, {"isi", p.ISI, c.ISI}, {"wsv", p.WSV, c.WSV},
			{"be", p.BE, c.BE}, {"sf", p.SF, c.SF}, {"csi", p.CSI, c.CSI}, {"rso", p.RSO, c.RSO},
			{"ros", p.ROS, c.ROS}, {"cfb", p.CFB, c.CFB}, {"cfc", p.CFC, c.CFC}, {"tfc", p.TFC, c.TFC},
			{"hfi", p.HFI, c.HFI}, {"lb", p.LB, c.LB}, {"bros", p.BROS, c.BROS}, {"fros", p.FROS, c.FROS},
			{"fcfb", p.FCFB, c.FCFB}, {"bcfb", p.BCFB, c.BCFB}, {"ftfc", p.FTFC, c.FTFC},
			{"btfc", p.BTFC, c.BTFC}, {"ffi", p.FFI, c.FFI}, {"bfi", p.BFI, c.BFI},
			{"ti", p.TI, c.TI}, {"fti", p.FTI, c.FTI}, {"bti", p.BTI, c.BTI},
			{"hrost", p.HROSt, c.HROSt}, {"brost", p.BROSt, c.BROSt}, {"frost", p.FROSt, c.FROSt},
			{"lbt", p.LBt, c.LBt}, {"dh", p.DH, c.DH}, {"db", p.DB, c.DB}, {"df", p.DF, c.DF},
			{"raz", cffdrstest.AngleDiff(p.RAZ, c.RAZ), 0},
		} {
			s := stats[col.name]
			if s == nil {
				s = &stat{}
				stats[col.name] = s
			}
			s.worst = cffdrstest.MaxF(s.worst, cffdrstest.RelErr(col.got, col.want))
			if !cffdrstest.CloseEnough(col.got, col.want, tol) {
				s.bad++
				if shown++; shown <= 15 {
					t.Errorf("case %d %s ffmc=%v ws=%v wd=%v gs=%v cbh=%v cfl=%v hr=%v accel=%v: %s = %v, cffdrs %v",
						i, c.Fuel, c.FFMC, c.WS, c.WD, c.GS, c.CBH, c.CFL, c.HR, c.Accel, col.name, col.got, col.want)
				}
			}
		}
	}

	names := make([]string, 0, len(stats))
	for k := range stats {
		names = append(names, k)
	}
	sort.Strings(names)
	total := 0
	for _, k := range names {
		total += stats[k].bad
		t.Logf("  %-6s %5d mismatched, worst relative error %.3g", k, stats[k].bad, stats[k].worst)
	}
	if total > 0 {
		t.Errorf("%d column mismatches in all", total)
	}
	n := 0
	for _, v := range perFuel {
		n += v
	}
	t.Logf("Predict %d rows end to end, %d refused as the register says, %d fuels", n, refused, len(perFuel))
	if len(perFuel) < len(fuel.Fuels) {
		t.Errorf("only %d of %d fuels reached", len(perFuel), len(fuel.Fuels))
	}
	if refused == 0 {
		t.Error("no row the register covers was reached — its refusals are unasserted against the sweep")
	}
}

// inputFromCase is a fixture row as fbp() received it, translated once at the
// boundary: wind and aspect are "from" in fbp() and "towards" here, and hours
// are minutes.
func inputFromCase(c cffdrstest.Case) Input {
	return Input{
		Fuel: cffdrstest.Fuel(c.Fuel), FFMC: c.FFMC, BUI: c.BUI,
		WindKmh: c.WS, WindAzimuthDeg: c.WD + 180,
		SlopePct: c.GS, UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect is 0
		PC: c.PC, PDF: c.PDF, CuringPct: c.CC,
		GrassFuelLoad: cffdrstest.SweepGrassFuelLoad,
		Site:          &Site{Lat: c.LAT, Long: c.LONG, ElevM: c.ELV, DayOfYear: c.DJ, D0: c.D0},
		Crown:         CrownCFFDRS, CBH: c.CBH, CFL: c.CFL, StandDensity: c.SD, StandHeight: c.SH,
		ElapsedMin: c.HR * 60, PointIgnition: c.Accel == 1,
	}
}
