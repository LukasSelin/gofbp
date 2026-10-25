package fwi

import (
	"errors"
	"math"
	"strconv"
	"sync/atomic"
	"testing"
)

// gridOf lays the same weather and latitude sweep out as a grid, so the tests
// below can compare StepGrid with Step over the same cells.
func gridOf() (Grid, []Weather, []float64) {
	var g Grid
	var wx []Weather
	var lats []float64
	for _, temp := range []float64{-5, 10, 25, 40} {
		for _, rh := range []float64{0, 30, 70, 100, 120} {
			for _, wind := range []float64{0, 12, 60} {
				for _, prec := range []float64{0, 0.51, 2.81, 30} {
					for _, lat := range []float64{-40, -15, 0, 15, 25, 55, 68} {
						g.TempC = append(g.TempC, temp)
						g.RHPct = append(g.RHPct, rh)
						g.WindKmh = append(g.WindKmh, wind)
						g.PrecipMm = append(g.PrecipMm, prec)
						g.LatDeg = append(g.LatDeg, lat)
						wx = append(wx, Weather{temp, rh, wind, prec})
						lats = append(lats, lat)
					}
				}
			}
		}
	}
	return g, wx, lats
}

// Every finite cell is exactly what Step computes for it — not close, equal,
// because they are the same arithmetic.
func TestStepGridIsStepPerCell(t *testing.T) {
	g, wx, lats := gridOf()
	y := UniformGridState(StartupState(), len(wx))
	for _, month := range []int{1, 7, 11} {
		got, err := StepGrid(y, g, month)
		if err != nil {
			t.Fatal(err)
		}
		for i := range wx {
			_, want := Step(StartupState(), wx[i], month, lats[i])
			cell := Indices{got.FFMC[i], got.DMC[i], got.DC[i], got.ISI[i], got.BUI[i], got.FWI[i], got.DSR[i]}
			if cell != want {
				t.Fatalf("month %d cell %d: StepGrid %+v, Step %+v", month, i, cell, want)
			}
		}
	}
}

// Chaining through State() is chaining Step: two days over the grid are two
// Steps per cell.
func TestStepGridChains(t *testing.T) {
	g, wx, lats := gridOf()
	day1, err := StepGrid(UniformGridState(StartupState(), len(wx)), g, 6)
	if err != nil {
		t.Fatal(err)
	}
	day2, err := StepGrid(day1.State(), g, 6)
	if err != nil {
		t.Fatal(err)
	}
	for i := range wx {
		s, _ := Step(StartupState(), wx[i], 6, lats[i])
		_, want := Step(s, wx[i], 6, lats[i])
		if day2.FWI[i] != want.FWI || day2.DC[i] != want.DC {
			t.Fatalf("cell %d: day 2 FWI %v DC %v, Step chain %v %v", i, day2.FWI[i], day2.DC[i], want.FWI, want.DC)
		}
	}
}

// The doc comment's parallelism claim: cells share no state, so any split of
// the slices gives the same numbers as the whole.
func TestStepGridSplitIsBitIdentical(t *testing.T) {
	g, wx, _ := gridOf()
	n := len(wx)
	y := UniformGridState(StartupState(), n)
	whole, err := StepGrid(y, g, 8)
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{0, 1, n / 3, n - 1, n} {
		part := func(lo, hi int) GridIndices {
			out, err := StepGrid(GridState{y.FFMC[lo:hi], y.DMC[lo:hi], y.DC[lo:hi]},
				Grid{g.TempC[lo:hi], g.RHPct[lo:hi], g.WindKmh[lo:hi], g.PrecipMm[lo:hi], g.LatDeg[lo:hi]}, 8)
			if err != nil {
				t.Fatal(err)
			}
			return out
		}
		a, b := part(0, cut), part(cut, n)
		for i := 0; i < n; i++ {
			got := b.DSR
			j := i - cut
			if i < cut {
				got, j = a.DSR, i
			}
			if math.Float64bits(got[j]) != math.Float64bits(whole.DSR[i]) {
				t.Fatalf("cut %d, cell %d: %v split, %v whole", cut, i, got[j], whole.DSR[i])
			}
		}
	}
}

