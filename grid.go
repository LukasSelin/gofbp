package fbp

import "math"

// GridFuelCodes is fbpRaster()'s fuel-code table: code i+1 is GridFuelCodes[i].
// It is cffdrs' sort() of the fuel names, which is why NF lands at 13 and WA at
// 19 rather than at the end. The spellings are this package's (see
// CanonicalFuelCode); NF and WA are cffdrs' non-fuel classes, and Predict
// refuses them.
var GridFuelCodes = [19]string{
	"C1", "C2", "C3", "C4", "C5", "C6", "C7", "D1",
	"M1", "M2", "M3", "M4", "NF", "O1A", "O1B", "S1", "S2", "S3", "WA",
}

// FuelFromGridCode is the fuel a numeric fbpRaster() code names. The second
// return is false for anything that is not exactly one of 1-19 — 0, 20, 7.5 or
// NaN — which fbpRaster() would quietly turn into C2 (see PredictGrid).
func FuelFromGridCode(code float64) (string, bool) {
	if !(code >= 1 && code <= float64(len(GridFuelCodes))) || code != math.Trunc(code) {
		return "", false
	}
	return GridFuelCodes[int(code)-1], true
}

// ErrGridFuelCode is PredictGrid's refusal of a cell whose fuel code is not one
// of fbpRaster()'s 1-19.
var ErrGridFuelCode = &PredictError{kind: "fuel code is not one of fbpRaster()'s 1-19"}

// Code is FD as fbpRaster() writes it into a layer: 1 surface, 2 intermittent
// crown, 3 continuous crown, and NaN for no description — a refused cell, or
// what fbpRaster() writes as NA for NF and WA.
func (d FireDescription) Code() float64 {
	switch d {
	case SurfaceFire:
		return 1
	case IntermittentCrown:
		return 2
	case ContinuousCrown:
		return 3
	}
	return math.NaN()
}

// Grid is the FBP System's inputs over a grid of cells, as cffdrs' fbpRaster()
// takes them: one slice per layer, cell i of every slice describing the same
// place. The layout is the caller's — row-major is what terra hands over, and
// nothing here depends on it.
//
// Fuel is required and fixes the grid's length. Every other layer is either
// that length or nil, and a nil layer is the corresponding Input field's zero
// value in every cell — the same as leaving the field out of an Input, which
// Predict accepts or refuses on its own terms. It is not a default: fbpRaster()
// fills a missing layer with fbp()'s own (FFMC 90, BUI 60, wind 10 km/h, ...),
// and nothing here does.
//
// Units and directions are Input's: the azimuths point towards, so a layer
// ported from fbpRaster()'s WD or ASPECT has 180 added once, at the boundary.
// The Site layers (Lat, Long, ElevM, DayOfYear, D0) build each cell's Site, and
// only when Lat is given; FMC is otherwise the caller's, as it is for Predict.
type Grid struct {
	Fuel []float64 // fbpRaster()'s numeric codes, 1-19 — see GridFuelCodes

	FFMC, BUI []float64

	WindKmh, WindAzimuthDeg, SlopePct, UpslopeAzimuthDeg []float64

	PC, PDF, CuringPct, GrassFuelLoad []float64

	FMC                             []float64
	Lat, Long, ElevM, DayOfYear, D0 []float64

	CBH, CFL, StandDensity, StandHeight []float64

	ElapsedMin []float64
}

// GridOptions are the settings fbpRaster() takes as layers but that a raster
// rarely varies, and that this package takes once for the whole grid.
type GridOptions struct {
	Crown         CrownSource // as Input.Crown; CrownUnset refuses every cell
	PointIgnition bool        // as Input.PointIgnition

	// Workers is how many goroutines share the cells; 0 or 1 is one, the
	// calling goroutine. It is the caller's because the package cannot ask:
	// runtime.NumCPU is an import, and the package imports math alone.
	Workers int
}

// GridError is PredictGrid's refusal of the whole grid: a layer whose length
// is not Fuel's. Cell refusals are not GridErrors; see PredictGrid.
type GridError struct {
	Layer string // the Grid field at fault
}

