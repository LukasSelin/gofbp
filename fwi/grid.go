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
// grid across goroutines; StepGrid is StepGridParallel with one worker, and
// StepGridInto is either into slices the caller provides and may reuse.
//
// To chain days, pass the result's State() back in as yesterday, or hand
// StepGridDays every day at once, which is the same chain to the bit with one
// fork and join for the lot. fwiRaster() itself never chains; it does one day.
//
// Not ported: reading or writing rasters (the caller's), the "all" output that
// returns the input layers too (it writes RH back as 100 wherever RH was 100 or
// more, so an input of 120 comes back as 100), and the out/uppercase options,
// which only rename layers.
func StepGrid(yesterday GridState, w Grid, month int) (GridIndices, error) {
	return StepGridParallel(yesterday, w, month, 1)
}

// StepGridParallel is StepGrid with its cells shared among workers goroutines:
// the grid is cut into a few contiguous chunks per worker, each worker takes
// the next chunk until none is left, and each chunk writes only its own cells
// of the output. It changes no value and no
// cell's place — a cell is the same arithmetic on whichever goroutine runs it,
// so the result is bit-identical to StepGrid's. The refusals are StepGrid's and
// come first, on the calling goroutine, so a refused grid starts no worker and
// names the same cell. workers <= 1 is StepGrid exactly, and workers above the
// cell count is the cell count. The count is the caller's because the package
// cannot ask: runtime.NumCPU is an import, and the package imports math alone.
func StepGridParallel(yesterday GridState, w Grid, month, workers int) (GridIndices, error) {
	if err := checkGrid(w, codeLengths(yesterday)); err != nil {
		return GridIndices{}, err
	}
	n := len(w.TempC)
	out := GridIndices{
		make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n), make([]float64, n),
	}
	stepInto(out, yesterday, w, month, workers)
	return out, nil
}

// StepGridInto is StepGridParallel writing into out, whose seven slices must
// each be as long as w.TempC, instead of into slices it makes. Every cell of
// every slice is written, so a buffer reused from an earlier call carries
// nothing over, and the values are StepGrid's to the bit. The refusals are
// StepGrid's, in its order, and then out's lengths, in GridIndices' field
// order (Layer "out.FFMC" and so on, Cell -1); a refused call writes nothing.
//
// out may be yesterday itself — out.FFMC the very slice yesterday.FFMC, and so
// for DMC and DC — because a cell reads its three codes before it writes any,
// and reads no other cell's. That steps a grid in place, which is how
// StepGridDays chains without a buffer per day. Slices that overlap at an
// offset, or out's slices overlapping each other or w, are not supported.
//
// It exists for the caller who runs the same grid size day after day. At 100k
// cells the seven slices StepGridParallel makes are 5.6 MB, zeroed on the
// calling goroutine before any worker starts; that was 0.67 ms of a 9.1 ms
// call at 24 workers on the machine bench_test.go was measured on (7%), and it
// does not shrink with more workers. workers <= 1 allocates nothing here.
func StepGridInto(out GridIndices, yesterday GridState, w Grid, month, workers int) error {
	if err := checkGrid(w, codeLengths(yesterday)); err != nil {
		return err
	}
	if err := checkOut(out, len(w.TempC)); err != nil {
		return err
	}
	stepInto(out, yesterday, w, month, workers)
	return nil
}

// checkOut is StepGridInto's refusal of out: the first of its slices, in
// GridIndices' field order, that is not n long.
func checkOut(out GridIndices, n int) *GridError {
	for _, l := range [...]struct {
		name string
		v    []float64
	}{
		{"out.FFMC", out.FFMC}, {"out.DMC", out.DMC}, {"out.DC", out.DC}, {"out.ISI", out.ISI},
		{"out.BUI", out.BUI}, {"out.FWI", out.FWI}, {"out.DSR", out.DSR},
	} {
		if len(l.v) != n {
			return &GridError{Layer: l.name, Cell: -1,
				msg: "fwi: output slice " + l.name + " differs in length from TempC"}
		}
	}
	return nil
}

// codeLengths is the lengths of yesterday's FFMC, DMC and DC, which is all
// checkGrid reads of it.
func codeLengths(yesterday GridState) [3]int {
	return [3]int{len(yesterday.FFMC), len(yesterday.DMC), len(yesterday.DC)}
}