// A NaN input reaches only the outputs that read it, as terra's NA does in
// fwiRaster() — and unlike Step, which poisons the whole day.
func TestStepGridNaNStaysInItsLane(t *testing.T) {
	nan := math.NaN()
	g := Grid{
		TempC:    []float64{20, 20, 20},
		RHPct:    []float64{40, 40, 40},
		WindKmh:  []float64{15, nan, 15},
		PrecipMm: []float64{0, 0, 0},
		LatDeg:   []float64{55, 55, nan},
	}
	got, err := StepGrid(UniformGridState(StartupState(), 3), g, 7)
	if err != nil {
		t.Fatal(err)
	}
	isNaN := func(xs ...float64) bool {
		for _, x := range xs {
			if !math.IsNaN(x) {
				return false
			}
		}
		return true
	}
	finite := func(xs ...float64) bool {
		for _, x := range xs {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return false
			}
		}
		return true
	}
	// No wind: FFMC reads wind, so FFMC and everything after it is NaN; the
	// duff codes and BUI never read it.
	if !isNaN(got.FFMC[1], got.ISI[1], got.FWI[1], got.DSR[1]) || !finite(got.DMC[1], got.DC[1], got.BUI[1]) {
		t.Errorf("no-wind cell: %v %v %v %v %v %v %v", got.FFMC[1], got.DMC[1], got.DC[1], got.ISI[1], got.BUI[1], got.FWI[1], got.DSR[1])
	}
	// No latitude: the day lengths are NaN, FFMC and ISI are not.
	if !finite(got.FFMC[2], got.ISI[2]) || !isNaN(got.DMC[2], got.DC[2], got.BUI[2], got.FWI[2]) {
		t.Errorf("no-latitude cell: %v %v %v %v %v %v", got.FFMC[2], got.ISI[2], got.DMC[2], got.DC[2], got.BUI[2], got.FWI[2])
	}
	if !finite(got.FFMC[0], got.DMC[0], got.DC[0], got.ISI[0], got.BUI[0], got.FWI[0], got.DSR[0]) {
		t.Errorf("a NaN in one cell reached its neighbour: %+v", got)
	}
	_, step := Step(StartupState(), Weather{20, 40, nan, 0}, 7, 55)
	if !math.IsNaN(step.DMC) {
		t.Error("Step's contract changed: a NaN weather field no longer poisons the whole day")
	}
}

// Negative precipitation, wind or RH refuses the whole grid, naming the first
// offender in fwiRaster()'s order: precipitation, then wind, then RH.
func TestStepGridRefusesNegatives(t *testing.T) {
	base := func() Grid {
		return Grid{
			TempC: []float64{20, 20, 20}, RHPct: []float64{40, 40, 40},
			WindKmh: []float64{15, 15, 15}, PrecipMm: []float64{0, 0, 0}, LatDeg: []float64{55, 55, 55},
		}
	}
	y := UniformGridState(StartupState(), 3)
	for _, tc := range []struct {
		name  string
		edit  func(*Grid)
		layer string
		cell  int
	}{
		{"precipitation", func(g *Grid) { g.PrecipMm[2] = -0.1 }, "PrecipMm", 2},
		{"wind", func(g *Grid) { g.WindKmh[1] = -1 }, "WindKmh", 1},
		{"RH", func(g *Grid) { g.RHPct[0] = -5 }, "RHPct", 0},
		{"RH and precipitation: precipitation first", func(g *Grid) { g.RHPct[0] = -5; g.PrecipMm[2] = -1 }, "PrecipMm", 2},
		{"wind and RH: wind first", func(g *Grid) { g.RHPct[0] = -5; g.WindKmh[2] = -1 }, "WindKmh", 2},
	} {
		g := base()
		tc.edit(&g)
		got, err := StepGrid(y, g, 7)
		var ge *GridError
		if !errors.As(err, &ge) || ge.Layer != tc.layer || ge.Cell != tc.cell {
			t.Errorf("%s: err %v, want a GridError on %s cell %d", tc.name, err, tc.layer, tc.cell)
			continue
		}
		if got.FFMC != nil {
			t.Errorf("%s: a refused grid returned output", tc.name)
		}
	}
	// RH above 100 is not negative and is not refused: it is clamped.
	g := base()
	g.RHPct[1] = 120
	got, err := StepGrid(y, g, 7)
	if err != nil {
		t.Fatalf("RH 120: %v", err)
	}
	_, want := Step(StartupState(), Weather{20, 99.9999, 15, 0}, 7, 55)
	if got.DMC[1] != want.DMC {
		t.Errorf("RH 120: DMC %v, want the clamped %v", got.DMC[1], want.DMC)
	}
}