func (e *GridError) Error() string {
	return "fbp: grid layer " + e.Layer + " differs in length from Fuel"
}

// PredictGrid is Predict over a grid: cffdrs' fbpRaster(), output = "All". It
// adds no equation. Each cell's fuel code is mapped through GridFuelCodes and
// the cell is what Predict returns for it, in the Grid's cell order. FD as a
// layer is Prediction.FD.Code().
//
// A cell Predict refuses does not refuse the grid. Its Prediction is NaN in
// every field (FD empty) and its entry in the second return is the error, which
// matches Predict's sentinels — or ErrGridFuelCode for a code outside 1-19 —
// under errors.Is. The third return is a *GridError when the layers differ in
// length, and then there is no output at all.
//
// Three things differ from fbpRaster(), all measured against the pinned oracle
// (cffdrs 1.9.2) rather than read off the code, and all three are fbpRaster()
// filling a cell in rather than computing it:
//
//   - A fuel code outside 1-19 — 0, 20, 7.5, NaN — is refused with
//     ErrGridFuelCode. fbpRaster() maps it to NA, and fbp() then computes the
//     cell as C2, with a warning and nothing in the output to say so.
//   - An NaN in any other layer is whatever Predict makes of it, which is a
//     refusal wherever fbp() would substitute a default (FFMC 90, BUI 60, LAT
//     55, ...). Where fbp() and Predict agree on what a missing value means —
//     FMC computed from the site, CBH and CFL from the table under CrownCFFDRS,
//     a grass load on a fuel that never reads it — the cell is computed.
//   - NF and WA are refused with ErrUnknownFuel, as Predict refuses them.
//     fbpRaster() writes zeros in every output and NA in FD.
//
// And one thing fbpRaster() does that is a defect rather than a choice, and is
// not reproduced: a cell that is NaN in every layer is dropped before fbp()
// runs, and the shorter result is written back by position, so every later
// cell takes the next cell's answer and the last takes the first cell's.
// Here such a cell is refused like any other, and no other cell moves.
//
// Cells are independent and share no state, so GridOptions.Workers above 1
// splits the grid into that many contiguous blocks, one goroutine each, every
// one writing only its own block of the output. It changes no value, no error
// and no cell's place: a cell is the same arithmetic on whichever goroutine
// runs it, and the result is bit-identical to Workers 1. fbpRaster()'s m and
// cores are the same chunking, and change no value either. Workers above the
// cell count is the cell count; the call returns when every block is done.
//
// Not ported: reading or writing rasters (the caller's); the reprojection to
// EPSG:4326 that fbpRaster() runs when there is no LAT layer, which needs sf —
// Lat is a layer here; the rate at a bearing (TROS and the five built on it),
// for the reason Predict gives; and select and output, which only choose which
// layers come back.
func PredictGrid(g Grid, opt GridOptions) ([]Prediction, []error, error) {
	n := len(g.Fuel)
	layers := []struct {
		name string
		v    []float64
	}{
		{"FFMC", g.FFMC}, {"BUI", g.BUI},
		{"WindKmh", g.WindKmh}, {"WindAzimuthDeg", g.WindAzimuthDeg},
		{"SlopePct", g.SlopePct}, {"UpslopeAzimuthDeg", g.UpslopeAzimuthDeg},
		{"PC", g.PC}, {"PDF", g.PDF}, {"CuringPct", g.CuringPct}, {"GrassFuelLoad", g.GrassFuelLoad},
		{"FMC", g.FMC}, {"Lat", g.Lat}, {"Long", g.Long}, {"ElevM", g.ElevM},
		{"DayOfYear", g.DayOfYear}, {"D0", g.D0},
		{"CBH", g.CBH}, {"CFL", g.CFL}, {"StandDensity", g.StandDensity}, {"StandHeight", g.StandHeight},
		{"ElapsedMin", g.ElapsedMin},
	}
	for _, l := range layers {
		if l.v != nil && len(l.v) != n {
			return nil, nil, &GridError{Layer: l.name}
		}
	}

	out := make([]Prediction, n)
	errs := make([]error, n)
	if opt.Workers <= 1 {
		// Not through inChunks: the closure it takes escapes to the goroutines,
		// and what it captures with it, which would cost the serial path
		// allocations it never had. Hence also the copies below.
		predictCells(g, opt, out, errs, 0, n)
	} else {
		g, opt := g, opt
		inChunks(n, opt.Workers, func(lo, hi int) { predictCells(g, opt, out, errs, lo, hi) })
	}
	return out, errs, nil
}