// checkGrid is StepGrid's refusal of the whole grid, in fwiRaster()'s order:
// a layer whose length is not TempC's — the weather's, then yesterday's codes,
// given as their lengths ny — then a negative precipitation, wind or RH.
// Callers test the *GridError against nil before it becomes an error.
func checkGrid(w Grid, ny [3]int) *GridError {
	n := len(w.TempC)
	for _, l := range [...]struct {
		name string
		n    int
	}{
		{"RHPct", len(w.RHPct)}, {"WindKmh", len(w.WindKmh)}, {"PrecipMm", len(w.PrecipMm)},
		{"LatDeg", len(w.LatDeg)}, {"FFMC", ny[0]}, {"DMC", ny[1]}, {"DC", ny[2]},
	} {
		if l.n != n {
			return &GridError{Layer: l.name, Cell: -1,
				msg: "fwi: grid layer " + l.name + " differs in length from TempC"}
		}
	}
	for _, l := range [...]struct {
		name, what string
		v          []float64
	}{
		{"PrecipMm", "precipitation", w.PrecipMm},
		{"WindKmh", "wind speed", w.WindKmh},
		{"RHPct", "relative humidity", w.RHPct},
	} {
		for i, x := range l.v {
			if x < 0 {
				return &GridError{Layer: l.name, Cell: i,
					msg: "fwi: negative " + l.what + " in the grid"}
			}
		}
	}
	return nil
}

// stepInto is every cell of a checked grid, written into out, on workers
// goroutines.
func stepInto(out GridIndices, yesterday GridState, w Grid, month, workers int) {
	if workers <= 1 {
		// Not through inChunks: the closure it takes escapes to the goroutines,
		// and what it captures with it, which would cost the serial path
		// allocations it never had. Hence also the copies below.
		stepCells(yesterday, w, month, out, 0, len(w.TempC))
	} else {
		out, yesterday, w := out, yesterday, w
		inChunks(len(w.TempC), workers, func(lo, hi int) { stepCells(yesterday, w, month, out, lo, hi) })
	}
}

// GridDay is one day for StepGridDays: the day's weather over the grid and its
// month, as StepGrid takes them.
type GridDay struct {
	Weather Grid
	Month   int
}

// DayError is StepGridDays' refusal of day Day (0 for the first): Err is what
// StepGrid, or StepGridInto, would have refused on that day, layer, cell and
// all, and errors.As reaches it.
type DayError struct {
	Day int
	Err *GridError
}

func (e *DayError) Error() string {
	msg := e.Err.msg
	if len(msg) > 5 && msg[:5] == "fwi: " {
		msg = msg[5:]
	}
	return "fwi: day " + itoa(e.Day) + " of the run: " + msg
}

func (e *DayError) Unwrap() error { return e.Err }

// StepGridDays is StepGrid chained over consecutive days: day 0 steps
// yesterday, and day d steps day d-1's output. The result is one GridIndices
// per day, each bit-identical to what the chain of StepGrid calls returns for
// that day, and the last one's State() is the state to carry on from. No days
// is no output and no error.
//
// What it saves is the joins. StepGridParallel D times forks and joins D
// times, and every day the workers wait on the slowest chunk and a cell's
// codes come back from memory on whatever core gets that chunk next. Here a
// chunk is taken once and stepped through every day before the worker takes
// the next, so there is one fork and one join per call, and day d+1 reads day
// d's codes from the cache that wrote them. A cell's days still run in order,
// on one goroutine, and no cell reads another, which is why the values are the
// chain's. Ten days of 100k cells at 24 workers, stepped in place either way,
// took 51 ms fused against 59 ms chained on a 12-core Ryzen 9 3900X (13%); at
// 4 workers the two are within 4%, so the saving is mostly the joins.
//
// Every day is checked before any is computed, in day order, each with
// StepGrid's checks in StepGrid's order: day d's weather layers against its
// own TempC, then the codes it would be handed (yesterday's for day 0, the
// grid's length for the rest), then negative precipitation, wind and RH. The
// first refusal is a *DayError naming the day, and there is no output at all,
// where the chain would have returned the days before it.
func StepGridDays(yesterday GridState, days []GridDay, workers int) ([]GridIndices, error) {
	if err := checkDays(yesterday, days, nil); err != nil {
		return nil, err
	}
	out := make([]GridIndices, len(days))
	for d := range out {
		n := len(days[d].Weather.TempC)
		out[d] = GridIndices{
			make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n),
			make([]float64, n), make([]float64, n), make([]float64, n),
		}
	}
	stepDaysInto(out, yesterday, days, workers)
	return out, nil
}