func TestStepGridRefusesRaggedLayers(t *testing.T) {
	g := Grid{
		TempC: []float64{20, 20}, RHPct: []float64{40, 40},
		WindKmh: []float64{15, 15}, PrecipMm: []float64{0, 0}, LatDeg: []float64{55},
	}
	_, err := StepGrid(UniformGridState(StartupState(), 2), g, 7)
	var ge *GridError
	if !errors.As(err, &ge) || ge.Layer != "LatDeg" || ge.Cell != -1 {
		t.Errorf("short LatDeg: err %v", err)
	}
	g.LatDeg = []float64{55, 55}
	_, err = StepGrid(UniformGridState(StartupState(), 3), g, 7)
	if !errors.As(err, &ge) || ge.Layer != "FFMC" {
		t.Errorf("long yesterday: err %v", err)
	}
	got, err := StepGrid(GridState{}, Grid{}, 7)
	if err != nil || len(got.FWI) != 0 {
		t.Errorf("empty grid: %v, %d cells", err, len(got.FWI))
	}
}

// parallelCases is a sweep with NaNs in a weather layer, a latitude and a
// code, and an empty grid.
func parallelCases() []struct {
	name string
	y    GridState
	g    Grid
} {
	g, wx, _ := gridOf()
	n := len(wx)
	g.TempC[5], g.WindKmh[77], g.LatDeg[n-1] = math.NaN(), math.NaN(), math.NaN()
	y := UniformGridState(StartupState(), n)
	y.DMC[300] = math.NaN()
	return []struct {
		name string
		y    GridState
		g    Grid
	}{{"sweep", y, g}, {"empty", GridState{}, Grid{}}}
}

// parallelWorkers is the worker counts every parallel path is held to: none,
// one, counts that divide the grid and counts that do not, and more than the
// cells.
func parallelWorkers(n int) []int { return []int{-1, 0, 1, 2, 3, 7, n + 5} }

// newGridIndices is n cells of out, every one -12345, so a cell a call fails to
// write shows.
func newGridIndices(n int) GridIndices {
	ix := GridIndices{}
	for _, p := range []*[]float64{&ix.FFMC, &ix.DMC, &ix.DC, &ix.ISI, &ix.BUI, &ix.FWI, &ix.DSR} {
		*p = make([]float64, n)
		for i := range *p {
			(*p)[i] = -12345
		}
	}
	return ix
}

// sameIndices is got against want, every layer, every cell, to the bit.
func sameIndices(t *testing.T, name string, got, want GridIndices) {
	t.Helper()
	for _, l := range []struct {
		name      string
		got, want []float64
	}{
		{"FFMC", got.FFMC, want.FFMC}, {"DMC", got.DMC, want.DMC}, {"DC", got.DC, want.DC},
		{"ISI", got.ISI, want.ISI}, {"BUI", got.BUI, want.BUI}, {"FWI", got.FWI, want.FWI},
		{"DSR", got.DSR, want.DSR},
	} {
		if len(l.got) != len(l.want) {
			t.Fatalf("%s: %s has %d cells, want %d", name, l.name, len(l.got), len(l.want))
		}
		for i := range l.want {
			if math.Float64bits(l.got[i]) != math.Float64bits(l.want[i]) {
				t.Errorf("%s: %s cell %d = %v, serial %v", name, l.name, i, l.got[i], l.want[i])
			}
		}
	}
}

// StepGridParallel and StepGridInto are StepGrid to the bit, cell for cell,
// whatever the worker count — dividing the grid, not dividing it, exceeding it,
// or with no cells — and StepGridInto is so into a dirty buffer and in place.
func TestStepGridParallelIsBitIdentical(t *testing.T) {
	for _, tc := range parallelCases() {
		want, err := StepGrid(tc.y, tc.g, 8)
		if err != nil {
			t.Fatal(err)
		}
		n := len(tc.g.TempC)
		for _, workers := range parallelWorkers(n) {
			name := tc.name + ", " + strconv.Itoa(workers) + " workers"
			got, err := StepGridParallel(tc.y, tc.g, 8, workers)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			sameIndices(t, name+", StepGridParallel", got, want)

			into := newGridIndices(n)
			if err := StepGridInto(into, tc.y, tc.g, 8, workers); err != nil {
				t.Fatalf("%s: StepGridInto: %v", name, err)
			}
			sameIndices(t, name+", StepGridInto", into, want)

			// In place: yesterday's own slices are out's codes.
			inPlace := newGridIndices(n)
			copy(inPlace.FFMC, tc.y.FFMC)
			copy(inPlace.DMC, tc.y.DMC)
			copy(inPlace.DC, tc.y.DC)
			if err := StepGridInto(inPlace, inPlace.State(), tc.g, 8, workers); err != nil {
				t.Fatalf("%s: StepGridInto in place: %v", name, err)
			}
			sameIndices(t, name+", StepGridInto in place", inPlace, want)
		}
	}
}

