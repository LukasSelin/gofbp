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

func BenchmarkStepGrid(b *testing.B) {
	const n = 100_000
	r := rand.New(rand.NewSource(1))
	layer := func(lo, hi float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = lo + r.Float64()*(hi-lo)
		}
		return v
	}
	w := Grid{TempC: layer(5, 35), RHPct: layer(10, 100), WindKmh: layer(0, 40),
		PrecipMm: layer(0, 10), LatDeg: layer(45, 65)}
	y := UniformGridState(StartupState(), n)
	for _, workers := range []int{1, 4, 12, 24} {
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
