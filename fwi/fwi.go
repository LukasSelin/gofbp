package fwi

import "math"

// The conventional start-up codes: what cffdrs' fwi() starts from when it is
// given no init (Van Wagner 1987's default spring start-up). They are the
// caller's to choose. A season that starts after snowmelt on dry ground, or a DC
// carried over a winter, starts somewhere else, and that decision belongs with
// whoever knows the ground — so these are exported values, not defaults any
// function here applies.
const (
	StartupFFMC = 85.0
	StartupDMC  = 6.0
	StartupDC   = 15.0
)

// ffmcCoefficient converts between FFMC and fine fuel moisture content (%).
// cffdrs' FFMC_COEFFICIENT: the exact 250·59.5/101, where Van Wagner (1987)
// prints the rounded 147.2. Package fbp carries the same constant; it is not
// shared, because this package imports nothing but math.
const ffmcCoefficient = 250.0 * 59.5 / 101.0

// The rain thresholds, read off cffdrs 1.9.2 (and unchanged at upstream
// 4d20a30). Each code ignores rain at or below its threshold: FFMC applies rain
// when precipitation is > 0.5 mm, DMC when > 1.5 mm, DC when > 2.8 mm.
const (
	ffmcRainThreshold = 0.5
	dmcRainThreshold  = 1.5
	dcRainThreshold   = 2.8
)

// FFMC is the Fine Fuel Moisture Code for today, from yesterday's FFMC and
// today's noon temperature (°C), relative humidity (%), wind (km/h) and 24-hour
// precipitation (mm). Van Wagner (1987) eqs. 1-10, as cffdrs'
// fine_fuel_moisture_code computes them:
//
//   - Rain applies only above 0.5 mm, and 0.5 mm is taken off as canopy
//     interception. Moisture after rain is capped at 250 %.
//   - The fuel then dries towards the drying equilibrium ed if it is wetter
//     than that, wets towards the wetting equilibrium ew if it is drier than
//     that, and is left alone in between.
//   - The result is clamped to [0, 101]. The top of that binds materially
//     only at temperatures no weather produces (above about 56.6 °C, at low
//     humidity, where ed goes negative); otherwise all it ever absorbs is a
//     rounding ulp, because eqs. 1 and 10 are inverses only up to rounding and
//     an unchanged FFMC of 101 comes back as 101.00000000000001. The bottom is
//     not reachable at all: it needs moisture
//     above 250 %, which needs ew above 250, which needs a temperature near
//     −1200 °C, where the temperature term has already flattened the rate to
//     nothing.
//
// RH is used as given. cffdrs' fwi() driver clamps RH ≥ 100 to 99.9999 before
// calling this; Step does the same, and this function, like its cffdrs
// counterpart, does not.
//
// A NaN in any input gives NaN, the way R's NA propagates. Negative wind gives
// NaN from the square root; fwi() refuses negative wind, RH and precipitation
// outright.
func FFMC(ffmcYesterday, tempC, rhPct, windKmh, precipMm float64) float64 {
	if anyNaN(ffmcYesterday, tempC, rhPct, windKmh, precipMm) {
		return math.NaN()
	}
	ffmc := ffmcUnclamped(ffmcYesterday, tempC, rhPct, windKmh, precipMm)
	if ffmc > 101 {
		ffmc = 101
	}
	if ffmc < 0 {
		ffmc = 0
	}
	return ffmc
}