// The whole-grid refusals do not depend on the worker count or the path: same
// layer, same cell, same message, and no output.
func TestStepGridParallelRefusesAsStepGrid(t *testing.T) {
	g, wx, _ := gridOf()
	n := len(wx)
	y := UniformGridState(StartupState(), n)
	g.RHPct[3], g.WindKmh[n-2], g.PrecipMm[n/2], g.PrecipMm[n-1] = -1, -1, -1, -1
	ragged := g
	ragged.LatDeg = g.LatDeg[:n-1]
	for _, gg := range []Grid{g, ragged} {
		_, want := StepGrid(y, gg, 7)
		var wge *GridError
		if !errors.As(want, &wge) {
			t.Fatalf("StepGrid: err %v, want a GridError", want)
		}
		for _, workers := range []int{2, 3, 7, n + 5} {
			got, err := StepGridParallel(y, gg, 7, workers)
			var ge *GridError
			if !errors.As(err, &ge) || *ge != *wge || got.FFMC != nil {
				t.Errorf("%d workers: err %v, want %v and no output", workers, err, want)
			}
			// A short out too, which must not be what is reported.
			into := newGridIndices(n)
			into.DSR = into.DSR[:1]
			err = StepGridInto(into, y, gg, 7, workers)
			if !errors.As(err, &ge) || *ge != *wge || into.FFMC[0] != -12345 {
				t.Errorf("%d workers: StepGridInto err %v, want %v and nothing written", workers, err, want)
			}
		}
	}
}

// StepGridInto refuses an out slice of the wrong length after the grid's own
// refusals, in GridIndices' field order, and writes nothing when it does.
func TestStepGridIntoRefusesOutLengths(t *testing.T) {
	g, wx, _ := gridOf()
	n := len(wx)
	y := UniformGridState(StartupState(), n)
	for _, tc := range []struct {
		layer string
		cut   func(*GridIndices)
	}{
		{"out.FFMC", func(ix *GridIndices) { ix.FFMC = nil }},
		{"out.ISI", func(ix *GridIndices) { ix.ISI = ix.ISI[:n-1]; ix.DSR = ix.DSR[:3] }},
		{"out.DSR", func(ix *GridIndices) { ix.DSR = append(ix.DSR, 0) }},
	} {
		for _, workers := range []int{1, 3} {
			into := newGridIndices(n)
			tc.cut(&into)
			err := StepGridInto(into, y, g, 7, workers)
			var ge *GridError
			if !errors.As(err, &ge) || ge.Layer != tc.layer || ge.Cell != -1 {
				t.Errorf("%s, %d workers: err %v", tc.layer, workers, err)
			}
			if into.DMC[0] != -12345 {
				t.Errorf("%s, %d workers: written by a refused call", tc.layer, workers)
			}
		}
	}
	if err := StepGridInto(GridIndices{}, GridState{}, Grid{}, 7, 4); err != nil {
		t.Errorf("empty grid into empty out: %v", err)
	}
}

// The serial path allocates what it returns and nothing else: StepGrid its
// seven slices, StepGridInto nothing.
func TestStepGridSerialAllocations(t *testing.T) {
	g, wx, _ := gridOf()
	y := UniformGridState(StartupState(), len(wx))
	out := newGridIndices(len(wx))
	if a := testing.AllocsPerRun(20, func() { StepGrid(y, g, 7) }); a != 7 {
		t.Errorf("StepGrid: %v allocations, want 7", a)
	}
	if a := testing.AllocsPerRun(20, func() { StepGridInto(out, y, g, 7, 1) }); a != 0 {
		t.Errorf("StepGridInto, 1 worker: %v allocations, want 0", a)
	}
}

// inChunks hands every cell to f exactly once, in chunks that tile [0, n),
// whatever the worker count.
func TestInChunksCoversEveryCellOnce(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 7, 64, 1000, 1001} {
		for _, workers := range []int{-1, 0, 1, 2, 3, 7, n + 5} {
			seen := make([]int32, n)
			inChunks(n, workers, func(lo, hi int) {
				if lo > hi || lo < 0 || hi > n {
					t.Errorf("n %d, %d workers: chunk [%d, %d)", n, workers, lo, hi)
					return
				}
				for i := lo; i < hi; i++ {
					atomic.AddInt32(&seen[i], 1)
				}
			})
			for i, c := range seen {
				if c != 1 {
					t.Fatalf("n %d, %d workers: cell %d seen %d times", n, workers, i, c)
				}
			}
		}
	}
}
