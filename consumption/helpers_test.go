package consumption

import "github.com/LukasSelin/gofbp/spread"

// slopeAdjustedROS is spread's test helper of the same name — the surface rate
// with the slope folded into the wind. Restated because test helpers do not
// cross packages; see spread/cffdrs_test.go for why it is not package code.
func slopeAdjustedROS(s spread.SlopeWind, bui float64) (rosMMin, razDeg float64) {
	wsv, raz := spread.NetEffectiveWind(s)
	return spread.RSI(s.Code, spread.ISI(s.FFMC, wsv), s.PC, s.PDF, s.CuringPct) * spread.BuildupEffect(s.Code, bui), raz
}
