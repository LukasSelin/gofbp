package fbp

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestFuelFromGridCode(t *testing.T) {
	// fbpRaster()'s table is sort() of cffdrs' spellings, which puts NF and WA
	// inside the list rather than after it; the roxygen table agrees.
	want := map[float64]string{1: "C1", 6: "C6", 8: "D1", 12: "M4", 13: "NF", 14: "O1A", 15: "O1B", 18: "S3", 19: "WA"}
	for code, name := range want {
		if got, ok := FuelFromGridCode(code); !ok || got != name {
			t.Errorf("FuelFromGridCode(%v) = %q, %v; want %q", code, got, ok, name)
		}
	}
	for _, code := range []float64{0, -1, 20, 7.5, 1.0000001, math.NaN(), math.Inf(1)} {
		if got, ok := FuelFromGridCode(code); ok {
			t.Errorf("FuelFromGridCode(%v) = %q, want refusal", code, got)
		}
	}
	// Every code but the two non-fuels names a fuel this package implements.
	for i, name := range GridFuelCodes {
		_, ok := CanonicalFuelCode(name)
		if nonFuel := name == "NF" || name == "WA"; ok == nonFuel {
			t.Errorf("code %d %q: implemented = %v", i+1, name, ok)
		}
	}
}

func TestFireDescriptionCode(t *testing.T) {
	for d, want := range map[FireDescription]float64{SurfaceFire: 1, IntermittentCrown: 2, ContinuousCrown: 3} {
		if got := d.Code(); got != want {
			t.Errorf("%q.Code() = %v, want %v", d, got, want)
		}
	}
	if got := FireDescription("").Code(); !math.IsNaN(got) {
		t.Errorf(`"".Code() = %v, want NaN`, got)
	}
}

// gridOf is a small grid mixing fuels, slopes and a refused cell or two.
func gridOf() Grid {
	return Grid{
		Fuel:              []float64{2, 13, 6, 6, 14, 0, 9, 3},
		FFMC:              []float64{92, 92, 90, 90, 88, 92, 91, math.NaN()},
		BUI:               []float64{80, 80, 60, 60, 40, 80, 70, 50},
		WindKmh:           []float64{20, 20, 15, 15, 25, 20, 10, 5},
		WindAzimuthDeg:    []float64{180, 180, 90, 90, 0, 180, 270, 45},
		SlopePct:          []float64{10, 10, 0, 30, 0, 10, 50, 0},
		UpslopeAzimuthDeg: []float64{270, 270, 0, 0, 0, 270, 90, 0},
		PC:                []float64{50, 50, 50, 50, 50, 50, 60, 50},
		PDF:               []float64{35, 35, 35, 35, 35, 35, 35, 35},
		CuringPct:         []float64{90, 90, 90, 90, 90, 90, 90, 90},
		GrassFuelLoad:     []float64{0.35, 0.35, 0.35, 0.35, 0.35, 0.35, 0.35, 0.35},
		Lat:               []float64{50, 50, 50, 50, 50, 50, 50, 50},
		Long:              []float64{-110, -110, -110, -110, -110, -110, -110, -110},
		DayOfYear:         []float64{180, 180, 180, 180, 180, 180, 180, 180},
		CBH:               []float64{-1, -1, -1, -1, -1, -1, -1, -1},
		CFL:               []float64{-1, -1, -1, -1, -1, -1, -1, -1},
	}
}

