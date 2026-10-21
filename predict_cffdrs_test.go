package fbp

import (
	"errors"
	"math"
	"sort"
	"testing"
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
// C6 on a slope is the one exclusion, and it is asserted rather than skipped:
// Predict must refuse it with ErrC6OnSlope (see predict.go).
//
// ledger: fire_behaviour_prediction.r
func TestCFFDRSPredict(t *testing.T) {
	f := loadCFFDRS(t)
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
		fuel := ourFuel(c.Fuel)
		p, err := Predict(inputFromCase(c))
		if fuel == "C6" && c.GS > 0 {
			if !errors.Is(err, ErrC6OnSlope) {
				t.Errorf("case %d C6 gs=%v: err = %v, want ErrC6OnSlope", i, c.GS, err)
			}
			refused++
			continue
		}
		if err != nil {
			t.Errorf("case %d %s: %v", i, c.Fuel, err)
			continue
		}
		perFuel[fuel]++
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
			{"raz", angleDiff(p.RAZ, c.RAZ), 0},
		} {
			s := stats[col.name]
			if s == nil {
				s = &stat{}
				stats[col.name] = s
			}
			s.worst = maxF(s.worst, relErr(col.got, col.want))
			if !closeEnough(col.got, col.want, tol) {
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
	t.Logf("Predict %d rows end to end, %d C6-on-slope rows refused, %d fuels", n, refused, len(perFuel))
	if len(perFuel) < len(Fuels) {
		t.Errorf("only %d of %d fuels reached", len(perFuel), len(Fuels))
	}
	if refused == 0 {
		t.Error("no C6 row on a slope — ErrC6OnSlope is unasserted against the sweep")
	}
}

// inputFromCase is a fixture row as fbp() received it, translated once at the
// boundary: wind and aspect are "from" in fbp() and "towards" here, and hours
// are minutes.
func inputFromCase(c cffdrsCase) Input {
	return Input{
		Fuel: ourFuel(c.Fuel), FFMC: c.FFMC, BUI: c.BUI,
		WindKmh: c.WS, WindAzimuthDeg: c.WD + 180,
		SlopePct: c.GS, UpslopeAzimuthDeg: 0 + 180, // the fixture's Aspect is 0
		PC: c.PC, PDF: c.PDF, CuringPct: c.CC,
		GrassFuelLoad: sweepGrassFuelLoad,
		Site:          &Site{Lat: c.LAT, Long: c.LONG, ElevM: c.ELV, DayOfYear: c.DJ, D0: c.D0},
		Crown:         CrownCFFDRS, CBH: c.CBH, CFL: c.CFL, StandDensity: c.SD, StandHeight: c.SH,
		ElapsedMin: c.HR * 60, PointIgnition: c.Accel == 1,
	}
}

// angleDiff is the unsigned separation of two azimuths, so 359.9 and 0.1 are
// 0.2 apart rather than 359.8.
func angleDiff(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return math.Min(d, 360-d)
}
