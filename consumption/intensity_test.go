package consumption

import (
	"math"
	"testing"

	"github.com/LukasSelin/gofbp/crown"
	"github.com/LukasSelin/gofbp/internal/cffdrstest"
	"github.com/LukasSelin/gofbp/spread"
)

// Fireline intensity, without the fixture.
//
// Eq. 69 has one constant and no branches, so what these hold is that constant,
// the shape of the product, the absence of any floor, and the two compositions
// that make up HFI — including two end-to-end values copied from the oracle, so
// that CI on a fresh clone still checks the whole chain at one surface-fire-plus-
// crown point and one C6 point rather than only its algebra.

// 300 is 18,000 kJ/kg over 60 s/min, and it is the whole of eq. 69's content.
func TestFireIntensityConstant(t *testing.T) {
	if heatYieldFactor != 300 {
		t.Fatalf("heatYieldFactor = %v, want exactly 300 (18000 kJ/kg / 60 s/min)", heatYieldFactor)
	}
	if got := FireIntensity(1, 1); got != 300 {
		t.Errorf("FireIntensity(1 kg/m², 1 m/min) = %v kW/m, want 300", got)
	}
	// The order of magnitude worth having in mind: 1 kg/m² at 10 m/min is
	// already 3,000 kW/m.
	if got := FireIntensity(1, 10); got != 3000 {
		t.Errorf("FireIntensity(1, 10) = %v, want 3000", got)
	}
}

// The product is linear in each factor and symmetric in their roles: there is
// no per-fuel coefficient and no threshold, so doubling either doubles it.
func TestFireIntensityIsBilinear(t *testing.T) {
	for _, fc := range []float64{0, 0.25, 1, 2.6634536802915125, 7} {
		for _, ros := range []float64{0, 0.5, 3, 34.62953830166129, 80} {
			base := FireIntensity(fc, ros)
			if got := FireIntensity(2*fc, ros); !cffdrstest.CloseEnough(got, 2*base, 1e-15) {
				t.Errorf("FireIntensity(2·%v, %v) = %v, want 2·%v", fc, ros, got, base)
			}
			if got := FireIntensity(fc, 2*ros); !cffdrstest.CloseEnough(got, 2*base, 1e-15) {
				t.Errorf("FireIntensity(%v, 2·%v) = %v, want 2·%v", fc, ros, got, base)
			}
			if got := FireIntensity(ros, fc); !cffdrstest.CloseEnough(got, base, 1e-15) {
				t.Errorf("FireIntensity(%v, %v) = %v, but swapped it is %v", ros, fc, got, base)
			}
		}
	}
}

// No floor and no clamp, which is cffdrs' behaviour: zero in either factor is
// zero intensity, and a negative factor is passed through rather than corrected
// into something plausible.
func TestFireIntensityHasNoFloor(t *testing.T) {
	if got := FireIntensity(0, 50); got != 0 {
		t.Errorf("FireIntensity(0, 50) = %v, want 0", got)
	}
	if got := FireIntensity(3, 0); got != 0 {
		t.Errorf("FireIntensity(3, 0) = %v, want 0", got)
	}
	if got := FireIntensity(-1, 2); got != -600 {
		t.Errorf("FireIntensity(-1, 2) = %v, want -600 passed through", got)
	}
}

// An infinite input is a broken driver, and comes back NaN rather than as an
// infinitely intense fire; NaN propagates.
func TestFireIntensityNonFiniteInput(t *testing.T) {
	inf := math.Inf(1)
	for _, in := range [][2]float64{
		{inf, 1}, {1, inf}, {-inf, 1}, {inf, 0}, {math.NaN(), 1}, {1, math.NaN()},
		{math.MaxFloat64, math.MaxFloat64}, // finite in, overflows out
	} {
		if got := FireIntensity(in[0], in[1]); !math.IsNaN(got) {
			t.Errorf("FireIntensity(%v, %v) = %v, want NaN", in[0], in[1], got)
		}
	}
}

// Head fire intensity end to end, at two points copied from cffdrs 1.9.2's
// fbp() output (the hfi column, flat ground, explicit CBH 20 and CFL 1). The
// C2 point is an intermittent crown fire whose ROS is the surface rate and
// whose TFC carries 0.82 of the canopy; the C6 point is one whose ROS is the
// crown-blended rate. Together they check the composition HFI needs — the
// right rate for the right fuel — on every push, not only where the fixture
// has been generated. TestCFFDRSHeadFireIntensity is the full sweep.
func TestHeadFireIntensityMatchesCFFDRSWorkedPoints(t *testing.T) {
	const fmc, cbh, cfl = 99.8176, 20.0, 1.0
	const tol = 1e-9

	t.Run("C2", func(t *testing.T) {
		const ffmc, bui, ws = 92.0, 40.0, 30.0
		sfc := SurfaceFuelConsumption("C2", ffmc, bui, 50, 0.35)
		rss := spread.RSI("C2", spread.ISI(ffmc, ws), 50, 35, 80) * spread.BuildupEffect("C2", bui)
		cfb := crown.CrownFractionBurned(crown.Crown{FMC: fmc, SFC: sfc, CBH: cbh, CFL: cfl, SurfaceROS: rss})
		hfi := FireIntensity(TotalFuelConsumption("C2", cfl, cfb, sfc, 50, 35), rss)
		if !cffdrstest.CloseEnough(cfb, 0.819871907826142, tol) {
			t.Errorf("CFB = %v, cffdrs 0.819871907826142", cfb)
		}
		if !cffdrstest.CloseEnough(hfi, 27670.2513709067, tol) {
			t.Errorf("HFI = %v kW/m, cffdrs 27670.2513709067", hfi)
		}
	})

	t.Run("C6", func(t *testing.T) {
		const ffmc, bui, ws = 92.0, 100.0, 30.0
		isi := spread.ISI(ffmc, ws)
		sfc := SurfaceFuelConsumption("C6", ffmc, bui, 50, 0.35)
		rss := spread.RSI("C6", isi, 50, 35, 80) * spread.BuildupEffect("C6", bui)
		ros, cfb := crown.C6CrownFire(crown.Crown{FMC: fmc, SFC: sfc, CBH: cbh, CFL: cfl, SurfaceROS: rss}, isi)
		hfi := FireIntensity(TotalFuelConsumption("C6", cfl, cfb, sfc, 50, 35), ros)
		if !cffdrstest.CloseEnough(ros, 30.160812209884725, tol) {
			t.Errorf("blended ROS = %v, cffdrs 30.160812209884725", ros)
		}
		if !cffdrstest.CloseEnough(hfi, 27998.431435639646, tol) {
			t.Errorf("HFI = %v kW/m, cffdrs 27998.431435639646", hfi)
		}
		// The point of this subtest: the surface rate would give a different,
		// wrong number, so a caller who feeds RSS for C6 is detectably off.
		if wrong := FireIntensity(TotalFuelConsumption("C6", cfl, cfb, sfc, 50, 35), rss); cffdrstest.CloseEnough(wrong, hfi, 1e-3) {
			t.Errorf("HFI on the surface rate (%v) is indistinguishable from HFI on the blended rate (%v) — "+
				"this point no longer separates the two", wrong, hfi)
		}
	})
}