// ffmcUnclamped is eqs. 1-10 without cffdrs' final clamp to [0, 101]. It is
// separate so the claims FFMC's doc comment makes about when that clamp binds
// can be tested against the arithmetic rather than asserted.
func ffmcUnclamped(ffmcYesterday, tempC, rhPct, windKmh, precipMm float64) float64 {
	rh, temp, ws := rhPct, tempC, windKmh

	// Eq. 1: yesterday's moisture content.
	wmo := ffmcCoefficient * (101 - ffmcYesterday) / (59.5 + ffmcYesterday)

	// Eqs. 2, 3a, 3b: rain.
	if precipMm > ffmcRainThreshold {
		ra := precipMm - ffmcRainThreshold
		if wmo > 150 {
			wmo = wmo + 0.0015*(wmo-150)*(wmo-150)*math.Sqrt(ra) +
				42.5*ra*math.Exp(-100/(251-wmo))*(1-math.Exp(-6.93/ra))
		} else {
			wmo = wmo + 42.5*ra*math.Exp(-100/(251-wmo))*(1-math.Exp(-6.93/ra))
		}
	}
	if wmo > 250 {
		wmo = 250
	}

	// Eqs. 4, 5: the drying and wetting equilibria.
	ed := 0.942*math.Pow(rh, 0.679) + 11*math.Exp((rh-100)/10) +
		0.18*(21.1-temp)*(1-1/math.Exp(rh*0.115))
	ew := 0.618*math.Pow(rh, 0.753) + 10*math.Exp((rh-100)/10) +
		0.18*(21.1-temp)*(1-1/math.Exp(rh*0.115))

	wm := wmo
	// Eqs. 7a, 7b, 9: wetting, when drier than both equilibria. cffdrs tests
	// both; ed > ew for every RH in [0, 100], so it is the same as testing ew.
	if wmo < ed && wmo < ew {
		z := 0.424*(1-math.Pow((100-rh)/100, 1.7)) +
			0.0694*math.Sqrt(ws)*(1-math.Pow((100-rh)/100, 8))
		x := z * 0.581 * math.Exp(0.0365*temp)
		wm = ew - (ew-wmo)/math.Pow(10, x)
	}
	// Eqs. 6a, 6b, 8: drying, when wetter than the drying equilibrium.
	if wmo > ed {
		z := 0.424*(1-math.Pow(rh/100, 1.7)) +
			0.0694*math.Sqrt(ws)*(1-math.Pow(rh/100, 8))
		x := z * 0.581 * math.Exp(0.0365*temp)
		wm = ed + (wmo-ed)/math.Pow(10, x)
	}

	// Eq. 10.
	return (59.5 * (250 - wm)) / (ffmcCoefficient + wm)
}

// DMC is the Duff Moisture Code for today, from yesterday's DMC and today's
// noon temperature (°C), relative humidity (%) and 24-hour precipitation (mm),
// in the given month (1-12) at the given latitude (degrees, north positive).
// Van Wagner (1987) eqs. 11-17, as cffdrs' duff_moisture_code computes them:
//
//   - Temperature is floored at −1.1 °C.
//   - Drying is 1.894·(T+1.1)·(100−RH)·Le·10⁻⁴, with Le from DMCDayLength —
//     fwi()'s latitude-adjusted day length (lat.adjust = TRUE).
//   - Rain applies only above 1.5 mm. Moisture after rain uses cffdrs'
//     280/exp(0.023·P) and 43.43·(5.6348 − ln(M − 20)) rather than the
//     published eqs. 12 and 15, which upstream calls an alteration "to
//     calculate more accurately". The rain-adjusted code is floored at 0, and
//     so is the result.
//
// That alteration is not free, and it is worth knowing what it costs. The
// published pair are exact inverses: exp(5.6348 − P/43.43) out, 43.43·(5.6348 −
// ln(·)) back. The altered pair round 1/43.43 to 0.023 and exp(5.6348) to 280,
// so they are not — with no moisture added between them they would take a DMC
// of P to 0.99889·P + 0.00045. Every day with more than 1.5 mm of rain therefore
// knocks about 0.11 % more off the DMC than the published equations would,
// always downward. This package reproduces it, because matching cffdrs is the
// point; the oracle confirms the Go does the same.
//
// To reproduce fwi(lat.adjust = FALSE), pass any latitude north of 30°: the
// unadjusted table is the one that band uses. 46 is the latitude the table was
// built for.
//
// RH is used as given; see FFMC. A month outside 1-12 or a NaN input gives NaN.
func DMC(dmcYesterday, tempC, rhPct, precipMm float64, month int, latDeg float64) float64 {
	le := DMCDayLength(month, latDeg)
	if anyNaN(le, dmcYesterday, tempC, rhPct, precipMm) {
		return math.NaN()
	}
	temp, rh := tempC, rhPct
	if temp < -1.1 {
		temp = -1.1
	}
	// Eq. 16, times the 100 of eq. 17.
	rk := 1.894 * (temp + 1.1) * (100 - rh) * le * 1e-04

	pr := dmcYesterday
	if precipMm > dmcRainThreshold {
		ra := precipMm
		rw := 0.92*ra - 1.27                         // eq. 11
		wmi := 20 + 280/math.Exp(0.023*dmcYesterday) // eq. 12, as altered
		var b float64                                // eqs. 13a-13c
		switch {
		case dmcYesterday <= 33:
			b = 100 / (0.5 + 0.3*dmcYesterday)
		case dmcYesterday <= 65:
			b = 14 - 1.3*math.Log(dmcYesterday)
		default:
			b = 6.2*math.Log(dmcYesterday) - 17.2
		}
		wmr := wmi + 1000*rw/(48.77+b*rw)        // eq. 14
		pr = 43.43 * (5.6348 - math.Log(wmr-20)) // eq. 15, as altered
	}
	if pr < 0 {
		pr = 0
	}
	dmc := pr + rk
	if dmc < 0 {
		dmc = 0
	}
	return dmc
}