// predictCells is PredictGrid's cells lo to hi-1, written into out and errs.
func predictCells(g Grid, opt GridOptions, out []Prediction, errs []error, lo, hi int) {
	at := func(v []float64, i int) float64 {
		if v == nil {
			return 0
		}
		return v[i]
	}
	for i := lo; i < hi; i++ {
		fuel, ok := FuelFromGridCode(g.Fuel[i])
		if !ok {
			out[i], errs[i] = refusedCell(), ErrGridFuelCode
			continue
		}
		in := Input{
			Fuel: fuel, FFMC: at(g.FFMC, i), BUI: at(g.BUI, i),
			WindKmh: at(g.WindKmh, i), WindAzimuthDeg: at(g.WindAzimuthDeg, i),
			SlopePct: at(g.SlopePct, i), UpslopeAzimuthDeg: at(g.UpslopeAzimuthDeg, i),
			PC: at(g.PC, i), PDF: at(g.PDF, i), CuringPct: at(g.CuringPct, i),
			GrassFuelLoad: at(g.GrassFuelLoad, i),
			FMC:           at(g.FMC, i),
			Crown:         opt.Crown, CBH: at(g.CBH, i), CFL: at(g.CFL, i),
			StandDensity: at(g.StandDensity, i), StandHeight: at(g.StandHeight, i),
			ElapsedMin: at(g.ElapsedMin, i), PointIgnition: opt.PointIgnition,
		}
		if g.Lat != nil {
			in.Site = &Site{Lat: g.Lat[i], Long: at(g.Long, i), ElevM: at(g.ElevM, i),
				DayOfYear: at(g.DayOfYear, i), D0: at(g.D0, i)}
		}
		p, err := Predict(in)
		if err != nil {
			p = refusedCell()
		}
		out[i], errs[i] = p, err
	}
}

// inChunks runs f over [0, n) split into contiguous blocks, one goroutine per
// block, and returns when all of them have. With one block or none to share —
// workers <= 1, or n <= 1 — it is f(0, n) on the calling goroutine. The blocks
// differ in size by at most one cell, and the join is one channel receive per
// block, which is also what publishes each block's writes to the caller.
func inChunks(n, workers int, f func(lo, hi int)) {
	if workers > n {
		workers = n
	}
	if workers <= 1 {
		f(0, n)
		return
	}
	done := make(chan struct{}, workers)
	size, extra := n/workers, n%workers
	for w, lo := 0, 0; w < workers; w++ {
		hi := lo + size
		if w < extra {
			hi++
		}
		go func(lo, hi int) {
			f(lo, hi)
			done <- struct{}{}
		}(lo, hi)
		lo = hi
	}
	for w := 0; w < workers; w++ {
		<-done
	}
}

// refusedCell is a Prediction with NaN in every number and no FD.
func refusedCell() Prediction {
	nan := math.NaN()
	return Prediction{
		FMC: nan, CBH: nan, CFL: nan, SFC: nan,
		ISI: nan, WSV: nan, RAZ: nan, BE: nan, SF: nan,
		ROS: nan, CFB: nan, CFC: nan, TFC: nan, HFI: nan,
		CSI: nan, RSO: nan,
		LB: nan, BROS: nan, FROS: nan,
		FCFB: nan, BCFB: nan, FTFC: nan, BTFC: nan,
		FFI: nan, BFI: nan,
		TI: nan, FTI: nan, BTI: nan,
		HROSt: nan, BROSt: nan, FROSt: nan, LBt: nan,
		DH: nan, DB: nan, DF: nan,
	}
}
