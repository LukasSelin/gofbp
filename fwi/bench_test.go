package fwi

import (
	"math/rand"
	"strconv"
	"testing"
)

var sinkIndices Indices

func BenchmarkStep(b *testing.B) {
	w := Weather{TempC: 22, RHPct: 35, WindKmh: 18, PrecipMm: 0.5}
	s := StartupState()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, sinkIndices = Step(s, w, 7, 55)
	}
}

var sinkGrid GridIndices

// benchStepGrid is n cells of random but valid weather and a uniform state.
func benchStepGrid(n int, seed int64) (GridState, Grid) {
	r := rand.New(rand.NewSource(seed))
	layer := func(lo, hi float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = lo + r.Float64()*(hi-lo)
		}
		return v
	}
	w := Grid{TempC: layer(5, 35), RHPct: layer(10, 100), WindKmh: layer(0, 40),
		PrecipMm: layer(0, 10), LatDeg: layer(45, 65)}
	return UniformGridState(StartupState(), n), w
}

var benchWorkers = []int{1, 4, 12, 24}

func BenchmarkStepGrid(b *testing.B) {
	const n = 100_000
	y, w := benchStepGrid(n, 1)
	for _, workers := range benchWorkers {
		b.Run("workers="+strconv.Itoa(workers), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ix, err := StepGridParallel(y, w, 7, workers)
				if err != nil {
					b.Fatal(err)
				}
				sinkGrid = ix
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/cell")
		})
	}
}

// BenchmarkStepGridInto is BenchmarkStepGrid with one output buffer reused
// across calls: the difference is the seven slices, 5.6 MB at 100k cells,
// that StepGridParallel makes and zeroes per call before any worker starts.
func BenchmarkStepGridInto(b *testing.B) {
	const n = 100_000
	y, w := benchStepGrid(n, 1)
	out := GridIndices{make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n), make([]float64, n)}
	for _, workers := range benchWorkers {
		b.Run("workers="+strconv.Itoa(workers), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := StepGridInto(out, y, w, 7, workers); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/cell")
		})
	}
}

// BenchmarkStepGridDays is ten days over 100k cells, both ways, each into one
// reused buffer stepped in place so neither pays for memory: mode=chained is
// StepGridInto ten times, a fork and join per day; mode=fused is one
// StepGridDaysInto, a fork and join per call with each chunk taken through all
// ten days.
func BenchmarkStepGridDays(b *testing.B) {
	const n, nDays = 100_000, 10
	y, _ := benchStepGrid(n, 1)
	days := make([]GridDay, nDays)
	for d := range days {
		_, w := benchStepGrid(n, int64(d+2))
		days[d] = GridDay{Weather: w, Month: 5 + d%4}
	}
	one := GridIndices{make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n),
		make([]float64, n), make([]float64, n), make([]float64, n)}
	reset := func() {
		copy(one.FFMC, y.FFMC)
		copy(one.DMC, y.DMC)
		copy(one.DC, y.DC)
	}
	same := make([]GridIndices, nDays)
	for d := range same {
		same[d] = one
	}
	for _, mode := range []string{"chained", "fused"} {
		for _, workers := range benchWorkers {
			b.Run("mode="+mode+"/workers="+strconv.Itoa(workers), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					reset()
					b.StartTimer()
					if mode == "fused" {
						if err := StepGridDaysInto(same, one.State(), days, workers); err != nil {
							b.Fatal(err)
						}
						continue
					}
					for _, day := range days {
						if err := StepGridInto(one, one.State(), day.Weather, day.Month, workers); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n*nDays), "ns/cell-day")
			})
		}
	}
}
