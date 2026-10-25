package fwi

// Grid is one day's weather over a grid of cells, as cffdrs' fwiRaster() takes
// it: one slice per layer, cell i of every slice describing the same place.
// The layout is the caller's — row-major is what terra and most raster
// libraries hand over, and nothing here depends on it — but every slice must
// have the same length. Units are Weather's.
//
// LatDeg is a layer, not a default. fwiRaster() uses 55 °N for every cell
// when it is given no latitude layer; this package has no silent defaults (see
// Step), so a caller who wants that fills the slice with 55.
type Grid struct {
	TempC    []float64
	RHPct    []float64
	WindKmh  []float64
	PrecipMm []float64
	LatDeg   []float64
}

// GridState is State over a grid: yesterday's three codes, one slice each, the
// same length as the Grid they go with. fwiRaster() also takes a single
// FFMC/DMC/DC triple for every cell; UniformGridState builds that.
type GridState struct {
	FFMC []float64
	DMC  []float64
	DC   []float64
}

// UniformGridState is s in every one of n cells — fwiRaster()'s numeric init.
// StartupState is its default, 85/6/15, which this does not assume.
func UniformGridState(s State, n int) GridState {
	g := GridState{make([]float64, n), make([]float64, n), make([]float64, n)}
	for i := 0; i < n; i++ {
		g.FFMC[i], g.DMC[i], g.DC[i] = s.FFMC, s.DMC, s.DC
	}
	return g
}

// GridIndices is Indices over a grid, one slice per output, in the Grid's cell
// order.
type GridIndices struct {
	FFMC []float64
	DMC  []float64
	DC   []float64
	ISI  []float64
	BUI  []float64
	FWI  []float64
	DSR  []float64
}

// State returns the codes to carry into tomorrow's StepGrid. The slices are
// shared with ix, not copied.
func (ix GridIndices) State() GridState { return GridState{ix.FFMC, ix.DMC, ix.DC} }

// GridError is StepGrid's refusal. Cell is the first offending cell of the
// first offending layer, or -1 when the layers differ in length.
type GridError struct {
	Layer string // the Grid or GridState field at fault
	Cell  int
	msg   string
}

func (e *GridError) Error() string { return e.msg }

// StepGrid is Step over a grid: cffdrs' fwiRaster(), one day, lat.adjust =
// TRUE. Each cell is what Step computes for it — the RH ≥ 100 → 99.9999 clamp,
// FFMC, DMC, DC, ISI, BUI, FWI and DSR = 0.0272·FWI^1.77 — and the month is one
// value for the whole grid, as fwiRaster()'s mon is.
//
// Two things differ from calling Step per cell, both because fwiRaster() does
// them:
//
//   - A negative precipitation, wind or RH anywhere refuses the whole grid,
//     with an error and no output, where Step would return NaN for that cell
//     alone. fwiRaster() stop()s, checking precipitation, then wind, then RH;
//     the GridError names the first offender in that order.
//   - A NaN cell is not refused, and does not poison the cell: each output is
//     NaN only if an input it reads is, the way terra's NA flows through the
//     components. A cell with no wind still has a DMC, a DC and a BUI; a cell
//     with no latitude still has an FFMC and an ISI. Step instead makes every
//     output NaN, which is the right contract for a station and the wrong one
//     for a raster with a masked cell.
//
// Cells are independent and share no state, so StepGridParallel can split the
// grid across goroutines; StepGrid is StepGridParallel with one worker.
//
// To chain days, pass the result's State() back in as yesterday. fwiRaster()
// itself never chains; it does one day.
//
// Not ported: reading or writing rasters (the caller's), the "all" output that
// returns the input layers too (it writes RH back as 100 wherever RH was 100 or
// more, so an input of 120 comes back as 100), and the out/uppercase options,
// which only rename layers.
func StepGrid(yesterday GridState, w Grid, month int) (GridIndices, error) {
	return StepGridParallel(yesterday, w, month, 1)
}

// StepGridParallel is StepGrid with its cells shared among workers goroutines:
// the grid is split into that many contiguous blocks, one goroutine each, every
// one writing only its own block of the output. It changes no value and no
// cell's place — a cell is the same arithmetic on whichever goroutine runs it,
// so the result is bit-identical to StepGrid's. The refusals are StepGrid's and
// come first, on the calling goroutine, so a refused grid starts no worker and
// names the same cell. workers <= 1 is StepGrid exactly, and workers above the
// cell count is the cell count. The count is the caller's because the package
// cannot ask: runtime.NumCPU is an import, and the package imports math alone.
func StepGridParallel(yesterday GridState, w Grid, month, workers int) (GridIndices, error) {
	n := len(w.TempC)
	for _, l := range []struct {
		name string
		v    []float64
	}{
		{"RHPct", w.RHPct}, {"WindKmh", w.WindKmh}, {"PrecipMm", w.PrecipMm}, {"LatDeg", w.LatDeg},
		{"FFMC", yesterday.FFMC}, {"DMC", yesterday.DMC}, {"DC", yesterday.DC},
	} {
		if len(l.v) != n {
			return GridIndices{}, &GridError{Layer: l.name, Cell: -1,
				msg: "fwi: grid layer " + l.name + " differs in length from TempC"}
		}
	}
	for _, l := range []struct {
		name, what string
		v          []float64
	}{
		{"PrecipMm", "precipitation", w.PrecipMm},
		{"WindKmh", "wind speed", w.WindKmh},
		{"RHPct", "relative humidity", w.RHPct},
	} {
		for i, x := range l.v {
			if x < 0 {
				return GridIndices{}, &GridError{Layer: l.name, Cell: i,
					msg: "fwi: negative " + l.what + " in the grid"}
			}
		}
	}

	out := GridIndices{
		make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n), make([]float64, n),
	}
	if workers <= 1 {
		// Not through inChunks: the closure it takes escapes to the goroutines,
		// and what it captures with it, which would cost the serial path
		// allocations it never had. Hence also the copies below.
		stepCells(yesterday, w, month, out, 0, n)
	} else {
		yesterday, w, out := yesterday, w, out
		inChunks(n, workers, func(lo, hi int) { stepCells(yesterday, w, month, out, lo, hi) })
	}
	return out, nil
}

// stepCells is StepGrid's cells lo to hi-1, written into out.
func stepCells(yesterday GridState, w Grid, month int, out GridIndices, lo, hi int) {
	for i := lo; i < hi; i++ {
		ix := step(State{yesterday.FFMC[i], yesterday.DMC[i], yesterday.DC[i]},
			Weather{w.TempC[i], w.RHPct[i], w.WindKmh[i], w.PrecipMm[i]}, month, w.LatDeg[i])
		out.FFMC[i], out.DMC[i], out.DC[i] = ix.FFMC, ix.DMC, ix.DC
		out.ISI[i], out.BUI[i], out.FWI[i], out.DSR[i] = ix.ISI, ix.BUI, ix.FWI, ix.DSR
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
