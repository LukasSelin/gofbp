package fbp

import (
	"errors"
	"math"
	"sort"
	"testing"

	"github.com/LukasSelin/gofbp/internal/cffdrstest"
	"github.com/LukasSelin/gofbp/internal/oracle"
)

// fbpGridInputs are the layers the generator sends fbpRaster(), as named in the
// fixture (an input that is also an output carries _in).
var fbpGridInputs = []string{
	"fuel_code", "lat", "long", "elv", "ffmc_in", "bui", "ws", "wd", "gs", "dj", "aspect",
	"pc", "pdf", "gfl", "cc", "cbh", "cfl", "fmc_in", "d0_in", "sd", "sh", "hr",
	"accel", "theta", "isi_in", "buieff",
}

// PredictGrid against fbpRaster(), cell for cell.
//
// Oracle case (b): the generator builds small terra grids and records every
// cell's fbpRaster() output under fbp_grid_cases. Each grid is rebuilt here as a
// Grid — wind and aspect turned from "from" to "towards", hours to minutes —
// and passed to PredictGrid in one call, so what is asserted is the grid entry
// point, not Predict cell by cell.
//
// Every cell is one of two things, and testdata/divergences.json says which:
//
//   - Computed: every numeric output and FD (as 1/2/3) equal fbpRaster()'s.
//     That is every cell no register entry covers.
//   - Refused, where fbpRaster() filled the cell in. A gofbp-refuses entry names
//     the error, and the refusal has to be that one: ErrGridFuelCode for a code
//     outside 1-19 (cffdrs ran C2), ErrUnknownFuel for NF and WA (cffdrs wrote
//     zeros), ErrC6OnSlope for C6 on a slope. A cell with an NA input falls
//     under a gofbp-may-refuse entry: refused for any reason, since fbp()
//     substituted a default there, or computed, and then it must match.
//
// A refused cell no entry covers fails the test. The all-NA cell is not in the
// fixture: fbpRaster() shifts every later cell when given one, and the generator
// asserts that rather than recording it (see PredictGrid, and the register's
// grid-all-na-cell-shifts).
//
// ledger: fbpRaster.r
func TestCFFDRSPredictGrid(t *testing.T) {
	f := cffdrstest.Load(t)
	reg := cffdrstest.LoadRegister(t)
	if len(f.FBPGrid) == 0 {
		t.Fatal("the fixture has no fbp_grid_cases: it predates the fbpRaster() block " +
			"(testdata/README.md lists it among the earlier digests). Regenerate it.")
	}
	const tol = 1e-9

	byGrid := map[string][]cffdrstest.GridCase{}
	for _, c := range f.FBPGrid {
		byGrid[c.Grid] = append(byGrid[c.Grid], c)
	}
	ids := make([]string, 0, len(byGrid))
	for id := range byGrid {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	outCols := []string{
		"cfb", "cfc", "fd", "hfi", "raz", "ros", "sfc", "tfc", "be", "sf", "isi",
		"fmc", "rso", "csi", "fros", "bros", "hrost", "frost", "brost", "fcfb",
		"bcfb", "ffi", "bfi", "ftfc", "btfc", "ti", "fti", "bti", "lb", "lbt",
		"wsv", "dh", "db", "df",
	}
	worst := map[string]float64{}
	computed, refused, naRefused, shown := 0, 0, 0, 0
	why := map[string]int{}
	codes := map[float64]bool{}
	fds := map[float64]int{}
	for _, id := range ids {
		cells := byGrid[id]
		sort.Slice(cells, func(i, j int) bool { return cells[i].Cell < cells[j].Cell })
		n := len(cells)
		for i, c := range cells {
			if c.Cell != i+1 {
				t.Fatalf("%s: cells are not 1..%d (cell %d at %d)", id, n, c.Cell, i)
			}
			for _, k := range append(append([]string{}, fbpGridInputs...), outCols...) {
				if !c.Has(k) {
					t.Fatalf("%s cell %d: no %q column", id, c.Cell, k)
				}
			}
		}
		layer := func(fn func(c cffdrstest.GridCase) float64) []float64 {
			v := make([]float64, n)
			for i, c := range cells {
				v[i] = fn(c)
			}
			return v
		}
		col := func(k string) []float64 { return layer(func(c cffdrstest.GridCase) float64 { return c.Col(k) }) }
		accel := math.NaN()
		for _, c := range cells {
			if a := c.Col("accel"); !math.IsNaN(a) {
				if !math.IsNaN(accel) && a != accel {
					t.Fatalf("%s: accel varies across the grid; PredictGrid takes it once", id)
				}
				accel = a
			}
		}
		g := Grid{
			Fuel: col("fuel_code"), FFMC: col("ffmc_in"), BUI: col("bui"),
			WindKmh:           col("ws"),
			WindAzimuthDeg:    layer(func(c cffdrstest.GridCase) float64 { return c.Col("wd") + 180 }),
			SlopePct:          col("gs"),
			UpslopeAzimuthDeg: layer(func(c cffdrstest.GridCase) float64 { return c.Col("aspect") + 180 }),
			PC:                col("pc"), PDF: col("pdf"), CuringPct: col("cc"), GrassFuelLoad: col("gfl"),
			FMC: col("fmc_in"), Lat: col("lat"), Long: col("long"), ElevM: col("elv"),
			DayOfYear: col("dj"), D0: col("d0_in"),
			CBH: col("cbh"), CFL: col("cfl"), StandDensity: col("sd"), StandHeight: col("sh"),
			ElapsedMin: layer(func(c cffdrstest.GridCase) float64 { return c.Col("hr") * 60 }),
		}
		preds, errs, err := PredictGrid(g, GridOptions{Crown: CrownCFFDRS, PointIgnition: accel == 1})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}

		for i, c := range cells {
			code := c.Col("fuel_code")
			codes[code] = true
			fuel, _ := FuelFromGridCode(code)
			d := reg.Find("fbp_grid_cases", cffdrstest.GridGetter(c), "*")
			switch {
			case d != nil && d.Verdict == oracle.GoFBPRefuses:
				if want := sentinelErrors[d.Error]; !errors.Is(errs[i], want) {
					t.Errorf("%s cell %d code %v: err = %v, want %v (divergence %s)", id, c.Cell, code, errs[i], want, d.ID)
				}
				refused++
				why[d.ID]++
				continue
			case d != nil && d.Verdict == oracle.GoFBPMayRefuse && errs[i] != nil:
				refused++
				naRefused++
				why[d.ID+": "+errs[i].Error()]++
				continue
			case d != nil && d.Verdict != oracle.GoFBPMayRefuse:
				t.Fatalf("%s: verdict %s in fbp_grid_cases, which this test does not know how to hold", d.ID, d.Verdict)
			case errs[i] != nil:
				t.Errorf("%s cell %d code %v: refused, and no entry in %s explains it: %v",
					id, c.Cell, code, cffdrstest.RegisterPath, errs[i])
				refused++
				continue
			}
			computed++
			p := preds[i]
			fds[c.Col("fd")]++
			got := map[string]float64{
				"cfb": p.CFB, "cfc": p.CFC, "fd": p.FD.Code(), "hfi": p.HFI, "ros": p.ROS,
				"sfc": p.SFC, "tfc": p.TFC, "be": p.BE, "sf": p.SF, "isi": p.ISI, "fmc": p.FMC,
				"rso": p.RSO, "csi": p.CSI, "fros": p.FROS, "bros": p.BROS, "hrost": p.HROSt,
				"frost": p.FROSt, "brost": p.BROSt, "fcfb": p.FCFB, "bcfb": p.BCFB, "ffi": p.FFI,
				"bfi": p.BFI, "ftfc": p.FTFC, "btfc": p.BTFC, "ti": p.TI, "fti": p.FTI, "bti": p.BTI,
				"lb": p.LB, "lbt": p.LBt, "wsv": p.WSV, "dh": p.DH, "db": p.DB, "df": p.DF,
			}
			for _, k := range outCols {
				g, w := got[k], c.Col(k)
				if k == "raz" {
					g, w = cffdrstest.AngleDiff(p.RAZ, w), 0
				}
				worst[k] = cffdrstest.MaxF(worst[k], cffdrstest.RelErr(g, w))
				if !cffdrstest.CloseEnough(g, w, tol) {
					if shown++; shown <= 15 {
						t.Errorf("%s cell %d code %v (%s): %s = %v, fbpRaster %v", id, c.Cell, code, fuel, k, g, w)
					}
				}
			}
		}
	}

	if shown > 0 {
		t.Errorf("%d mismatched outputs in all", shown)
	}
	keys := make([]string, 0, len(worst))
	for k := range worst {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	overall, at := 0.0, ""
	for _, k := range keys {
		if worst[k] > overall {
			overall, at = worst[k], k
		}
	}
	t.Logf("%d grids, %d cells: %d computed (worst relative error %.3g, %s), %d refused", len(ids),
		len(f.FBPGrid), computed, overall, at, refused)
	whys := make([]string, 0, len(why))
	for k := range why {
		whys = append(whys, k)
	}
	sort.Strings(whys)
	for _, k := range whys {
		t.Logf("  refused %4d  %s", why[k], k)
	}
	t.Logf("FD over computed cells: S %d, I %d, C %d", fds[1], fds[2], fds[3])

	for code := 1.0; code <= 19; code++ {
		if !codes[code] {
			t.Errorf("fuel code %v never appears in the grids", code)
		}
	}
	for _, code := range []float64{0, 20, 7.5} {
		if !codes[code] {
			t.Errorf("out-of-range code %v never appears in the grids", code)
		}
	}
	if fds[1] == 0 || fds[2] == 0 || fds[3] == 0 {
		t.Errorf("not every FD class is reached: %v", fds)
	}
	if naRefused == 0 {
		t.Error("no NA input was refused — the NA grid is not reaching PredictGrid's refusals")
	}
}
