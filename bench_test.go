package fbp

import (
	"math/rand"
	"testing"
)

// benchInputs is a spread of stands across the fuel types, under CrownCFFDRS
// with FMC from a Site, so every path through Predict — crown, C6, grass,
// mixedwood, slash, the acceleration model — is in the mix.
func benchInputs() []Input {
	var ins []Input
	for _, fuel := range []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "D1",
		"M1", "M2", "M3", "M4", "O1A", "O1B", "S1", "S2", "S3"} {
		slope := 30.0
		if fuel == "C6" {
			slope = 0
		}
		ins = append(ins, Input{
			Fuel: fuel, FFMC: 92, BUI: 80,
			WindKmh: 20, WindAzimuthDeg: 45, SlopePct: slope, UpslopeAzimuthDeg: 90,
			PC: 50, PDF: 35, CuringPct: 90, GrassFuelLoad: 0.35,
			Site:  &Site{Lat: 55, Long: -120, ElevM: 500, DayOfYear: 182},
			Crown: CrownCFFDRS, CBH: -1, CFL: -1,
			ElapsedMin: 60, PointIgnition: true,
		})
	}
	return ins
}

var sinkPrediction Prediction

func BenchmarkPredict(b *testing.B) {
	ins := benchInputs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, err := Predict(ins[i%len(ins)])
		if err != nil {
			b.Fatal(err)
		}
		sinkPrediction = p
	}
}

// BenchmarkPredictFuel isolates each fuel, so a slow path shows up by name.
func BenchmarkPredictFuel(b *testing.B) {
	for _, in := range benchInputs() {
		b.Run(in.Fuel, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sinkPrediction, _ = Predict(in)
			}
		})
	}
}

// benchGrid is n cells of random but valid inputs, every layer present.
func benchGrid(n int) Grid {
	r := rand.New(rand.NewSource(1))
	layer := func(lo, hi float64) []float64 {
		v := make([]float64, n)
		for i := range v {
			v[i] = lo + r.Float64()*(hi-lo)
		}
		return v
	}
	g := Grid{
		Fuel: make([]float64, n),
		FFMC: layer(80, 96), BUI: layer(20, 150),
		WindKmh: layer(0, 40), WindAzimuthDeg: layer(0, 360),
		SlopePct: layer(0, 60), UpslopeAzimuthDeg: layer(0, 360),
		PC: layer(0, 100), PDF: layer(0, 100), CuringPct: layer(50, 100),
		GrassFuelLoad: layer(0.1, 1),
		FMC:           make([]float64, n),
		Lat:           layer(45, 65), Long: layer(-130, -90), ElevM: layer(0, 1500),
		DayOfYear: layer(120, 270), D0: make([]float64, n),
		CBH: layer(-1, -1), CFL: layer(-1, -1),
		StandDensity: layer(0, 0), StandHeight: layer(0, 0),
		ElapsedMin: layer(10, 120),
	}
	for i := range g.Fuel {
		// Skip NF (13) and WA (19), which are refused.
		c := 1 + r.Intn(17)
		if c >= 13 {
			c++
		}
		if c >= 19 {
			c = 18
		}
		g.Fuel[i] = float64(c)
		if c == 6 {
			g.SlopePct[i] = 0 // C6 on a slope is refused
		}
	}
	return g
}

var sinkGrid []Prediction

func BenchmarkPredictGrid(b *testing.B) {
	for _, n := range []int{1_000, 100_000} {
		g := benchGrid(n)
		b.Run(itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out, errs, err := PredictGrid(g, GridOptions{Crown: CrownCFFDRS, PointIgnition: true})
				if err != nil {
					b.Fatal(err)
				}
				for _, e := range errs {
					if e != nil {
						b.Fatal(e)
					}
				}
				sinkGrid = out
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/cell")
		})
	}
}

func itoa(n int) string {
	if n >= 1000 && n%1000 == 0 {
		return itoa(n/1000) + "k"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

// BenchmarkPredictParallel is Predict on every core: cells share nothing, so
// this is the ceiling for a caller who splits a grid across goroutines.
func BenchmarkPredictParallel(b *testing.B) {
	ins := benchInputs()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		var p Prediction
		for i := 0; pb.Next(); i++ {
			p, _ = Predict(ins[i%len(ins)])
		}
		_ = p
	})
}
