# gofbp

[![Go](https://github.com/LukasSelin/gofbp/actions/workflows/go.yml/badge.svg)](https://github.com/LukasSelin/gofbp/actions/workflows/go.yml)
[![Oracle](https://github.com/LukasSelin/gofbp/actions/workflows/oracle.yml/badge.svg)](https://github.com/LukasSelin/gofbp/actions/workflows/oracle.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/LukasSelin/gofbp.svg)](https://pkg.go.dev/github.com/LukasSelin/gofbp)

The Canadian Forest Fire Behaviour Prediction (FBP) System's head-fire rate of
spread, in Go — and, in a package of its own beside it, the daily Canadian Fire
Weather Index (FWI) System that produces FBP's inputs. Zero dependencies — each
package imports `math` and the module's own packages below it, nothing else, and
FBP and FWI never import each other. Both checked against `cffdrs`, the Canadian
Forest Service's own R implementation.

```
go get github.com/LukasSelin/gofbp
```

The module is `gofbp`. The FBP System is five packages — the four that hold its
equations, and `fbp`, which composes them — and the FWI System is one:

| Import | Package | What it is |
|---|---|---|
| `github.com/LukasSelin/gofbp` | `fbp` | the FBP driver: `Predict` for one stand, `PredictGrid` over a grid |
| `github.com/LukasSelin/gofbp/fuel` | `fuel` | the fuel types: the coefficient table and `CanonicalFuelCode` |
| `github.com/LukasSelin/gofbp/spread` | `spread` | rate of spread: RSI, BE, SF, ISI, the slope back-solve, ROS, the fire ellipse, acceleration |
| `github.com/LukasSelin/gofbp/crown` | `crown` | the crown-fire threshold (CSI, RSO, CFB, FD), C6, foliar moisture, CBH/CFL tables |
| `github.com/LukasSelin/gofbp/consumption` | `consumption` | surface, crown and total fuel consumption, and fire intensity |
| `github.com/LukasSelin/gofbp/fwi` | `fwi` | the FWI System: FFMC, DMC, DC, ISI, BUI, FWI, DSR from daily weather |

`fuel` is at the bottom; `spread`, `crown` and `consumption` each import only
`fuel`; `fbp` imports all four. `import "github.com/LukasSelin/gofbp"` binds the
identifier `fbp`, and no alias is needed. Most of this README is about the FBP
packages; see [The FWI System: package `fwi`](#the-fwi-system-package-fwi) for
the other one.

## What this is

The FBP packages are the FBP System as published: ST-X-3's equations and
coefficient tables, plus the revisions the Canadian Forest Service published in
2009 (the grass curing function, and the M3/M4 dead-balsam-fir mixedwoods) —
with no local adaptation, no defaults chosen for a particular country, and no
judgement about what the ground is made of. That constraint is the point — the
claim "this is the Canadian FBP System, unmodified" is only checkable if the
packages making it contain nothing else. So the FWI System is not in them: FBP takes FFMC and BUI
as given, and package `fwi` is where they can come from.

What it produces is a **rate of spread in metres per minute**: a physical
quantity, not a danger score. A fire-danger index is a unitless number whose
range is tied to whatever scale published it; a spread rate has units that make
it checkable against observed fire behaviour. They answer different questions —
*how dangerous are conditions today* versus *if it starts here, how fast does it
move* — and multiplying one by the other destroys both. Do not fold this into an
index.

The FWI is exactly such an index, which is why it lives in a different package.
Having both in one module makes it easy to compute them side by side; it does
not make them combinable.

Everything local — which fuel type a stand maps to, what curing to assume, what
to do with wetland — is the caller's, and reaches the package as the fuel code
its functions take as an argument.

## Units and conventions

This is where the package is most likely to be misused, because none of these are
the units weather data usually arrives in.

| Quantity | Unit |
|---|---|
| Wind speed | **km/h** (ST-X-3's unit — convert from m/s yourself) |
| Rate of spread | metres per minute |
| Slope | **percent rise**, not degrees — 45° is 100%, not 45%. Use `SlopePercentFromDegrees` |
| FFMC, BUI, ISI | the FWI System's own scales |
| Azimuths | degrees clockwise from true north |

Every azimuth is the direction the thing **pushes towards**, never where it comes
from:

- A meteorological *wind from* bearing — which is what almost all weather data
  gives you — is `WindAzimuthDeg - 180`.
- A *downslope* aspect — which is what a terrain aspect raster normally stores —
  is `UpslopeAzimuthDeg - 180`.

Only the difference of the two reaches the net wind **speed**, so getting both
conventions wrong in the same direction still gives you the right speed and the
wrong **direction**. Do not rely on that cancellation.

## Usage

```go
s := spread.SlopeWind{
	Code:              "C2",
	FFMC:              92,
	SlopePct:          spread.SlopePercentFromDegrees(20), // percent rise, not degrees
	WindKmh:           15,                              // km/h, not m/s
	WindAzimuthDeg:    45,                              // the wind pushes towards NE
	UpslopeAzimuthDeg: 45,                              // the ground rises towards NE
	PC:                100,                             // percent conifer; M1/M2 only
	// PDF (percent dead balsam fir, M3/M4 only) and CuringPct (O1A/O1B only)
	// are left at zero: C2 ignores both.
}
const bui = 80

// Slope is not a multiplier. It is back-solved into an equivalent wind and
// vector-added to the real one.
wsv, raz := spread.NetEffectiveWind(s)

// The head rate. slopePct is 0 on purpose: the slope is already inside wsv.
isi := spread.ISI(s.FFMC, wsv)
head := spread.ROS(s.Code, isi, bui, s.PC, s.PDF, s.CuringPct, 0)

// The ellipse, for "how fast is this coming at something due east of me".
back := spread.ROS(s.Code, isi*spread.BackISIRatio(wsv), bui, s.PC, s.PDF, s.CuringPct, 0)
lb := spread.LengthToBreadth(s.Code, wsv)
flank := spread.FlankROS(head, back, lb)
east := spread.ROSAtAngle(head, flank, back, spread.AngleBetweenDeg(raz, 90))
```

```
net effective wind: 30.7 km/h towards 45°
head:  44.96 m/min
flank: 5.81 m/min
back:  0.71 m/min
east:  4.65 m/min
```

That spread — 45 m/min at the head against 0.71 at the back — is why the head
rate alone is the wrong number for "how fast is this coming at *me*". See
`spread/example_test.go`, which is this program and is run by the test suite.

## The driver: `Predict`

One call that composes everything above the way `cffdrs::fbp()` does, for one
stand — and, unlike `fbp()`, defaults nothing. Every input `fbp()` would quietly
replace is an error here, and the two lookups it would quietly apply are
opt-ins you name:

```go
p, err := fbp.Predict(fbp.Input{
	Fuel: "C2", FFMC: 92, BUI: 40,
	WindKmh: 30, WindAzimuthDeg: 180, // towards: fbp()'s WD + 180
	PC: 50, PDF: 35, CuringPct: 80,
	Site:  &fbp.Site{Lat: 55, Long: -120, DayOfYear: 180}, // opt in: FMC from the site
	Crown: fbp.CrownCFFDRS, CBH: -1, CFL: -1,              // opt in: the published tables
	ElapsedMin: 60, PointIgnition: true,
})
// p.ROS, p.CFB, p.FD, p.HFI, p.RAZ, p.TFC, … p.DH, p.TI
```

Leave `Crown` unset and `Predict` returns `ErrCrownUnset`; leave out both `FMC`
and `Site` for a fuel with a crown and it returns `ErrNoFMC`; give a grass fuel
no `GrassFuelLoad` and it returns `ErrNoGrassLoad`. It refuses C6 on sloped
ground (`ErrC6OnSlope`) rather than disagree with cffdrs for a reason you
cannot see — see below — and it does not compute the rate at a bearing, which
`ROSAtAngle` does from its head, flank and back rates. Asserted against
`fbp()` on every row of the reference sweep.

`PredictGrid` is the same over a grid, as cffdrs' `fbpRaster()` computes it:
one slice per layer, the fuel layer holding `fbpRaster()`'s numeric codes 1–19
(`GridFuelCodes`; NF is 13 and WA 19), and FD as a layer is `p.FD.Code()`, 1/2/3.
A nil layer is that `Input` field's zero value. A cell `Predict` refuses gets NaN
in every output and its own error, and the rest of the grid carries on. That
departs from `fbpRaster()` in three places, all of them cells it fills in
without saying so: a code outside 1–19 (or NA) is computed there as C2; an NA
layer gets `fbp()`'s default for it; and NF and WA come back as zeros. Here
each of those is refused. A cell that is NA in every layer makes `fbpRaster()`
shift every later cell's answer by one. That is a defect, and it is not
reproduced: no other cell moves. The cells run in sequence; they are
independent, so split the slices across goroutines if you want parallelism.

## What is implemented

- `CanonicalFuelCode` — folds a fuel code to the table's spelling and says
  whether the fuel is implemented; see [Fuel codes](#fuel-codes)
- `RSI` — initial spread rate, all 17 fuel types, including the M1/M2
  percent-conifer blends, the M3/M4 percent-dead-balsam-fir blends and the O1
  grass curing factor
- `BuildupEffect` (BE), `SlopeFactor` (SF), `SlopePercentFromDegrees`
- `ISI` — the FWI System's Initial Spread Index with FBP's high-wind wind
  function (which makes it a different function from `fwi.ISI`; see below)
- `EquivalentWind` and `NetEffectiveWind` — the slope back-solve (WSE, WSV, RAZ)
- `ROS` — head-fire rate of spread
- The fire ellipse: `LengthToBreadth`, `BackISIRatio`, `FlankROS`, `ROSAtAngle`,
  `AngleBetweenDeg`
- The crown-fire threshold: `CriticalSurfaceIntensity` (CSI),
  `CriticalSurfaceROS` (RSO), `CrownFractionBurned` (CFB), `DescribeFire` (FD)
- C6's crown path: `FoliarMoistureEffect` (FME), `C6CrownROS` (RSC),
  `C6CrownFractionBurned`, `C6ROS` and `C6CrownFire` — the CFB-weighted blend of
  surface and crown rates that FBP reports for the one fuel whose ROS is not the
  surface rate
- `SurfaceFuelConsumption` (SFC) — all eleven per-fuel equations, including
  GLC-X-10's revised C1
- `FoliarMoistureContent` (FMC) and `DateOfMinimumFoliarMoisture` (D0) — foliar
  moisture from latitude, longitude, elevation and the day of the year
- `CrownFuelConsumption` (CFC) and `TotalFuelConsumption` (TFC) — the crown fuel
  a fire consumed and the surface-plus-crown total, with the M1/M2 percent-conifer
  and M3/M4 percent-dead-fir weightings the 2009 revision added
- The published per-fuel crown defaults, as a lookup you opt into:
  `DefaultCrownBaseHeight` and `DefaultCrownFuelLoad` return the table values;
  `CrownBaseHeight` and `CrownFuelLoad` reproduce what `cffdrs::fbp()` does with
  a supplied value — keep it if it is in range ((0, 50] m, (0, 2] kg/m²),
  otherwise substitute the table, or for C6 with stand data
  `C6StandCrownBaseHeight`. `Crown` calls none of them:

  ```go
  cbh, _ := crown.DefaultCrownBaseHeight("C3") // 8 m
  cfl, _ := crown.DefaultCrownFuelLoad("C3")   // 1.15 kg/m²
  cfb := crown.CrownFractionBurned(crown.Crown{FMC: fmc, SFC: sfc, CBH: cbh, CFL: cfl, SurfaceROS: rss})
  ```
- The acceleration model for a point ignition: `AccelerationAlpha` (eq. 72),
  `ROSAtTime` (eq. 70), `DistanceAtTime` (eq. 71) and `LengthToBreadthAtTime`
  (GLC-X-10 eq. 81). Time is **minutes** since ignition — `cffdrs::fbp()` takes
  hours and multiplies by 60. Pass the head fire's CFB for every rate, back
  included, to match `fbp()`
- `Predict` — the whole prediction in one call; see [The driver](#the-driver-predict)
- `PredictGrid` — `Predict` over a grid, as `cffdrs::fbpRaster()`, with
  `GridFuelCodes`, `FuelFromGridCode` and `FireDescription.Code`
- `FireIntensity` — Byram's fireline intensity, `300·FC·ROS` kW/m (eq. 69).
  The head fire intensity HFI is `FireIntensity(TotalFuelConsumption(…), ros)`,
  where `ros` is the surface rate for sixteen fuels and `C6CrownFire`'s blended
  rate for C6 — exactly how `cffdrs::fbp()` computes its `HFI` column

## The FWI System: package `fwi`

```go
import "github.com/LukasSelin/gofbp/fwi"
```

The daily Canadian Forest Fire Weather Index System, ported from cffdrs'
`fwi()` and its six component functions: one pure function per code, each taking
yesterday's value and today's weather — `FFMC`, `DMC`, `DC` — then `ISI`, `BUI`,
`FWI` and `DSR` from those, plus `DMCDayLength` and `DCDayLength`, the
latitude-adjusted day-length factors `fwi()` applies (`lat.adjust = TRUE`).
`Step` chains one day exactly the way one iteration of `fwi()`'s loop does,
including the one thing `fwi()` does that the codes do not: relative humidity
at or above 100 % becomes 99.9999 before any code sees it. `StepGrid` is the
same day over a grid, as cffdrs' `fwiRaster()` computes it: one slice per layer
(temperature, RH, wind, precipitation, latitude, and yesterday's three codes),
all the same length, in whatever cell order your raster library uses. It
refuses a negative precipitation, wind or RH anywhere with a `*GridError`, as
`fwiRaster()` stops, and lets a NaN cell through to only the outputs that read
it, as terra's NA does — a cell with no wind still has a DMC. It runs the cells
in sequence; they are independent, so split the slices across goroutines if you
want parallelism.

```go
s := fwi.StartupState() // FFMC 85, DMC 6, DC 15 — your choice, not a default
for _, day := range days {
	var ix fwi.Indices
	s, ix = fwi.Step(s, fwi.Weather{
		TempC:    day.NoonTempC,
		RHPct:    day.NoonRH,
		WindKmh:  day.NoonWindMS * 3.6, // km/h, not m/s
		PrecipMm: day.Precip24hMm,      // 24 h to noon, not hourly
	}, day.Month, lat)
	use(ix.FFMC, ix.BUI, ix.FWI)
}
```

`fwi.ISI` is **not** `spread.ISI`. They share a name and a source file
(`initial_spread_index.r`), but the FWI System's uses `exp(0.05039·wind)` at every
wind speed where FBP's switches to a saturating high-wind function at 40 km/h —
at 60 km/h the FWI System's is about 1.85 times FBP's. Feed FBP `spread.ISI`.

**Units** are cffdrs', and not the ones weather data usually arrives in:

| Quantity | Unit |
|---|---|
| Temperature | °C, at local noon |
| Relative humidity | %, at local noon |
| Wind speed | **km/h**, 10 m open wind, at local noon — multiply m/s by 3.6 |
| Precipitation | **mm over the 24 h ending at local noon** — not hourly, not metres |
| Month | 1–12 |
| Latitude | degrees, north positive |

Getting the last two rows wrong is silent. An hour's rain passed as the day's
total falls under the rain thresholds (0.5, 1.5 and 2.8 mm) almost every time,
so the codes never wet and the season looks drier every day it goes on.

**What it deliberately is not:** a time-series driver. There is no loop over
days, no raster I/O, no start-of-season or overwintering logic, and no default for
a missing latitude or month (`fwi()` and `fwiRaster()` fall back to 55 °N and July
with only a warning). Where a season starts, what the codes start from, and what to do across
a gap in the weather are the caller's decisions, next to the data that decides
them — `StartupFFMC`, `StartupDMC` and `StartupDC` are exported so the
conventional start is something you pass rather than inherit. `fwi()`'s refusal
of negative precipitation, wind or humidity is `Step` returning NaN in every field.

**Where it follows cffdrs rather than Van Wagner (1987)**, and is checked to: the
latitude-adjusted day lengths (identical to the 1987 tables north of 30 °N, so
all of Canada and Scandinavia see no difference); FFMC's exact moisture constant;
the altered DMC rain equations, which cost about 0.11 % of DMC per rain day
against the paper; FFMC clamped to [0, 101]; and the RH clamp in `Step`. The
package doc and [MIGRATION.md](MIGRATION.md) list each with its evidence.

**Correctness.** Ten `TestCFFDRS*` assert it against the same pinned oracle as
`fbp`: every component over grids that reach every branch (each rain threshold
from both sides, both temperature floors, RH 0 and 100, zero and 150 km/h wind,
every month, every `lat.adjust` band of both the DMC and the DC), 3,420 one-day
`fwi()` runs, ten chains of 40 to 120 days through `fwi()` itself with the Go
carrying its own state, cffdrs' own `test_fwi` sample, and 2,270 cells of
`fwiRaster()` grids, NA cells included. The worst relative error anywhere is
1e-14.

## Fuel codes

Implemented: `C1`–`C7`, `D1`, `M1`–`M4`, `S1`–`S3`, `O1A`, `O1B` — the fifteen
fuel types of ST-X-3 plus `M3` and `M4`, the dead-balsam-fir mixedwoods from the
2009 revision. That is every fuel `cffdrs` implements.

Case and separators do not matter, so `O1a`, `o1b` and `C-2` reach the same
coefficients as `O1A`, `O1B` and `C2`. The lowercase grass spellings are the ones
ST-X-3 itself prints, and a raster labelled the way the source document labels it
must not read as a different fuel.

### The two mixedwood blends take different inputs

| Fuel | Weighted by | Against | Equation |
|---|---|---|---|
| `M1`, `M2` | `pc` — percent conifer | the C2 curve | eq. 27 |
| `M3`, `M4` | `pdf` — percent dead balsam fir | the fuel's own eq. 30 curve | eqs. 29, 33 |

They are **not interchangeable**. Both are "how much of this stand is the
flammable component" and both are percentages, which is exactly why they are
separate parameters rather than one — a fuel map carrying both carries them in
different columns, and a single parameter serving both would turn a transposed
column into a plausible number instead of a compile error. `M2` and `M4` each
carry a 0.2 weight on their deciduous half; `M1` and `M3` do not.

### Not implemented

`D2`, and `cffdrs`'s non-fuel classes `WA` and `NF`.

That needs a decision from you, because the API cannot make it for you. Every
function here returns a `float64`, so an unimplemented fuel arrives as a spread
rate of **0** — which is exactly what a cell that will not carry fire returns.
`CanonicalFuelCode` is the only thing that tells the two apart:

```go
code, ok := fuel.CanonicalFuelCode(rasterClass)
if !ok {
	// A typo, a non-FBP class name, or D2. Decide here — not downstream,
	// where it is a zero with no reason attached.
	return fmt.Errorf("fuel %q is not implemented", rasterClass)
}
```

Check it where fuel classes enter, once per class rather than once per cell. The
already-canonical path does not allocate, so per-cell is affordable too.

## Crown fire: the threshold, not the inputs

`CrownFractionBurned` answers whether the fire is still in the surface fuels, and
`DescribeFire` folds that into the published system's three classes — surface,
intermittent crown, continuous crown.

It does **not** change any spread rate, and that is the published system's own
behaviour rather than a shortcut. In FBP the final rate of spread is the surface
rate for every fuel type except C6, which alone has a separate crown rate of
spread blended in through CFB. So for sixteen of the seventeen fuels the spread
rate was already right; what was missing was the statement of what kind of fire
it describes.

C6 is the seventeenth, and it has its own call — see
[C6: the one fuel whose rate of spread is not the surface rate](#c6-the-one-fuel-whose-rate-of-spread-is-not-the-surface-rate).
`ROS` still returns the surface rate for every fuel including C6, because its
signature has nowhere to put the FMC, SFC, CBH and CFL the blend needs.

You supply the inputs. `Crown` takes foliar moisture content (FMC), surface fuel
consumption (SFC), crown base height (CBH) and crown fuel load (CFL). All four
have a source in this package — `FoliarMoistureContent`, `SurfaceFuelConsumption`,
and the published per-fuel CBH/CFL tables behind `DefaultCrownBaseHeight` and
`DefaultCrownFuelLoad` — but `Crown` will not call any of them for you. For the
two tables the reason is that a stand you have measured should not be quietly
replaced by a published average; for the other two: one would have to invent a grass fuel load, the other a location and a
date.

```go
fmc := crown.FoliarMoistureContent(55, 120 /* °W */, 0 /* elev */, 200 /* day */, 0)
sfc := consumption.SurfaceFuelConsumption("C2", ffmc, bui, 0 /* pc */, 0 /* gfl */)

cfb := crown.CrownFractionBurned(crown.Crown{
    FMC: fmc, SFC: sfc, CBH: 3, CFL: 0.8,
    SurfaceROS: surfaceROS, // RSI·BE on the net effective wind — NOT RSI·BE·SF
})
fd := crown.DescribeFire(cfb) // "S", "I" or "C"
```

`SurfaceROS` must be the surface rate from the full slope path, not `ROS`'s
`RSI · BE · SF` product. The slope is already inside the net effective wind, and
feeding the simplified product here over-predicts crowning by the whole slope
factor — up to tenfold, straight into an exponential.

## C6: the one fuel whose rate of spread is not the surface rate

C6 (conifer plantation) has a crown rate of spread RSC of its own, and once the
fire is in the crowns the published answer is a CFB-weighted blend of the surface
and crown rates. That blend is what `cffdrs` reports in its ROS column for C6,
and `C6CrownFire` is how you get it here.

```go
// RSS, the surface rate, assembled the ordinary way. Eqs. 62 and 63 are just the
// generic curve at C6's table row, so RSI and BuildupEffect already cover them.
isi := spread.ISI(ffmc, windKmh)
rss := spread.RSI("C6", isi, 0, 0, 0) * spread.BuildupEffect("C6", bui)

ros, cfb := crown.C6CrownFire(crown.Crown{
    FMC: fmc, SFC: sfc, CBH: 7, CFL: 0.8,
    SurfaceROS: rss,
}, isi)
fd := crown.DescribeFire(cfb)
```

Three things about it are easy to get the wrong way round:

- **C6's CFB is not computed from its crown rate.** It is eq. 58 on the *surface*
  rate, the same equation every other fuel uses. RSC only gates whether that
  value is reported at all — `C6CrownFractionBurned` adds the gate and nothing
  else.
- **RSC does not depend on BUI.** Eq. 64 is a function of ISI and foliar moisture
  alone, so a drought that raises BUI moves C6's surface rate and leaves its
  crown rate where it was.
- **The blend saturates.** RSC approaches `60·FME/FMEAvg` as ISI grows, which at
  FMC 100 is 56.9 m/min. No wind produces a faster C6 crown fire at that
  moisture.

`SurfaceROS` carries the same contract it does for the threshold: the surface
rate from the full slope path, not `ROS`'s `RSI · BE · SF` product.

## What is not implemented

**D2, and the non-fuel classes WA and NF.** See [Fuel codes](#fuel-codes) — an
unimplemented fuel comes back as a spread rate of zero, so screen codes with
`CanonicalFuelCode` rather than reading the number.

**Defaults applied for you.** The per-fuel CBH and CFL tables, FMC and SFC are all
ported and oracle-asserted, but every one of them is a function you call, never a
default `Crown` applies. That is deliberate and it will stay that way; the one
consequence worth knowing is that CFL is what keeps the fuels with no crown (D1,
S1–S3, O1A, O1B) reporting zero, so a caller supplying their own CFL for those
fuels is overriding the published 0. `CrownFuelConsumption` inherits that: it
multiplies by the CFL you hand it and applies no per-fuel gate of its own.

**C6 on sloped ground.** C6's crown rate of spread is implemented (above), but
the slope back-solve for that fuel is not, and the two are separate gaps.
`cffdrs` derives the slope-equivalent wind from `rate_of_spread()`, which for C6
returns the *blended* rate — so its equivalent wind for a sloped C6 stand moves
with FMC and CBH, and no surface-only inversion reproduces it.
`NetEffectiveWind` is surface-only for every fuel. On flat ground the two agree
exactly; on a slope, feed `C6CrownFire` an ISI you trust rather than one
back-solved here.

Also absent: the rate at a bearing among `cffdrs::fbp()`'s outputs (TROS and
everything built on it — see `ROSAtAngle`), and `fbp()`'s ISI override and
BUIEFF switch. The flank and back intensities came off this list with
`Predict`. HFI came off
this list on 2026-10-21 (`FireIntensity`, above).

The three paragraphs above are a summary. [MIGRATION.md](MIGRATION.md) is the
full account: every file in `cffdrs`'s `R/` with a status, what is deliberately
out of scope and why, and the order the remaining pieces have to be done in —
they are not independent, and porting them out of order produces code the oracle
cannot yet check.

## Slope: two paths, and why `ROS` is an upper bound

The FBP System does not treat slope as a multiplier. It routes slope through a
zero-wind spread rate, back-solves the **equivalent wind** that the slope alone
implies, and vector-adds that to the real wind. `NetEffectiveWind` does this, and
it is why wind blowing *downhill* can cancel a slope rather than being multiplied
by it.

`ROS` takes the older, simpler path — `RSI · BE · SF` — because the back-solve
needs FFMC and wind, and a caller with neither cannot run it. Since `SF` is a
multiplier it applies whole regardless of wind direction, so `ROS` on sloped
ground is an **upper bound, never an under-estimate**:

| wind vs. slope | median | p95 | worst |
|---|---|---|---|
| driving upslope | 1.28x | 5.91x | 7.61x |
| cross-slope | 2.02x | 5.95x | 7.77x |
| opposing slope | 3.29x | 67x | 99x |

Note the upslope row: even with the wind helping, `ROS` is not a safe stand-in on
steep dry ground.

**With all four inputs**, do what the example above does — take `WSV` from
`NetEffectiveWind`, feed `ISI(FFMC, WSV)` into `ROS`, and pass `slopePct = 0`.
The slope is already inside `WSV`; passing it again counts it twice.

At zero wind the two paths usually agree exactly. Two exceptions, neither exotic:
no mixedwood satisfies it (eqs. 42/42b/42c blend ISF, not RSF), and neither does any fuel
where the RSI clamp or the equivalent-wind cap binds — which on dry steep ground
is most of them. Both leave `ROS` reading high.

## Correctness

Every coefficient here was transcribed by hand from ST-X-3, and a transcription
error looks exactly like correct code. The `TestCFFDRS*` tests assert this
package against [cffdrs](https://cran.r-project.org/package=cffdrs), maintained
by the Canadian Forest Service authors of the FBP System — the only oracle that
can say the tables are *right* rather than merely self-consistent.

RSI, BE and SF reproduce it exactly across all 17 fuel types. ISI, WSV and the
full slope path reproduce it to machine precision over every sloped case.

It has earned that twice:

- A missing grass-curing branch had fully-cured grass spreading **2.2x too
  fast**. The error came from the 1992 source itself, so a second careful
  transcription would have agreed precisely in being wrong.
- The FFMC moisture constant was the rounded 147.2 rather than the exact
  250·59.5/101 — a systematic ~1e-3 relative bias in ISI that every back-solved
  equivalent wind would have inherited in the same direction.

`ROSAtAngle`'s ellipse geometry is pinned by exact identities at 0° and 180° plus
a shape assertion rather than by the oracle. Not for want of a column: cffdrs
offers two rates at a bearing and **both are defective**. `fbp()`'s `TROS`
subtracts a value in degrees (`RAZ`) from one in radians (`THETA`) inside a
single `cos()`, so it does not return `ROS` even at the head; and eq. 94's
`rate_of_spread_at_theta` — unexported, never called by `fbp()` — has a pole at
90° behind a guard that never fires, returns negative rates between 30° and 90°,
and yields `ROS` at both 0° and 180° so `BROS` never comes back. Measured against
the pinned oracle on 2026-09-18; see `ROSAtAngle`'s doc comment and
[MIGRATION.md](MIGRATION.md).

**The `Go` workflow does not run the oracle.** The 15.8 MB fixture is generated
rather than committed, so the thirty-eight fixture-backed tests skip on a fresh clone
and a green `Go` badge means the identities, round-trips, invariants and NaN
sweeps pass.

The `Oracle` workflow is the other half. It builds the pinned cffdrs container,
regenerates the fixture and runs the `TestCFFDRS*` tests for real, monthly and on
demand. Its badge is the one that says the coefficient tables are still right.

It is not on push, because every run rebuilds an R and GDAL image from scratch.
After changing a coefficient or the generator, dispatch it — or just run
`./testdata/regen-cffdrs.sh` locally, which is the same thing.

## Capabilities

This package is arithmetic on floats. It opens no files, makes no network calls,
runs no subprocesses, uses no reflection and no cgo — importing it adds nothing
to a caller's attack surface.

That is asserted rather than claimed, and it is an invariant rather than a
baseline. The `Go` workflow runs [Capslock](https://github.com/google/capslock)
against an empty capability set and fails the build if the transitive call graph
grows one. There is no `capabilities.json` to regenerate: the expected value is
written in the workflow, so widening it means editing that file, in the diff, on
purpose.

The check exists because this failure is quiet. Reaching for `os.Getenv` to read
a fuel table, or a logging package that opens a file, breaks no test at all; it
changes what every consumer is trusting, and it does not look like anything in a
diff full of coefficients.

To run it locally:

```
go install github.com/google/capslock/cmd/capslock@v0.3.3
echo '{}' > /tmp/no-capabilities.json
capslock -packages=./... -output=compare /tmp/no-capabilities.json
```

A failure names the capability and the call path that introduced it. Test files
are out of scope: the fixture tests read `testdata` and shell out to Docker,
which is fine, and is not what a consumer links.

## Regenerating the fixture

```
./testdata/regen-cffdrs.sh
```

Needs Docker and nothing else; it pins R 4.6.1 and cffdrs 1.9.2 in a container.
See [testdata/README.md](testdata/README.md) for why the fixture is not committed
and how to read a regeneration.

## References

- Forestry Canada Fire Danger Group (1992). *Development and Structure of the
  Canadian Forest Fire Behavior Prediction System.* Information Report ST-X-3.
  Coefficient tables 6–7; equations throughout.
- Wotton, B.M., Alexander, M.E. & Taylor, S.W. (2009). *Updates and Revisions to
  the 1992 Canadian Forest Fire Behavior Prediction System.* Information Report
  GLC-X-10. — the grass curing revision in `CuringFactor`.
- Van Wagner, C.E. (1987). *Development and Structure of the Canadian Forest Fire
  Weather Index System.* Forestry Technical Report 35. — the fine-fuel moisture
  term (eq. 10) and ISI (eqs. 24–26).
- Wang, X., Wotton, B.M., Cantin, A.S., Parisien, M.-A., Anderson, K., Moore, B.
  & Flannigan, M.D. *cffdrs: Canadian Forest Fire Danger Rating System.* R
  package — the oracle these tests assert against.

## Contributing

Issues and pull requests welcome. `go test ./...` is the whole build. A change to
any coefficient or equation needs the oracle: regenerate the fixture, run
`go test ./... -run TestCFFDRS`, and say in the PR what the reference numbers did.

To port something still missing, start from [MIGRATION.md](MIGRATION.md)'s
dependency order rather than from whichever row looks smallest, and decide how
the oracle will assert it *before* writing the Go — for two of the remaining
rows the fixture column already exists, and for one of them no column can exist
at all. [DAILY-CHECK.md](DAILY-CHECK.md) is the procedure that keeps the ledger
matching the code, including on the days nothing moves. `/migration-check` and
`/migration-port` run the two halves of it.

## Licence

MIT — see [LICENSE](LICENSE). The `cffdrs` R package used to generate the test
fixture is GPL-2 and is not redistributed here; see
[testdata/README.md](testdata/README.md).