// PredictGrid is Predict per cell: the same numbers, bit for bit, and the same
// errors, with the two refusals of its own (a code outside 1-19) on top.
func TestPredictGridIsPredictPerCell(t *testing.T) {
	g := gridOf()
	opt := GridOptions{Crown: CrownCFFDRS}
	preds, errs, err := PredictGrid(g, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(preds) != len(g.Fuel) || len(errs) != len(g.Fuel) {
		t.Fatalf("got %d predictions and %d errors for %d cells", len(preds), len(errs), len(g.Fuel))
	}
	for i := range g.Fuel {
		fuel, ok := FuelFromGridCode(g.Fuel[i])
		if !ok {
			if !errors.Is(errs[i], ErrGridFuelCode) {
				t.Errorf("cell %d: err = %v, want ErrGridFuelCode", i, errs[i])
			}
			continue
		}
		want, wantErr := Predict(Input{
			Fuel: fuel, FFMC: g.FFMC[i], BUI: g.BUI[i], WindKmh: g.WindKmh[i],
			WindAzimuthDeg: g.WindAzimuthDeg[i], SlopePct: g.SlopePct[i],
			UpslopeAzimuthDeg: g.UpslopeAzimuthDeg[i], PC: g.PC[i], PDF: g.PDF[i],
			CuringPct: g.CuringPct[i], GrassFuelLoad: g.GrassFuelLoad[i],
			Site:  &Site{Lat: g.Lat[i], Long: g.Long[i], DayOfYear: g.DayOfYear[i]},
			Crown: CrownCFFDRS, CBH: g.CBH[i], CFL: g.CFL[i],
		})
		if (wantErr == nil) != (errs[i] == nil) || (wantErr != nil && wantErr.Error() != errs[i].Error()) {
			t.Errorf("cell %d %s: err = %v, Predict says %v", i, fuel, errs[i], wantErr)
			continue
		}
		if wantErr == nil && !reflect.DeepEqual(preds[i], want) {
			t.Errorf("cell %d %s: PredictGrid differs from Predict:\n%+v\n%+v", i, fuel, preds[i], want)
		}
	}
	for i, want := range map[int]error{1: ErrUnknownFuel, 3: ErrC6OnSlope, 5: ErrGridFuelCode, 7: ErrInvalidInput} {
		if !errors.Is(errs[i], want) {
			t.Errorf("cell %d: err = %v, want %v", i, errs[i], want)
		}
	}
}

// A refused cell is NaN in every number, so a caller writing layers out cannot
// mistake it for a cell that will not burn.
func TestPredictGridRefusedCellIsNaN(t *testing.T) {
	preds, errs, err := PredictGrid(gridOf(), GridOptions{Crown: CrownCFFDRS})
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for i, p := range preds {
		if errs[i] == nil {
			continue
		}
		refused++
		v := reflect.ValueOf(p)
		for j := 0; j < v.NumField(); j++ {
			if fv := v.Field(j); fv.Kind() == reflect.Float64 && !math.IsNaN(fv.Float()) {
				t.Errorf("cell %d: %s = %v, want NaN", i, v.Type().Field(j).Name, fv.Float())
			}
		}
		if p.FD != "" || !math.IsNaN(p.FD.Code()) {
			t.Errorf("cell %d: FD = %q", i, p.FD)
		}
	}
	if refused == 0 {
		t.Fatal("gridOf refuses no cell")
	}
}

// fbpRaster() drops a cell that is NA in every layer and writes the shorter
// result back by position, shifting every later cell. PredictGrid must not: an
// all-NaN cell is refused and its neighbours are untouched.
func TestPredictGridAllNaNCellMovesNothing(t *testing.T) {
	g := gridOf()
	before, _, err := PredictGrid(g, GridOptions{Crown: CrownCFFDRS})
	if err != nil {
		t.Fatal(err)
	}
	holed := gridOf()
	v := reflect.ValueOf(&holed).Elem()
	for j := 0; j < v.NumField(); j++ {
		if s, ok := v.Field(j).Interface().([]float64); ok && s != nil {
			s[2] = math.NaN()
		}
	}
	after, errs, err := PredictGrid(holed, GridOptions{Crown: CrownCFFDRS})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(errs[2], ErrGridFuelCode) {
		t.Errorf("all-NaN cell: err = %v, want ErrGridFuelCode", errs[2])
	}
	for i := range before {
		if i != 2 && !reflect.DeepEqual(nanSafe(before[i]), nanSafe(after[i])) {
			t.Errorf("cell %d moved when cell 2 went NaN", i)
		}
	}
}

// nanSafe makes NaN fields compare equal under reflect.DeepEqual.
func nanSafe(p Prediction) Prediction {
	v := reflect.ValueOf(&p).Elem()
	for j := 0; j < v.NumField(); j++ {
		if f := v.Field(j); f.Kind() == reflect.Float64 && math.IsNaN(f.Float()) {
			f.SetFloat(-12345)
		}
	}
	return p
}

// Cells are independent, so splitting the slices — the caller's way to run
// PredictGrid in parallel — gives bit-identical output.
func TestPredictGridSplitIsBitIdentical(t *testing.T) {
	g := gridOf()
	whole, _, err := PredictGrid(g, GridOptions{Crown: CrownCFFDRS})
	if err != nil {
		t.Fatal(err)
	}
	slice := func(g Grid, lo, hi int) Grid {
		out := Grid{}
		src, dst := reflect.ValueOf(g), reflect.ValueOf(&out).Elem()
		for j := 0; j < src.NumField(); j++ {
			if s, ok := src.Field(j).Interface().([]float64); ok && s != nil {
				dst.Field(j).Set(reflect.ValueOf(s[lo:hi]))
			}
		}
		return out
	}
	for _, cut := range []int{1, 3, 5} {
		a, _, err1 := PredictGrid(slice(g, 0, cut), GridOptions{Crown: CrownCFFDRS})
		b, _, err2 := PredictGrid(slice(g, cut, len(g.Fuel)), GridOptions{Crown: CrownCFFDRS})
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		for i, p := range append(a, b...) {
			if !reflect.DeepEqual(nanSafe(p), nanSafe(whole[i])) {
				t.Errorf("cut %d: cell %d differs from the whole-grid run", cut, i)
			}
		}
	}
}

func TestPredictGridLayerLengths(t *testing.T) {
	g := gridOf()
	g.BUI = g.BUI[:3]
	preds, errs, err := PredictGrid(g, GridOptions{Crown: CrownCFFDRS})
	var ge *GridError
	if !errors.As(err, &ge) || ge.Layer != "BUI" || preds != nil || errs != nil {
		t.Errorf("short BUI layer: preds %v, errs %v, err %v", preds, errs, err)
	}

	// A nil layer is the Input field's zero value: no Site without Lat, so a
	// crowned fuel with no FMC layer is ErrNoFMC, and a crownless one computes.
	g = gridOf()
	g.Lat = nil
	_, errs, err = PredictGrid(g, GridOptions{Crown: CrownCFFDRS})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(errs[0], ErrNoFMC) {
		t.Errorf("C2 with no Lat or FMC layer: err = %v, want ErrNoFMC", errs[0])
	}
	if errs[4] != nil {
		t.Errorf("O1A with no Lat layer: %v", errs[4])
	}

	// An empty grid is no cells and no error.
	preds, errs, err = PredictGrid(Grid{}, GridOptions{Crown: CrownCFFDRS})
	if err != nil || len(preds) != 0 || len(errs) != 0 {
		t.Errorf("empty grid: %v %v %v", preds, errs, err)
	}

	// CrownUnset is refused per cell, as Predict refuses it.
	_, errs, _ = PredictGrid(gridOf(), GridOptions{})
	if !errors.Is(errs[0], ErrCrownUnset) {
		t.Errorf("CrownUnset: err = %v", errs[0])
	}
}
