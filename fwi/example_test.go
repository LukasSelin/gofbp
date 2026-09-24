package fwi_test

import (
	"fmt"

	"github.com/LukasSelin/gofbp/fwi"
)

// Three July days at Umeå (63.8 °N), chained from the conventional start-up
// codes: two dry days, then a wet one. The weather arrives the way model output
// does — wind in m/s — and is converted at the boundary, because the FWI System
// takes km/h.
//
// The numbers are cffdrs' own: fwi() on the same three days gives FFMC 89.5531,
// 92.0631, 45.6301 and FWI 8.71897, 16.6471, 0.0861684. The 9.4 mm day is
// above all three rain thresholds, so every code falls — and the DC, the
// slowest, barely does.
func Example() {
	const lat = 63.8
	const month = 7

	days := []struct {
		tempC, rhPct, windMS, precip24hMm float64
	}{
		{22, 35, 4.2, 0},   // warm and dry
		{25, 28, 5.5, 0},   // drier and windier
		{16, 80, 3.0, 9.4}, // a front: 9.4 mm in the 24 h to noon
	}

	s := fwi.StartupState() // 85, 6, 15 — the caller's choice, not a default
	for i, d := range days {
		w := fwi.Weather{
			TempC:    d.tempC,
			RHPct:    d.rhPct,
			WindKmh:  d.windMS * 3.6, // m/s → km/h
			PrecipMm: d.precip24hMm,
		}
		var ix fwi.Indices
		s, ix = fwi.Step(s, w, month, lat)
		fmt.Printf("day %d  FFMC %5.1f  DMC %5.1f  DC %5.1f  ISI %4.1f  BUI %5.1f  FWI %4.1f  DSR %4.2f\n",
			i+1, ix.FFMC, ix.DMC, ix.DC, ix.ISI, ix.BUI, ix.FWI, ix.DSR)
	}
	// Output:
	// day 1  FFMC  89.6  DMC   9.5  DC  22.7  ISI  8.6  BUI   9.5  FWI  8.7  DSR 1.26
	// day 2  FFMC  92.1  DMC  13.9  DC  30.9  ISI 15.6  BUI  13.9  FWI 16.6  DSR 3.95
	// day 3  FFMC  45.6  DMC   7.7  DC  23.8  ISI  0.2  BUI   8.5  FWI  0.1  DSR 0.00
}