// DC is the Drought Code for today, from yesterday's DC and today's noon
// temperature (°C), relative humidity (%) and 24-hour precipitation (mm), in
// the given month (1-12) at the given latitude (degrees, north positive). Van
// Wagner (1987) eqs. 18-23, as cffdrs' drought_code computes them:
//
//   - Temperature is floored at −2.8 °C.
//   - Potential evapotranspiration is (0.36·(T+2.8) + Lf)/2, floored at 0,
//     with Lf from DCDayLength — fwi()'s latitude-adjusted day length.
//   - Rain applies only above 2.8 mm, through cffdrs' rearrangement of eq. 21,
//     DC − 400·ln(1 + 3.937·rw/Q), which is the published form's algebra. The
//     rain-adjusted code is floored at 0.
//
// The DC does not depend on relative humidity. It takes one anyway because
// cffdrs' drought_code does, and so that the parameter lists of the three
// codes line up; the value is ignored, NaN included.
//
// To reproduce fwi(lat.adjust = FALSE), pass any latitude north of 20°.
//
// A month outside 1-12 or a NaN input gives NaN.
func DC(dcYesterday, tempC, rhPct, precipMm float64, month int, latDeg float64) float64 {
	lf := DCDayLength(month, latDeg)
	if anyNaN(lf, dcYesterday, tempC, precipMm) {
		return math.NaN()
	}
	temp := tempC
	if temp < -2.8 {
		temp = -2.8
	}
	pe := (0.36*(temp+2.8) + lf) / 2 // eq. 22, halved as in eq. 23
	if pe < 0 {
		pe = 0
	}

	dr := dcYesterday
	if precipMm > dcRainThreshold {
		ra := precipMm
		rw := 0.83*ra - 1.27                      // eq. 18
		smi := 800 * math.Exp(-1*dcYesterday/400) // eq. 19
		dr0 := dcYesterday - 400*math.Log(1+3.937*rw/smi)
		if dr0 < 0 {
			dr0 = 0
		}
		dr = dr0
	}
	dc := dr + pe
	if dc < 0 {
		dc = 0
	}
	return dc
}