// StepGridDaysInto is StepGridDays writing day d into out[d], instead of into
// slices it makes. out must have one GridIndices per day; a different count is
// a *GridError on Layer "out" (Cell -1) before anything else. Each day is then
// refused as StepGridDays refuses it, followed by out[d]'s lengths as
// StepGridInto refuses them — the order a chain of StepGridInto calls would
// meet them in — and a refused call writes nothing.
//
// out[d] may be out[d-1] — the very same slices — and out[0]'s FFMC, DMC and
// DC may be yesterday's, for the reason StepGridInto may step in place. Passing
// one GridIndices for every day, its codes yesterday's, runs any number of
// days in one day's memory and leaves the final day in it. Slices that overlap
// at an offset, or one output overlapping another output of a different
// layer, are not supported.
func StepGridDaysInto(out []GridIndices, yesterday GridState, days []GridDay, workers int) error {
	if len(out) != len(days) {
		return &GridError{Layer: "out", Cell: -1, msg: "fwi: out has a different number of days from days"}
	}
	if err := checkDays(yesterday, days, out); err != nil {
		return err
	}
	stepDaysInto(out, yesterday, days, workers)
	return nil
}

// checkDays is StepGridDays' refusal: each day's checkGrid, in day order, with
// the codes day d > 0 is handed being the grid's length, since they are day
// d-1's output — and after each day, out[d]'s checkOut, when out is not nil.
func checkDays(yesterday GridState, days []GridDay, out []GridIndices) error {
	ny := codeLengths(yesterday)
	for d, day := range days {
		n := len(day.Weather.TempC)
		if err := checkGrid(day.Weather, ny); err != nil {
			return &DayError{Day: d, Err: err}
		}
		if out != nil {
			if err := checkOut(out[d], n); err != nil {
				return &DayError{Day: d, Err: err}
			}
		}
		ny = [3]int{n, n, n}
	}
	return nil
}

// stepDaysInto is every day of checked days, day d written into out[d], on
// workers goroutines that each take a chunk of cells through every day.
func stepDaysInto(out []GridIndices, yesterday GridState, days []GridDay, workers int) {
	if len(days) == 0 {
		return
	}
	if workers <= 1 {
		// Not through inChunks, for StepGridParallel's reason.
		stepDayCells(out, yesterday, days, 0, len(days[0].Weather.TempC))
	} else {
		out, yesterday, days := out, yesterday, days
		inChunks(len(days[0].Weather.TempC), workers, func(lo, hi int) { stepDayCells(out, yesterday, days, lo, hi) })
	}
}

// stepDayCells is cells lo to hi-1 through every day, day by day.
func stepDayCells(out []GridIndices, yesterday GridState, days []GridDay, lo, hi int) {
	for d, day := range days {
		if d > 0 {
			yesterday = out[d-1].State()
		}
		stepCells(yesterday, day.Weather, day.Month, out[d], lo, hi)
	}
}

// itoa is n in decimal: strconv is an import, and the package imports math
// alone.
func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
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

// chunksPerWorker is how many chunks inChunks cuts per worker. One block per
// worker leaves the call waiting on the slowest block, and the cost of a cell
// is not uniform: an FBP cell refused for NF or WA costs a fraction of a C6
// crown fire, and a raster puts them in patches. With several chunks each, a
// worker that finishes early takes another. Against one block per worker, at
// 100k cells and 24 workers on a 12-core Ryzen 9 3900X, 8 took 16% off the
// clustered FBP grid in bench_test.go and 39% off the uniform FWI grid (where
// cells cost alike but workers do not run alike: they start at different
// times, and at 24 two share each core). 4 was within noise of 8 on the
// clustered grid and took only 32% off the FWI one.
const chunksPerWorker = 8

// inChunks runs f over [0, n) cut into contiguous chunks, on workers
// goroutines, and returns when all of them have. With one worker or no more
// than one cell — workers <= 1, or n <= 1 — it is f(0, n) on the calling
// goroutine.
//
// There are chunksPerWorker chunks per worker (at most n), differing in size by
// at most one cell, and they are handed out in order through a buffered
// channel of chunk indices, filled and closed before any worker starts: each
// worker takes the next index until there are none. Which goroutine runs a
// chunk is the scheduler's, but every cell is in exactly one chunk and f
// writes only its own cells, so no value depends on it. The join is one
// channel receive per worker, which is also what publishes the workers' writes
// to the caller.
func inChunks(n, workers int, f func(lo, hi int)) {
	if workers > n {
		workers = n
	}
	if workers <= 1 {
		f(0, n)
		return
	}
	chunks := min(workers*chunksPerWorker, n)
	next := make(chan int, chunks)
	for c := 0; c < chunks; c++ {
		next <- c
	}
	close(next)
	size, extra := n/chunks, n%chunks
	done := make(chan struct{}, workers)
	for w := 0; w < workers; w++ {
		go func() {
			for c := range next {
				lo := c*size + min(c, extra)
				hi := lo + size
				if c < extra {
					hi++
				}
				f(lo, hi)
			}
			done <- struct{}{}
		}()
	}
	for w := 0; w < workers; w++ {
		<-done
	}
}
