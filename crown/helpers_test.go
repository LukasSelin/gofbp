package crown

import (
	"testing"

	"github.com/LukasSelin/gofbp/internal/cffdrstest"
	"github.com/LukasSelin/gofbp/spread"
)

// slopeAdjustedROS is spread's test helper of the same name — the surface rate
// with the slope folded into the wind. Restated because test helpers do not
// cross packages; see spread/cffdrs_test.go for why it is not package code.
func slopeAdjustedROS(s spread.SlopeWind, bui float64) (rosMMin, razDeg float64) {
	wsv, raz := spread.NetEffectiveWind(s)
	return spread.RSI(s.Code, spread.ISI(s.FFMC, wsv), s.PC, s.PDF, s.CuringPct) * spread.BuildupEffect(s.Code, bui), raz
}

// The fixture helpers restate the CBH and CFL bounds cffdrs honours verbatim,
// because cffdrstest cannot import this package. If the two drift, every crown
// test filters the fixture on the wrong rows without failing.
func TestFixtureBoundsAreCrowns(t *testing.T) {
	if cffdrstest.MaxSuppliedCBH != MaxSuppliedCrownBaseHeightM {
		t.Errorf("cffdrstest.MaxSuppliedCBH = %v, MaxSuppliedCrownBaseHeightM = %v",
			cffdrstest.MaxSuppliedCBH, MaxSuppliedCrownBaseHeightM)
	}
	if cffdrstest.MaxSuppliedCFL != MaxSuppliedCrownFuelLoadKgM2 {
		t.Errorf("cffdrstest.MaxSuppliedCFL = %v, MaxSuppliedCrownFuelLoadKgM2 = %v",
			cffdrstest.MaxSuppliedCFL, MaxSuppliedCrownFuelLoadKgM2)
	}
}