// The day-length tables, as cffdrs 1.9.2 carries them.
var (
	// DMC effective day length Le, hours, by month. ell01 is Van Wagner
	// (1987)'s table, built for 46 °N; the others are cffdrs' latitude
	// adjustment.
	dmcEll01 = [12]float64{6.5, 7.5, 9, 12.8, 13.9, 13.9, 12.4, 10.9, 9.4, 8, 7, 6}
	dmcEll02 = [12]float64{7.9, 8.4, 8.9, 9.5, 9.9, 10.2, 10.1, 9.7, 9.1, 8.6, 8.1, 7.8}
	dmcEll03 = [12]float64{10.1, 9.6, 9.1, 8.5, 8.1, 7.8, 7.9, 8.3, 8.9, 9.4, 9.9, 10.2}
	dmcEll04 = [12]float64{11.5, 10.5, 9.2, 7.9, 6.8, 6.2, 6.5, 7.4, 8.7, 10, 11.2, 11.8}

	// DC day-length factor Lf by month. dcFl01 is Van Wagner (1987)'s.
	dcFl01 = [12]float64{-1.6, -1.6, -1.6, 0.9, 3.8, 5.8, 6.4, 5, 2.4, 0.4, -1.6, -1.6}
	dcFl02 = [12]float64{6.4, 5, 2.4, 0.4, -1.6, -1.6, -1.6, -1.6, -1.6, 0.9, 3.8, 5.8}
)

// dmcEquatorialLe and dcEquatorialLf are the constant factors cffdrs uses near
// the equator, for every month.
const (
	dmcEquatorialLe = 9.0
	dcEquatorialLf  = 1.4
)

// DMCDayLength is the DMC's effective day length Le for a month (1-12) at a
// latitude (degrees, north positive), with fwi()'s latitude adjustment. The
// bands, exactly as cffdrs' duff_moisture_code draws them:
//
//	lat > 30           Van Wagner (1987)'s table, built for 46 °N
//	10 < lat ≤ 30      cffdrs' 20 °N table
//	−10 < lat ≤ 10     9 in every month
//	−30 < lat ≤ −10    cffdrs' 20 °S table
//	−90 ≤ lat ≤ −30    cffdrs' 40 °S table
//	lat < −90          Van Wagner's table again
//
// Two of those are worth knowing about. Exactly 30° is in the 20 °N band:
// upstream's comment says the 46 °N table applies from 30 °N, and the code
// says strictly above. And a latitude below −90 matches none of the adjusted
// bands, so it falls through to the northern table; that is invalid input, but
// it is what cffdrs returns, so it is what this returns.
//
// All of Scandinavia and Canada are in the first band, where the adjusted and
// unadjusted factors agree. A month outside 1-12 or a NaN latitude gives NaN.
func DMCDayLength(month int, latDeg float64) float64 {
	if month < 1 || month > 12 || math.IsNaN(latDeg) {
		return math.NaN()
	}
	m := month - 1
	switch {
	case latDeg <= 30 && latDeg > 10:
		return dmcEll02[m]
	case latDeg <= -10 && latDeg > -30:
		return dmcEll03[m]
	case latDeg <= -30 && latDeg >= -90:
		return dmcEll04[m]
	case latDeg <= 10 && latDeg > -10:
		return dmcEquatorialLe
	default:
		return dmcEll01[m]
	}
}

// DCDayLength is the DC's day-length factor Lf for a month (1-12) at a latitude
// (degrees, north positive), with fwi()'s latitude adjustment. The bands, as
// cffdrs' drought_code draws them:
//
//	lat > 20           Van Wagner (1987)'s table
//	−20 < lat ≤ 20     1.4 in every month
//	lat ≤ −20          cffdrs' southern table, with no lower bound
//
// These are not the DMC's bands: 15 °N is in the DMC's 20 °N band and the DC's
// equatorial one. A month outside 1-12 or a NaN latitude gives NaN.
func DCDayLength(month int, latDeg float64) float64 {
	if month < 1 || month > 12 || math.IsNaN(latDeg) {
		return math.NaN()
	}
	m := month - 1
	switch {
	case latDeg <= -20:
		return dcFl02[m]
	case latDeg > -20 && latDeg <= 20:
		return dcEquatorialLf
	default:
		return dcFl01[m]
	}
}

