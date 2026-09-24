// Package fwi is the Canadian Forest Fire Weather Index (FWI) System's daily
// codes and indices, ported from cffdrs, the Canadian Forest Service's R
// implementation, and checked against it.
//
//	import "github.com/LukasSelin/gofbp/fwi"
//
// It sits beside package fbp in the same module and shares nothing with it. The
// FWI System turns weather into moisture codes and fire-danger indices; the FBP
// System takes two of those codes (FFMC and BUI) as given and turns them into a
// rate of spread. Keeping them apart keeps package fbp's claim — the FBP System
// and nothing else — true, and keeps an index out of the package that produces
// a physical quantity. Neither package imports the other, and this one imports
// math and nothing else.
//
// # What it is
//
// One pure function per code, each taking yesterday's value and today's
// weather: FFMC, DMC, DC, then ISI, BUI, FWI and DSR from those. DMCDayLength
// and DCDayLength are the latitude-adjusted day-length factors the DMC and DC
// use. Step chains one day the way cffdrs' fwi() does, including the one thing
// fwi() does that the component functions do not: it clamps relative humidity
// at or above 100 % to 99.9999 before any code sees it.
//
// What it is not: a time-series driver. There is no loop over days, no
// gridding, no start-of-season or overwintering logic, and no default for a
// missing latitude or month (fwi() falls back to 55 °N and July with only a
// warning). Where a
// season starts, what the codes start at, and what to do across a gap in the
// weather are the caller's decisions. StartupFFMC, StartupDMC and StartupDC are
// the conventional start-up values, exported so a caller can choose them
// rather than inherit them.
//
// # Units
//
// These are cffdrs' units, which are the FWI System's own. They are not the
// units weather data usually arrives in.
//
//	Quantity          Unit
//	Temperature       °C, at local noon
//	Relative humidity %, at local noon
//	Wind speed        km/h, 10 m open wind, at local noon — NOT m/s
//	Precipitation     mm over the 24 hours ending at local noon — NOT hourly
//	Month             1–12
//	Latitude          degrees, north positive
//	FFMC, DMC, DC     the codes' own scales (FFMC 0–101; DMC, DC ≥ 0)
//	ISI, BUI, FWI, DSR the indices' own unitless scales
//
// Two conversions are where this goes wrong in practice:
//
//   - Wind from a model or reanalysis is almost always m/s. Multiply by 3.6.
//     Passing m/s understates the wind by that factor, and ISI is exponential
//     in wind, so FWI comes out far too low with nothing to show for it.
//   - Precipitation usually arrives hourly or as an accumulation since some
//     model reference time (often in metres). The codes want the 24-hour total
//     ending at noon local standard time, in millimetres. An hour's rain passed
//     as the day's total falls under the rain thresholds (0.5, 1.5 and 2.8 mm)
//     almost every time, and the codes then never wet.
//
// # Where this follows cffdrs rather than the 1987 report
//
// Van Wagner (1987) is the published system; cffdrs is what the Canadian Forest
// Service ships. Where they differ, this package does what cffdrs does:
//
//   - The day-length factors are adjusted by latitude (fwi()'s lat.adjust,
//     default TRUE). The 1987 tables are the northern mid-latitude ones; they
//     are what DMCDayLength and DCDayLength return north of 30 °N and 20 °N,
//     which covers Canada and the whole of Scandinavia.
//   - FFMC's moisture scale is the exact 250·59.5/101 = 147.2772…, not 1987's
//     rounded 147.2.
//   - DMC after rain uses 280/exp(0.023·P) and 43.43·(5.6348 − ln(M − 20)) —
//     cffdrs' "alteration … to calculate more accurately" of eqs. 12 and 15.
//   - FFMC is clamped to [0, 101], and RH at or above 100 % is clamped to
//     99.9999 by the driver (Step), not by the codes.
//
// # Correctness
//
// The TestCFFDRS* tests in this package assert every function against the
// cffdrs fixture (testdata/cffdrs.json, generated rather than committed — see
// testdata/README.md): each component over a grid that reaches every branch,
// fwi()'s one-day step, multi-day chains run through fwi() itself, and
// cffdrs' own test_fwi sample dataset.
package fwi