// ISI is the FWI System's Initial Spread Index from FFMC and wind (km/h): Van
// Wagner (1987) eqs. 24-26, cffdrs' initial_spread_index with fbpMod = FALSE,
// which is what fwi() uses.
//
// This is not fbp.ISI, and the two must not be swapped. They share a name and
// eqs. 25-26, but fbp.ISI uses the FBP System's high-wind wind function (ST-X-3
// eq. 53a, fbpMod = TRUE), which takes over at 40 km/h and saturates, where this
// one stays exp(0.05039·wind) at every speed. At 60 km/h the FWI System's wind
// function is about 1.85 times FBP's, and the gap grows without bound. fbp.ISI also returns 0 for FFMC outside
// (0, 101]; this one computes FFMC 0 like any other value, because an FFMC of 0
// is a legitimate output of FFMC.
//
// FFMC above 101 has negative moisture and gives NaN, as in cffdrs.
func ISI(ffmc, windKmh float64) float64 {
	fm := ffmcCoefficient * (101 - ffmc) / (59.5 + ffmc)                  // eq. 10's inverse
	fW := math.Exp(0.05039 * windKmh)                                     // eq. 24
	fF := 91.9 * math.Exp(-0.1386*fm) * (1 + math.Pow(fm, 5.31)/49300000) // eq. 25
	return 0.208 * fW * fF                                                // eq. 26
}

// BUI is the Buildup Index from today's DMC and DC: Van Wagner (1987) eqs. 27a
// and 27b, as cffdrs' buildup_index computes them. BUI is 0 whenever DMC is 0,
// including the 0/0 case where both codes are.
func BUI(dmc, dc float64) float64 {
	var bui1 float64
	if !(dmc == 0 && dc == 0) {
		bui1 = 0.8 * dc * dmc / (dmc + 0.4*dc) // eq. 27a
	}
	var p float64
	if dmc != 0 {
		p = (dmc - bui1) / dmc
	}
	cc := 0.92 + math.Pow(0.0114*dmc, 1.7)
	bui0 := dmc - cc*p // eq. 27b
	if bui0 < 0 {
		bui0 = 0
	}
	if bui1 < dmc {
		bui1 = bui0
	}
	return bui1
}

// FWI is the Fire Weather Index from ISI and BUI: Van Wagner (1987) eqs. 28-30,
// as cffdrs' fire_weather_index computes them.
//
// FWI is not monotone in BUI, and that is the published system rather than a
// transcription slip. The duff moisture function fD switches from eq. 28a to
// eq. 28b above BUI 80, and the two do not meet there: fD is 23.686 at 80 and
// 23.666 just above, a 0.08 % step down. Within each side FWI rises with BUI;
// across 80 it dips by that much. TestFWIStepsDownAtBUI80 pins it.
func FWI(isi, bui float64) float64 {
	var bb float64
	if bui > 80 {
		bb = 0.1 * isi * (1000 / (25 + 108.64/math.Exp(0.023*bui))) // eqs. 28b, 29
	} else {
		bb = 0.1 * isi * (0.626*math.Pow(bui, 0.809) + 2) // eqs. 28a, 29
	}
	if bb <= 1 {
		return bb // eq. 30b
	}
	return math.Exp(2.72 * math.Pow(0.434*math.Log(bb), 0.647)) // eq. 30a
}

// DSR is the Daily Severity Rating from FWI: 0.0272·FWI^1.77, as fwi()
// computes it (upstream labels it eq. 31). cffdrs has no separate function
// for it; it is one line inside fwi().
func DSR(fwi float64) float64 {
	return 0.0272 * math.Pow(fwi, 1.77)
}

func anyNaN(xs ...float64) bool {
	for _, x := range xs {
		if math.IsNaN(x) {
			return true
		}
	}
	return false
}
